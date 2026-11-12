package i18n

import (
	"slices"
	"strings"

	mygoi18n "github.com/egoist/mygo/i18n"
)

// SourceLocale is the tag of the source catalogue: message IDs come from
// its text.
const SourceLocale = "en"

// PseudoLocale is accented, stretched English (MyGo's pseudo locale). It is
// shown only when set by hand, never matched from a preference.
const PseudoLocale = "en-XA"

// SelectableShare is how much of Sona's messages a locale's layers must
// translate before Sona shows it: a less complete one would show mostly
// English.
const SelectableShare = 0.9

func langOf(tag string) string {
	l, _, _ := strings.Cut(tag, "-")
	return l
}

// TranslationCandidates are the preferred tags (the account's storefront
// language, then the system's) to look for a translation of, in order: up
// to the first English one, since English, the source, answers it. Empty
// and repeated tags are skipped.
func TranslationCandidates(preferred []string) []string {
	var out []string
	for _, tag := range preferred {
		if tag == "" {
			continue
		}
		lower := strings.ToLower(tag)
		if langOf(lower) == SourceLocale {
			break
		}
		if !slices.ContainsFunc(out, func(t string) bool { return strings.EqualFold(t, lower) }) {
			out = append(out, tag)
		}
	}
	return out
}

// FormatLocales are the locales numbers and dates are written for while
// locale's catalogue is shown: the system's and then the account's tags of
// the same language, so an English UI on an en-GB system keeps 24-hour
// clocks and day-month dates, then the catalogue's own tag. A pseudo
// locale writes its own tag alone: MyGo transforms a message only when it
// is formatted for the pseudo locale, so a regional tag first would show
// the English untouched.
func FormatLocales(locale string, preferred []string) []string {
	if mygoi18n.IsPseudo(locale) {
		return []string{locale}
	}
	lang := langOf(strings.ToLower(locale))
	var out []string
	for _, tag := range preferred {
		if tag != "" && langOf(strings.ToLower(tag)) == lang && !slices.Contains(out, tag) {
			out = append(out, tag)
		}
	}
	if !slices.Contains(out, locale) {
		out = append(out, locale)
	}
	return out
}
