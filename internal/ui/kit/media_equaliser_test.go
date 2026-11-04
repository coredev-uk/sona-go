package kit

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

func TestEqLevelStepsThroughThreeValuesAndBack(t *testing.T) {
	// 900 ms up in three steps (steps(3, jump-none): .35, .675, 1), 900 back.
	want := []struct {
		at    time.Duration
		scale float32
		step  int
	}{
		{0, 0.35, 0},
		{299 * time.Millisecond, 0.35, 0},
		{300 * time.Millisecond, 0.675, 1},
		{599 * time.Millisecond, 0.675, 1},
		{600 * time.Millisecond, 1, 2},
		{899 * time.Millisecond, 1, 2},
		{900 * time.Millisecond, 1, 2},
		{1200 * time.Millisecond, 0.675, 1},
		{1799 * time.Millisecond, 0.35, 0},
		{1800 * time.Millisecond, 0.35, 0}, // the cycle repeats
		{-300 * time.Millisecond, 0.35, 0},
	}
	for _, w := range want {
		scale, step := eqLevel(w.at)
		if d := scale - w.scale; d > 0.001 || d < -0.001 || step != w.step {
			t.Errorf("eqLevel(%v) = %v, %d; want %v, %d", w.at, scale, step, w.scale, w.step)
		}
	}
	// The next step begins within a third of a second, whatever the time.
	for _, at := range []time.Duration{0, 1, 299 * time.Millisecond, 300 * time.Millisecond, 1234 * time.Millisecond} {
		if d := untilEqStep(at); d <= 0 || d > eqPerStep {
			t.Errorf("untilEqStep(%v) = %v", at, d)
		}
	}
}

type eqApp struct {
	playing bool
	p       *MotionPolicy
}

func (a *eqApp) view(c *ui.Context) {
	theme.Apply(c)
	ui.Box(c).Padding(10).Children(func() {
		EqualiserBars(c.Key("eq"), a.playing, a.p)
	})
}

// barHeights are the amber pixels in each bar's column, from the bottom.
func barHeights(tt *ui.Tester) [3]int {
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
	tt := ui.NewTester((&eqApp{}).view, 60, 40)
	tt.Frame()
	if _, ok := tt.Find("Paused"); !ok {
		t.Error("not named Paused")
	}
	// 60%, 100% and 40% of 12 DIPs.
	h := barHeights(tt)
	if h != [3]int{7, 12, 5} && h != [3]int{8, 12, 5} && h != [3]int{7, 12, 4} {
		t.Errorf("bar heights %v, want about 7, 12, 5", h)
	}
	tt.Advance(time.Second)
	tt.Frame()
	if got := barHeights(tt); got != h {
		t.Errorf("a paused equaliser moved: %v then %v", h, got)
	}
}

func TestEqualiserBarsMoveInStepsWhilePlaying(t *testing.T) {
	theme.SetMode(theme.ModeLight)
	defer theme.SetMode(theme.ModeSystem)
	a := &eqApp{playing: true}
	tt := ui.NewTester(a.view, 60, 40)
	tt.Frame()
	if _, ok := tt.Find("Now playing"); !ok {
		t.Error("not named Now playing")
	}
	seen := map[[3]int]bool{}
	for range 40 {
		seen[barHeights(tt)] = true
		tt.Advance(100 * time.Millisecond)
		tt.Frame()
	}
	// Three heights per bar, and only a few combinations: stepped, not smooth.
	if len(seen) < 3 || len(seen) > 9 {
		t.Errorf("%d different bar shapes over four seconds, want a few: %v", len(seen), seen)
	}
}

func TestEqualiserBarsStopWhenCoveredOrMotionIsReduced(t *testing.T) {
	e := newMotionEnv(t)
	a := &eqApp{playing: true, p: e.p}
	tt := ui.NewTester(a.view, 60, 40)
	moves := func() bool {
		tt.Frame()
		first := barHeights(tt)
		for range 20 {
			tt.Advance(100 * time.Millisecond)
			tt.Frame()
			if barHeights(tt) != first {
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
	if moves() {
		t.Error("the bars move under reduced motion")
	}
}
