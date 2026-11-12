package home

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// Geometry of the index lines (v0.2.0's ShelfIndex), in DIPs.
const (
	// thumb is a thumbnail's edge, thumbGap the gap between thumbnails.
	thumb    = 44
	thumbGap = 8
	// indexH is a line's height; indexPad its side padding; indexChevron
	// the chevron's width.
	indexH       = 64
	indexPad     = 8
	indexChevron = 16
	indexGap     = 12
	// The title column is 28% of the line, between these.
	indexColShare            = 0.28
	indexColMin, indexColMax = 160, 280
)

// indexLine is the state of one index line: whether it is open, its width
// and its cover row (selection, navigation, menu).
type indexLine struct {
	open  bool
	width kit.WidthTracker
	row   kit.CoverState
	menus *kit.ActionMenus
	cap   captioned

	// holds is what the line's section holds, kept for the items and locale
	// it was formatted for.
	holds       string
	holdsFirst  *services.Item
	holdsN      int
	holdsLocale string
}

// summary is what the section holds, formatted again only when its items or
// the locale change.
func (ln *indexLine) summary(items []services.Item) string {
	var first *services.Item
	if len(items) > 0 {
		first = &items[0]
	}
	if loc := i18n.Locale(); ln.holdsFirst != first || ln.holdsN != len(items) || ln.holdsLocale != loc {
		ln.holds, ln.holdsFirst, ln.holdsN, ln.holdsLocale = holds(items), first, len(items), loc
	}
	return ln.holds
}

func (h *home) indexState(id string) *indexLine {
	ln := kit.Entry(&h.index, id)
	kit.MenusIn(&ln.menus, h.env.Actions, &ln.row.Sel)
	return ln
}

// shelfIndex is the rest of Apple's shelves (genres, "more like", fans also
// like), one line each: the title, what it holds, and a strip of its covers. A
// line opens its shelf in place. Twelve shelves take one screen instead of
// four, and their full-size covers load only when asked for.
func (h *home) shelfIndex(c *ui.Context, sections []services.Section, ctx kit.ItemsCtx) {
	title := i18n.T("More for you")
	ui.Column(c.Key("index")).Margin(48, 0, 0, 0).Role(ui.RoleGroup).Label(title).Children(func() {
		theme.Text(c, title, "heading").TextColor(theme.C("ink")).SingleLine().Margin(0, 0, 8, 0).Role(ui.RoleHeading).Level(2)
		ui.Column(c.Key("list")).Role(ui.RoleList).Children(func() {
			for i := range sections {
				h.indexLine(c, sections[i], ctx)
			}
		})
	})
}

func (h *home) indexLine(c *ui.Context, s services.Section, ctx kit.ItemsCtx) {
	tk := theme.Current()
	ln := h.indexState(s.ID)
	summary := ln.summary(s.Items)
	ui.Column(c.Key(s.ID)).Role(ui.RoleListItem).Children(func() {
		name := s.Title
		if summary != "" {
			name += ", " + summary
		}
		btn := ui.ButtonBase(c.Key("line")).Height(indexH).FillWidth().Gap(indexGap).PaddingX(indexPad).Radius(tk.Radius["md"]).
			Role(ui.RoleDisclosure).Expanded(ln.open).Label(name)
		width := ln.width.Track(c, btn)
		bg := ui.Transparent
		if btn.Hovered() {
			bg = theme.C("hover")
		}
		btn.Background(bg)
		theme.Fade(btn, "press")
		btn.Children(func() {
			ic := theme.Glyph(c, "chevron-right", indexChevron, theme.GlyphOpts{Color: theme.C("ink-muted")}).Shrink(0)
			turn := float32(0)
			if ln.open {
				turn = 90
			}
			if theme.ReducedMotion() {
				ic.Rotate(turn)
			} else {
				ic.Rotate(ic.Animate("turn", turn, tk.Motion["press"].Duration))
			}
			inner := width - 2*indexPad
			col := min(max(indexColShare*inner, indexColMin), indexColMax)
			ui.Column(c.Key("title")).Width(col).Shrink(0).MinWidth(0).Children(func() {
				theme.Text(c, s.Title, "heading").TextColor(theme.C("ink")).SingleLine().TextAlign(ui.Start)
				if summary != "" {
					theme.Text(c, summary, "meta").TextColor(theme.C("ink-muted")).FontFeatures("tnum").SingleLine().TextAlign(ui.Start)
				}
			})
			// Open, the shelf's own covers are right below: no strip.
			ui.Row(c.Key("strip")).Grow(1).MinWidth(0).Gap(thumbGap).ClipX().Role(ui.RoleNone).Children(func() {
				if ln.open || width <= 0 {
					return
				}
				strip := inner - indexChevron - col - 2*indexGap
				for i := range min(len(s.Items), max(0, int((strip+thumbGap)/(thumb+thumbGap)))) {
					it := &s.Items[i]
					art := it.Art.Small
					if art == "" {
						art = it.Art.Medium
					}
					ui.Box(c.Key(i)).Shrink(0).Children(func() {
						kit.Artwork(c, h.env.Art, art, thumb, kit.ArtOpts{Round: kit.ItemRound(*it)})
					})
				}
			})
		})
		if btn.Clicked() {
			ln.open = !ln.open
		}
		if ln.open {
			btn.Controls(h.indexPanel(c, ln, s, ctx))
		}
	})
}

// indexPanel is an open line's shelf: its covers in place, and "See all" when
// the section has a page of its own.
func (h *home) indexPanel(c *ui.Context, ln *indexLine, s services.Section, ctx kit.ItemsCtx) ui.Element {
	items := ln.cap.of(s.Items)
	label := s.Title
	if label == "" {
		label = i18n.T("Items")
	}
	return ui.Column(c.Key("panel")).Padding(8, indexPad, 32, indexPad).Role(ui.RoleGroup).Label(label).Children(func() {
		tiles := make([]kit.CoverItem, len(items))
		for i := range items {
			tiles[i] = kit.CoverItemOf(items[i], ctx, i)
		}
		kit.CoverRow(c, &ln.row, tiles, kit.CoverRowOpts{
			CoverOpts: kit.CoverOpts{
				Label: label, Video: kit.RowAspect(items) == kit.AspectVideo, Art: h.env.Art, Motion: h.env.Motion,
				Menus: ln.menus, Restricted: ctx.Restricted,
			},
			Rows: -1,
		})
		if more := shelfMore(s); more != "" {
			ui.Row(c.Key("more")).Margin(16, 0, 0, 0).Children(func() {
				kit.ShelfSeeAll(c, "see-all", func() {
					if ctx.Nav != nil {
						ctx.Nav(more)
					}
				})
			})
		}
	})
}

// holds is what a shelf holds, by kind: "8 albums · 4 playlists".
func holds(items []services.Item) string {
	var kinds []string
	count := map[string]int{}
	for i := range items {
		k := items[i].Kind
		if count[k] == 0 {
			kinds = append(kinds, k)
		}
		count[k]++
	}
	parts := make([]string, 0, len(kinds))
	for _, k := range kinds {
		n := count[k]
		switch k {
		case services.ItemAlbum:
			parts = append(parts, i18n.T("{n, plural, one {# album} other {# albums}}", "n", n))
		case services.ItemPlaylist:
			parts = append(parts, i18n.T("{n, plural, one {# playlist} other {# playlists}}", "n", n))
		case services.ItemStation:
			parts = append(parts, i18n.T("{n, plural, one {# station} other {# stations}}", "n", n))
		case services.ItemArtist:
			parts = append(parts, i18n.T("{n, plural, one {# artist} other {# artists}}", "n", n))
		case services.ItemSong:
			parts = append(parts, i18n.T("{n, plural, one {# song} other {# songs}}", "n", n))
		}
	}
	return strings.Join(parts, " · ")
}
