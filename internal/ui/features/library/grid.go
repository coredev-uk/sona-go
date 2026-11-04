package library

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// coverList is the state of a paged list shown as a cover grid.
type coverList struct {
	st    kit.CoverState
	menus *kit.ActionMenus
	pager
}

// itemMenus is the item menu of a list over its selection, made once; nil
// when the shell has no action registry.
func (l *library) itemMenus(m **kit.ActionMenus, sel kit.SelectionScope) *kit.ActionMenus {
	if *m == nil && l.env.Actions != nil {
		*m = kit.NewActionMenus(l.env.Actions, sel)
	}
	return *m
}

// restricted: the account may not play explicit items.
func (l *library) restricted() bool {
	auth := l.authStatus()
	return auth.HasData && auth.Data.Restricted
}

// coverGrid builds a paged list as a kit.CoverGrid in pane: tile makes the
// tile of a loaded row, and a list still reading its first page shows
// loading tiles at their real sizes. The rows built decide the pages fetched.
func coverGrid[T any](l *library, c *ui.Context, g *coverList, pane *kit.Pane, list *listView[T], label string, tile func(*T) kit.CoverItem) {
	count := list.Total
	at := func(i int) *kit.CoverItem {
		row := list.At(i)
		if row == nil {
			return nil
		}
		t := tile(row)
		return &t
	}
	if list.Pending() {
		count, at = skeletonTiles, func(int) *kit.CoverItem { return nil }
	}
	kit.CoverGrid(c, &g.st, count, at, kit.CoverGridOpts{
		CoverOpts: kit.CoverOpts{
			Label: label, Art: l.env.Art, Motion: l.env.Motion, Restricted: l.restricted(),
			Menus: l.itemMenus(&g.menus, &g.st.Sel),
		},
		Pane:    pane,
		OnRange: g.onRange(c),
	})
}
