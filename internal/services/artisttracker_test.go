package services

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/auth"
	"github.com/coredev-uk/sona-go/internal/auth/store"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/db/dbq"
	"github.com/coredev-uk/sona-go/internal/desktop/notify"
)

// trackerRelease is an album the fake catalogue lists for an artist.
type trackerRelease struct {
	id, name, date string
	single, pre    bool
}

// trackerApple is the fake Apple: the artists rated 1 (/v1/me/ratings/artists)
// and each artist's releases (/v1/catalog/gb/artists?ids=…&views=…). It
// counts every request.
type trackerApple struct {
	mu       sync.Mutex
	rated    map[string]bool
	releases map[string][]trackerRelease // newest first, as Apple's views are
	requests []string                    // "METHOD path ids"
	fail     bool                        // answer catalogue requests with 400
}

func (a *trackerApple) set(artist string, rs ...trackerRelease) {
	a.mu.Lock()
	a.releases[artist] = rs
	a.mu.Unlock()
}

func (a *trackerApple) rate(on bool, ids ...string) {
	a.mu.Lock()
	for _, id := range ids {
		a.rated[id] = on
	}
	a.mu.Unlock()
}

func (a *trackerApple) log() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.requests)
}

func (a *trackerApple) catalogueRequests() []string {
	var out []string
	for _, r := range a.log() {
		if strings.Contains(r, "/catalog/") {
			out = append(out, r)
		}
	}
	return out
}

func (a *trackerApple) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ids := strings.Split(r.URL.Query().Get("ids"), ",")
	a.mu.Lock()
	defer a.mu.Unlock()
	a.requests = append(a.requests, fmt.Sprintf("%s %s %s", r.Method, r.URL.Path, strings.Join(ids, ",")))
	switch r.URL.Path {
	case "/v1/me/ratings/artists":
		var data []map[string]any
		for _, id := range ids {
			if a.rated[id] {
				data = append(data, map[string]any{"id": id, "type": "ratings", "attributes": map[string]any{"value": 1}})
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	case "/v1/catalog/gb/artists":
		if a.fail {
			http.Error(w, `{"errors":[{"status":"400"}]}`, http.StatusBadRequest)
			return
		}
		if got := r.URL.Query().Get("views"); got != "latest-release,full-albums,singles" {
			http.Error(w, "views="+got, http.StatusBadRequest)
			return
		}
		var data []map[string]any
		for _, id := range ids {
			rs, ok := a.releases[id]
			if !ok {
				continue
			}
			album := func(x trackerRelease) map[string]any {
				return map[string]any{"id": x.id, "type": "albums", "attributes": map[string]any{
					"name": x.name, "artistName": "Artist " + id, "releaseDate": x.date, "isSingle": x.single, "isPrerelease": x.pre,
					"trackCount": 3, "artwork": map[string]any{"url": "https://is1-ssl.mzstatic.com/image/thumb/" + x.id + "/{w}x{h}bb.jpg", "width": 100, "height": 100},
				}}
			}
			views := map[string]any{"latest-release": map[string]any{"data": []any{}}, "full-albums": map[string]any{"data": []any{}}, "singles": map[string]any{"data": []any{}}}
			for i, x := range rs {
				if i == 0 {
					views["latest-release"] = map[string]any{"data": []any{album(x)}}
				}
				v := "full-albums"
				if x.single {
					v = "singles"
				}
				views[v] = map[string]any{"data": append(views[v].(map[string]any)["data"].([]any), album(x))}
			}
			data = append(data, map[string]any{"id": id, "type": "artists", "attributes": map[string]any{"name": "Artist " + id}, "views": views})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	default:
		http.NotFound(w, r)
	}
}

type trackerHarness struct {
	s        *ArtistTrackerService
	e        *Env
	clock    *testClock
	apple    *trackerApple
	notes    *fakeNotifications
	emit     *recorder
	cfg      *config.Store
	d        *db.DB
	signedIn bool
}

func newTrackerHarness(t *testing.T) *trackerHarness {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(t.Context(), filepath.Join(dir, "sona.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	h := &trackerHarness{
		apple: &trackerApple{rated: map[string]bool{}, releases: map[string][]trackerRelease{}},
		clock: &testClock{t: localDay(t, "2026-11-15").Add(10 * time.Hour)},
		notes: &fakeNotifications{shown: make(chan notify.Notification, 8)},
		emit:  &recorder{},
		d:     d, signedIn: true,
	}
	srv := httptest.NewServer(h.apple)
	t.Cleanup(srv.Close)
	api := applemusic.New(srv.Client(), staticToken{}, func() string {
		if h.signedIn {
			return "mut"
		}
		return ""
	})
	api.Base, api.AMPBase = srv.URL, srv.URL
	h.cfg, err = config.OpenStore(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	authm, err := auth.NewManager(auth.Config{Dev: testDev{}, Open: func() (store.Store, error) { return &recordStore{}, nil }, Accounts: AccountStore{DB: d}, Logger: discard()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(authm.Close)
	h.e = &Env{Deps: Deps{
		DB: d, API: api, Storefront: func() string { return "gb" }, Gate: NewGate(time.Hour, nil), Settings: h.cfg,
		Auth: authm, Emit: h.emit, Logger: discard(), Now: h.clock.Now, Platform: Platform{Notifications: h.notes.connect},
	}, m: mapper{now: h.clock.Now}}
	h.s = newArtistTrackerService(h.e)
	h.s.startDelay, h.s.pause = 0, 0
	t.Cleanup(h.e.close)
	return h
}

// library puts artists (catalogue IDs) in the synced library.
func (h *trackerHarness) library(t *testing.T, ids ...string) {
	t.Helper()
	for _, id := range ids {
		rid, err := h.d.Q.UpsertLibraryArtist(t.Context(), dbq.UpsertLibraryArtistParams{
			LibraryID: sql.NullString{String: "r." + id, Valid: true}, CatalogID: sql.NullString{String: id, Valid: true}, Name: "Artist " + id, UpdatedAt: 1,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := h.d.Q.MarkLibraryItem(t.Context(), dbq.MarkLibraryItemParams{Kind: "artist", Ref: rid, Generation: 1}); err != nil {
			t.Fatal(err)
		}
	}
}

func (h *trackerHarness) enable(t *testing.T, desktop bool) {
	t.Helper()
	if _, err := h.cfg.Update(func(s *config.Settings) { s.ArtistTracker = config.ArtistTracker{Enabled: true, Desktop: desktop} }); err != nil {
		t.Fatal(err)
	}
}

func (h *trackerHarness) disable(t *testing.T) {
	t.Helper()
	if _, err := h.cfg.Update(func(s *config.Settings) { s.ArtistTracker = config.ArtistTracker{} }); err != nil {
		t.Fatal(err)
	}
}

// check runs the loop's pass a day after the last one.
func (h *trackerHarness) check(t *testing.T) {
	t.Helper()
	h.clock.set(h.clock.Now().Add(25 * time.Hour))
	h.s.pass()
}

func (h *trackerHarness) releases(t *testing.T) NewReleases {
	t.Helper()
	r, err := h.s.Releases(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func (h *trackerHarness) names(t *testing.T) []string {
	var out []string
	for _, r := range h.releases(t).Items {
		out = append(out, r.Album.Name)
	}
	return out
}

func (h *trackerHarness) noNote(t *testing.T) {
	t.Helper()
	select {
	case n := <-h.notes.shown:
		t.Fatalf("unexpected notification %+v", n)
	case <-time.After(50 * time.Millisecond):
	}
}

func (h *trackerHarness) note(t *testing.T) notify.Notification {
	t.Helper()
	select {
	case n := <-h.notes.shown:
		return n
	case <-time.After(5 * time.Second):
		t.Fatal("no notification")
		return notify.Notification{}
	}
}

// While the setting is off, nothing is asked of Apple, even with the
// first paint done and a loop running.
func TestArtistTrackerOffMakesNoRequests(t *testing.T) {
	h := newTrackerHarness(t)
	h.library(t, "1")
	h.apple.rate(true, "1")
	h.apple.set("1", trackerRelease{id: "10", name: "Old", date: "2026-01-01"})
	h.e.Gate.Open()
	h.s.poke()
	if wait := h.s.pass(); wait != trackerEvery {
		t.Fatalf("wait while off = %v", wait)
	}
	time.Sleep(100 * time.Millisecond)
	if got := h.apple.log(); len(got) != 0 {
		t.Fatalf("requests while off: %v", got)
	}
}

// Nothing is asked before the first paint, whatever the setting says.
func TestArtistTrackerWaitsForTheFirstPaint(t *testing.T) {
	h := newTrackerHarness(t)
	h.library(t, "1")
	h.apple.rate(true, "1")
	h.apple.set("1", trackerRelease{id: "10", name: "Old", date: "2026-01-01"})
	h.enable(t, false)
	time.Sleep(100 * time.Millisecond)
	if got := h.apple.log(); len(got) != 0 {
		t.Fatalf("requests before the first paint: %v", got)
	}
	h.e.Gate.Open()
	for deadline := time.Now().Add(5 * time.Second); len(h.apple.catalogueRequests()) == 0; time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("no check after the first paint")
		}
	}
}

// Turning the tracker on records what is out as seen; only what comes out
// afterwards is listed and announced, once, across checks and a restart.
func TestArtistTrackerSeedsThenFindsNewReleases(t *testing.T) {
	ctx := t.Context()
	h := newTrackerHarness(t)
	h.library(t, "1", "2")
	h.apple.rate(true, "1") // artist 2 is in the library but not a favourite
	h.apple.set("1",
		trackerRelease{id: "11", name: "Latest", date: "2026-11-01"},
		trackerRelease{id: "10", name: "Back catalogue", date: "2024-03-03"})
	h.apple.set("2", trackerRelease{id: "20", name: "Not mine", date: "2026-11-01"})
	h.enable(t, true)

	h.s.pass()
	if got := h.names(t); len(got) != 0 {
		t.Fatalf("first check listed %v, want nothing", got)
	}
	h.noNote(t)
	reqs := h.apple.log()
	if len(reqs) != 2 || !strings.HasPrefix(reqs[0], "GET /v1/me/ratings/artists 1,2") || !strings.HasPrefix(reqs[1], "GET /v1/catalog/gb/artists 1") {
		t.Fatalf("requests = %q, want one ratings call and one catalogue call for the favourite only", reqs)
	}
	// Within the day nothing is asked again.
	if wait := h.s.pass(); wait <= 0 || wait > trackerEvery || len(h.apple.log()) != 2 {
		t.Fatalf("second pass: wait %v, requests %q", wait, h.apple.log())
	}

	// A release of the favourite, and one of the artist that isn't.
	h.apple.set("1",
		trackerRelease{id: "12", name: "Fresh - EP", date: "2026-11-15", single: true},
		trackerRelease{id: "11", name: "Latest", date: "2026-11-01"},
		trackerRelease{id: "10", name: "Back catalogue", date: "2024-03-03"})
	h.apple.set("2", trackerRelease{id: "21", name: "Still not mine", date: "2026-11-15"})
	h.check(t)
	list := h.releases(t)
	if len(list.Items) != 1 || list.Unread != 1 || list.CheckedAt == 0 {
		t.Fatalf("list = %+v", list)
	}
	r := list.Items[0]
	if r.Album.CatalogID != "12" || r.Album.Name != "Fresh - EP" || r.Kind != ReleaseEP || r.Read || r.Album.Prerelease || r.Album.Artist != "Artist 1" || r.Album.ArtistID != "1" || !r.Album.Single {
		t.Fatalf("release = %+v", r)
	}
	n := h.note(t)
	if n.Summary != "Fresh - EP" || n.Body != "New EP by Artist 1" || len(n.Actions) != 1 || n.Actions[0].Key != notify.DefaultAction {
		t.Fatalf("notification = %+v", n)
	}
	h.notes.mu.Lock()
	click := h.notes.actions[len(h.notes.actions)-1]
	h.notes.mu.Unlock()
	click(notify.DefaultAction)
	h.emit.mu.Lock()
	open := h.emit.data[len(h.emit.data)-1]
	h.emit.mu.Unlock()
	if open != (OpenTarget{Kind: "album", ID: "12"}) {
		t.Fatalf("click emitted %+v", open)
	}

	// The same release in the next check, and after a restart, is not new.
	h.check(t)
	h.noNote(t)
	if got := h.names(t); !slices.Equal(got, []string{"Fresh - EP"}) {
		t.Fatalf("list after another check = %v", got)
	}
	h.e.close()
	h.s = newArtistTrackerService(h.e)
	h.check(t)
	h.noNote(t)
	if got := h.releases(t); len(got.Items) != 1 || got.Unread != 1 {
		t.Fatalf("list after a restart = %+v", got)
	}
	_ = ctx
}

func TestArtistTrackerOneSummaryForSeveralReleases(t *testing.T) {
	h := newTrackerHarness(t)
	h.library(t, "1", "2")
	h.apple.rate(true, "1", "2")
	h.apple.set("1", trackerRelease{id: "10", name: "Old", date: "2025-01-01"})
	h.apple.set("2", trackerRelease{id: "20", name: "Old", date: "2025-01-01"})
	h.enable(t, true)
	h.s.pass()
	h.apple.set("1", trackerRelease{id: "11", name: "One", date: "2026-11-14"}, trackerRelease{id: "10", name: "Old", date: "2025-01-01"})
	h.apple.set("2", trackerRelease{id: "21", name: "Two", date: "2026-11-15", single: true}, trackerRelease{id: "20", name: "Old", date: "2025-01-01"})
	h.check(t)
	n := h.note(t)
	if n.Summary != "New releases" || n.Body != "From your favourite artists: 2" {
		t.Fatalf("notification = %+v", n)
	}
	h.noNote(t)
	h.notes.mu.Lock()
	click := h.notes.actions[len(h.notes.actions)-1]
	h.notes.mu.Unlock()
	click(notify.DefaultAction)
	h.emit.mu.Lock()
	open := h.emit.data[len(h.emit.data)-1]
	h.emit.mu.Unlock()
	if open != (OpenTarget{Kind: OpenNewReleases}) {
		t.Fatalf("click emitted %+v", open)
	}
	if got := h.names(t); !slices.Equal(got, []string{"Two", "One"}) {
		t.Fatalf("list = %v, want newest first", got)
	}
}

// Without the desktop option the list still fills but nothing is shown.
func TestArtistTrackerDesktopOptionIsSeparate(t *testing.T) {
	h := newTrackerHarness(t)
	h.library(t, "1")
	h.apple.rate(true, "1")
	h.apple.set("1", trackerRelease{id: "10", name: "Old", date: "2025-01-01"})
	h.enable(t, false)
	h.s.pass()
	h.apple.set("1", trackerRelease{id: "11", name: "New", date: "2026-11-15"}, trackerRelease{id: "10", name: "Old", date: "2025-01-01"})
	h.check(t)
	h.noNote(t)
	if got := h.names(t); !slices.Equal(got, []string{"New"}) {
		t.Fatalf("list = %v", got)
	}
}

// A release still to come is listed (so "Remind me" is there) but is no
// notification; once found it isn't announced again when it comes out.
func TestArtistTrackerListsUpcomingReleasesWithoutNotifying(t *testing.T) {
	h := newTrackerHarness(t)
	h.library(t, "1", "2")
	h.apple.rate(true, "1", "2")
	h.apple.set("1", trackerRelease{id: "10", name: "Soon", date: "2026-12-01", pre: true})
	h.apple.set("2", trackerRelease{id: "20", name: "Old", date: "2025-01-01"})
	h.enable(t, true)
	h.s.pass() // seeding: the upcoming one is listed, already read
	list := h.releases(t)
	if len(list.Items) != 1 || list.Unread != 0 || !list.Items[0].Album.Prerelease || list.Items[0].Album.ReleaseAt != localDay(t, "2026-12-01").UnixMilli() {
		t.Fatalf("list after seeding = %+v", list)
	}
	h.apple.set("2", trackerRelease{id: "21", name: "Announced", date: "2027-01-15", pre: true}, trackerRelease{id: "20", name: "Old", date: "2025-01-01"})
	h.check(t)
	h.noNote(t)
	list = h.releases(t)
	if len(list.Items) != 2 || list.Unread != 1 || list.Items[0].Album.Name != "Announced" {
		t.Fatalf("list = %+v", list)
	}
	// It comes out: the same record, no longer upcoming, nothing new.
	h.clock.set(localDay(t, "2026-12-02"))
	h.s.pass()
	h.clock.set(localDay(t, "2026-12-04"))
	h.check(t)
	h.noNote(t)
	list = h.releases(t)
	if len(list.Items) != 2 || list.Items[1].Album.Prerelease {
		t.Fatalf("list after the release = %+v", list)
	}
}

// A release found for the first time that is long out is the catalogue
// growing, not news.
func TestArtistTrackerIgnoresOldReleasesItFindsLate(t *testing.T) {
	h := newTrackerHarness(t)
	h.library(t, "1")
	h.apple.rate(true, "1")
	h.apple.set("1", trackerRelease{id: "10", name: "Old", date: "2025-01-01"})
	h.enable(t, true)
	h.s.pass()
	h.apple.set("1", trackerRelease{id: "10", name: "Old", date: "2025-01-01"}, trackerRelease{id: "9", name: "Filed late", date: "2020-05-05"})
	h.check(t)
	h.noNote(t)
	if got := h.names(t); len(got) != 0 {
		t.Fatalf("list = %v", got)
	}
}

func TestArtistTrackerMarksReadDismissesAndClears(t *testing.T) {
	ctx := t.Context()
	h := newTrackerHarness(t)
	h.library(t, "1")
	h.apple.rate(true, "1")
	h.apple.set("1", trackerRelease{id: "10", name: "Old", date: "2025-01-01"})
	h.enable(t, false)
	h.s.pass()
	h.apple.set("1",
		trackerRelease{id: "13", name: "C", date: "2026-11-15"}, trackerRelease{id: "12", name: "B", date: "2026-11-14"},
		trackerRelease{id: "11", name: "A", date: "2026-11-13"}, trackerRelease{id: "10", name: "Old", date: "2025-01-01"})
	h.check(t)
	if l := h.releases(t); l.Unread != 3 {
		t.Fatalf("unread = %d", l.Unread)
	}
	if err := h.s.MarkRead(ctx, "13"); err != nil {
		t.Fatal(err)
	}
	if l := h.releases(t); l.Unread != 2 || !l.Items[0].Read {
		t.Fatalf("after MarkRead: %+v", l)
	}
	if err := h.s.Dismiss(ctx, "12"); err != nil {
		t.Fatal(err)
	}
	if got := h.names(t); !slices.Equal(got, []string{"C", "A"}) {
		t.Fatalf("after Dismiss: %v", got)
	}
	h.check(t) // a dismissed release is not found again
	if got := h.names(t); !slices.Equal(got, []string{"C", "A"}) {
		t.Fatalf("after another check: %v", got)
	}
	if err := h.s.MarkAllRead(ctx); err != nil {
		t.Fatal(err)
	}
	if l := h.releases(t); l.Unread != 0 || len(l.Items) != 2 {
		t.Fatalf("after MarkAllRead: %+v", l)
	}
	if err := h.s.Clear(ctx); err != nil {
		t.Fatal(err)
	}
	h.check(t)
	if l := h.releases(t); len(l.Items) != 0 || l.Unread != 0 {
		t.Fatalf("after Clear: %+v", l)
	}
	if err := h.s.MarkRead(ctx, ""); CodeOf(err) != CodeInvalidArgument {
		t.Fatalf("MarkRead(\"\"): %v", err)
	}
	h.emit.mu.Lock()
	defer h.emit.mu.Unlock()
	if !slices.Contains(h.emit.events, EventArtistReleasesChanged) {
		t.Fatalf("events = %v", h.emit.events)
	}
}

// An artist that stops being a favourite leaves the list; favouriting it
// again seeds it quietly.
func TestArtistTrackerFollowsFavourites(t *testing.T) {
	h := newTrackerHarness(t)
	h.library(t, "1")
	h.apple.rate(true, "1")
	h.apple.set("1", trackerRelease{id: "10", name: "Old", date: "2025-01-01"})
	h.enable(t, true)
	h.s.pass()
	h.apple.set("1", trackerRelease{id: "11", name: "New", date: "2026-11-15"}, trackerRelease{id: "10", name: "Old", date: "2025-01-01"})
	h.check(t)
	h.note(t)
	h.apple.rate(false, "1")
	h.check(t)
	if got := h.names(t); len(got) != 0 {
		t.Fatalf("list after un-favouriting = %v", got)
	}
	h.apple.rate(true, "1")
	h.check(t)
	h.noNote(t)
	if got := h.names(t); len(got) != 0 {
		t.Fatalf("list after favouriting again = %v", got)
	}
}

// A favourite made in Sona, whose artist isn't in the synced library, is a
// candidate too.
func TestArtistTrackerReadsFavouritesMadeInSona(t *testing.T) {
	h := newTrackerHarness(t)
	h.apple.rate(true, "5")
	h.apple.set("5", trackerRelease{id: "50", name: "Old", date: "2025-01-01"})
	if err := h.d.Q.UpsertFavorite(t.Context(), dbq.UpsertFavoriteParams{Kind: "artists", AppleID: "5", Value: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	h.enable(t, false)
	h.s.pass()
	if got := h.apple.catalogueRequests(); len(got) != 1 || !strings.HasSuffix(got[0], " 5") {
		t.Fatalf("catalogue requests = %q", got)
	}
}

// A long list of favourites is asked about in batches.
func TestArtistTrackerBatchesTheCatalogue(t *testing.T) {
	h := newTrackerHarness(t)
	var ids []string
	for i := 1; i <= 60; i++ {
		id := fmt.Sprint(1000 + i)
		ids = append(ids, id)
		h.apple.set(id, trackerRelease{id: "a" + id, name: "Old", date: "2025-01-01"})
	}
	h.library(t, ids...)
	h.apple.rate(true, ids...)
	h.enable(t, false)
	h.s.pass()
	reqs := h.apple.catalogueRequests()
	if len(reqs) != 3 {
		t.Fatalf("catalogue requests = %d, want 3 (25+25+10)", len(reqs))
	}
	for i, want := range []int{25, 25, 10} {
		if got := len(strings.Split(strings.Fields(reqs[i])[2], ",")); got != want {
			t.Errorf("batch %d has %d artists, want %d", i, got, want)
		}
	}
	var n int
	_ = h.d.R.QueryRow(`SELECT count(*) FROM tracked_artists`).Scan(&n)
	if n != 60 {
		t.Fatalf("tracked artists = %d", n)
	}
}

// A failed check keeps nothing half-done as complete, and is tried again
// after a wait that grows.
func TestArtistTrackerRetriesAFailedCheck(t *testing.T) {
	h := newTrackerHarness(t)
	h.library(t, "1")
	h.apple.rate(true, "1")
	h.apple.set("1", trackerRelease{id: "10", name: "Old", date: "2025-01-01"})
	h.apple.fail = true
	h.enable(t, false)
	first := h.s.pass()
	second := h.s.pass()
	if first != trackerRetryMin || second != 2*trackerRetryMin {
		t.Fatalf("waits = %v, %v", first, second)
	}
	if l := h.releases(t); l.CheckedAt != 0 {
		t.Fatalf("a failed check counted: %+v", l)
	}
	h.apple.mu.Lock()
	h.apple.fail = false
	h.apple.mu.Unlock()
	if wait := h.s.pass(); wait != trackerEvery {
		t.Fatalf("wait after success = %v", wait)
	}
	if l := h.releases(t); l.CheckedAt == 0 {
		t.Fatal("successful check not recorded")
	}
}

// Turning the setting on again forgets what was seen as tracked, so releases
// that came out while it was off are not announced.
func TestArtistTrackerReseedsWhenTurnedBackOn(t *testing.T) {
	h := newTrackerHarness(t)
	h.library(t, "1")
	h.apple.rate(true, "1")
	h.apple.set("1", trackerRelease{id: "10", name: "Old", date: "2025-01-01"})
	h.enable(t, true)
	h.s.pass()
	h.disable(t)
	h.apple.set("1", trackerRelease{id: "11", name: "While off", date: "2026-11-14"}, trackerRelease{id: "10", name: "Old", date: "2025-01-01"})
	before := len(h.apple.log())
	h.clock.set(h.clock.Now().Add(48 * time.Hour))
	h.s.pass()
	if len(h.apple.log()) != before {
		t.Fatal("requests while off")
	}
	h.enable(t, true)
	h.s.pass()
	h.noNote(t)
	if got := h.names(t); len(got) != 0 {
		t.Fatalf("list = %v, want the release that came out while off seen, not announced", got)
	}
}

func TestArtistTrackerSignedOutAsksNothing(t *testing.T) {
	h := newTrackerHarness(t)
	h.library(t, "1")
	h.apple.rate(true, "1")
	h.enable(t, true)
	h.signedIn = false
	if wait := h.s.pass(); wait != trackerNoAccount {
		t.Fatalf("wait = %v", wait)
	}
	if got := h.apple.log(); len(got) != 0 {
		t.Fatalf("requests signed out: %v", got)
	}
}

func TestArtistTrackerClearsOnSignOut(t *testing.T) {
	ctx := t.Context()
	h := newTrackerHarness(t)
	h.library(t, "1")
	h.apple.rate(true, "1")
	h.apple.set("1", trackerRelease{id: "10", name: "Old", date: "2025-01-01"})
	h.enable(t, false)
	h.s.pass()
	h.apple.set("1", trackerRelease{id: "11", name: "New", date: "2026-11-15"}, trackerRelease{id: "10", name: "Old", date: "2025-01-01"})
	h.check(t)
	if err := h.e.Auth.SignOut(ctx); err != nil {
		t.Fatal(err)
	}
	l, err := h.s.Releases(ctx)
	if err != nil || l.Items == nil || len(l.Items) != 0 || l.Unread != 0 || l.CheckedAt != 0 {
		t.Fatalf("list after sign-out = %#v, %v", l, err)
	}
	for _, table := range []string{"tracked_artists", "artist_releases", "artist_tracker_state"} {
		var n int
		if err := h.d.R.QueryRow(`SELECT count(*) FROM ` + table).Scan(&n); err != nil || n != 0 {
			t.Fatalf("%s: %d rows, %v", table, n, err)
		}
	}
	h.emit.mu.Lock()
	defer h.emit.mu.Unlock()
	last := h.emit.data[len(h.emit.data)-1]
	if got, ok := last.(NewReleases); !ok || len(got.Items) != 0 {
		t.Fatalf("last event = %+v", last)
	}
}

func TestReleaseKind(t *testing.T) {
	for _, tc := range []struct {
		name   string
		single bool
		want   string
	}{
		{"Album", false, ReleaseAlbum},
		{"Four Songs - EP", true, ReleaseEP},
		{"Song - Single", true, ReleaseSingle},
		{"Song (Remix)", true, ReleaseSingle},
		{"Odd - EP", false, ReleaseEP},
	} {
		if got := releaseKind(tc.name, tc.single); got != tc.want {
			t.Errorf("releaseKind(%q, %t) = %q, want %q", tc.name, tc.single, got, tc.want)
		}
	}
}
