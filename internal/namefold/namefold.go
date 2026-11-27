// Package namefold folds an artist's name to the form two spellings of it
// share, so names compare the way a person reads them: a credited name, the
// library's artist and a song page's artist are one artist when they fold
// alike. The database keeps the folded form (name_key columns) because SQLite
// cannot fold Unicode itself.
package namefold

import (
	"strings"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Fold is name Unicode-folded (widths and compatibility forms, accents
// dropped, case folded), its words single-spaced and typographic apostrophes
// straight ("Ed O’Brien" is "Ed O'Brien"). A transform chain keeps state, so
// each call builds its own.
func Fold(name string) string {
	fold := transform.Chain(norm.NFKD, runes.Remove(runes.In(unicode.Mn)), norm.NFC, cases.Fold())
	folded, _, err := transform.String(fold, name)
	if err != nil {
		folded = strings.ToLower(name)
	}
	folded = strings.NewReplacer("’", "'", "ʼ", "'", "‘", "'").Replace(folded)
	return strings.Join(strings.Fields(folded), " ")
}
