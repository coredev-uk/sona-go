package home

import (
	"context"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// shelfLoading is how many placeholder tiles the shelf page shows while it
// loads.
const shelfLoading = 12

// shelfPage is the state of HomeShelfPage: the grid of the shelf showing and
// its item menu.
type shelfPage struct {
	id    string
	view  pageView
	grid  kit.ItemGridState
	menus *kit.ActionMenus
}

// shelfTitle is the window title of a shelf's page: its title once it has
// been read.
func (h *home) shelfTitle(r *ui.Route) string {
	if t := h.titles[r.Param("id")]; t != "" {
		return t
	}
	return i18n.T("For you")
}

// shelfPage is HomeShelfPage: a Home shelf in full (up to 100 items) as a
// grid, headed by its title.
func (h *home) shelfPage(c *ui.Context, r *ui.Route) {
	id := r.Param("id")
	g := &h.grid
	if g.id != id || g.menus == nil {
		*g = shelfPage{id: id}
		g.menus = kit.NewActionMenus(h.env.Actions, &g.grid.Sel)
	}
	h.scrollPage(c, &g.view, id, func(pane *kit.Pane) {
		q := h.env.Queries
		res := store.Use(q, keyHomeShelf(id), func(cx context.Context) (services.Section, error) {
			return h.disc.HomeShelf(cx, id)
		}, store.Options{Stale: store.Forever})
		if res.Err != nil {
			kit.ErrorBlock(c, i18n.T("Couldn't load this shelf"), res.Err, kit.ErrorBlockOpts{
				OnRetry:  func() { q.Invalidate(keyHomeShelf(id)) },
				Retrying: res.Fetching,
			})
			return
		}
		title := ""
		count := shelfLoading
		at := func(int) *services.Item { return nil }
		if res.HasData {
			title, count = res.Data.Title, len(res.Data.Items)
			h.titles[id] = title
			items := res.Data.Items
			at = func(i int) *services.Item { return &items[i] }
		}
		kit.PageHeader(c, title, "", nil)
		label := title
		if label == "" {
			label = i18n.T("Loading")
		}
		kit.ItemGrid(c, &g.grid, count, at, h.env.ItemsCtx(g.menus), kit.ItemGridOpts{Label: label, Pane: pane})
	})
}
