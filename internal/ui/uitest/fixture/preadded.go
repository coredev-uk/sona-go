package fixture

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/db/dbq"
	"github.com/coredev-uk/sona-go/internal/library"
)

// PreAdded is an album added to the library before its release, stored as
// the sync stores what Apple lists (docs/apple-music-reference.md, "Pre-added
// albums"): the album has no release date of its own, only its catalog
// album's (Out) and the isPrerelease flag; a song already out has its name,
// artist, playParams and date; the others are placeholders, "Track N" by
// "unknown", with no playParams, no date and no duration.
type PreAdded struct {
	Name, Artist string
	// Out is the day the album is out, "YYYY-MM-DD".
	Out string
	// Songs are the names of the songs out, "" for a song that is not out
	// yet; its place is its number on the album.
	Songs []string
}

// The identifiers PreAdd gives the album and its songs: song n (from 1) is
// PreAddedSongID(n), and its catalog ID PreAddedSongCatalogID(n) once out.
const (
	PreAddedAlbumID  = "l.pre"
	PreAddedArtistID = "r.pre"
)

// PreAddedSongID is the library ID of song n (from 1) of the pre-added album.
func PreAddedSongID(n int) string { return fmt.Sprintf("i.pre%d", n) }

// PreAddedSongCatalogID is the catalog ID of song n once it is out.
func PreAddedSongCatalogID(n int) string { return fmt.Sprint(9000 + n) }

const preAddedSongOut = "2025-12-01"

// PreAdd stores p in a seeded library, the newest album and song in it, and
// rebuilds the search index.
func PreAdd(ctx context.Context, d *db.DB, p PreAdded) error {
	const now = 1_700_000_000_000
	err := d.Tx(ctx, func(q *dbq.Queries) error {
		aref, err := q.UpsertLibraryArtist(ctx, dbq.UpsertLibraryArtistParams{
			LibraryID: db.NullString(PreAddedArtistID), CatalogID: db.NullString("599"), Name: p.Artist, UpdatedAt: now,
		})
		if err != nil {
			return err
		}
		if err := q.MarkLibraryItem(ctx, dbq.MarkLibraryItemParams{Kind: "artist", Ref: aref, Generation: 1}); err != nil {
			return err
		}
		ref, err := q.UpsertLibraryAlbum(ctx, dbq.UpsertLibraryAlbumParams{
			LibraryID: db.NullString(PreAddedAlbumID), CatalogID: db.NullString("900"),
			Name: p.Name, ArtistName: p.Artist, ArtistLibraryID: db.NullString(PreAddedArtistID),
			Genre: "Pop", TrackCount: int64(len(p.Songs)), IsPrerelease: 1, CatalogReleaseDate: p.Out, UpdatedAt: now,
		})
		if err != nil {
			return err
		}
		if err := q.MarkLibraryItem(ctx, dbq.MarkLibraryItemParams{
			Kind: "album", Ref: ref, DateAdded: sql.NullInt64{Int64: 1000 * day, Valid: true}, Generation: 1,
		}); err != nil {
			return err
		}
		for i, name := range p.Songs {
			n := i + 1
			t := dbq.UpsertLibraryTrackParams{
				LibraryID: db.NullString(PreAddedSongID(n)), Name: fmt.Sprintf("Track %d", n), ArtistName: "unknown",
				AlbumName: p.Name, AlbumLibraryID: db.NullString(PreAddedAlbumID), AlbumCatalogID: db.NullString("900"),
				TrackNumber: int64(n), DiscNumber: 1, NoPlayParams: 1, UpdatedAt: now,
			}
			if name != "" {
				t.CatalogID, t.Name, t.ArtistName = db.NullString(PreAddedSongCatalogID(n)), name, p.Artist
				t.ArtistLibraryID, t.DurationMs, t.ReleaseDate, t.NoPlayParams = db.NullString(PreAddedArtistID), 200_000, preAddedSongOut, 0
			}
			tref, err := q.UpsertLibraryTrack(ctx, t)
			if err != nil {
				return err
			}
			if err := q.MarkLibraryItem(ctx, dbq.MarkLibraryItemParams{
				Kind: "song", Ref: tref, AddedRank: sql.NullInt64{Int64: int64(-1000 + n), Valid: true}, Generation: 1,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("fixture: pre-add: %w", err)
	}
	return library.RebuildSearch(ctx, d)
}

// PublishPreAdded stores what the sync after the release stores: Apple has
// published the album (its own date, no flag) and gives the songs named in
// names (by number from 1, "" leaving a song as it is) their real metadata.
func PublishPreAdded(ctx context.Context, d *db.DB, artist, out string, names map[int]string) error {
	for n, name := range names {
		if _, err := d.W.ExecContext(ctx, `UPDATE tracks SET name = ?, artist_name = ?, artist_library_id = ?, catalog_id = ?,
			duration_ms = 200000, release_date = ?, no_play_params = 0 WHERE library_id = ?`,
			name, artist, PreAddedArtistID, PreAddedSongCatalogID(n), out, PreAddedSongID(n)); err != nil {
			return fmt.Errorf("fixture: publish song %d: %w", n, err)
		}
	}
	if _, err := d.W.ExecContext(ctx, `UPDATE albums SET release_date = ?, catalog_release_date = '', is_prerelease = 0 WHERE library_id = ?`,
		out, PreAddedAlbumID); err != nil {
		return fmt.Errorf("fixture: publish album: %w", err)
	}
	return library.RebuildSearch(ctx, d)
}
