package radio

import (
	"fmt"
	"math"
	"strings"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
)

// Running times as v0.2.0 wrote them with Intl.DurationFormat in its "short"
// style: "45 min", "1 hr, 18 min". MyGo's i18n has no unit formatter, so the
// words of the languages Apple Music's catalogue shows live here; a language
// not listed reads in English. i18n has no shared formatter for this yet: the
// album feature keeps the same table (hoist both into i18n).

// unitWords are the short words of a duration unit: singular and plural ("hr"
// has no plural in en, "hrs" is en-GB's).
type unitWords struct{ one, many string }

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

// wordsFor finds a language's duration words: its own tag, its language, then
// English.
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

func (u unitWords) of(n int) string {
	if n == 1 {
		return u.one
	}
	return u.many
}

// minutesText is a number of minutes as Intl writes the unit in its short
// style ("57 min").
func minutesText(n int) string {
	return fmt.Sprintf("%s %s", i18n.FormatNumber(float64(n)), wordsFor(formatLocale()).minute.of(n))
}

// formatLocale is the locale numbers and dates are written for (the shown
// language, or the system's and the account's regional tags first: an English
// UI on an en-GB system says "mins"); the shown language when none is known.
func formatLocale() string {
	if l := i18n.Locales(); len(l) > 0 {
		return l[0]
	}
	return i18n.Locale()
}

// runningTime is a time as v0.2.0's formatRunningTime: rounded to the minute,
// hours and minutes with a zero unit left out ("1 hr, 18 min", "45 min").
func runningTime(ms int64) string {
	minutes := int(math.Round(float64(ms) / 60_000))
	hours, rest := minutes/60, minutes%60
	w := wordsFor(formatLocale())
	var parts []string
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%s %s", i18n.FormatNumber(float64(hours)), w.hour.of(hours)))
	}
	if rest > 0 || hours == 0 {
		parts = append(parts, minutesText(rest))
	}
	return strings.Join(parts, ", ")
}
