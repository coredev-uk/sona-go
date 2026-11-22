package library

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/coredev-uk/sona-go/internal/applemusic"
)

// catalogRequests is the catalogue song lookups the fake Apple served.
func catalogRequests(e *env) []string {
	var out []string
	for _, r := range e.fake.seen() {
		if strings.HasPrefix(r, "/v1/catalog/") {
			out = append(out, r)
		}
	}
	return out
}

// uploadSong is a library song that has no catalogue copy.
func uploadSong(id, name, artist, artistID string) map[string]any {
	s := song(id, name, artist, "Mine", "", artistID)
	delete(s["attributes"].(map[string]any)["playParams"].(map[string]any), "catalogId")
	return s
}

// collabLibrary is a library whose songs by "Ada & Bo" and "Ada, Bo & Cy"
// the catalogue credits to Ada and Bo (library artists) and Cy (not one).
func collabLibrary(e *env) {
	e.sync.Storefront = func() string { return "us" }
	f := e.fake
	f.artists = []map[string]any{artist("r.ada", "Ada"), artist("r.bo", "Bo"), artist("r.duo", "Ada & Bo"), artist("r.trio", "Ada, Bo & Cy"), artist("r.up", "Up & Down")}
	f.albums = []map[string]any{
		album("l.ada", "Ada Alone", "Ada", "r.ada", "2024-01-01T00:00:00Z"),
		album("l.bo", "Bo Alone", "Bo", "r.bo", "2024-01-02T00:00:00Z"),
	}
	f.songs = []map[string]any{
		song("i.s1", "Solo A", "Ada", "Ada Alone", "l.ada", "r.ada"),
		song("i.s2", "Solo B", "Bo", "Bo Alone", "l.bo", "r.bo"),
		song("i.c1", "Together", "Ada & Bo", "Ada Alone", "l.ada", "r.duo"),
		song("i.c2", "Three", "Ada, Bo & Cy", "Ada Alone", "l.ada", "r.trio"),
		uploadSong("i.up", "Mine", "Up & Down", "r.up"),
	}
	f.credits = map[string][][2]string{
		"cc1": {{"cr.ada", "Ada"}, {"cr.bo", "Bo"}},
		"cc2": {{"cr.ada", "Ada"}, {"cr.bo", "Bo"}, {"900", "Cy"}},
	}
}

func artistNamed(t *testing.T, index []ArtistSummary, name string) (ArtistSummary, bool) {
	t.Helper()
	for _, a := range index {
		if a.Name == name {
			return a, true
		}
	}
	return ArtistSummary{}, false
}

func songNames(songs []Song) []string {
	out := make([]string, len(songs))
	for i, s := range songs {
		out[i] = s.Name
	}
	slices.Sort(out)
	return out
}

func TestCreditedSongsGroupUnderEachArtist(t *testing.T) {
	e := newEnv(t)
	collabLibrary(e)
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	index, err := e.lib.ArtistIndex(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][3]string{ // name → library ID, albums, songs
		"Ada": {"r.ada", "1", "3"}, "Bo": {"r.bo", "1", "3"},
		"Cy":        {"", "0", "1"}, // credited only: no library artist
		"Up & Down": {"r.up", "0", "1"},
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
			t.Errorf("%q is still listed though every song of it is credited elsewhere", gone)
		}
	}
	if len(index) != len(want) {
		t.Errorf("index = %+v, want %d entries", index, len(want))
	}
	cy, _ := artistNamed(t, index, "Cy")
	if cy.CatalogID != "900" {
		t.Errorf("Cy's key = %q, want the catalogue ID 900", cy.CatalogID)
	}

	// The compilation of the collaborations lists under each artist.
	for id, songs := range map[string][]string{
		"r.ada": {"Solo A", "Three", "Together"},
		"r.bo":  {"Solo B", "Three", "Together"},
		"900":   {"Three"},
		"r.up":  {"Mine"},
	} {
		a, albums, got, err := e.lib.Artist(ctx, id)
		if err != nil {
			t.Fatalf("Artist(%s): %v", id, err)
		}
		if !slices.Equal(songNames(got), songs) {
			t.Errorf("Artist(%s) songs = %v, want %v", id, songNames(got), songs)
		}
		if id == "900" && (a.Name != "Cy" || a.LibraryID != "" || len(albums) != 0) {
			t.Errorf("credit-only artist = %+v with %d albums", a, len(albums))
		}
		if _, albums, err := e.lib.ArtistAlbums(ctx, id); err != nil || (id == "r.ada" && len(albums) != 1) {
			t.Errorf("ArtistAlbums(%s) = %d albums, %v", id, len(albums), err)
		}
	}

	// Search lists the same artists.
	for term, id := range map[string]string{"Cy": "900", "Ada": "r.ada"} {
		r, err := e.lib.SearchAll(ctx, term, 10)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.ContainsFunc(r.Artists, func(a Artist) bool { return cmpKey(a) == id }) {
			t.Errorf("search %q: artists %+v lack %s", term, r.Artists, id)
		}
	}
	if r, err := e.lib.SearchAll(ctx, "Down", 10); err != nil || len(r.Artists) != 1 {
		t.Errorf("an upload's artist stays searchable: %+v, %v", r.Artists, err)
	}
}

func cmpKey(a Artist) string {
	if a.LibraryID != "" {
		return a.LibraryID
	}
	return a.CatalogID
}

func TestACombinedArtistWithAlbumsStaysListed(t *testing.T) {
	e := newEnv(t)
	collabLibrary(e)
	e.fake.albums = append(e.fake.albums, album("l.duo", "Both", "Ada & Bo", "r.duo", "2024-01-03T00:00:00Z"))
	e.fake.songs = append(e.fake.songs, song("i.c3", "Both Of Us", "Ada & Bo", "Both", "l.duo", "r.duo"))
	e.fake.credits["cc3"] = [][2]string{{"cr.ada", "Ada"}, {"cr.bo", "Bo"}}
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	index, err := e.lib.ArtistIndex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	duo, ok := artistNamed(t, index, "Ada & Bo")
	if !ok || duo.Albums != 1 || duo.Songs != 0 {
		t.Errorf("Ada & Bo = %+v (%v), want listed with its album and no songs: albums are not attributed by credit yet", duo, ok)
	}
}

func TestCreditsAreAskedInBatchesAndOnce(t *testing.T) {
	e := newEnv(t)
	e.sync.Storefront = func() string { return "us" }
	f := e.fake
	f.artists = []map[string]any{artist("r.x", "X & Y")}
	f.albums = []map[string]any{album("l.x", "Album", "X & Y", "r.x", "2024-01-01T00:00:00Z")}
	f.credits = map[string][][2]string{}
	for i := range 250 {
		id := fmt.Sprintf("i.%d", i)
		f.songs = append(f.songs, song(id, fmt.Sprint("Song ", i), "X & Y", "Album", "l.x", "r.x"))
		f.credits[fmt.Sprintf("c%d", i)] = [][2]string{{"cr.x", "X"}, {"cr.y", "Y"}}
	}
	// Songs that name one artist are never asked about.
	f.songs = append(f.songs, song("i.solo", "Solo", "Plain", "Album", "l.x", "r.x"))
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	reqs := catalogRequests(e)
	if len(reqs) != 3 {
		t.Fatalf("%d catalogue requests for 250 songs, want 3 (100 per request): %v", len(reqs), reqs)
	}
	var asked int
	for _, r := range reqs {
		if !strings.Contains(r, "include=artists") {
			t.Errorf("request %q does not ask for the artists", r)
		}
		ids := strings.Split(strings.SplitN(strings.SplitN(r, "ids=", 2)[1], "&", 2)[0], "%2C")
		if len(ids) > 100 {
			t.Errorf("a request names %d songs", len(ids))
		}
		asked += len(ids)
	}
	if asked != 250 {
		t.Errorf("%d songs asked about, want the 250 that may name several", asked)
	}
	var n int
	if err := e.db.R.QueryRowContext(context.Background(), `SELECT count(*) FROM track_artists`).Scan(&n); err != nil || n != 500 {
		t.Errorf("track_artists = %d, %v, want 500", n, err)
	}

	// Nothing is asked again; a song added later is the only one asked.
	e.fake.reset()
	if err := e.runFull(t); err != nil {
		t.Fatal(err)
	}
	if reqs := catalogRequests(e); len(reqs) != 0 {
		t.Errorf("a second sync asked again: %v", reqs)
	}
	f.songs = append(f.songs, song("i.new", "New", "X & Y", "Album", "l.x", "r.x"))
	f.credits["cnew"] = [][2]string{{"cr.x", "X"}}
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	if reqs := catalogRequests(e); len(reqs) != 1 || strings.Count(reqs[0], "cnew") != 1 || strings.Contains(reqs[0], "%2C") {
		t.Errorf("requests for one new song = %v", reqs)
	}
}

func TestCreditsGoWithTheirSong(t *testing.T) {
	e := newEnv(t)
	collabLibrary(e)
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	count := func() (n int) {
		if err := e.db.R.QueryRowContext(ctx, `SELECT count(*) FROM track_artists`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if got := count(); got != 5 {
		t.Fatalf("track_artists = %d, want 5", got)
	}
	if err := e.lib.Forget(ctx, applemusic.TypeSongs, "i.c2"); err != nil {
		t.Fatal(err)
	}
	if got := count(); got != 2 {
		t.Errorf("track_artists = %d after forgetting the song credited to three, want 2", got)
	}
}

func TestNoCreditsWithoutAStorefront(t *testing.T) {
	e := newEnv(t)
	collabLibrary(e)
	e.sync.Storefront = nil
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	if reqs := catalogRequests(e); len(reqs) != 0 {
		t.Errorf("asked the catalogue without a storefront: %v", reqs)
	}
	index, err := e.lib.ArtistIndex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if duo, ok := artistNamed(t, index, "Ada & Bo"); !ok || duo.Songs != 1 {
		t.Errorf("uncredited songs stay under their library artist: %+v", duo)
	}
}
