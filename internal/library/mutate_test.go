package library

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/coredev-uk/sona-go/internal/applemusic"
)

// ampRecorder answers the web player's mutation endpoints with 2xx and
// records them.
type ampRecorder struct {
	mu   sync.Mutex
	reqs []string
}

func (a *ampRecorder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	a.reqs = append(a.reqs, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
	a.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (a *ampRecorder) take() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := a.reqs
	a.reqs = nil
	return out
}

func withAMP(t *testing.T, e *env) *ampRecorder {
	rec := &ampRecorder{}
	srv := httptest.NewServer(rec)
	t.Cleanup(srv.Close)
	e.api.AMPBase = srv.URL
	e.api.Base = srv.URL // ratings go to the public host
	return rec
}

func seedLibrary(t *testing.T, e *env) {
	t.Helper()
	f := e.fake
	f.artists = []map[string]any{artist("r.a", "Zed"), artist("r.b", "Abba")}
	f.albums = []map[string]any{
		album("l.1", "Beta", "Zed", "r.a", "2024-01-02T00:00:00Z"),
		album("l.2", "Alpha", "Abba", "r.b", "2024-03-01T00:00:00Z"),
	}
	f.songs = []map[string]any{
		song("i.1", "One", "Zed", "Beta", "l.1", "r.a"),
		song("i.2", "Two", "Abba", "Alpha", "l.2", "r.b"),
		song("i.3", "Three", "Zed", "Beta", "l.1", "r.a"),
	}
	f.playlists = []map[string]any{playlist("p.1", "Mix", "2024-02-01T00:00:00Z")}
	f.plTracks["p.1"] = []map[string]any{
		song("i.1", "One", "Zed", "Beta", "l.1", "r.a"),
		song("i.2", "Two", "Abba", "Alpha", "l.2", "r.b"),
	}
	f.ratings[applemusic.TypeLibrarySongs] = map[string]int{"i.3": 1, "i.2": -1}
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
}

func TestRecentlyAddedAndFavoriteSongs(t *testing.T) {
	e := newEnv(t)
	seedLibrary(t, e)
	ctx := context.Background()

	added, total, err := e.lib.RecentlyAdded(ctx, ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, a := range added {
		if a.Album != nil {
			got = append(got, a.Album.LibraryID)
		} else {
			got = append(got, a.Playlist.LibraryID)
		}
	}
	// Playlist p.1 was added 2024-01-01, album l.1 2024-01-02, l.2 2024-03-01.
	if total != 3 || !slices.Equal(got, []string{"l.2", "l.1", "p.1"}) {
		t.Fatalf("recently added = %v (total %d)", got, total)
	}
	if page, _, _ := e.lib.RecentlyAdded(ctx, ListOptions{Offset: 1, Limit: 1}); len(page) != 1 || page[0].Album.LibraryID != "l.1" {
		t.Fatalf("page 2 = %+v", page)
	}

	favs, n, err := e.lib.FavoriteSongs(ctx, ListOptions{})
	if err != nil || n != 1 || len(favs) != 1 || favs[0].LibraryID != "i.3" {
		t.Fatalf("favourite songs = %+v (%d), %v", favs, n, err)
	}
}

func TestFavoriteSuggestLessMirror(t *testing.T) {
	e := newEnv(t)
	seedLibrary(t, e)
	rec := withAMP(t, e)
	ctx := context.Background()

	// A catalog song with a library copy: the web player's favourites call,
	// mirrored under both IDs.
	it := Rated{Kind: applemusic.TypeSongs, CatalogID: "c1", LibraryID: "i.1"}
	if err := e.lib.SetFavorite(ctx, it, true); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{applemusic.TypeSongs + ":c1", applemusic.TypeLibrarySongs + ":i.1"} {
		typ, id, _ := strings.Cut(k, ":")
		if v, _ := e.lib.Rating(ctx, typ, id); v != 1 {
			t.Fatalf("%s rating = %d", k, v)
		}
	}
	if favs, n, _ := e.lib.FavoriteSongs(ctx, ListOptions{}); n != 2 || favs[0].LibraryID != "i.1" {
		t.Fatalf("favourites after favouriting i.1 = %+v", favs)
	}
	if err := e.lib.SetFavorite(ctx, it, false); err != nil {
		t.Fatal(err)
	}
	// A private playlist (no catalog copy): the rating itself.
	pl := Rated{Kind: applemusic.TypePlaylists, LibraryID: "p.9"}
	if err := e.lib.SetFavorite(ctx, pl, true); err != nil {
		t.Fatal(err)
	}
	// Suggest Less goes to the catalog resource when there is one.
	if err := e.lib.SetSuggestLess(ctx, Rated{Kind: applemusic.TypeArtists, CatalogID: "166566"}, true); err != nil {
		t.Fatal(err)
	}
	if v, _ := e.lib.Rating(ctx, applemusic.TypeArtists, "166566"); v != -1 {
		t.Fatalf("artist rating = %d", v)
	}
	if err := e.lib.SetSuggestLess(ctx, Rated{Kind: applemusic.TypeArtists, CatalogID: "166566"}, false); err != nil {
		t.Fatal(err)
	}
	if v, _ := e.lib.Rating(ctx, applemusic.TypeArtists, "166566"); v != 0 {
		t.Fatalf("artist rating after undo = %d", v)
	}
	want := []string{
		"POST /v1/me/favorites?ids%5Bsongs%5D=c1",
		"DELETE /v1/me/favorites?ids%5Bsongs%5D=c1",
		"PUT /v1/me/ratings/library-playlists/p.9?",
		"PUT /v1/me/ratings/artists/166566?",
		"DELETE /v1/me/ratings/artists/166566?",
	}
	if got := rec.take(); !slices.Equal(got, want) {
		t.Fatalf("requests:\n%s", strings.Join(got, "\n"))
	}
}

func TestRemoveAlbumForgetsItsSongs(t *testing.T) {
	e := newEnv(t)
	seedLibrary(t, e)
	rec := withAMP(t, e)
	ctx := context.Background()

	if err := e.lib.Remove(ctx, applemusic.TypeAlbums, "l.1"); err != nil {
		t.Fatal(err)
	}
	if got := rec.take(); !slices.Equal(got, []string{"DELETE /v1/me/library/albums/l.1?"}) {
		t.Fatalf("requests = %v", got)
	}
	c, _ := e.lib.Counts(ctx)
	if c.Albums != 1 || c.Songs != 1 {
		t.Fatalf("counts after removing l.1 = %+v", c)
	}
	if _, _, err := e.lib.Album(ctx, "l.1"); err != ErrNotFound {
		t.Fatalf("album l.1 still readable: %v", err)
	}
	// i.3 was a favourite on that album.
	if favs, n, _ := e.lib.FavoriteSongs(ctx, ListOptions{}); n != 0 || len(favs) != 0 {
		t.Fatalf("favourites = %+v", favs)
	}
	// i.1 also left the playlist.
	p, tracks, err := e.lib.Playlist(ctx, "p.1", ListOptions{})
	if err != nil || len(tracks) != 1 || tracks[0].LibraryID != "i.2" || p.TrackCount != 1 {
		t.Fatalf("playlist = %+v %+v %v", p, tracks, err)
	}
	if hits, _ := e.lib.Search(ctx, "One", 5); len(hits) != 0 {
		t.Fatalf("search still finds removed song: %+v", hits)
	}

	// Removing an artist takes its albums and songs.
	if err := e.lib.Remove(ctx, applemusic.TypeArtists, "r.b"); err != nil {
		t.Fatal(err)
	}
	if c, _ := e.lib.Counts(ctx); c != (Counts{Playlists: 1, Artists: 1}) {
		t.Fatalf("counts after removing r.b = %+v", c)
	}
	// Music videos are not stored locally: Apple only.
	if err := e.lib.Remove(ctx, applemusic.TypeMusicVideos, "i.v"); err != nil {
		t.Fatal(err)
	}
	if err := e.lib.Forget(ctx, applemusic.TypeSongs, "i.unknown"); err != nil {
		t.Fatal(err)
	}
}
