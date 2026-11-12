package onboarding

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// The parts the steps are made of (parts.tsx).

// stepGroup is a part of a step: a heading one level under the step's
// title (h3; the dialog's title is the h2), then its controls. The group is
// named by the heading, so assistive technology reads it as a region.
func stepGroup(c *ui.Context, id, title string, build func()) {
	ui.Column(c.Key("group:" + id)).FillWidth().Gap(8).Role(ui.RoleGroup).Label(title).Children(func() {
		theme.Text(c, title, "ui").FontWeight(600).TextColor(theme.C("ink")).Role(ui.RoleHeading).Level(3)
		build()
	})
}

// choice is one choice of two or three for choiceCards.
type choice struct {
	value, label, hint string
	// picture is drawn over the label (the theme's windows); nil for none.
	picture func(c *ui.Context)
}

// choiceCards is one choice of two or three, as cards side by side: radio
// buttons, so Tab reaches the chosen one and the arrow keys choose (each
// choice saves at once, onChange). The chosen card takes an ink ring and a
// filled dot; the dot keeps the choice readable without the ring's weight.
// The group is named by group (the heading above it).
func choiceCards(c *ui.Context, id, group, value string, choices []choice, disabled bool, onChange func(value string)) {
	chosen := value
	ui.RadioGroup(c.Key(id), func() {
		for _, ch := range choices {
			choiceCard(c, ch, &chosen, disabled)
		}
	}).Row().FillWidth().Gap(8).AlignItems(ui.Stretch).Label(group)
	if chosen != value {
		onChange(chosen)
	}
}

func choiceCard(c *ui.Context, ch choice, chosen *string, disabled bool) {
	checked := *chosen == ch.value
	card := ui.RadioBase(c.Key("card:"+ch.value), chosen, ch.value).
		Grow(1).Basis(0).MinWidth(0).Padding(12).Radius(theme.Current().Radius["md"]).
		Background(theme.C("surface")).Label(ch.label).Description(ch.hint).Cursor(ui.CursorPointer)
	if !disabled && card.Hovered() {
		card.Background(theme.C("hover"))
	}
	if checked {
		card.Shadow(0, 0, 0, 2, theme.C("ink"))
	}
	if disabled {
		card.Disabled(true)
	}
	card.Children(func() {
		ui.Column(c.Key("body")).Grow(1).MinWidth(0).Gap(10).Children(func() {
			if ch.picture != nil {
				ch.picture(c)
			}
			ui.Row(c.Key("line")).Gap(10).AlignItems(ui.Start).Children(func() {
				dot := ui.Row(c.Key("dot")).Size(16, 16).Shrink(0).Margin(2, 0, 0, 0).Radius(8).Center().
					Border(1, theme.C("control-line"))
				if checked {
					dot.Border(1, theme.C("ink"))
				}
				dot.Children(func() {
					if checked {
						ui.Box(c.Key("fill")).Size(8, 8).Radius(4).Background(theme.C("ink"))
					}
				})
				ui.Column(c.Key("words")).Shrink(1).MinWidth(0).Children(func() {
					theme.Text(c, ch.label, "ui").FontWeight(500).TextColor(theme.C("ink"))
					theme.Text(c, ch.hint, "sub").TextColor(theme.C("ink-muted"))
				})
			})
		})
	})
}

// switchRow is an on/off choice: its name and one line on what it does, the
// switch at the end (named by the label and described by the hint).
// children builds what sits under it while it applies (crossfade's length).
func switchRow(c *ui.Context, id, label, hint string, checked, disabled bool, onChange func(on bool), children func()) {
	ui.Column(c.Key("row:"+id)).FillWidth().Padding(12, 0).Children(func() {
		ui.Row(c.Key("head")).FillWidth().Gap(24).AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
			ui.Column(c.Key("words")).Grow(1).Shrink(1).MinWidth(0).Children(func() {
				theme.Text(c, label, "ui").FontWeight(500).TextColor(theme.C("ink"))
				theme.Text(c, hint, "sub").TextColor(theme.C("ink-muted"))
			})
			on := checked
			sw := kit.Switch(c.Key("switch"), &on, kit.SwitchOpts{A11yLabel: label, Disabled: disabled}).Description(hint)
			if sw.Changed() && on != checked {
				onChange(on)
			}
		})
		if children != nil {
			children()
		}
	})
}

// list lays rows out as one list, divided by the structural line
// (SwitchList, and the divide-y lists of the welcome and the last step).
func list(c *ui.Context, id string, rows []func()) {
	if len(rows) == 0 {
		return
	}
	ui.Column(c.Key("list:" + id)).FillWidth().Children(func() {
		for i, row := range rows {
			if i > 0 {
				ui.Box(c.Key(i)).Height(1).FillWidth().Background(theme.C("line"))
			}
			ui.Column(c.Key(-1 - i)).FillWidth().Children(row)
		}
	})
}

// well is a flat block on the surface tone, for a step's one status
// (Widevine, the sign-in); build its content in the returned row's Children.
func well(c *ui.Context) ui.Element {
	return ui.Column(c).FillWidth().Padding(12, 16).Radius(theme.Current().Radius["md"]).Background(theme.C("surface"))
}

// loading is a step's loading state: a surface block at about the height of
// what will land, a busy status read as "Loading".
func loading(c *ui.Context, height float32) ui.Element {
	return ui.Box(c.Key("loading")).FillWidth().Height(height).Radius(theme.Current().Radius["md"]).
		Background(theme.C("surface")).Role(ui.RoleStatus).Busy(true).Label(i18n.T("Loading"))
}

// statusIcon is a small coloured icon beside text in a status line.
func statusIcon(c *ui.Context, name, colour string) ui.Element {
	return ui.Icon(c.Key("icon"), theme.Icon(name)).FontSize(16).TextColor(theme.C(colour)).Shrink(0)
}
