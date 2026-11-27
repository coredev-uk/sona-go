package library

import (
	"context"
	"slices"
	"testing"
)

// Apple's library artist for "A & B" carries A's catalogue ID: the library
// artist "Mira Vale & Theo Lund" has Mira Vale's ID, and "Kai Moreno & Ivy
// Lane" Kai Moreno's. withCatalogID gives a fake library artist that
// catalogue ID.
func withCatalogID(a map[string]any, id string) map[string]any {
	cat := a["relationships"].(map[string]any)["catalog"].(map[string]any)["data"].([]any)[0].(map[string]any)
	cat["id"] = id
	return a
}

const (
	miraID = "100001"
	theoID = "100002"
	miraIm = "https://art/mira/{w}x{h}.jpg"
)

// miraLibrary is the combined case: the library artist "Mira Vale & Theo
// Lund" (Mira Vale's catalogue ID), the library artist "Theo Lund" (inserted
// after it), and no library artist for Mira Vale. The song "Signals" and its
// album are by "Mira Vale & Theo Lund" in Apple's library, and credited to
// Mira Vale and Theo Lund in the catalogue.
func miraLibrary(e *env) {
	e.sync.Storefront = func() string { return "us" }
	f := e.fake
	f.artists = []map[string]any{
		withCatalogID(artist("r.az", "Mira Vale & Theo Lund"), miraID),
		withCatalogID(artist("r.zl", "Theo Lund"), theoID),
	}
	f.albums = []map[string]any{album("l.w", "Signals - Single", "Mira Vale & Theo Lund", "r.az", "2024-01-01T00:00:00Z")}
	f.songs = []map[string]any{song("i.w", "Signals", "Mira Vale & Theo Lund", "Signals - Single", "l.w", "r.az")}
	credited := [][2]string{{miraID, "Mira Vale"}, {theoID, "Theo Lund"}}
	f.credits = map[string][][2]string{"cw": credited}
	f.albumCredits = map[string][][2]string{"cl.w": credited}
	f.creditArtwork = map[string]string{miraID: miraIm}
}

func indexNames(index []ArtistSummary) []string {
	out := make([]string, len(index))
	for i, a := range index {
		out[i] = a.Name
	}
	return out
}

// A combined library artist that carries the first artist's catalogue ID does
// not take that artist's credits: with no library artist of their own, the
// first artist is listed alone (by catalogue ID) with the song and the album,
// the second keeps their library artist, and the combined one is gone.
func TestCombinedArtistDoesNotTakeTheFirstArtistsCredits(t *testing.T) {
	e := newEnv(t)
	miraLibrary(e)
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	index, err := e.lib.ArtistIndex(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := indexNames(index); !slices.Equal(got, []string{"Mira Vale", "Theo Lund"}) {
		t.Fatalf("artists = %v, want Mira Vale and Theo Lund alone", got)
	}
	mira, theo := index[0], index[1]
	if mira.LibraryID != "" || mira.CatalogID != miraID || mira.Albums != 1 || mira.Songs != 1 {
		t.Errorf("Mira Vale = %+v, want credit-only, 1 album, 1 song", mira)
	}
	if theo.LibraryID != "r.zl" || theo.Albums != 1 || theo.Songs != 1 {
		t.Errorf("Theo Lund = %+v, want their library artist, 1 album, 1 song", theo)
	}
	for _, key := range []string{miraID, "r.zl"} {
		_, albums, songs, err := e.lib.Artist(ctx, key)
		if err != nil || !slices.Equal(albumNames(albums), []string{"Signals - Single"}) || !slices.Equal(songNames(songs), []string{"Signals"}) {
			t.Errorf("Artist(%s) = %v, %v, %v", key, albumNames(albums), songNames(songs), err)
		}
	}
	_, albums, songs, err := e.lib.Artist(ctx, "r.az")
	if err != nil || len(albums) != 0 || len(songs) != 0 {
		t.Errorf("Artist(r.az) = %v, %v, %v, want nothing", albumNames(albums), songNames(songs), err)
	}
	// Search lists the same artists.
	for term, want := range map[string]bool{"Mira": true, "Theo": true} {
		r, err := e.lib.SearchAll(ctx, term, 10)
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range r.Artists {
			if a.Name == "Mira Vale & Theo Lund" {
				t.Errorf("search %q lists the combined artist", term)
			}
		}
		if got := len(r.Artists) > 0; got != want {
			t.Errorf("search %q artists = %+v", term, r.Artists)
		}
	}
}

// A credit-only artist shows the picture the credits pass stored from the
// answer it already gets (include=artists carries each artist's artwork), in
// the list and on their page.
func TestCreditOnlyArtistShowsTheCataloguePicture(t *testing.T) {
	e := newEnv(t)
	miraLibrary(e)
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	index, err := e.lib.ArtistIndex(ctx)
	if err != nil {
		t.Fatal(err)
	}
	mira, ok := artistNamed(t, index, "Mira Vale")
	if !ok || mira.ArtworkTemplate != miraIm {
		t.Errorf("Mira Vale in the list = %+v, want the artwork %s", mira, miraIm)
	}
	a, _, err := e.lib.ArtistAlbums(ctx, miraID)
	if err != nil || a.ArtworkTemplate != miraIm {
		t.Errorf("Mira Vale's page = %+v, %v, want the artwork %s", a, err, miraIm)
	}
	if n := catalogArtistRows(t, e, miraID); n != 1 {
		t.Errorf("%d catalogue rows for Mira Vale, want 1", n)
	}
	// An artist the answer gives no artwork for is not stored (nothing to add).
	if n := catalogArtistRows(t, e, theoID); n != 0 {
		t.Errorf("%d catalogue rows for Theo Lund, whose answer had no artwork", n)
	}
}

func catalogArtistRows(t *testing.T, e *env, catalogID string) int {
	t.Helper()
	var n int
	err := e.db.R.QueryRow(`SELECT count(*) FROM artists WHERE library_id IS NULL AND catalog_id = ?`, catalogID).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// With the credit's artist holding a library row of their own, inserted after
// the combined one (a higher ID) that shares their catalogue ID, the credit
// still goes to that artist's row: the name decides, not the insertion order.
func TestCreditGoesToTheArtistOfThatNameWhateverTheOrder(t *testing.T) {
	e := newEnv(t)
	e.sync.Storefront = func() string { return "us" }
	f := e.fake
	const kai, ivy = "100003", "100004"
	f.artists = []map[string]any{
		withCatalogID(artist("r.aa", "Kai Moreno & Ivy Lane"), kai),
		withCatalogID(artist("r.ac", "Kai Moreno & Rhys Hale"), kai),
		withCatalogID(artist("r.aw", "Kai Moreno"), kai),
	}
	f.albums = []map[string]any{album("l.al", "Echoes", "Kai Moreno", "r.aw", "2024-01-01T00:00:00Z")}
	f.songs = []map[string]any{
		song("i.a1", "Echoes Pt. II", "Kai Moreno & Ivy Lane", "Echoes", "l.al", "r.aa"),
		song("i.a2", "Echoes Pt. III", "Kai Moreno & Rhys Hale", "Echoes", "l.al", "r.ac"),
	}
	f.credits = map[string][][2]string{
		"ca1": {{kai, "Kai Moreno"}, {ivy, "Ivy Lane"}},
		"ca2": {{kai, "kai moreno"}, {"900", "Rhys Hale"}}, // folded: case
	}
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	index, err := e.lib.ArtistIndex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := indexNames(index); !slices.Equal(got, []string{"Ivy Lane", "Kai Moreno", "Rhys Hale"}) {
		t.Fatalf("artists = %v, want Ivy Lane, Kai Moreno and Rhys Hale", got)
	}
	if km := index[1]; km.LibraryID != "r.aw" || km.Songs != 2 || km.Albums != 1 {
		t.Errorf("Kai Moreno = %+v, want their library artist with both songs and their album", km)
	}
	if il := index[0]; il.LibraryID != "" || il.CatalogID != ivy || il.Songs != 1 {
		t.Errorf("Ivy Lane = %+v, want a credit-only artist with 1 song", il)
	}
}

// A combined library artist keeps what Apple's catalogue does not credit: an
// upload under "A & B" (no catalogue copy, so no credits) stays listed under
// it, with that item only; the credited song and album go to the artists.
func TestCombinedArtistKeepsWhatIsUncredited(t *testing.T) {
	e := newEnv(t)
	miraLibrary(e)
	up := uploadSong("i.up", "Demo", "Mira Vale & Theo Lund", "r.az")
	e.fake.songs = append(e.fake.songs, up)
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	index, err := e.lib.ArtistIndex(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := indexNames(index); !slices.Equal(got, []string{"Mira Vale", "Mira Vale & Theo Lund", "Theo Lund"}) {
		t.Fatalf("artists = %v", got)
	}
	combined, _ := artistNamed(t, index, "Mira Vale & Theo Lund")
	if combined.LibraryID != "r.az" || combined.Songs != 1 || combined.Albums != 0 {
		t.Errorf("combined artist = %+v, want its upload only", combined)
	}
	_, albums, songs, err := e.lib.Artist(ctx, "r.az")
	if err != nil || len(albums) != 0 || !slices.Equal(songNames(songs), []string{"Demo"}) {
		t.Errorf("Artist(r.az) = %v, %v, %v, want the upload only", albumNames(albums), songNames(songs), err)
	}
}
