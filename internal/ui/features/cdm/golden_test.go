package cdm

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// goldenNotice draws the notice while Widevine downloads, 42% in, on the
// player bar's ground.
func goldenNotice(t *testing.T, dark bool) *uitest.Session {
	t.Helper()
	if err := theme.RegisterFonts(); err != nil {
		t.Fatal(err)
	}
	h := uitest.New(t, uitest.Bare())
	h.Env.HasRoute = func(p string) bool { return p == "/settings" }
	New(h.Env).Init()
	tt := h.Tester(t, func(c *ui.Context) {
		ui.Row(c).Fill().Padding(12).AlignItems(ui.Center).Background(theme.C("ground")).Children(func() { h.Env.PlayerNotice.Build(c) })
	}, 360, 48)
	if dark {
		theme.SetMode(theme.ModeDark)
		t.Cleanup(func() { theme.SetMode(theme.ModeSystem) })
	}
	h.Bus.Emit(services.EventCDMStatus, services.CDMStatus{State: "downloading", Received: 42, Total: 100})
	tt.Until("the percent", tt.Shows("42%"))
	tt.Settle()
	return tt
}

func TestGoldenNoticeLight(t *testing.T) {
	uitest.Golden(t, goldenNotice(t, false).Image(), "notice-downloading-light")
}

func TestGoldenNoticeDark(t *testing.T) {
	uitest.Golden(t, goldenNotice(t, true).Image(), "notice-downloading-dark")
}
