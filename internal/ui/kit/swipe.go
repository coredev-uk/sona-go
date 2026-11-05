package kit

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

// The two-finger swipe between pages, as Safari and Finder have it on a Mac
// whose trackpad is on its default, "Scroll left or right with two fingers":
// MyGo reports such a swipe as scrolling (an InputScroll of a touchpad, with
// DX), and only the three-finger "Swipe between pages" setting as KeyBack and
// KeyForward. Swipe turns the scrolling into the same two moves.
const (
	// swipeLock is how far a gesture must travel before it is told
	// horizontal from vertical: less is the hand settling, which scrolls
	// the page as it would.
	swipeLock = 12
	// swipeTravel is how far a horizontal gesture must travel to go back or
	// forward: a deliberate swipe, not a page nudged sideways.
	swipeTravel = 120
	// swipeSlope is how much further a horizontal gesture goes sideways
	// than up or down, at the lock and at the end: a vertical scroll that
	// drifts does not turn a page.
	swipeSlope = 2
	// swipeGap is the quiet that ends a gesture when the system reports no
	// momentum after it: fingers on a trackpad produce events every few
	// milliseconds, even resting ones.
	swipeGap = 200 * time.Millisecond
)

// Swipe recognises the two-finger swipe between pages (see above) from the
// scroll events a window's root element is handed (Input). It is MyGo's
// InputScroll, which carries no gesture phase (began, ended, momentum
// beginning), so a gesture is what the events make: one runs from the first
// event after a gap (swipeGap) or after momentum to its last, and goes at
// most one page, however far it travels or coasts. Momentum events
// (InputEvent.Momentum), which carry on after the fingers lift, never count.
//
// A gesture is horizontal when it travels sideways at least swipeSlope times
// as far as up or down once it has travelled swipeLock; from then on it is
// the swipe's alone, so the page does not drift up and down under it. One
// that begins over something that scrolls sideways (ScrollSideways) is that
// container's: a shelf of covers goes on scrolling.
//
// Fingers moving to the right go back, as pushing the page to the right
// shows the page before it; to the left, forward (the other way in a right
// to left layout, RightToLeft). That is for macOS's "Natural scrolling", the
// default: MyGo does not report the setting
// (NSEvent.isDirectionInvertedFromDevice), so with it off the two are
// swapped. Mouse wheels, which scroll by lines, never navigate. The owner
// turns it on only while the system's "Swipe between pages" setting lets a
// two-finger scroll swipe (NSEvent.isSwipeTrackingFromScrollEventsEnabled,
// which MyGo does not report either).
//
// Main thread only, like every view. The zero value is not usable: NewSwipe.
type Swipe struct {
	// Enabled turns it on; off, Input takes nothing and keeps nothing.
	Enabled bool
	// RightToLeft mirrors the swipe, as the window's layout is: fingers
	// to the left go back, as WebKit does.
	RightToLeft bool

	back, forward func()

	// A gesture's state: when its last event came, whether it is coasting
	// (momentum came), how far it has travelled, and what it came to.
	last     time.Time
	coasting bool
	sx, sy   float32
	phase    swipePhase
}

type swipePhase uint8

const (
	swipeIdle      swipePhase = iota // no gesture yet
	swipeUndecided                   // travelling less than swipeLock
	swipeSideways                    // horizontal, short of swipeTravel
	swipeDone                        // went a page: the rest of the gesture is nothing
	swipeOther                       // vertical, or a container's: not for navigating
)

// NewSwipe makes a recogniser that calls back or forward for a swipe, off
// until Enabled is set.
func NewSwipe(back, forward func()) *Swipe {
	return &Swipe{back: back, forward: forward}
}

// sidewaysClaim is set by a container that scrolls sideways (ScrollSideways)
// as it handles a scroll event it can scroll, and read and cleared by Input,
// which handles the same event after it: MyGo hands an event to the
// handlers from the innermost element out. Main thread only.
var sidewaysClaim bool

// ScrollSideways is ui.ScrollHorizontal tracked in pos: a row that scrolls
// its children sideways, as a shelf that does not fit its width. A swipe
// that begins over it while it can scroll that way scrolls it, and does not
// turn a page (Swipe): use it, rather than ui.ScrollHorizontal or
// ui.ScrollBoth, for any container whose horizontal scrolling must win over
// swiping between pages.
func ScrollSideways(c *ui.Context, pos *ui.ScrollState) ui.Element {
	e := ui.ScrollHorizontal(c).TrackScroll(pos)
	return e.HandleInput(func(ev ui.InputEvent) bool {
		if ev.Kind == ui.InputScroll && ev.DX != 0 && math.Abs(float64(ev.DX)) >= math.Abs(float64(ev.DY)) {
			dx := ev.DX
			if c.Direction() == ui.RightToLeft {
				dx = -dx // the offset counts from the right
			}
			if dx > 0 && pos.X < pos.MaxX || dx < 0 && pos.X > 0 {
				sidewaysClaim = true
			}
		}
		return false // the container scrolls
	})
}

// Input is the root element's HandleInput. It takes the scrolling of a
// horizontal gesture that has been told from the vertical ones, and leaves
// every other event to what scrolls.
func (s *Swipe) Input(ev ui.InputEvent) bool {
	if ev.Kind != ui.InputScroll {
		return false
	}
	claimed := sidewaysClaim
	sidewaysClaim = false
	if !s.Enabled || !ev.Precise || ev.Mods != 0 {
		return false
	}
	if ev.Momentum {
		// The gesture coasts on: nothing goes by it, and what the swipe
		// had taken it still takes, or the page would drift.
		s.coasting, s.last = true, ev.Time
		return s.phase == swipeSideways || s.phase == swipeDone
	}
	if s.phase == swipeIdle || s.coasting || ev.Time.Sub(s.last) > swipeGap {
		s.phase, s.coasting, s.sx, s.sy = swipeUndecided, false, 0, 0
	}
	s.last = ev.Time
	if claimed && s.phase != swipeDone {
		s.phase = swipeOther
	}
	switch s.phase {
	case swipeUndecided:
		s.sx += ev.DX
		s.sy += ev.DY
		if math.Hypot(float64(s.sx), float64(s.sy)) < swipeLock {
			return false
		}
		if math.Abs(float64(s.sx)) < swipeSlope*math.Abs(float64(s.sy)) {
			s.phase = swipeOther
			return false
		}
		s.phase = swipeSideways
		return s.travelled()
	case swipeSideways:
		s.sx += ev.DX
		s.sy += ev.DY
		return s.travelled()
	case swipeDone:
		return true
	}
	return false
}

// travelled goes a page once the gesture has travelled far enough, and
// reports that the gesture is the swipe's.
func (s *Swipe) travelled() bool {
	ax, ay := math.Abs(float64(s.sx)), math.Abs(float64(s.sy))
	if ax < swipeTravel || ax < swipeSlope*ay {
		return true
	}
	s.phase = swipeDone
	// DX counts the way the content scrolls: negative is the fingers
	// moving right, which goes back left to right; right to left, where
	// the Back arrow points right and pages slide the other way, forward.
	if (s.sx < 0) != s.RightToLeft {
		s.back()
	} else {
		s.forward()
	}
	return true
}
