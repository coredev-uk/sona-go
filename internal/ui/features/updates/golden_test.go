package updates

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// goldenRow draws the notification centre's update row on the panel's
// raised surface in its main state: a self-updating install with a release
// to install.
func goldenRow(t *testing.T, dark bool) *uitest.Session {
	t.Helper()
	if err := theme.RegisterFonts(); err != nil {
		t.Fatal(err)
	}
	h := uitest.New(t)
	m := newUpdates(h.Env, nil, nil).module()
	m.Init()
	tt := h.Tester(t, func(c *ui.Context) {
		ui.Column(c).Fill().Padding(8).Background(theme.C("raised")).Children(func() {
			if a := m.Slots.Activity; a.Status(c).Shown {
				a.Build(c)
			}
		})
	}, 360, 120)
	if dark {
		theme.SetMode(theme.ModeDark)
		t.Cleanup(func() { theme.SetMode(theme.ModeSystem) })
	}
	h.Bus.Emit(services.EventUpdateAvailable, services.UpdateStatus{Current: "0.2.0", Checkable: true, Latest: "0.3.0", Available: true, SelfUpdate: true})
	tt.Until("the row", tt.Shows("Install and restart"))
	tt.Settle()
	return tt
}

func TestGoldenRowLight(t *testing.T) {
	uitest.Golden(t, goldenRow(t, false).Image(), "row-light")
}

func TestGoldenRowDark(t *testing.T) {
	uitest.Golden(t, goldenRow(t, true).Image(), "row-dark")
}
