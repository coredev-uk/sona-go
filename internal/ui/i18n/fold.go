package i18n

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// foldSplit are the letters NFD does not split into a base and a mark.
var foldSplit = strings.NewReplacer(
	"ß", "ss", "æ", "ae", "œ", "oe", "ø", "o", "đ", "d", "ð", "d", "ł", "l", "þ", "th", "ı", "i",
)

// Fold is text in lower case without accents: "Édith Piaf" → "edith piaf".
// A filter folds both what is typed and what it searches, so neither case
// nor accents matter.
func Fold(text string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(strings.ToLower(text)) {
		if !unicode.Is(unicode.M, r) {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(foldSplit.Replace(b.String()))
}
