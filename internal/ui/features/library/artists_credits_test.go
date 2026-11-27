package library

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"testing"

	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/db/dbq"
	"github.com/coredev-uk/sona-go/internal/namefold"
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
				TrackID: track, ArtistCatalogID: catalogOf(artists, name), Name: name, NameKey: namefold.Fold(name), Position: int64(pos),
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

// seedCollabAlbum adds the album "Both Of Us" of "Ada & Bo", with the
// credits the sync stores for it (Syncer.syncCredits): Ada and Bo. Apple's
// library names the combined library artist of seedCollab as its artist.
func seedCollabAlbum(ctx context.Context, d *db.DB) error {
	const now = 1_700_000_000_000
	artists := fixture.Artists(collabAlbums())
	return d.Tx(ctx, func(q *dbq.Queries) error {
		ref, err := q.UpsertLibraryAlbum(ctx, dbq.UpsertLibraryAlbumParams{
			LibraryID: db.NullString("l.duo"), CatalogID: db.NullString("7100"), Name: "Both Of Us", ArtistName: "Ada & Bo",
			ArtistLibraryID: db.NullString("r.duo"), ReleaseDate: "2021-05-01", Genre: "Pop", TrackCount: 1, UpdatedAt: now,
		})
		if err != nil {
			return err
		}
		if err := q.MarkLibraryItem(ctx, dbq.MarkLibraryItemParams{
			Kind: "album", Ref: ref, DateAdded: sql.NullInt64{Int64: now, Valid: true}, Generation: 1,
		}); err != nil {
			return err
		}
		for pos, name := range []string{"Ada", "Bo"} {
			err := q.InsertAlbumArtist(ctx, dbq.InsertAlbumArtistParams{
				AlbumID: ref, ArtistCatalogID: catalogOf(artists, name), Name: name, NameKey: namefold.Fold(name), Position: int64(pos),
			})
			if err != nil {
				return err
			}
		}
		return q.MarkAlbumCredited(ctx, dbq.MarkAlbumCreditedParams{CreditsChecked: now, ID: ref})
	})
}

// An album credited to two artists is on both their pages and counts for
// both, and the combined artist is gone once its song and its album are
// credited elsewhere.
func TestArtistsListACollaborationAlbumUnderEachCreditedArtist(t *testing.T) {
	artists := fixture.Artists(collabAlbums())
	open := func(key string) *pageRig {
		return newPageRig(t, artistsAt(key), 1132, 900, nil, uitest.Albums(collabAlbums()...), uitest.Seeded(seedCollab), uitest.Seeded(seedCollabAlbum))
	}
	// Each has their own album and Both Of Us; Together and their own two songs.
	for _, name := range []string{"Ada", "Bo"} {
		r := open(fixture.ArtistLibraryID(slices.Index(artists, name)))
		r.tt.Until(name+"'s page", r.tt.Shows(
			"3 artists", "Ada, 2 albums, 3 songs", "Bo, 2 albums, 3 songs", "Cy, 1 song",
			"Albums by "+name+" in your library", name+" Alone", "Both Of Us",
		))
		if r.tt.HasText("Ada & Bo") {
			t.Errorf("%s's page: the combined artist is still listed though its song and album are credited elsewhere", name)
		}
	}
	// Cy is credited on the song only: the album is not theirs.
	r := open("900")
	r.tt.Until("Cy's song", r.tt.Shows("Songs in Your Library", "Together", "Artist · 1 song"))
	if r.tt.HasText("Both Of Us") {
		t.Error("Cy's page lists an album Apple does not credit to them")
	}
}

// Apple's library artist for "A & B" carries A's catalogue ID: the library
// artist "Mira Vale & Theo Lund" has Mira Vale's. With no library artist of
// their own, Mira Vale's credits must not go to that combined artist.
const (
	miraID = "100001"
	theoID = "100002"
)

// seedMira adds the combined library artist "Mira Vale & Theo Lund"
// (Mira Vale's catalogue ID), the library artist "Theo Lund" (added after
// it), and the song "Signals" and its album "Signals - Single", which Apple's
// library gives to the combined artist and the catalogue credits to Mira Vale
// and Theo Lund (what Syncer.syncCredits stores).
func seedMira(ctx context.Context, d *db.DB) error {
	const now = 1_700_000_000_000
	return d.Tx(ctx, func(q *dbq.Queries) error {
		for _, a := range [][3]string{
			{"r.az", miraID, "Mira Vale & Theo Lund"},
			{"r.zl", theoID, "Theo Lund"},
		} {
			ref, err := q.UpsertLibraryArtist(ctx, dbq.UpsertLibraryArtistParams{
				LibraryID: db.NullString(a[0]), CatalogID: db.NullString(a[1]), Name: a[2], NameKey: namefold.Fold(a[2]), UpdatedAt: now,
			})
			if err != nil {
				return err
			}
			if err := q.MarkLibraryItem(ctx, dbq.MarkLibraryItemParams{Kind: "artist", Ref: ref, Generation: 1}); err != nil {
				return err
			}
		}
		credited := [][2]string{{miraID, "Mira Vale"}, {theoID, "Theo Lund"}}
		album, err := q.UpsertLibraryAlbum(ctx, dbq.UpsertLibraryAlbumParams{
			LibraryID: db.NullString("l.words"), CatalogID: db.NullString("7200"), Name: "Signals - Single", ArtistName: "Mira Vale & Theo Lund",
			ArtistLibraryID: db.NullString("r.az"), ReleaseDate: "2021-05-01", Genre: "Pop", TrackCount: 1, UpdatedAt: now,
		})
		if err != nil {
			return err
		}
		if err := q.MarkLibraryItem(ctx, dbq.MarkLibraryItemParams{Kind: "album", Ref: album, DateAdded: sql.NullInt64{Int64: now, Valid: true}, Generation: 1}); err != nil {
			return err
		}
		track, err := q.UpsertLibraryTrack(ctx, dbq.UpsertLibraryTrackParams{
			LibraryID: db.NullString("i.words"), CatalogID: db.NullString("7201"), Name: "Signals", ArtistName: "Mira Vale & Theo Lund",
			ArtistLibraryID: db.NullString("r.az"), DurationMs: 200_000, TrackNumber: 1, DiscNumber: 1,
			ReleaseDate: "2021-05-01", Genre: "Pop", UpdatedAt: now,
		})
		if err != nil {
			return err
		}
		if err := q.MarkLibraryItem(ctx, dbq.MarkLibraryItemParams{Kind: "song", Ref: track, AddedRank: sql.NullInt64{Int64: 98, Valid: true}, Generation: 1}); err != nil {
			return err
		}
		for pos, c := range credited {
			if err := q.InsertAlbumArtist(ctx, dbq.InsertAlbumArtistParams{
				AlbumID: album, ArtistCatalogID: c[0], Name: c[1], NameKey: namefold.Fold(c[1]), Position: int64(pos),
			}); err != nil {
				return err
			}
			if err := q.InsertTrackArtist(ctx, dbq.InsertTrackArtistParams{
				TrackID: track, ArtistCatalogID: c[0], Name: c[1], NameKey: namefold.Fold(c[1]), Position: int64(pos),
			}); err != nil {
				return err
			}
		}
		if err := q.MarkAlbumCredited(ctx, dbq.MarkAlbumCreditedParams{CreditsChecked: now, ID: album}); err != nil {
			return err
		}
		return q.MarkTrackCredited(ctx, dbq.MarkTrackCreditedParams{CreditsChecked: now, ID: track})
	})
}

// Mira Vale, who has no library artist, is listed alone with Signals and its
// album; Theo Lund lists them too; "Mira Vale & Theo Lund", whose only song
// and album are credited to them, is gone.
func TestArtistsListAnArtistWhoseCreditsRodeOnACombinedArtist(t *testing.T) {
	open := func(key string) *pageRig {
		return newPageRig(t, artistsAt(key), 1132, 900, nil, uitest.Albums(collabAlbums()...), uitest.Seeded(seedMira))
	}
	r := open(miraID)
	r.tt.Until("Mira Vale's page", r.tt.Shows(
		"4 artists", "Mira Vale, 1 album, 1 song", "Theo Lund, 1 album, 1 song",
		"Artist · 1 album, 1 song", "Albums by Mira Vale in your library", "Signals", "2021 · Single",
	))
	if r.tt.HasText("Mira Vale & Theo Lund") {
		t.Error("the combined artist is still listed though its song and album are credited to Mira Vale and Theo Lund")
	}
	r = open("r.zl")
	r.tt.Until("Theo Lund's page", r.tt.Shows("Artist · 1 album, 1 song", "Albums by Theo Lund in your library", "Signals", "2021 · Single"))
}
