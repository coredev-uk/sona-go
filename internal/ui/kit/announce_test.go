package kit

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
)

// AnnounceMounted says a status as it first appears, which MyGo does not
// read, and leaves what changes it after that to MyGo: with a screen reader
// on, each text is read once.
func TestAnnounceMountedSaysTheFirstTextAndTheStatusSaysTheRest(t *testing.T) {
	msg, shown := "No match", false
	tt := controlsTester(t, 300, 100, func(c *ui.Context) {
		ui.Text(c.Key("before"), "Before")
		if !shown {
			return
		}
		note := ui.Text(c.Key("note"), msg).Role(ui.RoleStatus)
		AnnounceMounted(c, note, msg)
	})
	tt.UseScreenReader()
	tt.Announcements()

	shown = true
	tt.Frame()
	if got := tt.Announcements(); !slices.Equal(got, []string{"No match"}) {
		t.Errorf("a status appearing read %q, want it once", got)
	}
	tt.Frame()
	if got := tt.Announcements(); len(got) != 0 {
		t.Errorf("read again: %q", got)
	}

	msg = "No match for “z”"
	tt.Frame()
	if got := tt.Announcements(); !slices.Equal(got, []string{"No match for “z”"}) {
		t.Errorf("a status changing read %q, want it once", got)
	}

	// Gone and back, it is news again; an empty text says nothing.
	shown = false
	tt.Frame()
	tt.Announcements()
	msg, shown = "", true
	tt.Frame()
	if got := tt.Announcements(); len(got) != 0 {
		t.Errorf("an empty status read %q", got)
	}
}
