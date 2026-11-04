package player

import (
	"slices"
	"testing"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// Ported from TrackAnnouncer.test.tsx.
func TestTheBarAnnouncesANewTrackPolitelyOnce(t *testing.T) {
	r := newRig(t, true)
	r.tt.Announcements()
	r.seed(func(s *services.PlaybackState) {
		s.Current = &services.QueueItem{ID: "a", Name: "Yellow", Artist: "Coldplay"}
	})
	if got := r.tt.Announcements(); !slices.Equal(got, []string{"Now playing: Yellow by Coldplay"}) {
		t.Fatalf("announced %q", got)
	}
	r.tt.Frame()
	if got := r.tt.Announcements(); len(got) != 0 {
		t.Errorf("announced again: %q", got)
	}
}

func TestTheBarWaitsForTheEnginesReportBeforeAnnouncingAPreview(t *testing.T) {
	r := newRig(t, true)
	r.tt.Announcements()
	song := func(s *services.PlaybackState) {
		s.Current = &services.QueueItem{ID: "a", Name: "Yellow", Artist: "Coldplay"}
	}
	r.seed(song, func(s *services.PlaybackState) { s.State = store.StateLoading })
	if got := r.tt.Announcements(); len(got) != 0 {
		t.Fatalf("announced while loading: %q", got)
	}
	r.seed(song, func(s *services.PlaybackState) { s.Preview = true })
	if got := r.tt.Announcements(); !slices.Equal(got, []string{"Now playing a preview: Yellow by Coldplay"}) {
		t.Errorf("announced %q", got)
	}
	// Without an artist.
	r.seed(func(s *services.PlaybackState) {
		s.Current = &services.QueueItem{ID: "b", Name: "Clocks"}
		s.Preview = true
	})
	if got := r.tt.Announcements(); !slices.Equal(got, []string{"Now playing a preview: Clocks"}) {
		t.Errorf("announced %q", got)
	}
}
