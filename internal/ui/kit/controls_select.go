package kit

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// controlsPopupPanel styles the panel of a Select's or Combobox's options: a
// popover with 4px inside, a border and the floating shadow, 4px off its
// field.
func controlsPopupPanel(panel ui.Element) ui.Element {
	sh := theme.Current().Shadow["float"]
	return panel.Margin(4, 0, 4, 0).Padding(4).Radius(controlsRadius("md")).
		Background(theme.C("popover")).Border(1, theme.C("border")).Shadow(sh.X, sh.Y, sh.Blur, sh.Spread, sh.Color)
}

// controlsPopupHeight is how tall a popup's list may be: the room below its
// anchor, or above it where that is more, less the panel's own padding
// (items past it scroll). The first frame, with no bounds yet, gets 320.
func controlsPopupHeight(c *ui.Context, anchor ui.Element) float32 {
	_, wh := c.Size()
	b := anchor.Bounds()
	if b.H == 0 {
		return 320
	}
	room := max(wh-(b.Y+b.H), b.Y)
	return max(room-4-4-8-2-8, 96) // margin, padding, border and the window's edge
}

// controlsOption styles an option row of a popup (SelectItem's classes): 8px
// left, 32 right for the check at its end, 4px radius, accent tint while
// highlighted, 50% when disabled. It builds its label and the check.
func controlsOption(c *ui.Context, item ui.Element, label string, highlighted, chosen, disabled bool) {
	item.Padding(6, 8, 6, 8).Gap(8).Radius(controlsRadius("sm")).AlignItems(ui.Center).FillWidth()
	if highlighted && !disabled {
		// The tint alone is about 1.1:1 on the popover: the inset ring is the
		// keyboard highlight (docs/accessibility.md, "Focus").
		ring, radius := theme.C("ring"), controlsRadius("sm")
		item.Background(theme.C("accent")).TextColor(theme.C("accent-foreground")).
			DrawOver(func(p *ui.Painter, r ui.Rect) { p.Stroke(r, ring, radius, 2) })
	}
	if disabled {
		item.Disabled(true)
	}
	item.Children(func() {
		controlsText(c, label, "body").SingleLine().Grow(1)
		if chosen {
			ui.Icon(c, theme.Icon("check")).FontSize(16).Shrink(0)
		} else {
			ui.Box(c).Size(16, 16).Shrink(0)
		}
	})
}

// selectRadio makes an option of a select a radio item, checked while chosen
// (a screen reader reads "Pro, radio item, checked" and presses it).
func selectRadio(item ui.Element, label string, chosen bool) ui.Element {
	return item.Role(ui.RoleMenuItemRadio).Label(label).Checked(chosen)
}

// SelectItem is one row of a Select's list: an option, a group label or a
// separator.
type SelectItem struct {
	Value, Label string
	Disabled     bool
	kind         selectKind
}

type selectKind int

const (
	selectOption selectKind = iota
	selectLabel
	selectSeparator
)

// SelectOption is a choosable row: Value is what *value gets, Label what
// shows.
func SelectOption(value, label string) SelectItem { return SelectItem{Value: value, Label: label} }

// SelectLabel is a muted heading for the options after it.
func SelectLabel(label string) SelectItem { return SelectItem{Label: label, kind: selectLabel} }

// SelectSeparator is a line between groups of options.
func SelectSeparator() SelectItem { return SelectItem{kind: selectSeparator} }

// SelectOpts are the options of Select.
type SelectOpts struct {
	// Small makes the trigger 32 high instead of 36.
	Small bool
	// Placeholder shows (muted) while *value is none of the options.
	Placeholder string
	// A11yLabel names the select for assistive technology and tests.
	A11yLabel string
	Disabled  bool
	// Invalid draws the destructive border and tells assistive technology
	// the choice is not valid, reading Error ("Invalid" when empty).
	Invalid bool
	Error   string
	// Width fixes the trigger's width; 0 fits its text.
	Width float32
}

// Select is a drop-down choosing the Value of one of items into *value: the
// trigger shows the choice and a chevron; a click, or Enter, Space or an
// arrow with the focus on it, opens the list, where the arrows, Home and
// End move, Enter, Space or a click chooses, and Escape closes and returns
// the focus to the trigger. A long list scrolls. The chosen row ends with a
// check. Changed reports a new choice. (shared/ui/select.tsx)
func Select(c *ui.Context, value *string, items []SelectItem, o SelectOpts) ui.Element {
	sel := ui.SelectBase(c, value)
	h := float32(controlsFieldHeight)
	if o.Small {
		h = 32
	}
	trigger := sel.Trigger
	trigger.Height(h).PaddingX(12).Gap(8).Justify(ui.SpaceBetween).AlignItems(ui.Center)
	if o.Width > 0 {
		trigger.Width(o.Width)
	}
	if o.A11yLabel != "" {
		trigger.Label(o.A11yLabel)
	}
	shown, placeholder := "", true
	for _, it := range items {
		if it.kind == selectOption && it.Value == *value {
			shown, placeholder = it.Label, false
		}
	}
	if placeholder {
		shown = o.Placeholder
	}
	trigger.Value(shown)
	controlsFieldFace(trigger, trigger.Focused(), o.Invalid, o.Disabled)
	invalidFor(trigger, o.Invalid, o.Error)
	if trigger.Hovered() && !o.Disabled && theme.Dark() {
		trigger.Background(theme.C("input").Alpha(0.5))
	}
	trigger.Children(func() {
		t := controlsText(c, shown, "body").SingleLine().Shrink(1).MinWidth(0)
		if placeholder {
			t.TextColor(theme.C("muted-foreground"))
		}
		ui.Icon(c, theme.Icon("chevron-down")).FontSize(16).Opacity(0.5).Shrink(0)
	})
	width := trigger.Bounds().W
	sel.Popup(func(panel ui.Element) {
		controlsPopupPanel(panel).MinWidth(max(width, 128)).Modal().Label(o.A11yLabel)
		ui.Scroll(c).MaxHeight(controlsPopupHeight(c, trigger)).Role(ui.RoleGroup).Children(func() {
			for i, it := range items {
				switch it.kind {
				case selectLabel:
					controlsText(c.Key(i), it.Label, "meta").TextColor(theme.C("muted-foreground")).Padding(6, 8)
				case selectSeparator:
					ui.Box(c.Key(i)).Height(1).FillWidth().Margin(4, -4, 4, -4).Background(theme.C("border"))
				case selectOption:
					if it.Disabled {
						// Not an Item: the arrows and Enter pass it by.
						controlsOption(c, selectRadio(ui.Row(c.Key(i)), it.Label, false), it.Label, false, false, true)
						continue
					}
					item := sel.Item(it.Value)
					controlsOption(c, selectRadio(item, it.Label, it.Value == *value), it.Label, item.Highlighted(), it.Value == *value, false)
				}
			}
		})
	})
	return trigger
}

// SegmentOption is one segment of a Segmented control.
type SegmentOption[V comparable] struct {
	Value V
	Label string
}

// Segmented is a short row of exclusive choices shown together (two to
// four): the chosen one is raised, the others muted. For longer lists use
// Select. label names the group for assistive technology. As in v0.2.0 it is
// a group of toggle buttons, each its own stop of Tab and pressed while
// chosen; Enter or Space or a click chooses, the arrows do not move the
// choice. Changed reports a click on any segment, the chosen one too, as
// v0.2.0's onChange did. (shared/components/Segmented.tsx)
func Segmented[V comparable](c *ui.Context, value *V, options []SegmentOption[V], label string) ui.Element {
	track := ui.Row(c).Height(32).Padding(2).Radius(controlsRadius("md")).Background(theme.C("hover")).
		AlignItems(ui.Center).Shrink(0).Role(ui.RoleGroup).Label(label)
	clicked := false
	track.Children(func() {
		for i, o := range options {
			on := o.Value == *value
			s := ui.ButtonBase(c.Key(i)).Height(28).PaddingX(12).Radius(5).Role(ui.RoleToggleButton).Checked(on).Label(o.Label)
			fg := theme.C("ink-muted")
			switch {
			case on:
				s.Background(theme.C("raised"))
				fg = theme.C("ink")
			case s.Hovered():
				fg = theme.C("ink")
			}
			s.TextColor(fg)
			if !theme.ReducedMotion() {
				m := theme.Current().Motion["press"]
				s.Transition(ui.ElementTransition{Colors: true, Duration: m.Duration, Ease: m.Ease})
			}
			s.Children(func() { controlsText(c, o.Label, "secondary").FontWeight(500).SingleLine() })
			if s.Clicked() {
				clicked = true
				*value = o.Value
			}
		}
	})
	if clicked {
		track.SetChanged()
	}
	return track
}
