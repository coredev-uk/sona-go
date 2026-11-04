package kit

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// InputOpts are the options of Input.
type InputOpts struct {
	Placeholder string
	// A11yLabel names the field for assistive technology and tests.
	A11yLabel string
	// Invalid draws the destructive border and ring and tells assistive
	// technology the value is not valid (aria-invalid).
	Invalid bool
	// Error is the message assistive technology reads with an invalid value
	// (aria-describedby on the error text): the one the page shows. "Invalid"
	// when empty.
	Error    string
	Disabled bool
	// Password hides what is typed.
	Password bool
}

// invalidFor marks e as not valid for assistive technology, reading msg.
func invalidFor(e ui.Element, invalid bool, msg string) {
	if !invalid {
		return
	}
	if msg == "" {
		msg = i18n.T("Invalid")
	}
	e.Error(msg)
}

// controlsFieldHeight is the height of Input, Select and the like
// (shared/ui/input.tsx's h-9).
const controlsFieldHeight = 36

// controlsFieldFace styles the box of a field: control-line border,
// transparent fill (a faint control-line wash in dark), the ring colour on
// the border while focused, danger when invalid, 120 ms colour ease.
func controlsFieldFace(e ui.Element, focused, invalid, disabled bool) {
	border := theme.C("input")
	switch {
	case invalid:
		border = theme.C("destructive")
	case focused:
		border = theme.C("ring")
	}
	bg := ui.Transparent
	if theme.Dark() {
		bg = theme.C("input").Alpha(0.3)
	}
	e.Radius(controlsRadius("md")).Background(bg).Border(1, border)
	if invalid {
		ring := theme.C("destructive").Alpha(0.2)
		if theme.Dark() {
			ring = theme.C("destructive").Alpha(0.4)
		}
		e.Shadow(0, 0, 0, 2, ring)
	}
	if !theme.ReducedMotion() {
		m := theme.Current().Motion["feedback"]
		e.Transition(ui.ElementTransition{Colors: true, Duration: m.Duration, Ease: m.Ease})
	}
	controlsDisabled(e, disabled)
}

// Input is a single-line text field editing *value: 36px high, a
// control-line border, a 2px ring and ring-coloured border while focused
// (shared/ui/input.tsx). Handle it as MyGo's TextInput: Changed, Submitted.
func Input(c *ui.Context, value *string, o InputOpts) ui.Element {
	e := ui.TextInputBase(c, value)
	if o.Password {
		e.Password()
	}
	t := theme.Current().Type["body"]
	e.Height(controlsFieldHeight).Padding(7, 12).FontSize(t.Size).FixedLineHeight(t.LineHeight).
		Placeholder(o.Placeholder).PlaceholderColor(theme.C("muted-foreground")).
		TextColor(theme.C("foreground"))
	if o.A11yLabel != "" {
		e.Label(o.A11yLabel)
	}
	controlsFieldFace(e, e.Focused(), o.Invalid, o.Disabled)
	invalidFor(e, o.Invalid, o.Error)
	return e
}

// SearchFieldOpts are the options of SearchField.
type SearchFieldOpts struct {
	// Placeholder says what is searched; "Search" when empty.
	Placeholder string
	// A11yLabel names the field; the placeholder when empty.
	A11yLabel string
	Disabled  bool
	// Width fixes the field's width; 0 fills the parent.
	Width float32
}

// SearchField is a field for searching, editing *query: a magnifying glass,
// the text, and a button clearing it while it holds any, as Escape does
// then. Handle the returned text input as MyGo's SearchField: Changed
// reports a change, Submitted Enter.
func SearchField(c *ui.Context, query *string, o SearchFieldOpts) ui.Element {
	placeholder := o.Placeholder
	if placeholder == "" {
		placeholder = i18n.T("Search")
	}
	box := ui.Row(c).Height(32).PaddingX(12).Gap(8).AlignItems(ui.Center).Cursor(ui.CursorText)
	if o.Width > 0 {
		box.Width(o.Width).Shrink(0)
	} else {
		box.FillWidth()
	}
	var in ui.Element
	box.Children(func() {
		ui.Icon(c, theme.Icon("search")).FontSize(16).TextColor(theme.C("ink-faint"))
		t := theme.Current().Type["body"]
		in = ui.TextInputBase(c.Key("text"), query).Grow(1).MinWidth(0).FontSize(t.Size).FixedLineHeight(t.LineHeight).
			Placeholder(placeholder).PlaceholderColor(theme.C("muted-foreground")).TextColor(theme.C("foreground"))
		name := o.A11yLabel
		if name == "" {
			name = placeholder
		}
		in.Label(name)
		if *query != "" {
			clear := ui.ButtonBase(c.Key("clear")).Size(20, 20).Radius(controlsRadius("full")).KeepFocus().Label(i18n.T("Clear search"))
			if clear.Hovered() {
				clear.Background(theme.C("hover"))
			}
			clear.Children(func() {
				ui.Icon(c, theme.Icon("x")).FontSize(12).TextColor(theme.C("muted-foreground"))
			})
			if clear.Clicked() {
				*query = ""
				in.SetChanged().Focus()
			}
		}
		if *query != "" && in.Shortcut(0, ui.KeyEscape) {
			*query = ""
			in.SetChanged()
		}
	})
	if box.Clicked() {
		in.Focus()
	}
	controlsFieldFace(box, in.Focused(), false, o.Disabled)
	return in
}

// ComboboxOpts are the options of Combobox.
type ComboboxOpts struct {
	Placeholder string
	A11yLabel   string
	Disabled    bool
	// Width fixes the field's width; 0 fills the parent.
	Width float32
}

// Combobox chooses *selected among options: a field showing the choice,
// whose text filters the options below it as the user types (those
// containing it, those starting with it first), while a click or Down shows
// them all. Up and Down move among them, Enter or a click chooses, Escape
// closes, and the text goes back to the choice when the field loses the
// focus. The returned field reports Changed when the choice changes.
func Combobox(c *ui.Context, selected *string, options []string, o ComboboxOpts) ui.Element {
	box := ui.Row(c).Height(controlsFieldHeight).PaddingX(12).Gap(8).AlignItems(ui.Center).Cursor(ui.CursorText)
	if o.Width > 0 {
		box.Width(o.Width).Shrink(0)
	} else {
		box.FillWidth()
	}
	text := ui.Local(box, "text", func() string { return *selected })
	var in ui.Element
	var cb ui.ComboboxParts
	box.Children(func() {
		cb = ui.ComboboxBase(c.Key("input"), text)
		in = cb.Input
		t := theme.Current().Type["body"]
		in.Grow(1).MinWidth(0).FontSize(t.Size).FixedLineHeight(t.LineHeight).
			Placeholder(o.Placeholder).PlaceholderColor(theme.C("muted-foreground")).TextColor(theme.C("foreground"))
		if o.A11yLabel != "" {
			in.Label(o.A11yLabel)
		}
		// Not a button: it is no stop of Tab and no element to assistive
		// technology (the field is the combo box); the pointer still opens it.
		toggle := ui.Row(c.Key("toggle")).Size(20, 20).Center().Shrink(0).KeepFocus()
		toggle.Children(func() {
			ui.Icon(c, theme.Icon("chevron-down")).FontSize(16).Opacity(0.5)
		})
		if toggle.Clicked() && !o.Disabled {
			cb.SetOpen(!cb.Open())
			in.Focus()
		}
	})
	if box.Clicked() {
		in.Focus()
	}
	shown := options
	if cb.Filtering() {
		shown = controlsMatching(options, *text)
	}
	width := box.Bounds().W
	cb.Popup(func(panel ui.Element) {
		controlsPopupPanel(panel).AttachTo(box, ui.AnchorBottomStart, ui.AnchorTopStart).MinWidth(width)
		ui.Scroll(c).MaxHeight(controlsPopupHeight(c, box)).Children(func() {
			for _, opt := range shown {
				item := cb.Item(opt)
				controlsOption(c, item, opt, item.Highlighted(), opt == *selected, false)
			}
		})
	})
	if v, ok := cb.Chosen(); ok {
		controlsCommit(box, selected, text, v)
	} else if in.Submitted() && cb.Filtering() && len(shown) > 0 {
		controlsCommit(box, selected, text, shown[0])
		cb.SetOpen(false)
	} else if !in.Focused() && !cb.Open() && *text != *selected {
		*text = *selected // what was typed matched no choice
	}
	controlsFieldFace(box, in.Focused(), false, o.Disabled)
	return box
}

// controlsCommit makes v the choice, and its text the field's.
func controlsCommit(box ui.Element, selected, text *string, v string) {
	if *selected != v {
		*selected = v
		box.SetChanged()
	}
	*text = v
}

// controlsMatching are the options containing query, ignoring case, those
// starting with it first; all of them for a blank query.
func controlsMatching(options []string, query string) []string {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return options
	}
	var prefix, rest []string
	for _, o := range options {
		switch l := strings.ToLower(o); {
		case strings.HasPrefix(l, q):
			prefix = append(prefix, o)
		case strings.Contains(l, q):
			rest = append(rest, o)
		}
	}
	return append(prefix, rest...)
}
