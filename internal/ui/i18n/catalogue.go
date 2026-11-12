package i18n

import (
	"strings"
	"time"
)

// Meta joins the non-empty parts of a meta line with a middle dot:
// "Album · 2008 · 10 songs".
func Meta(parts ...string) string {
	var b strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString(" · ")
		}
		b.WriteString(p)
	}
	return b.String()
}

// SongCount is "1 song" or "12 songs".
func SongCount(n int) string {
	return T("{songs, plural, one {# song} other {# songs}}", "songs", n)
}

// CatalogueLine is the catalogue line (docs/design.md): `Album · 2008 · 10
// songs, 42 min`. Empty parts are dropped (no songs, no running time, no
// year); the song count and running time share a slot. kind is already
// translated.
func CatalogueLine(kind, year string, songs int, runningMs int64) string {
	count := ""
	if songs > 0 {
		count = SongCount(songs)
	}
	t := ""
	if runningMs > 0 {
		t = FormatRunningTime(time.Duration(runningMs) * time.Millisecond)
	}
	tail := count
	switch {
	case count != "" && t != "":
		tail = T("{count}, {time}", "count", count, "time", t)
	case count == "":
		tail = t
	}
	return Meta(kind, year, tail)
}
