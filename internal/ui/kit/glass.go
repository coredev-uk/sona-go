package kit

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// popoverSurface is the surface of a menu, popover or option list: the popover
// colour with a border and the float shadow, or, in the macOS glass spike
// (glass_darwin.go), a pane of Liquid Glass of the same radius, with no
// shadow (its rim is its edge).
func popoverSurface(panel ui.Element, radius float32) ui.Element {
	if GlassOn("menus") {
		return GlassPane(panel, "menus", radius)
	}
	sh := theme.Current().Shadow["float"]
	return panel.Background(theme.C("popover")).Border(1, theme.C("border")).Shadow(sh.X, sh.Y, sh.Blur, sh.Spread, sh.Color)
}
