package lyricsync

import (
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
)

// Dur is ms milliseconds as a duration.
func Dur(ms int64) time.Duration { return time.Duration(ms) * time.Millisecond }

// SameLines reports whether b is the lines a is: the lyrics query hands out a
// copy of the DTO each frame, so the lines' backing array says whether they
// are the same lyrics.
func SameLines(a, b []services.LyricLine) bool {
	return len(a) == len(b) && len(a) > 0 && &a[0] == &b[0]
}

// SpanCache is the spans of one song's lines, worked out again only for other
// lyrics.
type SpanCache struct {
	lines []services.LyricLine
	spans []Span
}

// Of is the spans of lines.
func (c *SpanCache) Of(lines []services.LyricLine) []Span {
	if !SameLines(c.lines, lines) {
		c.lines, c.spans = lines, LineSpans(lines)
	}
	return c.spans
}
