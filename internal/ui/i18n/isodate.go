package i18n

import (
	"regexp"
	"time"
)

var (
	isoYearRE = regexp.MustCompile(`^\d{4}`)
	isoDayRE  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
)

// YearOf is the year of an ISO release date ("2008-06-02" or "2008"), "" when
// it has none (v0.2.0's yearOf).
func YearOf(date string) string { return isoYearRE.FindString(date) }

// FormatISODate writes a release date in words in style (DateLong:
// "10 October 2007"): a full ISO day in full, the year for a partial date,
// "" for none (v0.2.0's longDate).
func FormatISODate(date string, style DateStyle) string {
	if !isoDayRE.MatchString(date) {
		return YearOf(date)
	}
	at, err := time.ParseInLocation("2006-01-02", date, time.Local)
	if err != nil {
		return YearOf(date)
	}
	// Midday local time, so no time zone moves it to another day.
	return FormatDate(at.Add(12*time.Hour), style)
}
