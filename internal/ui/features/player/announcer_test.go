package player

import (
	"slices"
	"testing"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

func trackOf(id string) *services.QueueItem {
	return &services.QueueItem{ID: id, Name: "Song " + id, Artist: "Artist"}
}

// Ported from announce.test.ts.
func TestAnnouncedTrack(t *testing.T) {
	state := func(st string, cur *services.QueueItem) services.PlaybackState {
		return services.PlaybackState{State: st, Current: cur}
	}
	for _, tc := range []struct {
		name string
		seen string
		s    services.PlaybackState
		want string // the announced track's ID, "" for none
	}{
		{"a new track once it plays", "", state(store.StatePlaying, trackOf("a")), "a"},
		{"another track once it plays", "a", state(store.StatePlaying, trackOf("b")), "b"},
		{"waits while the new track loads", "a", state(store.StateLoading, trackOf("b")), ""},
		{"quiet for the same track paused", "a", state(store.StatePaused, trackOf("a")), ""},
		{"quiet for the same track resumed", "a", state(store.StatePlaying, trackOf("a")), ""},
		{"quiet for a track restored paused at launch", "", state(store.StatePaused, trackOf("a")), ""},
		{"quiet for an idle player holding a track", "", state(store.StateIdle, trackOf("a")), ""},
		{"quiet with nothing playing", "a", state(store.StateIdle, nil), ""},
	} {
		got := announcedTrack(tc.seen, tc.s)
		switch {
		case tc.want == "" && got != nil:
			t.Errorf("%s: announced %q", tc.name, got.ID)
		case tc.want != "" && (got == nil || got.ID != tc.want):
			t.Errorf("%s: announced %v, want %q", tc.name, got, tc.want)
		}
	}
}

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
