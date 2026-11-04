package discover

import (
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// front is the front page's state (v0.2.0's FrontPage): which card leads,
// the lead on its way out, the page of the list the user turned to, the
// user's pause and the rotation's timer.
type front struct {
	width measure
	// at is the lead's place in the items.
	at int
	// fading: the lead from is on its way out, since fadeAt, while the new
	// one fades in.
	fading bool
	from   int
	fadeAt time.Time
	// turned: the user turned the list to page turnPage while the lead was
	// turnAt; the list shows the lead's page otherwise.
	turned           bool
	turnAt, turnPage int
	// paused is the user's pause (the control's state), not the temporary
	// holds.
	paused bool
	rot    rotation
	menus  *kit.ActionMenus
}

// choose makes card to the lead; the old lead fades out while it fades in,
// unless motion is reduced.
func (f *front) choose(c *ui.Context, to int) {
	if to == f.at {
		return
	}
	if theme.ReducedMotion() {
		f.at, f.fading = to, false
		return
	}
	f.from, f.at, f.fading, f.fadeAt = f.at, to, true, c.Now()
}

// frontPage is Discover's front page: Apple's featured cards, one of them the
// lead, large, and all of them as a list beside it (under it in a narrow
// window), paged so the list never runs past the lead. The lead moves on by
// itself unless the user pauses it or asks for reduced motion; choosing a row
// makes it the lead, and the list shows the page the lead is on.
func (d *discover) frontPage(c *ui.Context, items []services.Item) {
	count := len(items)
	if count == 0 {
		return
	}
	f := &d.front
	tk := theme.Current()
	gapX := tk.Metric["tile-gap-x"]
	ctx := d.env.ItemsCtx(f.menus)
	label := i18n.T("Featured")

	sec := ui.Column(c.Key("front")).FillWidth().Margin(0, 0, 48, 0).Role(ui.RoleGroup).Label(label)
	width := f.width.track(c, sec)
	lay := layoutFor(width, gapX)
	// A page that is built again after it went away starts over: the first
	// card leads, rotation is not paused (v0.2.0's component state was lost on
	// unmount). An element's state lives only while it is built.
	if mounted := ui.Local(sec, "mounted", func() bool { return false }); !*mounted {
		*mounted = true
		f.at, f.fading, f.paused, f.turned, f.rot = 0, false, false, false, rotation{}
	}
	f.at = min(f.at, count-1)

	// The rotation: one deadline, asked for as a frame, no per-frame work.
	available := !theme.ReducedMotion() && count > 1
	engaged := sec.Hovered() || sec.FocusWithin()
	running := available && width > 0 && !f.paused && !engaged && d.seen(sec)
	advance, wait := f.rot.step(c.Now(), running, f.at, d.occupied)
	switch {
	case advance:
		f.choose(c, (f.at+1)%count)
		f.rot.restart(c.Now(), f.at)
		c.Invalidate()
	case wait > 0:
		c.After(wait)
	}
	at := f.at

	// The cross-fade: the new lead in as the old one goes out.
	ease := float32(1)
	if f.fading {
		p := float32(c.Now().Sub(f.fadeAt)) / float32(leadFade)
		if p >= 1 || theme.ReducedMotion() {
			f.fading = false
		} else {
			ease = ui.EaseInOut(max(p, 0))
			c.AnimationFrame()
		}
	}

	pages := max(1, (count+lay.perPage-1)/lay.perPage)
	// The list shows the lead's page until the user turns it, and returns to
	// it when the lead changes.
	page := at / lay.perPage
	if f.turned && f.turnAt == at {
		page = f.turnPage
	}
	page = min(page, pages-1)
	shown := items[page*lay.perPage : min(count, (page+1)*lay.perPage)]

	sec.Children(func() {
		if width <= 0 {
			return // the first frame measures; the next one lays it out
		}
		var grid ui.Element
		if lay.split {
			grid = ui.Row(c.Key("grid")).FillWidth().Gap(gapX).AlignItems(ui.Start)
		} else {
			grid = ui.Column(c.Key("grid")).FillWidth().Gap(24)
		}
		grid.Children(func() {
			ui.Box(c.Key("lead")).Width(lay.lead).Shrink(0).Children(func() {
				if f.fading && f.from < count {
					ui.Column(c.Key("out:"+leadKey(items[f.from], f.from))).Absolute().Top(0).Left(0).Width(lay.lead).
						IgnoreInput(true).Opacity(1-ease).Translate(-leadSlide*ease, 0).Children(func() {
						d.leadCard(c, items[f.from], lay, ctx, false)
					})
				}
				in := ui.Column(c.Key("in:" + leadKey(items[at], at))).Width(lay.lead)
				if ease < 1 {
					in.Opacity(ease).Translate(leadSlide*(1-ease), 0)
				}
				in.Children(func() { d.leadCard(c, items[at], lay, ctx, true) })
			})
			if count > 1 {
				d.featureList(c, items, shown, page, pages, available, lay, ctx, label)
			}
		})
		f.menus.Popup(c)

		// The next lead's picture is fetched and decoded while this one is
		// read, so it fades in as a picture rather than as an empty box: the
		// loader is asked for it where it will show, and nothing is drawn.
		if available {
			next := items[(at+1)%count]
			if src, contain := leadArt(next); src != "" {
				ui.Box(c.Key("prefetch")).Hidden(true).Children(func() {
					kit.Artwork(c, ctx.Art, src, lay.lead, kit.ArtOpts{Aspect: heroRatio, Contain: contain})
				})
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

// occupied is what only the tick can say: a menu is open, or the page is
// covered (under Now Playing, lyrics or a modal dialog). The lead waits a
// whole interval more.
func (d *discover) occupied() bool {
	if d.front.menus.IsOpen() || d.newPg.menus.IsOpen() || kit.ConfirmPending() {
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
// the cover contained on its colour), the title opening the item's page, the
// subtitle, a play button on hover or focus and the item's menu. live false is
// the copy on its way out: it takes no input.
func (d *discover) leadCard(c *ui.Context, it services.Item, lay frontLayout, ctx kit.ItemsCtx, live bool) {
	tk := theme.Current()
	name := it.Name
	open, link := d.opener(it, ctx)
	var card ui.Element
	if live && open != nil {
		card = ui.ButtonBase(c.Key("card")).Column().AlignItems(ui.Start).Radius(tk.Radius["sm"]).Label(name)
		if link {
			card.Role(ui.RoleLink)
		}
	} else {
		card = ui.Column(c.Key("card")).AlignItems(ui.Start)
		if live {
			card.Label(name)
		}
	}
	card.Width(lay.lead)
	hot := live && (card.Hovered() || card.FocusWithin())
	target, hasTarget := kit.ActionItemOf(it, ctx.Players)
	menu := kit.ActionTarget{Item: target, Name: name, Key: "lead"}

	card.Children(func() {
		colour, hasColour := itemColour(it)
		art := ui.Box(c.Key("art")).Size(lay.lead, lay.artHeight()).Radius(tk.Radius["sm"]).ClipX().ClipY().Background(theme.C("hover"))
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
			if live && ctx.Players.CanPlay(it) {
				play := kit.PlayButton(c.Key("play"), i18n.T("Play {name}", "name", name), 48).Absolute().Left(16).Bottom(16)
				fade(play, hot || play.Focused())
				if play.Clicked() {
					ctx.Players.PlayItem(it)
				}
			}
		})
		ui.Row(c.Key("title")).Height(22).Margin(8, 0, 0, 0).Gap(4).AlignItems(ui.Center).MinWidth(0).Children(func() {
			t := text(c, name, "heading").TextColor(theme.C("ink")).SingleLine().Grow(1).MinWidth(0).TextAlign(ui.Start)
			if live && card.Hovered() && open != nil {
				t.Underline()
			}
			if live && hasTarget && ctx.Menus != nil {
				ctx.Menus.Trigger(c.Key("more"), menu, kit.TriggerOpts{Visible: hot})
			}
		})
		text(c, kit.ItemSubtitle(it), "secondary").TextColor(theme.C("ink-muted")).SingleLine().Height(18).TextAlign(ui.Start)
	})
	if live {
		if card.Clicked() && open != nil {
			open()
		}
		if hasTarget && ctx.Menus != nil {
			ctx.Menus.Row(card, menu)
		}
	}
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

// featureList is the list of featured cards beside or under the lead, and
// its pager and the rotation's control.
func (d *discover) featureList(c *ui.Context, items, shown []services.Item, page, pages int, available bool, lay frontLayout, ctx kit.ItemsCtx, label string) {
	f := &d.front
	tk := theme.Current()
	gapX := tk.Metric["tile-gap-x"]
	at := f.at
	list := ui.Column(c.Key("list")).MinWidth(0)
	if lay.split {
		list.Grow(1).Basis(0).Height(lay.artHeight() + leadCaption)
	} else {
		list.FillWidth()
	}
	list.Children(func() {
		cols := ui.Row(c.Key("cards")).AlignSelf(ui.Stretch).Margin(0, -8, 0, -8).Gap(gapX - 16).Role(ui.RoleList).Label(i18n.T("Featured cards"))
		cols.Children(func() {
			for col := range lay.columns {
				ui.Column(c.Key(col)).Grow(1).Basis(0).MinWidth(0).Children(func() {
					for r := range lay.perColumn {
						i := col*lay.perColumn + r
						if i >= len(shown) {
							break
						}
						index := page*lay.perPage + i
						d.featureRow(c, shown[i], index, index == at, lay, ctx)
					}
				})
			}
		})
		if pages <= 1 && !available {
			return
		}
		if lay.split {
			ui.Box(c.Key("fill")).Grow(1) // the pager ends level with the lead's caption
		}
		d.pager(c, page, pages, available, label)
	})
}

// pager is the row under the list: the rotation's pause control and, with
// more than one page, "N of M" and the buttons that turn it. The pager wraps
// round rather than disabling a button, so focus never drops from it to the
// page.
func (d *discover) pager(c *ui.Context, page, pages int, available bool, label string) {
	f := &d.front
	ui.Row(c.Key("pager")).Height(28).Margin(8, 0, 0, 0).Shrink(0).Justify(ui.End).Gap(4).AlignItems(ui.Center).Children(func() {
		if available {
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
		if pages <= 1 {
			return
		}
		text(c, i18n.T("{current} of {pages}", "current", page+1, "pages", pages), "meta").
			TextColor(theme.C("ink-muted")).FontFeatures("tnum").Margin(0, 4, 0, 0)
		turn := func(by int) {
			to := (page + by + pages) % pages
			f.turned, f.turnAt, f.turnPage = true, f.at, to
			c.Announce(i18n.T("{current} of {pages}", "current", to+1, "pages", pages))
		}
		prev := kit.Button(c.Key("previous"), "", kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM, Icon: "chevron-left", A11yLabel: i18n.T("Previous in {label}", "label", label)})
		prev.Size(28, 28)
		if prev.Clicked() {
			turn(-1)
		}
		next := kit.Button(c.Key("next"), "", kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM, Icon: "chevron-right", A11yLabel: i18n.T("Next in {label}", "label", label)})
		next.Size(28, 28)
		if next.Clicked() {
			turn(1)
		}
	})
}

// featureRow is a list row: choosing it makes its card the lead (the lead's
// own title opens the card). current: the row is the lead now.
func (d *discover) featureRow(c *ui.Context, it services.Item, index int, current bool, lay frontLayout, ctx kit.ItemsCtx) {
	tk := theme.Current()
	f := &d.front
	name := it.Name
	row := ui.ButtonBase(c.Key(strconv.Itoa(index) + ":" + it.Kind + ":" + it.ID)).Height(lay.rowHeight).Shrink(0).PaddingX(8).Gap(12).
		AlignItems(ui.Center).Radius(tk.Radius["md"]).Label(name).Selected(current)
	hovered := row.Hovered()
	hot := hovered || row.FocusWithin()
	switch {
	case current:
		row.Background(theme.C("active"))
	case hovered:
		row.Background(theme.C("hover"))
	}
	if !theme.ReducedMotion() {
		m := tk.Motion["press"]
		row.Transition(ui.ElementTransition{Colors: true, Duration: m.Duration, Ease: m.Ease})
	}
	target, hasTarget := kit.ActionItemOf(it, ctx.Players)
	menu := kit.ActionTarget{Item: target, Name: name, Key: "row:" + strconv.Itoa(index)}
	row.Children(func() {
		src := it.Art.Medium
		if src == "" {
			src = it.Art.Large
		}
		if src == "" {
			src = it.Wide.Medium
		}
		thumb := ui.Box(c.Key("thumb")).Size(listThumb, listThumb).Shrink(0).Radius(tk.Radius["sm"]).ClipX().ClipY().Background(theme.C("hover"))
		if colour, ok := itemColour(it); ok {
			thumb.Background(colour)
		}
		thumb.Children(func() {
			kit.Artwork(c, ctx.Art, src, listThumb, kit.ArtOpts{Edge: true, Fallback: func(*ui.Context) {}})
			if ctx.Players.CanPlay(it) {
				play := kit.PlayButton(c.Key("play"), i18n.T("Play {name}", "name", name), 32).Absolute().Left(16).Top(16)
				fade(play, hot || play.Focused())
				if play.Clicked() {
					ctx.Players.PlayItem(it)
				}
			}
		})
		ui.Column(c.Key("text")).Grow(1).Basis(0).MinWidth(0).Children(func() {
			label := tk.Type["label"]
			ui.Text(c, name).FontSize(label.Size).FixedLineHeight(label.LineHeight).FontWeight(int(label.Weight)).
				TextColor(theme.C("ink")).SingleLine().TextAlign(ui.Start)
			if sub := kit.ItemSubtitle(it); sub != "" {
				text(c, sub, "secondary").TextColor(theme.C("ink-muted")).SingleLine().TextAlign(ui.Start)
			}
		})
		if hasTarget && ctx.Menus != nil {
			ctx.Menus.Trigger(c.Key("more"), menu, kit.TriggerOpts{Visible: hot})
		}
	})
	if row.Clicked() {
		f.choose(c, index)
	}
	if hasTarget && ctx.Menus != nil {
		ctx.Menus.Row(row, menu)
	}
}

// frontSkeleton is the front page's frame while it loads: the lead and its
// list as flat bars at their real sizes (v0.2.0's FrontPageSkeleton).
func (d *discover) frontSkeleton(c *ui.Context, m *measure) {
	tk := theme.Current()
	gapX := tk.Metric["tile-gap-x"]
	root := ui.Column(c.Key("front-skeleton")).FillWidth().Margin(0, 0, 48, 0).Role(ui.RoleNone)
	width := m.track(c, root)
	lay := layoutFor(width, gapX)
	root.Children(func() {
		if width <= 0 {
			return
		}
		var grid ui.Element
		if lay.split {
			grid = ui.Row(c).FillWidth().Gap(gapX).AlignItems(ui.Start)
		} else {
			grid = ui.Column(c).FillWidth().Gap(24)
		}
		grid.Children(func() {
			ui.Column(c.Key("lead")).Width(lay.lead).Shrink(0).Children(func() {
				ui.Box(c).Size(lay.lead, lay.artHeight()).Radius(tk.Radius["sm"]).Background(theme.C("hover"))
				ui.Row(c).Height(22).Margin(8, 0, 0, 0).AlignItems(ui.Center).Children(func() { bar(c, lay.lead/3, 16) })
				ui.Row(c).Height(18).AlignItems(ui.Center).Children(func() { bar(c, lay.lead/4, 12) })
			})
			ui.Row(c.Key("list")).Grow(1).Basis(0).MinWidth(0).Gap(gapX).Children(func() {
				for col := range lay.columns {
					ui.Column(c.Key(col)).Grow(1).Basis(0).MinWidth(0).Children(func() {
						for r := range lay.perColumn {
							if col*lay.perColumn+r >= lay.perPage {
								break
							}
							ui.Row(c.Key(r)).Height(lay.rowHeight).Gap(12).AlignItems(ui.Center).Children(func() {
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
	})
}
