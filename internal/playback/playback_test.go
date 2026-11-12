package playback

import (
	"math"
	"testing"
	"time"
)

func TestVolumeGainClamps(t *testing.T) {
	for v, want := range map[float64]float64{
		math.NaN():   0,
		math.Inf(-1): 0,
		-0.5:         0,
		0.5:          0.125,
		1:            1,
		math.Inf(1):  1,
	} {
		if got := VolumeGain(v); got != want {
			t.Errorf("VolumeGain(%v) = %v, want %v", v, got, want)
		}
	}
}

func TestPositionAt(t *testing.T) {
	t0 := time.Unix(1000, 0)
	for name, tc := range map[string]struct {
		s    Status
		now  time.Time
		want time.Duration
	}{
		"paused stays":        {Status{State: StatePaused, Position: 5 * time.Second, UpdatedAt: t0}, t0.Add(time.Minute), 5 * time.Second},
		"playing advances":    {Status{State: StatePlaying, Position: 5 * time.Second, Duration: time.Minute, UpdatedAt: t0}, t0.Add(2 * time.Second), 7 * time.Second},
		"clamped to duration": {Status{State: StatePlaying, Position: 5 * time.Second, Duration: 10 * time.Second, UpdatedAt: t0}, t0.Add(time.Minute), 10 * time.Second},
		"unknown duration":    {Status{State: StatePlaying, Position: 5 * time.Second, UpdatedAt: t0}, t0.Add(time.Minute), 65 * time.Second},
		"never negative":      {Status{State: StatePlaying, Position: time.Second, UpdatedAt: t0}, t0.Add(-time.Minute), 0},
	} {
		if got := tc.s.PositionAt(tc.now); got != tc.want {
			t.Errorf("%s: PositionAt = %v, want %v", name, got, tc.want)
		}
	}
}

func TestSendLatestKeepsNewest(t *testing.T) {
	ch := make(chan Event, 2)
	for i := range 5 {
		SendLatest(ch, Event{Kind: EventState, Status: Status{Position: time.Duration(i)}})
	}
	if len(ch) != 2 {
		t.Fatalf("%d queued, want 2", len(ch))
	}
	first, last := <-ch, <-ch
	if first.Status.Position != 3 || last.Status.Position != 4 {
		t.Errorf("kept positions %v and %v, want 3 and 4", first.Status.Position, last.Status.Position)
	}
}
