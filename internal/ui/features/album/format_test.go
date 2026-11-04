package album

import (
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
)

func TestCatalogueLine(t *testing.T) {
	for _, c := range []struct {
		kind, year string
		songs      int
		ms         int64
		want       string
	}{
		{"Album", "2008", 10, 42 * 60000, "Album · 2008 · 10 songs, 42 min"},
		{"Single", "2020", 1, 3 * 60000, "Single · 2020 · 1 song, 3 min"},
		{"Album", "2008", 13, 72 * 60000, "Album · 2008 · 13 songs, 1 hr, 12 min"},
		{"Album", "", 3, 0, "Album · 3 songs"},
		{"", "", 14, 58 * 60000, "14 songs, 58 min"},
		{"Album", "2026", 0, 0, "Album · 2026"},
	} {
		if got := catalogueLine(c.kind, c.year, c.songs, c.ms); got != c.want {
			t.Errorf("catalogueLine = %q, want %q", got, c.want)
		}
	}
}

func TestDates(t *testing.T) {
	if got := yearOf("2008-06-02"); got != "2008" {
		t.Errorf("yearOf %q", got)
	}
	if got := yearOf("soon"); got != "" {
		t.Errorf("yearOf of text %q", got)
	}
	if got := longDate("2007-10-10"); got != "October 10, 2007" {
		t.Errorf("longDate %q", got)
	}
	// A year alone is all there is to say.
	if got := longDate("2007"); got != "2007" {
		t.Errorf("longDate of a year %q", got)
	}
}

func TestUpcoming(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	future := services.Album{Prerelease: true, ReleaseAt: now.Add(time.Hour).UnixMilli()}
	past := services.Album{Prerelease: true, ReleaseAt: now.Add(-time.Hour).UnixMilli()}
	yearOnly := services.Album{Prerelease: true}
	out := services.Album{ReleaseAt: now.Add(time.Hour).UnixMilli()}
	if !isUpcoming(future, now) || isUpcoming(past, now) || !isUpcoming(yearOnly, now) || isUpcoming(out, now) {
		t.Error("isUpcoming")
	}
	if got := releaseBadge(future, now); got != "Out Nov 4" {
		t.Errorf("releaseBadge %q", got)
	}
	if releaseBadge(yearOnly, now) != "" || releaseBadge(past, now) != "" {
		t.Error("a badge without a full date or after the release")
	}
}

func TestNamesPhrase(t *testing.T) {
	if got := joinNames([]string{"A", "B", "C"}); got != "A, B, and C" {
		t.Errorf("%q", got)
	}
	var many []string
	for _, n := range "ABCDEFGHIJKL" {
		many = append(many, string(n))
	}
	if got := joinNames(many); got != "A, B, C, D, E, F, G, H, and 4 others" {
		t.Errorf("%q", got)
	}
	// One name over the limit is named, not counted.
	if got := joinNames(many[:9]); got != "A, B, C, D, E, F, G, H, and I" {
		t.Errorf("%q", got)
	}
}
