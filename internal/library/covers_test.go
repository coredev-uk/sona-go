package library

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/artwork"
)

func coverTmpl(album string) string {
	return "https://is1-ssl.mzstatic.com/image/thumb/Music/" + album + "/{w}x{h}bb.jpg"
}

// coverSongs returns library songs i.<album>.<n> whose artwork is each
// album's; "" is a song without artwork.
func coverSongs(albums ...string) []applemusic.Song {
	out := make([]applemusic.Song, len(albums))
	for i, a := range albums {
		s := applemusic.Song{Resource: applemusic.Resource[applemusic.SongAttributes]{
			ID: fmt.Sprintf("i.%s.%d", a, i), Type: applemusic.TypeLibrarySongs,
			Attributes: applemusic.SongAttributes{Name: "S", PlayParams: &applemusic.PlayParams{CatalogID: fmt.Sprintf("c%d", i)}},
		}}
		if a != "" {
			s.Attributes.Artwork = &applemusic.Artwork{URL: coverTmpl(a)}
		}
		out[i] = s
	}
	return out
}

func storePlaylist(t *testing.T, e *env, id, ownArt string, albums ...string) {
	t.Helper()
	p := applemusic.Playlist{Resource: applemusic.Resource[applemusic.PlaylistAttributes]{
		ID: id, Type: applemusic.TypeLibraryPlaylists,
		Attributes: applemusic.PlaylistAttributes{Name: id, CanEdit: true, LastModifiedDate: "2026-10-30T11:03:20Z"},
	}}
	if ownArt != "" {
		p.Attributes.Artwork = &applemusic.Artwork{URL: ownArt}
	}
	if err := e.lib.StorePlaylist(context.Background(), p, "", coverSongs(albums...)); err != nil {
		t.Fatal(err)
	}
}

func templates(pls []Playlist) map[string]string {
	out := map[string]string{}
	for _, p := range pls {
		out[p.LibraryID] = p.ArtworkTemplate
	}
	return out
}

// Playlists Apple gave no cover get one from their tracks, wherever a
// playlist is read; a cover Apple did render is never replaced.
func TestPlaylistsWithoutArtworkGetACover(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	own := coverTmpl("own")
	storePlaylist(t, e, "p.one", "", "a", "a", "a", "a", "a")                     // one album
	storePlaylist(t, e, "p.few", "", "a", "b", "a", "c", "b")                     // three albums
	storePlaylist(t, e, "p.many", "", "a", "a", "b", "", "a", "c", "b", "d", "e") // five; one track bare
	storePlaylist(t, e, "p.late", "", "", "", "z", "z")                           // first tracks bare
	storePlaylist(t, e, "p.bare", "", "", "")                                     // no artwork anywhere
	storePlaylist(t, e, "p.empty", "")
	storePlaylist(t, e, "p.own", own, "a", "b", "c", "d", "e")

	want := map[string]string{
		"p.one":   coverTmpl("a"),
		"p.few":   coverTmpl("a"),
		"p.many":  artwork.Mosaic([]string{coverTmpl("a"), coverTmpl("b"), coverTmpl("c"), coverTmpl("d")}),
		"p.late":  coverTmpl("z"),
		"p.bare":  "",
		"p.empty": "",
		"p.own":   own,
	}
	if !strings.HasPrefix(want["p.many"], "mosaic:") {
		t.Fatalf("expected mosaic, got %q", want["p.many"])
	}
	check := func(where string, got map[string]string) {
		t.Helper()
		for id, w := range want {
			if g, ok := got[id]; ok && g != w {
				t.Errorf("%s: %s cover = %q, want %q", where, id, g, w)
			}
		}
	}

	list, err := e.lib.Playlists(ctx, ListOptions{})
	if err != nil || len(list) != len(want) {
		t.Fatalf("playlists = %d, %v", len(list), err)
	}
	check("list", templates(list))

	for id := range want {
		p, _, err := e.lib.Playlist(ctx, id, ListOptions{})
		if err != nil {
			t.Fatal(err)
		}
		check("page", map[string]string{id: p.ArtworkTemplate})
	}

	added, _, err := e.lib.RecentlyAdded(ctx, ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, a := range added {
		if a.Playlist != nil {
			got[a.Playlist.LibraryID] = a.Playlist.ArtworkTemplate
		}
	}
	check("recently added", got)

	if err := RebuildSearch(ctx, e.db); err != nil {
		t.Fatal(err)
	}
	res, err := e.lib.SearchAll(ctx, "p", 20)
	if err != nil {
		t.Fatal(err)
	}
	check("search", templates(res.Playlists))
	if len(res.Playlists) == 0 {
		t.Fatal("search found no playlists")
	}

	pls := []Playlist{{LibraryID: "p.many"}, {LibraryID: "p.own", ArtworkTemplate: own}, {Name: "no ID"}}
	if err := e.lib.FillCovers(ctx, pls); err != nil {
		t.Fatal(err)
	}
	if pls[0].ArtworkTemplate != want["p.many"] || pls[1].ArtworkTemplate != own || pls[2].ArtworkTemplate != "" {
		t.Fatalf("FillCovers = %+v", pls)
	}
}

// The cover follows the playlist's tracks: it is the first four distinct
// albums in track order, however the playlist is edited.
func TestPlaylistCoverFollowsItsTracks(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	storePlaylist(t, e, "p.mix", "", "a", "b", "c", "d", "e")
	cover := func() string {
		t.Helper()
		p, _, err := e.lib.Playlist(ctx, "p.mix", ListOptions{})
		if err != nil {
			t.Fatal(err)
		}
		return p.ArtworkTemplate
	}
	tiles := func(albums ...string) string {
		t.Helper()
		tm := make([]string, len(albums))
		for i, a := range albums {
			tm[i] = coverTmpl(a)
		}
		return artwork.Mosaic(tm)
	}
	if got, want := cover(), tiles("a", "b", "c", "d"); got != want {
		t.Fatalf("cover = %q, want %q", got, want)
	}
	// A fifth album changes nothing.
	storeTracks := func(albums ...string) {
		t.Helper()
		if err := e.lib.StorePlaylistTracks(ctx, "p.mix", "2026-10-30T11:04:00Z", coverSongs(albums...)); err != nil {
			t.Fatal(err)
		}
	}
	storeTracks("a", "b", "c", "d", "e", "f")
	if got, want := cover(), tiles("a", "b", "c", "d"); got != want {
		t.Fatalf("later tracks changed the cover: %q", got)
	}
	// Removing the first album brings the next one in.
	storeTracks("b", "c", "d", "e", "f")
	if got, want := cover(), tiles("b", "c", "d", "e"); got != want {
		t.Fatalf("cover = %q, want %q", got, want)
	}
	// Reordering changes the tiles' places.
	storeTracks("c", "b", "d", "e")
	if got, want := cover(), tiles("c", "b", "d", "e"); got != want {
		t.Fatalf("cover = %q, want %q", got, want)
	}
	// Down to fewer than four albums: the first one alone.
	storeTracks("c", "c", "b", "b")
	if got := cover(); got != coverTmpl("c") {
		t.Fatalf("cover = %q, want the first album", got)
	}
	// And to nothing.
	storeTracks()
	if got := cover(); got != "" {
		t.Fatalf("empty playlist cover = %q", got)
	}
}

// A page of a large library is covered in a few statements.
func TestFillCoversBatches(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	n := coverBatch + 20
	for i := range n {
		storePlaylist(t, e, fmt.Sprintf("p.%04d", i), "", "a", "b", "c", "d")
	}
	list, err := e.lib.Playlists(ctx, ListOptions{Limit: 1000})
	if err != nil || len(list) != n {
		t.Fatalf("playlists = %d, %v", len(list), err)
	}
	want := artwork.Mosaic([]string{coverTmpl("a"), coverTmpl("b"), coverTmpl("c"), coverTmpl("d")})
	if bad := slices.IndexFunc(list, func(p Playlist) bool { return p.ArtworkTemplate != want }); bad >= 0 {
		t.Fatalf("playlist %d cover = %q", bad, list[bad].ArtworkTemplate)
	}
}
