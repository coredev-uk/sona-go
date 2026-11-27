package library

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/coredev-uk/sona-go/internal/applemusic"
)

// albumCreditsLibrary is collabLibrary with albums of the kinds the grouping
// tells apart:
//   - "Both" by "Ada & Bo" and "Trio" by "Ada, Bo & Cy": the catalogue credits
//     them to Ada and Bo, and to Ada, Bo and Cy (Cy is no library artist);
//   - "Now 123", a compilation by "Various Artists": no separator in the
//     string, so never asked about, and Apple credits it to no artist anyway;
//   - "Cast Recording" by "Cast, Crew & Co": asked about, and answered with no
//     artist at all (a compilation too);
//   - "Mine" by "Up & Down", an upload with no catalogue copy.
func albumCreditsLibrary(e *env) {
	collabLibrary(e)
	f := e.fake
	f.artists = append(f.artists, artist("r.va", "Various Artists"), artist("r.sb", "Some Band"), artist("r.cast", "Cast, Crew & Co"))
	upAlbum := album("l.up", "Mine", "Up & Down", "r.up", "2024-01-08T00:00:00Z")
	delete(upAlbum["relationships"].(map[string]any), "catalog")
	f.albums = append(f.albums,
		album("l.duo", "Both", "Ada & Bo", "r.duo", "2024-01-03T00:00:00Z"),
		album("l.trio", "Trio", "Ada, Bo & Cy", "r.trio", "2024-01-04T00:00:00Z"),
		album("l.va", "Now 123", "Various Artists", "r.va", "2024-01-05T00:00:00Z"),
		album("l.cast", "Cast Recording", "Cast, Crew & Co", "r.cast", "2024-01-06T00:00:00Z"),
		upAlbum,
	)
	upSong := uploadSong("i.up2", "Mine Too", "Up & Down", "r.up")
	upSong["relationships"].(map[string]any)["albums"] = map[string]any{"data": []any{map[string]any{"id": "l.up", "type": "library-albums"}}}
	f.songs = append(f.songs,
		song("i.d1", "Both Of Us", "Ada & Bo", "Both", "l.duo", "r.duo"),
		song("i.t1", "Trio Song", "Ada, Bo & Cy", "Trio", "l.trio", "r.trio"),
		song("i.v1", "Hit", "Some Band", "Now 123", "l.va", "r.sb"),
		song("i.k1", "Overture", "Cast, Crew & Co", "Cast Recording", "l.cast", "r.cast"),
		upSong,
	)
	f.credits["cd1"] = [][2]string{{"cr.ada", "Ada"}, {"cr.bo", "Bo"}}
	f.credits["ct1"] = [][2]string{{"cr.ada", "Ada"}, {"cr.bo", "Bo"}, {"900", "Cy"}}
	f.albumCredits = map[string][][2]string{
		"cl.duo":  {{"cr.ada", "Ada"}, {"cr.bo", "Bo"}},
		"cl.trio": {{"cr.ada", "Ada"}, {"cr.bo", "Bo"}, {"900", "Cy"}},
		"cl.cast": {}, // a compilation: Apple credits it to no artist
	}
}

func albumNames(albums []Album) []string {
	out := make([]string, len(albums))
	for i, a := range albums {
		out[i] = a.Name
	}
	slices.Sort(out)
	return out
}

func TestCollaborationAlbumsGroupUnderEachCreditedArtist(t *testing.T) {
	e := newEnv(t)
	albumCreditsLibrary(e)
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	index, err := e.lib.ArtistIndex(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][3]string{ // name → library ID, albums, songs
		"Ada": {"r.ada", "3", "5"}, "Bo": {"r.bo", "3", "5"}, // their own album, Both and Trio
		"Cy":        {"", "1", "2"}, // credited only: no library artist; Trio and its song
		"Up & Down": {"r.up", "1", "2"},
		// A compilation stays with its library artist; its songs list under
		// their own artists.
		"Various Artists": {"r.va", "1", "0"},
		"Some Band":       {"r.sb", "0", "1"},
		// An answer of no artist leaves the album, and the song, under the string.
		"Cast, Crew & Co": {"r.cast", "1", "1"},
	}
	for name, w := range want {
		a, ok := artistNamed(t, index, name)
		if !ok {
			t.Errorf("no %q entry in %v", name, index)
			continue
		}
		if got := [3]string{a.LibraryID, fmt.Sprint(a.Albums), fmt.Sprint(a.Songs)}; got != w {
			t.Errorf("%s = %v, want %v", name, got, w)
		}
	}
	for _, gone := range []string{"Ada & Bo", "Ada, Bo & Cy"} {
		if _, ok := artistNamed(t, index, gone); ok {
			t.Errorf("%q is still listed though every song and album of it is credited elsewhere", gone)
		}
	}
	if len(index) != len(want) {
		t.Errorf("index = %+v, want %d entries", index, len(want))
	}
	if cy, _ := artistNamed(t, index, "Cy"); cy.CatalogID != "900" {
		t.Errorf("Cy's key = %q, want the catalogue ID 900", cy.CatalogID)
	}

	// An artist's albums: theirs, and every collaboration credited to them.
	for key, albums := range map[string][]string{
		"r.ada": {"Ada Alone", "Both", "Trio"},
		"r.bo":  {"Bo Alone", "Both", "Trio"},
		"900":   {"Trio"},
		"r.va":  {"Now 123"},
		"r.up":  {"Mine"},
		// The combined library artist still holds the albums it carries itself.
		"r.duo": {"Both"},
	} {
		a, got, err := e.lib.ArtistAlbums(ctx, key)
		if err != nil {
			t.Fatalf("ArtistAlbums(%s): %v", key, err)
		}
		if !slices.Equal(albumNames(got), albums) {
			t.Errorf("ArtistAlbums(%s) = %v, want %v", key, albumNames(got), albums)
		}
		if key == "900" && (a.Name != "Cy" || a.LibraryID != "" || a.CatalogID != "900") {
			t.Errorf("credit-only artist = %+v", a)
		}
		_, albums2, songs, err := e.lib.Artist(ctx, key)
		if err != nil || len(albums2) != len(got) {
			t.Errorf("Artist(%s) = %d albums, %d songs, %v", key, len(albums2), len(songs), err)
		}
	}
	// The compilation is nobody's but its library artist's.
	for _, key := range []string{"r.ada", "r.bo", "900", "r.sb", "r.cast"} {
		if _, albums, err := e.lib.ArtistAlbums(ctx, key); err != nil || slices.Contains(albumNames(albums), "Now 123") {
			t.Errorf("ArtistAlbums(%s) lists the compilation: %v, %v", key, albumNames(albums), err)
		}
	}

	// Search lists the same artists, and no combined one.
	for term, key := range map[string]string{"Cy": "900", "Ada": "r.ada", "Various": "r.va"} {
		r, err := e.lib.SearchAll(ctx, term, 10)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.ContainsFunc(r.Artists, func(a Artist) bool { return cmpKey(a) == key }) {
			t.Errorf("search %q: artists %+v lack %s", term, r.Artists, key)
		}
	}
	for _, term := range []string{"Ada", "Bo", "Cy"} {
		r, err := e.lib.SearchAll(ctx, term, 10)
		if err != nil {
			t.Fatal(err)
		}
		if slices.ContainsFunc(r.Artists, func(a Artist) bool { return a.Name == "Ada & Bo" || a.Name == "Ada, Bo & Cy" }) {
			t.Errorf("search %q lists a combined artist: %+v", term, r.Artists)
		}
	}
}

// An artist the library knows by an album alone is listed, with that album's
// cover.
func TestAnArtistCreditedOnAnAlbumAloneIsListed(t *testing.T) {
	e := newEnv(t)
	e.sync.Storefront = func() string { return "us" }
	f := e.fake
	f.artists = []map[string]any{artist("r.duo", "Dee & Eff")}
	f.albums = []map[string]any{album("l.d", "Split", "Dee & Eff", "r.duo", "2024-01-01T00:00:00Z")}
	f.albums[0]["attributes"].(map[string]any)["artwork"] = map[string]any{"url": "https://art/split/{w}x{h}bb.jpg"}
	// The catalogue knows no credit of the song: it stays under the string.
	f.songs = []map[string]any{song("i.d", "Side A", "Dee & Eff", "Split", "l.d", "r.duo")}
	f.albumCredits = map[string][][2]string{"cl.d": {{"800", "Dee"}, {"801", "Eff"}}}
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	index, err := e.lib.ArtistIndex(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for key, name := range map[string]string{"800": "Dee", "801": "Eff"} {
		a, ok := artistNamed(t, index, name)
		if !ok || a.LibraryID != "" || a.CatalogID != key || a.Albums != 1 || a.Songs != 0 {
			t.Errorf("%s = %+v (%v), want a credit-only artist with the album alone", name, a, ok)
		}
		if a.ArtworkTemplate == "" {
			t.Errorf("%s has no cover though the album credited to them has one", name)
		}
		if _, albums, err := e.lib.ArtistAlbums(ctx, key); err != nil || !slices.Equal(albumNames(albums), []string{"Split"}) {
			t.Errorf("ArtistAlbums(%s) = %v, %v", key, albumNames(albums), err)
		}
	}
	// The song has no credit, so the combined entry keeps it, and not the album.
	if duo, ok := artistNamed(t, index, "Dee & Eff"); !ok || duo.Albums != 0 || duo.Songs != 1 {
		t.Errorf("Dee & Eff = %+v (%v), want its song alone", duo, ok)
	}
}

func TestAlbumCreditsAreAskedInBatchesAndOnce(t *testing.T) {
	e := newEnv(t)
	e.sync.Storefront = func() string { return "us" }
	f := e.fake
	f.artists = []map[string]any{artist("r.x", "X & Y")}
	f.albumCredits = map[string][][2]string{}
	for i := range 250 {
		id := fmt.Sprintf("l.%d", i)
		f.albums = append(f.albums, album(id, fmt.Sprint("Album ", i), "X & Y", "r.x", "2024-01-01T00:00:00Z"))
		f.songs = append(f.songs, song(fmt.Sprintf("i.%d", i), fmt.Sprint("Song ", i), "Plain", fmt.Sprint("Album ", i), id, "r.x"))
		f.albumCredits["c"+id] = [][2]string{{"cr.x", "X"}, {"cr.y", "Y"}}
	}
	// Albums that name one artist are never asked about.
	f.albums = append(f.albums, album("l.solo", "Solo", "Plain", "r.x", "2024-01-01T00:00:00Z"))
	f.songs = append(f.songs, song("i.solo", "Solo", "Plain", "Solo", "l.solo", "r.x"))
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	reqs := albumRequests(e)
	if len(reqs) != 3 {
		t.Fatalf("%d catalogue requests for 250 albums, want 3 (100 per request): %v", len(reqs), reqs)
	}
	var asked int
	for _, r := range reqs {
		if !strings.Contains(r, "/albums?ids=") || !strings.Contains(r, "include=artists") {
			t.Errorf("request %q is not a batch that asks for the artists", r)
		}
		ids := requestedIDs(r)
		if len(ids) > 100 {
			t.Errorf("a request names %d albums", len(ids))
		}
		asked += len(ids)
	}
	if asked != 250 {
		t.Errorf("%d albums asked about, want the 250 that may name several", asked)
	}
	if reqs := catalogRequests(e); len(reqs) != 0 {
		t.Errorf("songs naming one artist were asked about: %v", reqs)
	}
	var n int
	if err := e.db.R.QueryRowContext(context.Background(), `SELECT count(*) FROM album_artists`).Scan(&n); err != nil || n != 500 {
		t.Errorf("album_artists = %d, %v, want 500", n, err)
	}

	// Nothing is asked again; an album added later is the only one asked.
	f.songs = append(f.songs, song("i.new", "New", "Plain", "Album new", "l.new", "r.x"))
	f.albums = append(f.albums, album("l.new", "Album new", "X & Y", "r.x", "2024-01-02T00:00:00Z"))
	f.albumCredits["cl.new"] = [][2]string{{"cr.x", "X"}}
	e.fake.reset()
	if err := e.runFull(t); err != nil {
		t.Fatal(err)
	}
	reqs = albumRequests(e)
	if len(reqs) != 1 || len(requestedIDs(reqs[0])) != 1 || !strings.Contains(reqs[0], "cl.new") {
		t.Errorf("requests after one new album = %v, want one for it alone", reqs)
	}
	e.fake.reset()
	if err := e.runFull(t); err != nil {
		t.Fatal(err)
	}
	if reqs := albumRequests(e); len(reqs) != 0 {
		t.Errorf("a further sync asked again: %v", reqs)
	}
}

func TestAlbumCreditsGoWithTheirAlbum(t *testing.T) {
	e := newEnv(t)
	albumCreditsLibrary(e)
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	count := func() (n int) {
		if err := e.db.R.QueryRowContext(ctx, `SELECT count(*) FROM album_artists`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if got := count(); got != 5 {
		t.Fatalf("album_artists = %d, want 5 (Both: 2, Trio: 3)", got)
	}
	if err := e.lib.Forget(ctx, applemusic.TypeAlbums, "l.trio"); err != nil {
		t.Fatal(err)
	}
	if got := count(); got != 2 {
		t.Errorf("album_artists = %d after forgetting the album credited to three, want 2", got)
	}
	index, err := e.lib.ArtistIndex(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if cy, ok := artistNamed(t, index, "Cy"); !ok || cy.Albums != 0 || cy.Songs != 1 {
		t.Errorf("Cy = %+v (%v), want the song credited to three alone: Trio is gone", cy, ok)
	}
	if ada, _ := artistNamed(t, index, "Ada"); ada.Albums != 2 {
		t.Errorf("Ada = %+v, want her album and Both", ada)
	}
}

// An album that stops naming several loses its credits; the albums that name
// one are marked as checked, so a sync does not read them again.
func TestStaleAlbumCreditsAreClearedAndAskedAgainOnANewCatalogueID(t *testing.T) {
	e := newEnv(t)
	albumCreditsLibrary(e)
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	checked := func(libraryID string) (n int, c int64) {
		ctx := context.Background()
		if err := e.db.R.QueryRowContext(ctx, `SELECT count(*) FROM album_artists WHERE album_id = (SELECT id FROM albums WHERE library_id = ?)`, libraryID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if err := e.db.R.QueryRowContext(ctx, `SELECT credits_checked FROM albums WHERE library_id = ?`, libraryID).Scan(&c); err != nil {
			t.Fatal(err)
		}
		return n, c
	}
	if n, c := checked("l.duo"); n != 2 || c == 0 {
		t.Fatalf("credits of Both = %d (checked %d), want 2", n, c)
	}
	if _, c := checked("l.ada"); c == 0 {
		t.Error("an album naming one artist is not marked as checked")
	}
	if _, c := checked("l.up"); c != 0 {
		t.Error("an album with no catalogue copy was marked as asked about")
	}

	// Apple now names Ada alone on Both: its credits are stale.
	duo := e.fake.albums[slices.IndexFunc(e.fake.albums, func(a map[string]any) bool { return a["id"] == "l.duo" })]
	duo["attributes"].(map[string]any)["artistName"] = "Ada"
	e.fake.reset()
	if err := e.runFull(t, DomainAlbums); err != nil {
		t.Fatal(err)
	}
	if n, c := checked("l.duo"); n != 0 || c == 0 {
		t.Errorf("credits of Both = %d (checked %d) after its artist string named one, want none and checked", n, c)
	}
	if reqs := albumRequests(e); len(reqs) != 0 {
		t.Errorf("album requests = %v, want none: the string names one artist", reqs)
	}

	// Its catalogue album changes (and names several again): asked afresh.
	duo["attributes"].(map[string]any)["artistName"] = "Ada & Bo"
	duo["relationships"].(map[string]any)["catalog"].(map[string]any)["data"].([]any)[0].(map[string]any)["id"] = "cl.duo2"
	e.fake.albumCredits["cl.duo2"] = [][2]string{{"cr.bo", "Bo"}}
	e.fake.reset()
	if err := e.runFull(t, DomainAlbums); err != nil {
		t.Fatal(err)
	}
	reqs := albumRequests(e)
	if len(reqs) != 1 || len(requestedIDs(reqs[0])) != 1 || !strings.Contains(reqs[0], "cl.duo2") {
		t.Fatalf("requests = %v, want one for the new catalogue ID", reqs)
	}
	if n, _ := checked("l.duo"); n != 1 {
		t.Errorf("credits of Both = %d, want the new answer's 1", n)
	}
}

func TestNoAlbumCreditsWithoutAStorefront(t *testing.T) {
	e := newEnv(t)
	albumCreditsLibrary(e)
	e.sync.Storefront = nil
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	if reqs := albumRequests(e); len(reqs) != 0 {
		t.Errorf("asked the catalogue without a storefront: %v", reqs)
	}
	index, err := e.lib.ArtistIndex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if duo, ok := artistNamed(t, index, "Ada & Bo"); !ok || duo.Albums != 1 {
		t.Errorf("uncredited albums stay under their library artist: %+v", duo)
	}
}
