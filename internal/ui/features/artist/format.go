package artist

import (
	"regexp"
	"strings"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
)

// What the page borrows of v0.2.0's shared/lib/format.ts and release.ts.

var yearRe = regexp.MustCompile(`^(\d{4})`)

// yearOf is the year of an ISO date ("2008-06-02" or "2008"), or "".
func yearOf(date string) string { return yearRe.FindString(date) }

var fullDate = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// longDate is a release date in words, "10 Oct 2007", for a full ISO date,
// the year for a partial one, "" for none.
func longDate(date string) string {
	if !fullDate.MatchString(date) {
		return yearOf(date)
	}
	// Midday local time, so no time zone moves it to another day.
	at, err := time.ParseInLocation("2006-01-02", date, time.Local)
	if err != nil {
		return yearOf(date)
	}
	at = at.Add(12 * time.Hour)
	return i18n.FormatDate(at, i18n.DateMedium)
}

// catalogueLine is the catalogue line (docs/design.md): "Single · 1 song".
// Empty parts are dropped; kind is already translated.
func catalogueLine(kind, year string, songs int) string {
	count := ""
	if songs > 0 {
		count = i18n.T("{songs, plural, one {# song} other {# songs}}", "songs", songs)
	}
	var parts []string
	for _, p := range []string{kind, year, count} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " · ")
}

// upcoming is v0.2.0's isUpcoming: an album is still to come when it is
// prerelease and has no full release date, or that date is ahead.
func upcoming(a services.Album, now time.Time) bool {
	return a.Prerelease && (a.ReleaseAt == 0 || now.UnixMilli() < a.ReleaseAt)
}

// releaseBadge is the badge of an upcoming album ("Out 19 Nov", with the
// year when it is not this one's); "" for one that is out or has no full
// date.
func releaseBadge(a services.Album, now time.Time) string {
	if !upcoming(a, now) || a.ReleaseAt == 0 {
		return ""
	}
	return i18n.T("Out {releaseDate}", "releaseDate", i18n.ShortReleaseDate(time.UnixMilli(a.ReleaseAt), now))
}
