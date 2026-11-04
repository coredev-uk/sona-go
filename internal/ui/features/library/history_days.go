package library

import (
	"math"
	"time"

	mygoi18n "github.com/egoist/mygo/i18n"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
)

// historyGroup is one day of the History page: each song once, at its latest
// play that day (v0.2.0's HistoryGroup).
type historyGroup struct {
	// day is local midnight (Unix ms); 0 for recent plays on other devices,
	// which Apple gives no time.
	day   int64
	songs []services.Song
	// plays is every play that day, repeats included.
	plays int
	// elsewhere is the songs Apple reports from the account's other devices.
	elsewhere int
}

// historyGroups is Go's days as the page lists them: repeats of a song
// within a day fold into its latest play.
func historyGroups(h services.ListeningHistory) []historyGroup {
	out := make([]historyGroup, len(h.Days))
	for i, d := range h.Days {
		seen := map[string]bool{}
		g := historyGroup{day: d.Day, plays: len(d.Entries)}
		for _, e := range d.Entries {
			if seen[e.Song.ID] {
				continue
			}
			seen[e.Song.ID] = true
			g.songs = append(g.songs, e.Song)
			if e.PlayedAt == 0 {
				g.elsewhere++
			}
		}
		out[i] = g
	}
	return out
}

// dayLabel is a day heading: "Today", "Yesterday", the weekday within the
// last week, then the date ("Monday 26 October" in the locale's order), with
// the year once it is not this year's. day is local midnight in Unix
// milliseconds, now the time the data arrived, both read in loc.
func dayLabel(day int64, now time.Time, loc *time.Location) string {
	if day == 0 {
		return i18n.T("Recently, on your other devices")
	}
	d := time.UnixMilli(day).In(loc)
	n := now.In(loc)
	today := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, loc)
	// Whole days between the two midnights (DST days are 23 or 25 hours long).
	ago := int(math.Round(float64(today.Sub(d)) / float64(24*time.Hour)))
	switch {
	case ago == 0:
		return i18n.T("Today")
	case ago == 1:
		return i18n.T("Yesterday")
	case ago > 1 && ago < 7:
		return mygoi18n.FormatDate(formatLocale(), d, mygoi18n.DateOptions{Skeleton: "EEEE"})
	}
	skeleton := "MMMMEEEEd"
	if d.Year() != today.Year() {
		skeleton = "yMMMMEEEEd"
	}
	return mygoi18n.FormatDate(formatLocale(), d, mygoi18n.DateOptions{Skeleton: skeleton})
}

// formatLocale is the locale dates are written for: the regional one, which
// is not always the language shown (an English UI on an en-GB system writes
// day before month).
func formatLocale() string {
	if l := i18n.Locales(); len(l) > 0 {
		return l[0]
	}
	return i18n.Locale()
}
