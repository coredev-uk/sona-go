package i18n

import (
	"math"
	"time"

	mygoi18n "github.com/egoist/mygo/i18n"
)

// RelativeTime writes how long before now t was, as v0.2.0's relativeTime
// (shared/lib/format.ts): "never" for the zero time, "just now" under 45
// seconds, then rounded minutes, hours and days in the short style, with
// one day written "yesterday" (Intl's numeric "auto"). A time after now
// reads "just now".
func RelativeTime(t, now time.Time) string {
	if t.IsZero() || t.UnixMilli() <= 0 {
		return T("never")
	}
	if s, ok := shortAgo(now.Sub(t)); ok {
		return s
	}
	s := jsRound(float64(now.Sub(t).Milliseconds()) / 1000)
	h := jsRound(float64(jsRound(float64(s)/60)) / 60)
	d := jsRound(float64(h) / 24)
	if d == 1 {
		return T("yesterday")
	}
	return T("{days, plural, one {# day ago} other {# days ago}}", "days", d)
}

// jsRound is JavaScript's Math.round: halves go up.
func jsRound(x float64) int { return int(math.Floor(x + 0.5)) }

// shortAgo is RelativeTime under a day: "just now" under 45 seconds (and
// for a time ahead), then rounded minutes and hours; false from 24 rounded
// hours on.
func shortAgo(d time.Duration) (string, bool) {
	s := jsRound(float64(d.Milliseconds()) / 1000)
	if s < 45 {
		return T("just now"), true
	}
	m := jsRound(float64(s) / 60)
	if m < 60 {
		return T("{minutes, plural, one {# min. ago} other {# min. ago}}", "minutes", m), true
	}
	h := jsRound(float64(m) / 60)
	if h < 24 {
		return T("{hours, plural, one {# hr. ago} other {# hr. ago}}", "hours", h), true
	}
	return "", false
}

// CalendarDays is how many calendar days t's day comes before now's, both
// read in loc: 0 for today, 1 for yesterday, negative for a later day. It
// counts dates, not 24-hour spans, so 23:59 yesterday is a day ago at
// 00:01, and a 23- or 25-hour day across a daylight-saving change is one.
func CalendarDays(t, now time.Time, loc *time.Location) int {
	d, n := t.In(loc), now.In(loc)
	day := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
	today := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
	return int(today.Sub(day) / (24 * time.Hour))
}

// NextMidnight is the start of the day after now's in loc: when a label
// written by RelativeDay or CalendarDays next changes its day. Where the
// clocks skip midnight it is the first time that day has.
func NextMidnight(now time.Time, loc *time.Location) time.Time {
	n := now.In(loc)
	return time.Date(n.Year(), n.Month(), n.Day()+1, 0, 0, 0, 0, loc)
}

// RelativeDay writes when t was, before now, in calendar words read in loc,
// to follow a verb ("Updated {when}"). The buckets:
//
//   - today: "just now", "12 min. ago", "3 hr. ago" (RelativeTime's);
//   - the day before: "yesterday";
//   - two to six days ago: "on Tuesday";
//   - seven to 13 days ago: "last week";
//   - earlier in this month: "earlier this month";
//   - earlier in this year: "in March";
//   - the year before: "last year";
//   - older: "in 2023".
//
// The zero time is "never", and a time after now "just now", as in
// RelativeTime. Names and digits are the shown language's.
func RelativeDay(t, now time.Time, loc *time.Location) string {
	if t.IsZero() || t.UnixMilli() <= 0 {
		return T("never")
	}
	days := CalendarDays(t, now, loc)
	if days <= 0 {
		if s, ok := shortAgo(now.Sub(t)); ok {
			return s
		}
		// Still today after 23 and a half hours: a 25-hour day, or a time
		// just past midnight read late in the evening.
		return T("{hours, plural, one {# hr. ago} other {# hr. ago}}", "hours", int(now.Sub(t).Hours()))
	}
	d, n := t.In(loc), now.In(loc)
	switch {
	case days == 1:
		return T("yesterday")
	case days < 7:
		// translators: When something happened, two to six days ago, after a verb ("Updated on Tuesday"); weekday is the day's name.
		return T("on {weekday}", "weekday", skeleton(d, "EEEE"))
	case days < 14:
		// translators: When something happened, seven to 13 days ago, after a verb ("Updated last week").
		return T("last week")
	case d.Year() == n.Year() && d.Month() == n.Month():
		// translators: When something happened, two weeks or more ago but in this calendar month, after a verb ("Updated earlier this month").
		return T("earlier this month")
	case d.Year() == n.Year():
		// translators: When something happened, in an earlier month of this year, after a verb ("Updated in March"); month is the month's name standing alone.
		return T("in {month}", "month", skeleton(d, "LLLL"))
	case d.Year() == n.Year()-1:
		// translators: When something happened, in the calendar year before this one, after a verb ("Updated last year").
		return T("last year")
	}
	// translators: When something happened, two calendar years ago or more, after a verb ("Updated in 2023"); year is the year's number.
	return T("in {year}", "year", skeleton(d, "y"))
}

// FormatDateTime writes the day and the time of day in full, in the shown
// language's conventions: "Tuesday, November 10, 2026 at 2:30 PM". It is the
// exact time behind a RelativeDay label.
func FormatDateTime(t time.Time) string {
	return mygoi18n.FormatDate(current.Load().format, t, mygoi18n.DateOptions{Date: mygoi18n.FullStyle, Time: mygoi18n.ShortStyle})
}

// skeleton writes t's fields named by a CLDR skeleton in the shown
// language's formats.
func skeleton(t time.Time, s string) string {
	return mygoi18n.FormatDate(current.Load().format, t, mygoi18n.DateOptions{Skeleton: s})
}
