package onboarding

import (
	"strconv"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// The rail's metrics: 36px items 2px apart, 16px above and below.
const (
	railWidth = 192
	railPad   = 16
	railItem  = 36
	railGap   = 2
)

// railHeight is the rail's height: what a card needs under its head to hold it.
func railHeight() float32 {
	n := float32(len(setupSteps))
	return 2*railPad + n*railItem + (n-1)*railGap
}

// stripHeight is the strip's height: a segment's 4 above, 4 for its bar, 6
// gap, the name's 16 line and 2 under.
const stripHeight = 32

// stepRail is where the setup is on the wide card: one row per step down the
// card's leading side on the surface tone, each with its number, or a check
// once the step is behind, and its name; the current step has an ink bar and
// the heavier weight. It is the strip's keyboard contract turned to a column:
// one Tab stop, ↑/↓ (and ←/→) moving between the rows, Home and End to the
// ends, Enter or Space going to the step, and the same named list of one
// button per step, the current one the current step.
func (o *onboarding) stepRail(c *ui.Context, step string) {
	at := stepIndex(step)
	ui.Column(c.Key("rail")).Width(railWidth).Shrink(0).Padding(railPad, 8).Gap(railGap).Background(theme.C("surface")).
		FocusGroup(ui.Horizontal|ui.Vertical).Role(ui.RoleList).Label(i18n.T("Setup steps")).Children(func() {
		for i, s := range setupSteps {
			clicked := false
			ui.Column(c.Key("item:" + s)).FillWidth().Role(ui.RoleListItem).Children(func() {
				clicked = o.railButton(c, s, i, at, step).Clicked()
			})
			if clicked {
				o.goToStep(s)
			}
		}
	})
}

// railButton is one row of the rail, the button to the step s (the i-th; at is
// the current step's index).
func (o *onboarding) railButton(c *ui.Context, s string, i, at int, step string) ui.Element {
	name := stepName(s)
	b := ui.ButtonBase(c.Key("step:"+s)).Row().FillWidth().Height(railItem).Padding(0, 12, 0, 16).Gap(10).
		Justify(ui.Start).AlignItems(ui.Center).Radius(theme.Current().Radius["md"]).Cursor(ui.CursorPointer).Label(name)
	current := s == step
	if current {
		b.Current(ui.CurrentStep)
	}
	hover := b.Hovered()
	if hover && !current {
		b.Background(theme.C("hover"))
	}
	fg, weight := theme.C("ink-muted"), 400
	switch {
	case current:
		fg, weight = theme.C("ink"), 600
	case hover:
		fg = theme.C("ink")
	}
	return b.Children(func() {
		if current {
			ui.Box(c.Key("bar")).Absolute().Left(0).Top(10).Size(3, railItem-20).Radius(2).Background(theme.C("ink"))
		}
		ui.Row(c.Key("marker")).Size(16, 16).Shrink(0).Center().Children(func() {
			if i < at {
				ui.Icon(c.Key("done"), theme.Icon("check")).FontSize(14).TextColor(theme.C("ink-muted"))
				return
			}
			num := theme.C("ink-faint")
			if current {
				num = theme.C("ink")
			}
			text(c.Key("number"), strconv.Itoa(i+1), "meta").FontWeight(weight).TextColor(num).FontFeatures("tnum")
		})
		text(c.Key("name"), name, "ui").FontWeight(weight).TextColor(fg).SingleLine().MinWidth(0)
	})
}

// stepStrip is where the setup is on the narrow card, as a strip of one segment per step,
// filled up to the current one, each named under it; every segment is a
// button to its step (back, or ahead: each step can be skipped). One Tab
// stop, with ←/→ (swapped right to left), Home and End moving the focus
// between the segments, Enter or Space going there (StepProgress.tsx; MyGo's
// focus group is the one stop and the arrow keys; the Tab stop is the
// segment that had the focus last, else the first: MyGo's group only enters
// on a checked element, which a button must not claim to be). It is a list
// named "Setup steps" of one item per step (the nav and ol of v0.2.0), and
// the current step's button is the current step (aria-current="step").
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
// current one is told as the current step.
func (o *onboarding) stepButton(c *ui.Context, s string, i, at int, step string) ui.Element {
	name := stepName(s)
	b := ui.ButtonBase(c.Key("step:"+s)).Column().Grow(1).Gap(6).
		Padding(4, 0, 2, 0).AlignItems(ui.Stretch).Radius(theme.Current().Radius["sm"]).
		Cursor(ui.CursorPointer).Label(name)
	if s == step {
		b.Current(ui.CurrentStep)
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
