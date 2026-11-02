package services

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/auth"
	"github.com/coredev-uk/sona-go/internal/auth/store"
	"github.com/coredev-uk/sona-go/internal/catalog"
	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/desktop/notify"
)

// testClock is a settable clock safe to read from the reminder loop.
type testClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *testClock) set(t time.Time) {
	c.mu.Lock()
	c.t = t
	c.mu.Unlock()
}

// fakeNotifications records every notification shown and keeps each
// notifier's action callback.
type fakeNotifications struct {
	shown   chan notify.Notification
	mu      sync.Mutex
	actions []func(string)
}

type fakeNotifier struct{ f *fakeNotifications }

func (n fakeNotifier) Show(_ context.Context, msg notify.Notification) error {
	n.f.shown <- msg
	return nil
}
func (fakeNotifier) Dismiss(context.Context) error { return nil }
func (fakeNotifier) Close()                        {}

func (f *fakeNotifications) connect(_ context.Context, onAction func(string)) (Notifier, error) {
	f.mu.Lock()
	f.actions = append(f.actions, onAction)
	f.mu.Unlock()
	return fakeNotifier{f}, nil
}

// releaseDates serves catalog album pages whose release dates the test
// changes.
type releaseDates struct {
	mu    sync.Mutex
	dates map[string]string
}

func (r *releaseDates) set(id, date string) {
	r.mu.Lock()
	r.dates[id] = date
	r.mu.Unlock()
}

func (r *releaseDates) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	id, ok := strings.CutPrefix(req.URL.Path, "/v1/catalog/gb/albums/")
	r.mu.Lock()
	date, known := r.dates[id]
	r.mu.Unlock()
	if !ok || !known {
		http.NotFound(w, req)
		return
	}
	_, _ = fmt.Fprintf(w, `{"data":[{"id":%q,"type":"albums","attributes":{"name":"Album %s","artistName":"Band","releaseDate":%q,
		"artwork":{"url":"https://is1-ssl.mzstatic.com/image/thumb/x/{w}x{h}bb.jpg"}}}]}`, id, id, date)
}

type remindersHarness struct {
	s     *RemindersService
	e     *Env
	clock *testClock
	apple *releaseDates
	notes *fakeNotifications
	emit  *recorder
}

func newRemindersHarness(t *testing.T, notifications bool) *remindersHarness {
	t.Helper()
	d, err := db.Open(t.Context(), filepath.Join(t.TempDir(), "sona.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	apple := &releaseDates{dates: map[string]string{}}
	srv := httptest.NewServer(apple)
	t.Cleanup(srv.Close)
	api := applemusic.New(srv.Client(), staticToken{}, func() string { return "" })
	api.Base, api.AMPBase = srv.URL, srv.URL
	sf := func() string { return "gb" }
	authm, err := auth.NewManager(auth.Config{Dev: testDev{}, Open: func() (store.Store, error) { return &recordStore{}, nil }, Accounts: AccountStore{DB: d}, Logger: discard()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(authm.Close)
	h := &remindersHarness{
		clock: &testClock{t: localDay(t, "2026-11-01").Add(14 * time.Hour)},
		apple: apple,
		notes: &fakeNotifications{shown: make(chan notify.Notification, 4)},
		emit:  &recorder{},
	}
	var plat Platform
	if notifications {
		plat.Notifications = h.notes.connect
	}
	h.e = &Env{Deps: Deps{
		DB: d, API: api, Catalog: catalog.New(d, api, sf, nil), Storefront: sf, Gate: NewGate(time.Hour, nil),
		Auth: authm, Emit: h.emit, Logger: discard(), Now: h.clock.Now, Platform: plat,
	}}
	h.s = newRemindersService(h.e)
	t.Cleanup(h.e.close)
	return h
}

func (h *remindersHarness) note(t *testing.T) notify.Notification {
	t.Helper()
	select {
	case n := <-h.notes.shown:
		return n
	case <-time.After(5 * time.Second):
		t.Fatal("no notification")
		return notify.Notification{}
	}
}

func (h *remindersHarness) noNote(t *testing.T) {
	t.Helper()
	select {
	case n := <-h.notes.shown:
		t.Fatalf("unexpected notification %+v", n)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestRemindersNeedNotificationsAndAPrerelease(t *testing.T) {
	ctx := t.Context()
	h := newRemindersHarness(t, false)
	h.apple.set("7", "2026-11-19")
	if _, err := h.s.Set(ctx, "7", "Out now"); CodeOf(err) != CodeUnsupported {
		t.Fatalf("without notifications: %v, want unsupported", err)
	}

	h = newRemindersHarness(t, true)
	h.apple.set("8", "2026-10-30")
	h.apple.set("9", "2027")
	for _, id := range []string{"8", "9"} {
		if _, err := h.s.Set(ctx, id, "Out now"); CodeOf(err) != CodeInvalidArgument {
			t.Fatalf("album %s: %v, want invalid_argument", id, err)
		}
	}
	h.apple.set("7", "2026-11-19")
	if _, err := h.s.Set(ctx, "7", strings.Repeat("x", maxReminderMessage+1)); CodeOf(err) != CodeInvalidArgument {
		t.Fatalf("long message: %v", err)
	}
	if _, err := h.s.Set(ctx, "404", "Out now"); CodeOf(err) != CodeNotFound {
		t.Fatalf("unknown album: %v", err)
	}
}

// A reminder set while the album is upcoming fires once, after the first
// paint of the next start when its release passed while Sona was closed,
// and its click opens the album.
func TestReminderFiresOnceAfterFirstPaint(t *testing.T) {
	ctx := t.Context()
	h := newRemindersHarness(t, true)
	h.apple.set("7", "2026-11-19")
	r, err := h.s.Set(ctx, "7", "Out now · Band")
	if err != nil {
		t.Fatal(err)
	}
	release := localDay(t, "2026-11-19")
	if r.AlbumID != "7" || r.Name != "Album 7" || r.Artist != "Band" || r.ReleaseAt != release.UnixMilli() {
		t.Fatalf("reminder = %+v", r)
	}
	if list, _ := h.s.List(ctx); len(list) != 1 || list[0] != r {
		t.Fatalf("list = %+v", list)
	}
	if h.emit.events[0] != EventRemindersChanged {
		t.Fatalf("events = %v", h.emit.events)
	}

	// The release passes before the first paint: nothing is shown yet.
	h.clock.set(release.Add(3 * 24 * time.Hour))
	h.noNote(t)
	h.e.Gate.Open()
	n := h.note(t)
	if n.Summary != "Album 7" || n.Body != "Out now · Band" || n.Urgency != notify.UrgencyNormal || len(n.Actions) != 1 || n.Actions[0].Key != notify.DefaultAction {
		t.Fatalf("notification = %+v", n)
	}
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		if list, _ := h.s.List(ctx); len(list) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("fired reminder kept")
		}
	}
	h.s.poke()
	h.noNote(t)

	h.notes.mu.Lock()
	click := h.notes.actions[0]
	h.notes.mu.Unlock()
	click(notify.DefaultAction)
	h.emit.mu.Lock()
	last := h.emit.data[len(h.emit.data)-1]
	h.emit.mu.Unlock()
	if last != (OpenTarget{Kind: "album", ID: "7"}) {
		t.Fatalf("click emitted %+v", last)
	}
}

// When Apple moves the release later, the reminder waits for the new date;
// Remove drops one before it fires.
func TestReminderFollowsAMovedRelease(t *testing.T) {
	ctx := t.Context()
	h := newRemindersHarness(t, true)
	h.e.Gate.Open()
	h.apple.set("7", "2026-11-19")
	h.apple.set("8", "2026-11-20")
	for _, id := range []string{"7", "8"} {
		if _, err := h.s.Set(ctx, id, "Out now"); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.s.Remove(ctx, "8"); err != nil {
		t.Fatal(err)
	}
	h.apple.set("7", "2026-12-04")
	if _, err := h.e.Catalog.ClearCache(ctx); err != nil {
		t.Fatal(err)
	}
	moved := localDay(t, "2026-12-04")
	h.clock.set(localDay(t, "2026-11-21"))
	h.s.poke()
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		list, _ := h.s.List(ctx)
		if len(list) == 1 && list[0].ReleaseAt == moved.UnixMilli() {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("reminders = %+v, want album 7 moved to %d", list, moved.UnixMilli())
		}
	}
	h.noNote(t)
	h.clock.set(moved)
	h.s.poke()
	if n := h.note(t); n.Summary != "Album 7" {
		t.Fatalf("notification = %+v", n)
	}
	h.noNote(t)
}

func TestRemindersClearOnSignOut(t *testing.T) {
	ctx := t.Context()
	h := newRemindersHarness(t, true)
	h.apple.set("7", "2026-11-19")
	if _, err := h.s.Set(ctx, "7", "Out now"); err != nil {
		t.Fatal(err)
	}
	if err := h.e.Auth.SignOut(ctx); err != nil {
		t.Fatal(err)
	}
	if list, err := h.s.List(ctx); err != nil || list == nil || len(list) != 0 {
		t.Fatalf("list after sign-out = %#v, %v", list, err)
	}
	h.emit.mu.Lock()
	defer h.emit.mu.Unlock()
	if last := h.emit.data[len(h.emit.data)-1]; len(last.([]Reminder)) != 0 {
		t.Fatalf("last event = %+v", last)
	}
}
