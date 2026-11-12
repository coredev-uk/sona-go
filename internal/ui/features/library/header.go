package library

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// headOpts are what a library page's header says.
type headOpts struct {
	// title is the page's h1; empty while a page is still loading it.
	title string
	// count is the size of what the page lists, a whole phrase ("24
	// albums"); empty until it is known.
	count string
	// note is one meta line under both, for what the count cannot say
	// (where History's plays come from).
	note string
	// actions builds the page's controls into the row at the right edge.
	actions func()
}

// pageHead is the header every library page shares: the title at 24/650 and
// the size of the list on its baseline, the page's controls at the right
// edge on the same line, then one 1px rule that starts the list. The count
// takes no height of its own, so the controls and the rule do not move when
// it arrives.
func (l *library) pageHead(c *ui.Context, o headOpts) {
	ui.Column(c.Key("head")).FillWidth().Shrink(0).Margin(0, 0, 24, 0).Children(func() {
		ui.Row(c.Key("line")).FillWidth().Wrap().AlignItems(ui.Center).Justify(ui.SpaceBetween).Gap(16).Children(func() {
			ui.Column(c.Key("who")).MinWidth(0).Shrink(1).Children(func() {
				ui.Row(c.Key("title")).AlignItems(ui.Baseline).Gap(12).MinWidth(0).Children(func() {
					head := theme.Text(c, o.title, "page").TextColor(theme.C("ink")).SingleLine().Role(ui.RoleHeading).Level(1)
					if o.title == "" {
						head.Role(ui.RoleNone) // a page still loading has no title to read
					}
					if o.count != "" {
						theme.Text(c, o.count, "sub").TextColor(theme.C("ink-muted")).FontFeatures("tnum").SingleLine()
					}
				})
				if o.note != "" {
					theme.Text(c, o.note, "meta").TextColor(theme.C("ink-faint")).Margin(4, 0, 0, 0)
				}
			})
			if o.actions != nil {
				ui.Row(c.Key("actions")).Shrink(0).Gap(8).AlignItems(ui.Center).Children(o.actions)
			}
		})
		ui.Box(c.Key("rule")).FillWidth().Height(1).Background(theme.C("line")).Margin(16, 0, 0, 0)
	})
}
