package library

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"testing"

	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/db/dbq"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/fixture"
)

// collabAlbums are two artists with an album each; the library also holds one
// song, "Together", that Apple's library credits to "Ada & Bo" and the
// catalogue to Ada and Bo (seedCollab).
func collabAlbums() []fixture.Album {
	return []fixture.Album{
		{Name: "Ada Alone", Artist: "Ada"},
		{Name: "Bo Alone", Artist: "Bo"},
	}
}

// seedCollab adds the song and the library artist "Ada & Bo" of it, with the
// credits the sync stores (Syncer.syncCredits): songs only, no album.
func seedCollab(ctx context.Context, d *db.DB) error {
	const now = 1_700_000_000_000
	artists := fixture.Artists(collabAlbums())
	return d.Tx(ctx, func(q *dbq.Queries) error {
		ref, err := q.UpsertLibraryArtist(ctx, dbq.UpsertLibraryArtistParams{
			LibraryID: db.NullString("r.duo"), CatalogID: db.NullString("700"), Name: "Ada & Bo", UpdatedAt: now,
		})
		if err != nil {
			return err
		}
		if err := q.MarkLibraryItem(ctx, dbq.MarkLibraryItemParams{Kind: "artist", Ref: ref, Generation: 1}); err != nil {
			return err
		}
		track, err := q.UpsertLibraryTrack(ctx, dbq.UpsertLibraryTrackParams{
			LibraryID: db.NullString("i.together"), CatalogID: db.NullString("7001"), Name: "Together", ArtistName: "Ada & Bo",
			ArtistLibraryID: db.NullString("r.duo"), DurationMs: 200_000, TrackNumber: 1, DiscNumber: 1,
			ReleaseDate: "2020-01-01", Genre: "Pop", UpdatedAt: now,
		})
		if err != nil {
			return err
		}
		if err := q.MarkLibraryItem(ctx, dbq.MarkLibraryItemParams{Kind: "song", Ref: track, AddedRank: sql.NullInt64{Int64: 99, Valid: true}, Generation: 1}); err != nil {
			return err
		}
		for pos, name := range []string{"Ada", "Bo", "Cy"} {
			// Cy is no library artist: the library knows them by this credit only.
			err := q.InsertTrackArtist(ctx, dbq.InsertTrackArtistParams{
				TrackID: track, ArtistCatalogID: catalogOf(artists, name), Name: name, Position: int64(pos),
			})
			if err != nil {
				return err
			}
		}
		return q.MarkTrackCredited(ctx, dbq.MarkTrackCreditedParams{CreditsChecked: now, ID: track})
	})
}

// catalogOf is the catalogue ID the fixture gives artist name.
func catalogOf(artists []string, name string) string {
	if n := slices.Index(artists, name); n >= 0 {
		return fmt.Sprint(500 + n)
	}
	return "900"
}

// A song credited to several artists counts and lists under each of them,
// and the library artist that carried only the combined name is gone.
func TestArtistsListACollaborationUnderEachCreditedArtist(t *testing.T) {
	artists := fixture.Artists(collabAlbums())
	open := func(key string) *pageRig {
		return newPageRig(t, artistsAt(key), 1132, 900, nil, uitest.Albums(collabAlbums()...), uitest.Seeded(seedCollab))
	}
	// Ada's and Bo's rows count it (2 songs of their album and Together).
	r := open(fixture.ArtistLibraryID(slices.Index(artists, "Ada")))
	r.tt.Until("the list", r.tt.Shows("3 artists", "Ada, 1 album, 3 songs", "Bo, 1 album, 3 songs", "Cy, 1 song"))
	if r.tt.HasText("Ada & Bo") {
		t.Error("the combined artist is still listed though only its song carried it")
	}
	// Cy is listed from the credit alone, under its catalogue ID, and the song is theirs.
	r = open("900")
	r.tt.Until("Cy's song", r.tt.Shows("Songs in Your Library", "Together", "Artist · 1 song"))
}
