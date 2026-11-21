package lyricsync

import "math"

const (
	// GapMinMs is how long a stretch with no line sung has to be, between one
	// line's end and the next one's begin or before the first line, to be an
	// instrumental gap that is shown as three dots (Cider's lyrics.ts).
	GapMinMs = 5_000
	// GapStepMs is the grain of the dots' fill: about 12 steps a second.
	GapStepMs = 1000 / 12
	// GapDots is how many dots the gap fills, one over each third of it.
	GapDots = 3
)

// Gaps are the instrumental gaps before the lines: gaps[i] runs from where the
// lines before line i end (0 before the first line) to where line i begins,
// and is the zero Span where that is shorter than GapMinMs. Lines that run
// into each other (background vocals under the next line) leave no gap.
func Gaps(spans []Span) []Span {
	out := make([]Span, len(spans))
	end := int64(0)
	for i, s := range spans {
		if s.BeginMs-end >= GapMinMs {
			out[i] = Span{BeginMs: end, EndMs: s.BeginMs}
		}
		end = max(end, s.BeginMs, s.EndMs)
	}
	return out
}

// HasGap reports whether g is a gap (the zero Span is none).
func HasGap(g Span) bool { return g.EndMs > g.BeginMs }

// GapFill is how far each dot of the gap g has filled at t, 0 to 1: the first
// dot over the first third of the gap, the second over the second and the
// third over the last. The fill is stepped, GapStepMs at a time, as a held
// note's is; whole lights each dot all at once as its third begins (reduced
// motion). next is when the fill next changes (Never: not again), for a timer
// instead of a loop of frames.
func GapFill(g Span, t int64, whole bool) (fill [GapDots]float32, next int64) {
	switch {
	case !HasGap(g):
		return fill, Never
	case t < g.BeginMs:
		return fill, g.BeginMs
	case t >= g.EndMs:
		return [GapDots]float32{1, 1, 1}, Never
	}
	third := float64(g.EndMs-g.BeginMs) / GapDots
	e := float64(t - g.BeginMs)
	if whole {
		lit := min(int(e/third), GapDots-1)
		for j := 0; j <= lit; j++ {
			fill[j] = 1
		}
		if lit == GapDots-1 {
			return fill, g.EndMs
		}
		return fill, g.BeginMs + int64(math.Ceil(float64(lit+1)*third))
	}
	q := math.Floor(e/GapStepMs) * GapStepMs
	for j := range fill {
		fill[j] = float32(min(max((q-float64(j)*third)/third, 0), 1))
	}
	return fill, min(g.BeginMs+int64(q)+GapStepMs, g.EndMs)
}
