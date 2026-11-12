package discover

import (
	"strconv"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// front is the front page's state (v0.2.0's FrontPage): which card leads,
// the page of the strip the user turned to, the
// user's pause and the rotation's timer.
type front struct {
	// openers are the lead's and the strip's play buttons, which a music
	// video's view returns the focus to when it closes.
	openers kit.Openers
	width   measure
	// at is the lead's place in the items.
	at int
	// turned: the user turned the strip to page turnPage while the lead was
	// turnAt; the strip shows the lead's page otherwise.
	turned           bool
	turnAt, turnPage int
	// paused is the user's pause (the control's state), not the temporary
	// holds.
	paused bool
	rot    rotation
	menus  *kit.ActionMenus
}

// choose makes card to the lead; leadMotion cross-fades the cards.
func (f *front) choose(to int) { f.at = to }

// pageState is what the pager under the lead shows: the strip's page of pages,
// and whether the rotation is on offer (a control for it).
type pageState struct {
	page, pages int
	available   bool
	label       string
}

// reserve is the room the lead's caption leaves at its end for the pager,
// which sits over it: the rotation control and, with more than one page, the
// count and the two buttons.
func (p pageState) reserve() float32 {
	var w float32
	if p.available {
		w += 36
	}
	if p.pages > 1 {
		w += 120
	}
	return w
}

// frontPage is Discover's front page, set as the head of an issue: Apple's
// featured cards, one of them the lead, as wide as the column (to leadMax) in
// the hero card's shape, its title and line in a caption under it with the
// pager at the caption's end, and all of them as a strip of cards under that,
// four a page. The lead moves on by itself unless the user pauses it or asks
// for reduced motion; choosing a card makes it the lead, and the strip shows
// the page the lead is on.
func (d *discover) frontPage(c *ui.Context, items []services.Item) {
	count := len(items)
	if count == 0 {
		return
	}
	f := &d.front
	ctx := d.env.ItemsCtx(f.menus)
	label := i18n.T("Featured")

	sec := ui.Column(c.Key("front")).FillWidth().Margin(0, 0, 48, 0).Role(ui.RoleGroup).Label(label)
	width := f.width.track(c, sec)
	lay := layoutFor(width)
	// A page that is built again after it went away starts over: the first
	// card leads, rotation is not paused (v0.2.0's component state was lost on
	// unmount). An element's state lives only while it is built.
	if mounted := ui.Local(sec, "mounted", func() bool { return false }); !*mounted {
		*mounted = true
		f.at, f.paused, f.turned, f.rot = 0, false, false, rotation{}
	}
	f.at = min(f.at, count-1)

	// The rotation: one deadline, asked for as a frame, no per-frame work.
	available := !theme.ReducedMotion() && count > 1
	engaged := sec.Hovered() || sec.FocusWithin()
	running := available && width > 0 && !f.paused && !engaged && d.seen(sec)
	advance, wait := f.rot.step(c.Now(), running, f.at, d.occupied)
	switch {
	case advance:
		f.choose((f.at + 1) % count)
		f.rot.restart(c.Now(), f.at)
		c.Invalidate()
	case wait > 0:
		c.After(wait)
	}
	at := f.at

	pages := max(1, (count+lay.perPage-1)/lay.perPage)
	// The strip shows the lead's page until the user turns it, and returns to
	// it when the lead changes.
	page := at / lay.perPage
	if f.turned && f.turnAt == at {
		page = f.turnPage
	}
	page = min(page, pages-1)
	shown := items[page*lay.perPage : min(count, (page+1)*lay.perPage)]
	pg := pageState{page: page, pages: pages, available: available, label: label}

	sec.Children(func() {
		if width <= 0 {
			return // the first frame measures; the next one lays it out
		}
		ui.Box(c.Key("lead")).Width(lay.lead).Shrink(0).Children(func() {
			// The cross-fade: the new lead comes in as the old one goes out.
			// MyGo leaves the old card as an inert copy, off the
			// accessibility tree, and takes it away when the fade ends.
			in := ui.Column(c.Key("in:" + leadKey(items[at], at))).Width(lay.lead)
			if !theme.ReducedMotion() {
				in.Transition(ui.ElementTransition{
					Duration: leadFade, Ease: ui.EaseInOut,
					Enter: &ui.Motion{X: leadSlide}, Exit: &ui.Motion{X: -leadSlide},
				})
			}
			in.Children(func() { d.leadCard(c, items[at], lay, ctx, pg) })
		})
		if count > 1 {
			d.strip(c, shown, page, lay, ctx, label)
		}
		f.menus.Popup(c)

		// The next lead's picture is fetched and decoded while this one is
		// read, so it fades in as a picture rather than as an empty box: the
		// loader is asked for it where it will show, and nothing is drawn.
		if available {
			next := items[(at+1)%count]
			if src, contain := leadArt(next); src != "" {
				kit.PrefetchArtwork(c, ctx.Art, src, lay.lead, kit.ArtOpts{Aspect: heroRatio, Contain: contain})
			}
		}
	})
}

// seen reports whether the user is looking at the hero: the window is shown
// (the shell tells the motion policy when it hides or minimises) and has the
// keyboard focus (Env.Window: another app or the mini player in front means
// nobody is reading the lead), and the hero is on screen. The shell redraws
// the window when the focus changes, so the rotation sets off again from a
// whole interval.
func (d *discover) seen(hero ui.Element) bool {
	if m := d.env.Motion; m != nil && !m.Visible() {
		return false
	}
	if !d.env.Window.Focused() {
		return false
	}
	return hero.InView() || hero.Bounds().W == 0 // not laid out yet: on screen
}

// occupied is what only the tick can say: a menu is open (any, the sidebar's
// and the account's too), or the page is
// covered (under Now Playing, lyrics or a modal dialog). The lead waits a
// whole interval more.
func (d *discover) occupied() bool {
	if d.front.menus.IsOpen() || d.newPg.menus.IsOpen() || kit.MenusOpen() || kit.ConfirmPending() {
		return true
	}
	return d.env.Motion.Covered()
}

// leadKey is a card's key: its own card, so a card that changes place keeps
// its state.
func leadKey(it services.Item, index int) string {
	return strconv.Itoa(index) + ":" + it.Kind + ":" + it.ID
}

// leadArt is the lead's picture: the wide banner cropped to the card, else the
// cover whole on the item's colour (contain).
func leadArt(it services.Item) (src string, contain bool) {
	banner := it.Wide.Large
	if banner == "" {
		banner = it.Wide.Medium
	}
	if banner != "" {
		return banner, false
	}
	src = it.Art.Large
	if src == "" {
		src = it.Art.Medium
	}
	return src, true
}

// fade shows or hides e over the theme's feedback duration, at once while
// motion is reduced.
func fade(e ui.Element, show bool) {
	to := float32(0)
	if show {
		to = 1
	}
	if theme.ReducedMotion() {
		e.Opacity(to)
		return
	}
	e.Opacity(e.Animate("fade", to, theme.Current().Motion["press"].Duration))
}

// opener is what opening a card does: go to its page when that route exists,
// or, for an item with no page, what that item does (a station plays, a web
// page opens in the browser). nil: the card leads nowhere (its page is not
// ported). link says it opens a page of Sona's.
func (d *discover) opener(it services.Item, ctx kit.ItemsCtx) (open func(), link bool) {
	path := kit.ItemLink(it)
	switch {
	case path == "":
		if !kit.PlaysWhenOpened(it, ctx.Players) && (it.URL == "" || ctx.OpenURL == nil) {
			return nil, false
		}
		return func() { kit.OpenWithoutPage(it, ctx) }, false
	case d.env.HasRoute != nil && d.env.HasRoute(path):
		return func() { d.env.Nav.Push(path) }, true
	}
	return nil, false
}

// leadCard is the lead: the picture (the wide banner cropped to the card, else
// the cover contained on its colour), under it the caption: the title opening
// the item's page and the subtitle, with the pager at its end. A play button
// shows on hover or focus and the item's menu sits beside the pager.
//
// The card is one opener; the play button, the more button and the pager are
// elements of their own over it (a button or a link is one piece for
// assistive technology, so inside it they would not be on its tree).
func (d *discover) leadCard(c *ui.Context, it services.Item, lay frontLayout, ctx kit.ItemsCtx, pg pageState) {
	tk := theme.Current()
	name := it.Name
	open, link := d.opener(it, ctx)
	target, hasTarget := kit.ActionItemOf(it, ctx.Players)
	menu := kit.ActionTarget{Item: target, Name: name, Key: "lead"}
	hasMenu := hasTarget && ctx.Menus != nil && d.hasMenu(target)
	artH := lay.artHeight()
	pager := pg.reserve()
	captionTop := artH + leadCaption - 48 // the caption's first line

	wrap := ui.Column(c.Key("wrap")).AlignItems(ui.Start).Width(lay.lead)
	hot := wrap.Hovered() || wrap.FocusWithin()
	wrap.Children(func() {
		var card ui.Element
		if open != nil {
			card = ui.ButtonBase(c.Key("card")).Column().AlignItems(ui.Start).Radius(tk.Radius["sm"]).Label(name)
			if link {
				card.Role(ui.RoleLink)
			}
			if sub := kit.ItemSubtitle(it); sub != "" {
				card.Description(sub) // inside the opener, the line is its description
			}
		} else {
			card = ui.Column(c.Key("card")).AlignItems(ui.Start).Label(name)
		}
		card.Width(lay.lead)
		card.Children(func() {
			colour, hasColour := itemColour(it)
			art := ui.Box(c.Key("art")).Size(lay.lead, artH).Radius(tk.Radius["sm"]).ClipX().ClipY().Background(theme.C("hover"))
			if hasColour {
				art.Background(colour)
			}
			art.Children(func() {
				if src, contain := leadArt(it); src != "" {
					// The item's colour shows where the picture does not, and
					// stays when it fails to load.
					kit.Artwork(c, ctx.Art, src, lay.lead, kit.ArtOpts{
						Aspect: heroRatio, Edge: true, Contain: contain, Fallback: func(*ui.Context) {},
					})
				}
			})
			ui.Row(c.Key("caption")).Width(lay.lead).Margin(12, 0, 0, 0).Children(func() {
				ui.Column(c.Key("text")).Grow(1).Basis(0).MinWidth(0).Children(func() {
					t := text(c, name, "title").TextColor(theme.C("ink")).SingleLine().TextAlign(ui.Start)
					if wrap.Hovered() && open != nil {
						t.Underline()
					}
					text(c, kit.ItemSubtitle(it), "secondary").TextColor(theme.C("ink-muted")).SingleLine().Height(18).TextAlign(ui.Start)
				})
				room := pager
				if hasMenu {
					room += moreReserve
				}
				ui.Box(c.Key("pager-room")).Size(room, 1).Shrink(0).IgnoreInput(true) // where the more button and the pager sit
			})
		})
		if ctx.Players.CanPlay(it) {
			play := kit.PlayButton(c.Key("play"), i18n.T("Play {name}", "name", name), 48).Absolute().Left(16).Top(artH - 16 - 48)
			fade(play, hot || play.Focused())
			opener := d.front.openers.For("lead:" + leadKey(it, 0))
			play.Bind(opener)
			if play.Clicked() {
				ctx.Players.PlayItemFrom(it, opener)
			}
		}
		if hasMenu {
			ctx.Menus.Trigger(c.Key("more"), menu, kit.TriggerOpts{Visible: hot}).Absolute().Right(pager).Top(captionTop + 12)
		}
		if pager > 0 {
			d.pager(c, pg, captionTop+10)
		}
		if card.Clicked() && open != nil {
			open()
		}
		if hasTarget && ctx.Menus != nil {
			ctx.Menus.Row(card, menu)
		}
	})
}

// hasMenu says whether the item has a more menu (a kind with actions, or one
// the bulk actions take): the more button is built, and its room kept, only
// then.
func (d *discover) hasMenu(it kit.Item) bool {
	return d.env.Actions.Has(it.Kind) || len(d.env.Actions.BulkFor([]kit.Item{it})) > 0
}

// itemColour is the item's own colour, "#rrggbb" from Apple, for the block
// behind its picture.
func itemColour(it services.Item) (ui.Color, bool) {
	h := it.Color
	if len(h) != 7 || h[0] != '#' {
		return ui.Color{}, false
	}
	var v [3]uint8
	for i := range v {
		n, err := strconv.ParseUint(h[1+2*i:3+2*i], 16, 8)
		if err != nil {
			return ui.Color{}, false
		}
		v[i] = uint8(n)
	}
	return ui.RGB(v[0], v[1], v[2]), true
}

// strip is the featured cards under the lead: one page of them, in a row of
// four or two rows of two. It reaches 8 past the lead's edges on each side, so
// that a card's picture and text, which sit 8 inside it, line up with the
// lead's.
func (d *discover) strip(c *ui.Context, shown []services.Item, page int, lay frontLayout, ctx kit.ItemsCtx, label string) {
	f := &d.front
	ui.Grid(c.Key("list")).Columns(lay.columns).GapX(stripGap).GapY(stripGap/2).Width(lay.lead+16).Margin(20, -8, 0, -8).
		Role(ui.RoleList).Label(label).Children(func() {
		for i := range shown {
			index := page*lay.perPage + i
			d.featureRow(c, shown[i], index, index == f.at, lay, ctx)
		}
	})
}

// pager is the control group at the lead caption's end, over the room the
// caption left for it: the rotation's pause control and, with more than one
// page of the strip, "N of M" and the buttons that turn it. The pager wraps
// round rather than disabling a button, so focus never drops from it to the
// page. top is where it sits in the lead's box.
func (d *discover) pager(c *ui.Context, pg pageState, top float32) {
	f := &d.front
	ui.Row(c.Key("pager")).Height(28).Shrink(0).Absolute().Right(0).Top(top).Gap(4).AlignItems(ui.Center).Children(func() {
		if pg.available {
			name, icon := i18n.T("Pause featured rotation"), "pause"
			if f.paused {
				name, icon = i18n.T("Play featured rotation"), "play"
			}
			b := kit.Button(c.Key("rotation"), "", kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM, Icon: icon, Filled: true, A11yLabel: name})
			b.Size(28, 28).Margin(0, 8, 0, 0)
			if b.Clicked() {
				if f.paused {
					c.Announce(i18n.T("Rotation playing"))
				} else {
					c.Announce(i18n.T("Rotation paused"))
				}
				f.paused = !f.paused
			}
		}
		if pg.pages <= 1 {
			return
		}
		text(c, i18n.T("{current} of {pages}", "current", pg.page+1, "pages", pg.pages), "meta").
			TextColor(theme.C("ink-muted")).FontFeatures("tnum").Margin(0, 4, 0, 0)
		turn := func(by int) {
			to := (pg.page + by + pg.pages) % pg.pages
			f.turned, f.turnAt, f.turnPage = true, f.at, to
			c.Announce(i18n.T("{current} of {pages}", "current", to+1, "pages", pg.pages))
		}
		prev := kit.Button(c.Key("previous"), "", kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM, Icon: "chevron-left", A11yLabel: i18n.T("Previous in {label}", "label", pg.label)})
		prev.Size(28, 28)
		if prev.Clicked() {
			turn(-1)
		}
		next := kit.Button(c.Key("next"), "", kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM, Icon: "chevron-right", A11yLabel: i18n.T("Next in {label}", "label", pg.label)})
		next.Size(28, 28)
		if next.Clicked() {
			turn(1)
		}
	})
}

// featureRow is a strip card: choosing it makes its card the lead (the lead's
// own title opens the card). current: the card is the lead now, marked by the
// active tone and not by amber (amber is what plays).
//
// The card is a list item holding the choosing button and, over it, the play
// button and the more button: controls inside a button are not on the tree
// assistive technology reads.
func (d *discover) featureRow(c *ui.Context, it services.Item, index int, current bool, lay frontLayout, ctx kit.ItemsCtx) {
	tk := theme.Current()
	f := &d.front
	name := it.Name
	item := ui.Row(c.Key(strconv.Itoa(index) + ":" + it.Kind + ":" + it.ID)).Width(lay.card).Height(lay.rowHeight).Shrink(0).Role(ui.RoleListItem)
	hovered := item.Hovered()
	hot := hovered || item.FocusWithin()
	target, hasTarget := kit.ActionItemOf(it, ctx.Players)
	menu := kit.ActionTarget{Item: target, Name: name, Key: "row:" + strconv.Itoa(index)}
	hasMenu := hasTarget && ctx.Menus != nil && d.hasMenu(target)
	var row ui.Element
	item.Children(func() {
		sub := kit.ItemSubtitle(it)
		row = ui.ButtonBase(c.Key("row")).Grow(1).MinWidth(0).Height(lay.rowHeight).PaddingX(8).Gap(12).
			AlignItems(ui.Center).Radius(tk.Radius["md"]).Label(name)
		if sub != "" {
			row.Description(sub)
		}
		switch {
		case current:
			row.Background(theme.C("active")).Current(ui.CurrentItem)
		case hovered:
			row.Background(theme.C("hover"))
		}
		if !theme.ReducedMotion() {
			m := tk.Motion["press"]
			row.Transition(ui.ElementTransition{Colors: true, Duration: m.Duration, Ease: m.Ease})
		}
		row.Children(func() {
			src := it.Art.Medium
			if src == "" {
				src = it.Art.Large
			}
			if src == "" {
				src = it.Wide.Medium
			}
			// The slot keeps the column and the play button where they are for
			// every card; the picture sits in it at its own shape.
			tw, th := thumbBox(src)
			ui.Row(c.Key("slot")).Size(listThumb, listThumb).Shrink(0).Center().Children(func() {
				thumb := ui.Box(c.Key("thumb")).Size(tw, th).Radius(tk.Radius["sm"]).ClipX().ClipY().Background(theme.C("hover"))
				if colour, ok := itemColour(it); ok {
					thumb.Background(colour)
				}
				thumb.Children(func() {
					kit.Artwork(c, ctx.Art, src, tw, kit.ArtOpts{Aspect: tw / th, Edge: true, Fallback: func(*ui.Context) {}})
				})
			})
			ui.Column(c.Key("text")).Grow(1).Basis(0).MinWidth(0).Children(func() {
				label := tk.Type["label"]
				ui.Text(c, name).FontSize(label.Size).FixedLineHeight(label.LineHeight).FontWeight(int(label.Weight)).
					TextColor(theme.C("ink")).SingleLine().TextAlign(ui.Start)
				if sub != "" {
					text(c, sub, "secondary").TextColor(theme.C("ink-muted")).SingleLine().TextAlign(ui.Start)
				}
			})
			if hasMenu {
				ui.Box(c.Key("more-room")).Size(24, 24).Shrink(0).IgnoreInput(true) // where the more button sits
			}
		})
		if ctx.Players.CanPlay(it) {
			play := kit.PlayButton(c.Key("play"), i18n.T("Play {name}", "name", name), 32).Absolute().
				Left(8 + (listThumb-32)/2).Top((lay.rowHeight - 32) / 2)
			fade(play, hot || play.Focused())
			opener := d.front.openers.For("row:" + leadKey(it, index))
			play.Bind(opener)
			if play.Clicked() {
				ctx.Players.PlayItemFrom(it, opener)
			}
		}
		if hasMenu {
			ctx.Menus.Trigger(c.Key("more"), menu, kit.TriggerOpts{Visible: hot}).Absolute().Right(8).Top((lay.rowHeight - 24) / 2)
		}
	})
	if row.Clicked() {
		f.choose(index)
	}
	if hasTarget && ctx.Menus != nil {
		ctx.Menus.Row(row, menu)
	}
}

// frontSkeleton is the front page's frame while it loads: the lead, its
// caption and the strip's cards as flat bars at their real sizes.
func (d *discover) frontSkeleton(c *ui.Context, m *measure) {
	tk := theme.Current()
	root := ui.Column(c.Key("front-skeleton")).FillWidth().Margin(0, 0, 48, 0).Role(ui.RoleNone)
	width := m.track(c, root)
	lay := layoutFor(width)
	root.Children(func() {
		if width <= 0 {
			return
		}
		ui.Column(c.Key("lead")).Width(lay.lead).Shrink(0).Children(func() {
			ui.Box(c).Size(lay.lead, lay.artHeight()).Radius(tk.Radius["sm"]).Background(theme.C("hover"))
			ui.Row(c).Height(30).Margin(12, 0, 0, 0).AlignItems(ui.Center).Children(func() { bar(c, lay.lead/3, 20) })
			ui.Row(c).Height(18).AlignItems(ui.Center).Children(func() { bar(c, lay.lead/4, 12) })
		})
		ui.Column(c.Key("list")).Width(lay.lead+16).Margin(20, -8, 0, -8).Gap(stripGap / 2).Children(func() {
			for r := range lay.rows {
				ui.Row(c.Key(r)).Gap(stripGap).Children(func() {
					for col := range lay.columns {
						ui.Row(c.Key(col)).Width(lay.card).Height(lay.rowHeight).PaddingX(8).Gap(12).AlignItems(ui.Center).Children(func() {
							bar(c, listThumb, listThumb)
							ui.Column(c).Grow(1).MinWidth(0).Children(func() {
								ui.Box(c).WidthPercent(75).Height(14).Radius(tk.Radius["sm"]).Background(theme.C("hover"))
								ui.Box(c).WidthPercent(50).Height(12).Margin(8, 0, 0, 0).Radius(tk.Radius["sm"]).Background(theme.C("hover"))
							})
						})
					}
				})
			}
		})
	})
}
