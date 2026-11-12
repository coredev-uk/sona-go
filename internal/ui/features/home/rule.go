package home

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// shelfGapTop is the space above a shelf: the page's rhythm.
const shelfGapTop = 48

// ruledOpts are the options of ruledShelf.
type ruledOpts struct {
	// Title heads the section and names it for assistive technology.
	Title string
	// First leaves out the room above.
	First bool
	// More is "See all": the section's own page. Nil: no link.
	More func()
	// Actions builds extra controls at the right end of the rule (the
	// "Show all" toggle).
	Actions func()
}

// hairline is the colour of every rule on the page: ink at low alpha, so it
// reads on the ground and on a cover's wash alike.
func hairline() ui.Color { return theme.C("ink").Alpha(0.14) }

// rule is a 1 DIP line that takes the room between a heading and its
// controls.
func rule(c *ui.Context) ui.Element {
	return ui.Box(c.Key("rule")).Grow(1).Basis(0).Height(1).Background(hairline())
}

// ruledHead is a section's head: the heading at 16/600, a hairline that runs
// to the right edge, and the section's controls on its end ("See all", "Show
// all"). The line is what gives the page its rhythm.
func ruledHead(c *ui.Context, o ruledOpts) {
	h := theme.Current().Type["heading"]
	ui.Row(c.Key("head")).MinHeight(28).Margin(0, 0, 12, 0).Gap(16).AlignItems(ui.Center).Children(func() {
		ui.Text(c, o.Title).FontSize(h.Size).FixedLineHeight(h.LineHeight).FontWeight(int(h.Weight)).
			TextColor(theme.C("ink")).SingleLine().Shrink(1).MinWidth(0).Role(ui.RoleHeading).Level(2)
		rule(c)
		if o.Actions == nil && o.More == nil {
			return
		}
		ui.Row(c.Key("controls")).Shrink(0).Gap(4).AlignItems(ui.Center).Children(func() {
			if o.Actions != nil {
				o.Actions()
			}
			if o.More != nil {
				kit.ShelfSeeAll(c, "see-all", o.More)
			}
		})
	})
}

// ruledShelf is a titled section of the page: a ruled head over its content,
// 48 DIPs below the one before.
func ruledShelf(c *ui.Context, o ruledOpts, content func()) {
	sec := ui.Column(c.Key("shelf." + o.Title)).Role(ui.RoleGroup).Label(o.Title)
	if !o.First {
		sec.Margin(shelfGapTop, 0, 0, 0)
	}
	sec.Children(func() {
		ruledHead(c, o)
		content()
	})
}

// showAll is the head's "Show all {count}" / "Show fewer" toggle.
func showAll(c *ui.Context, count int, open *bool) {
	label := i18n.T("Show all {count}", "count", count)
	if *open {
		label = i18n.T("Show fewer")
	}
	if kit.ShelfDisclosure(c, label, *open).Clicked() {
		*open = !*open
	}
}
