// Package kit is Sona's component kit (docs/ui-architecture.md, "The kit"):
// MyGo widgets and elements in Sona's look, built from the theme's tokens.
// PR 0.2 ships the two the foundation is checked with, Button and Artwork
// (with the ArtLoader that feeds it); the phase 1 kit PRs add the rest.
package kit

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
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
	// Secondary is a --secondary (hover-toned) fill.
	Secondary
	// Link is ink text that underlines under the pointer, with no face.
	Link
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
	// SizeLG is 40 high (button.tsx's lg and icon-lg).
	SizeLG
)

// ButtonOpts are the options of Button.
type ButtonOpts struct {
	Variant ButtonVariant
	Size    ButtonSize
	// Icon is a Lucide name (theme.Icon), shown before the label.
	Icon string
	// IconAfter puts the icon after the label (the external-link mark of a
	// button that leaves the app).
	IconAfter bool
	// Pressed makes a toggle button: it shows the active tone while true
	// and tells assistive technology its state. The caller flips it on
	// Clicked.
	Pressed *bool
	// Disabled grays the button out: it takes no click and no focus.
	Disabled bool
	// A11yLabel names an icon-only button (no label) for assistive
	// technology and tests; it is also its tooltip unless Tip says another.
	A11yLabel string
	// Reason makes the button one that does not apply right now
	// (aria-disabled): it is unavailable to assistive technology
	// (AccessDisabled) but stays enabled, so that it keeps its place in the
	// Tab order and can say why, drawn at half strength, with the reason as
	// its description and the tooltip's second line. The caller ignores its
	// click.
	Reason string
	// Tip is the tooltip when it should differ from A11yLabel ("Play" for a
	// button named "Play OK Computer"). A button with a label has none
	// unless it is given one.
	Tip string
	// Keys are the chords the control answers to (Env.KeysOf): the first
	// is drawn in the tooltip as keycaps and read in the description.
	Keys []string
	// Filled draws the icon solid, as the play glyph of the amber disc and
	// a favourite star are: only icons with a solid form (play, pause,
	// star, thumbs-down) change.
	Filled bool
	// Muted makes a ghost button's ink --ink-muted until the pointer is on
	// it (the rating buttons of a header).
	Muted bool
	// Quiet keeps a pressed toggle's colours: Pressed only tells assistive
	// technology and the Filled icon says it (the Favourite star), with no
	// active fill.
	Quiet bool
	// Spin turns the icon once a second while the button is busy (a
	// "loader-circle"); reduced motion leaves it still.
	Spin bool
}

// sizing is a ButtonSize's metrics, in DIPs (button.tsx's h-9, h-8, h-6).
type sizing struct {
	height, padX, padXIcon, gap, font, icon float32
}

var sizes = [...]sizing{
	SizeMD: {height: 36, padX: 16, padXIcon: 12, gap: 8, font: 14, icon: 16},
	SizeSM: {height: 32, padX: 12, padXIcon: 10, gap: 6, font: 14, icon: 16},
	SizeXS: {height: 24, padX: 8, padXIcon: 6, gap: 4, font: 12, icon: 12},
	SizeLG: {height: 40, padX: 24, padXIcon: 16, gap: 8, font: 14, icon: 16},
}

// playGlyph is the size of the solid play glyph of a Play button at SizeLG
// (PlayButtons.tsx's size-[18px]).
const playGlyph = 18

// Button is a push button in Sona's look. Click it as MyGo's: the returned
// element's Clicked reports a click, or Enter or Space while it has the
// keyboard focus.
//
//	if kit.Button(c.Key("more"), "", kit.ButtonOpts{Icon: "ellipsis", Variant: kit.Ghost, Size: kit.SizeSM, A11yLabel: i18n.T("More")}).Clicked() {…}
//
// The face changes over 120 ms (the theme's "feedback" motion) unless motion
// is reduced. With no label the button is a square showing only its icon:
// give it an A11yLabel. IconButton and PlayButton are the common cases.
func Button(c *ui.Context, label string, o ButtonOpts) ui.Element {
	sz := sizes[o.Size]
	e := ui.ButtonBase(c)
	hovered := e.Hovered() && !o.Disabled
	pressed := o.Pressed != nil && *o.Pressed

	bg, border, fg := variantColors(o, hovered, pressed)
	iconOnly := label == ""
	padX := sz.padX
	if o.Icon != "" {
		padX = sz.padXIcon
	}
	if iconOnly {
		padX = 0
	}

	e.Height(sz.height).Gap(sz.gap).PaddingX(padX).Radius(theme.Current().Radius["md"]).
		Background(bg).TextColor(fg)
	if iconOnly {
		e.Size(sz.height, sz.height)
	}
	if o.Variant == Play {
		e.Radius(theme.Current().Radius["full"])
	}
	if border.A > 0 {
		e.Border(1, border)
	}
	if o.Disabled {
		e.Disabled(true) // MyGo paints a disabled element at 50%
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
	tip := o.Tip
	if tip == "" && iconOnly {
		tip = name
	}
	if o.A11yLabel != "" || iconOnly && name != "" {
		e.Label(name)
	}

	e.Children(func() {
		icon := func() {
			if o.Icon == "" {
				return
			}
			size := sz.icon
			if o.Variant == Play && o.Size == SizeLG {
				size = playGlyph
			}
			ic := ui.Icon(c, controlsIcon(o.Icon, o.Filled)).FontSize(size)
			if o.Spin && !theme.ReducedMotion() {
				ic.Rotate(ic.Loop("spin", time.Second, ui.Linear) * 360)
			}
			if o.Variant == Play && o.Icon == "play" {
				ic.Margin(0, 0, 0, 1) // the triangle's weight sits left of its box
			}
		}
		if !o.IconAfter {
			icon()
		}
		if !iconOnly {
			t := controlsText(c, label, "label").FontSize(sz.font).SingleLine()
			if o.Variant == Link && hovered {
				t.Underline()
			}
		}
		if o.IconAfter {
			icon()
		}
	})
	if o.Reason != "" {
		e.Opacity(0.5).AccessDisabled(true).Description(o.Reason)
		controlsTooltip(c, e, name+"\n"+o.Reason, false)
		return e
	}
	controlsTooltipKeys(c, e, tip, tip != name, o.Keys)
	return e
}

// variantColors are a button's face: its fill, its border (transparent for
// none) and the colour of its text and icon, given the pointer and a toggle.
func variantColors(o ButtonOpts, hovered, pressed bool) (bg, border, fg ui.Color) {
	clear := ui.Transparent
	c := theme.C
	switch o.Variant {
	case Ghost:
		bg, border, fg = clear, clear, c("ink")
		if o.Muted && !pressed && !hovered {
			fg = c("ink-muted")
		}
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
		if o.Disabled {
			// button.tsx keeps the disc at full opacity: --hover with faint
			// ink. MyGo paints every disabled element at 50% over what is
			// behind it, so the colours are chosen to come out as those over
			// the page's ground.
			bg, fg = controlsUndim(c("hover")), controlsUndim(c("ink-faint"))
		}
	case Destructive:
		bg, border, fg = clear, c("danger-line"), c("danger")
		if hovered {
			bg = c("danger-wash")
		}
	case Secondary:
		bg, border, fg = c("secondary"), clear, c("secondary-foreground")
		if hovered {
			bg = c("secondary").Alpha(0.8)
		}
	case Link:
		bg, border, fg = clear, clear, c("primary")
	default:
		bg, border, fg = c("primary"), clear, c("primary-foreground")
		if hovered {
			bg = c("primary").Alpha(0.9)
		}
	}
	if pressed && !o.Quiet {
		bg = c("active")
		if o.Variant == Default || o.Variant == Play {
			fg = c("ink")
		}
	}
	return bg, border, fg
}

// IconButton is an icon-only Button: icon is a Lucide name, label names it
// for assistive technology and is its tooltip. The size is o.Size's square
// (SizeXS 24, SizeSM 32, SizeMD 36, SizeLG 40).
func IconButton(c *ui.Context, icon, label string, o ButtonOpts) ui.Element {
	o.Icon, o.A11yLabel = icon, label
	return Button(c, "", o)
}

// PlayButton is the round amber disc that starts playback ("amber means
// playing"): size DIPs across with a solid play triangle, named by label
// ("Play OK Computer"). Click it as MyGo's: Clicked. The caller decides when
// it shows (a tile reveals it on hover or focus).
func PlayButton(c *ui.Context, label string, size float32) ui.Element {
	e := ui.ButtonBase(c).Size(size, size).Radius(controlsRadius("full")).Label(label)
	bg := theme.C("play")
	if e.Hovered() {
		bg = bg.Alpha(0.9)
	}
	e.Background(bg).TextColor(theme.C("on-play"))
	if !theme.ReducedMotion() {
		m := theme.Current().Motion["feedback"]
		e.Transition(ui.ElementTransition{Colors: true, Duration: m.Duration, Ease: m.Ease})
	}
	e.Children(func() {
		ui.Icon(c, controlsIcon("play", true)).FontSize(max(size*0.45, 12)).Margin(0, 0, 0, 1)
	})
	return e
}

// PlayButtonsOpts are the options of PlayButtons.
type PlayButtonsOpts struct {
	// Label is what plays, by name: the Play button reads "Play {label}".
	Label string
	// PlayLabel is the Play button's whole accessible name when what plays
	// is not a name ("Play your songs"); it wins over Label.
	PlayLabel string
	// Shuffle offers the Shuffle button; leave it off for a single item
	// (a song).
	Shuffle  bool
	Disabled bool
	// Bind, when set, is bound to the Play button (Element.Bind), for what
	// opens a view over the page from it to give the focus back on closing
	// (store.VideoView.OpenFrom).
	Bind *ui.Handle
}

// PlayButtons is a collection's play control: the 40px amber Play disc
// (tooltip "Play") and, when offered, an outline Shuffle button beside it,
// built into the current parent. It reports which was clicked. Disabled, the
// disc turns --hover with faint ink. (shared/components/PlayButtons.tsx)
func PlayButtons(c *ui.Context, o PlayButtonsOpts) (play, shuffle bool) {
	name := o.PlayLabel
	if name == "" {
		name = i18n.T("Play {label}", "label", o.Label)
	}
	btn := Button(c.Key("play"), "", ButtonOpts{
		Variant: Play, Size: SizeLG, Icon: "play", Filled: true, Disabled: o.Disabled,
		A11yLabel: name, Tip: i18n.T("Play"),
	})
	if o.Bind != nil {
		btn.Bind(o.Bind)
	}
	play = btn.Clicked()
	if o.Shuffle {
		shuffle = Button(c.Key("shuffle"), i18n.T("Shuffle"), ButtonOpts{Variant: Outline, Size: SizeSM, Icon: "shuffle", Disabled: o.Disabled}).Clicked()
	}
	return play, shuffle
}
