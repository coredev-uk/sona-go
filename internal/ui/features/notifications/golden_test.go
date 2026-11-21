package notifications

import (
	"testing"

	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// goldenPanel opens the centre with a sync under way, an unread release, an
// add-on switched off and the foot line.
func goldenPanel(t *testing.T, dark bool) *uitest.Session {
	t.Helper()
	if err := theme.RegisterFonts(); err != nil {
		t.Fatal(err)
	}
	r := newRig(t, entries())
	if dark {
		theme.SetMode(theme.ModeDark)
		t.Cleanup(func() { theme.SetMode(theme.ModeSystem) })
	}
	r.live = module.ActivityStatus{Shown: true, Busy: true, Progress: 0.4, Label: "Syncing songs, 812 of 2,040"}
	r.tt.Drain()
	r.tt.MustClick("Notifications, 2 new; Syncing songs, 812 of 2,040")
	r.tt.Until("the panel", r.tt.Shows("In Rainbows is out", "Webhook was switched off"))
	r.tt.Settle()
	return r.tt
}

func TestGoldenPanelLight(t *testing.T) {
	uitest.Golden(t, goldenPanel(t, false).Image(), "panel-light")
}

func TestGoldenPanelDark(t *testing.T) {
	uitest.Golden(t, goldenPanel(t, true).Image(), "panel-dark")
}
