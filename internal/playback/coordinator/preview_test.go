package coordinator

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/coredev-uk/sona-go/internal/playback"
)

// An engine playing previews: each 100 s song plays its 30 s clip, the
// coordinator moves on when the clip ends at 30 s, and no play is
// reported (no activity for Apple, scrobbles or history, no OnPlayed, no
// session history), whether the clip plays out or is skipped. The
// snapshot says Preview.
func TestPreviewAdvancesAtItsEndAndReportsNothing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := &activityLog{}
		h := newHarness(t, func(o *Options) { o.OnActivity = log.add })
		h.eng.preview = true
		ctx := t.Context()

		must(t, h.c.SetQueue(ctx, items("a", "b", "c"), 0, "album:1"))
		synctest.Wait()
		s := h.c.Snapshot()
		if !s.Preview || s.Duration != previewLength || h.current() != "a" || s.State != playback.StatePlaying {
			t.Fatalf("first: preview %v, duration %v, current %q, %s", s.Preview, s.Duration, h.current(), s.State)
		}

		time.Sleep(previewLength)
		h.eng.end()
		synctest.Wait()
		s = h.c.Snapshot()
		if h.current() != "b" || s.State != playback.StatePlaying || !s.Preview || s.Position != 0 {
			t.Fatalf("after a's clip: current %q, %s at %v, preview %v", h.current(), s.State, s.Position, s.Preview)
		}

		time.Sleep(20 * time.Second) // past HistoryMin and half the clip
		must(t, h.c.Next(ctx))
		synctest.Wait()
		if h.current() != "c" {
			t.Fatalf("after skipping b: current %q", h.current())
		}

		if got := h.eng.playList(); len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
			t.Errorf("plays %v", got)
		}
		expect(t, log)
		if p := h.playedList(); len(p) != 0 {
			t.Errorf("OnPlayed got %v", p)
		}
		if hist := h.c.Snapshot().History; len(hist) != 0 {
			t.Errorf("session history %v", hist)
		}
	})
}
