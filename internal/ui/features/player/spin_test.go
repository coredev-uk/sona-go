package player

import (
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// spinFrames shows the bar with the song loading at scale 2, steps 60 frames
// of 16.7 ms and returns the builds they cost and in how many the picture
// changed.
func spinFrames(t *testing.T) (builds, changed int) {
	t.Helper()
	r, n := seekRig(t, 2)
	r.seed(func(s *services.PlaybackState) { s.State = store.StateLoading })
	r.tt.Drain()
	before, built := r.tt.Image(), *n
	for range 60 {
		r.tt.Advance(16700 * time.Microsecond)
		img := r.tt.Image()
		if !sameImage(before, img) {
			changed++
		}
		before = img
	}
	return *n - built, changed
}

// The play disc's spinner turns by painting: the view is not built again for
// it, and the picture changes from frame to frame.
func TestTheLoadingDiscTurnsByPaintingNotBuilding(t *testing.T) {
	builds, changed := spinFrames(t)
	if builds > 2 {
		t.Errorf("the bar was built %d times in 60 frames of a loading song", builds)
	}
	if changed < 30 {
		t.Errorf("the picture changed in %d of 60 frames: the disc does not turn", changed)
	}
}

func TestTheLoadingDiscStandsStillWhenMotionIsReduced(t *testing.T) {
	theme.SetReducedMotion(true)
	t.Cleanup(func() { theme.SetReducedMotion(false) })
	builds, changed := spinFrames(t)
	if builds != 0 || changed != 0 {
		t.Errorf("reduced motion: %d builds, %d changed frames in 60", builds, changed)
	}
}
