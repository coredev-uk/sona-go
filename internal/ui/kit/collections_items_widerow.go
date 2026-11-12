package kit

import (
	"strconv"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// WideRowState is the state of a WideRow: the page showing.
type WideRowState struct {
	// Page is the page showing, from 0; a page beyond the last shows the
	// last.
	Page  int
	width WidthTracker
}

// WideRowOpts are the options of WideRow.
type WideRowOpts struct {
	// Title heads the row; empty for Apple's untitled heroes, which are
	// labelled "Featured" and get a heading only when they have pages.
	Title string
	// First leaves out the 40 DIPs above: the first section of a page.
	First bool
	// Min is the smallest card width: 420 for heroes, 240 for category
	// bricks.
	Min float32
	// Ratio is the artwork frame's width over its height. With Crop it is
	// every frame's shape; otherwise it is the tallest frame (2 for
	// bricks), and a row whose banners are all wider gets their shape
	// instead.
	Ratio float32
	// Crop crops every banner to Ratio, as Apple's hero shelf does: its 4:1
	// art keeps what matters in the centre, and Apple's CDN crops it to the
	// box.
	Crop bool
}

// WideRow is wide editorial cards (v0.2.0's WideRow): the banner (never under
// text), the title and the line below. Every frame in a row has one shape. A
// cropping row (heroes) fills it with each banner cropped to it. Otherwise
// banners sit whole on their own background colour in the tallest of the row's
// banner shapes within Ratio, so a row of 4:1 bricks has no empty bands and a
// mixed row letterboxes only the wider ones. As many cards as fit; the arrows
// page through the rest.
func WideRow(c *ui.Context, st *WideRowState, items []services.Item, ctx ItemsCtx, o WideRowOpts) {
	tk := theme.Current()
	// Apple leaves heroes untitled; a paged one still gets a heading.
	label := o.Title
	if label == "" {
		label = i18n.T("Featured")
	}
	root := ui.Column(c.Key("kit.wide." + label)).Role(ui.RoleGroup).Label(label)
	if !o.First {
		root.Margin(40, 0, 0, 0)
	}
	width := st.width.Track(c, root)
	root.Children(func() {
		if width <= 0 {
			ui.Box(c).Height(1) // the first frame measures; the next builds the cards
			return
		}
		cols, tile := CoverColumns(width, o.Min)
		frame := o.Ratio
		if !o.Crop {
			if a := itemsWidestShape(items); a > 0 {
				frame = max(o.Ratio, a)
			}
		}
		pages := max(1, (len(items)+cols-1)/cols)
		at := min(st.Page, pages-1)
		shown := items[min(len(items), at*cols):min(len(items), at*cols+cols)]

		if o.Title != "" || pages > 1 {
			st.header(c, label, at, pages)
		}
		ui.Row(c).Gap(TileGapX).AlignItems(ui.Start).Role(ui.RoleList).Label(label).Children(func() {
			for i := range shown {
				it := shown[i]
				key := strconv.Itoa(at) + ":" + strconv.Itoa(i) + ":" + it.Kind + ":" + it.ID
				ui.Column(c.Key(key)).Width(tile).Shrink(0).Role(ui.RoleListItem).Children(func() {
					itemsWideCard(c, it, tile, frame, o.Crop, ctx, tk)
				})
			}
		})
		if ctx.Menus != nil && !ctx.NoPopup {
			ctx.Menus.Popup(c)
		}
	})
}

// itemsWidestShape is the tallest banner shape of the row: the smallest width
// over height of its items' wide artwork, 0 when none has any.
func itemsWidestShape(items []services.Item) float32 {
	var tallest float32
	for i := range items {
		wide := items[i].Wide.Large
		if wide == "" {
			wide = items[i].Wide.Medium
		}
		if wide == "" {
			continue
		}
		if a := itemsArtAspect(wide); tallest == 0 || a < tallest {
			tallest = a
		}
	}
	return tallest
}

// header is the heading row: the title, and with pages "{n} of {pages}" and
// the arrows.
func (st *WideRowState) header(c *ui.Context, label string, at, pages int) {
	tk := theme.Current()
	meta := tk.Type["meta"]
	ui.Row(c.Key("head")).MinHeight(28).Margin(0, 0, 12, 0).Gap(16).AlignItems(ui.Center).Children(func() {
		theme.StyleText(ui.Text(c, label), "heading").
			TextColor(theme.C("ink")).SingleLine().Grow(1).MinWidth(0).Role(ui.RoleHeading).Level(2)
		if pages <= 1 {
			return
		}
		ui.Row(c.Key("pager")).Shrink(0).Gap(4).AlignItems(ui.Center).Children(func() {
			// A status, read as the page turns ("2 of 3"), once; keyed as
			// a status is told from the last frame's by its ID.
			ui.Text(c.Key("page"), i18n.T("{current} of {pages}", "current", at+1, "pages", pages)).
				FontSize(meta.Size).FixedLineHeight(meta.LineHeight).TextColor(theme.C("ink-faint")).
				FontFeatures("tnum").SingleLine().Margin(0, 4, 0, 0).Role(ui.RoleStatus)
			prev := Button(c.Key("prev"), "", ButtonOpts{
				Variant: Ghost, Size: SizeXS, Icon: "chevron-left", Disabled: at == 0,
				A11yLabel: i18n.T("Previous in {label}", "label", label),
			})
			next := Button(c.Key("next"), "", ButtonOpts{
				Variant: Ghost, Size: SizeXS, Icon: "chevron-right", Disabled: at >= pages-1,
				A11yLabel: i18n.T("Next in {label}", "label", label),
			})
			if prev.Clicked() {
				st.Page = at - 1
			}
			if next.Clicked() {
				st.Page = at + 1
			}
		})
	})
}

// itemsWideCard is one card: the banner on top, the title and the line below,
// never text over the banner. The whole card opens the item.
func itemsWideCard(c *ui.Context, it services.Item, width, ratio float32, crop bool, ctx ItemsCtx, tk *theme.Tokens) {
	link := ItemLink(it)
	banner := it.Wide.Large
	if banner == "" {
		banner = it.Wide.Medium
	}
	// A card without a banner shows its cover, whole, on the same colour.
	src := banner
	if src == "" {
		if src = it.Art.Large; src == "" {
			src = it.Art.Medium
		}
	}
	cover := crop && banner != ""

	fh := width / ratio
	// The card is a wrapper holding the opener and, beside it, the play
	// control and the more button: a button or a link is a leaf on the
	// accessibility tree, so controls inside it would not be on it.
	w := ui.Box(c.Key("card")).Column().Width(width).Shrink(0)
	hot := w.Hovered() || w.FocusWithin()
	target, hasTarget := ActionItemOf(it, ctx.Players)
	more := hasTarget && ctx.Menus != nil
	canPlay := ctx.Players.CanPlay(it)

	var card ui.Element
	w.Children(func() {
		card = ui.ButtonBase(c.Key("open")).Column().AlignItems(ui.Stretch).Justify(ui.Start).Label(it.Name)
		if link != "" {
			card.Role(ui.RoleLink)
		}
		card.Children(func() {
			bg := theme.C("hover")
			if col, ok := ParseHex(it.Color); ok {
				bg = col
			}
			fr := ui.Row(c.Key("frame")).Size(width, fh).Center().Radius(tk.Radius["sm"]).ClipX().ClipY().Background(bg)
			var v *artWindow
			if ctx.Art != nil {
				l := ctx.Art
				v = l.view(c)
				fr.Draw(func(p *ui.Painter, _ ui.Rect) { l.painted(v, p.Scale()) })
			}
			rk := rankBuilt
			if fr.InView() {
				rk = rankView
			}
			fr.Children(func() { itemsBanner(c, ctx.Art, v, rk, src, width, fh, cover, tk.Radius["sm"]) })

			ui.Row(c.Key("title")).Height(20).Margin(8, 0, 0, 0).Gap(4).AlignItems(ui.Center).MinWidth(0).Children(func() {
				name := theme.StyleText(ui.Text(c, it.Name), "label").
					TextColor(theme.C("ink")).SingleLine().Grow(1).MinWidth(0).TextAlign(ui.Start)
				if card.Hovered() {
					name.Underline()
				}
				if more {
					ui.Box(c.Key("more-room")).Size(24, 1).Shrink(0) // where the more button sits
				}
			})
			if sub := ItemSubtitle(it); sub != "" {
				sec := tk.Type["sub"]
				ui.Text(c, sub).FontSize(sec.Size).FixedLineHeight(sec.LineHeight).TextColor(theme.C("ink-muted")).
					MaxLines(2).TextAlign(ui.Start)
				// The subtitle is drawn inside the opener, which is read as
				// one piece named by its label: it is its description.
				card.Description(sub)
			} else {
				ui.Box(c).Height(18)
			}
		})
		if canPlay {
			play := PlayButton(c.Key("play"), i18n.T("Play {name}", "name", it.Name), 40).Absolute().Left(12).Top(fh - 12 - 40)
			if !hot && !play.Focused() {
				play.Opacity(0)
			}
			if play.Clicked() {
				ctx.Players.PlayItem(it)
			}
		}
		if more {
			ctx.Menus.Trigger(c.Key("more"), ActionTarget{Item: target, Name: it.Name}, TriggerOpts{Visible: hot}).
				Absolute().Right(0).Top(fh + 6)
		}
	})
	if card.Clicked() {
		ctx.opened(it)
		if link != "" {
			ctx.nav(link)
		} else {
			OpenWithoutPage(it, ctx)
		}
	}
}

// itemsBanner draws the card's image in its w by h frame, or nothing while it
// loads and when it failed, leaving the item colour: a banner is decoded at
// the frame's own shape, centred whole (contain) at its own aspect, or
// filling the frame when the row crops it.
func itemsBanner(c *ui.Context, l *ArtLoader, v *artWindow, r artRank, src string, w, h float32, crop bool, radius float32) {
	if l == nil || src == "" {
		return
	}
	bw, bh := w, h
	if !crop {
		if a := itemsArtAspect(src); a >= w/h {
			bh = w / a
		} else {
			bw = h * a
		}
	}
	cv := l.cover(v, r, src, bw, bh)
	bmp := cv.bitmap()
	if bmp == nil {
		return
	}
	edge := theme.C("edge")
	ui.Image(c, bmp).Size(bw, bh).Fit(ui.Cover).Radius(radius).
		Draw(func(*ui.Painter, ui.Rect) { cv.drawn() }).
		DrawOver(func(p *ui.Painter, r ui.Rect) { p.Stroke(r, edge, radius, 1) })
}
