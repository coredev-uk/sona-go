//go:build linux

package native

import (
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/cdmhost"
	"github.com/coredev-uk/sona-go/internal/playback/fmp4"
)

// trimmed is the fixture songs' duration: 30 s of playlist minus priming.
var trimmed = 30*time.Second - fmp4.Time(fmp4.AACEncoderDelay, 44100)

func (f *fakeHost) nextCalls() []nextCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]nextCall(nil), f.nexts...)
}

func (f *fakeHost) fedTrack(track uint64) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.feeds {
		if c.track == track {
			n++
		}
	}
	return n
}

// TestArmedNextIsQueuedAndAdvances: once the current item is fed to its
// end, the armed next item is prepared, queued in the host and fed; the
// host's EventAdvance makes it current without a Play.
func TestArmedNextIsQueuedAndAdvances(t *testing.T) {
	h := newHarness(t, -1, false)
	h.e.SetCrossfade(5 * time.Second)
	if err := h.e.Play(ctx(t), "A", 0); err != nil {
		t.Fatal(err)
	}
	h.e.SetNext(playback.Next{ID: "B", Gapless: true})
	f := h.host(0)
	eventually(t, "B queued", func() bool { return len(f.nextCalls()) == 1 })
	q := f.nextCalls()[0]
	if q.fade != 0 || q.dur != trimmed {
		t.Fatalf("queued %+v: a gapless next joins without crossfade, with its trimmed duration %v", q, trimmed)
	}
	eventually(t, "B fed", func() bool { return f.fedTrack(q.track) == 3 })

	drainEvents(h.e)
	f.emit(cdmhost.Event{Kind: cdmhost.EventAdvance, Track: q.track, Position: 20 * time.Millisecond, At: time.Now()})
	ev := nextEvent(t, h.e)
	if ev.Kind != playback.EventAdvance || ev.Status.ItemID != "B" || ev.Status.State != playback.StatePlaying ||
		ev.Status.Position != 20*time.Millisecond || ev.Status.Duration != trimmed {
		t.Fatalf("advance event %+v", ev)
	}
	// Events for B are now the current item's.
	f.emit(cdmhost.Event{Kind: cdmhost.EventPosition, Track: q.track, Position: 5 * time.Second, At: time.Now()})
	eventually(t, "B position", func() bool { return h.e.Status().Position == 5*time.Second })
}

func TestCrossfadeIntoNextAndSeekRequeues(t *testing.T) {
	h := newHarness(t, -1, false)
	h.e.SetCrossfade(20 * time.Second) // clamped to 12 s, then to half a track
	if err := h.e.Play(ctx(t), "A", 0); err != nil {
		t.Fatal(err)
	}
	h.e.SetNext(playback.Next{ID: "B"})
	f := h.host(0)
	eventually(t, "B queued", func() bool { return len(f.nextCalls()) == 1 })
	if q := f.nextCalls()[0]; q.fade != 12*time.Second {
		t.Fatalf("crossfade %v, want the 12 s maximum", q.fade)
	}
	// A seek reloads A in the host, which drops B; B is queued again
	// once A is fed to its end.
	if err := h.e.Seek(ctx(t), 10*time.Second); err != nil {
		t.Fatal(err)
	}
	eventually(t, "B queued again", func() bool { return len(f.nextCalls()) == 2 })
	calls := f.nextCalls()
	if calls[1].track == calls[0].track {
		t.Fatalf("requeued under the old generation: %+v", calls)
	}
	// Disarming tells the host.
	h.e.SetNext(playback.Next{})
	eventually(t, "B dropped", func() bool {
		c := f.nextCalls()
		return len(c) == 3 && c[2].track == 0
	})
}

func drainEvents(e *Engine) {
	for {
		select {
		case <-e.Events():
		default:
			return
		}
	}
}

func nextEvent(t *testing.T, e *Engine) playback.Event {
	t.Helper()
	select {
	case ev := <-e.Events():
		return ev
	case <-time.After(2 * time.Second):
		t.Fatal("no event")
		return playback.Event{}
	}
}
