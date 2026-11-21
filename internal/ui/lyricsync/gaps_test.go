package lyricsync

import "testing"

func TestGapsAreStretchesOfFiveSecondsOrMoreWithNoLineSung(t *testing.T) {
	spans := []Span{
		{BeginMs: 6_000, EndMs: 9_000},   // starts 6 s in: a gap before the first line
		{BeginMs: 13_999, EndMs: 16_000}, // 4.999 s after the line before: none
		{BeginMs: 21_000, EndMs: 24_000}, // exactly 5 s after: a gap
		{BeginMs: 23_000, EndMs: 25_000}, // runs into the line before: none
		{BeginMs: 40_000, EndMs: 41_000}, // after the longest end so far (25 s): a gap
	}
	got := Gaps(spans)
	want := []Span{{0, 6_000}, {}, {16_000, 21_000}, {}, {25_000, 40_000}}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("gap before line %d: %+v, want %+v", i, got[i], want[i])
		}
	}
	for i, g := range got {
		if HasGap(g) != (want[i] != Span{}) {
			t.Errorf("HasGap(%+v) is %v", g, HasGap(g))
		}
	}
}

func TestASongThatStartsWithinFiveSecondsHasNoGapBeforeItsFirstLine(t *testing.T) {
	if g := Gaps([]Span{{BeginMs: 4_999, EndMs: 6_000}}); HasGap(g[0]) {
		t.Errorf("a gap before a line at 4.999 s: %+v", g[0])
	}
	if g := Gaps(nil); len(g) != 0 {
		t.Errorf("gaps of no lines: %+v", g)
	}
}

func TestALineWithNoEndLeavesNoGapUntilTheNextLineIsFiveSecondsAfterItsBegin(t *testing.T) {
	got := Gaps([]Span{{BeginMs: 1_000}, {BeginMs: 7_000, EndMs: 8_000}})
	if !HasGap(got[1]) || got[1] != (Span{1_000, 7_000}) {
		t.Errorf("gap %+v", got[1])
	}
}

func TestTheDotsFillOneOverEachThirdOfTheGapInSteps(t *testing.T) {
	g := Span{BeginMs: 10_000, EndMs: 16_000} // thirds of 2 s
	at := func(ms int64) [GapDots]float32 {
		f, _ := GapFill(g, ms, false)
		return f
	}
	if f := at(9_999); f != [GapDots]float32{} {
		t.Errorf("before the gap: %v", f)
	}
	if f := at(10_000); f != [GapDots]float32{} {
		t.Errorf("at its start: %v", f)
	}
	// Half way through the first third, to the step: 1000 ms is 12 steps of 83.
	f := at(11_000)
	if f[0] < 0.4 || f[0] > 0.5 || f[1] != 0 || f[2] != 0 {
		t.Errorf("half way through the first third: %v", f)
	}
	if f := at(12_300); f[0] != 1 || f[1] <= 0 || f[1] >= 0.3 || f[2] != 0 {
		t.Errorf("a little into the second third: %v", f)
	}
	if f := at(14_500); f[0] != 1 || f[1] != 1 || f[2] <= 0 || f[2] >= 1 {
		t.Errorf("in the last third: %v", f)
	}
	if f := at(15_999); f[0] != 1 || f[1] != 1 || f[2] < 0.9 {
		t.Errorf("at the end: %v", f)
	}
	if f := at(16_000); f != [GapDots]float32{1, 1, 1} {
		t.Errorf("after the gap: %v", f)
	}
	// Stepped: nothing changes within a step.
	a, next := GapFill(g, 11_000, false)
	if next <= 11_000 || next > 11_000+GapStepMs {
		t.Errorf("the next step is at %d, want within %d ms of 11000", next, GapStepMs)
	}
	if b, _ := GapFill(g, next-1, false); b != a {
		t.Errorf("the fill moved inside a step: %v then %v", a, b)
	}
	if b, _ := GapFill(g, next, false); b == a {
		t.Errorf("the fill did not move at the next step: %v", b)
	}
}

func TestTheDotsFillAboutTwelveTimesASecond(t *testing.T) {
	g := Span{BeginMs: 0, EndMs: 30_000}
	steps, prev := 0, [GapDots]float32{}
	for ms := int64(0); ms < 3_000; ms++ {
		f, _ := GapFill(g, ms, false)
		if f != prev {
			steps++
			prev = f
		}
	}
	if steps < 34 || steps > 37 {
		t.Errorf("%d changes in 3 s, want about 36", steps)
	}
}

func TestUnderReducedMotionEachDotLightsWholeAsItsThirdBegins(t *testing.T) {
	g := Span{BeginMs: 10_000, EndMs: 16_000}
	for _, c := range []struct {
		ms   int64
		want [GapDots]float32
		next int64
	}{
		{9_000, [GapDots]float32{}, 10_000},
		{10_000, [GapDots]float32{1, 0, 0}, 12_000},
		{11_999, [GapDots]float32{1, 0, 0}, 12_000},
		{12_000, [GapDots]float32{1, 1, 0}, 14_000},
		{14_500, [GapDots]float32{1, 1, 1}, 16_000},
		{16_000, [GapDots]float32{1, 1, 1}, Never},
	} {
		got, next := GapFill(g, c.ms, true)
		if got != c.want || next != c.next {
			t.Errorf("at %d: %v, next %d; want %v, next %d", c.ms, got, next, c.want, c.next)
		}
	}
}

func TestNoGapFillsNothingAndAsksForNothing(t *testing.T) {
	if f, next := GapFill(Span{}, 5_000, false); f != [GapDots]float32{} || next != Never {
		t.Errorf("%v, %d", f, next)
	}
}
