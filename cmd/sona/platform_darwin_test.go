//go:build darwin

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sync"
	"testing"

	"github.com/egoist/mygo"

	"github.com/coredev-uk/sona-go/internal/app"
	"github.com/coredev-uk/sona-go/internal/desktop/notify"
	"github.com/coredev-uk/sona-go/internal/services"
)

func TestInAppBundle(t *testing.T) {
	for path, want := range map[string]bool{
		"/Applications/Sona.app/Contents/MacOS/sona":    true,
		"/tmp/Sona dev.app/Contents/MacOS/sona-dev":     true,
		"/Users/me/sona/bin/sona":                       false, // a source build
		"/Users/me/sona/bin/Sona.app/sona":              false,
		"/Applications/Sona.app/Contents/Resources/sna": false,
		"/Applications/Sona/Contents/MacOS/sona":        false, // no .app
		"/Applications/Sona.app/MacOS/sona":             false, // no Contents
	} {
		if got := inAppBundle(path); got != want {
			t.Errorf("inAppBundle(%q) = %v, want %v", path, got, want)
		}
	}
}

// The test binary is a bare executable: this is the answer outside Sona.app.
func TestRunsFromAppBundleOutsideTheBundle(t *testing.T) {
	if runsFromAppBundle() {
		t.Error("a test binary reports it runs from an app bundle")
	}
}

func TestDialogFilters(t *testing.T) {
	json := app.FileFilter{Name: "Sona settings (JSON)", Patterns: []string{"*.json"}}
	for _, tc := range []struct {
		name string
		in   []app.FileFilter
		want []mygo.FileFilter
	}{
		{"none", nil, nil},
		{"an extension", []app.FileFilter{json}, []mygo.FileFilter{{Name: json.Name, Extensions: []string{"json"}}}},
		{
			"several",
			[]app.FileFilter{{Name: "Pictures", Patterns: []string{"*.png", "*.jpg"}}},
			[]mygo.FileFilter{{Name: "Pictures", Extensions: []string{"png", "jpg"}}},
		},
		{
			"all files",
			[]app.FileFilter{{Name: "Everything", Patterns: []string{"*"}}},
			[]mygo.FileFilter{{Name: "Everything", Extensions: []string{"*"}}},
		},
		// A panel filters by extension only: a name pattern has no meaning
		// there, and a filter left with nothing is not offered.
		{
			"a name pattern",
			[]app.FileFilter{{Name: "Sona", Patterns: []string{"sona*", "*.json"}}},
			[]mygo.FileFilter{{Name: "Sona", Extensions: []string{"json"}}},
		},
		{"nothing expressible", []app.FileFilter{{Name: "Sona", Patterns: []string{"sona*", "*.j?on", "*."}}}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := dialogFilters(tc.in); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("dialogFilters = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestSavePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if got := savePath("/some/folder", "sona-settings.json"); got != "/some/folder/sona-settings.json" {
		t.Errorf("with a folder: %q", got)
	}
	if got := savePath("", ""); got != "" {
		t.Errorf("with nothing: %q", got)
	}
	// With no folder, the panel starts in Documents, or the home folder
	// without one, never in the current directory.
	if got := savePath("", "sona-settings.json"); got != filepath.Join(home, "sona-settings.json") {
		t.Errorf("without Documents: %q", got)
	}
	if err := os.Mkdir(filepath.Join(home, "Documents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := savePath("", "sona-settings.json"); got != filepath.Join(home, "Documents", "sona-settings.json") {
		t.Errorf("with Documents: %q", got)
	}
}

func TestAskReturnsTheAnswerOrGivesUpWithItsContext(t *testing.T) {
	got, err := ask(context.Background(), func() (string, error) { return "/tmp/x.json", nil })
	if got != "/tmp/x.json" || err != nil {
		t.Fatalf("answer = %q, %v", got, err)
	}
	wantErr := errors.New("the panel failed")
	if _, err := ask(context.Background(), func() (string, error) { return "", wantErr }); !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	// A panel MyGo can't close: the caller isn't held by it once its
	// context ends.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	release := make(chan struct{})
	defer close(release)
	if _, err := ask(ctx, func() (string, error) { <-release; return "", nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("a cancelled ask = %v, want context.Canceled", err)
	}
}

func TestInhibitorHoldsOneAssertionAtATime(t *testing.T) {
	var reasons []string
	held := 0
	i := newMacInhibitor(func() string { return "Playing music" })
	i.keepAwake = func(reason string) func() {
		reasons = append(reasons, reason)
		held++
		return func() { held-- }
	}
	if i.Backend() != "" {
		t.Fatalf("before Acquire: %q", i.Backend())
	}
	for range 2 { // a second Acquire holds the same assertion
		if err := i.Acquire(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if held != 1 || !slices.Equal(reasons, []string{"Playing music"}) || i.Backend() != backendPowerAssertion {
		t.Fatalf("held=%d reasons=%v backend=%q", held, reasons, i.Backend())
	}
	for range 2 { // releasing twice is harmless
		if err := i.Release(); err != nil {
			t.Fatal(err)
		}
	}
	if held != 0 || i.Backend() != "" {
		t.Fatalf("after Release: held=%d backend=%q", held, i.Backend())
	}
	// The reason is read each time it is taken, so it follows the language.
	i.why = func() string { return "Musique en cours" }
	_ = i.Acquire(context.Background())
	if err := i.Close(); err != nil {
		t.Fatal(err)
	}
	if held != 0 || !slices.Equal(reasons, []string{"Playing music", "Musique en cours"}) {
		t.Fatalf("after Close: held=%d reasons=%v", held, reasons)
	}
}

func TestInhibitorWithAnEndedContextHoldsNothing(t *testing.T) {
	i := newMacInhibitor(func() string { return "" })
	i.keepAwake = func(string) func() { t.Error("took the assertion"); return func() {} }
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := i.Acquire(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Acquire = %v", err)
	}
}

// fakeCenter is the system's notification center.
type fakeCenter struct {
	mu      sync.Mutex
	err     error // what Show fails with
	shown   []note
	removed []string
	hooks   map[int]func()
	hookID  int
}

func (c *fakeCenter) Show(_ context.Context, n note) (func(), error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return nil, c.err
	}
	c.shown = append(c.shown, n)
	return func() { c.mu.Lock(); c.removed = append(c.removed, n.ID); c.mu.Unlock() }, nil
}

func (c *fakeCenter) OnWillQuit(fn func()) func() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.hooks == nil {
		c.hooks = map[int]func(){}
	}
	c.hookID++
	id := c.hookID
	c.hooks[id] = fn
	return func() { c.mu.Lock(); delete(c.hooks, id); c.mu.Unlock() }
}

// quit runs the hooks the app runs as it quits.
func (c *fakeCenter) quit() {
	c.mu.Lock()
	var fns []func()
	for _, fn := range c.hooks {
		fns = append(fns, fn)
	}
	c.mu.Unlock()
	for _, fn := range fns {
		fn()
	}
}

func newTestNotifier(t *testing.T, c *fakeCenter, actions *[]string) *macNotifier {
	t.Helper()
	n, err := newNotifier(c, func(key string) { *actions = append(*actions, key) })
	if err != nil {
		t.Fatal(err)
	}
	return n
}

var (
	trackMsg    = notify.Notification{Summary: "Song", Body: "Artist — Album", Actions: []notify.Action{{Key: notify.DefaultAction, Label: "Show Sona"}, {Key: "next", Label: "Next"}}}
	reminderMsg = notify.Notification{Summary: "Album", Body: "Out now", Urgency: notify.UrgencyNormal}
)

// A track notification is the song and the artist, silent; the Next button
// and the artwork MyGo can't show are left out; the next song replaces it.
func TestNotifierShowsTrackSilentlyAndReplacesIt(t *testing.T) {
	c := &fakeCenter{}
	n := newTestNotifier(t, c, new([]string))
	for _, msg := range []notify.Notification{trackMsg, {Summary: "Next song", Body: "Band"}} {
		if err := n.Show(t.Context(), msg); err != nil {
			t.Fatal(err)
		}
	}
	if len(c.shown) != 2 || c.shown[0].ID != c.shown[1].ID || c.shown[0].ID == "" {
		t.Fatalf("shown %+v: want two under one identifier", c.shown)
	}
	first := c.shown[0]
	if first.Title != "Song" || first.Body != "Artist — Album" || !first.Silent {
		t.Errorf("track notification = %+v", first)
	}
}

func TestNotifierSoundsForAReminder(t *testing.T) {
	c := &fakeCenter{}
	n := newTestNotifier(t, c, new([]string))
	if err := n.Show(t.Context(), reminderMsg); err != nil {
		t.Fatal(err)
	}
	if got := c.shown[0]; got.Silent || got.Title != "Album" || got.Body != "Out now" {
		t.Errorf("reminder = %+v", got)
	}
}

// Every reminder keeps its own notification, apart from the track's.
func TestNotifiersShowUnderTheirOwnIdentifiers(t *testing.T) {
	c := &fakeCenter{}
	ids := map[string]bool{}
	for range 3 {
		n := newTestNotifier(t, c, new([]string))
		if err := n.Show(t.Context(), reminderMsg); err != nil {
			t.Fatal(err)
		}
		ids[c.shown[len(c.shown)-1].ID] = true
	}
	if len(ids) != 3 {
		t.Errorf("identifiers = %v, want three distinct", ids)
	}
}

func TestNotifierClickOpensSonaUntilClosed(t *testing.T) {
	c := &fakeCenter{}
	var actions []string
	n := newTestNotifier(t, c, &actions)
	if err := n.Show(t.Context(), trackMsg); err != nil {
		t.Fatal(err)
	}
	c.shown[0].OnClick()
	if !slices.Equal(actions, []string{notify.DefaultAction}) {
		t.Fatalf("a click reported %v, want the default action", actions)
	}
	n.Close()
	c.shown[0].OnClick()
	if len(actions) != 1 {
		t.Fatalf("a click after Close reported %v", actions)
	}
	if err := n.Show(t.Context(), trackMsg); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("Show after Close = %v", err)
	}
}

func TestNotifierDeniedIsTheServicesError(t *testing.T) {
	c := &fakeCenter{err: mygo.ErrNotificationsDenied}
	n := newTestNotifier(t, c, new([]string))
	if err := n.Show(t.Context(), trackMsg); !errors.Is(err, services.ErrNotificationsDenied) {
		t.Fatalf("Show = %v, want services.ErrNotificationsDenied", err)
	}
	other := errors.New("the notification center failed")
	c.err = other
	if err := n.Show(t.Context(), trackMsg); !errors.Is(err, other) || errors.Is(err, services.ErrNotificationsDenied) {
		t.Fatalf("another failure = %v", err)
	}
}

func TestNotifierDismissRemovesTheShownNotification(t *testing.T) {
	c := &fakeCenter{}
	n := newTestNotifier(t, c, new([]string))
	if err := n.Dismiss(t.Context()); err != nil { // nothing shown yet
		t.Fatal(err)
	}
	if err := n.Show(t.Context(), trackMsg); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := n.Dismiss(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if !slices.Equal(c.removed, []string{c.shown[0].ID}) {
		t.Errorf("removed %v, want the shown one once", c.removed)
	}
}

// MyGo can't remove a notification once its loop has stopped, so the track
// notification goes as the app quits; a reminder stays, as on Linux.
func TestTrackNotificationGoesWhenSonaQuitsAndReminderStays(t *testing.T) {
	c := &fakeCenter{}
	track := newTestNotifier(t, c, new([]string))
	reminder := newTestNotifier(t, c, new([]string))
	if err := track.Show(t.Context(), trackMsg); err != nil {
		t.Fatal(err)
	}
	if err := track.Show(t.Context(), trackMsg); err != nil { // one hook, however many songs
		t.Fatal(err)
	}
	if err := reminder.Show(t.Context(), reminderMsg); err != nil {
		t.Fatal(err)
	}
	if len(c.hooks) != 1 {
		t.Fatalf("%d quit hooks, want one for the track notifier", len(c.hooks))
	}
	c.quit()
	if want := []string{c.shown[0].ID}; !slices.Equal(c.removed, want) {
		t.Fatalf("quit removed %v, want only the track's %v", c.removed, want)
	}
	track.Close()
	if len(c.hooks) != 0 {
		t.Errorf("%d quit hooks after Close", len(c.hooks))
	}
}
