package kit

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// SwitchOpts are the options of Switch.
type SwitchOpts struct {
	// Small is 24×14 instead of 32×18.
	Small bool
	// A11yLabel names the switch for assistive technology and tests.
	A11yLabel string
	Disabled  bool
}

// Switch turns *on on and off for a setting that applies at once: a click,
// or Space with the focus on it, toggles it, and the thumb slides over
// 140 ms (not when motion is reduced). Checked it is ink-filled, unchecked
// control-line. Changed reports a click. (shared/ui/switch.tsx)
func Switch(c *ui.Context, on *bool, o SwitchOpts) ui.Element {
	w, h, thumb := float32(32), float32(18.4), float32(16)
	if o.Small {
		w, h, thumb = 24, 14, 12
	}
	e := ui.SwitchBase(c, on).Size(w, h).Radius(controlsRadius("full")).Border(1, ui.Transparent).Cursor(ui.CursorPointer)
	if o.A11yLabel != "" {
		e.Label(o.A11yLabel)
	}
	pos := float32(0)
	if *on {
		pos = 1
	}
	if !theme.ReducedMotion() {
		pos = e.Animate("knob", pos, 140*time.Millisecond)
	}
	off := theme.C("input")
	if theme.Dark() {
		off = off.Alpha(0.8)
	}
	e.Background(off.Mix(theme.C("primary"), pos))
	if *on {
		e.Background(theme.C("primary"))
	}
	thumbOff, thumbOn := theme.C("background"), theme.C("background")
	if theme.Dark() {
		thumbOff, thumbOn = theme.C("foreground"), theme.C("primary-foreground")
	}
	rtl := c.Direction() == ui.RightToLeft
	e.Draw(func(p *ui.Painter, r ui.Rect) {
		at := pos
		if rtl {
			at = 1 - pos
		}
		inner := r.W - 2
		k := ui.Rect{X: r.X + 1 + at*(inner-thumb), Y: r.Y + (r.H-thumb)/2, W: thumb, H: thumb}
		p.Fill(k, thumbOff.Mix(thumbOn, pos), thumb/2)
	})
	controlsDisabled(e, o.Disabled)
	return e
}

// SliderOpts are the options of Slider.
type SliderOpts struct {
	// A11yLabel names the slider; ValueText is what assistive technology
	// reads as its value ("50%", "1:20 / 3:40") instead of the number.
	A11yLabel string
	ValueText string
	Disabled  bool
	// Step snaps the value to lo and multiples of it; 0 is continuous (a
	// hundredth of the range per arrow).
	Step float64
	// Range is the colour of the filled part; --primary (ink) when zero.
	Range ui.Color
	// Vertical goes up from lo at the bottom, 176 high at least.
	Vertical bool
	// Width fixes a horizontal slider's width; 0 fills the parent.
	Width float32
}

// Slider sets *value between lo and hi: dragging the thumb or pressing the
// track sets it; the arrows, Page Up/Down, Home and End move it while it has
// the focus. A 4px track at 40% of --ink-faint, the range in o.Range, a 12px
// ink thumb that is always drawn, with a 4px ring while keyboard-focused.
// Its name and value text go on the slider itself. Changed reports a new
// value. (shared/ui/slider.tsx)
func Slider(c *ui.Context, value *float64, lo, hi float64, o SliderOpts) ui.Element {
	const thumb = 12
	s := ui.SliderBase(c, value, lo, hi).FocusRing(false).Cursor(ui.CursorPointer)
	if o.Step > 0 {
		s.Step(o.Step)
	}
	if o.Vertical {
		s.Vertical().Width(16).MinHeight(176).PaddingY(thumb / 2).Shrink(0)
	} else {
		s.Height(16).PaddingX(thumb / 2).Shrink(1)
		if o.Width > 0 {
			s.Width(o.Width).Shrink(0)
		} else {
			s.FillWidth()
		}
	}
	if o.A11yLabel != "" {
		s.Label(o.A11yLabel)
	}
	if o.ValueText != "" {
		s.Description(o.ValueText)
	}
	controlsDisabled(s, o.Disabled)

	at := float32(0)
	if hi > lo {
		at = float32((*value - lo) / (hi - lo))
	}
	at = min(max(at, 0), 1)
	rtl := c.Direction() == ui.RightToLeft && !o.Vertical
	if rtl {
		at = 1 - at
	}
	track, ink, ring := theme.C("ink-faint").Alpha(0.4), theme.C("ink"), theme.C("ring")
	rng := o.Range
	if rng == (ui.Color{}) {
		rng = theme.C("primary")
	}
	focus, vertical := s.FocusVisible(), o.Vertical
	s.Draw(func(p *ui.Painter, r ui.Rect) {
		var tr, fill, k ui.Rect
		if vertical {
			x := r.X + (r.W-4)/2
			y0, hgt := r.Y+thumb/2, r.H-thumb
			tr = ui.Rect{X: x, Y: y0, W: 4, H: hgt}
			fh := hgt * at
			fill = ui.Rect{X: x, Y: y0 + hgt - fh, W: 4, H: fh}
			k = ui.Rect{X: r.X + (r.W-thumb)/2, Y: y0 + hgt - fh - thumb/2, W: thumb, H: thumb}
		} else {
			y := r.Y + (r.H-4)/2
			x0, w := r.X+thumb/2, r.W-thumb
			tr = ui.Rect{X: x0, Y: y, W: w, H: 4}
			if rtl {
				fw := w * (1 - at)
				fill = ui.Rect{X: x0 + w - fw, Y: y, W: fw, H: 4}
			} else {
				fill = ui.Rect{X: x0, Y: y, W: w * at, H: 4}
			}
			k = ui.Rect{X: x0 + w*at - thumb/2, Y: r.Y + (r.H-thumb)/2, W: thumb, H: thumb}
		}
		p.Fill(tr, track, 2)
		if fill.W > 0 && fill.H > 0 {
			p.Fill(fill, rng, 2)
		}
		if focus {
			p.Fill(ui.Rect{X: k.X - 4, Y: k.Y - 4, W: k.W + 8, H: k.H + 8}, ring, thumb/2+4)
		}
		p.Fill(k, ink, thumb/2)
	})
	return s
}

// ProgressOpts are the options of ProgressBar.
type ProgressOpts struct {
	// A11yLabel names the bar (the figures beside it are decoration).
	A11yLabel string
	// ValueText is what assistive technology reads as its value ("45%,
	// 3 MB of 7 MB"); the percentage when empty.
	ValueText string
	// Height is the track's height: 4 when zero.
	Height float32
	// Width fixes the width; 0 fills the parent.
	Width float32
}

// ProgressBar shows value from 0 to 1 as a track at 40% of --ink-faint with
// an --ink-muted fill that eases over 120 ms. A negative value is progress
// of unknown length: the empty track, which does not move. It is a progress
// indicator, never a live region.
func ProgressBar(c *ui.Context, value float64, o ProgressOpts) ui.Element {
	h := o.Height
	if h == 0 {
		h = 4
	}
	e := ui.Box(c).Height(h).Shrink(0).Radius(controlsRadius("full")).Background(theme.C("ink-faint").Alpha(0.4)).Clip()
	if o.Width > 0 {
		e.Width(o.Width)
	} else {
		e.FillWidth()
	}
	e.Role(ui.RoleProgress)
	if o.A11yLabel != "" {
		e.Label(o.A11yLabel)
	}
	if value < 0 {
		return e
	}
	value = min(value, 1)
	shown := float32(value)
	if !theme.ReducedMotion() {
		shown = e.Animate("fill", shown, theme.Current().Motion["feedback"].Duration)
	}
	e.Range(0, 100, float64(int(value*100+0.5)))
	text := o.ValueText
	if text == "" {
		text = fmt.Sprintf("%d%%", int(value*100+0.5))
	}
	e.Value(text)
	fill := theme.C("ink-muted")
	return e.Draw(func(p *ui.Painter, r ui.Rect) {
		if w := r.W * shown; w > 0 {
			p.Fill(ui.Rect{X: r.X, Y: r.Y, W: w, H: r.H}, fill, r.H/2)
		}
	})
}

// TabsOpts are the options of Tabs.
type TabsOpts struct {
	// Line is the "line" variant: no background, the chosen tab marked by a
	// 2px ink rule instead of a raised fill.
	Line bool
	// Vertical stacks the tabs; the arrows still move among them.
	Vertical bool
	// A11yLabel names the tab list.
	A11yLabel string
}

// Tabs is a row (or column) of tabs showing labels, *selected the index of
// the one chosen: a click chooses a tab, and the arrows, Home and End move
// the choice and the focus. One stop of Tab, which goes to the tab chosen.
// Changed reports a new choice. Build the page of the tab chosen with
// TabPanel. (shared/ui/tabs.tsx)
func Tabs(c *ui.Context, selected *int, labels []string, o TabsOpts) ui.Element {
	tabs := ui.TabsBase(c, selected, len(labels))
	list := tabs.List.Padding(3).Radius(controlsRadius("lg")).AlignItems(ui.Center)
	if o.Vertical {
		list.Column()
		list.AlignItems(ui.Stretch)
	} else {
		list.Height(36)
	}
	if o.A11yLabel != "" {
		list.Label(o.A11yLabel)
	}
	if o.Line {
		list.Gap(4).Radius(0)
	} else {
		list.Background(theme.C("muted"))
	}
	list.Children(func() {
		for i, label := range labels {
			on := i == *selected
			tab := tabs.Tab(i).Padding(4, 8).Radius(controlsRadius("md")).Border(1, ui.Transparent)
			if o.Vertical {
				tab.Justify(ui.Start)
			} else {
				tab.Height(29)
			}
			fg := theme.C("foreground").Alpha(0.6)
			switch {
			case on:
				fg = theme.C("foreground")
				if !o.Line {
					bg := theme.C("background")
					if theme.Dark() {
						bg = theme.C("input").Alpha(0.3)
						tab.Border(1, theme.C("input"))
					}
					tab.Background(bg)
					sh := theme.Current().Shadow["sm"]
					tab.Shadow(sh.X, sh.Y, sh.Blur, sh.Spread, sh.Color)
				}
			case tab.Hovered():
				fg = theme.C("foreground")
			}
			tab.TextColor(fg)
			tab.Children(func() { controlsText(c, label, "label").SingleLine() })
			if on && o.Line {
				tab.DrawOver(func(p *ui.Painter, r ui.Rect) {
					if o.Vertical {
						p.Fill(ui.Rect{X: r.X + r.W + 2, Y: r.Y, W: 2, H: r.H}, theme.C("foreground"), 0)
						return
					}
					p.Fill(ui.Rect{X: r.X, Y: r.Y + r.H + 3, W: r.W, H: 2}, theme.C("foreground"), 0)
				})
			}
		}
	})
	return list
}

// TabPanel is the page of a tab: a tab panel named by label (the tab's)
// that fills the space under the tab list. Build only the chosen tab's.
func TabPanel(c *ui.Context, label string) ui.Element {
	return ui.Column(c).Grow(1).Role(ui.RoleTabPanel).Label(label)
}
