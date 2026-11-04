package main

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// text is a Text in one of the theme's type styles ("body", "meta"…).
func text(c *ui.Context, s, style string) ui.Element {
	t := theme.Current().Type[style]
	return ui.Text(c, s).FontSize(t.Size).FixedLineHeight(t.LineHeight).FontWeight(int(t.Weight)).LetterSpacing(t.Tracking)
}

// heading starts a block of a section.
func heading(c *ui.Context, s string) {
	text(c, s, "heading").TextColor(theme.C("ink")).Margin(24, 0, 4)
}

// note is a line of explanation.
func note(c *ui.Context, s string) {
	text(c, s, "secondary").TextColor(theme.C("ink-muted"))
}

// labelled is a small caption over a component.
func labelled(c *ui.Context, label string, fn func()) {
	ui.Column(c).Gap(6).Children(func() {
		text(c, label, "meta").TextColor(theme.C("ink-faint"))
		fn()
	})
}

// knob is a labelled control of a section's playground.
func knob(c *ui.Context, label string, fn func()) {
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		text(c, label, "label").TextColor(theme.C("ink-muted")).Width(96)
		fn()
	})
}

// panel is the surface a playground sits on.
func panel(c *ui.Context, fn func()) {
	ui.Column(c).Gap(12).Padding(16).Radius(theme.Current().Radius["lg"]).Background(theme.C("surface")).
		Border(1, theme.C("line")).Children(fn)
}
