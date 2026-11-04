package i18n

import (
	"math"
	"time"
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
	round := func(x float64) int { return int(math.Floor(x + 0.5)) } // JS Math.round
	s := round(float64(now.Sub(t).Milliseconds()) / 1000)
	if s < 45 {
		return T("just now")
	}
	m := round(float64(s) / 60)
	if m < 60 {
		return T("{minutes, plural, one {# min. ago} other {# min. ago}}", "minutes", m)
	}
	h := round(float64(m) / 60)
	if h < 24 {
		return T("{hours, plural, one {# hr. ago} other {# hr. ago}}", "hours", h)
	}
	d := round(float64(h) / 24)
	if d == 1 {
		return T("yesterday")
	}
	return T("{days, plural, one {# day ago} other {# days ago}}", "days", d)
}
