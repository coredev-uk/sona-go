package home

import (
	"math"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

const (
	// rowShelfRows is the rows shown before "Show all" on a row shelf.
	rowShelfRows = 2
	// featureMin is the smallest cover of a feature shelf.
	featureMin = 176
	// heroRatio is Apple's hero card shape (music.apple.com crops the 4:1
	// banner to it), heroShare the banner's share of the row.
	heroRatio float32 = 1060.0 / 608
	heroShare float32 = 0.4
)

// captioned keeps the shelf's items without their shared caption between
// frames: withoutSharedCaption copies, and the rows and tiles compare their
// items by place.
type captioned struct {
	src *services.Item
	n   int
	out []services.Item
}

func (cp *captioned) of(items []services.Item) []services.Item {
	if len(items) == 0 {
		return items
	}
	if cp.src != &items[0] || cp.n != len(items) {
		cp.src, cp.n, cp.out = &items[0], len(items), withoutSharedCaption(items)
	}
	return cp.out
}

// homeSection is one shelf shown in full, set by what it holds.
func (h *home) homeSection(c *ui.Context, s services.Section, ctx kit.ItemsCtx) {
	st := h.section(s.ID)
	ctx.Menus = st.menus
	more := shelfMore(s)
	switch s.Kind {
	case services.SectionHero:
		if len(s.Items) == 1 {
			h.heroCard(c, &st.hero, s.Items[0], ctx, st.menus)
			return
		}
		h.editorial(c, st, s, ctx)
	case services.SectionFeature:
		kit.ItemShelf(c, &st.shelf, s.Title, st.cap.of(s.Items), ctx, kit.ItemShelfOpts{More: more, Min: featureMin})
	case services.SectionConcerts:
		// Home's concerts shelf is the concerts feature's (ConcertsService
		// near the chosen city): features do not import each other, so the
		// concerts module hands it over through the Env.
		if h.env.ConcertsShelf != nil {
			h.env.ConcertsShelf(c, false)
		}
	case services.SectionShelf:
		if s.Source == recentlyPlayed || isStations(s) {
			h.rowShelf(c, st, s, ctx)
			return
		}
		kit.ItemShelf(c, &st.shelf, s.Title, st.cap.of(s.Items), ctx, kit.ItemShelfOpts{More: more})
	default:
		h.editorial(c, st, s, ctx)
	}
}

// editorial sets a section as the shared editorial sections do (v0.2.0's
// EditorialSections over one section).
func (h *home) editorial(c *ui.Context, st *sectionState, s services.Section, ctx kit.ItemsCtx) {
	kit.EditorialSections(c, &st.ed, []services.Section{s}, ctx, kit.EditorialOpts{MoreFor: shelfMore})
}

// rowShelf is Recently Played and station shelves as compact rows: what you go
// back to and what plays at once need a play control and a name more than a
// large cover. "Show all N" opens the rest in place when the shelf has no
// page of its own and its rows overflow.
func (h *home) rowShelf(c *ui.Context, st *sectionState, s services.Section, ctx kit.ItemsCtx) {
	rs := &st.rows
	so := kit.ShelfOpts{Title: s.Title}
	if more := shelfMore(s); more != "" {
		so.More = func() {
			if ctx.Nav != nil {
				ctx.Nav(more)
			}
		}
	} else if (rs.rows.cols > 0 && len(s.Items) > rs.rows.cols*rowShelfRows) || rs.open {
		so.Actions = func() {
			label := i18n.T("Show all {count}", "count", len(s.Items))
			if rs.open {
				label = i18n.T("Show fewer")
			}
			if kit.ShelfDisclosure(c, label, rs.open).Clicked() {
				rs.open = !rs.open
			}
		}
	}
	kit.Shelf(c, so, func() {
		h.itemRows(c, &rs.rows, st.cap.of(s.Items), ctx, rowsOpts{
			Label: s.Title, Rows: rowShelfRows, Open: rs.open,
			OnColumns: func(int) { c.Invalidate() }, // the toggle depends on how many fit
		})
	})
}

// rowShelf is the state of a row shelf: its rows and whether they are all
// shown.
type rowShelf struct {
	rows rowsState
	open bool
}

// heroCard is an untitled hero shelf with one card (Apple's featured station
// on Home): the banner beside its name, line and play control, filling the row
// instead of leaving half of it empty.
type heroCard struct {
	width kit.WidthTracker
}

func (h *home) heroCard(c *ui.Context, hc *heroCard, it services.Item, ctx kit.ItemsCtx, menus *kit.ActionMenus) {
	tk := theme.Current()
	root := ui.Row(c.Key("hero")).Margin(40, 0, 0, 0).Gap(24).AlignItems(ui.End).Role(ui.RoleGroup).Label(i18n.T("Featured"))
	width := hc.width.Track(c, root)
	banner := it.Wide.Large
	if banner == "" {
		banner = it.Wide.Medium
	}
	src := banner
	if src == "" {
		if src = it.Art.Large; src == "" {
			src = it.Art.Medium
		}
	}
	w := float32(math.Round(float64(width * heroShare)))
	link := kit.ItemLink(it)
	target, hasTarget := kit.ActionItemOf(it, ctx.Players)
	root.Children(func() {
		bg := theme.C("hover")
		if col, ok := kit.ParseHex(it.Color); ok {
			bg = col
		}
		ui.Box(c.Key("banner")).Size(w, w/heroRatio).Shrink(0).Radius(tk.Radius["sm"]).ClipX().ClipY().Background(bg).Children(func() {
			if w > 0 && src != "" {
				// The colour block stays while the picture loads and if it
				// fails; a square cover is shown whole on it.
				kit.Artwork(c, h.env.Art, src, w, kit.ArtOpts{
					Aspect: heroRatio, Edge: true, Contain: banner == "", Fallback: func(*ui.Context) {},
				})
			}
		})
		ui.Column(c.Key("text")).Grow(1).Shrink(1).MinWidth(0).Padding(0, 0, 4, 0).Children(func() {
			if h.titleBlock(c, it, "page", link).Clicked() {
				h.open(it, ctx)
			}
			ui.Row(c.Key("controls")).Margin(20, 0, 0, 0).Gap(8).AlignItems(ui.Center).Children(func() {
				if ctx.Players.CanPlay(it) {
					if play, _ := kit.PlayButtons(c, kit.PlayButtonsOpts{Label: it.Name}); play {
						ctx.Players.PlayItem(it)
					}
				}
				if hasTarget {
					menus.Trigger(c.Key("more"), kit.ActionTarget{Item: target, Name: it.Name}, kit.TriggerOpts{Header: true})
				}
			})
		})
		menus.Popup(c)
	})
}
