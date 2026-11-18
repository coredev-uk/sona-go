package coordinator

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/coredev-uk/sona-go/internal/playback"
)

// An engine event is a snapshot of the engine as it was when the event was
// emitted. The loop can take one and then wait for cmd while a command runs to
// its end (a Pause the user pressed just after the engine said "playing"), so
// the event is handled after the command's own status. It must not take the
// command back: the bar would offer Pause again, and the listen the pause
// closed would open again.
func TestAnEventFromBeforeACommandDoesNotUndoIt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := &activityLog{}
		h := newHarness(t, func(o *Options) { o.OnActivity = log.add })
		ctx := t.Context()

		must(t, h.c.SetQueue(ctx, items("a", "b"), 0, ""))
		expect(t, log, "start a 0s")
		wasPlaying := playback.Event{Kind: playback.EventState, Status: h.eng.Status()}

		time.Sleep(10 * time.Second)
		must(t, h.c.Pause(ctx))
		expect(t, log, "end a 10s paused")
		h.c.handle(wasPlaying)
		expect(t, log) // no new listen
		if got := h.c.Snapshot().State; got != playback.StatePaused {
			t.Errorf("a playing event from before the pause leaves the player %s", got)
		}

		wasPaused := playback.Event{Kind: playback.EventState, Status: h.eng.Status()}
		must(t, h.c.Resume(ctx))
		expect(t, log, "start a 10s")
		h.c.handle(wasPaused)
		expect(t, log) // the listen is not closed
		if got := h.c.Snapshot().State; got != playback.StatePlaying {
			t.Errorf("a paused event from before the resume leaves the player %s", got)
		}
	})
}
