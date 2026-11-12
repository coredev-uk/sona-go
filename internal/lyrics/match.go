package lyrics

import (
	"regexp"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// durationTolerance is how far a provider's track length may be from the
// song's for a title/artist match to count.
const durationTolerance = 3 * time.Second

var (
	// Bracketed parts of a title: "(feat. X)", "[Remastered 2011]", "【Live】".
	reBracketed = regexp.MustCompile(`\s*[(\[{（【][^)\]}）】]*[)\]}）】]`)
	// A trailing " - Remastered 2011", " - Live", " - Radio Edit".
	reDashTail = regexp.MustCompile(`\s+[-–—]\s+.*$`)
	// "feat." and friends, with whatever follows.
	reFeat = regexp.MustCompile(`(?i)\s+(feat\.?|ft\.?|featuring)\s+.*$`)
	// Separators between artist names.
	reArtistSep = regexp.MustCompile(`(?i)\s*(,|&|/|、|;|\+|\s+x\s+|\s+and\s+|\s+feat\.?\s+|\s+ft\.?\s+|\s+featuring\s+|\s+with\s+|\s+vs\.?\s+)\s*`)
)

// fold removes diacritics: "Beyoncé" → "Beyonce".
var fold = transform.Chain(norm.NFKD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)

// normalize lower-cases s, removes diacritics, reads "&" as "and" and
// keeps letters and digits only, separated by single spaces.
func normalize(s string) string {
	if folded, _, err := transform.String(fold, s); err == nil {
		s = folded
	}
	s = strings.ReplaceAll(strings.ToLower(s), "&", " and ")
	var b strings.Builder
	space := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteRune(r)
			space = false
		} else {
			space = true
		}
	}
	return b.String()
}

// baseTitle is a title without its bracketed parts, dash tail and
// featured artists, normalized.
func baseTitle(s string) string {
	t := reBracketed.ReplaceAllString(s, "")
	t = reDashTail.ReplaceAllString(t, "")
	t = reFeat.ReplaceAllString(t, "")
	if n := normalize(t); n != "" {
		return n
	}
	return normalize(s)
}

// titleMatches reports whether two titles name the same song: equal once
// normalized, or equal without versions and featured artists.
func titleMatches(want, got string) bool {
	if want == "" || got == "" {
		return false
	}
	return normalize(want) == normalize(got) || baseTitle(want) == baseTitle(got)
}

// artists splits an artist credit into normalized names.
func artists(s string) []string {
	var out []string
	for _, a := range reArtistSep.Split(s, -1) {
		if n := normalize(a); n != "" {
			out = append(out, n)
		}
	}
	return out
}

// artistMatches reports whether the credits share an artist (or are the
// same once normalized, for names that contain a separator, like
// "Simon & Garfunkel").
func artistMatches(want, got string) bool {
	if want == "" || got == "" {
		return false
	}
	if normalize(want) == normalize(got) {
		return true
	}
	gs := artists(got)
	for _, w := range artists(want) {
		for _, g := range gs {
			if w == g {
				return true
			}
		}
	}
	return false
}

// durationMatches reports whether two lengths are within the tolerance; an
// unknown length (0) matches any.
func durationMatches(want, got time.Duration) bool {
	if want <= 0 || got <= 0 {
		return true
	}
	d := want - got
	return d >= -durationTolerance && d <= durationTolerance
}

// candidate is a provider's track as matched against the request.
type candidate struct {
	Title, Artist, Album string
	Duration             time.Duration
}

// score rates c against t: -1 when it is not the same song (title, artist
// or length differ), else higher for an exact title, the same album and a
// closer length.
func score(t Track, c candidate) int {
	if !titleMatches(t.Title, c.Title) || !artistMatches(t.Artist, c.Artist) || !durationMatches(t.Duration, c.Duration) {
		return -1
	}
	s := 100
	if normalize(t.Title) == normalize(c.Title) {
		s += 20
	}
	if t.Album != "" && c.Album != "" && baseTitle(t.Album) == baseTitle(c.Album) {
		s += 10
	}
	if t.Duration > 0 && c.Duration > 0 {
		d := (t.Duration - c.Duration).Abs()
		s -= int(d / (100 * time.Millisecond)) // up to -30 within the tolerance
	}
	return s
}

// best returns the index of the best-scoring candidate, -1 when none
// matches. A candidate for which preferred (nil: none) is true outranks
// every other one.
func best(t Track, cs []candidate, preferred func(int) bool) int {
	idx, top := -1, -1
	for i, c := range cs {
		s := score(t, c)
		if s < 0 {
			continue
		}
		if preferred != nil && preferred(i) {
			s += 1000
		}
		if s > top {
			idx, top = i, s
		}
	}
	return idx
}
