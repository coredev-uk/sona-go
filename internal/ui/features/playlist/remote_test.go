package playlist

import (
	"slices"
	"testing"

	"github.com/coredev-uk/sona-go/internal/services"
)

// Opening a library playlist shows the stored copy, asks Apple whether it
// changed on another device, and shows the new order when it did, with no
// manual sync.
func TestOpeningAPlaylistChangedElsewhereShowsTheNewOrder(t *testing.T) {
	d := libraryDetail("p.1", "Road Trip", threeSongs()...)
	var r *pageRig
	r = newPageRigWith(t, "p.1", func(api *fakeAPI) {
		api.remote = func(id string) bool {
			// Reordered on the phone: the service stored it and announced it.
			api.mu.Lock()
			api.details[id] = libraryDetail(id, "Road Trip", reversed(threeSongs())...)
			api.mu.Unlock()
			r.changed(services.PlaylistsChanged{IDs: []string{id}})
			return true
		}
	}, d)
	names := []string{"Song A", "Song B", "Song C"}
	r.shows("Song A")
	r.did("CheckRemote p.1")
	r.tt.Until("the new order", func() bool {
		return slices.Equal(r.order(names...), []string{"Song C", "Song B", "Song A"})
	})
	if n := len(r.calls("CheckRemote")); n != 1 {
		t.Errorf("the playlist was checked %d times, want once", n)
	}
}

// A playlist that did not change is not refetched: the check asks once.
func TestOpeningAnUnchangedPlaylistChecksOnceAndKeepsTheList(t *testing.T) {
	r := newPageRig(t, "p.1", libraryDetail("p.1", "Road Trip", threeSongs()...))
	r.shows("Song A", "Song B", "Song C")
	r.did("CheckRemote p.1")
	r.tt.Frame()
	if n := len(r.calls("Playlist ")); n != 1 {
		t.Errorf("the page read the playlist %d times, want once: %v", n, r.calls("Playlist "))
	}
}

// A catalogue playlist is not Sona's to check, and edits made here that are
// still on their way hold the check back, so they are not overwritten.
func TestNoCheckForCatalogueOrWhileEditing(t *testing.T) {
	r := newPageRig(t, "pl.1", libraryDetail("pl.1", "Catalogue", threeSongs()...))
	r.shows("Song A")
	r.tt.Frame()
	if got := r.calls("CheckRemote"); len(got) != 0 {
		t.Errorf("a catalogue playlist was checked: %v", got)
	}

	release := make(chan struct{})
	r = newPageRig(t, "p.2", libraryDetail("p.2", "Mine", threeSongs()...))
	r.p.edits.serial("p.2", func() (services.PlaylistDetail, error) {
		<-release
		return services.PlaylistDetail{}, nil
	}, func(services.PlaylistDetail, error, bool) {})
	t.Cleanup(func() { close(release) })
	r.shows("Song A")
	r.tt.Frame()
	if got := r.calls("CheckRemote"); len(got) != 0 {
		t.Errorf("the playlist was checked with an edit on its way: %v", got)
	}
}

func reversed(in []services.Song) []services.Song {
	out := slices.Clone(in)
	slices.Reverse(out)
	return out
}
