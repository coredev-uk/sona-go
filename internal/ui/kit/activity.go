package kit

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// ActivitySpec is one live row of the notification centre, as a module's
// Activity builds it: work under way (a sync, a download) or a state that
// needs the person (an update to install, a download that failed).
type ActivitySpec struct {
	// Icon is the Lucide glyph at 16, in --ink-muted (--danger with Alert).
	Icon string
	// Alert: something failed. The glyph is --danger; the words say what.
	Alert bool
	// Title is the line in --ink at body size; Count sits at its end in
	// --ink-muted tabular figures ("812 of 2,040").
	Title string
	Count string
	// Detail is under the title in --ink-muted, up to two lines.
	Detail string
	// Progress draws a 3px bar under the words, 0 to 1 (below 0 while
	// unknown); nil draws none. ProgressLabel names it.
	Progress      *float64
	ProgressLabel string
	// Action is the row's button (a small ink button) and Secondary a
	// quieter one after it; each shows only with its func.
	Action      string
	OnAction    func()
	Secondary   string
	OnSecondary func()
}

// ActivityRow draws s: a --hover block at the md radius, the glyph at the
// leading edge in a 20px column, the words beside it and the buttons under
// them. The block is a labelled group, not a status: its count changes as
// the work moves and is not read out each time (the module announces the
// changes of phase that matter).
func ActivityRow(c *ui.Context, key string, s ActivitySpec) {
	tk := theme.Current()
	icon := theme.C("ink-muted")
	if s.Alert {
		icon = theme.C("danger")
	}
	ui.Row(c.Key(key)).FillWidth().Padding(10, 12).Gap(10).Radius(tk.Radius["md"]).Background(theme.C("hover")).
		AlignItems(ui.Start).Role(ui.RoleGroup).Label(s.Title).Children(func() {
		ui.Box(c.Key("glyph")).Width(20).Height(20).Shrink(0).Center().Children(func() {
			ui.Icon(c.Key("icon"), theme.Icon(s.Icon)).FontSize(16).TextColor(icon)
		})
		ui.Column(c.Key("words")).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
			ui.Row(c.Key("head")).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				theme.Text(c, s.Title, "body").FontWeight(500).TextColor(theme.C("ink")).Grow(1).Shrink(1).MinWidth(0).MaxLines(2)
				if s.Count != "" {
					theme.Text(c, s.Count, "meta").FontFeatures("tnum").TextColor(theme.C("ink-muted")).Shrink(0).SingleLine()
				}
			})
			if s.Detail != "" {
				theme.Text(c, s.Detail, "sub").TextColor(theme.C("ink-muted")).FillWidth().MaxLines(2)
			}
			if s.Progress != nil {
				ProgressBar(c.Key("progress"), *s.Progress, ProgressOpts{A11yLabel: s.ProgressLabel, ValueText: s.Count, Height: 3}).
					FillWidth().Margin(6, 0, 2, 0)
			}
			if (s.Action != "" && s.OnAction != nil) || (s.Secondary != "" && s.OnSecondary != nil) {
				ui.Row(c.Key("acts")).Gap(8).Margin(8, 0, 0, 0).Children(func() {
					if s.Action != "" && s.OnAction != nil {
						if Button(c.Key("act"), s.Action, ButtonOpts{Size: SizeSM}).Clicked() {
							s.OnAction()
						}
					}
					if s.Secondary != "" && s.OnSecondary != nil {
						if Button(c.Key("act2"), s.Secondary, ButtonOpts{Variant: Ghost, Size: SizeSM}).Clicked() {
							s.OnSecondary()
						}
					}
				})
			}
		})
	})
}
