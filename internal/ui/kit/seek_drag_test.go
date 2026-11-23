package kit

import (
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

func TestSeekDragShowsTheDraggedPositionAndCommitsOnceOnRelease(t *testing.T) {
	var d SeekDrag
	v := 20.0
	var commits []time.Duration
	live := 10 * time.Second
	var shown time.Duration
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Padding(40).Children(func() {
			shown = d.Pos(live)
			sl := Slider(c.Key("s"), &v, 0, 100, SliderOpts{A11yLabel: "Seek", Width: 200})
			if to, ok := d.Update(sl, v); ok {
				commits = append(commits, to)
			}
		})
	}, 400, 300)
	tt.Frame()
	if shown != live || d.Active() {
		t.Fatalf("idle: shown %v active %v, want the live position", shown, d.Active())
	}
	tt.Press(60, 48)
	tt.Move(120, 48)
	tt.Frame()
	if !d.Active() || shown == live {
		t.Errorf("while dragging: active %v shown %v, want the dragged position", d.Active(), shown)
	}
	if len(commits) != 0 {
		t.Fatalf("%d commits while the pointer is held", len(commits))
	}
	tt.Release(120, 48)
	tt.Frame()
	tt.Frame()
	if len(commits) != 1 {
		t.Fatalf("%d commits after the release, want 1", len(commits))
	}
	if want := time.Duration(v * float64(time.Second)); commits[0] != want {
		t.Errorf("committed %v, want the slider's %v", commits[0], want)
	}
	if d.Active() {
		t.Error("still dragging after the commit")
	}
}

func TestSeekDragCommitsAKeyboardMoveOnce(t *testing.T) {
	var d SeekDrag
	v := 20.0
	var commits []time.Duration
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Padding(40).Children(func() {
			sl := Slider(c.Key("s"), &v, 0, 100, SliderOpts{A11yLabel: "Seek", Step: 5, Width: 200})
			if to, ok := d.Update(sl, v); ok {
				commits = append(commits, to)
			}
		})
	}, 400, 300)
	tt.Frame()
	tt.SetFocused(true)
	tt.Key(0, ui.KeyTab) // onto the slider
	tt.Key(0, ui.KeyRight)
	tt.Frame()
	tt.Frame()
	if len(commits) != 1 {
		t.Fatalf("%d commits after one arrow key, want 1", len(commits))
	}
	if want := time.Duration(v * float64(time.Second)); v <= 20 || commits[0] != want {
		t.Errorf("committed %v with the slider at %v, want the moved value", commits[0], v)
	}
	tt.Frame()
	if len(commits) != 1 || d.Active() {
		t.Errorf("the commit repeated or the drag stayed active: %d commits, active %v", len(commits), d.Active())
	}
}

// seekApp is a slider over a 100 s song whose position is a clock the test
// moves, and counts how often its view is built.
type seekApp struct {
	drag    SeekDrag
	playing bool
	clock   time.Duration // the song's position now
	builds  int
}

const seekAppDur = 100 * time.Second

func (a *seekApp) view(c *ui.Context) {
	a.builds++
	theme.Apply(c)
	v := a.clock.Seconds()
	ui.Column(c).Padding(10).Children(func() {
		Slider(c.Key("seek"), &v, 0, seekAppDur.Seconds(), SliderOpts{
			Width: 300, Range: theme.C("amber"),
			Playhead: a.drag.Playhead(a.playing, seekAppDur, func() time.Duration { return a.clock }, nil),
		})
		a.drag.Tick(c, a.playing, a.clock, seekAppDur)
	})
}

// seekFill is how much of the slider's centre row is amber, in device pixels,
// partly covered pixels counted by their saturation.
func seekFill(tt *ui.Tester, scale float64) float64 {
	img := tt.Image()
	amber := theme.C("amber")
	satAmber := float64(max(amber.R, amber.G, amber.B) - min(amber.R, amber.G, amber.B))
	y := int(18 * scale) // 10 DIP of padding and half of the 16 high slider
	var sum float64
	for x := 0; x < img.Bounds().Dx(); x++ {
		c := img.RGBAAt(x, y)
		sum += min(float64(max(c.R, c.G, c.B)-min(c.R, c.G, c.B))/satAmber, 1)
	}
	return sum
}

func TestPlayheadMovesTheFillBetweenBuilds(t *testing.T) {
	theme.SetMode(theme.ModeLight)
	defer theme.SetMode(theme.ModeSystem)
	for _, scale := range []float32{1, 2} {
		a := &seekApp{playing: true, clock: 20 * time.Second}
		tt := ui.NewTester(a.view, 340, 40)
		tt.SetScale(scale)
		tt.Frame()
		tt.Advance(time.Second) // what the first frames set going has run out
		a.clock += time.Second
		tt.Frame()
		a.builds = 0
		const frame = time.Second / 60
		prev, first := seekFill(tt, float64(scale)), seekFill(tt, float64(scale))
		changed, widest := 0, 0.0
		for range 60 {
			a.clock += frame
			tt.Advance(frame)
			f := seekFill(tt, float64(scale))
			if f > prev+0.001 {
				changed++
			}
			widest = max(widest, f-prev)
			prev = f
		}
		// 288 DIP of track in 100 s: 2.88 DIP a second, a 4 DIP step (the
		// old 250 ms tick was 0.7 of one) is a pixel at 1x.
		want := 288.0 / 100 * float64(scale)
		if math.Abs(prev-first-want) > 0.15*want+0.3 {
			t.Errorf("scale %v: the fill grew %.2f device pixels in a second, want about %.2f", scale, prev-first, want)
		}
		if changed < 54 {
			t.Errorf("scale %v: the fill changed in %d of 60 frames", scale, changed)
		}
		if widest > 0.2*want+0.3 {
			t.Errorf("scale %v: the fill grew %.2f device pixels in one frame, want about %.2f", scale, widest, want/60)
		}
		if a.builds > 2 {
			t.Errorf("scale %v: the view was built %d times in 60 frames", scale, a.builds)
		}
	}
}

func TestPlayheadAsksForFramesOnlyWhileItMoves(t *testing.T) {
	pending := func(a *seekApp) bool {
		tt := ui.NewTester(a.view, 340, 40)
		tt.Frame()
		tt.Advance(time.Second)
		for range 20 {
			a.clock += 50 * time.Millisecond
			tt.Advance(50 * time.Millisecond)
			if tt.FramePending() {
				return true
			}
		}
		return false
	}
	if !pending(&seekApp{playing: true}) {
		t.Error("a playing song's bar asked for no frames")
	}
	if pending(&seekApp{}) {
		t.Error("a paused song's bar asked for frames")
	}
	a := &seekApp{playing: true}
	theme.SetReducedMotion(true)
	defer theme.SetReducedMotion(false)
	if pending(a) {
		t.Error("a bar under reduced motion asked for frames")
	}
	// A bar being dragged shows the pointer's value, not the clock's.
	b := &seekApp{playing: true}
	b.drag.active = true
	theme.SetReducedMotion(false)
	if pending(b) {
		t.Error("a bar being dragged asked for frames")
	}
}

// The text beside the bar is built again when the second it shows changes,
// not four times a second.
func TestTickBuildsAgainAtEachSecond(t *testing.T) {
	a := &seekApp{playing: true, clock: 20400 * time.Millisecond}
	tt := ui.NewTester(a.view, 340, 40)
	tt.Frame()
	tt.Advance(time.Second)
	a.clock = 20400 * time.Millisecond
	tt.Frame()
	a.builds = 0
	var at []time.Duration
	const step = 10 * time.Millisecond
	for elapsed := step; elapsed <= 3*time.Second; elapsed += step {
		before := a.builds
		a.clock += step
		tt.Advance(step)
		if a.builds != before {
			at = append(at, elapsed)
		}
	}
	// The clock reads 21, 22 and 23 s after 600, 1600 and 2600 ms.
	if len(at) != 3 {
		t.Fatalf("built at %v in three seconds, want at each second boundary: three times", at)
	}
	for i, want := range []time.Duration{600, 1600, 2600} {
		if d := at[i] - want*time.Millisecond; d < 0 || d > 3*step {
			t.Errorf("build %d at %v, want just after %v ms", i, at[i], want)
		}
	}
}
