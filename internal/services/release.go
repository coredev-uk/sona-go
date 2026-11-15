package services

import (
	"errors"
	"strings"
	"time"
)

// Pre-release albums. Apple lists an album before it comes out
// (isPrerelease, a releaseDate still ahead) with the songs already out
// (singles) playable and the rest without play parameters. Until the
// release Album.Prerelease is set, songs not out yet are not playable and
// PlaybackService refuses them (ErrNotReleased), and the album page counts
// down to Album.ReleaseAt. A song follows itself, never its album: its own
// playParams say it is out (some singles carry no releaseDate of their own,
// Flower In A Hurricane on the GTA VI album among them), a releaseDate still
// ahead says it is not, and only a song with neither falls back to its
// album's date (library.Song.Unreleased: the library decides, in SQL, for the
// song lists and for what plays). Apple releases at midnight in the storefront's time zone;
// Sona takes the computer's, which is the storefront's for nearly everyone.

// placeholderCredit is the artist Apple gives a song it has not published
// ("unknown"), a stand-in and not an artist.
const placeholderCredit = "unknown"

// credit is the artist to show for a song: the placeholder Apple gives a
// song with no playParams is no artist at all (the album's artist heads the
// page, and a song out gets its own artist at the next sync). A song that
// plays keeps whatever it is credited to, an artist who is called Unknown
// among them.
func credit(artist string, noPlayParams bool) string {
	if noPlayParams && strings.EqualFold(strings.TrimSpace(artist), placeholderCredit) {
		return ""
	}
	return artist
}

// ErrNotReleased is a play of a song that isn't out yet (code
// "not_released").
var ErrNotReleased = errors.New("not released yet")

// releaseAt returns the first instant of the local day date ("YYYY-MM-DD")
// as Unix ms, 0 when date is not a full date (Apple gives some only a year).
// That is local midnight, or the first moment after it where the clocks skip
// midnight (the library's SQL compares dates: a day is on from the instant
// the local date reads so).
func releaseAt(date string) int64 {
	t, err := time.ParseInLocation(time.DateOnly, date, time.Local)
	if err != nil {
		return 0
	}
	// A midnight that does not exist is read as 23:00 the day before.
	for range 6 {
		if t.Format(time.DateOnly) == date {
			break
		}
		t = t.Add(30 * time.Minute)
	}
	return t.UnixMilli()
}

// upcoming reports whether an item released on date is not out at now.
// Without a full date it trusts Apple's isPrerelease flag.
func upcoming(date string, flagged bool, now time.Time) bool {
	if at := releaseAt(date); at != 0 {
		return now.UnixMilli() < at
	}
	return flagged
}

// libraryRelease is the release date of a library album and whether it is
// not out at now. A pre-added album has no releaseDate of its own, so its
// catalog album's counts when the library has none or the catalog's is
// still ahead; without a full date Apple's isPrerelease decides.
func libraryRelease(date, catalogDate string, flagged bool, now time.Time) (string, bool) {
	if catalogDate != "" && (date == "" || upcoming(catalogDate, false, now)) {
		date = catalogDate
	}
	return date, upcoming(date, flagged, now)
}
