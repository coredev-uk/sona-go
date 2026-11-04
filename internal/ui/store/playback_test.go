package store

import (
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
)

var epoch = time.UnixMilli(1_000)

// The cases of v0.2.0's player/playback.test.ts, in milliseconds.
func TestLivePosition(t *testing.T) {
	base := EmptyState()
	base.State, base.PositionMs, base.DurationMs, base.At = StatePlaying, 90_000, 318_000, 1_000
	at := func(ms int64) time.Time { return time.UnixMilli(ms) }
	want := func(s services.PlaybackState, now int64, ms int64) {
		t.Helper()
		if got := LivePosition(s, at(now)); got != time.Duration(ms)*time.Millisecond {
			t.Errorf("state %q at %d: %v, want %d ms", s.State, now, got, ms)
		}
	}

	t.Run("extrapolates from the snapshot time while playing", func(t *testing.T) {
		want(base, 1_000, 90_000)
		want(base, 3_500, 92_500)
	})
	t.Run("clamps at the duration", func(t *testing.T) { want(base, 1_000_000, 318_000) })
	t.Run("never rewinds on clock skew", func(t *testing.T) { want(base, 500, 90_000) })
	t.Run("does not clamp when the duration is unknown", func(t *testing.T) {
		s := base
		s.DurationMs = 0
		want(s, 11_000, 100_000)
	})
	t.Run("holds the reported position when not playing", func(t *testing.T) {
		for _, st := range []string{StatePaused, StateLoading, StateIdle, StateEnded, StateError} {
			s := base
			s.State = st
			want(s, 60_000, 90_000)
		}
	})
}

func TestPlaybackFollowsEventsAndExtrapolates(t *testing.T) {
	b := NewBus(nil)
	b.SetPoster(nil)
	now := epoch
	p := NewPlayback(b, func() time.Time { return now })

	if s := p.State(); s.State != StateIdle || s.Index != -1 || s.Volume != 0.8 || !s.Controls.Queue {
		t.Fatalf("empty state %+v", s)
	}
	if p.Position() != 0 {
		t.Errorf("position %v before anything plays", p.Position())
	}

	st := p.State()
	st.State, st.PositionMs, st.DurationMs, st.At, st.Index = StatePlaying, 10_000, 200_000, epoch.UnixMilli(), 2
	b.Emit(services.EventPlaybackState, st)
	b.Emit(services.EventPlaybackQueue, services.QueueState{Version: 7, Index: 2, Items: []services.QueueItem{{}, {}, {}}})

	now = epoch.Add(2500 * time.Millisecond)
	if got := p.Position(); got != 12_500*time.Millisecond {
		t.Errorf("position %v, want 12.5s", got)
	}
	if p.State().Index != 2 || p.Queue().Version != 7 || len(p.Queue().Items) != 3 {
		t.Errorf("mirror: %+v %+v", p.State(), p.Queue())
	}

	// Pausing holds where Go said it paused.
	st.State, st.PositionMs, st.At = StatePaused, 12_000, now.UnixMilli()
	b.Emit(services.EventPlaybackState, st)
	now = now.Add(time.Minute)
	if got := p.Position(); got != 12*time.Second {
		t.Errorf("paused position %v, want 12s", got)
	}
}

func TestPlaybackSeedAndPatch(t *testing.T) {
	b := NewBus(nil)
	b.SetPoster(nil)
	p := NewPlayback(b, nil)
	s := EmptyState()
	s.Volume = 0.3
	p.Seed(s, services.QueueState{Version: 2})
	if p.State().Volume != 0.3 || p.Queue().Version != 2 {
		t.Fatalf("seed: %+v %+v", p.State(), p.Queue())
	}
	p.Patch(func(s *services.PlaybackState) { s.Volume = 0.6 })
	if p.State().Volume != 0.6 {
		t.Errorf("patch: %v", p.State().Volume)
	}
	s.Volume = 0.1
	b.Emit(services.EventPlaybackState, s)
	if p.State().Volume != 0.1 {
		t.Errorf("Go's snapshot must replace the patch: %v", p.State().Volume)
	}
}

func TestLyricsSongID(t *testing.T) {
	if LyricsSongID(nil) != "" || LyricsSongID(&services.QueueItem{CatalogID: "1", Video: true}) != "" {
		t.Error("nothing playing and music videos have no lyrics")
	}
	if LyricsSongID(&services.QueueItem{CatalogID: "1"}) != "1" {
		t.Error("a song's lyrics go with its catalogue ID")
	}
}

func TestUnmutedStartsAtV020sDefaultAndRemembersTheLastNonZeroVolume(t *testing.T) {
	p := NewPlayback(NewBus(nil), nil)
	if got := p.Unmuted(); got != 0.8 {
		t.Errorf("default %v, want 0.8", got)
	}
	p.SetUnmuted(0.35)
	if got := p.Unmuted(); got != 0.35 {
		t.Errorf("got %v after SetUnmuted(0.35)", got)
	}
	p.SetUnmuted(0) // a mute at zero remembers nothing
	if got := p.Unmuted(); got != 0.35 {
		t.Errorf("a zero volume replaced it: %v", got)
	}
}
