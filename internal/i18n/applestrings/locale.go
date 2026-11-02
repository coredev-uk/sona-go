package applestrings

import (
	"slices"
	"strings"
)

// BCP47 writes Apple's lower-case tags in BCP 47 case: "zh-hans-cn" →
// "zh-Hans-CN", "es-419" stays.
func BCP47(tag string) string {
	parts := strings.Split(strings.ToLower(tag), "-")
	for i, p := range parts {
		if i == 0 {
			continue
		}
		switch len(p) {
		case 2:
			parts[i] = strings.ToUpper(p)
		case 4:
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, "-")
}

// Skip says why Apple's locale is not used ("" to use it): its
// development strings, and English, which is Sona's own source text.
func Skip(tag string) string {
	switch t := strings.ToLower(tag); {
	case t == "dev":
		return "Apple's development strings"
	case t == "en" || strings.HasPrefix(t, "en-"):
		return "English: Sona's source text"
	}
	return ""
}

// subtags splits a lower-case tag into its language, script and region.
func subtags(tag string) (lang, script, region string) {
	parts := strings.Split(tag, "-")
	for _, p := range parts[1:] {
		switch {
		case len(p) == 4:
			script = p
		case len(p) == 2, len(p) == 3 && p[0] >= '0' && p[0] <= '9':
			region = p
		}
	}
	return parts[0], script, region
}

// MatchLocale picks Apple's locale for a preferred tag (any case) among
// available (Apple's lower-case tags): the same tag; else, within the
// language, the same region ("zh-TW" → "zh-hant-tw"), the same script, the
// bare language ("ko"), the language's own region ("fr" → "fr-fr"), then
// any. "" when Apple has none of the language.
func MatchLocale(want string, available []string) string {
	w := strings.ToLower(want)
	wl, ws, wr := subtags(w)
	if wl == "" {
		return ""
	}
	sorted := slices.Sorted(slices.Values(available))
	best, bestScore := "", 0
	for _, a := range sorted {
		al, as, ar := subtags(a)
		if al != wl {
			continue
		}
		score := 1
		switch {
		case a == w:
			score = 6
		case wr != "" && ar == wr:
			score = 5
		case ws != "" && as == ws:
			score = 4
		case a == wl:
			score = 3
		case ar == wl:
			score = 2
		}
		if score > bestScore {
			best, bestScore = a, score
		}
	}
	return best
}
