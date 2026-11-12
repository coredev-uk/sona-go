package kit

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// recordingSvc records the commands a Transport sends, in order.
type recordingSvc struct {
	mu   sync.Mutex
	got  []string
	done chan struct{}
	err  error
}

func newRecordingSvc() *recordingSvc { return &recordingSvc{done: make(chan struct{}, 16)} }

func (r *recordingSvc) record(s string) error {
	r.mu.Lock()
	r.got = append(r.got, s)
	r.mu.Unlock()
	r.done <- struct{}{}
	return r.err
}

func (r *recordingSvc) Toggle(context.Context) error { return r.record("toggle") }
func (r *recordingSvc) Next(context.Context) error   { return r.record("next") }
func (r *recordingSvc) Prev(context.Context) error   { return r.record("prev") }
func (r *recordingSvc) Seek(_ context.Context, ms int64) error {
	return r.record("seek " + time.Duration(ms*int64(time.Millisecond)).String())
}

func (r *recordingSvc) SetVolume(_ context.Context, v float64) error {
	return r.record("volume " + time.Duration(v*float64(time.Second)).String())
}

func (r *recordingSvc) wait(t *testing.T, n int) []string {
	t.Helper()
	for range n {
		select {
		case <-r.done:
		case <-time.After(5 * time.Second):
			t.Fatal("the service was not called")
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	got := slices.Clone(r.got)
	slices.Sort(got) // the commands run on goroutines: no order
	return got
}

func transportOver(svc PlaybackService, fail func(string, error)) (Transport, *store.Playback) {
	play := store.NewPlayback(store.NewBus(nil), nil)
	s := store.EmptyState()
	s.State, s.Current, s.DurationMs, s.Volume = store.StatePlaying, &services.QueueItem{ID: "i.1"}, 200_000, 0.6
	play.Seed(s, services.QueueState{Index: -1})
	return Transport{Svc: svc, Play: play, Fail: fail}, play
}

func TestTransportShowsSeekAndVolumeAtOnceAndClamps(t *testing.T) {
	svc := newRecordingSvc()
	tr, play := transportOver(svc, nil)
	now := time.UnixMilli(5_000)

	tr.SeekTo(90*time.Second, now)
	if s := play.State(); s.PositionMs != 90_000 || s.At != 5_000 {
		t.Errorf("the mirror shows position %d at %d, want 90000 at 5000", s.PositionMs, s.At)
	}
	tr.SetVolume(1.7)
	if v := play.State().Volume; v != 1 {
		t.Errorf("volume %v, want it clamped to 1", v)
	}
	if got := svc.wait(t, 2); !slices.Equal(got, []string{"seek 1m30s", "volume 1s"}) {
		t.Errorf("service got %v", got)
	}
}

func TestTransportSeekByStaysWithinTheSongAndAsksTheGuard(t *testing.T) {
	svc := newRecordingSvc()
	tr, play := transportOver(svc, nil)
	now := time.UnixMilli(0)
	play.Patch(func(s *services.PlaybackState) { s.State = store.StatePaused; s.PositionMs = 195_000 })

	tr.SeekBy(30*time.Second, now, nil)
	tr.SeekBy(-300*time.Second, now, nil)
	refused := 0
	tr.SeekBy(10*time.Second, now, func() bool { refused++; return true })
	play.Patch(func(s *services.PlaybackState) { s.DurationMs = 0 })
	tr.SeekBy(10*time.Second, now, nil) // no length: nothing to seek in

	if got := svc.wait(t, 2); !slices.Equal(got, []string{"seek 0s", "seek 3m20s"}) {
		t.Errorf("service got %v, want a seek to the end and one to the start", got)
	}
	if refused != 1 {
		t.Errorf("the guard was asked %d times, want 1", refused)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if len(svc.got) != 2 {
		t.Errorf("service got %v, want only the two seeks", svc.got)
	}
}

func TestTransportMuteRemembersTheVolumeBefore(t *testing.T) {
	svc := newRecordingSvc()
	tr, play := transportOver(svc, nil)
	tr.ToggleMute()
	if v := play.State().Volume; v != 0 {
		t.Fatalf("muted volume %v", v)
	}
	tr.ToggleMute()
	if v := play.State().Volume; v != 0.6 {
		t.Errorf("unmuted volume %v, want the 0.6 before the mute", v)
	}
	svc.wait(t, 2)
}

func TestTransportFailureGoesToFailWithTheCommandsTitle(t *testing.T) {
	svc := newRecordingSvc()
	svc.err = errors.New("boom")
	failed := make(chan string, 4)
	tr, _ := transportOver(svc, func(what string, err error) {
		if err != svc.err {
			t.Errorf("failure carried %v", err)
		}
		failed <- what
	})
	tr.Toggle()
	tr.Next()
	tr.Prev()
	var got []string
	for range 3 {
		select {
		case w := <-failed:
			got = append(got, w)
		case <-time.After(5 * time.Second):
			t.Fatalf("only %v failed", got)
		}
	}
	slices.Sort(got)
	if want := []string{"Couldn't go back", "Couldn't play or pause", "Couldn't skip"}; !slices.Equal(got, want) {
		t.Errorf("failure titles %v, want %v", got, want)
	}
}
