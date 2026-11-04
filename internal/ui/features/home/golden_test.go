package home

import (
	"testing"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// goldenHome draws Home over the invented page at the reference window size,
// the covers loaded.
func goldenHome(t *testing.T, w, h int, dark bool) *rig {
	t.Helper()
	if err := theme.RegisterFonts(); err != nil {
		t.Fatal(err)
	}
	r := newRig(t, "/", w, h, withHome(samplePage()), withUI(uitest.Empty()))
	if dark {
		theme.SetMode(theme.ModeDark)
		t.Cleanup(func() { theme.SetMode(theme.ModeSystem) })
	}
	r.until("the page", "Top Picks for You", "Because You Like Rock")
	r.tt.Until("the covers", func() bool { return r.h.Covers.Hits() > 0 })
	r.tt.Settle()
	return r
}

func TestGoldenHomeLight(t *testing.T) {
	r := goldenHome(t, 1132, 1253, false)
	uitest.Golden(t, r.tt.Image(), "home-light")
}

func TestGoldenHomeDark(t *testing.T) {
	r := goldenHome(t, 1132, 1253, true)
	uitest.Golden(t, r.tt.Image(), "home-dark")
}

func TestGoldenHomeNarrow(t *testing.T) {
	r := goldenHome(t, 940, 600, false)
	uitest.Golden(t, r.tt.Image(), "home-light-940")
}
