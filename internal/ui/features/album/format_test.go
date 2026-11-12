package album

import (
	"testing"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
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
		if got := i18n.CatalogueLine(c.kind, c.year, c.songs, c.ms); got != c.want {
			t.Errorf("CatalogueLine = %q, want %q", got, c.want)
		}
	}
}
