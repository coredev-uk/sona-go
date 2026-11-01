package services

import (
	"errors"
	"time"
)

// Pre-release albums. Apple lists an album before it comes out
// (isPrerelease, a releaseDate still ahead) with the songs already out
// (singles) playable and the rest without play parameters. Until the
// release Album.Prerelease is set, songs not out yet are not playable and
// PlaybackService refuses them (ErrNotReleased), and the album page counts
// down to Album.ReleaseAt. Apple releases at midnight in the storefront's
// time zone; Sona takes the computer's, which is the storefront's for
// nearly everyone.

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

// markUnreleased makes the tracks of the pre-release album not out yet
// unplayable: those whose own release date is still ahead or, without
// one, every track (Apple dates only the songs already out).
func markUnreleased(tracks []Song, now time.Time) {
	for i := range tracks {
		if tracks[i].ReleaseDate == "" || upcoming(tracks[i].ReleaseDate, false, now) {
			tracks[i].Playable = false
		}
	}
}
