package main

import (
	"maps"
	"slices"
	"strings"
	"unicode"

	"github.com/coredev-uk/sona-go/internal/i18n/applestrings"
)

// Match kinds: exact means the literal text is identical (placeholders may
// be named differently); normalised means it is identical once case,
// curly quotes, ellipses, spacing and closing punctuation are ignored.
const (
	matchExact      = "exact"
	matchNormalised = "normalised"
)

// candidate is one Apple key that matches a Sona message.
type candidate struct {
	Key          string
	Plural       bool
	Kind         string
	Placeholders map[string]string // Apple name → Sona name
}

// appleIndex is the en-GB bundle grouped for matching: plain keys, and
// plural groups (a key base with a `.other` form).
type appleIndex struct {
	bundle applestrings.Bundle
	plain  []string
	plural []string
}

func indexBundle(b applestrings.Bundle) *appleIndex {
	ix := &appleIndex{bundle: b}
	bases := map[string]bool{}
	for _, k := range slices.Sorted(maps.Keys(b)) {
		if base, _, ok := applestrings.SplitPluralKey(k); ok {
			if _, other := b[base+".other"]; other && !bases[base] {
				bases[base] = true
				ix.plural = append(ix.plural, base)
			}
			continue
		}
		ix.plain = append(ix.plain, k)
	}
	return ix
}

// match returns every Apple key that matches the Sona message, best first.
func (ix *appleIndex) match(sona applestrings.Message) []candidate {
	keys, plural := ix.plain, false
	if sona.IsPlural() {
		keys, plural = ix.plural, true
	}
	var out []candidate
	for _, k := range keys {
		apple, _ := applestrings.AppleMessage(ix.bundle, k, plural)
		if kind, renames, ok := compare(sona, apple); ok {
			out = append(out, candidate{Key: k, Plural: plural, Kind: kind, Placeholders: renames})
		}
	}
	slices.SortStableFunc(out, func(a, b candidate) int {
		if a.Kind != b.Kind {
			if a.Kind == matchExact {
				return -1
			}
			return 1
		}
		if ra, rb := keyRank(a.Key), keyRank(b.Key); ra != rb {
			return ra - rb
		}
		if len(a.Key) != len(b.Key) {
			return len(a.Key) - len(b.Key)
		}
		return strings.Compare(a.Key, b.Key)
	})
	return out
}

// keyRank prefers Apple Music's own strings over the shared web platform's
// and other apps' (iPod, concerts) when the same text has several keys.
func keyRank(key string) int {
	switch {
	case strings.HasPrefix(key, "ASE.Web.Music."):
		return 0
	case strings.HasPrefix(key, "ASE.Web.Platform.Shared."):
		return 1
	case strings.HasPrefix(key, "ASE.Web."):
		return 2
	default:
		return 3
	}
}

// compare matches a Sona message with an Apple one. Plural messages must
// have the same categories (English: one and other). Placeholders pair up
// by position, consistently in every form; the result maps Apple's names
// to Sona's.
func compare(sona, apple applestrings.Message) (kind string, renames map[string]string, ok bool) {
	if sona.IsPlural() != apple.IsPlural() {
		return "", nil, false
	}
	if !slices.Equal(sona.Categories(), apple.Categories()) {
		return "", nil, false
	}
	renames = map[string]string{}
	exact := true
	for _, c := range sona.Categories() {
		s, a := sona.Forms[c], apple.Forms[c]
		sArgs, aArgs := args(s), args(a)
		if len(sArgs) != len(aArgs) {
			return "", nil, false
		}
		for i, an := range aArgs {
			sn := sArgs[i]
			if sn == applestrings.PluralValue {
				sn = sona.Plural
			}
			if prev, seen := renames[an]; seen && prev != sn {
				return "", nil, false
			}
			renames[an] = sn
		}
		if literal(s) != literal(a) {
			exact = false
			if normalise(s) != normalise(a) {
				return "", nil, false
			}
		}
	}
	// Text that is only placeholders and punctuation ("{a}, {b}") says
	// nothing about meaning, so any Apple key of that shape would match.
	if !slices.ContainsFunc(sona.Categories(), func(c string) bool {
		return strings.IndexFunc(literal(sona.Forms[c]), unicode.IsLetter) >= 0
	}) {
		return "", nil, false
	}
	// Two Apple placeholders may not fill one Sona argument.
	seen := map[string]bool{}
	for _, sn := range renames {
		if seen[sn] {
			return "", nil, false
		}
		seen[sn] = true
	}
	if exact {
		return matchExact, renames, true
	}
	return matchNormalised, renames, true
}

func args(toks []applestrings.Token) []string {
	var names []string
	for _, t := range toks {
		if t.IsArg {
			names = append(names, t.Placeholder)
		}
	}
	return names
}

// argMark stands for any placeholder in compared text.
const argMark = "\x00"

func literal(toks []applestrings.Token) string {
	var b strings.Builder
	for _, t := range toks {
		if t.IsArg {
			b.WriteString(argMark)
		} else {
			b.WriteString(t.Text)
		}
	}
	return b.String()
}

var normaliser = strings.NewReplacer(
	"\u2018", "'", "\u2019", "'", "\u201A", "'", "\u2032", "'",
	"\u201C", `"`, "\u201D", `"`, "\u201E", `"`, "\u2033", `"`,
	"\u2026", "...",
	"\u00A0", " ", "\u202F", " ", "\u2009", " ",
	"\u2013", "-", "\u2014", "-",
)

// normalise folds case, quotes, ellipses, dashes and spacing, and drops
// closing punctuation (".", "!", ":", "...").
func normalise(toks []applestrings.Token) string {
	s := strings.ToLower(normaliser.Replace(literal(toks)))
	s = strings.Join(strings.FieldsFunc(s, unicode.IsSpace), " ")
	for {
		t := strings.TrimRight(s, ".!:")
		t = strings.TrimSpace(t)
		if t == s {
			return s
		}
		s = t
	}
}
