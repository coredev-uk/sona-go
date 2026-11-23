package player

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// seekRig is a bar playing a 3 min song, 30 s in, that counts the times the
// view is built.
func seekRig(t *testing.T, scale float32) (*rig, *int) {
	t.Helper()
	theme.SetMode(theme.ModeLight)
	t.Cleanup(func() { theme.SetMode(theme.ModeSystem) })
	builds := new(int)
	r := newRigWith(t, false, func(*ui.Context) { *builds++ })
	r.tt.SetScale(scale)
	r.seed()
	r.h.Clock.Advance(time.Second) // the bar's entrance has run its course
	r.tt.Advance(time.Second)
	r.seed()
	r.tt.Frame()
	return r, builds
}

// seekMarks reads the seek bar's picture, in device pixels along its rows:
// where the thumb is (the centroid of what is not the bar's background in a
// row only the thumb reaches) and how much of the centre row is amber, the
// fill, which the thumb hides the last half of.
func seekMarks(r *rig, scale float64) (thumb, fill float64) {
	b, _ := r.tt.Find("Seek")
	img := r.tt.Image()
	sat := func(x, y int) float64 {
		c := img.RGBAAt(x, y)
		return float64(max(c.R, c.G, c.B) - min(c.R, c.G, c.B))
	}
	amber := theme.C("amber")
	satAmber := float64(max(amber.R, amber.G, amber.B) - min(amber.R, amber.G, amber.B))
	top := int((b.Y + 1) * float32(scale)) // the thumb is 10 DIP high on a 3 DIP track
	bg := img.RGBAAt(img.Bounds().Dx()-1, top)
	mid := int((b.Y + seekThumb/2) * float32(scale))
	var sum, wsum float64
	for x := 0; x < img.Bounds().Dx(); x++ {
		c := img.RGBAAt(x, top)
		w := math.Abs(float64(c.R)-float64(bg.R)) + math.Abs(float64(c.G)-float64(bg.G)) + math.Abs(float64(c.B)-float64(bg.B))
		sum += w * (float64(x) + 0.5)
		wsum += w
		fill += min(sat(x, mid)/satAmber, 1)
	}
	return sum / wsum, fill
}

// While a song plays, the thumb and the fill move a fraction of a pixel in
// every frame, as the clock does, and the view is not built for them: it was
// moving in 250 ms steps of several pixels, as often as the view was built.
func TestSeekBarFollowsTheClockBetweenBuilds(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		t.Run(fmt.Sprint("scale ", scale), func(t *testing.T) {
			r, builds := seekRig(t, scale)
			s := float64(scale)
			b, _ := r.tt.Find("Seek")
			const frame = time.Second / 60
			const frames = 60 // a second, inside the one second the label shows
			*builds = 0
			prevThumb, prevFill := seekMarks(r, s)
			firstThumb, firstFill := prevThumb, prevFill
			var widest float64
			changed := 0
			for range frames {
				r.h.Clock.Advance(frame)
				r.tt.Advance(frame)
				th, fl := seekMarks(r, s)
				if th > prevThumb+0.001 || fl > prevFill+0.001 {
					changed++
				}
				if th < prevThumb-0.1 || fl < prevFill-0.1 {
					t.Fatalf("the bar went back: thumb %.2f then %.2f, fill %.2f then %.2f", prevThumb, th, prevFill, fl)
				}
				widest = max(widest, th-prevThumb, fl-prevFill)
				prevThumb, prevFill = th, fl
			}
			// The song is 180 s over the bar's width less its thumb.
			want := float64(b.W-seekThumb) / 180 * s
			for name, got := range map[string]float64{"thumb": prevThumb - firstThumb, "fill": prevFill - firstFill} {
				if math.Abs(got-want) > 0.1*want+1 {
					t.Errorf("the %s moved %.2f device pixels in a second, want about %.2f", name, got, want)
				}
			}
			if changed < frames*9/10 {
				t.Errorf("the bar changed in %d of %d frames, want nearly every one: it is stepping", changed, frames)
			}
			// 250 ms of the clock is 1.5 DIPs at 1x: no frame moves more than
			// a fraction of that.
			if step := want / 60; widest > 2*step+1 {
				t.Errorf("an edge moved %.2f device pixels in one frame, want about %.2f", widest, step)
			}
			if *builds > 2 {
				t.Errorf("the view was built %d times in %d frames: only the label's second needs one", *builds, frames)
			}
		})
	}
}

// Nothing moves, so nothing asks for a frame: paused, dragged, with motion
// reduced (the bar then steps at each second, as the view is built) or
// with the page covered.
func TestSeekBarAsksForNoFramesUnlessItMoves(t *testing.T) {
	asks := func(r *rig) bool {
		r.tt.Frame()
		r.h.Clock.Advance(time.Second) // what the bar's own entrance sets going has run out
		r.tt.Advance(time.Second)
		for range 20 {
			r.h.Clock.Advance(50 * time.Millisecond)
			r.tt.Advance(50 * time.Millisecond)
			if r.tt.FramePending() {
				return true
			}
		}
		return false
	}
	t.Run("playing", func(t *testing.T) {
		r, _ := seekRig(t, 1)
		r.tt.Advance(time.Millisecond)
		if !r.tt.FramePending() {
			t.Error("a playing bar asks for no frame")
		}
	})
	t.Run("paused", func(t *testing.T) {
		r, builds := seekRig(t, 1)
		r.seed(func(s *services.PlaybackState) { s.State = store.StatePaused })
		*builds = 0
		if asks(r) {
			t.Error("a paused bar asks for frames")
		}
	})
	t.Run("dragged", func(t *testing.T) {
		r, _ := seekRig(t, 1)
		b, _ := r.tt.Find("Seek")
		x, y := b.X+b.W/4, b.Y+b.H/2
		r.tt.Press(x, y)
		r.tt.Move(x+30, y)
		r.tt.Frame()
		if asks(r) {
			t.Error("a bar dragged by the pointer asks for frames")
		}
		r.tt.Release(x+30, y)
	})
	t.Run("reduced motion", func(t *testing.T) {
		r, _ := seekRig(t, 1)
		theme.SetReducedMotion(true)
		defer theme.SetReducedMotion(false)
		if asks(r) {
			t.Error("a bar under reduced motion asks for frames")
		}
		// It still moves, a second at a time, as the label does.
		th, _ := seekMarks(r, 1)
		r.h.Clock.Advance(3 * time.Second)
		r.tt.Advance(3 * time.Second)
		if after, _ := seekMarks(r, 1); after <= th {
			t.Errorf("the thumb stayed at %.1f over three seconds", th)
		}
	})
	t.Run("covered", func(t *testing.T) {
		r, _ := seekRig(t, 1)
		r.env.Motion = kit.NewMotionPolicy(r.env.Bus, r.env.Post, kit.MotionOptions{})
		release := r.env.Motion.Cover()
		r.tt.Frame()
		if asks(r) {
			t.Error("a bar under Now Playing asks for frames")
		}
		release()
		r.tt.Frame()
		if !asks(r) {
			t.Error("the bar did not resume")
		}
	})
}

// The label is built, not painted: it changes at the second the position
// crosses, as it did, and not at every quarter second.
func TestElapsedTimeChangesAtTheSecondBoundary(t *testing.T) {
	r, builds := seekRig(t, 1)
	r.seed(func(s *services.PlaybackState) { s.PositionMs = 30_400 })
	r.tt.Frame()
	if !r.tt.HasText("0:30") {
		t.Fatalf("not 0:30 at 30.4 s: %q", r.tt.Texts())
	}
	*builds = 0
	const step = 10 * time.Millisecond
	var at time.Duration
	for at < 2*time.Second {
		r.h.Clock.Advance(step)
		r.tt.Advance(step)
		at += step
		secs := 30 + int((400*time.Millisecond+at)/time.Second)
		shown := fmt.Sprintf("0:%02d", secs)
		// A frame at or just after the boundary may be the one that builds it.
		if (400*time.Millisecond+at)%time.Second < 2*step {
			continue
		}
		if !r.tt.HasText(shown) {
			t.Fatalf("%v after 30.4 s the label is not %s: %q", at, shown, r.tt.Texts())
		}
	}
	if *builds < 2 || *builds > 6 {
		t.Errorf("the view was built %d times in two seconds: once at each second, and not every 250 ms (which is 8)", *builds)
	}
}
