package i18n

// Stand-in for the shared duration and date formatters PR (FormatRunningTime,
// FormatTimeLeft, ShortReleaseDate): same names and signatures, on the
// regional format locale (never the shown language). Drop this file for that
// PR's at rebase.

import (
	"fmt"
	"math"
	"strings"
	"time"

	mygoi18n "github.com/egoist/mygo/i18n"
)

// ShortReleaseDate is a release date for a badge: month and day, with the
// year when it is not this year's ("Nov 19", "19 Nov 2027" by locale).
func ShortReleaseDate(t, now time.Time) string {
	skeleton := "MMMd"
	if t.Year() != now.Year() {
		skeleton = "yMMMd"
	}
	return mygoi18n.FormatDate(current.Load().format, t, mygoi18n.DateOptions{Skeleton: skeleton})
}

// durationUnit are the words of a duration unit in a language: short and long
// forms, singular and plural ("min", "mins" is the en-GB short plural).
type durationUnit struct{ short, shortN, long, longN string }

type durationSet struct{ day, hour, minute durationUnit }

var durationTable = map[string]durationSet{
	"en": {
		day:    durationUnit{"day", "days", "day", "days"},
		hour:   durationUnit{"hr", "hr", "hour", "hours"},
		minute: durationUnit{"min", "min", "minute", "minutes"},
	},
	"en-GB": {
		day:    durationUnit{"day", "days", "day", "days"},
		hour:   durationUnit{"hr", "hrs", "hour", "hours"},
		minute: durationUnit{"min", "mins", "minute", "minutes"},
	},
	"de": {
		day:    durationUnit{"Tg.", "Tg.", "Tag", "Tage"},
		hour:   durationUnit{"Std.", "Std.", "Stunde", "Stunden"},
		minute: durationUnit{"Min.", "Min.", "Minute", "Minuten"},
	},
	"fr": {
		day:    durationUnit{"j", "j", "jour", "jours"},
		hour:   durationUnit{"h", "h", "heure", "heures"},
		minute: durationUnit{"min", "min", "minute", "minutes"},
	},
	"es": {
		day:    durationUnit{"d", "d", "día", "días"},
		hour:   durationUnit{"h", "h", "hora", "horas"},
		minute: durationUnit{"min", "min", "minuto", "minutos"},
	},
	"it": {
		day:    durationUnit{"g", "g", "giorno", "giorni"},
		hour:   durationUnit{"h", "h", "ora", "ore"},
		minute: durationUnit{"min", "min", "minuto", "minuti"},
	},
	"pt": {
		day:    durationUnit{"dia", "dias", "dia", "dias"},
		hour:   durationUnit{"h", "h", "hora", "horas"},
		minute: durationUnit{"min", "min", "minuto", "minutos"},
	},
	"nl": {
		day:    durationUnit{"dag", "dgn.", "dag", "dagen"},
		hour:   durationUnit{"u", "u", "uur", "uur"},
		minute: durationUnit{"min.", "min.", "minuut", "minuten"},
	},
	"sv": {
		day:    durationUnit{"dag", "dgr", "dag", "dagar"},
		hour:   durationUnit{"tim", "tim", "timme", "timmar"},
		minute: durationUnit{"min", "min", "minut", "minuter"},
	},
}

// durationWordsFor finds a language's duration words: its own tag, its language,
// then English (units of languages not listed read in English).
func durationWordsFor(tag string) durationSet {
	if w, ok := durationTable[tag]; ok {
		return w
	}
	if i := strings.IndexAny(tag, "-_"); i > 0 {
		if w, ok := durationTable[tag[:i]]; ok {
			return w
		}
	}
	return durationTable["en"]
}

// formatDurationParts writes whole units ("1 hr, 12 min"), zero units left out, as
// Intl.DurationFormat's "auto" display does; long spells the units out.
func formatDurationParts(days, hours, minutes int, long bool) string {
	w := durationWordsFor(current.Load().format)
	var parts []string
	add := func(n int, u durationUnit) {
		if n == 0 {
			return
		}
		word := u.short
		switch {
		case long && n == 1:
			word = u.long
		case long:
			word = u.longN
		case n != 1:
			word = u.shortN
		}
		parts = append(parts, fmt.Sprintf("%s %s", FormatNumber(float64(n)), word))
	}
	add(days, w.day)
	add(hours, w.hour)
	add(minutes, w.minute)
	if len(parts) == 0 {
		parts = append(parts, FormatNumber(0)+" "+w.minute.shortN)
	}
	return strings.Join(parts, ", ")
}

// FormatRunningTime is a total running time for the catalogue line: "42 min",
// "1 hr, 12 min".
func FormatRunningTime(d time.Duration) string {
	minutes := int(math.Round(d.Minutes()))
	return formatDurationParts(0, minutes/60, minutes%60, false)
}

// FormatTimeLeft is the time left until a moment, in words and to the minute, never
// finer: days and hours a day or more ahead, then hours and minutes ("18
// days, 6 hours", "5 hours, 12 minutes", "1 minute").
func FormatTimeLeft(d time.Duration) string {
	minutes := max(1, int(math.Ceil(float64(d)/float64(time.Minute))))
	days, hours := minutes/1440, minutes%1440/60
	switch {
	case days > 0:
		return formatDurationParts(days, hours, 0, true)
	case hours > 0:
		return formatDurationParts(0, hours, minutes%60, true)
	}
	return formatDurationParts(0, 0, minutes, true)
}
