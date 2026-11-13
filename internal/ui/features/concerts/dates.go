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

// dayGroup is a group of shows on one day. row is the agenda row of its
// first show (rows are numbered through the whole list), which a day of the
// strip jumps to. The words are made with the group, once for each listing:
// the weekday and number of the day's stub, the day's name for assistive
// technology, and each show's facts.
type dayGroup struct {
	day             string
	shows           []services.Concert
	facts           []showFacts
	row             int
	weekday, number string
	long            string
}

// monthGroup is a month of shows, by the venue's calendar. count is how many
// of them the list holds, written as "2 shows" in counted.
type monthGroup struct {
	key, title string
	days       []dayGroup
	count      int
	counted    string
}

// showFacts is what a row says of its show's time: worked out with the
// locale's date and clock formats, so once per show and not on every frame.
type showFacts struct {
	day, month, dayLong      string
	weekday, date, monthName string // the date block: "Wed", "14", "Apr"
	meta                     string // the start and, where known, how far
	clock, away, place       string // the start, how far ("" when unknown), "Example Hall · London"
}

type factsKey struct {
	id       string
	startsAt int64
	zone     string
	km       int
	place    string
}

// factsCache holds the facts of the shows built so far, for the locale and
// this computer's zone they were worked out in: another one empties it.
var factsCache struct {
	sync.Mutex
	locale string
	here   *time.Location
	m      map[factsKey]showFacts
}

// factsMax is how many shows the cache holds before it starts over: a page
// of the agenda and a few more.
const factsMax = 1024

// factsOf is c's showFacts.
func factsOf(c services.Concert) showFacts {
	k := factsKey{c.ID, c.StartsAt, c.Timezone, c.DistanceKm, venueLine(c)}
	locale := i18n.Locales()[0]
	factsCache.Lock()
	defer factsCache.Unlock()
	if factsCache.locale != locale || factsCache.here != here || len(factsCache.m) >= factsMax {
		factsCache.locale, factsCache.here, factsCache.m = locale, here, map[factsKey]showFacts{}
	}
	if f, ok := factsCache.m[k]; ok {
		return f
	}
	f := showFacts{
		day: showDay(c), month: showMonth(c), dayLong: showDayLong(c),
		weekday: showWeekday(c), date: showDate(c), monthName: showMonthShort(c),
		clock: showTime(c), away: distanceLabel(c.DistanceKm), place: k.place,
	}
	f.meta = f.clock
	if f.away != "" {
		f.meta += " · " + f.away
	}
	factsCache.m[k] = f
	return f
}

// groupShows groups the shows (already soonest first) by month, then by day.
func groupShows(shows []services.Concert) []monthGroup {
	var months []monthGroup
	for i, c := range shows {
		f := factsOf(c)
		day := f.day
		key := day[:7]
		if len(months) == 0 || months[len(months)-1].key != key {
			if n := len(months); n > 0 {
				months[n-1].counted = showCount(months[n-1].count)
			}
			months = append(months, monthGroup{key: key, title: f.month})
		}
		m := &months[len(months)-1]
		if len(m.days) == 0 || m.days[len(m.days)-1].day != day {
			m.days = append(m.days, dayGroup{
				day: day, row: i, weekday: f.weekday, number: f.date, long: f.dayLong,
			})
		}
		d := &m.days[len(m.days)-1]
		d.shows = append(d.shows, c)
		d.facts = append(d.facts, f)
		m.count++
	}
	if n := len(months); n > 0 {
		months[n-1].counted = showCount(months[n-1].count)
	}
	return months
}

// The coming-days strip: this many days from the first day it covers, and
// at least this many (a shorter range, as a weekend, is already short).
const (
	stripDays = 14
	stripMin  = 7
)

// stripDay is a day of the strip: its weekday and number as the user's
// calendar reads them, and how many shows the list has on it (and where the
// first one is: its agenda row).
type stripDay struct {
	day     string // "2027-04-14"
	weekday string // "Wed"
	num     string // "14"
	label   string // "Wed, Apr 14, 2 shows"; "" for a day without shows
	count   int
	row     int
}

// showCount is "2 shows".
func showCount(n int) string {
	return i18n.T("{count, plural, one {# show} other {# shows}}", "count", n)
}

// comingDays is the strip over the days from first (a calendar day, at its
// midnight), stripDays of them at most, to last at the latest ("" for no
// end): each with the shows the month groups hold on it. It is nil when none
// of the days has a show, or when the range is too short to be a strip.
func comingDays(first time.Time, last string, months []monthGroup) []stripDay {
	type hit struct{ count, row int }
	hits := map[string]hit{}
	for _, m := range months {
		for _, d := range m.days {
			hits[d.day] = hit{len(d.shows), d.row}
		}
	}
	var days []stripDay
	busy := false
	for i := range stripDays {
		t := first.AddDate(0, 0, i)
		day := isoDay(t)
		if last != "" && day > last {
			break
		}
		h := hits[day]
		d := stripDay{day: day, weekday: formatSkeleton(t, "E"), num: formatSkeleton(t, "d"), count: h.count, row: h.row}
		if h.count > 0 {
			busy = true
			d.label = formatSkeleton(t, "MMMEd") + ", " + showCount(h.count)
		}
		days = append(days, d)
	}
	if !busy || len(days) < stripMin {
		return nil
	}
	return days
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
