package reminders

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

var now = time.Date(2026, time.November, 4, 12, 0, 0, 0, time.UTC)

func upcomingAlbum() services.Album {
	return services.Album{
		CatalogID: "1700", Name: "Out Soon", Artist: "An Artist", Prerelease: true,
		ReleaseAt: now.Add(10 * 24 * time.Hour).UnixMilli(),
	}
}

type rig struct {
	r    *reminders
	def  kit.ActionDef
	sets []bool
}

func newRig(t *testing.T) *rig {
	t.Helper()
	env := uitest.Env(t, uitest.Bare())
	g := &rig{}
	g.r = &reminders{env: env, now: func() time.Time { return now }}
	g.r.available = func(services.Album) bool { return true }
	g.r.set = func(_ services.Album, on bool) { g.sets = append(g.sets, on) }
	g.def = g.r.toggleAction()
	return g
}

func (g *rig) ctx(a services.Album, list []services.Reminder) *kit.ActionContext {
	ac := &kit.ActionContext{Items: []kit.Item{kit.AlbumItem(a)}}
	if list == nil {
		return ac.WithData(map[string]any{})
	}
	return ac.WithData(map[string]any{g.r.loaded().ID: list})
}

func TestTheAlbumActionIsListedOnlyForUpcomingAlbumsWithADateAnIDAndNotifications(t *testing.T) {
	g := newRig(t)
	if g.def.ID != "reminders.toggle" || g.def.Target != kit.KindAlbum || g.def.Group != kit.GroupItem || g.def.Order != 25 {
		t.Fatalf("action = %s %s group %d order %d", g.def.ID, g.def.Target, g.def.Group, g.def.Order)
	}
	ok := func(a services.Album) bool { return g.def.Available(g.ctx(a, nil)) }
	if !ok(upcomingAlbum()) {
		t.Error("an upcoming album with a date is not listed")
	}
	out := upcomingAlbum()
	out.Prerelease = false
	if ok(out) {
		t.Error("an album that is out is listed")
	}
	past := upcomingAlbum()
	past.ReleaseAt = now.Add(-time.Hour).UnixMilli() // released while the page was open
	if ok(past) {
		t.Error("an album whose release day has passed is listed")
	}
	// A pre-release with no full date counts as upcoming (Go has no day to
	// compare), and kit.RemindAvailable then refuses it: a reminder needs the date.
	undated := upcomingAlbum()
	undated.ReleaseAt = 0
	if !upcoming(undated, now) {
		t.Error("a pre-release with no date is not upcoming")
	}
	g.r.available = func(services.Album) bool { return false }
	if ok(upcomingAlbum()) {
		t.Error("listed where the platform has no notifications")
	}
}

func TestTheActionNamesItselfAndWaitsForTheList(t *testing.T) {
	g := newRig(t)
	a := upcomingAlbum()
	if g.def.Enabled(g.ctx(a, nil)) {
		t.Error("enabled before the reminders loaded")
	}
	empty := g.ctx(a, []services.Reminder{})
	if !g.def.Enabled(empty) || g.def.Label(empty) != "Remind me" {
		t.Errorf("no reminder: enabled %v, label %q", g.def.Enabled(empty), g.def.Label(empty))
	}
	g.def.Run(empty)
	set := g.ctx(a, []services.Reminder{{AlbumID: "1700"}})
	if g.def.Label(set) != "Turn off reminder" {
		t.Errorf("label with a reminder set = %q", g.def.Label(set))
	}
	g.def.Run(set)
	if len(g.sets) != 2 || !g.sets[0] || g.sets[1] {
		t.Errorf("runs asked for %v, want [true false]", g.sets)
	}
	other := g.ctx(a, []services.Reminder{{AlbumID: "other"}})
	if g.def.Label(other) != "Remind me" {
		t.Errorf("another album's reminder changes the label: %q", g.def.Label(other))
	}
}

func TestTheMenuLoadsTheListAndFallsBackToNone(t *testing.T) {
	g := newRig(t)
	load := g.r.loaded().Load
	g.r.list = func(context.Context) ([]services.Reminder, error) { return []services.Reminder{{AlbumID: "1"}}, nil }
	if got := load(context.Background(), nil).([]services.Reminder); len(got) != 1 {
		t.Errorf("loaded %v", got)
	}
	g.r.list = func(context.Context) ([]services.Reminder, error) { return nil, errors.New("db") }
	got, ok := load(context.Background(), nil).([]services.Reminder)
	if !ok || got == nil || len(got) != 0 {
		t.Errorf("a failed load gave %#v, want an empty list", got)
	}
}

func TestRemindersChangedUpdatesTheMirror(t *testing.T) {
	h := uitest.New(t, uitest.Bare())
	New(h.Env).Init()
	list := []services.Reminder{{AlbumID: "1700", Name: "Out Soon"}}
	h.Bus.Emit(services.EventRemindersChanged, list)
	h.Main.Drain()
	got, ok := store.Peek[[]services.Reminder](h.Env.Queries, store.KeyReminders)
	if !ok || len(got) != 1 || got[0].AlbumID != "1700" {
		t.Errorf("mirror = %v, %v", got, ok)
	}
	if _, found := store.ReminderFor(got, "1700"); !found {
		t.Error("a reminder is not found by its catalogue album ID")
	}
	if _, found := store.ReminderFor(got, "1"); found {
		t.Error("found a reminder of another album")
	}
}

// The service calls behind the button and the action: an album the platform
// cannot remind about says so; removing a reminder confirms.
func TestSetAndRemoveToastTheOutcome(t *testing.T) {
	h := uitest.New(t) // a real RemindersService; the test platform has no notifications
	tt := h.Tester(t, func(c *ui.Context) { kit.Toaster(c) }, 600, 300)
	album := upcomingAlbum()
	kit.SetReminder(h.Env.Core, album, true)
	tt.Until("the failure toast", tt.Shows("Couldn't set the reminder"))
	kit.SetReminder(h.Env.Core, album, false)
	tt.Until("the removal toast", tt.Shows("You won't be reminded about “Out Soon”"))
}
