package kit

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// A Tester draws in memory at a refresh rate it does not set, so it cannot
// show a 250 Hz GPU window; what it shows is what each drawing asks MyGo for
// (FrameProbe) while 4 ms steps of a second go by.
func TestSlowMotionAsksForAtMost60FramesASecondOnAGPU(t *testing.T) {
	theme.SetMode(theme.ModeLight)
	defer theme.SetMode(theme.ModeSystem)
	seek := &seekApp{playing: true}
	for _, c := range []struct {
		name string
		view func(c *ui.Context)
	}{
		{"equaliser", (&mediaEqApp{playing: true}).view},
		{"spinner", func(c *ui.Context) {
			theme.Apply(c)
			ui.Box(c).Padding(10).Children(func() { SpinIcon(c.Key("spin"), theme.Icon("loader-circle"), 16, theme.C("ink"), true) })
		}},
		{"playhead", seek.view},
		{"skeleton", func(c *ui.Context) {
			theme.Apply(c)
			Skeleton(c.Key("sk"), 100, 12)
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			probe, stop := ProbeFrames(true)
			defer stop()
			tt := ui.NewTester(c.view, 400, 100)
			tt.Frame()
			probe.Asked = nil
			for range 250 { // 1 s of a 250 Hz display
				tt.Advance(4 * time.Millisecond)
			}
			if len(probe.Asked) == 0 {
				t.Fatal("asked for no frames: the motion did not run")
			}
			if n := len(probe.Asked); n > 61 {
				t.Errorf("painted %d times in a second, want at most about 60", n)
			}
			for _, fps := range probe.Asked {
				if fps <= 0 || fps > 60 {
					t.Fatalf("asked for %v frames a second, want at most 60 (0 is the display's rate)", fps)
				}
			}
		})
	}
}

// The display's rate stays for what the eye follows closely, and the CPU
// rasteriser has 30 for both.
func TestAnimationFramesAskForTheDisplaysRateOnAGPUAndAmbientOnesFor60(t *testing.T) {
	for _, c := range []struct {
		name        string
		accelerated bool
		ask         func(*ui.Painter)
		want        float32
	}{
		{"sweep on a GPU", true, AnimationFrames, 0},
		{"ambient on a GPU", true, AmbientFrames, 60},
		{"sweep on the CPU", false, AnimationFrames, 30},
		{"ambient on the CPU", false, AmbientFrames, 30},
	} {
		t.Run(c.name, func(t *testing.T) {
			probe, stop := ProbeFrames(c.accelerated)
			defer stop()
			tt := ui.NewTester(func(ct *ui.Context) {
				ui.Box(ct).Size(10, 10).Draw(func(pt *ui.Painter, _ ui.Rect) { c.ask(pt) })
			}, 40, 40)
			tt.Frame()
			tt.Advance(100 * time.Millisecond)
			if len(probe.Asked) == 0 {
				t.Fatal("asked for no frames")
			}
			for _, fps := range probe.Asked {
				if fps != c.want {
					t.Fatalf("asked for %v, want %v", fps, c.want)
				}
			}
		})
	}
}
