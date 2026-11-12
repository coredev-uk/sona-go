package updates

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// goldenSlip draws the slip on the sidebar's surface in its main state: a
// self-updating install with a release to install.
func goldenSlip(t *testing.T, dark bool) *uitest.Session {
	t.Helper()
	if err := theme.RegisterFonts(); err != nil {
		t.Fatal(err)
	}
	h := uitest.New(t)
	m := newUpdates(h.Env, nil, nil).module()
	m.Init()
	tt := h.Tester(t, func(c *ui.Context) {
		ui.Column(c).Fill().Justify(ui.End).Background(theme.C("surface")).Children(func() { m.Slots.SidebarFooter(c) })
	}, 232, 120)
	if dark {
		theme.SetMode(theme.ModeDark)
		t.Cleanup(func() { theme.SetMode(theme.ModeSystem) })
	}
	h.Bus.Emit(services.EventUpdateAvailable, services.UpdateStatus{Current: "0.2.0", Checkable: true, Latest: "0.3.0", Available: true, SelfUpdate: true})
	tt.Until("the slip", tt.Shows("Install and restart"))
	tt.Settle()
	return tt
}

func TestGoldenSlipLight(t *testing.T) {
	uitest.Golden(t, goldenSlip(t, false).Image(), "slip-light")
}

func TestGoldenSlipDark(t *testing.T) {
	uitest.Golden(t, goldenSlip(t, true).Image(), "slip-dark")
}
