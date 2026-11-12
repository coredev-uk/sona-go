package discover

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/devstate"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// The page's frame, from AppShell's main: 28 above, 48 below, the gutter at
// the sides (24 under 960 DIPs).
const (
	padTop    = 28
	padBottom = 48
	narrow    = 960
)

// pageView is a page's scroll container: the scroll place and the size of
// the last frame, which the next frame's gutter and virtualisation follow.
type pageView struct {
	pos ui.ScrollState
	// width and height are the container's, top its place in the window.
	width, height, top float32
	// key is what the page shows; another one starts from the top.
	key string
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
	if pg.key != key {
		if pg.key != "" {
			pg.pos.X, pg.pos.Y = 0, 0 // another page: from its top
		}
		pg.key = key
	}
	pg.laidY = pg.builtY
	tk := theme.Current()
	gutter := tk.Metric["gutter"]
	if pg.width > 0 && pg.width < narrow {
		gutter = tk.Metric["gutter-narrow"]
	}
	devstate.Scroll(c, &pg.pos) // dev builds: the offset survives a restart
	sc := ui.Scroll(c).TrackScroll(&pg.pos).Grow(1).Basis(0).MinHeight(0).MinWidth(0)
	sc.Children(func() {
		// WebKitGTK's scroll bar took 12 DIPs of the page; MyGo's floats over
		// it, so the same gutter keeps the content the size it was.
		scrollbar := tk.Metric["scrollbar"]
		ui.Column(c).Padding(padTop, gutter+scrollbar, padBottom, gutter).Children(func() {
			build(&kit.Pane{Pos: &pg.pos, Bounds: sc.Bounds}, gutter)
		})
	})
	pg.builtY = pg.pos.Y
	if b := sc.Bounds(); b.W != pg.width || b.H != pg.height || b.Y != pg.top {
		pg.width, pg.height, pg.top = b.W, b.H, b.Y
		c.Invalidate() // the gutter and the columns follow the width
	}
}

// masthead is a page's nameplate: the title at the display size and, on its
// baseline, the issue line (when the page was fetched), with the page's
// controls at the row's end. While the data loads, holding is a placeholder
// bar that keeps the line's place, so nothing moves when it lands.
func (d *discover) masthead(c *ui.Context, title, line string, holding bool, actions func()) {
	tk := theme.Current()
	ui.Row(c.Key("masthead")).FillWidth().Justify(ui.SpaceBetween).AlignItems(ui.Center).Gap(24).Margin(0, 0, 28, 0).Shrink(0).Children(func() {
		ui.Row(c.Key("name")).MinWidth(0).Shrink(1).Wrap().AlignItems(ui.Baseline).Children(func() {
			h := theme.Text(c, title, "display").TextColor(theme.C("ink")).SingleLine().Role(ui.RoleHeading).Level(1)
			if title == "" {
				h.Role(ui.RoleNone) // a page still loading has no title to read
			}
			switch {
			case holding:
				ui.Row(c).Height(tk.Type["secondary"].LineHeight).Margin(0, 0, 0, 16).AlignItems(ui.Center).Children(func() { bar(c, 112, 12) })
			case line != "":
				theme.Text(c, line, "secondary").TextColor(theme.C("ink-muted")).Margin(0, 0, 0, 16)
			}
		})
		if actions != nil {
			ui.Row(c.Key("actions")).Shrink(0).Gap(8).AlignItems(ui.Center).Children(actions)
		}
	})
}

// bar is a flat --hover placeholder bar (the skeletons of v0.2.0's pages).
func bar(c *ui.Context, w, h float32) ui.Element {
	return ui.Box(c).Width(w).Height(h).Radius(theme.Current().Radius["sm"]).Background(theme.C("hover")).Shrink(0).Role(ui.RoleNone)
}

// measure follows the width an element laid out at: the width of the last
// frame, 0 before the first (v0.2.0's useElementWidth).
type measure struct{ w float32 }

// track reads root's width and asks for another frame when it changes. Call
// it on the element before its children are built.
func (m *measure) track(c *ui.Context, root ui.Element) float32 {
	root.Draw(func(_ *ui.Painter, r ui.Rect) {
		if r.W != m.w {
			m.w = r.W
			c.Invalidate() // the columns follow the width
		}
	})
	if w := root.Bounds().W; w > 0 && m.w == 0 {
		m.w = w
	}
	return m.w
}

// pageError is the page-level failure: what failed (a full sentence), Go's
// reason, Try again. retrying is the query's fetch in flight; retry fetches it
// again.
func (d *discover) pageError(c *ui.Context, title string, err error, retrying bool, retry func()) {
	kit.ErrorBlock(c, title, err, kit.ErrorBlockOpts{OnRetry: retry, Retrying: retrying})
}

// shelvesSkeleton is flat placeholder bars at the real sizes of a page of
// shelves: three shelf bars of ten tiles (a row of as many as fit), and, with
// hero, the banner above (v0.2.0's ShelvesSkeleton).
func (d *discover) shelvesSkeleton(c *ui.Context, m *measure, hero bool) {
	tk := theme.Current()
	root := ui.Column(c.Key("skeleton")).FillWidth().Gap(40).Role(ui.RoleStatus).Label(i18n.T("Loading"))
	width := m.track(c, root)
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
				bar(c, 160, 16).Margin(0, 0, 12, 0)
				ui.Row(c.Key("tiles")).Gap(tk.Metric["tile-gap-x"]).Children(func() {
					for i := range min(cols, 10) {
						ui.Column(c.Key(i)).Width(tile).Shrink(0).Children(func() {
							ui.Box(c).Size(tile, tile).Radius(tk.Radius["sm"]).Background(theme.C("hover"))
							bar(c, tile*0.75, 14).Margin(8, 0, 0, 0)
							bar(c, tile*0.5, 12).Margin(6, 0, 0, 0)
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

// retry fetches a page again; the page shows it as retrying while it does.
func (d *discover) retry(key store.Key) func() {
	return func() { d.env.Queries.Invalidate(key) }
}
