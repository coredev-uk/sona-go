package album

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	mygoi18n "github.com/egoist/mygo/i18n"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
)

// This file is the formatting the album page takes from v0.2.0's
// shared/lib/format.ts and release.ts.

var yearRE = regexp.MustCompile(`^(\d{4})`)

// yearOf is the year of an ISO date ("2008-06-02" or "2008"), or "".
func yearOf(date string) string {
	if m := yearRE.FindString(date); m != "" {
		return m
	}
	return ""
}

var fullDateRE = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})$`)

// longDate is a release date in words ("10 October 2007") for a full ISO
// date, the year for a partial one, "" for none.
func longDate(date string) string {
	m := fullDateRE.FindStringSubmatch(date)
	if m == nil {
		return yearOf(date)
	}
	y, _ := strconv.Atoi(m[1])
	mo, _ := strconv.Atoi(m[2])
	d, _ := strconv.Atoi(m[3])
	// Midday local time, so no time zone moves it to another day.
	return i18n.FormatDate(time.Date(y, time.Month(mo), d, 12, 0, 0, 0, time.Local), i18n.DateLong)
}

// fullReleaseDate is a release day in full: "Thursday 19 November 2026".
func fullReleaseDate(releaseAt int64) string {
	return i18n.FormatDate(time.UnixMilli(releaseAt), i18n.DateFull)
}

// isUpcoming is v0.2.0's isUpcoming: an album is still to come when it is
// prerelease and has no full release date, or that date is ahead.
func isUpcoming(a services.Album, now time.Time) bool {
	return a.Prerelease && (a.ReleaseAt == 0 || now.UnixMilli() < a.ReleaseAt)
}

// releaseBadge is the badge on an upcoming album's cover ("Out 19 Nov", with
// the year when it is not this one's); "" for an album that is out or has no
// full date.
func releaseBadge(a services.Album, now time.Time) string {
	if !isUpcoming(a, now) || a.ReleaseAt == 0 {
		return ""
	}
	at := time.UnixMilli(a.ReleaseAt)
	skeleton := "MMMd"
	if at.Year() != now.Year() {
		skeleton = "yMMMd"
	}
	date := mygoi18n.FormatDate(i18n.Locale(), at, mygoi18n.DateOptions{Skeleton: skeleton})
	return i18n.T("Out {releaseDate}", "releaseDate", date)
}

// unitWords are the words of a duration unit in a language: short and long
// forms, singular and plural ("min", "mins" is the en-GB short plural).
type unitWords struct{ short, shortN, long, longN string }

type durationWords struct{ day, hour, minute unitWords }

var durations = map[string]durationWords{
	"en": {
		day:    unitWords{"day", "days", "day", "days"},
		hour:   unitWords{"hr", "hr", "hour", "hours"},
		minute: unitWords{"min", "min", "minute", "minutes"},
	},
	"en-GB": {
		day:    unitWords{"day", "days", "day", "days"},
		hour:   unitWords{"hr", "hrs", "hour", "hours"},
		minute: unitWords{"min", "mins", "minute", "minutes"},
	},
	"de": {
		day:    unitWords{"Tg.", "Tg.", "Tag", "Tage"},
		hour:   unitWords{"Std.", "Std.", "Stunde", "Stunden"},
		minute: unitWords{"Min.", "Min.", "Minute", "Minuten"},
	},
	"fr": {
		day:    unitWords{"j", "j", "jour", "jours"},
		hour:   unitWords{"h", "h", "heure", "heures"},
		minute: unitWords{"min", "min", "minute", "minutes"},
	},
	"es": {
		day:    unitWords{"d", "d", "día", "días"},
		hour:   unitWords{"h", "h", "hora", "horas"},
		minute: unitWords{"min", "min", "minuto", "minutos"},
	},
	"it": {
		day:    unitWords{"g", "g", "giorno", "giorni"},
		hour:   unitWords{"h", "h", "ora", "ore"},
		minute: unitWords{"min", "min", "minuto", "minuti"},
	},
	"pt": {
		day:    unitWords{"dia", "dias", "dia", "dias"},
		hour:   unitWords{"h", "h", "hora", "horas"},
		minute: unitWords{"min", "min", "minuto", "minutos"},
	},
	"nl": {
		day:    unitWords{"dag", "dgn.", "dag", "dagen"},
		hour:   unitWords{"u", "u", "uur", "uur"},
		minute: unitWords{"min.", "min.", "minuut", "minuten"},
	},
	"sv": {
		day:    unitWords{"dag", "dgr", "dag", "dagar"},
		hour:   unitWords{"tim", "tim", "timme", "timmar"},
		minute: unitWords{"min", "min", "minut", "minuter"},
	},
}

// wordsFor finds a language's duration words: its own tag, its language,
// then English (units of languages not listed read in English).
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

// durationText writes whole units ("1 hr, 12 min"), zero units left out, as
// Intl.DurationFormat's "auto" display does; long spells the units out.
func durationText(days, hours, minutes int, long bool) string {
	w := wordsFor(i18n.Locale())
	var parts []string
	add := func(n int, u unitWords) {
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
		parts = append(parts, fmt.Sprintf("%s %s", i18n.FormatNumber(float64(n)), word))
	}
	add(days, w.day)
	add(hours, w.hour)
	add(minutes, w.minute)
	if len(parts) == 0 {
		parts = append(parts, i18n.FormatNumber(0)+" "+w.minute.shortN)
	}
	return strings.Join(parts, ", ")
}

// runningTime is a total running time for the catalogue line: "42 min",
// "1 hr, 12 min".
func runningTime(ms int64) string {
	minutes := int(math.Round(float64(ms) / 60000))
	return durationText(0, minutes/60, minutes%60, false)
}

// timeLeft is the time left until a moment, in words and to the minute, never
// finer: days and hours a day or more ahead, then hours and minutes ("18
// days, 6 hours", "5 hours, 12 minutes", "1 minute").
func timeLeft(d time.Duration) string {
	minutes := max(1, int(math.Ceil(float64(d)/float64(time.Minute))))
	days, hours := minutes/1440, minutes%1440/60
	switch {
	case days > 0:
		return durationText(days, hours, 0, true)
	case hours > 0:
		return durationText(0, hours, minutes%60, true)
	}
	return durationText(0, 0, minutes, true)
}

// catalogueLine is the catalogue line (docs/design.md): `Album · 2008 · 10
// songs, 42 min`. Empty parts are dropped; the song count and running time
// share a slot. kind is already translated.
func catalogueLine(kind, year string, songs int, runningMs int64) string {
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
	for _, p := range []string{kind, year, tail} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " · ")
}
