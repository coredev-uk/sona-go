package kit

import (
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
		got := AnnouncedTrack(tc.seen, tc.s)
		switch {
		case tc.want == "" && got != nil:
			t.Errorf("%s: announced %q", tc.name, got.ID)
		case tc.want != "" && (got == nil || got.ID != tc.want):
			t.Errorf("%s: announced %v, want %q", tc.name, got, tc.want)
		}
	}
}

func TestControlUnavailableSaysWhyOnlyForStations(t *testing.T) {
	live := services.PlaybackState{Current: &services.QueueItem{Live: true}}
	station := services.PlaybackState{Station: &services.QueueStation{AutoPlay: false}}
	autoplay := services.PlaybackState{Station: &services.QueueStation{AutoPlay: true}}
	for _, tc := range []struct {
		name    string
		s       services.PlaybackState
		applies bool
		want    string
	}{
		{"applies", live, true, ""},
		{"a live station", live, false, "Not available while listening to a live station"},
		{"a station of songs", station, false, "Not available while listening to a station"},
		{"AutoPlay's station", autoplay, false, ""},
		{"the end of a queue", services.PlaybackState{Current: &services.QueueItem{}}, false, ""},
	} {
		if got := ControlUnavailable(tc.s, tc.applies); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
}
