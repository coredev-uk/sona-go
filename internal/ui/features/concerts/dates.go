package concerts

import (
	"strings"
	"sync"
	"time"
	_ "time/tzdata" // venues' zones by name where the system has no zone database (Windows)

	mygoi18n "github.com/egoist/mygo/i18n"
	"golang.org/x/text/language"
	"golang.org/x/text/language/display"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
)

// Dates on the Concerts page (v0.2.0's dates.ts). A show happens on the
// venue's calendar day, at the venue's clock: Go sends the instant
// (StartsAt) and the venue's IANA zone, and every day, time and month here
// is read in that zone, in the language of the interface. The day filters
// are the user's own calendar days, "2026-11-14".

// dayRange is the days a filter covers, both ends included, as "2026-11-14".
type dayRange struct{ from, to string }

// isoDay is a calendar day as "2026-11-14".
func isoDay(t time.Time) string { return t.Format("2006-01-02") }

// presetRange is the days a preset covers from now; false for "any time"
// and for "dates" (a range the user types).
func presetRange(when string, now time.Time) (dayRange, bool) {
	day := func(offset int) time.Time {
		return time.Date(now.Year(), now.Month(), now.Day()+offset, 0, 0, 0, 0, now.Location())
	}
	weekday := int(now.Weekday()) // 0 is Sunday
	switch when {
	case whenWeek:
		// Today to Sunday: the rest of the week as people count it.
		return dayRange{isoDay(day(0)), isoDay(day((7 - weekday) % 7))}, true
	case whenWeekend:
		// Friday to Sunday; from today when it is already the weekend.
		toFriday := 5 - weekday
		switch weekday {
		case 0:
			toFriday = -2
		case 6:
			toFriday = -1
		}
		toSunday := (7 - weekday) % 7
		return dayRange{isoDay(day(max(toFriday, 0))), isoDay(day(toSunday))}, true
	case whenMonth:
		return dayRange{isoDay(day(0)), isoDay(day(29))}, true
	}
	return dayRange{}, false
}

// rangeFor is the range a filter asks for: a preset, or the dates typed in
// (both ends, in order, or none).
func rangeFor(p params, now time.Time) (dayRange, bool) {
	if p.when == whenDates {
		if p.from != "" && p.to != "" && p.from <= p.to {
			return dayRange{p.from, p.to}, true
		}
		return dayRange{}, false
	}
	return presetRange(p.when, now)
}

var zones sync.Map // name -> *time.Location

// zoneOf is the venue's zone; UTC for a name the zone database lacks (Go
// already sends "UTC" for those).
func zoneOf(name string) *time.Location {
	if v, ok := zones.Load(name); ok {
		return v.(*time.Location)
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		loc = time.UTC
	}
	zones.Store(name, loc)
	return loc
}

// here is this computer's zone: tests replace it.
var here = time.Local

// startOf is the show's start in the venue's zone.
func startOf(c services.Concert) time.Time {
	return time.UnixMilli(c.StartsAt).In(zoneOf(c.Timezone))
}

func formatSkeleton(t time.Time, skeleton string) string {
	return mygoi18n.FormatDate(i18n.Locales()[0], t, mygoi18n.DateOptions{Skeleton: skeleton})
}

// showDay is the show's calendar day at the venue, "2026-11-14".
func showDay(c services.Concert) string { return isoDay(startOf(c)) }

// showMonth is "November 2026".
func showMonth(c services.Concert) string { return formatSkeleton(startOf(c), "yMMMM") }

// showWeekday is "Wed", the day's weekday in short.
func showWeekday(c services.Concert) string { return formatSkeleton(startOf(c), "E") }

// showDate is "14", the day of the month.
func showDate(c services.Concert) string { return formatSkeleton(startOf(c), "d") }

// showMonthShort is "Apr".
func showMonthShort(c services.Concert) string { return formatSkeleton(startOf(c), "MMM") }

// showDayLong is "Wed 14 Apr": the whole day, for where it stands without
// a day heading.
func showDayLong(c services.Concert) string { return formatSkeleton(startOf(c), "MMMEd") }

// sameClock is whether the venue keeps the same clock as this computer at
// the show's time.
func sameClock(c services.Concert) bool {
	t := time.UnixMilli(c.StartsAt)
	_, there := t.In(zoneOf(c.Timezone)).Zone()
	_, local := t.In(here).Zone()
	return there == local
}

// showTime is the show's start at the venue's clock, "18:00" (or "6:00 pm"
// by the locale): with the zone's name ("EDT") when that clock is not this
// computer's, so a show in another country is never read as local time.
func showTime(c services.Concert) string {
	t := startOf(c)
	clock := i18n.FormatClock(t)
	if sameClock(c) {
		return clock
	}
	name, _ := t.Zone()
	if !isLetters(name) {
		// A zone with no abbreviation of its own is a GMT offset in the
		// locale's format ("GMT+3").
		name = strings.TrimSpace(strings.TrimPrefix(formatSkeleton(t, "jmz"), clock))
	}
	return clock + " " + name
}

func isLetters(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

// dayGroup is a group of shows on one day.
type dayGroup struct {
	day   string
	shows []services.Concert
}

// monthGroup is a month of shows, by the venue's calendar.
type monthGroup struct {
	key, title string
	days       []dayGroup
}

// groupShows groups the shows (already soonest first) by month, then by day.
func groupShows(shows []services.Concert) []monthGroup {
	var months []monthGroup
	for _, c := range shows {
		day := showDay(c)
		key := day[:7]
		if len(months) == 0 || months[len(months)-1].key != key {
			months = append(months, monthGroup{key: key, title: showMonth(c)})
		}
		m := &months[len(months)-1]
		if len(m.days) == 0 || m.days[len(m.days)-1].day != day {
			m.days = append(m.days, dayGroup{day: day})
		}
		d := &m.days[len(m.days)-1]
		d.shows = append(d.shows, c)
	}
	return months
}

// mileRegions are the regions whose roads, and so whose "how far", are in miles.
var mileRegions = map[string]bool{"US": true, "GB": true, "LR": true, "MM": true}

func regionOfLocale() string {
	r, _ := language.Make(i18n.Locales()[0]).Region()
	return r.String()
}

// distanceLabel is "12 km away", or "7 mi away" where distances are in
// miles; "" when unknown.
func distanceLabel(km int) string {
	if km <= 0 {
		return ""
	}
	text := i18n.FormatDistance(float64(km), mileRegions[regionOfLocale()])
	return i18n.T("{text} away", "text", text)
}

// venueLine is "Example Hall · London", the venue as its line gives it.
func venueLine(c services.Concert) string {
	v := c.Venue
	if v == nil {
		return ""
	}
	var parts []string
	for _, s := range []string{v.Name, v.City} {
		if s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " · ")
}

// countryName is a country's name in the interface's language, from its ISO
// code; the code when the language has none.
func countryName(code string) string {
	upper := strings.ToUpper(code)
	region, err := language.ParseRegion(upper)
	if err != nil {
		return upper
	}
	if name := display.Regions(language.Make(i18n.Locale())).Name(region); name != "" {
		return name
	}
	return upper
}
