package library

import "time"

// Songs not out yet. A pre-added album's songs that are not out are stored
// like any song (Apple lists them in the library with placeholder metadata,
// "Track 3" by "unknown"), but they are not songs yet: the song lists, their
// counts, the letter rail, an artist's songs and the search leave them out
// until they are released.
//
// This is the one rule of whether a library song is out, for everything: a
// song goes by its own release date when it has one, else it is out when
// Apple gave it playParams, else it goes by its library album's release state
// (the catalog album's date when the library album has none or the catalog's
// is still ahead, and isPrerelease when no date is a full one). The lists need
// it in SQL, to page, count and match, and every song read carries its answer
// (Song.Unreleased, the last column of trackColumns): the Playable flag of the
// album page, and what PlaybackService refuses, ask the library, not a second
// copy. The album-level countdown (services.libraryRelease) is the same rule
// for the album's own date; a test holds a song to its album's answer.
//
// Release dates are local midnight of a YYYY-MM-DD date. A date is ahead
// exactly while it is later than today's date in the computer's zone, so SQL
// takes today as a "YYYY-MM-DD" string (Library.today, parameter ?1) rather
// than a time. A date that is not a full one (a year alone) is never ahead
// by itself: the album's isPrerelease flag decides.

// fullDate is the GLOB of a full YYYY-MM-DD date.
const fullDate = `'[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'`

// aheadSQL is the SQL of services.upcoming (for an album's date): whether date (a column or
// expression) is ahead of today (?1), else flagged.
func aheadSQL(date, flagged string) string {
	return `(CASE WHEN ` + date + ` GLOB ` + fullDate + ` THEN ` + date + ` > ?1 ELSE ` + flagged + ` END)`
}

// albumDateSQL is the date of albums row al that services.libraryRelease
// picks: the catalog album's when the library album has none or the
// catalog's is still ahead.
const albumDateSQL = `(CASE WHEN al.catalog_release_date <> '' AND (al.release_date = '' OR (al.catalog_release_date GLOB ` + fullDate +
	` AND al.catalog_release_date > ?1)) THEN al.catalog_release_date ELSE al.release_date END)`

// songNotOut is the WHERE condition that holds for the tracks t (and
// only those) that are not out yet. It takes today as ?1 and must appear in
// a statement before any plain ? parameter, which then number from ?2.
var songNotOut = `(CASE
	WHEN t.release_date <> '' THEN ` + aheadSQL(`t.release_date`, `0`) + `
	WHEN t.no_play_params = 0 THEN 0
	ELSE coalesce((SELECT ` + aheadSQL(albumDateSQL, `al.is_prerelease`) + ` FROM albums al WHERE al.library_id = t.album_library_id LIMIT 1), 0)
END)`

// songOut holds for the tracks t that are out (songs of the lists).
var songOut = `NOT ` + songNotOut

// SetClock sets the clock that decides which songs are out (time.Now by
// default); the app gives it Deps.Now, so tests can move time.
func (l *Library) SetClock(now func() time.Time) { l.now = now }

// today is the date, in the computer's zone, that songs and albums are
// compared with: ?1 of the statements using songOut.
func (l *Library) today() string {
	now := time.Now
	if l.now != nil {
		now = l.now
	}
	return now().In(time.Local).Format(time.DateOnly)
}
