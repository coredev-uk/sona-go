package store

import "testing"

// fakeHold counts the holds taken and given back.
type fakeHold struct{ taken, released int }

func (f *fakeHold) hold() func() {
	f.taken++
	return func() { f.released++ }
}

func TestKeepScreenOnUnboundDoesNothing(t *testing.T) {
	w := NewWindow()
	w.KeepScreenOn(true)
	w.KeepScreenOn(false)
}

func TestKeepScreenOnTakesOneHoldAndReleasesOnce(t *testing.T) {
	f := &fakeHold{}
	w := NewWindow()
	w.BindScreen(f.hold)

	w.KeepScreenOn(false) // off while off is nothing
	if f.taken != 0 || f.released != 0 {
		t.Fatalf("off while off: taken %d, released %d", f.taken, f.released)
	}
	w.KeepScreenOn(true)
	w.KeepScreenOn(true)
	if f.taken != 1 || f.released != 0 {
		t.Fatalf("on twice: taken %d, released %d, want 1 and 0", f.taken, f.released)
	}
	w.KeepScreenOn(false)
	w.KeepScreenOn(false)
	if f.taken != 1 || f.released != 1 {
		t.Fatalf("off twice: taken %d, released %d, want 1 and 1", f.taken, f.released)
	}
	w.KeepScreenOn(true)
	if f.taken != 2 {
		t.Fatalf("on again took %d holds in all, want 2", f.taken)
	}
}

func TestKeepScreenOnNilReleaseStillCounts(t *testing.T) {
	taken := 0
	w := NewWindow()
	w.BindScreen(func() func() { taken++; return nil })
	w.KeepScreenOn(true)
	w.KeepScreenOn(true)
	w.KeepScreenOn(false)
	if taken != 1 {
		t.Fatalf("took %d holds, want 1", taken)
	}
}
