package search

import (
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// The results page's parts: the query as the title with its scope, the rail
// (or, narrow, the row) of the groups found, the best match, and each group
// as a block the rail jumps to.

const (
	// bestArt is the best match's cover, bestRows how many other top results
	// list beside it; bestSplit is the width of the results column from which
	// they sit beside it rather than below.
	bestArt   = 120
	bestRows  = 4
	bestSplit = 640
	// portrait is the smallest artist tile: a round cover about 100 across.
	portrait = 104
	// coverMin is the smallest tile of albums, playlists and the other item
	// shelves, denser than the 160 of other pages.
	coverMin = 136
	// songAlbumFrom is the results column's width from which the song list
	// shows its Album column too.
	songAlbumFrom = 720
)

// railEntry is one group found: what the rail and the narrow row list.
type railEntry struct {
	id, title string
	count     int
}

// groupID is the anchor of a result group.
func groupID(group string) string { return "g:" + group }

// queryHeader is the page's title, the term, with the scope switch on its
// baseline at the end.
func (s *search) queryHeader(c *ui.Context, q, scope string) {
	title := i18n.T("Search")
	if q != "" {
		title = "“" + q + "”"
	}
	ui.Row(c.Key("header")).FillWidth().AlignItems(ui.Baseline).Justify(ui.SpaceBetween).Gap(24).Margin(0, 0, 32, 0).Shrink(0).Children(func() {
		theme.Text(c, title, "display").TextColor(theme.C("ink")).SingleLine().Shrink(1).MinWidth(0).Role(ui.RoleHeading).Level(1)
		s.scopeSwitch(c, q, scope)
	})
}

// railList is the groups found as the rail: one jump button per group, its
// name and its count, the one in view marked. Built into the rail's column.
func (s *search) railList(c *ui.Context, pv *pageView, entries []railEntry) {
	label := i18n.T("Result groups")
	ids := make([]string, len(entries))
	for i, e := range entries {
		ids[i] = e.id
	}
	cur := pv.current(ids)
	if cur == "" && len(ids) > 0 {
		cur = ids[0]
	}
	ui.Column(c.Key("rail")).FillWidth().Gap(2).Role(ui.RoleGroup).Label(label).Children(func() {
		for _, e := range entries {
			s.jumpButton(c, pv, e, e.id == cur, true)
		}
	})
}

// chipRow is the rail, narrow: the groups found as a wrapping row of jump
// buttons above the results.
func (s *search) chipRow(c *ui.Context, pv *pageView, entries []railEntry) {
	label := i18n.T("Result groups")
	ui.Row(c.Key("chips")).FillWidth().Wrap().Gap(8).Margin(0, 0, 32, 0).Role(ui.RoleGroup).Label(label).Children(func() {
		for _, e := range entries {
			s.jumpButton(c, pv, e, false, false)
		}
	})
}

// jumpButton is a group's button: its name and count; in the rail the full
// width and a row, as a chip a pill with a line round it. The group in view
// is on the hover tone.
func (s *search) jumpButton(c *ui.Context, pv *pageView, e railEntry, current, rail bool) {
	tk := theme.Current()
	b := ui.ButtonBase(c.Key(e.id)).Height(32).PaddingX(10).Radius(tk.Radius["md"]).AlignItems(ui.Center).Gap(8).
		Label(i18n.T("{title}, {count}", "title", e.title, "count", e.count))
	if rail {
		b.FillWidth()
	} else {
		b.Border(1, theme.C("line"))
	}
	fg := theme.C("ink-muted")
	switch {
	case current:
		b.Background(theme.C("hover"))
		fg = theme.C("ink")
	case b.Hovered():
		b.Background(theme.C("hover").Alpha(0.6))
		fg = theme.C("ink")
	}
	b.Children(func() {
		w := 500
		if current {
			w = 600
		}
		theme.Text(c, e.title, "label").FontWeight(w).TextColor(fg).SingleLine().Grow(1).MinWidth(0).Role(ui.RoleNone)
		theme.Text(c, strconv.Itoa(e.count), "secondary").TextColor(theme.C("ink-faint")).FontFeatures("tnum").SingleLine().Shrink(0).Role(ui.RoleNone)
	})
	if b.Clicked() {
		pv.jump(e.id)
	}
}

// railSkeleton is the rail while the results load: a bar per group, at the
// buttons' size.
func railSkeleton(c *ui.Context) {
	ui.Column(c.Key("rail-skeleton")).FillWidth().Gap(2).Role(ui.RoleNone).Children(func() {
		for i, w := range []float32{88, 64, 72, 56, 80} {
			ui.Row(c.Key(i)).FillWidth().Height(32).PaddingX(10).AlignItems(ui.Center).Gap(8).Children(func() {
				kit.Skeleton(c, w, 14)
			})
		}
	})
}

// groupBlock is one result group: a hairline above (not the first), the
// group's own shelf, and the place the rail jumps to.
func (s *search) groupBlock(c *ui.Context, pv *pageView, group, title string, first bool, build func()) {
	sec := ui.Column(c.Key(groupID(group))).FillWidth()
	if !first {
		sec.Margin(32, 0, 0, 0)
	}
	pv.anchored(sec, groupID(group), title)
	sec.Children(func() {
		if !first {
			ui.Box(c.Key("rule")).FillWidth().Height(1).Background(theme.C("line")).Margin(0, 0, 24, 0).Role(ui.RoleNone)
		}
		build()
	})
}

// topResults is the Top results: the first as the best match panel, the
// next few as compact rows beside it (below it, narrow).
func (s *search) topResults(c *ui.Context, ctx kit.ItemsCtx, q string, items []services.Item) {
	v := &s.pages.search
	best := items[0]
	rest := items[1:min(len(items), 1+bestRows)]
	wide := v.inner <= 0 || v.inner >= bestSplit
	topFlow(c, wide).Children(func() {
		s.bestMatch(c, ctx, best, v.claim(), len(rest) > 0 && wide)
		if len(rest) == 0 {
			return
		}
		col := ui.Column(c.Key("next")).Role(ui.RoleList).Label(i18n.T("{title} for {q}", "title", groupTitle("top"), "q", q)).Gap(2)
		if wide {
			col.Grow(2).Basis(0).MinWidth(0).Justify(ui.Center)
		} else {
			col.FillWidth()
		}
		col.Children(func() {
			for i, it := range rest {
				ui.Column(c.Key(strconv.Itoa(i+1) + ":" + it.Kind + ":" + it.ID)).FillWidth().Role(ui.RoleListItem).Children(func() {
					s.itemRow(c, it, rowOpts{
						key: "row", size: 40, onOpen: ctx.OnOpen, menus: s.itemMenus(&v.rowMenus, nil),
					})
				})
			}
		})
	})
	if v.rowMenus != nil {
		v.rowMenus.Popup(c)
	}
}

// topFlow holds the best match and the rows: side by side when wide, else
// one above the other.
func topFlow(c *ui.Context, wide bool) ui.Element {
	if wide {
		return ui.Row(c.Key("top")).FillWidth().Gap(24).AlignItems(ui.Stretch)
	}
	return ui.Column(c.Key("top")).FillWidth().Gap(16)
}

// bestMatch is the first top result large: the cover, its kind, its name at
// the title size and its catalogue line, with the amber Play disc where the
// result plays (opening it is the panel). beside: it shares its line with
// the compact rows.
func (s *search) bestMatch(c *ui.Context, ctx kit.ItemsCtx, it services.Item, focus, beside bool) {
	v := &s.pages.search
	tk := theme.Current()
	run, role := s.itemRun(it)
	key := "best:" + it.Kind + ":" + it.ID
	menus := s.itemMenus(&v.rowMenus, nil)

	panel := ui.Row(c.Key(key)).AlignItems(ui.Center).Gap(8).Padding(16).Radius(tk.Radius["lg"]).
		Background(theme.C("surface")).Border(1, theme.C("line"))
	if beside {
		panel.Grow(3).Basis(0).MinWidth(0)
	} else {
		panel.FillWidth()
	}
	shown := panel.Hovered() || panel.FocusWithin()
	if shown {
		panel.Background(theme.C("hover"))
	}
	panel.Children(func() {
		body := func() {
			kit.Artwork(c, s.env.Art, it.Art.Small, bestArt, kit.ArtOpts{Round: kit.ItemRound(it), Edge: true})
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Margin(0, 0, 0, 16).Gap(2).Children(func() {
				if kind := kindLabel(it); kind != "" {
					theme.Text(c, kind, "body").TextColor(theme.C("ink-muted")).SingleLine().Role(ui.RoleNone)
				}
				ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
					theme.Text(c, it.Name, "title").TextColor(theme.C("ink")).MaxLines(2).Shrink(1).MinWidth(0)
					if it.Explicit {
						kit.ExplicitMark(c).Shrink(0)
					}
				})
				if it.Subtitle != "" {
					theme.Text(c, it.Subtitle, "body").TextColor(theme.C("ink-muted")).SingleLine().Role(ui.RoleNone)
				}
			})
		}
		if run == nil {
			ui.Row(c.Key("body")).Grow(1).Shrink(1).MinWidth(0).AlignItems(ui.Center).Children(body)
		} else {
			main := ui.ButtonBase(c.Key("open")).Grow(1).Shrink(1).MinWidth(0).AlignItems(ui.Center).Radius(tk.Radius["md"]).Role(role)
			if role == ui.RoleButton {
				// Named by its visible text (WCAG 2.5.3), with the action after it.
				main.Label(it.Name + ", " + i18n.T("Play"))
			} else {
				main.Label(strings.TrimSpace(it.Name + " " + itemMeta(it)))
			}
			main.Children(func() {
				body()
				if role == ui.RoleButton {
					disc(c, 40)
				}
			})
			if focus {
				main.Focus()
			}
			if menus != nil && main.RightClicked() {
				if ai, ok := kit.ActionItemOf(it, s.env.Players); ok {
					x, y, _ := main.PointerPosition()
					wx, wy := main.LocalToWindow(x, y)
					menus.OpenAt(kit.ActionTarget{Item: ai, Name: it.Name, Key: key}, wx, wy, false)
				}
			}
			if main.Clicked() {
				if ctx.OnOpen != nil {
					ctx.OnOpen(it)
				}
				run()
			}
		}
		// A result that opens a page and also plays has its own disc.
		if role == ui.RoleLink && s.env.Players != nil && s.env.Players.CanPlay(it) {
			d := kit.PlayButton(c.Key("play"), i18n.T("Play {label}", "label", it.Name), 40)
			if d.Clicked() {
				if ctx.OnOpen != nil {
					ctx.OnOpen(it)
				}
				s.env.Players.PlayItem(it)
			}
		}
		if menus != nil {
			if ai, ok := kit.ActionItemOf(it, s.env.Players); ok {
				menus.Trigger(c, kit.ActionTarget{Item: ai, Name: it.Name, Key: key}, kit.TriggerOpts{Visible: shown})
			}
		}
	})
	if menus != nil {
		if ai, ok := kit.ActionItemOf(it, s.env.Players); ok {
			menus.Row(panel, kit.ActionTarget{Item: ai, Name: it.Name, Key: key})
		}
	}
}

// disc is the Play disc drawn into a control that plays as a whole: amber,
// with its triangle, not a control of its own.
func disc(c *ui.Context, size float32) {
	ui.Box(c.Key("disc")).Size(size, size).Shrink(0).Radius(theme.Current().Radius["full"]).Background(theme.C("play")).
		Center().Role(ui.RoleNone).Children(func() {
		theme.Glyph(c, "play", size*0.45, theme.GlyphOpts{Color: theme.C("on-play"), Filled: true}).Margin(0, 0, 0, 1)
	})
}

// itemBlock is an item group as a shelf of covers: round portraits for
// artists, denser covers for the rest. The first tile takes the field's
// focus request.
func (s *search) itemBlock(c *ui.Context, pv *pageView, st *kit.CoverShelfState, group, title string, items []services.Item, ctx kit.ItemsCtx, more string, first bool) {
	min := float32(coverMin)
	switch group {
	case "artist":
		min = portrait
	case "music_video":
		min = 0
	}
	s.groupBlock(c, pv, group, title, first, func() {
		kit.ItemShelf(c, st, title, items, ctx, kit.ItemShelfOpts{First: true, More: more, Min: min})
	})
}

// resultsSkeleton is the results column while they load, at the final
// layout's sizes: the best match and its rows, then a shelf of covers.
func (s *search) resultsSkeleton(c *ui.Context) {
	v := &s.pages.search
	tk := theme.Current()
	wide := v.inner <= 0 || v.inner >= bestSplit
	ui.Column(c.Key("loading")).FillWidth().Role(ui.RoleStatus).Label(i18n.T("Searching")).Children(func() {
		ui.Row(c.Key("heading")).Height(28).AlignItems(ui.Center).Margin(0, 0, 12, 0).Children(func() { kit.Skeleton(c, 96, 16) })
		topFlow(c, wide).Children(func() {
			panel := ui.Row(c.Key("best")).AlignItems(ui.Center).Gap(16).Padding(16).Radius(tk.Radius["lg"]).Border(1, theme.C("line"))
			if wide {
				panel.Grow(3).Basis(0).MinWidth(0)
			} else {
				panel.FillWidth()
			}
			panel.Children(func() {
				kit.Skeleton(c, bestArt, bestArt)
				ui.Column(c).Gap(8).Children(func() {
					kit.Skeleton(c, 56, 14)
					kit.Skeleton(c, 176, 24)
					kit.Skeleton(c, 112, 14)
				})
			})
			rows := ui.Column(c.Key("next")).Gap(2).Justify(ui.Center)
			if wide {
				rows.Grow(2).Basis(0).MinWidth(0)
			} else {
				rows.FillWidth()
			}
			rows.Children(func() {
				for i := range bestRows {
					ui.Row(c.Key(i)).FillWidth().Height(52).AlignItems(ui.Center).Gap(12).PaddingX(8).Children(func() {
						kit.Skeleton(c, 40, 40)
						ui.Column(c).Gap(6).Children(func() {
							kit.Skeleton(c, 128, 14)
							kit.Skeleton(c, 80, 12)
						})
					})
				}
			})
		})
		ui.Box(c.Key("rule")).FillWidth().Height(1).Background(theme.C("line")).Margin(32, 0, 24, 0).Role(ui.RoleNone)
		ui.Row(c.Key("heading2")).Height(28).AlignItems(ui.Center).Margin(0, 0, 12, 0).Children(func() { kit.Skeleton(c, 72, 16) })
		cols := v.columns(portrait, 24)
		tile := float32(portrait)
		if v.inner > 0 {
			tile = (v.inner - 24*float32(cols-1)) / float32(cols)
		}
		ui.Row(c.Key("covers")).Gap(24).Children(func() {
			for i := range cols {
				ui.Box(c.Key(i)).Size(tile, tile).Radius(tk.Radius["full"]).Background(theme.C("accent")).Shrink(0).Role(ui.RoleNone)
			}
		})
	})
}

// noResults is a search that found nothing, in the results column's own
// voice: the finding as the title, what to try, and the other scope as the
// one action.
func (s *search) noResults(c *ui.Context, title, body string, action *kit.EmptyAction) {
	ui.Column(c.Key("none")).FillWidth().AlignItems(ui.Start).Gap(8).Margin(16, 0, 0, 0).Children(func() {
		theme.Text(c, title, "title").TextColor(theme.C("ink")).Role(ui.RoleHeading).Level(2)
		theme.Text(c, body, "body").TextColor(theme.C("ink-muted")).MaxWidth(480)
		if action != nil && action.Run != nil {
			if kit.Button(c.Key("action"), action.Label, kit.ButtonOpts{Size: kit.SizeSM}).Margin(16, 0, 0, 0).Clicked() {
				action.Run()
			}
		}
	})
}
