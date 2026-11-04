package fakeengine

import (
	"slices"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/playback"
)

func TestPositionFollowsTheClockWhilePlaying(t *testing.T) {
	now := time.Unix(1000, 0)
	e := New()
	e.Now = func() time.Time { return now }
	ctx := t.Context()

	if err := e.Play(ctx, "i.1", 10*time.Second); err != nil {
		t.Fatal(err)
	}
	now = now.Add(5 * time.Second)
	if s := e.Status(); s.State != playback.StatePlaying || s.ItemID != "i.1" || s.Position != 15*time.Second || s.Duration != DefaultLength {
		t.Errorf("playing: %+v", s)
	}
	_ = e.Pause(ctx)
	now = now.Add(time.Minute)
	if s := e.Status(); s.State != playback.StatePaused || s.Position != 15*time.Second {
		t.Errorf("paused: %+v", s)
	}
	_ = e.Resume(ctx)
	_ = e.Seek(ctx, time.Hour) // past the end: clamped
	if s := e.Status(); s.Position != DefaultLength {
		t.Errorf("seek past the end: %+v", s)
	}
	_ = e.SetVolume(ctx, 7)
	if v := e.Status().Volume; v != 1 {
		t.Errorf("volume %v is not clamped", v)
	}
	_ = e.Stop(ctx)
	if s := e.Status(); s.State != playback.StateIdle || s.ItemID != "" {
		t.Errorf("stopped: %+v", s)
	}
	want := []string{"play i.1", "pause", "resume", "seek 1h0m0s", "volume 7", "stop"}
	if got := e.Calls(); !slices.Equal(got, want) {
		t.Errorf("calls %q, want %q", got, want)
	}
	if got := e.Played(); !slices.Equal(got, []string{"i.1"}) {
		t.Errorf("played %q", got)
	}
}

func TestEventsKeepOnlyTheLatest(t *testing.T) {
	e := New()
	ctx := t.Context()
	_ = e.Play(ctx, "a", 0)
	_ = e.Pause(ctx)
	_ = e.Resume(ctx)
	select {
	case ev := <-e.Events():
		if ev.Kind != playback.EventState || ev.Status.State != playback.StatePlaying {
			t.Errorf("event %+v is not the latest", ev)
		}
	default:
		t.Fatal("no event")
	}
	select {
	case ev := <-e.Events():
		t.Errorf("a second event %+v", ev)
	default:
	}
}
