package uitest

import (
	"database/sql"
	"testing"

	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/db/dbq"
)

// The pre-added album PreAdded puts in the library.
const (
	PreAddedID     = "l.soon"
	PreAddedName   = "Soon"
	PreAddedArtist = "Mia Vale"
)

// PreAddedSongs are the songs of the album PreAdded stores, in track order:
// the first is out (dated, with playParams), the next two are not (no
// playParams, no date of their own), the last is a single's, out with no date.
var PreAddedSongs = []string{"Out already", "Later", "Last", "Single"}

// PreAdded stores in d a pre-added library album as a sync leaves it: the
// catalog's release date (a YYYY-MM-DD) and flag, no date of its own, one song
// out and two with no playParams (docs/architecture.md, Sync), the way
// the real LibraryService reads it (PreAddedID, PreAddedSongs).
func PreAdded(t testing.TB, d *db.DB, release string) {
	t.Helper()
	ctx := t.Context()
	id, err := d.Q.UpsertLibraryAlbum(ctx, dbq.UpsertLibraryAlbumParams{
		LibraryID: db.NullString(PreAddedID), CatalogID: db.NullString("7"), Name: PreAddedName, ArtistName: PreAddedArtist,
		TrackCount: int64(len(PreAddedSongs)), IsPrerelease: 1, CatalogReleaseDate: release, UpdatedAt: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Q.MarkLibraryItem(ctx, dbq.MarkLibraryItemParams{Kind: "album", Ref: id, Generation: 1, DateAdded: sql.NullInt64{Int64: 1 << 40, Valid: true}}); err != nil {
		t.Fatal(err)
	}
	for i, name := range PreAddedSongs {
		var date string
		var noPlayParams int64
		switch i {
		case 0:
			date = "2020-01-01"
		case 1, 2:
			noPlayParams = 1
		}
		ref, err := d.Q.UpsertLibraryTrack(ctx, dbq.UpsertLibraryTrackParams{
			LibraryID: db.NullString("i.soon" + string(rune('0'+i))), Name: name, ArtistName: PreAddedArtist, AlbumName: PreAddedName,
			AlbumLibraryID: db.NullString(PreAddedID), TrackNumber: int64(i + 1), DiscNumber: 1,
			ReleaseDate: date, NoPlayParams: noPlayParams, UpdatedAt: 1,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := d.Q.MarkLibraryItem(ctx, dbq.MarkLibraryItemParams{Kind: "song", Ref: ref, Generation: 1, AddedRank: sql.NullInt64{Int64: int64(100 + i), Valid: true}}); err != nil {
			t.Fatal(err)
		}
	}
}
