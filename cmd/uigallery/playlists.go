package main

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

var textAreaState struct{ text, empty, invalid, disabled string }

func init() {
	register(section{name: "Controls / TextArea", build: func(c *ui.Context, g *gallery) {
		s := &textAreaState
		controlsNote(c, "kit.TextArea is a multi-line field on Input's face: three rows by default, Enter inserts a line break (the playlist dialog's description).")
		ui.Column(c).Gap(10).Width(360).Children(func() {
			kit.TextArea(c.Key("a"), &s.text, kit.TextAreaOpts{A11yLabel: "Description"})
			kit.TextArea(c.Key("b"), &s.empty, kit.TextAreaOpts{A11yLabel: "Placeholder", Placeholder: "Optional", Rows: 2})
			kit.TextArea(c.Key("c"), &s.invalid, kit.TextAreaOpts{A11yLabel: "Invalid", Invalid: true, Rows: 2})
			kit.TextArea(c.Key("d"), &s.disabled, kit.TextAreaOpts{A11yLabel: "Disabled", Disabled: true, Placeholder: "Disabled", Rows: 2})
		})
	}})
}
