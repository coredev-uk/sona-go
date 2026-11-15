package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/coredev-uk/sona-go/internal/db"
)

// Songs not out yet. A pre-added album's songs are in the library with
// placeholder metadata ("Track 3", an artist "unknown") until they are out.
// Apple gives them no playParams and no releaseDate, so this is the one rule
// for whether a library song is out at a given time, written once, in SQL,
// because the lists, their counts, the search and playback all need the same
// answer and the lists need it in the query:
//
//   - a song with a releaseDate of its own goes by it: out from the start of
//     that day, local time (a year alone is not a day: the song is out);
//   - else a song with playParams is out (some singles of a pre-added album
//     carry no date of their own);
//   - else it goes by its library album, as Apple's own state for it is
//     stored: the album's releaseDate, replaced by its catalogue album's
//     (catalog_release_date) when the library has none or the catalogue's
//     is still ahead; a day not reached is not out, and an album with no full
//     day goes by Apple's isPrerelease flag.
//
// The answer changes with the clock, not with the data, so it is computed as
// a query runs (today is the local day, as a literal: it is made here and is
// only digits) and no column holds it. The next sync stores the real names of
// the songs once Apple has them; until then, and from the moment their day
// comes, this decides.

// SetClock gives the library the clock the release rule reads (default
// time.Now). Set it before the library is used.
func (l *Library) SetClock(now func() time.Time) { l.now = now }

// today is the local day the release rule compares with, "YYYY-MM-DD".
func (l *Library) today() string {
	if l.now == nil {
		return Day(time.Now())
	}
	return Day(l.now())
}

// withAlbum joins the library album a track `t` belongs to as `ur`, the
// release rule's input (albums.library_id is unique: one row at most).
const withAlbum = ` LEFT JOIN albums ur ON ur.library_id = t.album_library_id`

// fullDay matches a "YYYY-MM-DD" date (SQLite's GLOB).
const fullDay = ` GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'`

// unreleased is the SQL expression (1 or 0) of the release rule for track `t`
// joined with withAlbum, as of the local day today. Its first term is a
// necessary condition of the rest (a song is not out only if it has no
// playParams or a date of its own ahead), there so that SQLite finds such
// songs by the indexes of migration 00013 and the library needn't read every
// song to count the ones that are out.
func unreleased(today string) string {
	day := "'" + today + "'"
	// The album's date: its catalogue album's when the library has none or the
	// catalogue's is still ahead.
	date := `(CASE WHEN ur.catalog_release_date <> '' AND (ur.release_date = '' OR (ur.catalog_release_date` + fullDay + ` AND ur.catalog_release_date > ` + day + `))
		THEN ur.catalog_release_date ELSE ur.release_date END)`
	return `((t.no_play_params = 1 OR t.release_date > ` + day + `) AND (CASE
		WHEN t.release_date <> '' THEN (t.release_date` + fullDay + ` AND t.release_date > ` + day + `)
		WHEN t.no_play_params = 0 THEN 0
		WHEN ` + date + fullDay + ` THEN ` + date + ` > ` + day + `
		ELSE coalesce(ur.is_prerelease, 0) END))`
}

// out is the SQL condition that track `t` (joined with withAlbum) is out.
func out(today string) string { return `NOT ` + unreleased(today) }

// Unreleased reports whether library song libraryID is not out yet:
// ErrNotFound when the library has no such song.
func (l *Library) Unreleased(ctx context.Context, libraryID string) (bool, error) {
	return SongUnreleased(ctx, l.db, l.today(), libraryID)
}

// SongUnreleased is Library.Unreleased over database d for the local day today
// ("YYYY-MM-DD"), for a caller that holds the database and its own clock
// (playback).
func SongUnreleased(ctx context.Context, d *db.DB, today, libraryID string) (bool, error) {
	var n int64
	err := d.R.QueryRowContext(ctx, `SELECT `+unreleased(today)+` FROM tracks t`+withAlbum+`
		WHERE t.library_id = ?`, libraryID).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, fmt.Errorf("library: release of %s: %w", libraryID, err)
	}
	return n != 0, nil
}

// Day is the local day at now, "YYYY-MM-DD": what the release rule compares
// with.
func Day(now time.Time) string { return now.In(time.Local).Format(time.DateOnly) }
