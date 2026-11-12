package lyricsync

import (
	"testing"

	"github.com/coredev-uk/sona-go/internal/services"
)

func TestSpanCacheWorksOutSpansOncePerLyrics(t *testing.T) {
	lines := []services.LyricLine{{BeginMs: 0, EndMs: 1000}, {BeginMs: 1000, EndMs: 2000}}
	var c SpanCache
	first := c.Of(lines)
	if len(first) != 2 {
		t.Fatalf("%d spans, want 2", len(first))
	}
	if again := c.Of(lines[:]); &again[0] != &first[0] {
		t.Error("the same lines were worked out again")
	}
	other := []services.LyricLine{{BeginMs: 0, EndMs: 500}}
	if got := c.Of(other); len(got) != 1 {
		t.Errorf("other lyrics gave %d spans, want 1", len(got))
	}
	if SameLines(nil, nil) {
		t.Error("no lines are never the same lyrics")
	}
}

func TestSpanCacheWorksOutLyricsOfTheSameLengthInAnotherArray(t *testing.T) {
	a := []services.LyricLine{{BeginMs: 0, EndMs: 1000}, {BeginMs: 1000, EndMs: 2000}}
	b := []services.LyricLine{{BeginMs: 0, EndMs: 3000}, {BeginMs: 3000, EndMs: 4000}}
	var c SpanCache
	first := c.Of(a)
	second := c.Of(b)
	if SameLines(a, b) {
		t.Fatal("two arrays of the same length are the same lyrics")
	}
	if first[0] == second[0] || second[0].EndMs != 3000 {
		t.Errorf("the other lyrics got the first's spans: %v then %v", first, second)
	}
}
