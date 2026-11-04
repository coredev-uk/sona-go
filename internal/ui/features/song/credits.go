package song

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
)

// creditTitle is a credit group's heading: Sona's title for the kinds it
// knows (Apple sends upper-case titles), else Apple's in sentence case with
// "&" as "and" (v0.2.0's credits.ts).
func creditTitle(g services.CreditGroup) string {
	switch g.Kind {
	case "performer":
		return i18n.T("Performers")
	case "composer-and-lyrics":
		return i18n.T("Composition and lyrics")
	case "production-and-engineering":
		return i18n.T("Production and engineering")
	}
	s := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(g.Title)), "&", "and")
	if s == "" {
		return i18n.T("Credits")
	}
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[n:]
}
