package home

import (
	"regexp"
	"strconv"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

const (
	// sideBySide is the width from which the lead and the other picks sit
	// side by side; below it they stack.
	sideBySide = 900
	// slateRows is the rows of the running order shown before "Show all".
	slateRows = 8
	// leadCover and leadCoverNarrow are the lead pick's cover, in DIPs: the
	// width of its column beside the running order, or the cover of its row
	// above it.
	leadCover       = 280
	leadCoverNarrow = 144
	// leadGap is the room between the lead and the running order.
	leadGap = 48
)

// leadState is the state of the lead: its width, the picks' rows and their
// toggle, and the lead pick's selection and menu.
type leadState struct {
	width kit.WidthTracker
	cols  int
	open  bool
	rows  rowsState
	pick  pickState
	skel  skeletonState
	cap   captioned
}

// backdropSize is the "<w>x<h>.jpg" a Go-served art path ends in.
var backdropSize = regexp.MustCompile(`/\d+(?:x\d+)?\.jpg($|\?)`)

// backdropURL is the pre-blurred backdrop of a cover: its art path with the
// size part replaced by "backdrop.jpg" ("" for no art).
func backdropURL(art string) string {
	if art == "" {
		return ""
	}
	return backdropSize.ReplaceAllString(art, "/backdrop.jpg$1")
}

// leadBand is the top of Home: the page header and the lead, on a wash of the
// lead's cover (Go's pre-blurred backdrop under the ground scrim), full bleed
// like a collection header. Without a lead it is the plain header.
func (h *home) leadBand(c *ui.Context, lead *services.Item, children func()) {
	if lead == nil {
		ui.Column(c.Key("band")).Children(children)
		return
	}
	tk := theme.Current()
	gutter, bar := h.page.Gutter(), tk.Metric["scrollbar"]
	art := lead.Art.Medium
	if art == "" {
		art = lead.Art.Large
	}
	// The band is the backdrop: what it holds draws over the wash.
	kit.Backdrop(c.Key("band"), h.env.Art, kit.BackdropOpts{Src: backdropURL(art), Color: lead.Color, Tone: kit.ToneGround}, children).
		AlignSelf(ui.Stretch).Shrink(0).Padding(kit.PagePadTop, gutter+bar, 24, gutter).Margin(-kit.PagePadTop, -(gutter + bar), 0, -gutter)
}

// homeLead is the first "feature" shelf (Top Picks) set as the A-side of a
// record: its first pick is 01, large, with what it is, Apple's reason and its
// play control; every other pick follows as a numbered running order (02, 03
// …) in one list beside it, so the first screen answers "what shall I play?"
// without scrolling. The numbers are Apple's order. Under 900 DIPs the lead is
// a row (cover, name) over the full-width order.
func (h *home) homeLead(c *ui.Context, sec services.Section, ctx kit.ItemsCtx) {
	st := &h.lead
	items := st.cap.of(sec.Items)
	first, rest := items[0], items[1:]
	title := sec.Title
	if title == "" {
		title = i18n.T("Top picks")
	}
	root := ui.Column(c.Key("lead")).Role(ui.RoleGroup).Label(title)
	wide := st.width.Track(c, root) >= sideBySide
	root.Children(func() {
		hidden := st.cols > 0 && len(rest) > st.cols*slateRows
		head := ruledOpts{Title: title}
		if hidden || st.open {
			head.Actions = func() { showAll(c, len(sec.Items), &st.open) }
		}
		ruledHead(c, head)
		var body ui.Element
		if wide {
			body = ui.Row(c.Key("body")).Gap(leadGap).AlignItems(ui.Start)
		} else {
			body = ui.Column(c.Key("body")).Gap(20)
		}
		body.Margin(12, 0, 0, 0).Children(func() {
			h.leadPick(c, first, wide, ctx)
			if len(rest) > 0 {
				order := ui.Column(c.Key("order")).MinWidth(0)
				if wide {
					order.Grow(1).Basis(0)
				}
				order.Children(func() {
					h.itemRows(c, &st.rows, rest, ctx, rowsOpts{
						Label: title, Rows: slateRows, Cols: 1, Min: rowMin, Open: st.open, OnBackdrop: true,
						Number: 2, Rules: true,
						OnColumns: func(n int) { st.cols = n; c.Invalidate() },
					})
				})
			}
		})
	})
}

// pickState is the lead pick's selection and menu.
type pickState struct {
	sel   kit.Selection
	menus *kit.ActionMenus
}

// leadPick is the lead pick: its cover, name, artist or curator, the catalogue
// line and its controls. It selects, opens its menu on a right-click or
// Shift+F10 and drags like any tile.
func (h *home) leadPick(c *ui.Context, it services.Item, wide bool, ctx kit.ItemsCtx) {
	st := &h.lead.pick
	cover := float32(leadCoverNarrow)
	if wide {
		cover = leadCover
	}
	menus := kit.MenusIn(&st.menus, h.env.Actions, &st.sel)
	tk := theme.Current()
	target, selectable := kit.ActionItemOf(it, ctx.Players)
	key := ""
	if selectable {
		key = "lead:" + it.Kind + ":" + it.ID
	}
	st.sel.Hold = func() bool { return menus.IsOpen() || kit.ConfirmPending() }
	st.sel.Rows(kit.RowKeys{key}, func(int) (kit.Item, bool) { return target, selectable })
	selected := selectable && st.sel.Selected(key)

	link := kit.ItemLink(it)
	withheld := ctx.Restricted && it.Explicit
	playable := ctx.Players.CanPlay(it) && !withheld
	art := it.Art.Large
	if art == "" {
		art = it.Art.Medium
	}
	at := kit.ActionTarget{Item: target, Name: it.Name, Key: key}
	var opener ui.Element

	dragKey := key
	if dragKey == "" {
		dragKey = "lead"
	}
	w, src := kit.DragItem(c, kit.DragOpts{
		Key: dragKey, Scope: st, Art: h.env.Art, Row: !wide,
		Payload: func() *kit.DragPayload {
			if !selectable || withheld {
				return nil
			}
			return &kit.DragPayload{Items: []kit.Item{target}, Label: it.Name, Art: it.Art.Small}
		},
		Outer: func(w ui.Element) {
			w.MinWidth(0).Role(ui.RoleNone)
			if wide {
				w.Width(cover).Shrink(0)
			}
			if selected {
				plate := theme.C("ink").Alpha(0.12)
				w.Draw(func(p *ui.Painter, r ui.Rect) {
					p.Fill(ui.Rect{X: r.X - 8, Y: r.Y - 8, W: r.W + 16, H: r.H + 16}, plate, tk.Radius["lg"])
				})
			}
			if withheld {
				w.Opacity(0.5).Tooltip(i18n.T("Explicit content is restricted on this account"))
			}
		},
		Source: func(s ui.Element) {
			if wide {
				s.AlignItems(ui.Start).Gap(16)
			} else {
				s.AlignItems(ui.End).Gap(20)
			}
			s.Radius(tk.Radius["sm"]).Role(ui.RoleNone)
		},
	}, func(w, s ui.Element) {
		kit.Artwork(c, h.env.Art, art, cover, kit.ArtOpts{Edge: true, Round: kit.ItemRound(it)})
		ui.Row(c.Key("words")).Grow(1).Shrink(1).MinWidth(0).AlignItems(ui.Start).Gap(8).Padding(0, 0, 4, 0).Children(func() {
			// The lead is 01 of the running order beside it.
			theme.Text(c, "01", "page").TextColor(theme.C("ink-faint")).FontFeatures("tnum").SingleLine().Width(32).Shrink(0)
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Children(func() {
				opener = h.titleBlock(c, it, "page", link)
				ui.Row(c.Key("controls")).Margin(20, 0, 0, 0).Gap(8).AlignItems(ui.Center).Children(func() {
					if play, _ := kit.PlayButtons(c, kit.PlayButtonsOpts{Label: it.Name, Disabled: !playable}); play {
						ctx.Players.PlayItem(it)
					}
					if selectable {
						menus.Trigger(c.Key("more"), at, kit.TriggerOpts{Header: true})
					}
				})
			})
		})
	})

	open := func() { h.open(it, ctx) }
	for _, e := range [...]ui.Element{src, opener} {
		if !e.Clicked() {
			continue
		}
		if mods := kit.ModsOf(e.ClickModifiers()); (mods.Range || mods.Toggle) && selectable {
			st.sel.Click(0, mods)
		} else {
			open()
		}
	}
	if opener.Focused() {
		st.sel.Keys(opener, 0)
	}
	st.sel.OutsidePress(w)
	if selectable {
		menus.OpenFor(&st.sel, src, opener, at, 0)
		menus.Popup(c)
	}
}

// titleBlock is the head of a featured item's text column: its name (a link
// when its page is served, plain text when it is not) in the type style, the
// subtitle and the catalogue line. It returns the name's button, the zero
// Element when the name is plain text.
func (h *home) titleBlock(c *ui.Context, it services.Item, style, link string) (opener ui.Element) {
	tk := theme.Current()
	if link != "" && !h.opens(link) {
		// Its page is not served: the name is plain text.
		theme.Text(c, it.Name, style).TextColor(theme.C("ink")).MaxLines(2).TextAlign(ui.Start)
	} else {
		opener = ui.ButtonBase(c.Key("open")).Justify(ui.Start).Radius(tk.Radius["sm"]).Label(it.Name)
		if link != "" {
			opener.Role(ui.RoleLink)
		}
		opener.Children(func() {
			t := theme.Text(c, it.Name, style).TextColor(theme.C("ink")).MaxLines(2).TextAlign(ui.Start)
			if opener.Hovered() {
				t.Underline()
			}
		})
	}
	if it.Subtitle != "" {
		theme.Text(c, it.Subtitle, "heading").FontWeight(500).TextColor(theme.C("ink")).SingleLine().Margin(6, 0, 0, 0)
	}
	if meta := catalogueLine(it); meta != "" {
		theme.Text(c, meta, "meta").TextColor(theme.C("ink-muted")).SingleLine().Margin(4, 0, 0, 0)
	}
	return opener
}

// signInLead is shown signed out: where the lead will be, what it needs and
// the one action.
func (h *home) signInLead(c *ui.Context) {
	h.leadNotice(c, "", i18n.T("Recommendations need Apple Music"), func() {
		theme.Text(c, i18n.T("Sign in to see picks, mixes and stations chosen for you."), "body").
			TextColor(theme.C("ink-muted")).MaxWidth(60*9).Margin(4, 0, 0, 0)
	}, func() {
		if kit.Button(c.Key("signin"), i18n.T("Sign in"), kit.ButtonOpts{Size: kit.SizeSM}).Clicked() && h.env.Surfaces != nil {
			h.env.Surfaces.OpenAccount()
		}
	})
}

// offlineLead is shown offline with no copy of Home yet: nothing to retry,
// Sona fetches it when it reconnects.
func (h *home) offlineLead(c *ui.Context) {
	h.leadNotice(c, "wifi-off", i18n.T("No recommendations while offline"), func() {
		theme.Text(c, i18n.T("Sona fetches them as soon as it reconnects."), "body").
			TextColor(theme.C("ink-muted")).MaxWidth(60*9).Margin(4, 0, 0, 0)
	}, nil)
}

// leadNotice holds the lead's place with a ledger band: a rule above and
// below, the title and what it says on the left, the one action at the right
// end. It is the lead's size, not a card.
func (h *home) leadNotice(c *ui.Context, icon, title string, body, action func()) {
	line := hairline()
	ui.Column(c.Key("notice")).FillWidth().Role(ui.RoleGroup).Label(title).Children(func() {
		ui.Box(c.Key("top")).FillWidth().Height(1).Background(line)
		ui.Row(c.Key("band")).FillWidth().Padding(32, 0).Gap(24).AlignItems(ui.Center).Children(func() {
			ui.Column(c.Key("words")).Grow(1).Basis(0).MinWidth(0).Children(func() {
				ui.Row(c.Key("title")).Gap(8).AlignItems(ui.Center).Children(func() {
					if icon != "" {
						theme.Glyph(c, icon, 20, theme.GlyphOpts{Color: theme.C("ink-muted")})
					}
					theme.Text(c, title, "page").TextColor(theme.C("ink")).Role(ui.RoleHeading).Level(2)
				})
				body()
			})
			if action != nil {
				ui.Row(c.Key("action")).Shrink(0).Children(action)
			}
		})
		ui.Box(c.Key("bottom")).FillWidth().Height(1).Background(line)
	})
}

// skeletonState is the loading lead's widths.
type skeletonState struct {
	width kit.WidthTracker
}

// leadSkeleton is the lead's shape while Home loads: the ruled head, the 01
// cover with the name's lines and the play circle, and the running order's
// eight rows with their numbers.
func (h *home) leadSkeleton(c *ui.Context) {
	st := &h.lead.skel
	tk := theme.Current()
	hover, r := theme.C("hover"), tk.Radius["sm"]
	root := ui.Column(c.Key("lead-skeleton")).Role(ui.RoleStatus).Label(i18n.T("Loading"))
	wide := st.width.Track(c, root) >= sideBySide
	root.Children(func() {
		ui.Row(c.Key("head")).MinHeight(28).Margin(0, 0, 12, 0).Gap(16).AlignItems(ui.Center).Children(func() {
			kit.Bar(c, 140, 16)
			rule(c)
		})
		var body ui.Element
		cover := float32(leadCoverNarrow)
		if wide {
			cover = leadCover
			body = ui.Row(c.Key("body")).Gap(leadGap).AlignItems(ui.Start)
		} else {
			body = ui.Column(c.Key("body")).Gap(20)
		}
		body.Margin(12, 0, 0, 0).Children(func() {
			words := func() {
				ui.Row(c.Key("words")).Gap(8).AlignItems(ui.Start).Grow(1).Children(func() {
					kit.Bar(c, 24, 24)
					ui.Column(c).Grow(1).Children(func() {
						ui.Box(c).WidthPercent(75).Height(24).Radius(r).Background(hover)
						ui.Box(c).WidthPercent(50).Height(16).Radius(r).Background(hover).Margin(10, 0, 0, 0)
						ui.Box(c).Size(40, 40).Radius(tk.Radius["full"]).Background(hover).Margin(20, 0, 0, 0)
					})
				})
			}
			if wide {
				ui.Column(c.Key("pick")).Width(cover).Shrink(0).Gap(16).Children(func() {
					kit.Bar(c, cover, cover)
					words()
				})
			} else {
				ui.Row(c.Key("pick")).Gap(20).AlignItems(ui.End).Children(func() {
					kit.Bar(c, cover, cover)
					words()
				})
			}
			order := ui.Column(c.Key("order")).MinWidth(0).Margin(0, -rowPadX, 0, -rowPadX)
			if wide {
				order.Grow(1).Basis(0)
			}
			order.Children(func() {
				for i := range slateRows {
					ui.Row(c.Key("row" + strconv.Itoa(i))).Height(rowH).PaddingX(rowPadX).Gap(rowGap).AlignItems(ui.Center).Children(func() {
						kit.Bar(c, rowNum-rowGap, 12)
						kit.Bar(c, rowArt, rowArt)
						ui.Column(c).Grow(1).Children(func() {
							ui.Box(c).WidthPercent(float32(40 + (i%3)*10)).Height(14).Radius(r).Background(hover)
							ui.Box(c).WidthPercent(30).Height(12).Radius(r).Background(hover).Margin(6, 0, 0, 0)
						})
					})
				}
			})
		})
	})
}
