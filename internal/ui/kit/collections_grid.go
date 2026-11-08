package kit

import (
	"slices"

	"github.com/egoist/mygo/ui"
)

// Pane is the scroll container a virtual collection is built in: its scroll
// state and its box in the window (the previous frame's, as Element.Bounds
// gives it). A collection built in a pane builds only the rows showing, and
// a few beyond.
type Pane struct {
	Pos    *ui.ScrollState
	Bounds func() ui.Rect
}

// CoverGridOpts are the options of CoverGrid.
type CoverGridOpts struct {
	CoverOpts
	// Pane is the scroll container the grid is built in. A grid of more
	// than VirtualFrom items in a pane is built by row, as it shows; without
	// one every row is built.
	Pane *Pane
	// OnRange reports the tiles built, first to last (inclusive), when they
	// change: for paged loading.
	OnRange func(start, end int)
	// Ahead returns tile i with at least its Art, Round and Video, for the
	// covers the grid has the loader decode before their rows are built: a
	// cheaper stand-in for at. Nil: at.
	Ahead func(i int) *CoverItem
}

// CoverGrid is a grid of cover tiles (v0.2.0's CoverGrid): tiles at least
// o.Min wide, 24 DIPs across and 28 down, growing to fill the row, at an
// exact width so artwork is decoded at the size it shows. A click opens a
// tile; Ctrl+click and Shift+click (or Shift+arrows, Ctrl+A) select tiles;
// arrows move between them; right-click or Shift+F10 opens the tile's menu,
// which acts on the whole selection when the tile is in it; tiles drag onto
// playlists, pins and the queue. at returns tile i, nil for one whose page is
// still loading.
//
// The grid takes the width of its parent and its height from its rows. In a
// pane with more than VirtualFrom items it builds only the rows in view and
// three beyond, and has the loader decode the covers of the two rows after
// those; the scroll position keeps the same tiles at the top when the columns
// change with the width.
func CoverGrid(c *ui.Context, st *CoverState, count int, at func(i int) *CoverItem, o CoverGridOpts) {
	root := ui.Column(c.Key("kit.grid")).FillWidth().Role(ui.RoleTable).Label(o.Label)
	root.Draw(func(_ *ui.Painter, r ui.Rect) {
		if r.W != st.width {
			st.width = r.W
			c.Invalidate() // the columns follow the width
		}
		if o.Pane != nil {
			// Paint runs after this frame's layout: the box and the scroll
			// offset belong to the same frame.
			top := r.Y - o.Pane.Bounds().Y + o.Pane.Pos.Y
			if !st.drawn || top != st.drawTop {
				st.drawTop, st.drawn = top, true
				c.Invalidate()
			}
		}
	})
	if w := root.Bounds().W; w > 0 && st.width == 0 {
		st.width = w
	}
	root.Children(func() {
		if st.width <= 0 {
			ui.Box(c).Height(1) // the first frame measures; the next builds the tiles
			return
		}
		cols, tile := CoverColumns(st.width, o.Min)
		rowH := CoverRowHeight(tile, o.Video)
		rows := (count + cols - 1) / cols
		virtual := count > VirtualFrom && o.Pane != nil

		var built []int
		if virtual {
			built = st.window(root, o.Pane, rows, cols, rowH)
		} else {
			for r := range rows {
				built = append(built, r)
			}
			st.cols, st.rowH = cols, rowH
		}
		st.setRows(count, at, cols, o.Menus, o.Hold)
		st.nav.Page = 0
		st.nav.Ends = false
		st.nav.ScrollTo = nil
		if virtual {
			st.nav.ScrollTo = func(i int) { st.scrollTo(o.Pane, i/cols, rowH) }
		}
		winFirst, winLast := 0, rows-1
		if len(built) > 0 {
			winFirst, winLast = built[0], built[len(built)-1] // not the dragged row's
		}
		if i, ok := DragPinned(st); ok && i < count && !slices.Contains(built, i/cols) {
			built = append(built, i/cols)
			slices.Sort(built)
		}

		prev := -1
		for _, r := range built {
			if gap := r - prev - 1; gap > 0 {
				ui.Box(c.Key(gapKey(prev))).Height(float32(gap) * rowH)
			}
			h := rowH
			if r == rows-1 {
				h -= TileGapY
			}
			ui.Row(c.Key(r)).Height(h).Gap(TileGapX).AlignItems(ui.Start).Role(ui.RoleRow).Children(func() {
				for i := r * cols; i < min(count, (r+1)*cols); i++ {
					st.tile(c, &o.CoverOpts, at(i), i, count, tile)
				}
			})
			prev = r
		}
		if gap := rows - 1 - prev; gap > 0 && len(built) > 0 {
			ui.Box(c.Key(gapKey(rows))).Height(float32(gap) * rowH)
		}
		if virtual {
			// The rows just past the built ones: their covers decode before
			// a scroll brings them in.
			ahead := at
			if o.Ahead != nil {
				ahead = o.Ahead
			}
			for r := winLast + 1; r <= min(rows-1, winLast+coverAhead); r++ {
				for i := r * cols; i < min(count, (r+1)*cols); i++ {
					st.prefetch(c, &o.CoverOpts, ahead(i), tile)
				}
			}
		}
		st.nav.Tick(c)
		st.Sel.OutsidePress(root)
		st.report(winFirst, winLast, cols, count, o.OnRange)
		if o.Menus != nil && !o.NoPopup {
			o.Menus.Popup(c)
		}
	})
}

// window works out which rows of a virtual grid to build: those in view and
// coverOverscan beyond, and keeps the same tiles at the top when the
// columns change.
func (st *CoverState) window(root ui.Element, p *Pane, rows, cols int, rowH float32) []int {
	pane := p.Bounds()
	if st.drawn {
		st.top = st.drawTop
	} else {
		st.top = root.Bounds().Y - pane.Y + p.Pos.Y
	}
	if st.cols != cols && st.cols > 0 && p.Pos.Y > st.top && st.rowH > 0 {
		item := int((p.Pos.Y-st.top)/st.rowH) * st.cols
		p.Pos.Y = st.top + float32(item/cols)*rowH
	}
	st.cols, st.rowH = cols, rowH
	y := p.Pos.Y - st.top
	h := pane.H
	if h <= 0 {
		h = rowH * 6 // before the first layout
	}
	first := max(0, int(y/rowH))
	last := min(rows-1, int((y+h)/rowH))
	from, to := max(0, first-coverOverscan), min(rows-1, last+coverOverscan)
	out := make([]int, 0, max(0, to-from+1))
	for r := from; r <= to; r++ {
		out = append(out, r)
	}
	return out
}

// scrollTo scrolls the pane as little as shows row r (its tiles, not the
// gap below), so the row gets built.
func (st *CoverState) scrollTo(p *Pane, r int, rowH float32) {
	pane := p.Bounds()
	top := st.top + float32(r)*rowH
	bottom := top + rowH - TileGapY
	const margin = 8
	switch {
	case top-margin < p.Pos.Y:
		p.Pos.Y = max(0, top-margin)
	case pane.H > 0 && bottom+margin > p.Pos.Y+pane.H:
		p.Pos.Y = bottom + margin - pane.H
	}
}

// report tells OnRange which tiles are built, when that changed.
func (st *CoverState) report(firstRow, lastRow, cols, count int, fn func(start, end int)) {
	if fn == nil || count == 0 {
		return
	}
	first, last := firstRow*cols, min(count, (lastRow+1)*cols)-1
	if st.reported == [2]int{first + 1, last + 1} {
		return
	}
	st.reported = [2]int{first + 1, last + 1}
	fn(first, last)
}

// gapKey keys the spacer after row n of a virtual grid.
type gapKey int
