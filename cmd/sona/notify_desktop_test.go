//go:build darwin || windows

package main

import (
	"context"
	"errors"
	"os"
	"slices"
	"sync"
	"testing"

	"github.com/egoist/mygo"

	"github.com/coredev-uk/sona-go/internal/desktop/notify"
	"github.com/coredev-uk/sona-go/internal/services"
)

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

func newTestNotifier(t *testing.T, c *fakeCenter, actions *[]string) *desktopNotifier {
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
