package reminders

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

func announced(ann []string, prefix string) bool {
	return slices.ContainsFunc(ann, func(s string) bool { return strings.HasPrefix(s, prefix) })
}

// The album menu lists the action named for its state, disabled until the
// list has loaded, and what setting or removing a reminder did is announced
// by its toast.
func TestA11yReminderActionNamesItsStateAndOutcomesAreAnnounced(t *testing.T) {
	album := upcomingAlbum()
	open := func(t *testing.T, want string, list func(context.Context) ([]services.Reminder, error)) access.Tree {
		t.Helper()
		g := newRig(t)
		g.r.list = list
		h := uitest.New(t, uitest.Bare())
		reg := kit.NewActionRegistry(g.def)
		reg.SetPoster(h.Env.Post)
		menus := kit.NewActionMenus(reg, nil)
		tt := h.Tester(t, func(c *ui.Context) {
			ui.Column(c).Fill().Padding(20).AlignItems(ui.Start).Children(func() {
				menus.Trigger(c, kit.ActionTarget{Item: kit.AlbumItem(album), Name: album.Name, Key: "a"}, kit.TriggerOpts{Visible: true})
			})
			menus.Popup(c)
		}, 400, 400)
		if err := tt.Click("More for Out Soon"); err != nil {
			t.Fatal(err)
		}
		tt.Drain()
		// The list loads off the main thread: the entry is named for its state
		// and enabled once it has.
		tt.Until("the list to load", tt.Shows(want))
		return access.Read(t, tt.Tester)
	}
	tr := open(t, "Remind me on Nov 14", func(context.Context) ([]services.Reminder, error) { return nil, nil })
	if n, ok := tr.Find(ui.RoleMenuItem, "Remind me on Nov 14"); !ok || n.Has(access.Disabled) {
		t.Errorf("menu with the list loaded:\n%s", tr.Dump())
	}
	tr = open(t, "Turn off reminder for Nov 14", func(context.Context) ([]services.Reminder, error) {
		return []services.Reminder{{AlbumID: album.CatalogID}}, nil
	})
	if _, ok := tr.Find(ui.RoleMenuItem, "Turn off reminder for Nov 14"); !ok {
		t.Errorf("menu with a reminder set:\n%s", tr.Dump())
	}

	h := uitest.New(t)
	tt := h.Tester(t, func(c *ui.Context) { kit.Toaster(c) }, 600, 300)
	kit.SetReminder(h.Env.Core, album, true)
	tt.Until("the failure toast", tt.Shows("Couldn't set the reminder"))
	if ann := tt.Announcements(); !announced(ann, "Couldn't set the reminder") {
		t.Errorf("announcements %q", ann)
	}
	kit.SetReminder(h.Env.Core, album, false)
	tt.Until("the removal toast", tt.Shows("You won't be reminded about “Out Soon”"))
	if ann := tt.Announcements(); !announced(ann, "You won't be reminded about “Out Soon”") {
		t.Errorf("announcements %q", ann)
	}
}
