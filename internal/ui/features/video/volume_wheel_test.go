package video

import (
	"math"
	"testing"
	"time"
)

// The video view's volume takes the wheel as the bar's does, through the same
// transport, and the controls it fades come back for it: the pointer rests
// over the control (moving it would wake them), they fade, and the wheel does.
func TestWheelOverTheViewsVolumeChangesItAndWakesTheControls(t *testing.T) {
	r := viewRig(t)
	r.open()
	for _, over := range []string{"Volume", "Mute"} {
		b, ok := r.tt.Find(over)
		if !ok {
			t.Fatalf("no %q: %q", over, r.tt.Texts())
		}
		x, y := b.X+b.W/2, b.Y+b.H/2
		r.tt.Move(x, y)
		r.pictures(500 * time.Millisecond)
		r.pictures(4 * time.Second)
		if r.v.view.alpha != 0 {
			t.Fatalf("over %s: the controls did not fade: %v", over, r.v.view.alpha)
		}
		before := r.env.Play.State().Volume
		r.tt.Wheel(x, y, 0, -40)
		r.tt.Advance(400 * time.Millisecond)
		if v := r.env.Play.State().Volume; math.Abs(v-(before+0.05)) > 1e-9 {
			t.Errorf("over %s: volume %v, want %v", over, v, before+0.05)
		}
		if r.v.view.alpha != 1 {
			t.Errorf("over %s: the controls stayed faded: %v", over, r.v.view.alpha)
		}
		r.svc.Wait(t, 1)
	}
}
