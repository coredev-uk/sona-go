package reminders

import (
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

func announced(ann []string, prefix string) bool {
	return slices.ContainsFunc(ann, func(s string) bool { return strings.HasPrefix(s, prefix) })
}

// The album menu's action says its state in its name (the page's button
// carries it as a pressed state, which a menu item has no equivalent of),
// stays listed but unavailable until the list has loaded, and what setting
// or removing a reminder did is announced by its toast.
func TestA11yReminderActionNamesItsStateAndOutcomesAreAnnounced(t *testing.T) {
	g := newRig(t)
	album := upcomingAlbum()
	if got := g.def.Label(g.ctx(album, []services.Reminder{})); got != "Remind me" {
		t.Errorf("label %q", got)
	}
	if got := g.def.Label(g.ctx(album, []services.Reminder{{AlbumID: album.CatalogID}})); got != "Turn off reminder" {
		t.Errorf("label when set %q", got)
	}
	if g.def.Enabled(g.ctx(album, nil)) {
		t.Error("the action is enabled before the list loaded")
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
