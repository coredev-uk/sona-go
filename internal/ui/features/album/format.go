package album

import (
	"strings"
	"time"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
)

// This file is the formatting the album page takes from v0.2.0's
// shared/lib/format.ts and release.ts.

// fullReleaseDate is a release day in full: "Thursday 19 November 2026".
func fullReleaseDate(releaseAt int64) string {
	return i18n.FormatDate(time.UnixMilli(releaseAt), i18n.DateFull)
}

// catalogueLine is the catalogue line (docs/design.md): `Album · 2008 · 10
// songs, 42 min`. Empty parts are dropped; the song count and running time
// share a slot. kind is already translated.
func catalogueLine(kind, year string, songs int, runningMs int64) string {
	count := ""
	if songs > 0 {
		count = i18n.T("{0, plural, one {# song} other {# songs}}", "0", songs)
	}
	t := ""
	if runningMs > 0 {
		t = i18n.FormatRunningTime(time.Duration(runningMs) * time.Millisecond)
	}
	tail := count
	switch {
	case count != "" && t != "":
		tail = i18n.T("{count}, {time}", "count", count, "time", t)
	case count == "":
		tail = t
	}
	var parts []string
	for _, p := range []string{kind, year, tail} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " · ")
}
