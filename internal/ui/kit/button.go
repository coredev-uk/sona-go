// Package kit is Sona's component kit (docs/ui-architecture.md, "The kit"):
// MyGo widgets and elements in Sona's look, built from the theme's tokens.
// PR 0.2 ships the two the foundation is checked with, Button and Artwork
// (with the ArtLoader that feeds it); the phase 1 kit PRs add the rest.
package kit

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// ButtonVariant is a button's look (v0.2.0's shared/ui/button.tsx).
type ButtonVariant int

const (
	// Default is ink-filled: near-black on light, near-white on dark.
	Default ButtonVariant = iota
	// Ghost has no face until hovered.
	Ghost
	// Outline has a control-line border and no fill.
	Outline
	// Play is the round amber button: "where the music is".
	Play
	// Destructive is an outline in --danger, never a red fill.
	Destructive
)

// ButtonSize is a button's size; one with no label is a square.
type ButtonSize int

const (
	// SizeMD is 36 high (shadcn's default).
	SizeMD ButtonSize = iota
	// SizeSM is 32 high.
	SizeSM
	// SizeXS is 24 high with small type.
	SizeXS
)

// ButtonOpts are the options of Button.
type ButtonOpts struct {
	Variant ButtonVariant
	Size    ButtonSize
	// Icon is a Lucide name (theme.Icon), shown before the label.
	Icon string
	// Pressed makes a toggle button: it shows the active tone while true
	// and tells assistive technology its state. The caller flips it on
	// Clicked.
	Pressed *bool
	// Disabled grays the button out: it takes no click and no focus.
	Disabled bool
	// A11yLabel names an icon-only button (no label) for assistive
	// technology and tests; it is also its tooltip.
	A11yLabel string
}

// sizing is a ButtonSize's metrics, in DIPs (button.tsx's h-9, h-8, h-6).
type sizing struct {
	height, padX, padXIcon, gap, font, icon float32
}

var sizes = [...]sizing{
	SizeMD: {height: 36, padX: 16, padXIcon: 12, gap: 8, font: 14, icon: 16},
	SizeSM: {height: 32, padX: 12, padXIcon: 10, gap: 6, font: 14, icon: 16},
	SizeXS: {height: 24, padX: 8, padXIcon: 6, gap: 4, font: 12, icon: 12},
}

// Button is a push button in Sona's look. Click it as MyGo's: the returned
// element's Clicked reports a click, or Enter or Space while it has the
// keyboard focus.
//
//	if kit.Button(c.Key("more"), "", kit.ButtonOpts{Icon: "ellipsis", Variant: kit.Ghost, Size: kit.SizeSM, A11yLabel: i18n.T("More")}).Clicked() {…}
//
// The face changes over 120 ms (the theme's "feedback" motion) unless motion
// is reduced. With no label the button is a square showing only its icon:
// give it an A11yLabel.
func Button(c *ui.Context, label string, o ButtonOpts) ui.Element {
	sz := sizes[o.Size]
	e := ui.ButtonBase(c)
	hovered := e.Hovered() && !o.Disabled
	pressed := o.Pressed != nil && *o.Pressed

	bg, border, fg := variantColors(o.Variant, hovered, pressed)
	iconOnly := label == ""
	width := float32(0)
	padX := sz.padX
	if o.Icon != "" {
		padX = sz.padXIcon
	}
	if iconOnly {
		width, padX = sz.height, 0
	}

	e.Height(sz.height).Gap(sz.gap).PaddingX(padX).Radius(theme.Current().Radius["md"]).
		Background(bg).TextColor(fg)
	if iconOnly {
		e.Width(width)
	}
	if o.Variant == Play {
		e.Radius(theme.Current().Radius["full"])
		if iconOnly {
			e.Size(sz.height, sz.height)
		}
	}
	if border.A > 0 {
		e.Border(1, border)
	}
	if o.Disabled {
		e.Disabled(true).Opacity(0.5)
	}
	if !theme.ReducedMotion() {
		m := theme.Current().Motion["feedback"]
		e.Transition(ui.ElementTransition{Colors: true, Duration: m.Duration, Ease: m.Ease})
	}
	if o.Pressed != nil {
		e.Role(ui.RoleToggleButton).Checked(pressed)
	}
	name := o.A11yLabel
	if name == "" {
		name = label
	}
	if iconOnly && name != "" {
		e.Label(name).Tooltip(name)
	}

	e.Children(func() {
		if o.Icon != "" {
			ui.Icon(c, theme.Icon(o.Icon)).FontSize(sz.icon)
		}
		if !iconOnly {
			ui.Text(c, label).FontSize(sz.font).FixedLineHeight(theme.Current().Type["label"].LineHeight).
				FontWeight(int(theme.Current().Type["label"].Weight)).SingleLine()
		}
	})
	return e
}

// variantColors are a variant's face: its fill, its border (transparent for
// none) and the colour of its text and icon, given the pointer and a toggle.
func variantColors(v ButtonVariant, hovered, pressed bool) (bg, border, fg ui.Color) {
	clear := ui.Transparent
	c := theme.C
	switch v {
	case Ghost:
		bg, border, fg = clear, clear, c("ink")
		if hovered {
			bg = c("accent-soft")
		}
	case Outline:
		bg, border, fg = clear, c("control-line"), c("ink")
		if hovered {
			bg = c("hover")
		}
	case Play:
		bg, border, fg = c("play"), clear, c("on-play")
		if hovered {
			bg = c("play").Alpha(0.9)
		}
	case Destructive:
		bg, border, fg = clear, c("danger-line"), c("danger")
		if hovered {
			bg = c("danger-wash")
		}
	default:
		bg, border, fg = c("primary"), clear, c("primary-foreground")
		if hovered {
			bg = c("primary").Alpha(0.9)
		}
	}
	if pressed {
		bg = c("active")
		if v == Default || v == Play {
			fg = c("ink")
		}
	}
	return bg, border, fg
}
