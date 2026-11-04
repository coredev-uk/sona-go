package lyricsync

import "github.com/coredev-uk/sona-go/internal/services"

// WordProgress is how far the voice is through w at t: 0 before it begins,
// 1 once it ends, linear in between. A word with no length is sung the
// moment it begins.
func WordProgress(w Span, t int64) float64 {
	if t < w.BeginMs {
		return 0
	}
	if t >= w.EndMs {
		return 1
	}
	return float64(t-w.BeginMs) / float64(w.EndMs-w.BeginMs)
}

// Sung is what a word shows.
type Sung int

const (
	// NotSung: not sung yet.
	NotSung Sung = iota
	// Singing: being sung (part of it filled).
	Singing
	// Done: sung.
	Done
)

// WordState is a word's state at t.
type WordState struct {
	Sung Sung
	P    float64
}

// HeldMs: words sung at least this long are held notes, which fill as the
// voice holds them in v0.2.0 (shorter ones lit whole, where a fill would be
// a blur of frames nobody could follow in a web view). The native sweep
// draws on the GPU every frame and fills every word (kit.SweepText); this
// stays for the policy the tests pin and for reduced motion.
const HeldMs = 600

// FillStepMs is the interval at which v0.2.0 redrew a held note's fill.
const FillStepMs = 80

// StateOf is a word's state at t. A held note (HeldMs) is part-sung while
// the voice holds it; any other word, and every word without smooth
// (reduced motion), is lit whole from its begin.
func StateOf(w Span, t int64, smooth bool) WordState {
	p := WordProgress(w, t)
	if t < w.BeginMs {
		return WordState{NotSung, 0}
	}
	if p >= 1 || !smooth || w.EndMs-w.BeginMs < HeldMs {
		return WordState{Done, 1}
	}
	return WordState{Singing, p}
}

// LineSpan is the time a line is sung: from its begin (or an earlier first
// word) to its end or its last word's end, whichever is later. Background
// vocals often run on past the line's own end, under the next line.
func LineSpan(l *services.LyricLine) Span {
	s := Span{l.BeginMs, l.EndMs}
	for _, w := range l.Words {
		s.BeginMs = min(s.BeginMs, w.BeginMs)
		s.EndMs = max(s.EndMs, w.EndMs)
	}
	return s
}

// VerseBreakMs: a pause this long between one line's end and the next one's
// begin is a break between verses: the sheet leaves a gap there, and
// nothing is lit while the voice rests.
const VerseBreakMs = 3_000

// restGraceMs is how long a line stays lit after it ends when a break
// follows it.
const restGraceMs = 500

// RestFrom is when the voice rests after line i: its end plus a short grace
// when a break follows it (the last line is followed by one), Never when the
// next line follows closely or the line has no end.
func RestFrom(spans []Span, i int) int64 {
	s := spans[i]
	if s.EndMs <= s.BeginMs {
		return Never
	}
	next := Never
	if i+1 < len(spans) {
		next = spans[i+1].BeginMs
	}
	if next == Never || next-s.EndMs >= VerseBreakMs {
		return s.EndMs + restGraceMs
	}
	return Never
}

// Sheet is the sung line and where the sheet should be.
type Sheet struct {
	// Active is the line sung now, -1 before the first and while the voice rests.
	Active int
	// Anchor is the line to keep in view: the sung one, or the one coming
	// after a rest.
	Anchor int
	// Next is when Active or Anchor next changes (Never: never).
	Next int64
}

// SungLine is the sung line at t: the last line that has begun, unless the
// voice is resting in a break after it. spans are the lines' spans in order.
func SungLine(spans []Span, t int64) Sheet {
	k := ActiveLine(spans, t)
	if k < 0 {
		s := Sheet{Active: -1, Anchor: -1, Next: Never}
		if len(spans) > 0 {
			s.Anchor, s.Next = 0, spans[0].BeginMs
		}
		return s
	}
	nextBegin := Never
	if k+1 < len(spans) {
		nextBegin = spans[k+1].BeginMs
	}
	rest := RestFrom(spans, k)
	if t >= rest {
		anchor := k
		if k+1 < len(spans) {
			anchor = k + 1
		}
		return Sheet{Active: -1, Anchor: anchor, Next: nextBegin}
	}
	return Sheet{Active: k, Anchor: k, Next: min(rest, nextBegin)}
}

// VerseStarts says which lines open a verse: after a break (VerseBreakMs),
// or where the song part the lyrics name changes ("Verse" to "Chorus"). The
// first line opens none: it starts the sheet.
func VerseStarts(lines []services.LyricLine, spans []Span) []bool {
	out := make([]bool, len(lines))
	for i := 1; i < len(lines); i++ {
		l, prev := &lines[i], &lines[i-1]
		if l.Part != "" && prev.Part != "" && l.Part != prev.Part {
			out[i] = true
			continue
		}
		end := spans[i-1].EndMs
		out[i] = end > spans[i-1].BeginMs && spans[i].BeginMs-end >= VerseBreakMs
	}
	return out
}

// Verses groups the lines' indexes into verses; plain lyrics (synced false)
// are one verse.
func Verses(lines []services.LyricLine, spans []Span, synced bool) [][]int {
	var starts []bool
	if synced {
		starts = VerseStarts(lines, spans)
	}
	var verses [][]int
	for i := range lines {
		if i == 0 || (starts != nil && starts[i]) {
			verses = append(verses, nil)
		}
		verses[len(verses)-1] = append(verses[len(verses)-1], i)
	}
	return verses
}

// lookBack is how many earlier lines may still be sung under the current one.
const lookBack = 4

// SingingLines are the lines whose words are lit at t, in order: the last
// line that has begun (it stays lit until the next begins, or until the voice
// rests after it) and the earlier ones still being sung, such as background
// vocals running on under the next line.
func SingingLines(spans []Span, t int64) []int {
	k := ActiveLine(spans, t)
	if k < 0 {
		return nil
	}
	var out []int
	for j := max(k-lookBack, 0); j < k; j++ {
		if spans[j].EndMs > t {
			out = append(out, j)
		}
	}
	if t < RestFrom(spans, k) {
		out = append(out, k)
	}
	return out
}

// NextChange is the next time after t at which anything lit could change: a
// word of words beginning or ending, the next line beginning, a singing line
// ending, or the voice resting. Never when nothing is left.
func NextChange(spans []Span, singing []int, words []Span, t int64) int64 {
	next := Never
	consider := func(ms int64) {
		if ms > t && ms < next {
			next = ms
		}
	}
	for _, w := range words {
		consider(w.BeginMs)
		consider(w.EndMs)
	}
	for _, i := range singing {
		consider(spans[i].EndMs)
	}
	k := ActiveLine(spans, t)
	if k >= 0 {
		consider(RestFrom(spans, k))
	}
	if k+1 < len(spans) {
		consider(spans[k+1].BeginMs)
	}
	return next
}

// Voice says whose line it is.
type Voice int

const (
	// VoiceOne is the lead voice.
	VoiceOne Voice = iota
	// VoiceTwo is a duet's other voices.
	VoiceTwo
	// VoiceAll is every voice together.
	VoiceAll
)

// LineVoices is each line's voice. Lines sung by a group are VoiceAll; when
// two or more other voices take lines, the first to sing is VoiceOne and the
// rest VoiceTwo. A song with one voice is VoiceOne throughout.
func LineVoices(lines []services.LyricLine) []Voice {
	lead, duet := "", false
	for _, l := range lines {
		if l.Agent == "" || l.AgentType == "group" {
			continue
		}
		if lead == "" {
			lead = l.Agent
		} else if l.Agent != lead {
			duet = true
		}
	}
	out := make([]Voice, len(lines))
	for i, l := range lines {
		switch {
		case l.AgentType == "group":
			out[i] = VoiceAll
		case duet && l.Agent != "" && l.Agent != lead:
			out[i] = VoiceTwo
		}
	}
	return out
}

// HasWordTiming reports whether the lyrics carry word timing to light word
// by word.
func HasWordTiming(l *services.Lyrics) bool {
	if l == nil || l.Timing != TimingWord {
		return false
	}
	for i := range l.Lines {
		if len(l.Lines[i].Words) > 0 {
			return true
		}
	}
	return false
}

// IsSinging reports whether line i is one of SingingLines(spans, t), without
// building the list (the sweep asks once per line per frame).
func IsSinging(spans []Span, t int64, i int) bool {
	k := ActiveLine(spans, t)
	switch {
	case k < 0 || i > k:
		return false
	case i == k:
		return t < RestFrom(spans, k)
	}
	return i >= k-lookBack && spans[i].EndMs > t
}
