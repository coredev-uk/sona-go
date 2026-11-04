package song

import (
	"testing"

	"github.com/coredev-uk/sona-go/internal/services"
)

func TestCreditTitle(t *testing.T) {
	cases := []struct {
		name string
		g    services.CreditGroup
		want string
	}{
		{"performer", services.CreditGroup{Kind: "performer", Title: "PERFORMING ARTISTS"}, "Performers"},
		{"composition", services.CreditGroup{Kind: "composer-and-lyrics", Title: "COMPOSITION & LYRICS"}, "Composition and lyrics"},
		{"production", services.CreditGroup{Kind: "production-and-engineering", Title: "PRODUCTION & ENGINEERING"}, "Production and engineering"},
		{"unknown kinds' titles in sentence case", services.CreditGroup{Kind: "other", Title: "MIXING & MASTERING"}, "Mixing and mastering"},
		{"trimmed", services.CreditGroup{Kind: "other", Title: "  VIDEO  "}, "Video"},
		{"no title", services.CreditGroup{Kind: "other", Title: ""}, "Credits"},
		{"blank title", services.CreditGroup{Kind: "other", Title: "   "}, "Credits"},
		{"a non-ASCII first letter", services.CreditGroup{Kind: "x", Title: "ÉQUIPE"}, "Équipe"},
	}
	for _, c := range cases {
		if got := creditTitle(c.g); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}
