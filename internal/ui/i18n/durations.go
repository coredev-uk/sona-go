package i18n

import (
	"strings"
	"time"

	mygoi18n "github.com/egoist/mygo/i18n"
	"golang.org/x/text/language"
)

// MyGo carries no CLDR unit names, so the words of Intl.DurationFormat
// (v0.2.0's formatRunningTime and formatTimeLeft) live in a table keyed by
// language, with CLDR's wording, its "short" and "long" widths and its
// plural categories. The version is CLDR 48.2.0, the one MyGo v0.9.0's
// tables are generated from (i18n/internal/gen); the words are those of
// unicode-org/cldr-json tag 48.2.0, cldr-units-full (units.json,
// duration-day/hour/minute, unitPattern-count-*) and cldr-misc-full
// (listPatterns.json, listPattern-type-unit and unit-short, the "2"
// pattern), checked against every regional locale of the table's languages.
// The table covers English (with en-001's "hrs" and "mins" for the regions
// that inherit from it, and en-CA's own), German, French, Spanish, Italian,
// Portuguese, Dutch, Swedish, Polish, Russian, Turkish, Japanese, Chinese
// (Simplified, Traditional, Hong Kong) and Korean: 14 languages and 5
// further rows (en-001, en-CA, fr-CA and the two Chinese variants). Any other locale reads in English, pluralised by English's
// rules. A language is added by adding its row; each row's expected strings
// in durations_test.go come from Node's Intl.DurationFormat, not from this
// table. Digits and grouping follow the format locale (FormatNumber's).

// unitForms are one unit's words by plural category, "{0}" the number:
// [one, other], or [one, few, many, other] for languages with those
// categories, or one form when the language has no plural.
type unitForms []string

// durationWords are a language's words for days, hours and minutes in a
// width, and what joins two parts (its unit list's two-item pattern: neither
// function writes more than two). The short width never shows days.
type durationWords struct {
	day, hour, minute unitForms
	join              string
}

type durationLang struct{ short, long durationWords }

func words(day, hour, minute unitForms, join string) durationWords {
	return durationWords{day, hour, minute, join}
}

func forms(f ...string) unitForms { return f }

var durations = map[string]durationLang{
	"en": {
		short: words(nil, forms("{0} hr"), forms("{0} min"), ", "),
		long:  words(forms("{0} day", "{0} days"), forms("{0} hour", "{0} hours"), forms("{0} minute", "{0} minutes"), ", "),
	},
	"en-001": {
		short: words(nil, forms("{0} hr", "{0} hrs"), forms("{0} min", "{0} mins"), ", "),
		long:  words(forms("{0} day", "{0} days"), forms("{0} hour", "{0} hours"), forms("{0} minute", "{0} minutes"), ", "),
	},
	// en-CA is CLDR's own row, equal to en-001's; MyGo's parent table does
	// not chain en-CA through en-001.
	"en-CA": {
		short: words(nil, forms("{0} hr", "{0} hrs"), forms("{0} min", "{0} mins"), ", "),
		long:  words(forms("{0} day", "{0} days"), forms("{0} hour", "{0} hours"), forms("{0} minute", "{0} minutes"), ", "),
	},
	"de": {
		short: words(nil, forms("{0} Std."), forms("{0} Min."), ", "),
		long:  words(forms("{0} Tag", "{0} Tage"), forms("{0} Stunde", "{0} Stunden"), forms("{0} Minute", "{0} Minuten"), ", "),
	},
	// French writes a no-break space (U+00A0) or a narrow one (U+202F) before
	// most units, as CLDR does; its long minutes take a plain space.
	"fr": {
		short: words(nil, forms("{0}\u202fh"), forms("{0}\u00a0min"), " et "),
		long:  words(forms("{0}\u00a0jour", "{0}\u00a0jours"), forms("{0}\u00a0heure", "{0}\u00a0heures"), forms("{0} minute", "{0} minutes"), " et "),
	},
	// Canadian French writes only no-break spaces (U+00A0): its hours are
	// "1 h", not France's "1" U+202F "h", and its long minutes take one too.
	"fr-CA": {
		short: words(nil, forms("{0}\u00a0h"), forms("{0}\u00a0min"), " et "),
		long:  words(forms("{0}\u00a0jour", "{0}\u00a0jours"), forms("{0}\u00a0heure", "{0}\u00a0heures"), forms("{0}\u00a0minute", "{0}\u00a0minutes"), " et "),
	},
	"es": {
		short: words(nil, forms("{0} h"), forms("{0} min"), " y "),
		long:  words(forms("{0} día", "{0} días"), forms("{0} hora", "{0} horas"), forms("{0} minuto", "{0} minutos"), " y "),
	},
	"it": {
		short: words(nil, forms("{0} h"), forms("{0} min"), " e "),
		long:  words(forms("{0} giorno", "{0} giorni"), forms("{0} ora", "{0} ore"), forms("{0} minuto", "{0} minuti"), " e "),
	},
	"pt": {
		short: words(nil, forms("{0} h"), forms("{0} min"), " e "),
		long:  words(forms("{0} dia", "{0} dias"), forms("{0} hora", "{0} horas"), forms("{0} minuto", "{0} minutos"), " e "),
	},
	"nl": {
		short: words(nil, forms("{0} uur"), forms("{0} min"), ", "),
		long:  words(forms("{0} dag", "{0} dagen"), forms("{0} uur"), forms("{0} minuut", "{0} minuten"), " en "),
	},
	"sv": {
		short: words(nil, forms("{0} tim"), forms("{0} min"), ", "),
		long:  words(forms("{0} dygn"), forms("{0} timme", "{0} timmar"), forms("{0} minut", "{0} minuter"), ", "),
	},
	"pl": {
		short: words(nil, forms("{0} godz."), forms("{0} min"), " i "),
		long: words(
			forms("{0} dzień", "{0} dni", "{0} dni", "{0} dnia"),
			forms("{0} godzina", "{0} godziny", "{0} godzin", "{0} godziny"),
			forms("{0} minuta", "{0} minuty", "{0} minut", "{0} minuty"), " i "),
	},
	"ru": {
		short: words(nil, forms("{0} ч"), forms("{0} мин"), " "),
		long: words(
			forms("{0} день", "{0} дня", "{0} дней", "{0} дня"),
			forms("{0} час", "{0} часа", "{0} часов", "{0} часа"),
			forms("{0} минута", "{0} минуты", "{0} минут", "{0} минуты"), " "),
	},
	"tr": {
		short: words(nil, forms("{0} sa."), forms("{0} dk."), " "),
		long:  words(forms("{0} gün"), forms("{0} saat"), forms("{0} dakika"), " "),
	},
	"ja": {
		short: words(nil, forms("{0} 時間"), forms("{0} 分"), " "),
		long:  words(forms("{0} 日"), forms("{0} 時間"), forms("{0} 分"), " "),
	},
	"zh": {
		short: words(nil, forms("{0}小时"), forms("{0}分钟"), ""),
		long:  words(forms("{0}天"), forms("{0}小时"), forms("{0}分钟"), ""),
	},
	"zh-Hant": {
		short: words(nil, forms("{0} 小時"), forms("{0} 分鐘"), " "),
		long:  words(forms("{0} 天"), forms("{0} 小時"), forms("{0} 分鐘"), " "),
	},
	"zh-HK": {
		short: words(nil, forms("{0} 小時"), forms("{0} 分鐘"), " "),
		long:  words(forms("{0} 日"), forms("{0} 小時"), forms("{0} 分鐘"), " "),
	},
	"ko": {
		short: words(nil, forms("{0}시간"), forms("{0}분"), " "),
		long:  words(forms("{0}일"), forms("{0}시간"), forms("{0}분"), " "),
	},
}

// durationFor finds a locale's words and the tag whose plural rules count
// them (see langRow).
func durationFor(tag string) (durationLang, string) { return langRow(durations, tag) }

// langRow finds a locale's row of a per-language table and the tag whose
// plural rules count its words: Traditional Chinese by its script (Hong Kong
// and Macao write differently), else the locale and its CLDR parents
// ("en-AU" reads as "en-001"); a locale with no row reads the English one,
// counted by English's rules.
func langRow[T any](table map[string]T, tag string) (T, string) {
	if langOf(tag) == "zh" {
		if script, _ := language.Make(tag).Script(); script.String() == "Hant" {
			if region, _ := language.Make(tag).Region(); region.String() == "HK" || region.String() == "MO" {
				return table["zh-HK"], tag
			}
			return table["zh-Hant"], tag
		}
	}
	for t := tag; t != ""; t = mygoi18n.Parent(t) {
		if d, ok := table[t]; ok {
			return d, tag
		}
	}
	return table["en"], "en"
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
// The digits follow the format locale, the plural form pluralTag's rules.
func (f unitForms) part(tag, pluralTag string, n int) string {
	number := mygoi18n.FormatNumber(tag, n, mygoi18n.NumberOptions{})
	return strings.Replace(f.pick(pluralTag, n), "{0}", number, 1)
}

// durationParts writes the nonzero amounts, in the language's words and
// joined as its unit lists are (Intl.DurationFormat omits zero units); ""
// when all are zero, as Intl writes it.
func durationParts(w durationWords, tag, pluralTag string, days, hours, minutes int) string {
	var parts []string
	if days > 0 {
		parts = append(parts, w.day.part(tag, pluralTag, days))
	}
	if hours > 0 {
		parts = append(parts, w.hour.part(tag, pluralTag, hours))
	}
	if minutes > 0 {
		parts = append(parts, w.minute.part(tag, pluralTag, minutes))
	}
	return strings.Join(parts, w.join)
}

// FormatRunningTime writes a total running time for the catalogue line, to
// the minute and short: "42 min", "1 hr, 12 min" (v0.2.0's
// formatRunningTime, Intl.DurationFormat style "short") in the regional
// format locale. Under half a minute it is "", as Intl writes an empty
// duration.
func FormatRunningTime(d time.Duration) string {
	tag := current.Load().format
	minutes := 0
	if d > 0 {
		minutes = int((d + 30*time.Second) / time.Minute)
	}
	lang, pluralTag := durationFor(tag)
	return durationParts(lang.short, tag, pluralTag, 0, minutes/60, minutes%60)
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
	lang, pluralTag := durationFor(tag)
	w := lang.long
	switch {
	case days > 0:
		return durationParts(w, tag, pluralTag, days, hours, 0)
	case hours > 0:
		return durationParts(w, tag, pluralTag, 0, hours, minutes%60)
	}
	return durationParts(w, tag, pluralTag, 0, 0, minutes)
}
