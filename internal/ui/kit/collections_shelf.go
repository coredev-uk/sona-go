package kit

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// ShelfOpts are the options of Shelf.
type ShelfOpts struct {
	// Title heads the section and names it for assistive technology.
	Title string
	// First leaves out the 40 DIPs above: the first shelf of a page.
	First bool
	// More is "See all": it opens the section's full list as its own page.
	// Nil: no link.
	More func()
	// Actions builds extra controls at the right of the heading (a
	// ShelfToggle).
	Actions func()
}

// shelfControl styles a heading-row control: "See all", "Show all".
func shelfControl(c *ui.Context, e ui.Element) ui.Element {
	tk := theme.Current()
	bg, fg := ui.Transparent, shelfInk(e)
	if e.Hovered() {
		bg = theme.C("hover")
	}
	e.Height(28).PaddingX(6).MarginEnd(-4).Gap(2).Radius(tk.Radius["md"]).Background(bg).TextColor(fg).Center()
	theme.Fade(e, "feedback")
	return e
}

// shelfInk is the colour of a heading-row control's text and icon: ink
// under the pointer, muted ink otherwise.
func shelfInk(e ui.Element) ui.Color {
	if e.Hovered() {
		return theme.C("ink")
	}
	return theme.C("ink-muted")
}

// ShelfToggle is a heading-row button that opens the shelf's content in
// place: "Show all", "Show fewer". open is what it shows now; read Clicked
// to flip it.
func ShelfToggle(c *ui.Context, label string, open bool) ui.Element {
	return shelfToggle(c, label, open, false)
}

// ShelfDisclosure is ShelfToggle with a chevron that turns over when open.
func ShelfDisclosure(c *ui.Context, label string, open bool) ui.Element {
	return shelfToggle(c, label, open, true)
}

func shelfToggle(c *ui.Context, label string, open, chevron bool) ui.Element {
	e := shelfControl(c, ui.ButtonBase(c.Key("kit.shelf.toggle")))
	e.Role(ui.RoleDisclosure).Expanded(open).Label(label)
	tk := theme.Current()
	e.Children(func() {
		ui.Text(c, label).FontSize(tk.Type["sub"].Size).FixedLineHeight(tk.Type["sub"].LineHeight).SingleLine()
		if !chevron {
			return
		}
		ic := theme.Glyph(c, "chevron-down", 14, theme.GlyphOpts{Color: shelfInk(e), Inherit: true})
		turn := float32(0)
		if open {
			turn = 180
		}
		if theme.ReducedMotion() {
			ic.Rotate(turn)
		} else {
			ic.Rotate(ic.Animate("turn", turn, tk.Motion["press"].Duration))
		}
	})
	return e
}

// ShelfSeeAll is a heading-row "See all" link under key: it runs open on a
// click.
func ShelfSeeAll(c *ui.Context, key string, open func()) {
	see := shelfControl(c, ui.ButtonBase(c.Key(key))).Role(ui.RoleLink)
	see.Children(func() {
		theme.Text(c, i18n.T("See all"), "sub").SingleLine()
		theme.Glyph(c, "chevron-right", 14, theme.GlyphOpts{Color: shelfInk(see), Inherit: true})
	})
	if see.Clicked() {
		open()
	}
}

// Shelf is a titled section of a page (v0.2.0's Shelf): the heading at 16/600
// with "See all" (or other controls) at its right, then the content. Shelves
// stack 40 DIPs apart (the first has none above).
func Shelf(c *ui.Context, o ShelfOpts, content func()) {
	sec := ui.Column(c.Key("kit.shelf." + o.Title)).Role(ui.RoleGroup).Label(o.Title)
	if !o.First {
		sec.Margin(40, 0, 0, 0)
	}
	sec.Children(func() {
		ui.Row(c).MinHeight(28).Margin(0, 0, 12, 0).Gap(16).AlignItems(ui.Center).Children(func() {
			theme.StyleText(ui.Text(c, o.Title), "heading").
				TextColor(theme.C("ink")).SingleLine().Grow(1).MinWidth(0).Role(ui.RoleHeading).Level(2)
			ui.Row(c).Shrink(0).Gap(4).AlignItems(ui.Center).Children(func() {
				if o.Actions != nil {
					o.Actions()
				}
				if o.More != nil {
					ShelfSeeAll(c, "kit.shelf.more", o.More)
				}
			})
		})
		content()
	})
}

// CoverShelfState is the state of a CoverShelf.
type CoverShelfState struct {
	// Row is the tiles' state (selection, navigation).
	Row CoverState
	// Open shows every row of tiles instead of one.
	Open bool
	cols int
}

// CoverShelfOpts are the options of CoverShelf.
type CoverShelfOpts struct {
	CoverOpts
	// First: see ShelfOpts.
	First bool
	// More is "See all" to the section's page; without one, "Show all"
	// opens the rest in place when more were sent than fit.
	More func()
}

// CoverShelf is a titled shelf of cover tiles: one row that fits the width,
// with "See all" to the section's page, or "Show all" in place when more
// were sent than fit. It builds nothing for an empty list.
func CoverShelf(c *ui.Context, st *CoverShelfState, title string, items []CoverItem, o CoverShelfOpts) {
	if len(items) == 0 {
		return
	}
	if o.Min == 0 && o.Video {
		o.Min = 240
	}
	so := ShelfOpts{Title: title, First: o.First, More: o.More}
	if o.More == nil && len(items) > st.cols && st.cols > 0 {
		so.Actions = func() {
			label := i18n.T("Show all")
			if st.Open {
				label = i18n.T("Show fewer")
			}
			if ShelfToggle(c, label, st.Open).Clicked() {
				st.Open = !st.Open
			}
		}
	}
	Shelf(c, so, func() {
		rows := 1
		if st.Open {
			rows = -1
		}
		ro := CoverRowOpts{CoverOpts: o.CoverOpts, Rows: rows, OnColumns: func(n int) {
			st.cols = n
			c.Invalidate() // the toggle depends on how many fit
		}}
		ro.Label = title
		CoverRow(c, &st.Row, items, ro)
	})
}
