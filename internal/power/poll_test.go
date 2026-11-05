package power

import (
	"sync/atomic"
	"testing"
	"time"
)

// pollRig is a Poller over a power source the test flips, driven by ticks
// the test sends: an unbuffered send returns only once Run is back at its
// select, so the read it triggered is finished.
type pollRig struct {
	battery atomic.Bool
	reads   atomic.Int32
	ticks   chan time.Time
	got     chan State
	ended   chan struct{}
	p       *Poller
}

func newPollRig(t *testing.T, onBattery bool) *pollRig {
	t.Helper()
	r := &pollRig{ticks: make(chan time.Time), got: make(chan State, 8), ended: make(chan struct{})}
	r.battery.Store(onBattery)
	r.p = NewPoller(PollOptions{
		OnBattery: func() bool { r.reads.Add(1); return r.battery.Load() },
		Ticks:     r.ticks,
	})
	go func() {
		defer close(r.ended)
		r.p.Run(func(st State) { r.got <- st })
	}()
	t.Cleanup(func() { _ = r.p.Close() })
	return r
}

// settle returns once every tick sent before it has been fully handled.
func (r *pollRig) settle() {
	r.ticks <- time.Time{}
	r.ticks <- time.Time{}
}

func TestPollerStartsFromTheSourceAsRead(t *testing.T) {
	for _, tc := range []struct {
		name      string
		onBattery bool
		want      State
	}{
		{"mains", false, State{Source: SourceAC}},
		{"battery", true, State{OnBattery: true, Source: SourceBattery, UPower: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newPollRig(t, tc.onBattery)
			if got := r.p.State(); got != tc.want {
				t.Fatalf("State() = %+v, want %+v", got, tc.want)
			}
			if r.reads.Load() != 1 {
				t.Fatalf("NewPoller read the source %d times, want once", r.reads.Load())
			}
		})
	}
}

func TestPollerReportsEachChangeOnceAndOnlyChanges(t *testing.T) {
	r := newPollRig(t, false)

	r.settle() // nothing changed: nothing is said
	if len(r.got) != 0 {
		t.Fatalf("unchanged source reported %d changes", len(r.got))
	}

	r.battery.Store(true) // unplugged
	r.settle()
	if len(r.got) != 1 {
		t.Fatalf("unplugging reported %d changes, want 1", len(r.got))
	}
	if st := <-r.got; !st.OnBattery || st.Source != SourceBattery {
		t.Fatalf("unplugged: %+v", st)
	}
	if !r.p.State().OnBattery {
		t.Fatal("State() still reports mains power after unplugging")
	}

	r.settle() // still on battery: no repeat
	if len(r.got) != 0 {
		t.Fatalf("a repeated reading reported again: %d", len(r.got))
	}

	r.battery.Store(false) // plugged in again
	r.settle()
	if st := <-r.got; st.OnBattery || st.Source != SourceAC {
		t.Fatalf("plugged in: %+v", st)
	}
}

// The state a poller reports never makes Diagnostics note a disagreement
// with UPower, which a poller doesn't have.
func TestPollerStateHasNoUPowerDisagreement(t *testing.T) {
	for _, onBattery := range []bool{false, true} {
		if st := pollState(onBattery); st.UPower != st.OnBattery || st.Ignored != 0 {
			t.Errorf("pollState(%v) = %+v", onBattery, st)
		}
	}
}

func TestPollerRefreshReadsWithoutATick(t *testing.T) {
	r := newPollRig(t, false)
	r.battery.Store(true)
	r.p.Refresh()
	if st := <-r.got; !st.OnBattery {
		t.Fatalf("after Refresh: %+v", st)
	}
}

func TestPollerRefreshNeverBlocks(t *testing.T) {
	// No Run: Refresh must not wait for anyone, however often it is asked.
	p := NewPoller(PollOptions{OnBattery: func() bool { return false }, Ticks: make(chan time.Time)})
	for range 100 {
		p.Refresh()
	}
}

func TestPollerCloseEndsRunAndIsRepeatable(t *testing.T) {
	r := newPollRig(t, false)
	if err := r.p.Close(); err != nil {
		t.Fatal(err)
	}
	<-r.ended
	if err := r.p.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestPollerClosedBeforeRunReturnsAtOnce(t *testing.T) {
	p := NewPoller(PollOptions{OnBattery: func() bool { return false }, Ticks: make(chan time.Time)})
	_ = p.Close()
	p.Run(func(State) { t.Error("a closed poller reported a change") })
}

func TestPollerUsesItsOwnTimerWithoutTicks(t *testing.T) {
	p := NewPoller(PollOptions{OnBattery: func() bool { return false }, Every: time.Hour})
	defer func() { _ = p.Close() }()
	if p.stop == nil {
		t.Fatal("no timer was started")
	}
}
