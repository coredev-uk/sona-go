package discover

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// pageView is a page's scroll container: the frame of the last frame, which
// the next frame's gutter and virtualisation follow, and the scroll offsets
// the page's anchors are placed by.
type pageView struct {
	kit.PageFrame
	// laidY is the scroll offset the last frame's layout was built at, and
	// builtY the offset this frame is built at: the places the last layout
	// gave (Element.Bounds) are in content terms by laidY.
	laidY, builtY float32
}

// body builds a page: one scroll container holding the padded column of the
// page's content. key says what the page shows (the route and what the URL
// chooses), so another list starts from its top. build gets the container as
// a kit.Pane (for virtual lists) and the gutter.
func (pg *pageView) body(c *ui.Context, key string, build func(pane *kit.Pane, gutter float32)) {
	pg.laidY = pg.builtY
	pg.Scroll(c, key, build)
	pg.builtY = pg.Pos.Y
}

// shelvesSkeleton is flat placeholder bars at the real sizes of a page of
// shelves: three shelf bars of ten tiles (a row of as many as fit), and, with
// hero, the banner above (v0.2.0's ShelvesSkeleton).
func (d *discover) shelvesSkeleton(c *ui.Context, m *kit.WidthTracker, hero bool) {
	tk := theme.Current()
	root := ui.Column(c.Key("skeleton")).FillWidth().Gap(40).Role(ui.RoleStatus).Label(i18n.T("Loading"))
	width := m.Track(c, root)
	root.Children(func() {
		if hero {
			ui.Box(c.Key("hero")).FillWidth().AspectRatio(4.8).Radius(tk.Radius["sm"]).Background(theme.C("hover")).Role(ui.RoleNone)
		}
		if width <= 0 {
			return // the first frame measures; the next one builds the tiles
		}
		cols, tile := kit.CoverColumns(width, tk.Metric["tile-min"])
		for s := range 3 {
			ui.Column(c.Key(s)).Children(func() {
				kit.Bar(c, 160, 16).Margin(0, 0, 12, 0)
				ui.Row(c.Key("tiles")).Gap(tk.Metric["tile-gap-x"]).Children(func() {
					for i := range min(cols, 10) {
						ui.Column(c.Key(i)).Width(tile).Shrink(0).Children(func() {
							kit.Bar(c, tile, tile)
							kit.Bar(c, tile*0.75, 14).Margin(8, 0, 0, 0)
							kit.Bar(c, tile*0.5, 12).Margin(6, 0, 0, 0)
						})
					}
				})
			})
		}
	})
}

// emptyPage is the page with no sections: "Nothing here right now".
func (d *discover) emptyPage(c *ui.Context) {
	kit.EmptyState(c, i18n.T("Nothing here right now"),
		i18n.T("Apple Music has no music on this page at the moment."), kit.EmptyOpts{Frame: kit.FrameCovers})
}
