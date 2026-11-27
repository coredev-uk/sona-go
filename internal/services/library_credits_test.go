package services

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/db/dbq"
	"github.com/coredev-uk/sona-go/internal/library"
	"github.com/coredev-uk/sona-go/internal/namefold"
)

// creditsStore is a library with the combined artist "Ada & Bo" (r.duo) and
// its song "Together", credited to catalogue artists 1 and 2: neither is a
// library artist, so each is credit-only.
func creditsStore(t *testing.T) *db.DB {
	t.Helper()
	ctx := t.Context()
	d, err := db.Open(ctx, filepath.Join(t.TempDir(), "sona.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	ref, err := d.Q.UpsertLibraryArtist(ctx, dbq.UpsertLibraryArtistParams{LibraryID: db.NullString("r.duo"), CatalogID: db.NullString("9"), Name: "Ada & Bo", UpdatedAt: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Q.MarkLibraryItem(ctx, dbq.MarkLibraryItemParams{Kind: "artist", Ref: ref, Generation: 1}); err != nil {
		t.Fatal(err)
	}
	track, err := d.Q.UpsertLibraryTrack(ctx, dbq.UpsertLibraryTrackParams{
		LibraryID: db.NullString("i.together"), CatalogID: db.NullString("70"), Name: "Together", ArtistName: "Ada & Bo",
		ArtistLibraryID: db.NullString("r.duo"), TrackNumber: 1, DiscNumber: 1, ReleaseDate: "2020-01-01", UpdatedAt: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Q.MarkLibraryItem(ctx, dbq.MarkLibraryItemParams{Kind: "song", Ref: track, Generation: 1, AddedRank: sql.NullInt64{Int64: 1, Valid: true}}); err != nil {
		t.Fatal(err)
	}
	for pos, c := range [][2]string{{"1", "Ada"}, {"2", "Bo"}} {
		if err := d.Q.InsertTrackArtist(ctx, dbq.InsertTrackArtistParams{TrackID: track, ArtistCatalogID: c[0], Name: c[1], NameKey: namefold.Fold(c[1]), Position: int64(pos)}); err != nil {
			t.Fatal(err)
		}
	}
	return d
}

// An artist the library holds only through credits is a catalogue item: its
// ID is a catalogue ID, so no library action (delete, pin, library state)
// may take it for a library one. The combined library artist stays one, with
// nothing left to page, pin or play: its song is credited to Ada and Bo.
func TestCreditOnlyArtistsAreCatalogueItems(t *testing.T) {
	ctx := t.Context()
	d := creditsStore(t)
	e := &Env{Deps: Deps{DB: d, Library: library.New(d, nil), Logger: discard()}}
	ls := &LibraryService{o: e}
	index, err := ls.ArtistIndex(ctx)
	if err != nil || len(index) != 2 {
		t.Fatalf("index = %+v, %v, want Ada and Bo (the combined artist has no album left to list)", index, err)
	}
	for _, id := range []string{"1", "2"} {
		a, err := ls.ArtistAlbums(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if a.Artist.Source != SourceCatalog || a.Artist.LibraryID != "" || a.Artist.ID != id || a.Artist.CatalogID != id {
			t.Errorf("credit-only artist %s = %+v, want a catalogue artist", id, a.Artist)
		}
		cat, lib := refIDs(ItemRef{Kind: "artists", ID: a.Artist.ID, Source: a.Artist.Source})
		if cat != id || lib != "" {
			t.Errorf("refIDs = %q, %q: the catalogue ID must not be taken for a library ID", cat, lib)
		}
		det, err := ls.Artist(ctx, id)
		if err != nil || len(det.Songs) != 1 || det.Songs[0].Name != "Together" {
			t.Errorf("Artist(%s) = %+v, %v", id, det.Songs, err)
		}
	}
	duo, err := ls.ArtistAlbums(ctx, "r.duo")
	if err != nil || duo.Artist.Source != SourceLibrary || duo.Artist.LibraryID != "r.duo" {
		t.Errorf("combined artist = %+v, %v, want a library artist", duo.Artist, err)
	}

	// The combined artist holds nothing: its song is credited to Ada and Bo.
	det, err := ls.Artist(ctx, "r.duo")
	if err != nil || len(det.Songs) != 0 {
		t.Errorf("Artist(r.duo) songs = %+v, %v, want none", det.Songs, err)
	}
	ps := &PlaylistsService{e: e}
	if songs, err := ps.PinTracks(ctx, "r.duo"); err != nil || len(songs) != 0 {
		t.Errorf("PinTracks(r.duo) = %+v, %v, want none", songs, err)
	}
}
