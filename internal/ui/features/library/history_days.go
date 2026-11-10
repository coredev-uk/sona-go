package library

import (
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
// milliseconds, now the time the data arrived, both read in loc; the days
// between are calendar days (i18n.CalendarDays, as RelativeDay counts them).
func dayLabel(day int64, now time.Time, loc *time.Location) string {
	if day == 0 {
		return i18n.T("Recently, on your other devices")
	}
	d := time.UnixMilli(day).In(loc)
	switch ago := i18n.CalendarDays(d, now, loc); {
	case ago == 0:
		return i18n.T("Today")
	case ago == 1:
		return i18n.T("Yesterday")
	case ago > 1 && ago < 7:
		return mygoi18n.FormatDate(i18n.Locales()[0], d, mygoi18n.DateOptions{Skeleton: "EEEE"})
	}
	skeleton := "MMMMEEEEd"
	if d.Year() != now.In(loc).Year() {
		skeleton = "yMMMMEEEEd"
	}
	return mygoi18n.FormatDate(i18n.Locales()[0], d, mygoi18n.DateOptions{Skeleton: skeleton})
}
