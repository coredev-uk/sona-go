package kit

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// TextAreaOpts are the options of TextArea.
type TextAreaOpts struct {
	Placeholder string
	// A11yLabel names the field for assistive technology and tests.
	A11yLabel string
	// Rows is how many lines it shows (3 when zero); longer text scrolls.
	Rows     int
	Invalid  bool
	Disabled bool
}

// TextArea is a multi-line text field editing *value: Input's face (control-line
// border, ring-coloured while focused) over MyGo's text area, Rows lines high
// and filling its parent's width. Enter inserts a line break; Changed reports
// an edit.
func TextArea(c *ui.Context, value *string, o TextAreaOpts) ui.Element {
	rows := o.Rows
	if rows <= 0 {
		rows = 3
	}
	e := ui.TextAreaBase(c, value)
	t := theme.Current().Type["body"]
	e.FillWidth().Lines(rows, rows).Padding(8, 12).FontSize(t.Size).FixedLineHeight(t.LineHeight).
		Placeholder(o.Placeholder).PlaceholderColor(theme.C("muted-foreground")).
		TextColor(theme.C("foreground"))
	if o.A11yLabel != "" {
		e.Label(o.A11yLabel)
	}
	controlsFieldFace(e, e.Focused(), o.Invalid, o.Disabled)
	return e
}
