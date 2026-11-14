package player

import (
	"math"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// centre is the middle of the control called name.
func (r *rig) centre(t *testing.T, name string) (x, y float32) {
	t.Helper()
	b, ok := r.tt.Find(name)
	if !ok {
		t.Fatalf("no %q: %q", name, r.tt.Texts())
	}
	return b.X + b.W/2, b.Y + b.H/2
}

func (r *rig) volume() float64 { return r.env.Play.State().Volume }

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// The wheel over the slider, and over the mute button, raises and lowers the
// volume by 5% a notch, through the transport the slider's drag uses: the
// service is told, the mirror follows, and the slider and the icon show it.
func TestWheelOverTheVolumeChangesIt(t *testing.T) {
	for _, over := range []string{"Volume", "Mute"} {
		r := newRig(t, true)
		r.seed(func(s *services.PlaybackState) { s.Volume = 0.6 })
		x, y := r.centre(t, over)

		r.tt.Wheel(x, y, 0, -40) // a notch up
		r.tt.Frame()
		if v := r.volume(); !near(v, 0.65) {
			t.Errorf("over %s, a notch up: volume %v, want 0.65", over, v)
		}
		if got := access.Read(t, r.tt.Tester).MustFind(t, ui.RoleSlider, "Volume").Now; got != 65 {
			t.Errorf("over %s: the slider reads %v, want 65", over, got)
		}
		r.tt.Wheel(x, y, 0, 40)
		r.tt.Wheel(x, y, 0, 40)
		r.tt.Frame()
		if v := r.volume(); !near(v, 0.55) {
			t.Errorf("over %s, two notches down: volume %v, want 0.55", over, v)
		}
		r.fake.Wait(t, 3)
		vols := r.fake.Volumes()
		if len(vols) != 3 || !near(vols[len(vols)-1], 0.55) && !near(vols[0], 0.55) {
			t.Errorf("over %s: the service was told %v", over, vols)
		}
	}
}

// A trackpad swipe sideways, and a vertical scroll, change it in proportion,
// whichever way moved further; Momentum changes nothing.
func TestTrackpadOverTheVolumeIsProportional(t *testing.T) {
	r := newRig(t, true)
	r.seed(func(s *services.PlaybackState) { s.Volume = 0.5 })
	x, y := r.centre(t, "Volume")

	r.tt.Scroll(x, y, -50, 4) // fingers right 50 DIPs (natural scrolling): +10%
	if v := r.volume(); !near(v, 0.6) {
		t.Errorf("a swipe right: volume %v, want 0.6", v)
	}
	r.tt.Scroll(x, y, 25, -3) // left 25 DIPs: -5%
	if v := r.volume(); !near(v, 0.55) {
		t.Errorf("a swipe left: volume %v, want 0.55", v)
	}
	r.tt.Scroll(x, y, 2, -50) // mostly vertical, up: +10%
	if v := r.volume(); !near(v, 0.65) {
		t.Errorf("a vertical scroll up: volume %v, want 0.65", v)
	}
	r.tt.Scroll(x, y, 0, 2) // under 1%: waits
	if v := r.volume(); !near(v, 0.65) {
		t.Errorf("2 DIPs moved the volume to %v", v)
	}
	r.fake.Wait(t, 3)
}

func TestWheelOverTheVolumeStopsAtBothEnds(t *testing.T) {
	r := newRig(t, true)
	r.seed(func(s *services.PlaybackState) { s.Volume = 0.9 })
	x, y := r.centre(t, "Volume")

	for range 6 {
		r.tt.Wheel(x, y, 0, -40)
	}
	if v := r.volume(); v != 1 {
		t.Errorf("past the top: volume %v, want 1", v)
	}
	for range 25 {
		r.tt.Wheel(x, y, 0, 40)
	}
	if v := r.volume(); v != 0 {
		t.Errorf("past the bottom: volume %v, want 0", v)
	}
	r.tt.Frame()
	if !r.tt.HasText("Unmute") {
		t.Errorf("at 0 the icon should offer Unmute: %q", r.tt.Texts())
	}
	// Muted, a notch up raises it, as dragging the slider off 0 does.
	r.tt.Wheel(x, y, 0, -40)
	r.tt.Frame()
	if v := r.volume(); !near(v, 0.05) {
		t.Errorf("up from muted: volume %v, want 0.05", v)
	}
	if !r.tt.HasText("Mute") || r.tt.HasText("Unmute") {
		t.Errorf("up from muted the icon should offer Mute: %q", r.tt.Texts())
	}
	r.fake.Wait(t, 1) // the changes ran off the main thread; the last one is enough to wait for
}

// The wheel is the volume's alone over it: a container the bar is in does
// not scroll under it, while the same wheel over the rest of the bar does
// scroll that container.
func TestWheelOverTheVolumeDoesNotScrollWhatItIsIn(t *testing.T) {
	var pos ui.ScrollState
	build := func(t *testing.T) *rig {
		h := uitest.New(t, uitest.Bare())
		r := &rig{fake: newFake(), h: h, env: h.Env}
		queueOver(r.env, r.fake)
		r.p = newPlayer(r.env, r.fake, func() bool { return true })
		r.p.now = h.Clock.Now
		r.tt = h.Tester(t, func(c *ui.Context) {
			ui.Scroll(c).TrackScroll(&pos).Fill().Children(func() {
				r.p.bar(c)
				ui.Box(c).Height(2000)
			})
		}, 1132, 400)
		r.seed(func(s *services.PlaybackState) { s.Volume = 0.6 })
		return r
	}

	pos = ui.ScrollState{}
	r := build(t)
	x, y := r.centre(t, "Next")
	r.tt.Scroll(x, y, 0, 60)
	if pos.Y == 0 {
		t.Fatal("scrolling over the bar's other controls does not scroll the container: the test shows nothing")
	}

	for _, over := range []string{"Volume", "Mute"} {
		pos = ui.ScrollState{}
		r := build(t)
		x, y := r.centre(t, over)
		r.tt.Wheel(x, y, 0, 40)
		r.tt.Scroll(x, y, 0, 60)
		r.tt.Scroll(x, y, 30, 0)
		r.tt.Advance(300e6)
		if pos.Y != 0 || pos.X != 0 {
			t.Errorf("over %s the container scrolled to %v, %v", over, pos.X, pos.Y)
		}
		if v := r.volume(); !near(v, 0.6-0.05-0.12-0.06) {
			t.Errorf("over %s: volume %v", over, v)
		}
		r.fake.Wait(t, 1)
	}
}

// Nothing about the keyboard or the tree changes: the wrapper is not a stop
// and says nothing, the slider keeps its arrows, and Ctrl+Up and Ctrl+Down
// still step by the same 5%.
func TestVolumeKeyboardAndTreeAreUnchangedByTheWheel(t *testing.T) {
	r := newRig(t, true)
	r.seed(func(s *services.PlaybackState) { s.Volume = 0.6 })
	access.NoNestedControls(t, r.tt.Tester)
	stops := access.StopLabels(access.TabWalk(t, r.tt.Tester, 60))
	access.InOrder(t, stops, "Queue", "Mute", "Volume")
	tr := access.Read(t, r.tt.Tester)
	if v := tr.MustFind(t, ui.RoleSlider, "Volume"); v.Description != "" || v.Now != 60 || v.Max != 100 {
		t.Errorf("Volume reads %v of %v and %q", v.Now, v.Max, v.Description)
	}

	tabTo(t, r.tt.Tester, "Volume")
	r.tt.Key(0, ui.KeyRight)
	r.tt.Frame()
	if v := r.volume(); v <= 0.6 {
		t.Errorf("Right on the focused slider left the volume at %v", v)
	}
}
