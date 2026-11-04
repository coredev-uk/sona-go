package main

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

func init() {
	register(section{name: "Type scale", build: typeScale})
}

// typeScale is every row of the type scale, largest first, set in its own
// style.
func typeScale(c *ui.Context, _ *gallery) {
	tk := theme.Current()
	note(c, "The interface font is Schibsted Grotesk. Rows are the type tokens; the CSS aliases (page, ui, sub, caption) show with the row they name.")
	names := make([]string, 0, len(tk.Type))
	for n := range tk.Type {
		names = append(names, n)
	}
	slices.SortFunc(names, func(a, b string) int {
		if d := cmp.Compare(tk.Type[b].Size, tk.Type[a].Size); d != 0 {
			return d
		}
		return cmp.Compare(a, b)
	})
	ui.Column(c).Gap(4).Margin(16, 0, 0).Children(func() {
		for _, n := range names {
			t := tk.Type[n]
			ui.Row(c).Gap(24).AlignItems(ui.Center).Padding(10, 0).Children(func() {
				ui.Column(c).Width(200).Gap(2).Children(func() {
					text(c, n, "label").TextColor(theme.C("ink"))
					text(c, fmt.Sprintf("%g / %g · weight %g · tracking %g", t.Size, t.LineHeight, t.Weight, t.Tracking), "meta").TextColor(theme.C("ink-faint"))
				})
				text(c, "Sphinx of black quartz, judge my vow 0123456789", n).TextColor(theme.C("ink")).SingleLine().Grow(1).MinWidth(0)
			})
			ui.Box(c).Height(1).FillWidth().Background(theme.C("line"))
		}
	})
}
