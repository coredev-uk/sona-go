package album

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
)

// This file is the formatting the album page takes from v0.2.0's
// shared/lib/format.ts and release.ts.

var yearRE = regexp.MustCompile(`^(\d{4})`)

// yearOf is the year of an ISO date ("2008-06-02" or "2008"), or "".
func yearOf(date string) string {
	if m := yearRE.FindString(date); m != "" {
		return m
	}
	return ""
}

var fullDateRE = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})$`)

// longDate is a release date in words ("10 October 2007") for a full ISO
// date, the year for a partial one, "" for none.
func longDate(date string) string {
	m := fullDateRE.FindStringSubmatch(date)
	if m == nil {
		return yearOf(date)
	}
	y, _ := strconv.Atoi(m[1])
	mo, _ := strconv.Atoi(m[2])
	d, _ := strconv.Atoi(m[3])
	// Midday local time, so no time zone moves it to another day.
	return i18n.FormatDate(time.Date(y, time.Month(mo), d, 12, 0, 0, 0, time.Local), i18n.DateLong)
}

// fullReleaseDate is a release day in full: "Thursday 19 November 2026".
func fullReleaseDate(releaseAt int64) string {
	return i18n.FormatDate(time.UnixMilli(releaseAt), i18n.DateFull)
}

// isUpcoming is v0.2.0's isUpcoming: an album is still to come when it is
// prerelease and has no full release date, or that date is ahead.
func isUpcoming(a services.Album, now time.Time) bool {
	return a.Prerelease && (a.ReleaseAt == 0 || now.UnixMilli() < a.ReleaseAt)
}

// releaseBadge is the badge on an upcoming album's cover ("Out 19 Nov", with
// the year when it is not this one's); "" for an album that is out or has no
// full date.
func releaseBadge(a services.Album, now time.Time) string {
	if !isUpcoming(a, now) || a.ReleaseAt == 0 {
		return ""
	}
	return i18n.T("Out {releaseDate}", "releaseDate", i18n.ShortReleaseDate(time.UnixMilli(a.ReleaseAt), now))
}

// catalogueLine is the catalogue line (docs/design.md): `Album · 2008 · 10
// songs, 42 min`. Empty parts are dropped; the song count and running time
// share a slot. kind is already translated.
func catalogueLine(kind, year string, songs int, runningMs int64) string {
	count := ""
	if songs > 0 {
		count = i18n.T("{0, plural, one {# song} other {# songs}}", "0", songs)
	}
	t := ""
	if runningMs > 0 {
		t = i18n.FormatRunningTime(time.Duration(runningMs) * time.Millisecond)
	}
	tail := count
	switch {
	case count != "" && t != "":
		tail = i18n.T("{count}, {time}", "count", count, "time", t)
	case count == "":
		tail = t
	}
	var parts []string
	for _, p := range []string{kind, year, tail} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " · ")
}
