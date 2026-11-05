package kit

import (
	"github.com/egoist/mygo/ui"
)

// CoverRowOpts are the options of CoverRow.
type CoverRowOpts struct {
	CoverOpts
	// Rows is how many rows of tiles to show (0: one); a negative number
	// shows every tile.
	Rows int
	// OnColumns reports how many tiles fit in one row, when it changes.
	OnColumns func(cols int)
}

// CoverRow is a shelf's tiles (v0.2.0's CoverRow): as many cover tiles as fit
// in o.Rows rows at the grid's tile size, never scrolling sideways; the rest
// are left out, but the covers of the next row are decoded while the shelf is
// on screen. Pair it with a Shelf's "See all". Tiles select, open their menu
// and drag as CoverGrid's. Its width is its parent's.
func CoverRow(c *ui.Context, st *CoverState, items []CoverItem, o CoverRowOpts) {
	root := ui.Column(c.Key("kit.row")).FillWidth().Gap(TileGapY).Role(ui.RoleTable).Label(o.Label)
	root.Draw(func(_ *ui.Painter, r ui.Rect) {
		if r.W != st.width {
			st.width = r.W
			c.Invalidate()
		}
	})
	if w := root.Bounds().W; w > 0 && st.width == 0 {
		st.width = w
	}
	root.Children(func() {
		if st.width <= 0 {
			ui.Box(c).Height(1)
			return
		}
		cols, tile := CoverColumns(st.width, o.Min)
		if st.cols != cols {
			st.cols = cols
			if o.OnColumns != nil {
				o.OnColumns(cols)
			}
		}
		rows := o.Rows
		if rows == 0 {
			rows = 1
		}
		n := len(items)
		if rows > 0 {
			n = min(n, cols*rows)
		}
		at := func(i int) *CoverItem { return &items[i] }
		st.setRows(n, at, cols, o.Menus, o.Hold)
		st.nav.Page, st.nav.Ends, st.nav.ScrollTo = 0, false, nil
		// One row for assistive technology per row of tiles.
		for r := 0; r*cols < n; r++ {
			ui.Row(c.Key(r)).Gap(TileGapX).AlignItems(ui.Start).Role(ui.RoleRow).Children(func() {
				for i := r * cols; i < min(n, (r+1)*cols); i++ {
					st.tile(c, &o.CoverOpts, &items[i], i, tile)
				}
			})
		}
		// The next row, which "Show all" opens, is decoded while the shelf
		// is on screen: a shelf further down the page waits its turn.
		if rows > 0 && len(items) > n && root.InView() {
			for i := n; i < min(len(items), n+cols); i++ {
				st.prefetch(c, &o.CoverOpts, &items[i], tile)
			}
		}
		st.nav.Tick(c)
		st.Sel.OutsidePress(root)
		if o.Menus != nil && !o.NoPopup {
			o.Menus.Popup(c)
		}
	})
}
