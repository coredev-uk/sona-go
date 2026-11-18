package coordinator

import (
	"testing"
	"testing/synctest"

	"github.com/coredev-uk/sona-go/internal/playback"
)

// The engine reloads the playing song in the other Spatial Audio mix by
// itself and says so in its status: the snapshot follows, with the reason
// a song plays stereo.
func TestSnapshotFollowsTheEnginesSpatialMix(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, nil)
		must(t, h.c.SetQueue(t.Context(), items("a", "b"), 0, "album:1"))
		synctest.Wait()
		if s := h.c.Snapshot(); s.Spatial || s.SpatialMiss != "" {
			t.Fatalf("snapshot %+v before the setting changes", s)
		}
		for _, tc := range []struct {
			spatial bool
			miss    string
		}{
			{false, playback.SpatialAppleOnly},
			{true, ""},
			{false, ""},
		} {
			h.eng.mu.Lock()
			h.eng.st.Spatial, h.eng.st.SpatialMiss = tc.spatial, tc.miss
			h.eng.emitLocked(playback.EventState)
			h.eng.mu.Unlock()
			synctest.Wait()
			if s := h.c.Snapshot(); s.Spatial != tc.spatial || s.SpatialMiss != tc.miss || s.State != playback.StatePlaying {
				t.Fatalf("snapshot spatial %v, miss %q, %s; want %v, %q", s.Spatial, s.SpatialMiss, s.State, tc.spatial, tc.miss)
			}
		}
		if got := h.eng.playList(); len(got) != 1 {
			t.Errorf("plays %v: the coordinator must not replay the song", got)
		}
	})
}
