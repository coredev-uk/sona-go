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

func TestFormatHelpers(t *testing.T) {
	for in, want := range map[string]string{"2008-06-02": "2008", "2008": "2008", "": "", "x2008": ""} {
		if got := yearOf(in); got != want {
			t.Errorf("yearOf(%q) = %q, want %q", in, got, want)
		}
	}
	for ms, want := range map[int64]string{0: "0:00", 59_000: "0:59", 284_000: "4:44", 3_600_000: "1:00:00", 3_725_000: "1:02:05", -5: "0:00"} {
		if got := formatTime(ms); got != want {
			t.Errorf("formatTime(%d) = %q, want %q", ms, got, want)
		}
	}
	if got := longDate("1997-05-21"); got != "May 21, 1997" {
		t.Errorf("longDate = %q", got)
	}
	if got := longDate("1997"); got != "1997" {
		t.Errorf("a year alone: %q", got)
	}
	if got := longDate(""); got != "" {
		t.Errorf("no date: %q", got)
	}
}
