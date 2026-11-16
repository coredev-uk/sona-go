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
// album's date. Apple releases at midnight in the storefront's time zone;
// Sona takes the computer's, which is the storefront's for nearly everyone.
// Whether a library song is out is library's one rule (library/release.go,
// in SQL, so the lists and the search apply it too); songs not out are not
// listed outside their album, nor credited to Apple's placeholder artist.

// ErrNotReleased is a play of a song that isn't out yet (code
// "not_released").
var ErrNotReleased = errors.New("not released yet")

// releaseAt returns local midnight on date ("YYYY-MM-DD") as Unix ms, 0
// when date is not a full date (Apple gives some only a year).
func releaseAt(date string) int64 {
	t, err := time.ParseInLocation(time.DateOnly, date, time.Local)
	if err != nil {
		return 0
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

// placeholderArtist is the artist Apple credits a song it has not published
// yet to: the literal "unknown".
const placeholderArtist = "unknown"

// shownArtist is the artist to show for a song: its credit, but not Apple's
// stand-in for a song that is not out yet, which names no one ("" then).
func shownArtist(artist string, notOut bool) string {
	if notOut && strings.EqualFold(strings.TrimSpace(artist), placeholderArtist) {
		return ""
	}
	return artist
}
