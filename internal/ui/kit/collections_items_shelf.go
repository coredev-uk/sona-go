package kit

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
)

// ItemShelfOpts are the options of ItemShelf.
type ItemShelfOpts struct {
	// First leaves out the 40 DIPs above: the first shelf of a page.
	First bool
	// More is the section's own "See all" page, as a route path; without
	// one, extra items open in place.
	More string
	// Min is the smallest tile width (default 160; feature shelves use
	// larger tiles).
	Min float32
	// Rows is how many rows show before "Show all" (default 1).
	Rows int
}

// ItemShelf is a titled row of item tiles (v0.2.0's ItemShelf): as many as
// fit, never scrolling sideways. A section with its own page links "See all"
// there; otherwise a "Show all {count}" toggle opens the rest of its items in
// place when there are more than the rows show. The row is labelled by the
// title ("Items" when it has none); the tile row is 16:9 only when every item
// is a video. It builds nothing for an empty list.
func ItemShelf(c *ui.Context, st *CoverShelfState, title string, items []services.Item, ctx ItemsCtx, o ItemShelfOpts) {
	if len(items) == 0 {
		return
	}
	rows := o.Rows
	if rows <= 0 {
		rows = 1
	}
	so := ShelfOpts{Title: title, First: o.First}
	if o.More != "" {
		so.More = func() { ctx.nav(o.More) }
	} else if (st.cols > 0 && len(items) > st.cols*rows) || st.Open {
		so.Actions = func() {
			label := i18n.T("Show all {count}", "count", len(items))
			if st.Open {
				label = i18n.T("Show fewer")
			}
			if ShelfToggle(c, label, st.Open).Clicked() {
				st.Open = !st.Open
			}
		}
	}
	Shelf(c, so, func() {
		label := title
		if label == "" {
			label = i18n.T("Items")
		}
		video := RowAspect(items) == AspectVideo
		// The shelf's own menu over its own selection: the page's one menu
		// cannot act on every shelf's selection. It draws its own popup.
		ctx.Menus, ctx.NoPopup = st.Row.menusOver(ctx.Menus), false
		ro := CoverRowOpts{CoverOpts: ctx.coverOpts(label, o.Min, video), Rows: rows, OnColumns: func(n int) {
			st.cols = n
			c.Invalidate() // the toggle depends on how many fit
		}}
		// Build only the tiles that show: a row, or every one when open; and
		// a row more of covers only, which CoverRow has the loader decode
		// ahead.
		shown, n := len(items), len(items)
		if st.Open {
			ro.Rows = -1
		} else if st.cols > 0 {
			shown = min(shown, st.cols*rows)
			n = min(n, st.cols*(rows+1))
		}
		tiles := make([]CoverItem, n)
		for i := range tiles {
			if i < shown {
				tiles[i] = CoverItemOf(items[i], ctx, i)
			} else {
				tiles[i] = itemAheadTile(items[i])
			}
		}
		CoverRow(c, &st.Row, tiles, ro)
	})
}

// ItemGridState is the state of an ItemGrid: its tiles' selection and
// navigation, and the tiles built this frame.
type ItemGridState struct {
	CoverState
	built map[int]*CoverItem
}

// ItemGridOpts are the options of ItemGrid.
type ItemGridOpts struct {
	// Label names the grid for assistive technology.
	Label string
	// Min is the smallest tile width (default 160).
	Min float32
	// Pane is the scroll container the grid is built in (see CoverGridOpts).
	Pane *Pane
	// OnRange reports the tiles built, first to last (inclusive), when they
	// change: for paged loading.
	OnRange func(start, end int)
}

// ItemGrid is every item as a tile grid (v0.2.0's ItemGrid: rooms, "See all"
// pages, charts), virtualised when long. at returns item i, nil for one whose
// page is still loading (a loading tile); the tiles' aspect is RowAspect of
// the loaded ones.
func ItemGrid(c *ui.Context, st *ItemGridState, count int, at func(i int) *services.Item, ctx ItemsCtx, o ItemGridOpts) {
	// The aspect is RowAspect of the loaded items: video when there are some
	// and every one is a video frame.
	video, loaded := true, false
	for i := range count {
		if it := at(i); it != nil {
			loaded = true
			if !ItemVideo(*it) {
				video = false
				break
			}
		}
	}
	video = video && loaded
	clear(st.built)
	if st.built == nil {
		st.built = map[int]*CoverItem{}
	}
	tile := func(i int) *CoverItem {
		if t, ok := st.built[i]; ok {
			return t
		}
		var t *CoverItem
		if it := at(i); it != nil {
			ci := CoverItemOf(*it, ctx, i)
			t = &ci
		}
		st.built[i] = t
		return t
	}
	g := CoverGridOpts{CoverOpts: ctx.coverOpts(o.Label, o.Min, video), Pane: o.Pane, OnRange: o.OnRange}
	g.Ahead = func(i int) *CoverItem {
		if it := at(i); it != nil {
			t := itemAheadTile(*it)
			return &t
		}
		return nil
	}
	CoverGrid(c, &st.CoverState, count, tile, g)
}
