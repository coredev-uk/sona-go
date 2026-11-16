package search

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// maxShelf is how many remembered entries the shelf shows.
const maxShelf = 12

// recentEntries are the entries the shelf shows for a scope: the terms
// searched there, and the items from it (library or catalog by source).
func recentEntries(all []services.RecentSearch, scope string) []services.RecentSearch {
	var out []services.RecentSearch
	for _, r := range all {
		if recentVisible(r, scope) {
			out = append(out, r)
		}
	}
	return out[:min(len(out), maxShelf)]
}

// recentShelf is v0.2.0's RecentSearches: the searches and opened results
// remembered on this computer, for the scope on screen. It builds nothing
// (false) when there are none; the first entry takes the field's ArrowDown.
func (s *search) recentShelf(c *ui.Context, pg *pageView, scope string) bool {
	r := s.recent()
	entries := recentEntries(r.Data, scope)
	if len(entries) == 0 {
		return false
	}
	title := i18n.T("Recently Searched")
	kit.Shelf(c, kit.ShelfOpts{Title: title, First: true, Actions: func() {
		if kit.Button(c.Key("clear"), i18n.T("Clear"), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeXS}).Clicked() {
			s.clearRecent()
		}
	}}, func() {
		cols, _ := kit.CoverColumns(pg.Inner(), 260)
		grid(c, "recent", title, cols, 24, 4, len(entries), func(i int) {
			s.recentEntry(c, pg, entries[i], scope)
		})
	})
	return true
}

// recentEntry is one remembered entry: a link that opens it again, and an X
// that forgets it, shown on hover and focus.
func (s *search) recentEntry(c *ui.Context, pg *pageView, e services.RecentSearch, scope string) {
	tk := theme.Current()
	var (
		path   string
		label  string
		onOpen func()
		art    func()
		plays  bool // a station with no page: choosing it plays it
	)
	if e.Kind == services.RecentTerm {
		label = e.Term
		path = searchLocation(e.Term, scope)
		onOpen = func() { s.rememberTerm(e.Term, scope) }
		art = func() {
			ui.Row(c).Size(40, 40).Shrink(0).Center().Radius(tk.Radius["sm"]).Background(theme.C("hover")).Children(func() {
				ui.Icon(c, theme.Icon("clock")).FontSize(16).TextColor(theme.C("ink-faint"))
			})
		}
	} else {
		path = recentPath(e)
		label = e.Name
		it := recentItem(e)
		plays = kit.PlaysWhenOpened(it, s.env.Players)
		onOpen = func() { s.rememberItem(it, e.Source) }
		art = func() { kit.Artwork(c, s.env.Art, e.Art, 40, kit.ArtOpts{Round: kit.ItemRound(it), Edge: true}) }
	}
	linked := e.Kind == services.RecentTerm || (path != "" && s.env.Routed(path))

	row := ui.Row(c.Key(e.Key)).FillWidth().AlignItems(ui.Center).Radius(tk.Radius["md"]).PaddingEnd(4)
	shown := kit.HoverRow(row)
	row.Children(func() {
		body := func() {
			art()
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Margin(0, 0, 0, 12).Children(func() {
				theme.Text(c, label, "body").TextColor(theme.C("ink")).SingleLine()
				if e.Kind != services.RecentTerm {
					if meta := itemMeta(recentItem(e)); meta != "" {
						theme.Text(c, meta, "secondary").TextColor(theme.C("ink-muted")).SingleLine()
					}
				}
			})
		}
		if linked || plays {
			main := ui.ButtonBase(c.Key("open")).Grow(1).Shrink(1).MinWidth(0).AlignItems(ui.Center).Padding(6, 8).
				Radius(tk.Radius["md"]).Role(ui.RoleLink)
			if plays {
				// Named by its visible text (WCAG 2.5.3), with the action after it.
				main.Role(ui.RoleButton).Label(e.Name + ", " + i18n.T("Play"))
			}
			if pg.claim() {
				main.Focus()
			}
			main.Children(func() {
				body()
				if plays {
					glyph := kit.PlayGlyph(c, 14, theme.GlyphOpts{Color: theme.C("ink-muted")}).Shrink(0)
					if !main.Hovered() && !main.FocusVisible() {
						glyph.Opacity(0)
					}
				}
			})
			if main.Clicked() {
				onOpen()
				if plays {
					kit.OpenWithoutPage(recentItem(e), s.env.ItemsCtx(nil))
				} else {
					s.env.Nav.Push(path)
				}
			}
		} else {
			ui.Row(c.Key("body")).Grow(1).Shrink(1).MinWidth(0).AlignItems(ui.Center).Padding(6, 8).Children(body)
		}
		x := kit.Button(c.Key("forget"), "", kit.ButtonOpts{
			Variant: kit.Ghost, Size: kit.SizeXS, Icon: "x", Muted: true,
			A11yLabel: i18n.T("Remove {label} from recent searches", "label", label),
		})
		if !shown {
			x.Opacity(0)
		}
		if x.Clicked() {
			s.forget(e.Key)
		}
	})
}
