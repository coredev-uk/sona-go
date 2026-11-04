package library

import (
	"testing"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

func TestStateOfItemCarriesTheDTOsIdentity(t *testing.T) {
	want := kit.StateItem{ID: "i.1", Source: services.SourceLibrary, CatalogID: "c1", LibraryID: "i.1"}
	for kind, it := range map[string]kit.Item{
		kit.StateSongs:       kit.SongItem(services.Song{ID: "i.1", Source: services.SourceLibrary, CatalogID: "c1", LibraryID: "i.1"}),
		kit.StateAlbums:      kit.AlbumItem(services.Album{ID: "i.1", Source: services.SourceLibrary, CatalogID: "c1", LibraryID: "i.1"}),
		kit.StateArtists:     kit.ArtistItem(services.Artist{ID: "i.1", Source: services.SourceLibrary, CatalogID: "c1", LibraryID: "i.1"}),
		kit.StatePlaylists:   kit.PlaylistItem(services.Playlist{ID: "i.1", Source: services.SourceLibrary, CatalogID: "c1", LibraryID: "i.1"}),
		kit.StateMusicVideos: kit.MusicVideoItem(services.MusicVideo{ID: "i.1", Source: services.SourceLibrary, CatalogID: "c1", LibraryID: "i.1"}),
	} {
		k, si, ok := stateOfItem(it)
		if !ok || k != kind || si != want {
			t.Errorf("%s: kind %q item %+v ok %v", kind, k, si, ok)
		}
	}
	if _, _, ok := stateOfItem(kit.StationItem(services.Station{ID: "ra.1"})); ok {
		t.Error("a station has no library state")
	}
}
