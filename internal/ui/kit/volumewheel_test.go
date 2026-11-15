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

// start is the volume transportOver begins at.
const start = 0.6

func TestVolumeWheelNotchesAreOneWheelNotchEach(t *testing.T) {
	tr, play := transportOver(newRecordingSvc(), nil)
	var w VolumeWheel
	// Up, which is DY negative: the view moves up the content.
	if !w.input(tr, scrolled(0, -NotchDIP, false), false) {
		t.Fatal("a notch was not taken")
	}
	if v := play.State().Volume; math.Abs(v-(start+WheelNotch)) > 1e-9 {
		t.Errorf("a notch up: volume %v, want %v", v, start+WheelNotch)
	}
	w.input(tr, scrolled(0, 120, false), false) // however far the platform counts a notch
	w.input(tr, scrolled(0, 40, false), false)
	if v := play.State().Volume; math.Abs(v-(start-WheelNotch)) > 1e-9 {
		t.Errorf("two notches down: volume %v, want %v", v, start-WheelNotch)
	}
}

func TestVolumeWheelClampsAtBothEnds(t *testing.T) {
	svc := newRecordingSvc()
	tr, play := transportOver(svc, nil)
	var w VolumeWheel
	up := int(math.Round((1 - start) / WheelNotch)) // notches to reach 1
	for range up + 10 {
		w.input(tr, scrolled(0, -40, false), false)
	}
	if v := play.State().Volume; v != 1 {
		t.Errorf("past the top the volume is %v, want 1", v)
	}
	down := int(math.Round(1 / WheelNotch)) // notches from 1 to 0
	for range down + 10 {
		w.input(tr, scrolled(0, 40, false), false)
	}
	if v := play.State().Volume; v != 0 {
		t.Errorf("past the bottom the volume is %v, want exactly 0 (muted)", v)
	}
	// Up from muted raises it: no step is lost to the end.
	w.input(tr, scrolled(0, -40, false), false)
	if v := play.State().Volume; math.Abs(v-WheelNotch) > 1e-9 {
		t.Errorf("up from 0: volume %v, want %v", v, WheelNotch)
	}
	// A notch at an end asks nothing of the service.
	want := up + down + 1
	if got := svc.wait(t, want); len(got) != want {
		t.Errorf("the service was asked %d times, want %d: %v", len(got), want, got)
	}
}

// A high-resolution wheel, or a compositor's scroll factor, sends a notch in
// fractions, all Precise: the volume must move as for a whole notch.
func TestVolumeWheelHighResolutionNotchesAreOneWheelNotchEach(t *testing.T) {
	svc := newRecordingSvc()
	tr, play := transportOver(svc, nil)
	play.Patch(func(s *services.PlaybackState) { s.Volume = 0 })
	var w VolumeWheel
	const events = 8
	notch := func(dy float32) {
		for range events {
			w.input(tr, scrolled(0, dy, true), false)
		}
	}
	notch(-float32(NotchDIP) / events)
	if v := play.State().Volume; math.Abs(v-WheelNotch) > 1e-9 {
		t.Fatalf("one notch of %d events: volume %v, want %v", events, v, WheelNotch)
	}
	for range int(1/WheelNotch) - 1 {
		notch(-float32(NotchDIP) / events)
	}
	if v := play.State().Volume; v != 1 {
		t.Errorf("after %d notches the volume is %v, want exactly 1", int(1/WheelNotch), v)
	}
	for range int(1 / WheelNotch) {
		notch(float32(NotchDIP) / events)
	}
	if v := play.State().Volume; v != 0 {
		t.Errorf("and back the volume is %v, want exactly 0", v)
	}
}

func TestVolumeWheelTakesAHorizontalSwipe(t *testing.T) {
	tr, play := transportOver(newRecordingSvc(), nil)
	var w VolumeWheel
	d := float32(2 * NotchDIP) // two notches of travel
	// Natural scrolling: fingers moving right move the content right, the
	// view left: DX negative, more.
	w.input(tr, scrolled(-d, 3, true), false)
	if v := play.State().Volume; math.Abs(v-(start+2*WheelNotch)) > 1e-9 {
		t.Errorf("a swipe of %v DIPs right: volume %v, want %v", d, v, start+2*WheelNotch)
	}
	w.input(tr, scrolled(d/2, -4, true), false)
	if v := play.State().Volume; math.Abs(v-(start+WheelNotch)) > 1e-9 {
		t.Errorf("a swipe of %v DIPs left: volume %v, want %v", d/2, v, start+WheelNotch)
	}
	// Right to left, the slider fills from the right: the other way.
	w.input(tr, scrolled(-d/2, 0, true), true)
	if v := play.State().Volume; math.Abs(v-start) > 1e-9 {
		t.Errorf("right to left, a swipe right: volume %v, want %v", v, start)
	}
}

func TestVolumeWheelTakesTheLargerAxis(t *testing.T) {
	tr, play := transportOver(newRecordingSvc(), nil)
	var w VolumeWheel
	w.input(tr, scrolled(30, -50, false), false) // more up than sideways: up
	if v := play.State().Volume; math.Abs(v-(start+WheelNotch)) > 1e-9 {
		t.Errorf("volume %v, want %v", v, start+WheelNotch)
	}
}

func TestVolumeWheelAccumulatesAndTurnsAtOnce(t *testing.T) {
	svc := newRecordingSvc()
	tr, play := transportOver(svc, nil)
	var w VolumeWheel
	// 5 DIPs is 0.2%: nothing changes until the travel adds up to 1%.
	for range 2 {
		w.input(tr, scrolled(0, -5, true), false)
	}
	if v := play.State().Volume; v != start {
		t.Fatalf("1%% not reached, yet the volume is %v", v)
	}
	w.input(tr, scrolled(0, -17.5, true), false) // 27.5 DIPs: 1.1%
	if v := play.State().Volume; math.Abs(v-(start+wheelGrain)) > 1e-9 {
		t.Errorf("after 27.5 DIPs the volume is %v, want %v", v, start+wheelGrain)
	}
	// Turning drops the 0.1% that waited: 12.5 DIPs down is 0.5% and is nothing yet.
	w.input(tr, scrolled(0, 12.5, true), false)
	if v := play.State().Volume; math.Abs(v-(start+wheelGrain)) > 1e-9 {
		t.Errorf("a turn of 0.5%% moved the volume to %v", v)
	}
	w.input(tr, scrolled(0, 12.5, true), false)
	if v := play.State().Volume; math.Abs(v-start) > 1e-9 {
		t.Errorf("1%% down: volume %v, want %v", v, start)
	}
	svc.wait(t, 2)
}

func TestVolumeWheelDropsTravelPastAnEnd(t *testing.T) {
	tr, play := transportOver(newRecordingSvc(), nil)
	play.Patch(func(s *services.PlaybackState) { s.Volume = 1 })
	var w VolumeWheel
	w.input(tr, scrolled(0, -500, true), false)     // far past the end
	w.input(tr, scrolled(0, NotchDIP, true), false) // and back: a notch down, at once
	if v := play.State().Volume; math.Abs(v-(1-WheelNotch)) > 1e-9 {
		t.Errorf("turning at the end: volume %v, want %v", v, 1-WheelNotch)
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
	if v := play.State().Volume; v != start {
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
	if v := play.State().Volume; math.Abs(v-(start+WheelNotch)) > 1e-9 {
		t.Errorf("volume %v, want %v", v, start+WheelNotch)
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
