package kit

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// swipeEvents drives a Swipe with the events of a trackpad as MyGo reports
// them (a Tester cannot send the momentum ones): DX negative for fingers
// moving right.
type swipeEvents struct {
	t          *testing.T
	s          *Swipe
	now        time.Time
	back, fwds int
}

func newSwipeEvents(t *testing.T) *swipeEvents {
	e := &swipeEvents{t: t, now: time.Unix(1000, 0)}
	e.s = NewSwipe(func() { e.back++ }, func() { e.fwds++ })
	e.s.Enabled = true
	return e
}

// scroll sends n events of (dx, dy) each, 8 ms apart, and returns what
// Input said of the last.
func (e *swipeEvents) scroll(n int, dx, dy float32, momentum bool) bool {
	var took bool
	for range n {
		e.now = e.now.Add(8 * time.Millisecond)
		took = e.s.Input(ui.InputEvent{Kind: ui.InputScroll, DX: dx, DY: dy, Precise: true, Momentum: momentum, Time: e.now})
	}
	return took
}

func (e *swipeEvents) want(back, fwds int) {
	e.t.Helper()
	if e.back != back || e.fwds != fwds {
		e.t.Fatalf("went back %d and forward %d times, want %d and %d", e.back, e.fwds, back, fwds)
	}
}

// The fingers lifting leaves momentum, which carries on for a second as the
// swipe's own: it neither goes another page nor scrolls the page under it,
// and it ends the gesture, so that the next one is a new swipe.
func TestSwipeMomentumNeitherCountsNorLeavesTheGesture(t *testing.T) {
	e := newSwipeEvents(t)
	e.scroll(10, -8, 0, false) // 80: short of a page
	e.want(0, 0)
	e.scroll(100, -20, 0, true) // coasting a long way
	e.want(0, 0)
	e.scroll(10, -8, 0, false) // a new gesture, no quiet between: 80 again
	e.want(0, 0)
	e.scroll(10, -8, 0, false) // 160: back
	e.want(1, 0)
	if !e.scroll(1, -20, 0, true) {
		t.Error("momentum after a swipe that went a page scrolls the page under it")
	}
	e.want(1, 0)

	// A new gesture starts after momentum, with no gap between.
	e.scroll(10, 40, 0, false)
	e.want(1, 1)
}

// The quiet of a gap ends a gesture where no momentum follows.
func TestSwipeGestureEndsWithAGap(t *testing.T) {
	e := newSwipeEvents(t)
	e.scroll(10, -8, 0, false)
	e.now = e.now.Add(swipeGap + time.Millisecond)
	e.scroll(10, -8, 0, false) // 80 of a new gesture, not 160 of the old
	e.want(0, 0)
	e.scroll(10, -8, 0, false)
	e.want(1, 0)
}

// The events of a vertical gesture, a mouse and a held modifier are left to
// what scrolls, and so are all that a container that scrolls sideways has
// claimed.
func TestSwipeLeavesOtherScrollingAlone(t *testing.T) {
	e := newSwipeEvents(t)
	if e.scroll(30, -1, 30, false) {
		t.Error("a vertical gesture is taken")
	}
	e.now = e.now.Add(time.Second)
	for _, ev := range []ui.InputEvent{
		{Kind: ui.InputScroll, DX: -400, Precise: false},
		{Kind: ui.InputScroll, DX: -400, Precise: true, Mods: ui.Shift},
		{Kind: ui.InputPointerDown},
	} {
		ev.Time = e.now
		if e.s.Input(ev) {
			t.Errorf("%+v is taken", ev)
		}
	}
	e.want(0, 0)

	// A shelf that scrolls that way claims the gesture whole: not a
	// page, even where it reaches the shelf's end.
	e.now = e.now.Add(time.Second)
	sidewaysClaim = true
	e.scroll(1, -20, 0, false)
	if took := e.scroll(40, -20, 0, false); took {
		t.Error("a claimed gesture is taken")
	}
	e.want(0, 0)

	// A claim that is not followed by an event is not carried to the next.
	e.s.Enabled = false
	sidewaysClaim = true
	e.scroll(1, -20, 0, false)
	if sidewaysClaim {
		t.Error("Input left the claim")
	}
}

// Disabled, it keeps nothing: turned on halfway, a gesture starts there.
func TestSwipeOffTakesNothing(t *testing.T) {
	e := newSwipeEvents(t)
	e.s.Enabled = false
	if e.scroll(100, -20, 0, false) {
		t.Error("a disabled swipe takes events")
	}
	e.want(0, 0)
}
