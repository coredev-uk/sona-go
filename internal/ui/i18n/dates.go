package i18n

import (
	"time"

	mygoi18n "github.com/egoist/mygo/i18n"
)

// ShortReleaseDate writes a day short, "19 Nov" (en-GB) or "Nov 19" (en),
// with the year when it is not the year of now (v0.2.0's shortReleaseDate:
// Intl's {day, month: short} and, off this year, {day, month: short, year}).
// It follows the regional format locale, like FormatDate, never the
// catalogue's language.
func ShortReleaseDate(t, now time.Time) string {
	skeleton := "MMMd"
	if t.Year() != now.Year() {
		skeleton = "yMMMd"
	}
	return mygoi18n.FormatDate(current.Load().format, t, mygoi18n.DateOptions{Skeleton: skeleton})
}
