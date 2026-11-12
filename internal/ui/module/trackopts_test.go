package module

import (
	"testing"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// A song loading counts as playing (its row shows the equaliser moving, as
// the shelves do), and the queue's current item reaches the options.
func TestTrackOptsTreatsLoadingAsPlaying(t *testing.T) {
	bus := store.NewBus(nil)
	bus.SetPoster(nil) // deliver at once
	env := &Env{Play: store.NewPlayback(bus, nil)}
	item := &services.QueueItem{ID: "1", Name: "Alpha"}
	for state, want := range map[string]bool{
		store.StatePlaying: true, store.StateLoading: true, store.StatePaused: false, store.StateIdle: false,
	} {
		bus.Emit(services.EventPlaybackState, services.PlaybackState{State: state, Current: item})
		o := env.TrackOpts(nil, true)
		if o.Playing != want || o.Current != item || !o.Restricted {
			t.Errorf("%s: Playing %v, Current %v, Restricted %v; want %v, %v, true", state, o.Playing, o.Current, o.Restricted, want, item)
		}
	}
}

// Without a playback mirror the options name nothing as current, and the
// links need a route a module serves.
func TestTrackOptsWithoutPlaybackOrRoutes(t *testing.T) {
	o := (&Env{}).TrackOpts(nil, false)
	if o.Current != nil || o.Playing || o.OpenArtist != nil || o.OpenAlbum != nil || o.OpenProfile != nil {
		t.Errorf("a hand-built Env gives %+v", o)
	}
	env := &Env{HasRoute: func(p string) bool { return p == "/artist/x" }}
	o = env.TrackOpts(nil, false)
	if o.OpenArtist == nil || o.OpenAlbum != nil {
		t.Errorf("links: artist %v, album %v", o.OpenArtist != nil, o.OpenAlbum != nil)
	}
}
