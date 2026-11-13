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
	// stripMin is the smallest cover of Recently Played, a strip of one row:
	// squares of 112 to about 130 DIPs with the name and the line under.
	stripMin = 112
	// heroRatio is Apple's hero card shape (music.apple.com crops the 4:1
	// banner to it), heroShare the banner's share of the row. The banner sits at the right end, the name
	// and the play disc at the left edge the page's text keeps.
	heroRatio float32 = 1060.0 / 608
	heroShare float32 = 0.45
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
		h.itemShelf(c, st, s.Title, st.cap.of(s.Items), ctx, shelfOpts{more: more, min: featureMin})
	case services.SectionConcerts:
		// Home's concerts shelf is the concerts feature's (ConcertsService
		// near the chosen city): features do not import each other, so the
		// concerts module hands it over through the Env.
		if h.env.ConcertsShelf != nil {
			h.env.ConcertsShelf(c, false)
		}
	case services.SectionShelf:
		switch {
		case s.Source == recentlyPlayed:
			h.stripShelf(c, st, s, ctx)
		case isStations(s):
			h.rowShelf(c, st, s, ctx)
		default:
			h.itemShelf(c, st, s.Title, st.cap.of(s.Items), ctx, shelfOpts{more: more})
		}
	default:
		h.editorial(c, st, s, ctx)
	}
}

// editorial sets a section as the shared editorial sections do (v0.2.0's
// EditorialSections over one section).
func (h *home) editorial(c *ui.Context, st *sectionState, s services.Section, ctx kit.ItemsCtx) {
	kit.EditorialSections(c, &st.ed, []services.Section{s}, ctx, kit.EditorialOpts{MoreFor: shelfMore})
}

// shelfOpts are the options of itemShelf.
type shelfOpts struct {
	// more is the section's own "See all" page, as a route path.
	more string
	// min is the smallest tile (the cover shelf's 160 when zero).
	min float32
}

// itemShelf is a ruled shelf of item tiles, as many as fit in one row and never
// scrolling sideways. A section with its own page links "See all" there;
// otherwise "Show all {count}" opens the rest in place when more were sent
// than fit. It builds nothing for an empty list.
func (h *home) itemShelf(c *ui.Context, st *sectionState, title string, items []services.Item, ctx kit.ItemsCtx, o shelfOpts) {
	if len(items) == 0 {
		return
	}
	cols := st.shelf.Row.Columns()
	ro := ruledOpts{Title: title}
	if o.more != "" {
		ro.More = func() {
			if ctx.Nav != nil {
				ctx.Nav(o.more)
			}
		}
	} else if (cols > 0 && len(items) > cols) || st.shelf.Open {
		ro.Actions = func() { showAll(c, len(items), &st.shelf.Open) }
	}
	ruledShelf(c, ro, func() {
		label := title
		if label == "" {
			label = i18n.T("Items")
		}
		if st.cover == nil {
			// The shelf's own menu over its own selection: the page's one
			// menu cannot act on every shelf's selection.
			st.cover = kit.NewActionMenus(h.env.Actions, &st.shelf.Row.Sel)
		}
		shown := len(items)
		rows := 1
		if st.shelf.Open {
			rows = -1
		} else if cols > 0 {
			shown = min(shown, cols)
		}
		tiles := make([]kit.CoverItem, shown)
		for i := range tiles {
			tiles[i] = kit.CoverItemOf(items[i], ctx, i)
		}
		kit.CoverRow(c, &st.shelf.Row, tiles, kit.CoverRowOpts{
			CoverOpts: kit.CoverOpts{
				Label: label, Min: o.min, Video: kit.RowAspect(items) == kit.AspectVideo, Art: ctx.Art,
				Motion: ctx.Motion, Menus: st.cover, Restricted: ctx.Restricted,
			},
			Rows:      rows,
			OnColumns: func(int) { c.Invalidate() }, // the toggle depends on how many fit
		})
	})
}

// stripShelf is Recently Played: one row of covers (squares of 112 to about
// 130 DIPs, the name and the line under each), the thing you go back to. It is
// one Tab stop like every list of Home: the arrow keys, Home and End move,
// Enter opens, Space plays. "Show all N" opens the rest in place.
func (h *home) stripShelf(c *ui.Context, st *sectionState, s services.Section, ctx kit.ItemsCtx) {
	rs := &st.rows
	so := ruledOpts{Title: s.Title}
	if more := shelfMore(s); more != "" {
		so.More = func() {
			if ctx.Nav != nil {
				ctx.Nav(more)
			}
		}
	} else if (rs.rows.cols > 0 && len(s.Items) > rs.rows.cols) || rs.open {
		so.Actions = func() { showAll(c, len(s.Items), &rs.open) }
	}
	ruledShelf(c, so, func() {
		h.itemRows(c, &rs.rows, st.cap.of(s.Items), ctx, rowsOpts{
			Label: s.Title, Rows: 1, Open: rs.open, Min: stripMin, Tile: true,
			OnColumns: func(int) { c.Invalidate() }, // the toggle depends on how many fit
		})
	})
}

// rowShelf is a shelf of stations as compact rows: what plays at once needs a
// play control and a name more than a large cover. "Show all N" opens the rest
// in place when the shelf has no page of its own and its rows overflow.
func (h *home) rowShelf(c *ui.Context, st *sectionState, s services.Section, ctx kit.ItemsCtx) {
	rs := &st.rows
	so := ruledOpts{Title: s.Title}
	if more := shelfMore(s); more != "" {
		so.More = func() {
			if ctx.Nav != nil {
				ctx.Nav(more)
			}
		}
	} else if (rs.rows.cols > 0 && len(s.Items) > rs.rows.cols*rowShelfRows) || rs.open {
		so.Actions = func() { showAll(c, len(s.Items), &rs.open) }
	}
	ruledShelf(c, so, func() {
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
	root := ui.Row(c.Key("hero")).Margin(shelfGapTop, 0, 0, 0).Gap(40).AlignItems(ui.Center).Role(ui.RoleGroup).Label(i18n.T("Featured"))
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
	// The box is whole DIPs in height too; its picture is asked for at it.
	bh := float32(math.Round(float64(w / heroRatio)))
	link := kit.ItemLink(it)
	target, hasTarget := kit.ActionItemOf(it, ctx.Players)
	root.Children(func() {
		ui.Column(c.Key("text")).Grow(1).Shrink(1).MinWidth(0).Children(func() {
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
		bg := theme.C("hover")
		if col, ok := kit.ParseHex(it.Color); ok {
			bg = col
		}
		ui.Box(c.Key("banner")).Size(w, bh).Shrink(0).Radius(tk.Radius["sm"]).ClipX().ClipY().Background(bg).Children(func() {
			if bh > 0 && src != "" {
				// The colour block stays while the picture loads and if it
				// fails; a square cover is shown whole on it.
				kit.Artwork(c, h.env.Art, src, w, kit.ArtOpts{
					Aspect: w / bh, Edge: true, Contain: banner == "", Fallback: func(*ui.Context) {},
				})
			}
		})
		menus.Popup(c)
	})
}
