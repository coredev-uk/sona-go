package library

import (
	"context"
	"errors"
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

const (
	presetA = "preset:aurora:3a7bd5:d5b43a"
	presetB = "preset:duo:b03040:20a060"
	upload1 = "cover:0123456789abcdef0123456789abcdef"
	upload2 = "cover:fedcba9876543210fedcba9876543210"
)

// A cover the user chose shows wherever a playlist is read, in place of
// Apple's artwork and of the mosaic; "none" shows the placeholder; another
// playlist is untouched; and it survives the sync that rewrites the row.
func TestChosenCoverShowsEverywhereAPlaylistIsRead(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	own := coverTmpl("own")
	storePlaylist(t, e, "p.own", own, "a", "b", "c", "d")
	storePlaylist(t, e, "p.mosaic", "", "a", "b", "c", "d")
	storePlaylist(t, e, "p.other", "", "a", "b", "c", "d")
	for id, c := range map[string]CoverChoice{
		"p.own":    {Kind: CoverPreset, Template: presetA},
		"p.mosaic": {Kind: CoverNone},
	} {
		if err := e.lib.SetPlaylistCover(ctx, id, c); err != nil {
			t.Fatal(err)
		}
	}
	mosaic := artwork.Mosaic([]string{coverTmpl("a"), coverTmpl("b"), coverTmpl("c"), coverTmpl("d")})
	want := map[string]string{"p.own": presetA, "p.mosaic": "", "p.other": mosaic}
	check := func(where string, got map[string]string) {
		t.Helper()
		for id, w := range want {
			if g, ok := got[id]; !ok || g != w {
				t.Errorf("%s: %s cover = %q (present %v), want %q", where, id, g, ok, w)
			}
		}
	}
	list, err := e.lib.Playlists(ctx, ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	check("list", templates(list))
	page := map[string]string{}
	for id := range want {
		p, _, err := e.lib.Playlist(ctx, id, ListOptions{})
		if err != nil {
			t.Fatal(err)
		}
		page[id] = p.ArtworkTemplate
	}
	check("page", page)
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

	// The automatic cover is still there to go back to.
	if auto, err := e.lib.AutoCover(ctx, "p.own"); err != nil || auto != own {
		t.Errorf("AutoCover(p.own) = %q, %v; want Apple's %q", auto, err, own)
	}
	if auto, err := e.lib.AutoCover(ctx, "p.mosaic"); err != nil || auto != mosaic {
		t.Errorf("AutoCover(p.mosaic) = %q, %v; want %q", auto, err, mosaic)
	}

	// A sync stores the playlist again: the choice stays.
	storePlaylist(t, e, "p.own", own, "a", "b", "c", "d", "e")
	if p, _, err := e.lib.Playlist(ctx, "p.own", ListOptions{}); err != nil || p.ArtworkTemplate != presetA {
		t.Errorf("after a sync the cover = %q, %v; want %q", p.ArtworkTemplate, err, presetA)
	}

	// Automatic again.
	if err := e.lib.SetPlaylistCover(ctx, "p.own", CoverChoice{}); err != nil {
		t.Fatal(err)
	}
	if p, _, _ := e.lib.Playlist(ctx, "p.own", ListOptions{}); p.ArtworkTemplate != own {
		t.Errorf("after going back to automatic the cover = %q, want %q", p.ArtworkTemplate, own)
	}
}

// The last uploaded image is kept while another cover shows, and a row that
// holds only that image counts as the automatic cover.
func TestCoverChoiceKeepsTheUploadAndFollowsThePlaylist(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	storePlaylist(t, e, "p.a", "", "a", "b", "c", "d")
	storePlaylist(t, e, "p.b", "", "a")
	if c, err := e.lib.PlaylistCover(ctx, "p.a"); err != nil || c != (CoverChoice{Kind: CoverAuto}) {
		t.Fatalf("a playlist nobody touched has %+v, %v", c, err)
	}
	for _, c := range []CoverChoice{
		{Kind: CoverUpload, Template: upload1, Upload: upload1},
		{Kind: CoverPreset, Template: presetB, Upload: upload1},
		{Kind: CoverAuto, Upload: upload1},
	} {
		if err := e.lib.SetPlaylistCover(ctx, "p.a", c); err != nil {
			t.Fatal(err)
		}
		if got, err := e.lib.PlaylistCover(ctx, "p.a"); err != nil || got != c {
			t.Fatalf("PlaylistCover = %+v, %v; want %+v", got, err, c)
		}
	}
	if p, _, _ := e.lib.Playlist(ctx, "p.a", ListOptions{}); !strings.HasPrefix(p.ArtworkTemplate, "mosaic:") {
		t.Errorf("a playlist that keeps an upload but shows the automatic cover has %q", p.ArtworkTemplate)
	}
	if err := e.lib.SetPlaylistCover(ctx, "p.b", CoverChoice{Kind: CoverUpload, Template: upload2, Upload: upload2}); err != nil {
		t.Fatal(err)
	}
	uploads, err := e.lib.CoverUploads(ctx)
	slices.Sort(uploads)
	if err != nil || !slices.Equal(uploads, []string{upload1, upload2}) {
		t.Fatalf("CoverUploads = %v, %v", uploads, err)
	}
	// The zero choice forgets everything, the kept image included.
	if err := e.lib.SetPlaylistCover(ctx, "p.a", CoverChoice{}); err != nil {
		t.Fatal(err)
	}
	if uploads, _ := e.lib.CoverUploads(ctx); !slices.Equal(uploads, []string{upload2}) {
		t.Errorf("CoverUploads = %v, want only p.b's", uploads)
	}
	if err := e.lib.SetPlaylistCover(ctx, "p.nobody", CoverChoice{Kind: CoverNone}); !errors.Is(err, ErrNotFound) {
		t.Errorf("a playlist the library lacks: %v", err)
	}
}

// A playlist that leaves the library takes its choice with it.
func TestCoverChoiceGoesWithThePlaylist(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	storePlaylist(t, e, "p.gone", "", "a")
	if err := e.lib.SetPlaylistCover(ctx, "p.gone", CoverChoice{Kind: CoverUpload, Template: upload1, Upload: upload1}); err != nil {
		t.Fatal(err)
	}
	if err := e.lib.DropPlaylist(ctx, "p.gone"); err != nil {
		t.Fatal(err)
	}
	if uploads, err := e.lib.CoverUploads(ctx); err != nil || len(uploads) != 0 {
		t.Errorf("CoverUploads = %v, %v; want none", uploads, err)
	}
}
