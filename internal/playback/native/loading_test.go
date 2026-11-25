//go:build linux || (cgo && (darwin || windows))

package native

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/cdmhost"
)

// hostTrack is the track the host last loaded.
func (f *fakeHost) hostTrack() uint64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.track
}

// starved makes f (and the hosts started after it) report no first audio, so
// no EventStarted comes: a reload then stays loading until a position says
// audio is playing.
func (h *harness) starved(f *fakeHost) {
	h.mu.Lock()
	h.starve = true
	h.mu.Unlock()
	f.mu.Lock()
	f.starve = true
	f.mu.Unlock()
}

// deliver hands the engine an event of f's current track, as its event loop
// would, and returns once it is applied.
func (h *harness) deliver(f *fakeHost, kind cdmhost.EventKind, pos time.Duration, buffering bool) {
	h.e.handle(f, 0, cdmhost.Event{Kind: kind, Track: f.hostTrack(), Position: pos, At: time.Now(), Buffering: buffering})
}

// stuckLoading checks a loading item against every event that must leave it
// loading, and the one that ends it: the engine says playing, with the
// position the host reported.
func (h *harness) stuckLoading(t *testing.T, f *fakeHost) {
	t.Helper()
	st := h.e.Status()
	if st.State != playback.StateLoading {
		t.Fatalf("status %+v, want loading", st)
	}
	from := st.Position

	// A position that has not moved: nothing is heard yet.
	h.deliver(f, cdmhost.EventPosition, from, false)
	if st := h.e.Status(); st.State != playback.StateLoading {
		t.Fatalf("a position that did not advance cleared loading: %+v", st)
	}
	// A position of another track (a seek's or reload's predecessor).
	h.e.handle(f, 0, cdmhost.Event{Kind: cdmhost.EventPosition, Track: f.hostTrack() - 1, Position: from + 7*time.Second, At: time.Now()})
	if st := h.e.Status(); st.State != playback.StateLoading {
		t.Fatalf("a position of an older track cleared loading: %+v", st)
	}
	// Audio moving on: loading is over, with no EventStarted.
	h.deliver(f, cdmhost.EventPosition, from+5*time.Second, false)
	if st := h.e.Status(); st.State != playback.StatePlaying || st.Position != from+5*time.Second {
		t.Fatalf("status %+v, want playing at %v", st, from+5*time.Second)
	}
}

// A song that plays on after a seek, a restart or a reload without the host
// ever sending EventStarted is playing, not loading.
func TestPositionEndsLoadingWithoutStarted(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func(t *testing.T) (*harness, *fakeHost)
	}{
		{"seek", func(t *testing.T) (*harness, *fakeHost) {
			h := newHarness(t, -1, false)
			if err := h.e.Play(ctx(t), "A", 0); err != nil {
				t.Fatal(err)
			}
			f := h.host(0)
			h.starved(f)
			if err := h.e.Seek(ctx(t), 12*time.Second); err != nil {
				t.Fatal(err)
			}
			return h, f
		}},
		{"seek during an underrun", func(t *testing.T) (*harness, *fakeHost) {
			h := newHarness(t, -1, false)
			if err := h.e.Play(ctx(t), "A", 0); err != nil {
				t.Fatal(err)
			}
			f := h.host(0)
			h.deliver(f, cdmhost.EventBuffering, 3*time.Second, true)
			if st := h.e.Status(); st.State != playback.StateLoading {
				t.Fatalf("underrun: %+v", st)
			}
			h.starved(f)
			if err := h.e.Seek(ctx(t), 12*time.Second); err != nil {
				t.Fatal(err)
			}
			return h, f
		}},
		{"seek again while loading", func(t *testing.T) (*harness, *fakeHost) {
			h := newHarness(t, -1, false)
			if err := h.e.Play(ctx(t), "A", 0); err != nil {
				t.Fatal(err)
			}
			f := h.host(0)
			h.starved(f)
			for _, to := range []time.Duration{8 * time.Second, 20 * time.Second} {
				if err := h.e.Seek(ctx(t), to); err != nil {
					t.Fatal(err)
				}
			}
			return h, f
		}},
		{"restart on a new host", func(t *testing.T) (*harness, *fakeHost) {
			h := newHarness(t, -1, false)
			if err := h.e.Play(ctx(t), "A", 0); err != nil {
				t.Fatal(err)
			}
			if err := h.e.Seek(ctx(t), 12*time.Second); err != nil {
				t.Fatal(err)
			}
			h.starved(h.host(0))
			h.host(0).exit(errors.New("signal: killed"))
			eventually(t, "second host loaded", func() bool {
				f := h.host(1)
				if f == nil {
					return false
				}
				loads, _, _ := f.snapshot()
				return len(loads) == 1
			})
			return h, h.host(1)
		}},
		{"spatial reload", func(t *testing.T) (*harness, *fakeHost) {
			h, _ := spatialHarness(t, false)
			if err := h.e.Play(ctx(t), "S1", 0); err != nil {
				t.Fatal(err)
			}
			f := h.host(0)
			h.starved(f)
			h.e.SetSpatial(true)
			eventually(t, "the spatial mix", func() bool { return h.e.Status().Spatial })
			return h, f
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, f := tc.run(t)
			h.stuckLoading(t, f)
		})
	}
}

// The position that ends loading also releases the Play that waits for first
// audio.
func TestPositionReleasesAWaitingPlay(t *testing.T) {
	h := newHarness(t, -1, false)
	if err := h.e.Play(ctx(t), "A", 0); err != nil {
		t.Fatal(err)
	}
	f := h.host(0)
	h.starved(f)
	done := make(chan error, 1)
	go func() { done <- h.e.Play(context.Background(), "B", 0) }()
	eventually(t, "B told to play", func() bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.playing && len(f.loads) == 2
	})
	select {
	case err := <-done:
		t.Fatalf("Play returned before any audio: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	h.deliver(f, cdmhost.EventPosition, 2*time.Second, false)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Play = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Play still waiting after the position")
	}
	if st := h.e.Status(); st.State != playback.StatePlaying || st.ItemID != "B" {
		t.Fatalf("status %+v", st)
	}
}

// A real underrun shows loading and keeps showing it while the position
// stands still; playing returns with the host's word that it resumed.
func TestUnderrunStaysLoadingUntilAudioMoves(t *testing.T) {
	h := newHarness(t, -1, false)
	if err := h.e.Play(ctx(t), "A", 0); err != nil {
		t.Fatal(err)
	}
	f := h.host(0)
	stall := 9 * time.Second
	h.deliver(f, cdmhost.EventBuffering, stall, true)
	for range 3 { // a stalled output repeating its position
		h.deliver(f, cdmhost.EventPosition, stall, false)
		if st := h.e.Status(); st.State != playback.StateLoading || st.Position != stall {
			t.Fatalf("status %+v, want loading at %v", st, stall)
		}
	}
	h.deliver(f, cdmhost.EventBuffering, stall, false)
	if st := h.e.Status(); st.State != playback.StatePlaying {
		t.Fatalf("status %+v, want playing after the underrun", st)
	}
	// And a second underrun shows loading again.
	h.deliver(f, cdmhost.EventBuffering, stall+time.Second, true)
	if st := h.e.Status(); st.State != playback.StateLoading {
		t.Fatalf("status %+v, want loading in the second underrun", st)
	}
}

// An underrun the host reported just before the pause is not a loading
// state: the item is paused and the host will not report its end.
func TestLateUnderrunDoesNotUnpauseTheState(t *testing.T) {
	h := newHarness(t, -1, false)
	if err := h.e.Play(ctx(t), "A", 0); err != nil {
		t.Fatal(err)
	}
	f := h.host(0)
	if err := h.e.Pause(ctx(t)); err != nil {
		t.Fatal(err)
	}
	h.deliver(f, cdmhost.EventBuffering, 3*time.Second, true)
	if st := h.e.Status(); st.State != playback.StatePaused {
		t.Fatalf("status %+v, want paused", st)
	}
	h.deliver(f, cdmhost.EventPosition, 4*time.Second, false)
	if st := h.e.Status(); st.State != playback.StatePaused {
		t.Fatalf("status %+v, want paused after a position", st)
	}
}
