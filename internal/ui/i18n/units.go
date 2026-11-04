package i18n

import (
	"math"
	"strings"

	mygoi18n "github.com/egoist/mygo/i18n"
)

// MyGo carries no CLDR unit names, so the short unit patterns of the sizes
// FormatBytes writes and the distances of the concerts page (v0.2.0's
// Intl.NumberFormat style "unit", unitDisplay "short") live in a table keyed
// by language, built like durations.go's: the words of unicode-org/cldr-json
// tag 48.2.0 (cldr-units-full units.json, short width: digital-byte,
// -kilobyte, -megabyte, -gigabyte, -terabyte, length-kilometer, -mile), for
// the same languages and rows. A language whose pattern has one form for
// every plural category lists it once. Any other locale reads in English.

// unitWords are a language's patterns for the units, "{0}" the number.
type unitWords struct {
	byteU, kilobyte, megabyte, gigabyte, terabyte, kilometer, mile unitForms
}

func unit(b, kb, mb, gb, tb, km, mi unitForms) unitWords {
	return unitWords{b, kb, mb, gb, tb, km, mi}
}

var units = map[string]unitWords{
	"en":    unit(forms("{0} byte"), forms("{0} kB"), forms("{0} MB"), forms("{0} GB"), forms("{0} TB"), forms("{0} km"), forms("{0} mi")),
	"en-CA": unit(forms("{0} byte", "{0} bytes"), forms("{0} kB"), forms("{0} MB"), forms("{0} GB"), forms("{0} TB"), forms("{0} km"), forms("{0} mi")),
	// German's gigabyte keeps a no-break space (U+00A0) and its bytes take
	// one in the singular.
	"de": unit(forms("{0}\u00a0Byte", "{0} Byte"), forms("{0} kB"), forms("{0} MB"), forms("{0}\u00a0GB"), forms("{0} TB"), forms("{0} km"), forms("{0} mi")),
	// French writes a narrow no-break space (U+202F) before its units.
	"fr":    unit(forms("{0}\u202fo"), forms("{0}\u202fko"), forms("{0}\u202fMo"), forms("{0}\u202fGo"), forms("{0}\u202fTo"), forms("{0}\u202fkm"), forms("{0}\u202fmi")),
	"fr-CA": unit(forms("{0}\u00a0o"), forms("{0}\u00a0ko"), forms("{0}\u00a0Mo"), forms("{0}\u00a0Go"), forms("{0}\u00a0To"), forms("{0}\u00a0km"), forms("{0}\u00a0mi")),
	"es":    unit(forms("{0} B"), forms("{0} kB"), forms("{0} MB"), forms("{0} GB"), forms("{0} TB"), forms("{0} km"), forms("{0} mi")),
	"it":    unit(forms("{0} byte"), forms("{0} kB"), forms("{0} MB"), forms("{0} GB"), forms("{0} TB"), forms("{0} km"), forms("{0} mi")),
	"pt":    unit(forms("{0} bytes"), forms("{0} kB"), forms("{0} MB"), forms("{0} GB"), forms("{0} TB"), forms("{0} km"), forms("{0} mi")),
	"nl":    unit(forms("{0} byte"), forms("{0} kB"), forms("{0} MB"), forms("{0} GB"), forms("{0} TB"), forms("{0} km"), forms("{0} mi")),
	"sv":    unit(forms("{0} B"), forms("{0} kB"), forms("{0} MB"), forms("{0} GB"), forms("{0} TB"), forms("{0} km"), forms("{0} mi")),
	"pl": unit(forms("{0} B"), forms("{0} kB"), forms("{0} MB"), forms("{0} GB"), forms("{0} TB"), forms("{0} km"),
		forms("{0} mila", "{0} mile", "{0} mil", "{0} mili")),
	"ru":      unit(forms("{0} Б"), forms("{0} кБ"), forms("{0} МБ"), forms("{0} ГБ"), forms("{0} ТБ"), forms("{0} км"), forms("{0} ми")),
	"tr":      unit(forms("{0} bayt"), forms("{0} kB"), forms("{0} MB"), forms("{0} GB"), forms("{0} TB"), forms("{0} km"), forms("{0} mil")),
	"ja":      unit(forms("{0} byte"), forms("{0} KB"), forms("{0} MB"), forms("{0} GB"), forms("{0} TB"), forms("{0} km"), forms("{0} mi")),
	"zh":      unit(forms("{0} B"), forms("{0} kB"), forms("{0} MB"), forms("{0} GB"), forms("{0} TB"), forms("{0}公里"), forms("{0}英里")),
	"zh-Hant": unit(forms("{0} byte"), forms("{0} kB"), forms("{0} MB"), forms("{0} GB"), forms("{0} TB"), forms("{0} 公里"), forms("{0} 英里")),
	"zh-HK":   unit(forms("{0} byte"), forms("{0} kB"), forms("{0} MB"), forms("{0} GB"), forms("{0} TB"), forms("{0} 公里"), forms("{0} 哩")),
	"ko":      unit(forms("{0}byte"), forms("{0}kB"), forms("{0}MB"), forms("{0}GB"), forms("{0}TB"), forms("{0}km"), forms("{0}mi")),
}

// writeUnit writes v in the pattern forms: the digits in the format
// locale's, the plural form of whole numbers by the pattern language's rules
// (a fraction is "other", as in CLDR's for these languages).
func writeUnit(f unitForms, tag, pluralTag string, v float64) string {
	pattern := f[len(f)-1]
	if v == math.Trunc(v) {
		pattern = f.pick(pluralTag, int(v))
	}
	return strings.Replace(pattern, "{0}", mygoi18n.FormatNumber(tag, v, mygoi18n.NumberOptions{}), 1)
}

// FormatDistance writes a length in whole kilometres or miles in the shown
// language's words and digits (Intl.NumberFormat style "unit", short):
// "12 km", "7 mi". miles converts km first, to at least one mile.
func FormatDistance(km float64, miles bool) string {
	tag := current.Load().format
	w, pluralTag := langRow(units, tag)
	if miles {
		return writeUnit(w.mile, tag, pluralTag, math.Max(1, math.Round(km*0.621371)))
	}
	return writeUnit(w.kilometer, tag, pluralTag, km)
}
