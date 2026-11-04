package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

func init() {
	register(section{name: "Colours", build: colours})
}

// hex is a colour as #rrggbb, with its alpha when it has one.
func hex(c ui.Color) string {
	if c.A == 255 {
		return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
	}
	return fmt.Sprintf("#%02x%02x%02x/%d%%", c.R, c.G, c.B, int(float64(c.A)/255*100+0.5))
}

// colours is every colour token of the current mode as a swatch, over the
// ground and over the surface (translucent tokens depend on what is below).
func colours(c *ui.Context, g *gallery) {
	tk := theme.Current()
	mode := "light"
	if g.dark {
		mode = "dark"
	}
	note(c, fmt.Sprintf("%d colour tokens, %s mode. Edit internal/ui/theme/tokens.json and they change here on the next frame.", len(tk.Color), mode))
	names := make([]string, 0, len(tk.Color))
	for n := range tk.Color {
		names = append(names, n)
	}
	slices.SortFunc(names, strings.Compare)
	ui.Row(c).Gap(12).Wrap().Margin(16, 0, 0).Children(func() {
		for _, n := range names {
			swatch(c, n, tk.Color[n])
		}
	})
}

// swatch shows a colour as a chip on the ground and one on the surface.
func swatch(c *ui.Context, name string, col ui.Color) {
	ui.Column(c).Width(168).Gap(6).Children(func() {
		ui.Row(c).Height(56).Radius(theme.Current().Radius["md"]).Border(1, theme.C("line")).ClipX().ClipY().Children(func() {
			ui.Box(c).Grow(1).FillHeight().Background(theme.C("ground")).Children(func() { ui.Box(c).Fill().Background(col) })
			ui.Box(c).Grow(1).FillHeight().Background(theme.C("surface")).Children(func() { ui.Box(c).Fill().Background(col) })
		})
		text(c, name, "label").TextColor(theme.C("ink")).SingleLine()
		text(c, hex(col), "meta").TextColor(theme.C("ink-muted")).SingleLine().Selectable()
	})
}
