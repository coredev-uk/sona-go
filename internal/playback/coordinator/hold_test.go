package coordinator

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"

	"github.com/coredev-uk/sona-go/internal/playback"
)

// holdingEngine is fakeEngine with a host to let go of (playback.HostHolder).
type holdingEngine struct {
	*fakeEngine
	holds, releases int
	err             error
}

func (e *holdingEngine) HoldHost(ctx context.Context) (func(), error) {
	if e.err != nil {
		return nil, e.err
	}
	e.mu.Lock()
	e.holds++
	e.mu.Unlock()
	_ = e.Pause(ctx) // a host's hold pauses the item first
	return func() {
		e.mu.Lock()
		e.releases++
		e.mu.Unlock()
	}, nil
}

func TestHoldHostPausesAndKeepsTheQueue(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		eng := &holdingEngine{fakeEngine: newFake()}
		h := newHarness(t, func(o *Options) { o.Engine = eng })
		ctx := t.Context()
		must(t, h.c.SetQueue(ctx, items("a", "b", "c"), 1, ""))
		synctest.Wait()
		if s := h.c.Snapshot(); s.State != playback.StatePlaying || h.current() != "b" {
			t.Fatalf("before: %s on %q", s.State, h.current())
		}

		release, err := h.c.HoldHost(ctx)
		if err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		s := h.c.Snapshot()
		if eng.holds != 1 || s.State != playback.StatePaused || h.current() != "b" || len(h.ids()) != 3 {
			t.Fatalf("held: holds %d, %s on %q, queue %v", eng.holds, s.State, h.current(), h.ids())
		}

		// The install failed: release, and playback continues from the queue.
		release()
		if eng.releases != 1 {
			t.Fatalf("releases = %d", eng.releases)
		}
		must(t, h.c.Resume(ctx))
		synctest.Wait()
		if s := h.c.Snapshot(); s.State != playback.StatePlaying || h.current() != "b" {
			t.Fatalf("after release: %s on %q", s.State, h.current())
		}
	})
}

func TestHoldHostFailureIsReported(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		boom := errors.New("host won't let go")
		h := newHarness(t, func(o *Options) { o.Engine = &holdingEngine{fakeEngine: newFake(), err: boom} })
		release, err := h.c.HoldHost(t.Context())
		if !errors.Is(err, boom) || release != nil {
			t.Fatalf("HoldHost = %v, %v", release != nil, err)
		}
	})
}

// An engine with no host (the fake, preview-only builds) is not touched.
func TestHoldHostOnAnEngineWithoutAHost(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, nil)
		ctx := t.Context()
		must(t, h.c.SetQueue(ctx, items("a"), 0, ""))
		synctest.Wait()
		release, err := h.c.HoldHost(ctx)
		if err != nil || release == nil {
			t.Fatalf("HoldHost = %v, %v", release != nil, err)
		}
		release()
		if s := h.c.Snapshot(); s.State != playback.StatePlaying {
			t.Fatalf("an engine with no host was paused: %s", s.State)
		}
	})
}
