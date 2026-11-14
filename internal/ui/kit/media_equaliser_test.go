package kit

import (
	"runtime"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

func TestEqLevelEasesBetweenLowestAndFull(t *testing.T) {
	near := func(got, want float32) bool { return got-want < 0.001 && want-got < 0.001 }
	for _, w := range []struct {
		at    time.Duration
		scale float32
	}{
		{0, eqLowest},
		{900 * time.Millisecond, 1},
		{1800 * time.Millisecond, eqLowest}, // the cycle repeats
		{450 * time.Millisecond, (eqLowest + 1) / 2},
		{1350 * time.Millisecond, (eqLowest + 1) / 2},
		{-900 * time.Millisecond, 1},
	} {
		if got := eqLevel(w.at); !near(got, w.scale) {
			t.Errorf("eqLevel(%v) = %v, want %v", w.at, got, w.scale)
		}
	}
	// Rising to the half, falling after it, and never a jump of more than a
	// few percent in a 30 fps frame: moving, not stepping.
	prev := eqLevel(0)
	for at := time.Second / eqFPS; at <= eqPeriod; at += time.Second / eqFPS {
		l := eqLevel(at)
		if (at <= eqPeriod/2) != (l >= prev) && !near(l, prev) {
			t.Errorf("eqLevel is %v then %v at %v, not rising then falling", prev, l, at)
		}
		if d := l - prev; d > 0.1 || d < -0.1 {
			t.Errorf("eqLevel jumps %v in a frame at %v", d, at)
		}
		prev = l
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
	a := &mediaEqApp{playing: true}
	var n barsPaintedAlone
	tt := ui.NewTester(func(c *ui.Context) {
		n.builds++
		a.view(c)
		ui.Box(c.Key("page")).Absolute().Left(0).Top(0).Size(1, 1).Draw(func(*ui.Painter, ui.Rect) { n.whole++ })
	}, 60, 40)
	tt.Frame()
	if _, ok := tt.Find("Now playing"); !ok {
		t.Error("not named Now playing")
	}
	n = barsPaintedAlone{}
	seen := map[[3]int]bool{}
	changes, last := 0, mediaBarHeights(tt)
	for range 60 { // two seconds in 33 ms steps, with no frame asked for from outside
		tt.Advance(33 * time.Millisecond)
		h := mediaBarHeights(tt)
		seen[h] = true
		if h != last {
			changes++
		}
		last = h
	}
	if n.builds != 0 || n.whole != 0 {
		t.Errorf("playing bars built the view %d times and painted the page whole %d times in two seconds, want 0 and 0", n.builds, n.whole)
	}
	// Bars of 12 DIPs at the tallest move through many heights, not three.
	if len(seen) < 10 || changes < 12 {
		t.Errorf("%d different bar shapes and %d changes in two seconds, want a smooth movement: %v", len(seen), changes, seen)
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
