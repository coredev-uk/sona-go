package i18n

import (
	"slices"
	"strings"
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

// MatchLocale returns the first of available that fits a preferred tag: an
// exact match ("pt-BR"), else one for the same language ("en-GB" → "en",
// "pt-BR" → "pt-PT"), ignoring case. Empty preferences are skipped.
func MatchLocale(preferred, available []string) (string, bool) {
	lower := make([]string, len(available))
	for i, a := range available {
		lower[i] = strings.ToLower(a)
	}
	for _, tag := range preferred {
		if tag == "" {
			continue
		}
		want := strings.ToLower(tag)
		i := slices.Index(lower, want)
		if i < 0 {
			i = slices.Index(lower, langOf(want))
		}
		if i < 0 {
			i = slices.IndexFunc(lower, func(a string) bool { return langOf(a) == langOf(want) })
		}
		if i >= 0 {
			return available[i], true
		}
	}
	return "", false
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

// TranslatedShare is the share of Sona's messages a locale translates:
// Apple's layer's coverage (counted by Go against the same catalogue) plus
// the community catalogue's messages Apple's layer lacks. apple and
// community are the layers by message ID (applestrings.MessageID), source
// the IDs of all Sona's messages.
func TranslatedShare(appleCoverage float64, apple, community map[string]string, source []string) float64 {
	if len(source) == 0 {
		return 0
	}
	extra := 0
	if community != nil {
		for _, id := range source {
			_, inCommunity := community[id]
			_, inApple := apple[id]
			if inCommunity && !inApple {
				extra++
			}
		}
	}
	return appleCoverage + float64(extra)/float64(len(source))
}

// Translation is a locale and the share of Sona's messages its layers
// translate.
type Translation struct {
	Locale string
	Share  float64
}

// ChooseTranslation is the translation to show: the first preferred locale
// (TranslationCandidates) whose layers translate something and reach
// minShare (SelectableShare when Sona chooses; 0 for the language the user
// chose in Settings, which shows however complete it is). ok is false for
// English. load returns a tag's translation, or false when it has none.
func ChooseTranslation(preferred []string, load func(tag string) (Translation, bool), minShare float64) (Translation, bool) {
	for _, tag := range TranslationCandidates(preferred) {
		if t, ok := load(tag); ok && t.Share > 0 && t.Share >= minShare {
			return t, true
		}
	}
	return Translation{}, false
}

// FormatLocales are the locales numbers and dates are written for while
// locale's catalogue is shown: the system's and then the account's tags of
// the same language, so an English UI on an en-GB system keeps 24-hour
// clocks and day-month dates, then the catalogue's own tag.
func FormatLocales(locale string, preferred []string) []string {
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
