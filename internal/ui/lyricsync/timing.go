// Package lyricsync is the timing logic of synced lyrics, ported from
// v0.2.0's features/player/lyrics.ts and lyric-words.ts: which line is
// sung at a position, where the voice rests, how far through a word it is,
// the per-song timing offset. It holds no UI, so the Now Playing view, its
// full-screen lyrics and the mini player's line share it.
//
// All times are milliseconds into the song, as the lyrics DTO has them.
package lyricsync

import (
	"math"
	"sort"

	"github.com/coredev-uk/sona-go/internal/services"
)

// Never is a time that does not come.
const Never = int64(math.MaxInt64)

const (
	// LeadMs: lines are shown this early so the eye arrives as the voice does.
	LeadMs = 250
	// OffsetStepMs is one press of "earlier" or "later".
	OffsetStepMs = 500
	// MaxOffsetMs is the furthest the lyrics can be moved either way.
	MaxOffsetMs = 10_000
)

// Timing values of services.Lyrics.Timing.
const (
	TimingWord = "Word"
	TimingLine = "Line"
)

// Span is a stretch of the song: a word, or the time a line is sung.
type Span struct{ BeginMs, EndMs int64 }

// ActiveLine is the index of the line sung at ms: the last line that has
// begun, or -1 before the first.
func ActiveLine(spans []Span, ms int64) int {
	// The first line that has not begun; the one before it is sung.
	return sort.Search(len(spans), func(i int) bool { return spans[i].BeginMs > ms }) - 1
}

// IsSynced reports lyrics timed by line or word (others are plain text).
func IsSynced(l *services.Lyrics) bool {
	return l != nil && (l.Timing == TimingLine || l.Timing == TimingWord)
}

// NudgeOffset is the offset after one nudge (dir -1 earlier, 1 later), on
// the 0.5 s grid and within ±MaxOffsetMs. A negative offset shows the
// lyrics earlier, a positive one later.
func NudgeOffset(offsetMs int64, dir int) int64 {
	steps := int64(math.Floor(float64(offsetMs)/OffsetStepMs+0.5)) + int64(dir) // JS Math.round
	return min(MaxOffsetMs, max(-MaxOffsetMs, steps*OffsetStepMs))
}

// LyricsClock is the lyrics clock at playback position positionMs: what
// ActiveLine reads.
func LyricsClock(positionMs, offsetMs int64) int64 { return positionMs + LeadMs - offsetMs }

// WordClock is the words' clock: like LyricsClock without the lead, so a
// word lights as the voice reaches it.
func WordClock(positionMs, offsetMs int64) int64 { return positionMs - offsetMs }

// SeekTarget is where to seek so that the line starting at beginMs is the
// one sung.
func SeekTarget(beginMs, offsetMs int64) int64 { return max(0, beginMs+offsetMs) }

// LineSpans are the lines' spans (LineSpan of each).
func LineSpans(lines []services.LyricLine) []Span {
	out := make([]Span, len(lines))
	for i := range lines {
		out[i] = LineSpan(&lines[i])
	}
	return out
}

// Extras tells which localizations lyrics carry (the toggles shown for them).
func Extras(l *services.Lyrics) (translation, pronunciation bool) {
	if l == nil {
		return false, false
	}
	return l.TranslationLanguage != "", l.TransliterationLanguage != ""
}
