package kit

import (
	"slices"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// foldText is lower case, accents folded, spaces collapsed: "Café  Mix" is
// "cafe mix" (v0.2.0's features/commands/rank.ts fold).
func foldText(s string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(s) {
		if !unicode.Is(unicode.M, r) {
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(strings.ToLower(b.String())), " ")
}

func wordsOf(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
}

func isSubsequence(q, text string) bool {
	qr := []rune(q)
	i := 0
	for _, c := range text {
		if i < len(qr) && c == qr[i] {
			i++
		}
	}
	return i == len(qr)
}

// MatchScore says how well query matches a label (and optional keywords),
// higher is better, 0 for no match: the whole label (100), its start (90),
// a word's start (80), every typed word starting a label word (70), anywhere
// in the label (60), every typed word starting a label or keyword word (40)
// and last the letters in order, "tgq" (20). An empty query matches all (1).
func MatchScore(query, label, keywords string) int {
	q := foldText(query)
	if q == "" {
		return 1
	}
	l := foldText(label)
	if l == q {
		return 100
	}
	if strings.HasPrefix(l, q) {
		return 90
	}
	lw := wordsOf(l)
	if slices.ContainsFunc(lw, func(w string) bool { return strings.HasPrefix(w, q) }) {
		return 80
	}
	typed := wordsOf(q)
	startsWord := func(pool []string) bool {
		for _, t := range typed {
			if !slices.ContainsFunc(pool, func(w string) bool { return strings.HasPrefix(w, t) }) {
				return false
			}
		}
		return true
	}
	if len(typed) > 0 && startsWord(lw) {
		return 70
	}
	if strings.Contains(l, q) {
		return 60
	}
	if len(typed) > 0 && startsWord(append(slices.Clone(lw), wordsOf(foldText(keywords))...)) {
		return 40
	}
	if isSubsequence(strings.ReplaceAll(q, " ", ""), l) {
		return 20
	}
	return 0
}

// RankCommands returns the items matching query, best first; equal scores
// keep their given order.
func RankCommands[T any](query string, items []T, label, keywords func(T) string) []T {
	type scored struct {
		item  T
		i, sc int
	}
	var hits []scored
	for i, it := range items {
		kw := ""
		if keywords != nil {
			kw = keywords(it)
		}
		if sc := MatchScore(query, label(it), kw); sc > 0 {
			hits = append(hits, scored{it, i, sc})
		}
	}
	slices.SortStableFunc(hits, func(a, b scored) int {
		if a.sc != b.sc {
			return b.sc - a.sc
		}
		return a.i - b.i
	})
	out := make([]T, len(hits))
	for i, h := range hits {
		out[i] = h.item
	}
	return out
}
