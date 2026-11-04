package search

import (
	"testing"

	"github.com/coredev-uk/sona-go/internal/services"
)

// v0.2.0's combobox.test.ts: group titles, "see all" and kind labels.

func TestGroupTitles(t *testing.T) {
	for group, want := range map[string]string{
		"top": "Top results", "artist": "Artists", "album": "Albums", "song": "Songs", "playlist": "Playlists",
		"radio_show": "Radio shows", "radio_episode": "Radio episodes", "station": "Stations",
		"music_video": "Music videos", "category": "Categories", "curator": "Curators", "record_label": "Labels",
		"profile": "Profiles", "shared_playlist": "Playlists by listeners",
		// An unknown group: its key in sentence case.
		"podcast": "Podcasts", "live_show": "Live shows", "tv-show": "Tv shows", "": "Results", "__": "Results",
	} {
		if got := groupTitle(group); got != want {
			t.Errorf("groupTitle(%q) = %q, want %q", group, got, want)
		}
	}
}

func TestGroupErrorAndEmptyTitles(t *testing.T) {
	if got := groupLoadError("album"); got != "Couldn't load albums" {
		t.Errorf("album error %q", got)
	}
	if got := groupLoadError("mystery"); got != "Couldn't load these results" {
		t.Errorf("default error %q", got)
	}
	if got := groupEmpty("song", "blue"); got != "No songs for “blue”" {
		t.Errorf("song empty %q", got)
	}
	if got := groupEmpty("mystery", "blue"); got != "Nothing for “blue”" {
		t.Errorf("default empty %q", got)
	}
	for _, g := range []string{
		"top", "artist", "album", "song", "playlist", "radio_show", "radio_episode", "station",
		"music_video", "category", "curator", "record_label", "profile", "shared_playlist",
	} {
		if groupLoadError(g) == groupLoadError("mystery") || groupEmpty(g, "x") == groupEmpty("mystery", "x") {
			t.Errorf("group %q has no title of its own", g)
		}
	}
}

func TestSeeAllIsOfferedWhenThereIsMoreAndNeverForTop(t *testing.T) {
	for _, c := range []struct {
		group             string
		next, shown, have int
		want              bool
	}{
		{"song", 21, 8, 21, true},
		{"album", 0, 21, 21, false},
		{"album", 21, 21, 21, true},
		{"artist", 0, 5, 9, true}, // more items than shown
		{"top", 21, 9, 12, false},
	} {
		if got := hasSeeAll(c.group, c.next, c.shown, c.have); got != c.want {
			t.Errorf("hasSeeAll(%q, %d, %d, %d) = %v, want %v", c.group, c.next, c.shown, c.have, got, c.want)
		}
	}
	// A songs section shows eight, so even without a next page ten songs have more.
	sec := services.SearchSection{Group: "song", Items: make([]services.Item, 10)}
	if !seeAllWanted(sec) {
		t.Error("ten songs, eight shown: no See all")
	}
	sec.Items = sec.Items[:8]
	if seeAllWanted(sec) {
		t.Error("eight songs: See all")
	}
	sec.Next = 21
	if !seeAllWanted(sec) {
		t.Error("a next page: no See all")
	}
}

func TestKindLabels(t *testing.T) {
	station := func(kind string) services.Item {
		return services.Item{Kind: services.ItemStation, Station: &services.Station{Kind: kind}}
	}
	for _, c := range []struct {
		it   services.Item
		want string
	}{
		{services.Item{Kind: services.ItemSong}, "Song"},
		{services.Item{Kind: services.ItemAlbum}, "Album"},
		{services.Item{Kind: services.ItemArtist}, "Artist"},
		{services.Item{Kind: services.ItemPlaylist}, "Playlist"},
		{station(services.StationEpisode), "Radio episode"},
		{station(services.StationLive), "Live radio"},
		{station(services.StationBroadcast), "Live radio"},
		{station(services.StationTracks), "Station"},
		{services.Item{Kind: services.ItemStation}, "Station"},
		{services.Item{Kind: services.ItemMusicVideo}, "Music video"},
		{services.Item{Kind: services.ItemCurator}, "Curator"},
		{services.Item{Kind: services.ItemProfile}, "Profile"},
		{services.Item{Kind: services.ItemRoom}, "Collection"},
		{services.Item{Kind: services.ItemMultiroom}, "Collection"},
		{services.Item{Kind: services.ItemGrouping}, "Collection"},
		{services.Item{Kind: services.ItemCharts}, "Charts"},
		{services.Item{Kind: services.ItemLink}, ""},
	} {
		if got := kindLabel(c.it); got != c.want {
			t.Errorf("kindLabel(%q) = %q, want %q", c.it.Kind, got, c.want)
		}
	}
}

func TestItemMetaJoinsKindAndSubtitle(t *testing.T) {
	if got := itemMeta(services.Item{Kind: services.ItemSong, Subtitle: "Blue Band"}); got != "Song · Blue Band" {
		t.Errorf("meta %q", got)
	}
	if got := itemMeta(services.Item{Kind: services.ItemSong}); got != "Song" {
		t.Errorf("meta without a subtitle %q", got)
	}
	if got := itemMeta(services.Item{Kind: services.ItemLink, Subtitle: "web"}); got != "web" {
		t.Errorf("meta without a kind %q", got)
	}
}

func TestItemPathSongsPlayInsteadOfLinking(t *testing.T) {
	if got := itemPath(services.Item{Kind: services.ItemSong, ID: "1"}); got != "" {
		t.Errorf("a song has page %q", got)
	}
	if got := itemPath(services.Item{Kind: services.ItemAlbum, ID: "5"}); got != "/album/5" {
		t.Errorf("album page %q", got)
	}
}

func TestSearchParams(t *testing.T) {
	for _, c := range []struct {
		loc, q, scope string
	}{
		{"/search", "", "catalog"},
		{"/search?q=blue", "blue", "catalog"},
		{"/search?q=%20blue%20moon%20", "blue moon", "catalog"},
		{"/search?q=%20%20", "", "catalog"},
		{"/search?q=blue&scope=library", "blue", "library"},
		{"/search?q=blue&scope=catalog", "blue", "catalog"},
		{"/search?q=blue&scope=web", "blue", "catalog"},
		{"/library/albums?q=blue&scope=library", "", "catalog"},
	} {
		if q, scope := searchParams(c.loc); q != c.q || scope != c.scope {
			t.Errorf("searchParams(%q) = (%q, %q), want (%q, %q)", c.loc, q, scope, c.q, c.scope)
		}
	}
}

func TestSearchLocationWritesTheLibraryScopeOnly(t *testing.T) {
	for _, c := range []struct{ q, scope, want string }{
		{"blue", "catalog", "/search?q=blue"},
		{"blue moon", "library", "/search?q=blue+moon&scope=library"},
		{"  ", "library", "/search?scope=library"},
		{"", "catalog", "/search"},
	} {
		if got := searchLocation(c.q, c.scope); got != c.want {
			t.Errorf("searchLocation(%q, %q) = %q, want %q", c.q, c.scope, got, c.want)
		}
	}
	if got := groupLocation("song", "blue band"); got != "/search/song?q=blue+band" {
		t.Errorf("groupLocation = %q", got)
	}
}

func TestItemSourceAndLibraryCards(t *testing.T) {
	lib := services.LibrarySearch{
		Songs:  []services.Song{{ID: "i.1", Name: "S", Artist: "A", Source: services.SourceLibrary}},
		Albums: []services.Album{{ID: "l.1", Name: "Al", Artist: "Ar", Source: services.SourceLibrary}},
	}
	artists, albums, playlists, songs := libraryCards(lib)
	if len(artists) != 0 || len(playlists) != 0 || len(albums) != 1 || len(songs) != 1 {
		t.Fatalf("cards: %d artists, %d albums, %d playlists, %d songs", len(artists), len(albums), len(playlists), len(songs))
	}
	if albums[0].Kind != services.ItemAlbum || albums[0].Subtitle != "Ar" || albums[0].Album == nil {
		t.Errorf("album card %+v", albums[0])
	}
	if itemSource(songs[0]) != services.SourceLibrary {
		t.Error("a library song has the catalog's source")
	}
	if itemSource(services.Item{Kind: services.ItemStation}) != services.SourceCatalog {
		t.Error("an untyped item is not the catalog's")
	}
}
