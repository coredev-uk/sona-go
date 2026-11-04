package library

import (
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

func TestStateItemsAndRefsCarryTheDTOsIdentity(t *testing.T) {
	song := services.Song{ID: "i.1", Source: services.SourceLibrary, CatalogID: "c1", LibraryID: "i.1"}
	want := stateItem{"i.1", services.SourceLibrary, "c1", "i.1"}
	if got := stateOfSong(song); got != want {
		t.Errorf("song: %+v", got)
	}
	for kind, it := range map[string]kit.Item{
		stateSongs:       kit.SongItem(song),
		stateAlbums:      kit.AlbumItem(services.Album{ID: "i.1", Source: services.SourceLibrary, CatalogID: "c1", LibraryID: "i.1"}),
		stateArtists:     kit.ArtistItem(services.Artist{ID: "i.1", Source: services.SourceLibrary, CatalogID: "c1", LibraryID: "i.1"}),
		statePlaylists:   kit.PlaylistItem(services.Playlist{ID: "i.1", Source: services.SourceLibrary, CatalogID: "c1", LibraryID: "i.1"}),
		stateMusicVideos: kit.MusicVideoItem(services.MusicVideo{ID: "i.1", Source: services.SourceLibrary, CatalogID: "c1", LibraryID: "i.1"}),
	} {
		k, si, ok := stateOfItem(it)
		if !ok || k != kind || si != want {
			t.Errorf("%s: kind %q item %+v ok %v", kind, k, si, ok)
		}
	}
	if _, _, ok := stateOfItem(kit.StationItem(services.Station{ID: "ra.1"})); ok {
		t.Error("a station has no library state")
	}
	if got := itemRefOf(stateSongs, want); got != (services.ItemRef{Kind: "songs", ID: "i.1", Source: services.SourceLibrary, CatalogID: "c1", LibraryID: "i.1"}) {
		t.Errorf("ref %+v", got)
	}
	if k := itemStateKey("songs", "i.1"); len(k) != 4 || k[0] != "library" || k[1] != "state" || k[2] != "songs" || k[3] != "i.1" {
		t.Errorf("key %v", k)
	}
}

func TestIsUpcomingIsV020sRule(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		a    services.Album
		want bool
	}{
		{services.Album{}, false},
		{services.Album{Prerelease: true}, true},
		{services.Album{Prerelease: true, ReleaseAt: now.Add(time.Hour).UnixMilli()}, true},
		{services.Album{Prerelease: true, ReleaseAt: now.Add(-time.Hour).UnixMilli()}, false},
		{services.Album{ReleaseAt: now.Add(time.Hour).UnixMilli()}, false},
	} {
		if got := isUpcoming(c.a, now); got != c.want {
			t.Errorf("%+v: %v", c.a, got)
		}
	}
}
