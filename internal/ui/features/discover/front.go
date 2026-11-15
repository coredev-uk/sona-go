package discover

import (
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// front is the front page's state (v0.2.0's FrontPage): which card leads,
// the user's pause and the rotation's timer.
type front struct {
	// openers are the lead's and the queue's play buttons, which a music
	// video's view returns the focus to when it closes.
	openers kit.Openers
	width   kit.WidthTracker
	// at is the lead's place in the items.
	at int
	// paused is the user's pause (the control's state), not the temporary
	// holds.
	paused bool
	rot    rotation
	menus  *kit.ActionMenus
	// keyboard: a control of the hero has the focus from the keyboard (what
	// the last build saw; each build gathers it again with seeKeyboard).
	keyboard bool
	// focusLead: the keyboard showed a queued card in the lead, whose tile
	// is gone with it; the lead takes the focus on the next build.
	focusLead bool
}

// seeKeyboard notes whether control e has the keyboard focus, with its ring
// showing, and returns it. A click focuses the control it lands on too, and
// that focus stays where it is until the user goes somewhere else, so only
// the keyboard's counts as someone at the hero's controls.
func (f *front) seeKeyboard(e ui.Element) ui.Element {
	if e.FocusVisible() {
		f.keyboard = true
	}
	return e
}

// choose makes card to the lead; leadMotion cross-fades the cards.
func (f *front) choose(to int) { f.at = to }

// pageState is what the pager at the end of the lead's caption shows: the
// lead's place among the cards, and whether the rotation is on offer (a
// control for it).
type pageState struct {
	at, count int
	available bool
	label     string
}

// reserve is the room the lead's caption leaves at its end for the pager,
// which sits over it: the rotation control and, with more than one card,
// the count and the two buttons.
func (p pageState) reserve() float32 {
	var w float32
	if p.available {
		w += 36
	}
	if p.count > 1 {
		w += 120
	}
	return w
}

// frontPage is Discover's front page, set as the head of an issue: Apple's
// featured cards, one of them the lead in the hero card's shape, its title
// and line in a caption under it with the pager at the caption's end, and
// the next ones in the rotation as a queue of tiles beside it (under it in
// a narrow window). The lead moves on by itself unless the user pauses it
// or asks for reduced motion, and a row of segments under its picture, one
// a card, shows where it is and how long until the next.
func (d *discover) frontPage(c *ui.Context, items []services.Item, base kit.ItemsCtx) {
	count := len(items)
	if count == 0 {
		return
	}
	f := &d.front
	ctx := base // the page's own, with the lead's menu
	ctx.Menus, ctx.NoPopup = f.menus, false
	label := i18n.T("Featured")

	sec := ui.Column(c.Key("front")).FillWidth().Margin(0, 0, 48, 0).Role(ui.RoleGroup).Label(label)
	width := f.width.Track(c, sec)
	lay := layoutFor(width, d.newPg.pg.Height)
	// A page that is built again after it went away starts over: the first
	// card leads, rotation is not paused (v0.2.0's component state was lost on
	// unmount). An element's state lives only while it is built.
	if mounted := ui.Local(sec, "mounted", func() bool { return false }); !*mounted {
		*mounted = true
		f.at, f.paused, f.rot, f.focusLead = 0, false, rotation{}, false
	}
	f.at = min(f.at, count-1)

	// The rotation: one deadline, asked for as a frame; only the progress
	// moves in between, in frames that paint it without building the view.
	available := !theme.ReducedMotion() && count > 1
	// Hovering holds, and so does the focus the keyboard put in the hero. The
	// focus a click left there (the pause control, the pager) does not: the
	// pointer has gone, and nothing would ever end the hold.
	keyboard := f.keyboard
	f.keyboard = false
	engaged := sec.Hovered() || (sec.FocusWithin() && keyboard)
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
	pg := pageState{at: at, count: count, available: available, label: label}

	defer func() {
		if f.keyboard != keyboard {
			c.Invalidate() // the rotation reads it on the next build
		}
	}()
	sec.Children(func() {
		if width <= 0 {
			return // the first frame measures; the next one lays it out
		}
		lead := func() {
			ui.Box(c.Key("lead")).Width(lay.lead).Shrink(0).Children(func() {
				// The cross-fade: the new lead comes in as the old one goes
				// out. MyGo leaves the old card as an inert copy, off the
				// accessibility tree, and takes it away when the fade ends.
				in := ui.Column(c.Key("in:" + leadKey(items[at], at))).Width(lay.lead)
				if !theme.ReducedMotion() {
					in.Transition(ui.ElementTransition{
						Duration: leadFade, Ease: ui.EaseInOut,
						Enter: &ui.Motion{X: leadSlide}, Exit: &ui.Motion{X: -leadSlide},
					})
				}
				in.Children(func() { d.leadCard(c, items[at], lay, ctx, pg) })
				// The pager stays while the lead changes under it, so the
				// chevron the keyboard pressed keeps the focus.
				if pg.reserve() > 0 {
					d.pager(c, pg, lay.art+progressGap+progressHeight+leadCaptionGap+10)
				}
			})
		}
		if lay.beside {
			ui.Row(c.Key("spread")).Gap(queueGap).AlignItems(ui.Start).Children(func() {
				lead()
				d.queue(c, items, lay, ctx)
			})
		} else {
			lead()
			d.queue(c, items, lay, ctx)
		}
		f.menus.Popup(c)

		// The next lead's picture is fetched and decoded while this one is
		// read, so it fades in as a picture rather than as an empty box: the
		// loader is asked for it where it will show, and nothing is drawn.
		if available {
			next := items[(at+1)%count]
			if src, contain := leadArt(next); src != "" {
				kit.PrefetchArtwork(c, ctx.Art, src, lay.lead, kit.ArtOpts{Aspect: lay.artAspect(), Contain: contain})
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
	case d.env.Routed(path):
		return func() { d.env.Nav.Push(path) }, true
	}
	return nil, false
}

// leadCard is the lead: the picture (the wide banner cropped to the card, else
// the cover contained on its colour), under it the rotation's progress, then
// the caption: the title opening the item's page, Apple's line and the
// byline, with the pager at its end. A play button shows on hover or focus
// and the item's menu sits beside the pager.
//
// The card is one opener; the play button, the more button and the pager are
// elements of their own over it (a button or a link is one piece for
// assistive technology, so inside it they would not be on its tree).
func (d *discover) leadCard(c *ui.Context, it services.Item, lay frontLayout, ctx kit.ItemsCtx, pg pageState) {
	tk := theme.Current()
	f := &d.front
	name := it.Name
	open, link := d.opener(it, ctx)
	target, hasTarget := kit.ActionItemOf(it, ctx.Players)
	menu := kit.ActionTarget{Item: target, Name: name, Key: "lead"}
	hasMenu := hasTarget && ctx.Menus.HasMenu(target)
	artH := lay.art
	pager := pg.reserve()
	captionTop := artH + progressGap + progressHeight + leadCaptionGap // the caption's first line

	wrap := ui.Column(c.Key("wrap")).AlignItems(ui.Start).Width(lay.lead)
	hot := wrap.Hovered() || wrap.FocusWithin()
	wrap.Children(func() {
		var card ui.Element
		if open != nil {
			card = ui.ButtonBase(c.Key("card")).Column().AlignItems(ui.Start).Radius(tk.Radius["sm"]).Label(name)
			if link {
				card.Role(ui.RoleLink)
			}
			// Inside the opener, Apple's line and the byline are its description.
			if desc := leadDescription(it); desc != "" {
				card.Description(desc)
			}
		} else {
			card = ui.Column(c.Key("card")).AlignItems(ui.Start).Label(name)
		}
		card.Width(lay.lead)
		if f.focusLead {
			f.focusLead = false
			card.Focus()
		}
		f.seeKeyboard(card)
		card.Children(func() {
			colour, hasColour := kit.ParseHex(it.Color)
			art := ui.Box(c.Key("art")).Size(lay.lead, artH).Radius(tk.Radius["sm"]).ClipX().ClipY().Background(theme.C("hover"))
			if hasColour {
				art.Background(colour)
			}
			art.Children(func() {
				if src, contain := leadArt(it); src != "" {
					// The item's colour shows where the picture does not, and
					// stays when it fails to load.
					kit.Artwork(c, ctx.Art, src, lay.lead, kit.ArtOpts{
						Aspect: lay.artAspect(), Edge: true, Contain: contain, Fallback: func(*ui.Context) {},
					})
				}
			})
			if pg.count > 1 {
				d.progress(c, pg, lay.lead)
			}
			// Headline, Apple's one line about the card, then the byline
			// (badge · artist or curator). The line's row is kept when a card
			// has none, so the queue beside it does not jump as the lead
			// rotates.
			gap := float32(leadCaptionGap)
			if pg.count <= 1 {
				gap += progressGap + progressHeight // where the progress would be
			}
			ui.Row(c.Key("caption")).Width(lay.lead).Margin(gap, 0, 0, 0).Children(func() {
				ui.Column(c.Key("text")).Grow(1).Basis(0).MinWidth(0).Children(func() {
					t := theme.Text(c, name, "title").TextColor(theme.C("ink")).SingleLine().TextAlign(ui.Start)
					if wrap.Hovered() && open != nil {
						t.Underline()
					}
					line := theme.Text(c, it.Description, "body").TextColor(theme.C("ink")).SingleLine().Height(leadLine).Margin(2, 0, 0, 0).TextAlign(ui.Start)
					if it.Description != "" {
						line.Tooltip(it.Description) // clipped: the whole line
					}
					theme.Text(c, kit.ItemSubtitle(it), "secondary").TextColor(theme.C("ink-muted")).SingleLine().Height(18).TextAlign(ui.Start)
				})
				room := pager
				if hasMenu {
					room += moreReserve
				}
				ui.Box(c.Key("pager-room")).Size(room, 1).Shrink(0).IgnoreInput(true) // where the more button and the pager sit
			})
		})
		if ctx.Players.CanPlay(it) {
			play := f.seeKeyboard(kit.PlayButton(c.Key("play"), i18n.T("Play {name}", "name", name), 48)).Absolute().Left(16).Top(artH - 16 - 48)
			kit.Fade(play, hot || play.Focused())
			opener := f.openers.For("lead:" + leadKey(it, 0))
			play.Bind(opener)
			if play.Clicked() {
				ctx.Players.PlayItemFrom(it, opener)
			}
		}
		if hasMenu {
			f.seeKeyboard(ctx.Menus.Trigger(c.Key("more"), card, menu, kit.TriggerOpts{Visible: hot})).Absolute().Right(pager).Top(captionTop + 12)
		}
		if card.Clicked() && open != nil {
			open()
		}
	})
}

// leadDescription is what the lead's opener is described by for assistive
// technology: Apple's line about the card, then its byline.
func leadDescription(it services.Item) string {
	var parts []string
	for _, s := range []string{it.Description, kit.ItemSubtitle(it)} {
		if s = strings.TrimSpace(s); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, ". ")
}

// progress is the rotation's progress under the lead's picture: a segment a
// card, the ones before the lead filled, the ones after it a track. The
// lead's own fills as its interval runs, in frames that only paint it (no
// build, no layout); while the rotation holds or is paused it is a muted
// mark, filled whole, of where the lead is, and so is it under reduced
// motion, where nothing rotates. It is drawn, not read: the pager's count
// says the same to assistive technology.
func (d *discover) progress(c *ui.Context, pg pageState, w float32) {
	rot := &d.front.rot
	track, mark, ink := theme.C("line"), theme.C("ink-muted"), theme.C("ink")
	count, at := pg.count, pg.at
	ui.Box(c.Key("progress")).Size(w, progressHeight).Margin(progressGap, 0, 0, 0).IgnoreInput(true).
		Draw(func(p *ui.Painter, r ui.Rect) {
			seg := (r.W - float32(count-1)*segmentGap) / float32(count)
			radius := r.H / 2
			for i := range count {
				box := ui.Rect{X: r.X + float32(i)*(seg+segmentGap), Y: r.Y, W: seg, H: r.H}
				switch {
				case i < at:
					p.Fill(box, ink, radius)
				case i > at:
					p.Fill(box, track, radius)
				case !rot.armed:
					p.Fill(box, mark, radius)
				default:
					p.Fill(box, track, radius)
					if done := rot.progress(p.Now()); done > 0 {
						box.W = max(seg*done, r.H)
						p.Fill(box, ink, radius)
					}
					// Thirty frames a second moves the fill by under a pixel
					// in any lead, at a quarter of a 120 Hz display's work.
					p.AnimationFrameRate(30)
				}
			}
		})
}

// pager is the control group at the lead caption's end, over the room the
// caption left for it: the rotation's pause control and, with more than one
// card, the buttons that bring the one before or after into the lead, with
// "N of M" between them. They wrap round rather than disabling a button, so
// focus never drops from them to the page. top is where it sits in the
// lead's box.
func (d *discover) pager(c *ui.Context, pg pageState, top float32) {
	f := &d.front
	ui.Row(c.Key("pager")).Height(28).Shrink(0).Absolute().Right(0).Top(top).Gap(4).AlignItems(ui.Center).Children(func() {
		if pg.available {
			name, icon := i18n.T("Pause featured rotation"), "pause"
			if f.paused {
				name, icon = i18n.T("Play featured rotation"), "play"
			}
			b := kit.Button(c.Key("rotation"), "", kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM, Icon: icon, Filled: true, A11yLabel: name})
			f.seeKeyboard(b).Size(28, 28).Margin(0, 8, 0, 0)
			if b.Clicked() {
				if f.paused {
					c.Announce(i18n.T("Rotation playing"))
				} else {
					c.Announce(i18n.T("Rotation paused"))
				}
				f.paused = !f.paused
			}
		}
		if pg.count <= 1 {
			return
		}
		turn := func(by int) {
			to := (pg.at + by + pg.count) % pg.count
			f.choose(to)
			c.Announce(i18n.T("{current} of {pages}", "current", to+1, "pages", pg.count))
		}
		prev := kit.Button(c.Key("previous"), "", kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM, Icon: "chevron-left", A11yLabel: i18n.T("Previous in {label}", "label", pg.label)})
		f.seeKeyboard(prev).Size(28, 28)
		if prev.Clicked() {
			turn(-1)
		}
		theme.Text(c, i18n.T("{current} of {pages}", "current", pg.at+1, "pages", pg.count), "meta").
			TextColor(theme.C("ink-muted")).FontFeatures("tnum").Margin(0, 4, 0, 4)
		next := kit.Button(c.Key("next"), "", kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM, Icon: "chevron-right", A11yLabel: i18n.T("Next in {label}", "label", pg.label)})
		f.seeKeyboard(next).Size(28, 28)
		if next.Clicked() {
			turn(1)
		}
	})
}

// queue is the next cards in the rotation after the lead, up to queueSize,
// as tiles: beside the lead in two rows of two, or in a row under it. Each
// turn of the rotation moves every tile on by one place, the one that leads
// now leaving and the next coming in at the end.
func (d *discover) queue(c *ui.Context, items []services.Item, lay frontLayout, ctx kit.ItemsCtx) {
	count := len(items)
	if count < 2 {
		return
	}
	gapX, gapY, top := float32(tileGapX), float32(tileGapY), float32(0)
	if !lay.beside {
		gapX, top = queueNarrowGap, 24
	}
	ui.Grid(c.Key("queue")).Columns(lay.columns).GapX(gapX).GapY(gapY).Margin(top, 0, 0, 0).Shrink(0).
		Role(ui.RoleList).Label(i18n.T("Up next")).Children(func() {
		for k := 1; k <= min(queueSize, count-1); k++ {
			index := (d.front.at + k) % count
			d.queueTile(c, items[index], index, lay, ctx)
		}
	})
}

// queueTile is a card of the queue: its picture in the hero card's shape
// over its title and byline. It opens its card as the lead does; a play
// button, a "Show in lead" button and the menu show on hover and focus.
//
// The tile is a list item holding the opener and, over it, its buttons:
// controls inside a button are not on the tree assistive technology reads.
func (d *discover) queueTile(c *ui.Context, it services.Item, index int, lay frontLayout, ctx kit.ItemsCtx) {
	tk := theme.Current()
	f := &d.front
	name := it.Name
	open, link := d.opener(it, ctx)
	target, hasTarget := kit.ActionItemOf(it, ctx.Players)
	menu := kit.ActionTarget{Item: target, Name: name, Key: "tile:" + strconv.Itoa(index)}
	hasMenu := hasTarget && ctx.Menus.HasMenu(target)
	w, artH := lay.tile, lay.tileArt

	item := ui.Column(c.Key(leadKey(it, index))).Width(w).Role(ui.RoleListItem)
	if !theme.ReducedMotion() {
		// The queue moves on with the lead: each tile slides to its new
		// place, the one now leading fades where it was and the next
		// comes in at the end.
		item.Transition(ui.ElementTransition{
			Duration: leadFade, Ease: ui.EaseInOut, Position: true,
			Enter: &ui.Motion{X: leadSlide}, Exit: &ui.Motion{},
		})
	}
	hovered := item.Hovered()
	hot := hovered || item.FocusWithin()
	item.Children(func() {
		var card ui.Element
		if open != nil {
			card = ui.ButtonBase(c.Key("card")).Column().AlignItems(ui.Start).Radius(tk.Radius["sm"]).Label(name)
			if link {
				card.Role(ui.RoleLink)
			}
			if sub := kit.ItemSubtitle(it); sub != "" {
				card.Description(sub)
			}
		} else {
			card = ui.Column(c.Key("card")).AlignItems(ui.Start).Label(name)
		}
		card.Width(w)
		f.seeKeyboard(card)
		card.Children(func() {
			art := ui.Box(c.Key("art")).Size(w, artH).Radius(tk.Radius["sm"]).ClipX().ClipY().Background(theme.C("hover"))
			if colour, ok := kit.ParseHex(it.Color); ok {
				art.Background(colour)
			}
			art.Children(func() {
				if src, contain := leadArt(it); src != "" {
					kit.Artwork(c, ctx.Art, src, w, kit.ArtOpts{Aspect: w / artH, Edge: true, Contain: contain, Fallback: func(*ui.Context) {}})
				}
			})
			ui.Row(c.Key("title")).Width(w).Height(20).Margin(8, 0, 0, 0).Gap(4).AlignItems(ui.Center).Children(func() {
				t := theme.StyleText(ui.Text(c, name), "label").TextColor(theme.C("ink")).SingleLine().Grow(1).MinWidth(0).TextAlign(ui.Start)
				if hovered && open != nil {
					t.Underline()
				}
				if hasMenu {
					ui.Box(c.Key("more-room")).Size(24, 1).Shrink(0).IgnoreInput(true) // where the more button sits
				}
			})
			theme.Text(c, kit.ItemSubtitle(it), "secondary").TextColor(theme.C("ink-muted")).SingleLine().Width(w).Height(18).TextAlign(ui.Start)
		})
		if ctx.Players.CanPlay(it) {
			play := f.seeKeyboard(kit.PlayButton(c.Key("play"), i18n.T("Play {name}", "name", name), 32)).Absolute().Left(8).Top(artH - 8 - 32)
			kit.Fade(play, hot || play.Focused())
			opener := f.openers.For("tile:" + leadKey(it, index))
			play.Bind(opener)
			if play.Clicked() {
				ctx.Players.PlayItemFrom(it, opener)
			}
		}
		show := kit.Button(c.Key("show"), i18n.T("Show in lead"), kit.ButtonOpts{Size: kit.SizeSM, Icon: "maximize-2", A11yLabel: i18n.T("Show in lead: {name}", "name", name)})
		f.seeKeyboard(show).Absolute().Right(8).Top(8)
		kit.Fade(show, hot || show.Focused())
		if show.Clicked() {
			// The tile goes with its card into the lead: the keyboard's focus
			// goes there too rather than to nowhere.
			f.focusLead = show.FocusVisible()
			f.choose(index)
			c.Invalidate()
		}
		if hasMenu {
			f.seeKeyboard(ctx.Menus.Trigger(c.Key("more"), card, menu, kit.TriggerOpts{Visible: hot})).Absolute().Right(0).Top(artH + 6)
		}
		if card.Clicked() && open != nil {
			open()
		}
	})
}

// frontSkeleton is the front page's frame while it loads: the lead, its
// progress and caption and the queue's tiles as flat bars at their real
// sizes.
func (d *discover) frontSkeleton(c *ui.Context, m *kit.WidthTracker) {
	tk := theme.Current()
	root := ui.Column(c.Key("front-skeleton")).FillWidth().Margin(0, 0, 48, 0).Role(ui.RoleNone)
	width := m.Track(c, root)
	lay := layoutFor(width, d.newPg.pg.Height)
	root.Children(func() {
		if width <= 0 {
			return
		}
		lead := func() {
			ui.Column(c.Key("lead")).Width(lay.lead).Shrink(0).Children(func() {
				ui.Box(c).Size(lay.lead, lay.art).Radius(tk.Radius["sm"]).Background(theme.C("hover"))
				ui.Box(c).Size(lay.lead, progressHeight).Margin(progressGap, 0, 0, 0).Radius(progressHeight / 2).Background(theme.C("hover"))
				ui.Row(c).Height(30).Margin(leadCaptionGap, 0, 0, 0).AlignItems(ui.Center).Children(func() { kit.Bar(c, lay.lead/3, 20) })
				ui.Row(c).Height(leadLine).Margin(2, 0, 0, 0).AlignItems(ui.Center).Children(func() { kit.Bar(c, lay.lead/2, 14) })
				ui.Row(c).Height(18).AlignItems(ui.Center).Children(func() { kit.Bar(c, lay.lead/4, 12) })
			})
		}
		tiles := func() {
			gapX, top := float32(tileGapX), float32(0)
			if !lay.beside {
				gapX, top = queueNarrowGap, 24
			}
			ui.Grid(c.Key("queue")).Columns(lay.columns).GapX(gapX).GapY(tileGapY).Margin(top, 0, 0, 0).Shrink(0).Children(func() {
				for k := range queueSize {
					ui.Column(c.Key(k)).Width(lay.tile).Children(func() {
						kit.Bar(c, lay.tile, lay.tileArt)
						ui.Row(c).Height(20).Margin(8, 0, 0, 0).AlignItems(ui.Center).Children(func() { kit.Bar(c, lay.tile*3/4, 14) })
						ui.Row(c).Height(18).AlignItems(ui.Center).Children(func() { kit.Bar(c, lay.tile/2, 12) })
					})
				}
			})
		}
		if lay.beside {
			ui.Row(c.Key("spread")).Gap(queueGap).AlignItems(ui.Start).Children(func() {
				lead()
				tiles()
			})
		} else {
			lead()
			tiles()
		}
	})
}
