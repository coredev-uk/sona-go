package app

import (
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// Over the window's material (macOS's, Windows' Mica) the sidebar lets it
// through under its tint while the page stays solid; Reduce effects and the
// desktop's Reduce Transparency make the sidebar solid again.
func TestTheSidebarShowsTheMaterialOnlyWhereEffectsAreOn(t *testing.T) {
	for _, m := range []mygo.Vibrancy{mygo.VibrancySidebar, mygo.VibrancyMica} {
		t.Run(string(m), func(t *testing.T) {
			t.Cleanup(func() { theme.SetReduceEffects(false) })
			a, tt := newShell(t, Options{}, pageModule("a", pageDef{"/albums", "Albums", module.NavLibrary, 1}))
			a.material, a.materialSet = m, m
			tt.SetVibrancy(true)
			// The sidebar's foot, under no text, and the middle of the page.
			sidebar := func() uint8 { tt.Frame(); return tt.Image().RGBAAt(4, 700).A }
			page := func() uint8 { return tt.Image().RGBAAt(700, 400).A }

			if got := sidebar(); got == 255 {
				t.Error("over the material the sidebar is opaque, hiding it")
			}
			if got := page(); got != 255 {
				t.Errorf("over the material the page is see-through (alpha %d)", got)
			}

			tt.SetPreferences(ui.Preferences{ReduceTransparency: true})
			if got := sidebar(); got != 255 {
				t.Errorf("with Reduce Transparency the sidebar is see-through (alpha %d)", got)
			}
			tt.SetPreferences(ui.Preferences{})
			if sidebar() == 255 {
				t.Error("after Reduce Transparency the material does not come back")
			}

			theme.SetReduceEffects(true)
			if got := sidebar(); got != 255 {
				t.Errorf("with Reduce effects the sidebar is see-through (alpha %d)", got)
			}
		})
	}
}
