package lyricsync

import (
	"math/rand/v2"
	"testing"
)

// SungUntil is the end of the stretch IsSinging says a line is sung in: for
// any lines, a line is sung exactly from its begin to SungUntil.
func TestSungUntilIsWhereIsSingingStops(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 11))
	for round := range 400 {
		n := 1 + rng.IntN(12)
		spans := make([]Span, n)
		at := int64(0)
		for i := range spans {
			at += int64(rng.IntN(6)) * 1000 * int64(rng.IntN(2)) // ties, closely and widely spaced
			at += int64(rng.IntN(1500))
			spans[i].BeginMs = at
			switch rng.IntN(5) {
			case 0:
				spans[i].EndMs = at // no end
			default:
				spans[i].EndMs = at + int64(rng.IntN(9000)) // may run on under the next lines
			}
		}
		last := int64(0)
		for _, s := range spans {
			last = max(last, s.EndMs+12000)
		}
		for i := range spans {
			until := SungUntil(spans, i)
			for ms := spans[i].BeginMs; ms <= last; ms += 25 {
				want := ms < until
				if got := IsSinging(spans, ms, i); got != want {
					t.Fatalf("round %d, spans %v, line %d at %d: IsSinging %v, but SungUntil is %d", round, spans, i, ms, got, until)
				}
			}
		}
	}
}

func TestSungUntilOfTheLines(t *testing.T) {
	spans := []Span{
		{BeginMs: 1000, EndMs: 4000},   // 0: a break follows (9000-4000 ≥ 3 s): rests 500 ms after its end
		{BeginMs: 9000, EndMs: 12000},  // 1: the next begins at 12200, close: lit until it does
		{BeginMs: 12200, EndMs: 15000}, // 2: runs on under line 3
		{BeginMs: 13000, EndMs: 16000}, // 3: last
	}
	for i, want := range []int64{4500, 12200, 15000, 16500} {
		if got := SungUntil(spans, i); got != want {
			t.Errorf("line %d is sung until %d, want %d", i, got, want)
		}
	}
	// A last line with no end is sung to the end of the song.
	if got := SungUntil([]Span{{BeginMs: 1000, EndMs: 1000}}, 0); got != Never {
		t.Errorf("no end: %d, want Never", got)
	}
}
