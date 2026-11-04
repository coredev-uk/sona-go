package i18n

import (
	"strings"
	"time"

	mygoi18n "github.com/egoist/mygo/i18n"
)

// MyGo carries no CLDR unit names, so the words of Intl.DurationFormat
// (v0.2.0's formatRunningTime and formatTimeLeft) live in a table keyed by
// language, with CLDR's own wording, "short" and "long" widths, and its
// plural categories. It covers the languages Sona's Language setting is most
// used in (English with its en-GB wording, German, French, Spanish, Italian,
// Portuguese, Dutch, Swedish, Polish, Russian, Turkish, Japanese, Chinese,
// Korean); every other locale reads in English. A language is added by
// adding its row. Digits and number grouping follow the format locale
// (FormatNumber's).

// unitForms are one unit's words by plural category, "{0}" the number:
// [one, other], or [one, few, many, other] for languages with those
// categories, or one form when the language has no plural.
type unitForms []string

// durationWords are a language's words for days, hours and minutes in a
// width, and what joins the parts.
type durationWords struct {
	day, hour, minute unitForms
	join              string
}

type durationLang struct{ short, long durationWords }

var durations = map[string]durationLang{
	"en": {
		short: durationWords{unitForms{"{0} day", "{0} days"}, unitForms{"{0} hr", "{0} hr"}, unitForms{"{0} min", "{0} min"}, ", "},
		long:  durationWords{unitForms{"{0} day", "{0} days"}, unitForms{"{0} hour", "{0} hours"}, unitForms{"{0} minute", "{0} minutes"}, ", "},
	},
	"en-GB": {
		short: durationWords{unitForms{"{0} day", "{0} days"}, unitForms{"{0} hr", "{0} hrs"}, unitForms{"{0} min", "{0} mins"}, ", "},
		long:  durationWords{unitForms{"{0} day", "{0} days"}, unitForms{"{0} hour", "{0} hours"}, unitForms{"{0} minute", "{0} minutes"}, ", "},
	},
	"de": {
		short: durationWords{unitForms{"{0} Tg.", "{0} Tg."}, unitForms{"{0} Std."}, unitForms{"{0} Min."}, ", "},
		long:  durationWords{unitForms{"{0} Tag", "{0} Tage"}, unitForms{"{0} Stunde", "{0} Stunden"}, unitForms{"{0} Minute", "{0} Minuten"}, ", "},
	},
	"fr": {
		short: durationWords{unitForms{"{0} j"}, unitForms{"{0} h"}, unitForms{"{0} min"}, ", "},
		long:  durationWords{unitForms{"{0} jour", "{0} jours"}, unitForms{"{0} heure", "{0} heures"}, unitForms{"{0} minute", "{0} minutes"}, ", "},
	},
	"es": {
		short: durationWords{unitForms{"{0} d"}, unitForms{"{0} h"}, unitForms{"{0} min"}, ", "},
		long:  durationWords{unitForms{"{0} día", "{0} días"}, unitForms{"{0} hora", "{0} horas"}, unitForms{"{0} minuto", "{0} minutos"}, ", "},
	},
	"it": {
		short: durationWords{unitForms{"{0} g"}, unitForms{"{0} h"}, unitForms{"{0} min"}, ", "},
		long:  durationWords{unitForms{"{0} giorno", "{0} giorni"}, unitForms{"{0} ora", "{0} ore"}, unitForms{"{0} minuto", "{0} minuti"}, ", "},
	},
	"pt": {
		short: durationWords{unitForms{"{0} d"}, unitForms{"{0} h"}, unitForms{"{0} min"}, ", "},
		long:  durationWords{unitForms{"{0} dia", "{0} dias"}, unitForms{"{0} hora", "{0} horas"}, unitForms{"{0} minuto", "{0} minutos"}, ", "},
	},
	"nl": {
		short: durationWords{unitForms{"{0} d"}, unitForms{"{0} u"}, unitForms{"{0} min."}, ", "},
		long:  durationWords{unitForms{"{0} dag", "{0} dagen"}, unitForms{"{0} uur"}, unitForms{"{0} minuut", "{0} minuten"}, ", "},
	},
	"sv": {
		short: durationWords{unitForms{"{0} dygn"}, unitForms{"{0} tim"}, unitForms{"{0} min"}, ", "},
		long:  durationWords{unitForms{"{0} dygn"}, unitForms{"{0} timme", "{0} timmar"}, unitForms{"{0} minut", "{0} minuter"}, ", "},
	},
	"pl": {
		short: durationWords{unitForms{"{0} dzień", "{0} dni", "{0} dni", "{0} dnia"}, unitForms{"{0} godz."}, unitForms{"{0} min"}, ", "},
		long: durationWords{
			unitForms{"{0} dzień", "{0} dni", "{0} dni", "{0} dnia"},
			unitForms{"{0} godzina", "{0} godziny", "{0} godzin", "{0} godziny"},
			unitForms{"{0} minuta", "{0} minuty", "{0} minut", "{0} minuty"}, ", "},
	},
	"ru": {
		short: durationWords{unitForms{"{0} дн.", "{0} дн.", "{0} дн.", "{0} дн."}, unitForms{"{0} ч"}, unitForms{"{0} мин."}, ", "},
		long: durationWords{
			unitForms{"{0} день", "{0} дня", "{0} дней", "{0} дня"},
			unitForms{"{0} час", "{0} часа", "{0} часов", "{0} часа"},
			unitForms{"{0} минута", "{0} минуты", "{0} минут", "{0} минуты"}, ", "},
	},
	"tr": {
		short: durationWords{unitForms{"{0} gün"}, unitForms{"{0} sa."}, unitForms{"{0} dk."}, ", "},
		long:  durationWords{unitForms{"{0} gün"}, unitForms{"{0} saat"}, unitForms{"{0} dakika"}, ", "},
	},
	"ja": {
		short: durationWords{unitForms{"{0}日"}, unitForms{"{0}時間"}, unitForms{"{0}分"}, " "},
		long:  durationWords{unitForms{"{0}日"}, unitForms{"{0}時間"}, unitForms{"{0}分"}, " "},
	},
	"zh": {
		short: durationWords{unitForms{"{0}天"}, unitForms{"{0}小时"}, unitForms{"{0}分钟"}, " "},
		long:  durationWords{unitForms{"{0}天"}, unitForms{"{0}小时"}, unitForms{"{0}分钟"}, " "},
	},
	"ko": {
		short: durationWords{unitForms{"{0}일"}, unitForms{"{0}시간"}, unitForms{"{0}분"}, " "},
		long:  durationWords{unitForms{"{0}일"}, unitForms{"{0}시간"}, unitForms{"{0}분"}, " "},
	},
}

// durationFor finds a locale's words: its own tag, then its language, then
// English.
func durationFor(tag string) durationLang {
	if d, ok := durations[tag]; ok {
		return d
	}
	if d, ok := durations[langOf(tag)]; ok {
		return d
	}
	return durations["en"]
}

// pick is the form for n in a language's plural rules.
func (f unitForms) pick(tag string, n int) string {
	if len(f) == 1 {
		return f[0]
	}
	op, err := mygoi18n.NewOperands(n)
	if err != nil {
		return f[len(f)-1]
	}
	switch mygoi18n.PluralCategory(tag, op) {
	case mygoi18n.PluralOne:
		return f[0]
	case mygoi18n.PluralFew:
		if len(f) == 4 {
			return f[1]
		}
	case mygoi18n.PluralMany:
		if len(f) == 4 {
			return f[2]
		}
	}
	return f[len(f)-1]
}

// part writes one amount of a unit: "5 min".
func (f unitForms) part(tag string, n int) string {
	number := mygoi18n.FormatNumber(tag, n, mygoi18n.NumberOptions{})
	return strings.Replace(f.pick(tag, n), "{0}", number, 1)
}

// durationParts writes the nonzero amounts, in the language's words and
// joined as its unit lists are (Intl.DurationFormat omits zero units).
func durationParts(w durationWords, tag string, days, hours, minutes int) string {
	var parts []string
	if days > 0 {
		parts = append(parts, w.day.part(tag, days))
	}
	if hours > 0 {
		parts = append(parts, w.hour.part(tag, hours))
	}
	if minutes > 0 {
		parts = append(parts, w.minute.part(tag, minutes))
	}
	if len(parts) == 0 {
		return w.minute.part(tag, 0)
	}
	return strings.Join(parts, w.join)
}

// FormatRunningTime writes a total running time for the catalogue line, to
// the minute and short: "42 min", "1 hr, 12 min" (v0.2.0's
// formatRunningTime, Intl.DurationFormat style "short") in the regional
// format locale.
func FormatRunningTime(d time.Duration) string {
	tag := current.Load().format
	minutes := int((d + 30*time.Second) / time.Minute)
	if d < 0 {
		minutes = 0
	}
	return durationParts(durationFor(tag).short, tag, 0, minutes/60, minutes%60)
}

// FormatTimeLeft writes the time left until a moment in words and to the
// minute, never finer (v0.2.0's formatTimeLeft, style "long"): days and
// hours a day or more ahead, then hours and minutes ("18 days, 6 hours",
// "5 hours, 12 minutes", "1 minute"). It rounds up and shows at least a
// minute.
func FormatTimeLeft(d time.Duration) string {
	tag := current.Load().format
	minutes := max(1, int((d+time.Minute-1)/time.Minute))
	days, hours := minutes/1440, minutes%1440/60
	w := durationFor(tag).long
	switch {
	case days > 0:
		return durationParts(w, tag, days, hours, 0)
	case hours > 0:
		return durationParts(w, tag, 0, hours, minutes%60)
	}
	return durationParts(w, tag, 0, 0, minutes)
}
