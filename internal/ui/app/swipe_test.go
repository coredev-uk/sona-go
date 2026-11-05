package app

import (
	"strconv"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// swipeRig is the shell over pages /a, /b and /c in a history, a page of a
// shelf that scrolls sideways (/shelf), one with a text field (/field) and an
// overlay that is open while overlay.
type swipeRig struct {
	t     *testing.T
	a     *App
	tt    *uitest.Session
	shelf ui.ScrollState
	// fits makes the shelf's content fit its box: nothing to scroll.
	fits bool
	text string
	// overlay is open; closes counts what the shell closed.
	overlay bool
	closes  int
}

func newSwipeRig(t *testing.T) *swipeRig {
	t.Helper()
	r := &swipeRig{t: t}
	r.a, r.tt = newShell(t, Options{}, func(*module.Env) module.Module {
		m := module.Module{ID: "swipe", Slots: module.Slots{Overlay: &module.Overlay{
			Open:  func() bool { return r.overlay },
			Build: func(c *ui.Context) { ui.Text(c, "overlay") },
			Close: func() { r.overlay = false; r.closes++ },
		}}}
		for _, p := range []string{"a", "b", "c"} {
			m.Routes = append(m.Routes, module.Route{Pattern: "/" + p, Build: func(c *ui.Context, _ *ui.Route) { ui.Text(c, "page "+p) }})
		}
		m.Routes = append(m.Routes,
			module.Route{Pattern: "/shelf", Build: r.shelfPage},
			module.Route{Pattern: "/field", Build: func(c *ui.Context, _ *ui.Route) {
				in := ui.TextInput(c.Key("name"), &r.text).Label("Name")
				if r.text == "" {
					in.Focus()
				}
			}})
		m.Nav = []module.NavItem{{Section: module.NavPrimary, Label: func() string { return "A" }, To: "/a", Order: 1}}
		return m
	})
	r.a.swipe.Enabled = true // as on macOS
	r.go_("/b", "/c")
	return r
}

// shelfPage is a column with a row of covers that scrolls sideways when they
// do not fit.
func (r *swipeRig) shelfPage(c *ui.Context, _ *ui.Route) {
	ui.Column(c).Padding(24).Gap(12).Children(func() {
		ui.Text(c, "page shelf")
		row := kit.ScrollSideways(c, &r.shelf).Width(400).Gap(8)
		row.Children(func() {
			for i := range 12 {
				w := float32(160)
				if r.fits {
					w = 20
				}
				ui.Box(c.Key(strconv.Itoa(i))).Width(w).Height(60).Children(func() { ui.Text(c, "cover "+strconv.Itoa(i)) })
			}
		})
	})
}

// go_ pushes pages and shows them.
func (r *swipeRig) go_(paths ...string) {
	r.t.Helper()
	for _, p := range paths {
		r.a.router.Push(p)
	}
	r.tt.Frame()
}

// pan is two fingers moving over the page at (x, y) by (dx, dy) in all, in
// steps a few milliseconds apart, as a trackpad reports them: precise,
// without momentum (a Tester cannot send it). MyGo reports a scroll as the
// content moves it, the opposite of the fingers on the pad.
func (r *swipeRig) pan(x, y, dx, dy float32) {
	r.t.Helper()
	const steps = 12
	for range steps {
		r.tt.Scroll(x, y, -dx/steps, dy/steps)
		r.tt.Advance(8 * time.Millisecond)
	}
}

// rest lets the hands go: a new gesture follows.
func (r *swipeRig) rest() { r.tt.Advance(400 * time.Millisecond) }

func (r *swipeRig) at(want string) {
	r.t.Helper()
	if got := r.a.router.Path(); got != want {
		r.t.Fatalf("the page is %q, want %q", got, want)
	}
}

// Fingers moving right push the page that is shown to the right, which shows
// the one before it: back. To the left is forward.
func TestTwoFingerSwipeGoesBackAndForward(t *testing.T) {
	r := newSwipeRig(t)
	r.at("/c")
	r.pan(700, 400, 400, 0) // to the right
	r.at("/b")
	r.rest()
	r.pan(700, 400, -400, 0)
	r.at("/c")
	if !r.tt.HasText("page c") {
		t.Errorf("forward did not show the page: %q", r.tt.Texts())
	}
}

// A long swipe goes one page, and so does one that goes on with the fingers
// still down; a gesture ends when the fingers rest.
func TestTwoFingerSwipeGoesOnePagePerGesture(t *testing.T) {
	r := newSwipeRig(t)
	r.pan(700, 400, 1500, 0)
	r.pan(700, 400, 1500, 0) // no gap: the same gesture
	r.at("/b")
	r.rest()
	r.pan(700, 400, 300, 0)
	r.at("/a")
	r.rest()
	r.pan(700, 400, 300, 0) // the start of the history: nowhere to go
	r.at("/a")
}

// A swipe that does not go far, or that is mostly up or down, or that comes
// from a mouse's wheel, is not a page turn: pages scroll.
func TestTwoFingerSwipeNeedsDistanceDirectionAndATouchpad(t *testing.T) {
	r := newSwipeRig(t)
	r.pan(700, 400, 60, 0)
	r.rest()
	r.pan(700, 400, 400, 300) // diagonal
	r.rest()
	r.pan(700, 400, 100, 600) // vertical, drifting
	r.rest()
	for range 20 {
		r.tt.Wheel(700, 400, -80, 0) // a mouse's wheel, held on Shift
		r.tt.Advance(8 * time.Millisecond)
	}
	r.at("/c")
}

// Where it is not macOS the swipe is only scrolling.
func TestTwoFingerSwipeIsOffUnlessEnabled(t *testing.T) {
	r := newSwipeRig(t)
	r.a.swipe.Enabled = false
	r.pan(700, 400, 600, 0)
	r.at("/c")
}

// A swipe that begins over a shelf that can scroll that way scrolls the
// shelf; at the shelf's start, where it cannot go that way, or over one that
// fits, it goes back.
func TestTwoFingerSwipeLeavesAHorizontalShelfItsScrolling(t *testing.T) {
	r := newSwipeRig(t)
	r.go_("/shelf")
	rect, ok := r.tt.Find("cover 0")
	if !ok {
		t.Fatalf("no shelf: %q", r.tt.Texts())
	}
	x, y := rect.X+rect.W/2, rect.Y+rect.H/2

	// Fingers to the left: the shelf scrolls toward its end.
	r.pan(x, y, -300, 0)
	r.at("/shelf")
	if r.shelf.X <= 0 {
		t.Fatalf("the shelf did not scroll: X = %v", r.shelf.X)
	}
	moved := r.shelf.X
	r.rest()
	// Fingers to the right scroll it back, and still do not turn the page.
	r.pan(x, y, 100, 0)
	r.at("/shelf")
	if r.shelf.X >= moved {
		t.Errorf("the shelf did not scroll back: X = %v, was %v", r.shelf.X, moved)
	}
	// At its start the shelf cannot go that way: the swipe goes back, as
	// Safari's does from a page scrolled to its left edge.
	r.shelf.X = 0
	r.tt.Frame()
	r.rest()
	r.pan(x, y, 400, 0)
	r.at("/c")
	r.rest()

	// Content that fits has nothing to scroll.
	r.go_("/shelf")
	r.fits = true
	r.tt.Frame()
	r.pan(x, y, -400, 0)
	r.at("/shelf")
	r.rest()
	r.pan(x, y, 400, 0)
	r.at("/c")
}

// The keys and swipes go back with the keyboard in a text field, which has
// no use for them.
func TestBackAndForwardKeysAndSwipesWorkFromATextField(t *testing.T) {
	r := newSwipeRig(t)
	r.go_("/field")
	r.tt.Frame()
	if !r.tt.Focused("Name") {
		t.Fatal("the text field has no focus")
	}
	r.tt.Key(0, ui.KeyBack)
	r.at("/c")
	r.tt.Key(0, ui.KeyForward)
	r.at("/field")
	r.tt.Frame()
	r.pan(700, 400, 400, 0)
	r.at("/c")
}

// A page that comes while an overlay covers the page (Now Playing) is behind
// it: the overlay gives way, from the history's keys and buttons as from a
// swipe. Escaping it is the module's own; a page that does not change leaves
// it open.
func TestAnOverlayGivesWayWhenThePageChanges(t *testing.T) {
	for name, back := range map[string]func(*swipeRig){
		"back key":   func(r *swipeRig) { r.tt.Key(0, ui.KeyBack) },
		"alt+left":   func(r *swipeRig) { r.tt.Key(ui.Alt, ui.KeyLeft) },
		"swipe":      func(r *swipeRig) { r.pan(700, 400, 400, 0) },
		"the button": func(r *swipeRig) { r.a.router.Back() },
	} {
		t.Run(name, func(t *testing.T) {
			r := newSwipeRig(t)
			r.overlay = true
			r.tt.Frame()
			if !r.tt.HasText("overlay") {
				t.Fatal("the overlay does not show")
			}
			r.tt.Frame()
			r.tt.Frame()
			if r.closes != 0 {
				t.Fatalf("the shell closed the overlay with no page change")
			}
			back(r)
			r.tt.Frame()
			r.at("/b")
			if r.overlay || r.closes != 1 {
				t.Errorf("overlay open %v, closed %d times", r.overlay, r.closes)
			}
			if r.tt.HasText("overlay") || !r.tt.HasText("page b") {
				t.Errorf("the page is not shown: %q", r.tt.Texts())
			}
		})
	}
}

// A dialog is modal: the page behind it does not turn under a swipe or a key.
func TestSwipeAndKeysLeaveAPageBehindADialogAlone(t *testing.T) {
	r := newSwipeRig(t)
	t.Cleanup(kit.CancelConfirm)
	kit.Confirm(kit.ConfirmOptions{Title: "Delete it?", Body: "Gone.", ConfirmLabel: "Delete"}, func(bool) {})
	r.tt.Frame()
	if !r.tt.HasText("Delete it?") {
		t.Fatal("no dialog")
	}
	r.pan(566, 400, 500, 0)
	r.pan(100, 400, 500, 0)
	r.rest()
	r.tt.Key(0, ui.KeyBack)
	r.at("/c")
}
