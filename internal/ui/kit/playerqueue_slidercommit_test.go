package kit

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSliderCommittedWaitsForTheReleaseEvenOffTheSlider(t *testing.T) {
	v := 20.0
	commits := 0
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Padding(40).Children(func() {
			sl := ui.SliderBase(c.Key("s"), &v, 0, 100).Height(16).Width(200)
			if SliderCommitted(sl) {
				commits++
			}
		})
	}, 400, 300)
	tt.Frame()
	tt.Press(60, 48)
	tt.Move(120, 48)
	tt.Frame()
	// Drifted far off the 16 DIP slider, still held.
	tt.Move(150, 250)
	tt.Move(180, 280)
	tt.Frame()
	if commits != 0 {
		t.Fatalf("%d commits while the pointer is still held", commits)
	}
	tt.Release(180, 280)
	tt.Frame()
	tt.Frame()
	if commits != 1 {
		t.Errorf("%d commits after the release, want 1", commits)
	}
	if v <= 20 {
		t.Errorf("the drag did not move the value: %v", v)
	}
	tt.Frame()
	if commits != 1 {
		t.Errorf("the commit repeated: %d", commits)
	}
}
