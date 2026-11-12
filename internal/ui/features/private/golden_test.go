package private

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// goldenPill draws the pill on, in a player-bar-sized strip wide enough for
// its word.
func goldenPill(t *testing.T, dark bool) *uitest.Session {
	t.Helper()
	if err := theme.RegisterFonts(); err != nil {
		t.Fatal(err)
	}
	h := uitest.New(t)
	m := New(h.Env)
	m.Init()
	tt := h.Tester(t, func(c *ui.Context) {
		ui.Row(c).Fill().Padding(8).Gap(8).AlignItems(ui.Center).Justify(ui.End).Background(theme.C("ground")).Children(func() {
			h.Env.PlayerControls.Build(c)
		})
	}, 1200, 48)
	if dark {
		theme.SetMode(theme.ModeDark)
		t.Cleanup(func() { theme.SetMode(theme.ModeSystem) })
	}
	tt.Until("the private state", func() bool {
		_, ok := store.Peek[services.PrivateState](h.Env.Queries, store.KeyPrivate)
		return ok
	})
	h.Bus.Emit(services.EventPrivateChanged, services.PrivateState{On: true, Source: "setting", Saved: true})
	tt.Until("the word", tt.Shows("Private"))
	tt.Settle()
	return tt
}

func TestGoldenPillLight(t *testing.T) {
	uitest.Golden(t, goldenPill(t, false).Image(), "pill-on-light")
}

func TestGoldenPillDark(t *testing.T) {
	uitest.Golden(t, goldenPill(t, true).Image(), "pill-on-dark")
}
