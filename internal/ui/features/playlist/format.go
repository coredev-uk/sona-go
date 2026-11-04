package playlist

import (
	"strings"
	"time"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
)

// This file is the formatting the playlist page takes from v0.2.0's
// shared/lib/format.ts: the catalogue line.

// catalogueLine is the catalogue line (docs/design.md): `Playlist · 10
// songs, 42 min`. Empty parts are dropped; the song count and running time
// share a slot. kind is already translated.
func catalogueLine(kind string, songs int, runningMs int64) string {
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
	for _, p := range []string{kind, tail} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " · ")
}
