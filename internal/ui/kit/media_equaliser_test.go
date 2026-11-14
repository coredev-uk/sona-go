package kit

import (
	"bytes"
	"fmt"
	"math"
	"runtime"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

func TestEqLevelEasesBetweenLowestAndFull(t *testing.T) {
	near := func(got, want float32) bool { return got-want < 0.001 && want-got < 0.001 }
	for _, b := range eqBars {
		p := b.period
		for _, w := range []struct {
			at    time.Duration
			scale float32
		}{
			{0, eqLowest},
			{p / 2, 1},
			{p, eqLowest}, // the cycle repeats
			{p / 4, (eqLowest + 1) / 2},
			{3 * p / 4, (eqLowest + 1) / 2},
			{-p / 2, 1},
			{37*p + p/2, 1},
		} {
			if got := eqLevel(p, w.at); !near(got, w.scale) {
				t.Errorf("period %v: eqLevel(%v) = %v, want %v", p, w.at, got, w.scale)
			}
		}
		// Rising to the half, falling after it, never lower than eqLowest nor
		// higher than 1, and never a jump of more than 6% of the full height in a
		// 60 Hz frame: moving, not stepping. It is quick through the middle
		// and slow at the turns.
		const frame = time.Second / 60
		prev, fastest, turning := eqLevel(p, 0), float32(0), float32(0)
		middle := float32(0)
		for at := frame; at <= p; at += frame {
			l := eqLevel(p, at)
			if (at <= p/2) != (l >= prev) && !near(l, prev) {
				t.Errorf("period %v: eqLevel is %v then %v at %v, not rising then falling", p, prev, l, at)
			}
			if l < eqLowest-0.001 || l > 1.001 {
				t.Errorf("period %v: eqLevel(%v) = %v, outside %v to 1", p, at, l, eqLowest)
			}
			d := l - prev
			if d < 0 {
				d = -d
			}
			if d > 0.06 {
				t.Errorf("period %v: eqLevel jumps %v in a frame at %v", p, d, at)
			}
			fastest = max(fastest, d)
			if u := float64(at) / float64(p); u <= 0.1 || u >= 0.9 || (u >= 0.4 && u <= 0.6) { // turning at the bottom or the top
				turning = max(turning, d)
			} else if (u >= 0.15 && u <= 0.35) || (u >= 0.65 && u <= 0.85) { // a stroke's middle
				middle = max(middle, d)
			}
			prev = l
		}
		if fastest < 0.04 || turning > fastest/2 || middle < 0.95*fastest {
			t.Errorf("period %v: %v of the range a frame at the quickest, %v at the turns, %v in the middle: want quick through the middle and slow at the turns", p, fastest, turning, middle)
		}
	}
}

// The bars' periods are each between 0.7 and 1.2 s and none divides another,
// so the three drift in and out of step and the pattern does not visibly
// repeat.
func TestEqBarsHaveTheirOwnPeriods(t *testing.T) {
	for i, a := range eqBars {
		if a.period < 700*time.Millisecond || a.period > 1200*time.Millisecond {
			t.Errorf("bar %d: period %v, want 0.7 to 1.2 s", i, a.period)
		}
		for j, b := range eqBars {
			if i != j && a.period%b.period == 0 {
				t.Errorf("bar %d's period %v is a multiple of bar %d's, %v", i, a.period, j, b.period)
			}
		}
	}
}

type mediaEqApp struct {
	playing bool
	p       *MotionPolicy
}

func (a *mediaEqApp) view(c *ui.Context) {
	theme.Apply(c)
	ui.Box(c).Padding(10).Children(func() {
		EqualiserBars(c.Key("eq"), a.playing, a.p)
	})
}

// mediaBarHeights are the amber pixels in each bar's column, from the bottom.
func mediaBarHeights(tt *ui.Tester) [3]int {
	amber := theme.C("amber")
	var h [3]int
	img := tt.Image()
	for i := range h {
		x := 10 + i*(eqBarW+eqGap) + 1
		for y := 0; y < 40; y++ {
			c := img.RGBAAt(x, y)
			if c.R == amber.R && c.G == amber.G && c.B == amber.B {
				h[i]++
			}
		}
	}
	return h
}

// mediaBarCoverage is how tall each bar is in device pixels, partly covered
// pixels counted by how much of them is amber: a top edge that sits between
// two pixels is a fraction, not a whole number.
func mediaBarCoverage(tt *ui.Tester, scale float64) [3]float64 {
	amber := theme.C("amber")
	img := tt.Image()
	bg := img.RGBAAt(0, 0)
	span := float64(bg.B) - float64(amber.B)
	var h [3]float64
	for i := range h {
		x := int((10 + float64(i)*(eqBarW+eqGap) + eqBarW/2) * scale)
		for y := 0; y < int(40*scale); y++ {
			h[i] += min(max((float64(bg.B)-float64(img.RGBAAt(x, y).B))/span, 0), 1)
		}
	}
	return h
}

func TestEqualiserBarsAreStillWhenPaused(t *testing.T) {
	theme.SetMode(theme.ModeLight)
	defer theme.SetMode(theme.ModeSystem)
	tt := ui.NewTester((&mediaEqApp{}).view, 60, 40)
	tt.Frame()
	if _, ok := tt.Find("Paused"); !ok {
		t.Error("not named Paused")
	}
	// 60%, 100% and 40% of 12 DIPs.
	h := mediaBarHeights(tt)
	// Exact pixel heights are Linux's rasteriser (as the goldens are); the
	// stillness below holds everywhere.
	if runtime.GOOS == "linux" && h != [3]int{7, 12, 5} && h != [3]int{8, 12, 5} && h != [3]int{7, 12, 4} {
		t.Errorf("bar heights %v, want about 7, 12, 5", h)
	}
	tt.Advance(time.Second)
	tt.Frame()
	if got := mediaBarHeights(tt); got != h {
		t.Errorf("a paused equaliser moved: %v then %v", h, got)
	}
}

// barsPaintedAlone counts what a page around the bars builds and paints
// whole, over the seconds the bars play: a frame that paints the bars again
// alone builds nothing and runs none of the page's own Draw callbacks.
type barsPaintedAlone struct{ builds, whole int }

func TestEqualiserBarsMoveSmoothlyByPaintingThemselvesAlone(t *testing.T) {
	theme.SetMode(theme.ModeLight)
	defer theme.SetMode(theme.ModeSystem)
	// At 1x the short bar's top travels a few pixels in half a second: a top
	// edge rounded to whole pixels moves a handful of times a second and
	// reads as stutter at any frame rate. The edge must move by fractions of
	// a pixel instead, so that every frame is a new picture, at 1x and at 2x.
	for _, scale := range []float32{1, 2} {
		t.Run(fmt.Sprintf("scale %v", scale), func(t *testing.T) {
			a := &mediaEqApp{playing: true}
			var n barsPaintedAlone
			tt := ui.NewTester(func(c *ui.Context) {
				n.builds++
				a.view(c)
				ui.Box(c.Key("page")).Absolute().Left(0).Top(0).Size(1, 1).Draw(func(*ui.Painter, ui.Rect) { n.whole++ })
			}, 60, 40)
			tt.SetScale(scale)
			tt.Frame()
			if _, ok := tt.Find("Now playing"); !ok {
				t.Error("not named Now playing")
			}
			n = barsPaintedAlone{}
			s := float64(scale)
			const frames = 120 // two seconds at 60 Hz, with no frame asked for from outside
			changes, fractional := 0, 0
			prevPix := append([]byte(nil), tt.Image().Pix...)
			prev := mediaBarCoverage(tt, s)
			lowest, highest := prev, prev
			var widest float64
			for range frames {
				tt.Advance(time.Second / 60)
				if pix := tt.Image().Pix; !bytes.Equal(pix, prevPix) {
					changes++
					prevPix = append(prevPix[:0], pix...)
				}
				cov := mediaBarCoverage(tt, s)
				for i := range cov {
					if f := cov[i] - math.Floor(cov[i]); f > 0.1 && f < 0.9 {
						fractional++
					}
					widest = max(widest, math.Abs(cov[i]-prev[i]))
					lowest[i], highest[i] = min(lowest[i], cov[i]), max(highest[i], cov[i])
				}
				prev = cov
			}
			if n.builds != 0 || n.whole != 0 {
				t.Errorf("playing bars built the view %d times and painted the page whole %d times in two seconds, want 0 and 0", n.builds, n.whole)
			}
			if changes < frames*9/10 {
				t.Errorf("%d new pictures in %d frames, want one nearly every frame: the bars are stepping", changes, frames)
			}
			if fractional < frames {
				t.Errorf("a bar's top edge sat between two pixels %d times in %d bar-frames, want it to move by fractions of a pixel", fractional, 3*frames)
			}
			// 0.9 DIPs is the most a bar travels in a 60 Hz frame.
			if widest > 0.9*s {
				t.Errorf("a top edge moved %.2f device pixels in one frame, want at most %.2f", widest, 0.9*s)
			}
			for i := range lowest {
				if lowest[i] > 0.35*eqHeight*s || highest[i] < 0.95*eqHeight*s {
					t.Errorf("bar %d went only from %.1f to %.1f device pixels, want about %.1f to %.1f", i, lowest[i], highest[i], eqLowest*eqHeight*s, eqHeight*s)
				}
			}
		})
	}
}

func TestEqualiserBarsCostNoFramesWhenPausedOrReduced(t *testing.T) {
	theme.SetMode(theme.ModeLight)
	defer theme.SetMode(theme.ModeSystem)
	for _, reduced := range []bool{false, true} {
		theme.SetReducedMotion(reduced)
		a := &mediaEqApp{playing: reduced} // reduced motion: playing, but still
		var n barsPaintedAlone
		tt := ui.NewTester(func(c *ui.Context) {
			n.builds++
			a.view(c)
			ui.Box(c.Key("page")).Absolute().Left(0).Top(0).Size(1, 1).Draw(func(*ui.Painter, ui.Rect) { n.whole++ })
		}, 60, 40)
		tt.Frame()
		n = barsPaintedAlone{}
		tt.Advance(2 * time.Second)
		if n != (barsPaintedAlone{}) || tt.FramePending() {
			t.Errorf("reduced=%v playing=%v: a still glyph drew frames: %+v", reduced, a.playing, n)
		}
	}
	theme.SetReducedMotion(false)
}

func TestEqualiserBarsStopWhenCoveredOrMotionIsReduced(t *testing.T) {
	e := newMediaMotionEnv(t)
	a := &mediaEqApp{playing: true, p: e.p}
	tt := ui.NewTester(a.view, 60, 40)
	moves := func() bool {
		tt.Frame()
		first := mediaBarHeights(tt)
		for range 20 {
			tt.Advance(100 * time.Millisecond)
			tt.Frame()
			if mediaBarHeights(tt) != first {
				return true
			}
		}
		return false
	}
	if !moves() {
		t.Fatal("the bars do not move while playing")
	}
	release := e.p.Cover()
	if moves() {
		t.Error("the bars move under Now Playing")
	}
	release()
	if !moves() {
		t.Error("the bars did not resume")
	}
	theme.SetReducedMotion(true)
	defer theme.SetReducedMotion(false)
	if moves() {
		t.Error("the bars move under reduced motion")
	}
}
