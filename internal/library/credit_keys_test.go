package library

import (
	"context"
	"slices"
	"testing"
)

// Apple's library artist for "A & B" carries A's catalogue ID, as live data
// shows: "Alesso & Zara Larsson" has Alesso's 432464201, "Alan Walker & Ava
// Max" Alan Walker's 1062085272. withCatalogID gives a fake library artist
// that catalogue ID.
func withCatalogID(a map[string]any, id string) map[string]any {
	cat := a["relationships"].(map[string]any)["catalog"].(map[string]any)["data"].([]any)[0].(map[string]any)
	cat["id"] = id
	return a
}

const (
	alessoID = "432464201"
	zaraID   = "570372593"
	alessoIm = "https://art/alesso/{w}x{h}.jpg"
)

// alessoLibrary is the live case: the library artist "Alesso & Zara Larsson"
// (Alesso's catalogue ID), the library artist "Zara Larsson" (inserted after
// it), and no library artist for Alesso. The song "Words" and its album are
// by "Alesso & Zara Larsson" in Apple's library, and credited to Alesso and
// Zara Larsson in the catalogue.
func alessoLibrary(e *env) {
	e.sync.Storefront = func() string { return "us" }
	f := e.fake
	f.artists = []map[string]any{
		withCatalogID(artist("r.az", "Alesso & Zara Larsson"), alessoID),
		withCatalogID(artist("r.zl", "Zara Larsson"), zaraID),
	}
	f.albums = []map[string]any{album("l.w", "Words - Single", "Alesso & Zara Larsson", "r.az", "2024-01-01T00:00:00Z")}
	f.songs = []map[string]any{song("i.w", "Words", "Alesso & Zara Larsson", "Words - Single", "l.w", "r.az")}
	credited := [][2]string{{alessoID, "Alesso"}, {zaraID, "Zara Larsson"}}
	f.credits = map[string][][2]string{"cw": credited}
	f.albumCredits = map[string][][2]string{"cl.w": credited}
	f.creditArtwork = map[string]string{alessoID: alessoIm}
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
	alessoLibrary(e)
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	index, err := e.lib.ArtistIndex(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := indexNames(index); !slices.Equal(got, []string{"Alesso", "Zara Larsson"}) {
		t.Fatalf("artists = %v, want Alesso and Zara Larsson alone", got)
	}
	alesso, zara := index[0], index[1]
	if alesso.LibraryID != "" || alesso.CatalogID != alessoID || alesso.Albums != 1 || alesso.Songs != 1 {
		t.Errorf("Alesso = %+v, want credit-only, 1 album, 1 song", alesso)
	}
	if zara.LibraryID != "r.zl" || zara.Albums != 1 || zara.Songs != 1 {
		t.Errorf("Zara Larsson = %+v, want her library artist, 1 album, 1 song", zara)
	}
	for _, key := range []string{alessoID, "r.zl"} {
		_, albums, songs, err := e.lib.Artist(ctx, key)
		if err != nil || !slices.Equal(albumNames(albums), []string{"Words - Single"}) || !slices.Equal(songNames(songs), []string{"Words"}) {
			t.Errorf("Artist(%s) = %v, %v, %v", key, albumNames(albums), songNames(songs), err)
		}
	}
	_, albums, songs, err := e.lib.Artist(ctx, "r.az")
	if err != nil || len(albums) != 0 || len(songs) != 0 {
		t.Errorf("Artist(r.az) = %v, %v, %v, want nothing", albumNames(albums), songNames(songs), err)
	}
	// Search lists the same artists.
	for term, want := range map[string]bool{"Alesso": true, "Zara": true} {
		r, err := e.lib.SearchAll(ctx, term, 10)
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range r.Artists {
			if a.Name == "Alesso & Zara Larsson" {
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
	alessoLibrary(e)
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	index, err := e.lib.ArtistIndex(ctx)
	if err != nil {
		t.Fatal(err)
	}
	alesso, ok := artistNamed(t, index, "Alesso")
	if !ok || alesso.ArtworkTemplate != alessoIm {
		t.Errorf("Alesso in the list = %+v, want the artwork %s", alesso, alessoIm)
	}
	a, _, err := e.lib.ArtistAlbums(ctx, alessoID)
	if err != nil || a.ArtworkTemplate != alessoIm {
		t.Errorf("Alesso's page = %+v, %v, want the artwork %s", a, err, alessoIm)
	}
	if n := catalogArtistRows(t, e, alessoID); n != 1 {
		t.Errorf("%d catalogue rows for Alesso, want 1", n)
	}
	// An artist the answer gives no artwork for is not stored (nothing to add).
	if n := catalogArtistRows(t, e, zaraID); n != 0 {
		t.Errorf("%d catalogue rows for Zara Larsson, whose answer had no artwork", n)
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
	const alan, ava = "1062085272", "1130143357"
	f.artists = []map[string]any{
		withCatalogID(artist("r.aa", "Alan Walker & Ava Max"), alan),
		withCatalogID(artist("r.ac", "Alan Walker & Conor Maynard"), alan),
		withCatalogID(artist("r.aw", "Alan Walker"), alan),
	}
	f.albums = []map[string]any{album("l.al", "Alone", "Alan Walker", "r.aw", "2024-01-01T00:00:00Z")}
	f.songs = []map[string]any{
		song("i.a1", "Alone Pt. II", "Alan Walker & Ava Max", "Alone", "l.al", "r.aa"),
		song("i.a2", "Alone Pt. III", "Alan Walker & Conor Maynard", "Alone", "l.al", "r.ac"),
	}
	f.credits = map[string][][2]string{
		"ca1": {{alan, "Alan Walker"}, {ava, "Ava Max"}},
		"ca2": {{alan, "alan walker"}, {"900", "Conor Maynard"}}, // folded: case
	}
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	index, err := e.lib.ArtistIndex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := indexNames(index); !slices.Equal(got, []string{"Alan Walker", "Ava Max", "Conor Maynard"}) {
		t.Fatalf("artists = %v, want Alan Walker, Ava Max and Conor Maynard", got)
	}
	if aw := index[0]; aw.LibraryID != "r.aw" || aw.Songs != 2 || aw.Albums != 1 {
		t.Errorf("Alan Walker = %+v, want his library artist with both songs and his album", aw)
	}
	if am := index[1]; am.LibraryID != "" || am.CatalogID != ava || am.Songs != 1 {
		t.Errorf("Ava Max = %+v, want a credit-only artist with 1 song", ava)
	}
}

// A combined library artist keeps what Apple's catalogue does not credit: an
// upload under "A & B" (no catalogue copy, so no credits) stays listed under
// it, with that item only; the credited song and album go to the artists.
func TestCombinedArtistKeepsWhatIsUncredited(t *testing.T) {
	e := newEnv(t)
	alessoLibrary(e)
	up := uploadSong("i.up", "Demo", "Alesso & Zara Larsson", "r.az")
	e.fake.songs = append(e.fake.songs, up)
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	index, err := e.lib.ArtistIndex(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := indexNames(index); !slices.Equal(got, []string{"Alesso", "Alesso & Zara Larsson", "Zara Larsson"}) {
		t.Fatalf("artists = %v", got)
	}
	combined, _ := artistNamed(t, index, "Alesso & Zara Larsson")
	if combined.LibraryID != "r.az" || combined.Songs != 1 || combined.Albums != 0 {
		t.Errorf("combined artist = %+v, want its upload only", combined)
	}
	_, albums, songs, err := e.lib.Artist(ctx, "r.az")
	if err != nil || len(albums) != 0 || !slices.Equal(songNames(songs), []string{"Demo"}) {
		t.Errorf("Artist(r.az) = %v, %v, %v, want the upload only", albumNames(albums), songNames(songs), err)
	}
}
