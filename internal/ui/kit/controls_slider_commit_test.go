package kit

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// commitRig is a slider whose commits are counted.
func commitRig(t *testing.T) (tt *ui.Tester, v *float64, commits *int) {
	t.Helper()
	val, n, active := 50.0, 0, false
	tt = controlsTester(t, 300, 100, func(c *ui.Context) {
		sl := Slider(c, &val, 0, 100, SliderOpts{A11yLabel: "Seek", Width: 200})
		if sl.Changed() {
			active = true
		}
		if SliderCommitted(sl, &active) {
			n++
		}
	})
	return tt, &val, &n
}

func TestSliderCommittedOnceAKeyMovedIt(t *testing.T) {
	tt, v, n := commitRig(t)
	tt.Key(0, ui.KeyTab)
	tt.Key(0, ui.KeyRight)
	if *v != 51 || *n != 1 {
		t.Errorf("a key moved it to %v with %d commits, want 51 and 1", *v, *n)
	}
	tt.Frame()
	if *n != 1 {
		t.Errorf("commits %d after more frames", *n)
	}
}

func TestSliderCommittedOnceWhenAClickLetsGo(t *testing.T) {
	tt, v, n := commitRig(t)
	r := controlsFind(t, tt, "Seek")
	tt.ClickAt(r.X+6+(r.W-12)*0.25, r.Y+r.H/2)
	if *v < 24 || *v > 26 || *n != 1 {
		t.Errorf("a click set %v with %d commits, want 25 and 1", *v, *n)
	}
}

func TestSliderDragThatLeavesTheTrackCommitsOnlyOnRelease(t *testing.T) {
	tt, v, n := commitRig(t)
	r := controlsFind(t, tt, "Seek")
	y := r.Y + r.H/2
	tt.Press(r.X+6+(r.W-12)*0.25, y)
	// The pointer drifts well off the 16 DIP track, button still down.
	for i := range 10 {
		tt.Move(r.X+6+(r.W-12)*(0.25+0.05*float32(i)), y+60)
		if *n != 0 {
			t.Fatalf("committed during the drag, at step %d (value %v)", i, *v)
		}
	}
	tt.Release(r.X+6+(r.W-12)*0.7, y+60)
	if *n != 1 {
		t.Errorf("%d commits after the release, want 1 (value %v)", *n, *v)
	}
}
