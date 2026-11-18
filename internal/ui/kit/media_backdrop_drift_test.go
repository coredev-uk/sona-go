package kit

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// The drift asks for its next paint when the picture has moved a device
// pixel, not every frame: at 4K it is a few repaints a second.
func TestBackdropDriftRepaintsPerDevicePixelNotPerFrame(t *testing.T) {
	d := &BackdropDrift{Period: 40 * time.Second, Scale: 1.08, X: -0.01, Y: -0.015}
	r := ui.Rect{W: 1920, H: 1080}
	_, _, _, next := d.at(d.Start.Add(10*time.Second), r, 2)
	if next < 50*time.Millisecond {
		t.Errorf("next paint in %v: that is a repaint nearly every frame", next)
	}
	_, _, _, next = d.at(d.Start, r, 2)
	if next < time.Second/60 {
		t.Errorf("next paint in %v, under a frame", next)
	}
	s, dx, dy, _ := d.at(d.Start.Add(40*time.Second), r, 2)
	if s < 1.079 || dx > -19 || dy > -16 {
		t.Errorf("at the far end the pose is %v %v %v", s, dx, dy)
	}
	s, dx, dy, _ = d.at(d.Start.Add(80*time.Second), r, 2)
	if s > 1.001 || dx < -0.1 || dy < -0.1 {
		t.Errorf("back at rest the pose is %v %v %v", s, dx, dy)
	}
}
