package onboarding

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// stripHeight is the strip's height: a segment's 4 above, 4 for its bar, 6
// gap, the name's 16 line and 2 under.
const stripHeight = 32

// stepStrip is where the setup is, as a strip of one segment per step,
// filled up to the current one, each named under it; every segment is a
// button to its step (back, or ahead: each step can be skipped). One Tab
// stop, with ←/→ (swapped right to left), Home and End moving the focus
// between the segments, Enter or Space going there (StepProgress.tsx; MyGo's
// focus group is the one stop and the arrow keys; the Tab stop is the
// segment that had the focus last, else the first: MyGo's group only enters
// on a checked element, which a button must not claim to be). It is a list
// named "Setup steps" of one item per step (the nav and ol of v0.2.0), and
// the current step's button says so in its description: MyGo has no
// aria-current, and "Current page" is what stands in for it.
func (o *onboarding) stepStrip(c *ui.Context, step string) {
	at := stepIndex(step)
	ui.Row(c.Key("strip")).FillWidth().Height(stripHeight).Shrink(0).Gap(4).AlignItems(ui.Stretch).
		FocusGroup(ui.Horizontal).Role(ui.RoleList).Label(i18n.T("Setup steps")).Children(func() {
		for i, s := range setupSteps {
			clicked := false
			ui.Column(c.Key("item:" + s)).Grow(1).Basis(0).MinWidth(0).AlignItems(ui.Stretch).Role(ui.RoleListItem).Children(func() {
				clicked = o.stepButton(c, s, i, at, step).Clicked()
			})
			if clicked {
				o.goToStep(s)
			}
		}
	})
}

// stepButton is one segment of the strip: the bar and the step's name, the
// button to the step s (the i-th; at is the current step's index). The
// current one is described as the current page.
func (o *onboarding) stepButton(c *ui.Context, s string, i, at int, step string) ui.Element {
	name := stepName(s)
	b := ui.ButtonBase(c.Key("step:"+s)).Column().Grow(1).Gap(6).
		Padding(4, 0, 2, 0).AlignItems(ui.Stretch).Radius(theme.Current().Radius["sm"]).
		Cursor(ui.CursorPointer).Label(name)
	if s == step {
		b.Description(i18n.T("Current page"))
	}
	hover := b.Hovered()
	bar := theme.C("ink").Alpha(0.15)
	switch {
	case i <= at:
		bar = theme.C("ink")
	case hover:
		bar = theme.C("ink").Alpha(0.35)
	}
	fg, weight := theme.C("ink-muted"), 400
	switch {
	case s == step:
		fg, weight = theme.C("ink"), 500
	case hover:
		fg = theme.C("ink")
	}
	return b.Children(func() {
		ui.Box(c.Key("bar")).Height(4).FillWidth().Shrink(0).Radius(2).Background(bar)
		text(c.Key("name"), name, "meta").FontWeight(weight).TextColor(fg).SingleLine().MinWidth(0)
	})
}
