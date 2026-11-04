package song

import (
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
)

var (
	yearRe = regexp.MustCompile(`^(\d{4})`)
	dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
)

// yearOf is the year of an ISO date ("2008-06-02" or "2008"), or "".
func yearOf(date string) string {
	if m := yearRe.FindString(date); m != "" {
		return m
	}
	return ""
}

// formatTime is milliseconds as m:ss or h:mm:ss.
func formatTime(ms int64) string {
	total := max(0, ms/1000)
	h, m, s := total/3600, total%3600/60, total%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// longDate is a release date in words: "10 October 2007" for a full ISO
// date, the year for a partial one, "" for none.
func longDate(date string) string {
	if !dateRe.MatchString(date) {
		return yearOf(date)
	}
	y, _ := strconv.Atoi(date[0:4])
	m, _ := strconv.Atoi(date[5:7])
	d, _ := strconv.Atoi(date[8:10])
	// Midday local time, so no time zone moves it to another day.
	return i18n.FormatDate(time.Date(y, time.Month(m), d, 12, 0, 0, 0, time.Local), i18n.DateLong)
}
