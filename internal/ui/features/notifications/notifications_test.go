package notifications

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// fakeAPI is NotificationsService over a list the test sets; each change
// is sent as Go sends it, "notifications:changed".
type fakeAPI struct {
	mu   sync.Mutex
	list services.Notifications
	bus  func(services.Notifications)
	// calls are the methods called, in order ("read", "dismiss:2", "clear").
	calls []string
}

func (f *fakeAPI) List(context.Context) (services.Notifications, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.list, nil
}

// change applies fn to a copy of the list, as Go sends a new list each time,
// and sends it.
func (f *fakeAPI) change(fn func(*services.Notifications), call string) {
	f.mu.Lock()
	f.list.Items = slices.Clone(f.list.Items)
	fn(&f.list)
	f.list.Unread = 0
	for _, n := range f.list.Items {
		if !n.Read {
			f.list.Unread++
		}
	}
	f.calls = append(f.calls, call)
	list := f.list
	f.mu.Unlock()
	f.bus(list)
}

func (f *fakeAPI) MarkRead(context.Context) error {
	f.change(func(l *services.Notifications) {
		for i := range l.Items {
			l.Items[i].Read = true
		}
	}, "read")
	return nil
}

func (f *fakeAPI) Dismiss(_ context.Context, id int64) error {
	f.change(func(l *services.Notifications) {
		l.Items = slices.DeleteFunc(l.Items, func(n services.Notification) bool { return n.ID == id })
	}, "dismiss")
	return nil
}

func (f *fakeAPI) Clear(context.Context) error {
	f.change(func(l *services.Notifications) { l.Items = nil }, "clear")
	return nil
}

func (f *fakeAPI) called() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

type rig struct {
	h    *uitest.Harness
	tt   *uitest.Session
	api  *fakeAPI
	acts []module.Activity
	// live is what the test Activity reports.
	live module.ActivityStatus
}

var now = time.Date(2026, 11, 21, 15, 0, 0, 0, time.UTC)

func entries() services.Notifications {
	return services.Notifications{Unread: 2, Items: []services.Notification{
		{ID: 2, Kind: services.NoticeReleaseOut, Subject: "In Rainbows", Artist: "Radiohead", Target: services.OpenTarget{Kind: "album", ID: "1109714933"}, CreatedAt: now.Add(-2 * time.Hour).UnixMilli()},
		{ID: 1, Kind: services.NoticeAddonOff, Subject: "Webhook", Target: services.OpenTarget{Kind: services.OpenSettings, ID: "addons"}, CreatedAt: now.Add(-26 * time.Hour).UnixMilli()},
	}}
}

func newRig(t *testing.T, list services.Notifications) *rig {
	t.Helper()
	h := uitest.New(t)
	r := &rig{h: h, api: &fakeAPI{list: list}}
	r.api.bus = func(l services.Notifications) { h.Bus.Emit(services.EventNotificationsChanged, l) }
	f := newFeature(h.Env, r.api)
	f.now = func() time.Time { return now }
	m := f.module()
	m.Init()
	r.acts = []module.Activity{{
		Status: func(*ui.Context) module.ActivityStatus { return r.live },
		Build: func(c *ui.Context) {
			p := r.live.Progress
			kit.ActivityRow(c, "test:live", kit.ActivitySpec{Icon: "refresh-cw", Title: "Syncing songs", Count: "812 of 2,040", Progress: &p})
		},
		Foot: func(c *ui.Context) { ui.Text(c, "Synced 2 minutes ago") },
	}}
	r.tt = h.Tester(t, func(c *ui.Context) {
		kit.Toaster(c)
		ui.Row(c).FillWidth().Height(48).Justify(ui.End).PaddingEnd(16).AlignItems(ui.Center).Children(func() {
			m.Slots.TopBarNotices(c, r.acts)
		})
	}, 480, 640)
	r.tt.Until("the entries", func() bool {
		_, ok := store.Peek[services.Notifications](h.Env.Queries, keyNotices)
		return ok
	})
	return r
}

func (r *rig) bellName(t *testing.T) string {
	t.Helper()
	tr := access.Read(t, r.tt.Tester)
	for _, n := range tr.Nodes {
		if n.Role == ui.RoleButton && n.HasPopup == ui.PopupDialog {
			return n.Label
		}
	}
	t.Fatalf("no bell:\n%s", tr.Dump())
	return ""
}

// The bell says how many are new; opening it lists the entries in words,
// newest first, marks them read but keeps their mark while open, and
// choosing one opens what it is about and closes the panel.
func TestOpeningTheBellListsTheEntriesAndMarksThemRead(t *testing.T) {
	r := newRig(t, entries())
	if got := r.bellName(t); got != "Notifications, 2 new" {
		t.Fatalf("bell = %q", got)
	}
	r.tt.MustClick("Notifications, 2 new")
	r.tt.Until("the panel", r.tt.Shows("In Rainbows is out"))
	for _, w := range []string{"Notifications", "Clear all", "Radiohead", "Webhook was switched off", "2 hr. ago", "yesterday"} {
		if !r.tt.HasText(w) {
			t.Errorf("panel lacks %q: %q", w, r.tt.Texts())
		}
	}
	r.tt.Until("read", func() bool { return slices.Contains(r.api.called(), "read") })
	r.tt.Drain()
	if got := r.bellName(t); got != "Notifications" {
		t.Errorf("bell after opening = %q", got)
	}
	var opened []services.OpenTarget
	store.On(r.h.Bus, services.EventAppOpen, func(o services.OpenTarget) { opened = append(opened, o) })
	r.tt.MustClick("In Rainbows is out")
	r.tt.Drain()
	if len(opened) != 1 || opened[0] != (services.OpenTarget{Kind: "album", ID: "1109714933"}) {
		t.Errorf("opened %+v", opened)
	}
	if r.tt.HasText("Webhook was switched off") {
		t.Error("the panel stayed open")
	}
}

// Remove drops one entry; Clear all drops them all and the panel says there
// is nothing new.
func TestEntriesCanBeRemovedAndCleared(t *testing.T) {
	r := newRig(t, entries())
	r.tt.MustClick("Notifications, 2 new")
	r.tt.Until("the panel", r.tt.Shows("Webhook was switched off"))
	tr := access.Read(t, r.tt.Tester)
	if _, ok := tr.Find(ui.RoleButton, "Remove “Webhook was switched off”"); !ok {
		t.Fatalf("no Remove named by its entry:\n%s", tr.Dump())
	}
	r.tt.MustClick("Remove “Webhook was switched off”")
	r.tt.Until("the entry gone", func() bool { return !r.tt.HasText("Webhook was switched off") })
	if !r.tt.HasText("In Rainbows is out") || !slices.Contains(r.api.called(), "dismiss") {
		t.Errorf("after Remove: %q, calls %q", r.tt.Texts(), r.api.called())
	}
	r.tt.MustClick("Clear all")
	r.tt.Until("nothing new", r.tt.Shows("Nothing new"))
	if !slices.Contains(r.api.called(), "clear") {
		t.Errorf("calls = %q", r.api.called())
	}
}

// Live work shows on the bell's name and as a row at the top of the panel;
// an Activity's alert or the unread entries put the dot on the bell, and
// the foot line is drawn under the entries.
func TestLiveRowsAndTheBellsMarks(t *testing.T) {
	r := newRig(t, services.Notifications{})
	if got := r.bellName(t); got != "Notifications" {
		t.Fatalf("bell = %q", got)
	}
	r.live = module.ActivityStatus{Shown: true, Busy: true, Progress: 0.4, Label: "Syncing songs, 812 of 2,040"}
	r.tt.Drain()
	if got := r.bellName(t); got != "Notifications; Syncing songs, 812 of 2,040" {
		t.Errorf("bell while syncing = %q", got)
	}
	if b := readBell(services.Notifications{}, []module.ActivityStatus{{Alert: true}}); !b.alert {
		t.Error("an alert doesn't mark the bell")
	}
	if b := readBell(services.Notifications{Unread: 1}, nil); !b.news || b.alert {
		t.Errorf("an unread entry = %+v, want news", b)
	}
	r.tt.MustClick("Notifications; Syncing songs, 812 of 2,040")
	r.tt.Until("the live row", r.tt.Shows("Syncing songs"))
	for _, w := range []string{"812 of 2,040", "Synced 2 minutes ago"} {
		if !r.tt.HasText(w) {
			t.Errorf("panel lacks %q: %q", w, r.tt.Texts())
		}
	}
	if r.tt.HasText("Nothing new") || r.tt.HasText("Clear all") {
		t.Errorf("an empty list beside live work: %q", r.tt.Texts())
	}
}

// The bell toggles the panel: a second press on it closes it, and so does
// Escape, which gives the focus back to the bell.
func TestTheBellTogglesThePanel(t *testing.T) {
	r := newRig(t, entries())
	r.tt.MustClick("Notifications, 2 new")
	r.tt.Until("the panel", r.tt.Shows("In Rainbows is out"))
	r.tt.Until("read", func() bool { return slices.Contains(r.api.called(), "read") })
	r.tt.Drain()
	r.tt.MustClick("Notifications")
	r.tt.Drain()
	if r.tt.HasText("In Rainbows is out") {
		t.Fatal("a second press on the bell left the panel open")
	}
	r.tt.MustClick("Notifications")
	r.tt.Until("the panel again", r.tt.Shows("In Rainbows is out"))
	r.tt.Key(0, ui.KeyEscape)
	r.tt.Drain()
	if r.tt.HasText("In Rainbows is out") {
		t.Fatal("Escape left the panel open")
	}
}
