package kit

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

const (
	// VolumeStep is one step of the volume: a press of Ctrl+Up or Ctrl+Down,
	// a notch of a mouse wheel over the volume control.
	VolumeStep = 0.05

	// wheelPerDIP is the volume a touchpad's scrolling changes by per DIP it
	// travels: 1% for 5 DIPs, so that a swipe as long as a wheel notch is
	// (40 to 50 DIPs) comes to a little more than a notch's step.
	wheelPerDIP = 0.002

	// wheelGrain is the smallest change a touchpad makes: 1%. Smaller
	// travel waits in VolumeWheel until it adds up to one, so a swipe asks
	// the service for a volume per percent, not per event.
	wheelGrain = 0.01

	// volumeRound rounds the volume to a ten-thousandth, so that twenty steps
	// of 5% down end at 0, muted, and not at 1e-17.
	volumeRound = 1e4
)

// VolumeWheel turns the scrolling over a volume control into volume
// changes: Attach it to the element that holds the mute button and the
// slider. One per control, main thread only; the zero value is ready.
//
// A mouse wheel's notch (InputEvent.Precise false) is one VolumeStep. A
// touchpad's scrolling (Precise) is proportional, 1% per 5 DIPs, travel
// short of 1% waiting here for the next event; a change of direction drops
// what waited, and so does reaching 0 or 100%, so that turning back at an
// end answers at once. Whichever axis moved further counts, so a vertical
// scroll and a horizontal swipe both work. The direction is the one the
// system reports: DY and DX are how far the view moves along the content,
// and the volume rises where it moves up or to the left (DY or DX negative:
// a wheel turned up; with "natural" scrolling, fingers moving down or to the
// right, as if they dragged the volume) and falls where it moves down or to
// the right. Right to left, where the slider fills from the right, the
// sideways direction is the other way.
//
// The momentum of a swipe that carries on after the fingers lift changes
// nothing, as a volume that drifts on would be unexpected, but is taken, so
// that it does not scroll what lies behind. With Ctrl, Alt or Super held
// the scrolling is not the control's: Ctrl is a zoom on Windows, and goes on
// to whatever zooms.
//
// MyGo reports scrolling to the element under the pointer that handles its
// input (HandleInput) before it scrolls a container (ui/input.go, scroll):
// the control takes the event and nothing scrolls behind it, and it costs
// nothing while nobody scrolls.
type VolumeWheel struct {
	// OnScroll, when set, is called with the time of each scroll the control
	// takes, as the video view brings back its faded controls with it.
	OnScroll func(at time.Time)

	pending float64 // travel of a touchpad not yet a change, in volume
}

// Attach has e, the element around the mute button and the slider, change
// the volume of t by the scrolling over it, and returns e.
func (w *VolumeWheel) Attach(c *ui.Context, e ui.Element, t Transport) ui.Element {
	rtl := c.Direction() == ui.RightToLeft
	return e.HandleInput(func(ev ui.InputEvent) bool { return w.input(t, ev, rtl) })
}

// input handles one event of Attach and reports whether it took it.
func (w *VolumeWheel) input(t Transport, ev ui.InputEvent, rtl bool) bool {
	if ev.Kind != ui.InputScroll || ev.Mods&(ui.Ctrl|ui.Alt|ui.Super) != 0 {
		return false
	}
	if w.OnScroll != nil {
		w.OnScroll(ev.Time)
	}
	// More is the opposite of how far the view moves along the content.
	d := -float64(ev.DY)
	if math.Abs(float64(ev.DX)) > math.Abs(float64(ev.DY)) {
		d = -float64(ev.DX)
		if rtl {
			d = -d
		}
	}
	if d == 0 || ev.Momentum {
		return true
	}
	var change float64
	if ev.Precise {
		if w.pending*d < 0 {
			w.pending = 0
		}
		w.pending += d * wheelPerDIP
		change = math.Trunc(w.pending/wheelGrain) * wheelGrain
		if change == 0 {
			return true
		}
		w.pending -= change
	} else {
		w.pending = 0
		change = math.Copysign(VolumeStep, d)
	}
	cur := t.Play.State().Volume
	next := min(max(math.Round((cur+change)*volumeRound)/volumeRound, 0), 1)
	if next == cur {
		w.pending = 0 // at an end
		return true
	}
	t.SetVolume(next)
	return true
}
