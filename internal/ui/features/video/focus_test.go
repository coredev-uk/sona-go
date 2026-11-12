package video

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// openerMount is viewMount with the page's "Show video" button, the bar's
// button that opens the view: bound to r.opener, it opens and toggles the
// view from itself. r.openerGone leaves it unbuilt.
func openerMount(r *rig) func(c *ui.Context) {
	return func(c *ui.Context) {
		open := r.v.layerOpen()
		ui.Column(c).Fill().IgnoreInput(open).Children(func() {
			kit.Button(c.Key("behind"), "Page behind", kit.ButtonOpts{})
			if !r.openerGone {
				btn := kit.Button(c.Key("opener"), "", kit.ButtonOpts{Icon: "clapperboard", A11yLabel: "Show video"}).Bind(&r.opener)
				if btn.Clicked() {
					r.env.Video.ToggleFrom(&r.opener)
				}
			}
			ui.Box(c).Grow(1)
		})
		ui.Column(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).IgnoreInput(!open).Children(func() { r.v.layer(c) })
		kit.OverlayLayer(c)
	}
}

// openFromButton plays a video and opens the view by pressing the page's
// "Show video" button.
func openFromButton(t *testing.T) *rig {
	t.Helper()
	r := newRig(t, false, openerMount, 1132, 800)
	r.play("playing")
	r.tt.MustClick("Show video")
	r.tt.Frame()
	r.tt.Until("the view", r.tt.Shows("Sky Video", "Mia Rivers"))
	r.tt.Advance(fadeIn)
	if !r.env.Video.IsOpen() || r.tt.Focused("Show video") {
		t.Fatalf("the view did not take the focus: open %v", r.env.Video.IsOpen())
	}
	return r
}

// settled draws the frames the focus needs to move.
func (r *rig) settled() {
	r.tt.Frame()
	r.tt.Frame()
	r.tt.Frame()
}

func TestEscapeClosesTheViewBackOnTheButtonThatOpenedIt(t *testing.T) {
	r := openFromButton(t)
	r.tt.Key(0, ui.KeyEscape)
	r.settled()
	if r.env.Video.IsOpen() {
		t.Fatal("Escape left the view open")
	}
	if !r.tt.Focused("Show video") {
		t.Error("the focus did not return to the Show video button")
	}
}

func TestCloseButtonReturnsTheFocusToTheOpener(t *testing.T) {
	r := openFromButton(t)
	r.tt.MustClick("Close video")
	r.settled()
	if r.env.Video.IsOpen() {
		t.Fatal("Close video left the view open")
	}
	if !r.tt.Focused("Show video") {
		t.Error("the focus did not return to the Show video button")
	}
}

func TestToggleKeyReturnsTheFocusToTheOpener(t *testing.T) {
	r := openFromButton(t)
	r.env.Video.Toggle() // the player's V key: it knows no opener of its own
	r.settled()
	if r.env.Video.IsOpen() {
		t.Fatal("the toggle left the view open")
	}
	if !r.tt.Focused("Show video") {
		t.Error("the focus did not return to the Show video button")
	}
}

func TestShowVideoButtonClosesTheViewBackOnItself(t *testing.T) {
	r := openFromButton(t)
	r.env.Video.ToggleFrom(&r.opener) // the bar's button pressed again
	r.settled()
	if r.env.Video.IsOpen() || !r.tt.Focused("Show video") {
		t.Errorf("open %v, the focus is on the button %v", r.env.Video.IsOpen(), r.tt.Focused("Show video"))
	}
}

func TestClosingAViewThatWasClosedDoesNotMoveTheFocus(t *testing.T) {
	r := openFromButton(t)
	r.env.Video.Close()
	r.settled()
	r.tt.Key(0, ui.KeyTab) // away from the button
	if r.tt.Focused("Show video") {
		t.Fatal("Tab stayed on the button")
	}
	r.settled()
	if r.tt.Focused("Show video") {
		t.Error("the focus went back to the opener again")
	}
}

func TestFocusGoesToThePageWhenTheOpenerIsGone(t *testing.T) {
	r := openFromButton(t)
	r.openerGone = true // the page that had the button is not built any more
	r.tt.Frame()
	r.tt.Key(0, ui.KeyEscape)
	r.settled()
	if r.env.Video.IsOpen() {
		t.Fatal("Escape left the view open")
	}
	if r.tt.Focused("Close video") || r.tt.Focused("Show video") {
		t.Error("the focus stayed in the closed view")
	}
	r.tt.Key(0, ui.KeyTab)
	if !r.tt.Focused("Page behind") {
		t.Error("Tab does not start at the page")
	}
	// The button coming back later does not take the focus.
	r.tt.Key(0, ui.KeyTab)
	r.openerGone = false
	r.settled()
	if r.tt.Focused("Show video") {
		t.Error("the opener took the focus when it came back")
	}
}

func TestFocusGoesToThePageWhenNothingOpenedTheView(t *testing.T) {
	r := newRig(t, false, openerMount, 1132, 800)
	r.open() // Open: no opener known
	r.tt.Key(0, ui.KeyEscape)
	r.settled()
	if r.env.Video.IsOpen() {
		t.Fatal("Escape left the view open")
	}
	if r.tt.Focused("Close video") || r.tt.Focused("Show video") {
		t.Error("the focus is on a control that did not open the view")
	}
	r.tt.Key(0, ui.KeyTab)
	if !r.tt.Focused("Page behind") {
		t.Error("Tab does not start at the page")
	}
}
