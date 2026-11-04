package uitest_test

import (
	"net/http"
	"slices"
	"testing"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/fixture"
)

func songNames(songs []services.Song) []string {
	out := make([]string, len(songs))
	for i, s := range songs {
		out[i] = s.Name
	}
	return out
}

// kit.TracksOf over the real services (kit's own tests cannot build a Core:
// uitest registers the golden flag kit's do): a library album is its library
// tracks, a catalogue album its catalogue tracks (by the item's source), a
// playlist its tracks, and a failure the service's own.
func TestTracksOfResolvesAlbumsAndPlaylistsThroughTheServices(t *testing.T) {
	apple := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/catalog/gb/albums/7" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"7","type":"albums","attributes":{"name":"Big","isPrerelease":false},
			"relationships":{"tracks":{"data":[
				{"id":"71","type":"songs","attributes":{"name":"Aria","discNumber":1,"trackNumber":1}},
				{"id":"72","type":"songs","attributes":{"name":"Variation","discNumber":1,"trackNumber":2}}]}}}]}`))
	})
	h := uitest.New(t, uitest.Apple(apple))
	ctx := t.Context()
	albums, lists := fixture.Albums(), fixture.Playlists()

	got, err := kit.TracksOf(ctx, h.Core, kit.AlbumItem(services.Album{ID: fixture.AlbumLibraryID(0), Source: services.SourceLibrary, Name: albums[0].Name}))
	want := []string{albums[0].Song(0), albums[0].Song(1)}
	if err != nil || !slices.Equal(songNames(got), want) {
		t.Errorf("library album: %q, %v; want %q", songNames(got), err, want)
	}

	got, err = kit.TracksOf(ctx, h.Core, kit.AlbumItem(services.Album{ID: "7", Source: services.SourceCatalog, Name: "Big"}))
	if err != nil || !slices.Equal(songNames(got), []string{"Aria", "Variation"}) {
		t.Errorf("catalogue album: %q, %v", songNames(got), err)
	}

	got, err = kit.TracksOf(ctx, h.Core, kit.PlaylistItem(services.Playlist{ID: fixture.PlaylistLibraryID(0), Name: lists[0].Name}))
	if err != nil || len(got) != len(lists[0].Songs) {
		t.Errorf("playlist: %d tracks, %v; want %d", len(got), err, len(lists[0].Songs))
	}
	if first := lists[0].Songs[0]; len(got) > 0 && got[0].Name != albums[first[0]].Song(first[1]) {
		t.Errorf("the playlist starts with %q", got[0].Name)
	}

	// An album the library does not hold is the service's error, with no tracks.
	got, err = kit.TracksOf(ctx, h.Core, kit.AlbumItem(services.Album{ID: "l.missing", Source: services.SourceLibrary}))
	if err == nil || len(got) != 0 {
		t.Errorf("unknown library album: %q, %v", songNames(got), err)
	}
}
