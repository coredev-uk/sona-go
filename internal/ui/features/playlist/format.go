package playlist

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
)

// This file is the formatting the playlist page takes from v0.2.0's
// shared/lib/format.ts: the catalogue line and relative times.

// unitWords are the words of a duration unit in a language: short forms,
// singular and plural ("min", "mins" is the en-GB short plural).
type unitWords struct{ short, shortN string }

type durationWords struct{ hour, minute unitWords }

var durations = map[string]durationWords{
	"en":    {hour: unitWords{"hr", "hr"}, minute: unitWords{"min", "min"}},
	"en-GB": {hour: unitWords{"hr", "hrs"}, minute: unitWords{"min", "mins"}},
	"de":    {hour: unitWords{"Std.", "Std."}, minute: unitWords{"Min.", "Min."}},
	"fr":    {hour: unitWords{"h", "h"}, minute: unitWords{"min", "min"}},
	"es":    {hour: unitWords{"h", "h"}, minute: unitWords{"min", "min"}},
	"it":    {hour: unitWords{"h", "h"}, minute: unitWords{"min", "min"}},
	"pt":    {hour: unitWords{"h", "h"}, minute: unitWords{"min", "min"}},
	"nl":    {hour: unitWords{"u", "u"}, minute: unitWords{"min.", "min."}},
	"sv":    {hour: unitWords{"tim", "tim"}, minute: unitWords{"min", "min"}},
}

// wordsFor finds a language's duration words: its own tag, its language,
// then English.
func wordsFor(tag string) durationWords {
	if w, ok := durations[tag]; ok {
		return w
	}
	if i := strings.IndexAny(tag, "-_"); i > 0 {
		if w, ok := durations[tag[:i]]; ok {
			return w
		}
	}
	return durations["en"]
}

// runningTime is a total running time for the catalogue line: "42 min",
// "1 hr, 12 min".
func runningTime(ms int64) string {
	minutes := int(math.Round(float64(ms) / 60000))
	hours, minutes := minutes/60, minutes%60
	w := wordsFor(i18n.Locale())
	var parts []string
	add := func(n int, u unitWords) {
		if n == 0 {
			return
		}
		word := u.short
		if n != 1 {
			word = u.shortN
		}
		parts = append(parts, fmt.Sprintf("%s %s", i18n.FormatNumber(float64(n)), word))
	}
	add(hours, w.hour)
	add(minutes, w.minute)
	if len(parts) == 0 {
		return "0 " + w.minute.shortN
	}
	return strings.Join(parts, ", ")
}

// catalogueLine is the catalogue line (docs/design.md): `Playlist · 10
// songs, 42 min`. Empty parts are dropped; the song count and running time
// share a slot. kind is already translated.
func catalogueLine(kind string, songs int, runningMs int64) string {
	count := ""
	if songs > 0 {
		count = i18n.T("{0, plural, one {# song} other {# songs}}", "0", songs)
	}
	t := ""
	if runningMs > 0 {
		t = runningTime(runningMs)
	}
	tail := count
	switch {
	case count != "" && t != "":
		tail = i18n.T("{count}, {time}", "count", count, "time", t)
	case count == "":
		tail = t
	}
	var parts []string
	for _, p := range []string{kind, tail} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " · ")
}

// relativeTime is v0.2.0's relativeTime: "just now", then minutes, hours
// and days ago in short form; stamp is Unix milliseconds.
func relativeTime(stamp int64, now time.Time) string {
	if stamp <= 0 {
		return i18n.T("never")
	}
	s := int64(now.Sub(time.UnixMilli(stamp)).Round(time.Second) / time.Second)
	if s < 45 {
		return i18n.T("just now")
	}
	m := (s + 30) / 60
	if m < 60 {
		return i18n.T("{minutes, plural, one {# min. ago} other {# min. ago}}", "minutes", m)
	}
	h := (m + 30) / 60
	if h < 24 {
		return i18n.T("{hours, plural, one {# hr. ago} other {# hr. ago}}", "hours", h)
	}
	d := (h + 12) / 24
	if d == 1 {
		return i18n.T("yesterday")
	}
	return i18n.T("{days, plural, one {# day ago} other {# days ago}}", "days", d)
}
