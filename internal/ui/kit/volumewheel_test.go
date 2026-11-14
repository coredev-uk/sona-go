package kit

import (
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
)

// scrolled is a wheel notch (precise false) or a touchpad's scrolling.
func scrolled(dx, dy float32, precise bool) ui.InputEvent {
	return ui.InputEvent{Kind: ui.InputScroll, DX: dx, DY: dy, Precise: precise}
}

func TestVolumeWheelNotchesAreOneStepEach(t *testing.T) {
	tr, play := transportOver(newRecordingSvc(), nil)
	var w VolumeWheel
	// Up, which is DY negative: the view moves up the content.
	if !w.input(tr, scrolled(0, -50, false), false) {
		t.Fatal("a notch was not taken")
	}
	if v := play.State().Volume; math.Abs(v-0.65) > 1e-9 {
		t.Errorf("a notch up: volume %v, want 0.65", v)
	}
	w.input(tr, scrolled(0, 120, false), false) // however far the platform counts a notch
	w.input(tr, scrolled(0, 40, false), false)
	if v := play.State().Volume; math.Abs(v-0.55) > 1e-9 {
		t.Errorf("two notches down: volume %v, want 0.55", v)
	}
}

func TestVolumeWheelClampsAtBothEnds(t *testing.T) {
	svc := newRecordingSvc()
	tr, play := transportOver(svc, nil)
	var w VolumeWheel
	for range 20 {
		w.input(tr, scrolled(0, -40, false), false)
	}
	if v := play.State().Volume; v != 1 {
		t.Errorf("after 20 notches up the volume is %v, want 1", v)
	}
	for range 30 {
		w.input(tr, scrolled(0, 40, false), false)
	}
	if v := play.State().Volume; v != 0 {
		t.Errorf("after 30 notches down the volume is %v, want exactly 0 (muted)", v)
	}
	// Up from muted raises it: no step is lost to the end.
	w.input(tr, scrolled(0, -40, false), false)
	if v := play.State().Volume; math.Abs(v-0.05) > 1e-9 {
		t.Errorf("up from 0: volume %v, want 0.05", v)
	}
	// 8 changes to reach 1 (0.6 → 1), 20 to reach 0, then 1 up: a notch at
	// an end asks nothing of the service.
	if got := svc.wait(t, 8+20+1); len(got) != 29 {
		t.Errorf("the service was asked %d times, want 29: %v", len(got), got)
	}
}

func TestVolumeWheelTakesAHorizontalSwipe(t *testing.T) {
	tr, play := transportOver(newRecordingSvc(), nil)
	var w VolumeWheel
	// Natural scrolling: fingers moving right move the content right, the
	// view left: DX negative, more.
	w.input(tr, scrolled(-100, 3, true), false)
	if v := play.State().Volume; math.Abs(v-0.8) > 1e-9 {
		t.Errorf("a swipe of 100 DIPs right: volume %v, want 0.8 (+20%%)", v)
	}
	w.input(tr, scrolled(50, -4, true), false)
	if v := play.State().Volume; math.Abs(v-0.7) > 1e-9 {
		t.Errorf("a swipe of 50 DIPs left: volume %v, want 0.7", v)
	}
	// Right to left, the slider fills from the right: the other way.
	w.input(tr, scrolled(-50, 0, true), true)
	if v := play.State().Volume; math.Abs(v-0.6) > 1e-9 {
		t.Errorf("right to left, a swipe right: volume %v, want 0.6", v)
	}
}

func TestVolumeWheelTakesTheLargerAxis(t *testing.T) {
	tr, play := transportOver(newRecordingSvc(), nil)
	var w VolumeWheel
	w.input(tr, scrolled(30, -50, false), false) // more up than sideways: up
	if v := play.State().Volume; math.Abs(v-0.65) > 1e-9 {
		t.Errorf("volume %v, want 0.65", v)
	}
}

func TestVolumeWheelAccumulatesAndTurnsAtOnce(t *testing.T) {
	svc := newRecordingSvc()
	tr, play := transportOver(svc, nil)
	var w VolumeWheel
	// 2 DIPs is 0.4%: nothing changes until the travel adds up to 1%.
	for range 2 {
		w.input(tr, scrolled(0, -2, true), false)
	}
	if v := play.State().Volume; v != 0.6 {
		t.Fatalf("1%% not reached, yet the volume is %v", v)
	}
	w.input(tr, scrolled(0, -2, true), false) // 6 DIPs: 1.2%
	if v := play.State().Volume; math.Abs(v-0.61) > 1e-9 {
		t.Errorf("after 6 DIPs the volume is %v, want 0.61", v)
	}
	// Turning drops the 0.2% that waited: 2.5 DIPs down is 0.5% and is nothing yet.
	w.input(tr, scrolled(0, 2.5, true), false)
	if v := play.State().Volume; math.Abs(v-0.61) > 1e-9 {
		t.Errorf("a turn of 0.5%% moved the volume to %v", v)
	}
	w.input(tr, scrolled(0, 2.5, true), false)
	if v := play.State().Volume; math.Abs(v-0.6) > 1e-9 {
		t.Errorf("1%% down: volume %v, want 0.6", v)
	}
	svc.wait(t, 2)
}

func TestVolumeWheelDropsTravelPastAnEnd(t *testing.T) {
	tr, play := transportOver(newRecordingSvc(), nil)
	play.Patch(func(s *services.PlaybackState) { s.Volume = 1 })
	var w VolumeWheel
	w.input(tr, scrolled(0, -500, true), false) // far past the end
	w.input(tr, scrolled(0, 10, true), false)   // and back: 2% down, at once
	if v := play.State().Volume; math.Abs(v-0.98) > 1e-9 {
		t.Errorf("turning at the end: volume %v, want 0.98", v)
	}
}

func TestVolumeWheelTakesMomentumWithoutChangingTheVolume(t *testing.T) {
	tr, play := transportOver(newRecordingSvc(), nil)
	var w VolumeWheel
	ev := scrolled(0, -100, true)
	ev.Momentum = true
	if !w.input(tr, ev, false) {
		t.Error("momentum was left to scroll what is behind")
	}
	if v := play.State().Volume; v != 0.6 {
		t.Errorf("momentum moved the volume to %v", v)
	}
}

func TestVolumeWheelLeavesZoomAndOtherInputAlone(t *testing.T) {
	tr, play := transportOver(newRecordingSvc(), nil)
	var w VolumeWheel
	for _, m := range []ui.Modifiers{ui.Ctrl, ui.Alt, ui.Super} {
		ev := scrolled(0, -40, false)
		ev.Mods = m
		if w.input(tr, ev, false) {
			t.Errorf("scrolling with %v held was taken", m)
		}
	}
	if w.input(tr, ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyUp}, false) {
		t.Error("a key was taken")
	}
	// Shift is a wheel turned sideways on some systems; it still counts.
	ev := scrolled(0, -40, false)
	ev.Mods = ui.Shift
	if !w.input(tr, ev, false) {
		t.Error("shift+wheel was not taken")
	}
	if v := play.State().Volume; math.Abs(v-0.65) > 1e-9 {
		t.Errorf("volume %v, want 0.65", v)
	}
}

func TestVolumeWheelTellsTheOwnerOfEachScroll(t *testing.T) {
	tr, _ := transportOver(newRecordingSvc(), nil)
	at := time.UnixMilli(7)
	var got []time.Time
	w := VolumeWheel{OnScroll: func(t time.Time) { got = append(got, t) }}
	ev := scrolled(0, 0, true) // no travel: still a touch of the control
	ev.Time = at
	w.input(tr, ev, false)
	if len(got) != 1 || !got[0].Equal(at) {
		t.Errorf("OnScroll got %v, want [%v]", got, at)
	}
}
