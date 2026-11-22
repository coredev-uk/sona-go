package kit

import (
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/lyrics"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// liftOf is the lift, in em, of letter i of word k of s at t, its line sung
// until until.
func liftOf(s *SweepText, k, i int, t, until time.Duration) (float32, bool) {
	return s.words[k].lift(&s.words[k].letters[i], t, until, s.size*0.5, false)
}

func fourLetters(t *testing.T, begin, end time.Duration, bg bool) *SweepText {
	t.Helper()
	useAppFont(t)
	return NewSweepText([]lyrics.Word{{Begin: begin, End: end, Text: "HHHH", Background: bg}}, "", 40, 700, 48, 400)
}

// A letter starts to rise as the colour reaches it and is up riseDur later,
// whatever the word's length: letter by letter, in a wave, with no lag.
func TestLettersRiseAsTheColourReachesThem(t *testing.T) {
	s := fourLetters(t, time.Second, 2*time.Second, false)
	w := &s.words[0]
	edge := s.size * 0.5
	if len(w.letters) != 4 {
		t.Fatalf("%d letters", len(w.letters))
	}
	// The front of the fill is at s/d·(length + edge) along the word.
	at := func(front float32) time.Duration {
		return time.Second + time.Duration(front/(w.total+edge)*float32(w.d))
	}
	for i := range w.letters {
		l := &w.letters[i]
		reach := at(l.l)
		if v, _ := liftOf(s, 0, i, reach-5*ms, NoUntil); v != 0 {
			t.Errorf("letter %d is %v em up before the colour reaches it", i, v)
		}
		if v, _ := liftOf(s, 0, i, reach+riseDur/4, NoUntil); v < liftEm/2 {
			t.Errorf("letter %d is only %v of %v up a quarter into its rise: too slow", i, v, liftEm)
		}
		if v, _ := liftOf(s, 0, i, reach+riseDur, NoUntil); !withinTol(v, liftEm, 1e-6) {
			t.Errorf("letter %d is %v em up riseDur after the colour reached it, want %v", i, v, liftEm)
		}
	}
	// A wave: at any time a letter further along is no higher than one before.
	for d := time.Duration(0); d <= time.Second+riseDur; d += 10 * ms {
		prev := float32(1)
		for i := range w.letters {
			v, _ := liftOf(s, 0, i, time.Second+d, NoUntil)
			if v > prev+1e-6 {
				t.Errorf("at %v letter %d (%v) is higher than the one before (%v)", d, i, v, prev)
			}
			prev = v
		}
	}
	// Moving until the last letter is up, then at rest.
	last := at(w.letters[3].l)
	if _, mv := liftOf(s, 0, 3, last+riseDur/2, NoUntil); !mv {
		t.Error("the last letter is not moving while it rises")
	}
	if _, mv := liftOf(s, 0, 3, 2*time.Second+riseDur, NoUntil); mv {
		t.Error("still moving after the last letter is up")
	}
}

// A quick word is up within riseDur of its colour finishing: no lag.
func TestAQuickWordHasRisenRightAfterItsColour(t *testing.T) {
	s := fourLetters(t, time.Second, 1250*ms, false)
	for i := range s.words[0].letters {
		if v, mv := liftOf(s, 0, i, 1250*ms+riseDur, NoUntil); !withinTol(v, liftEm, 1e-6) || mv {
			t.Errorf("letter %d riseDur after a 250 ms word: %v up, moving %v", i, v, mv)
		}
	}
	if _, mv := liftOf(s, 0, 0, 1100*ms, NoUntil); !mv {
		t.Error("not moving while the colour sweeps")
	}
}

// A slow word's letters do not creep up with its fill: each comes up in
// riseDur once the colour reaches it, so the first letter of a word held 3 s
// is up long before the last begins.
func TestASlowWordsLettersComeUpQuickly(t *testing.T) {
	s := fourLetters(t, 0, 3*time.Second, false)
	if v, _ := liftOf(s, 0, 0, riseDur, NoUntil); !withinTol(v, liftEm, 1e-6) {
		t.Errorf("the first letter riseDur into a 3 s word: %v, want up", v)
	}
	if v, _ := liftOf(s, 0, 3, 700*ms, NoUntil); v != 0 {
		t.Errorf("the last letter is up (%v) before the colour reaches it", v)
	}
}

func TestBackgroundLettersRiseTwiceAsFar(t *testing.T) {
	s := fourLetters(t, 0, 500*ms, true)
	if v, _ := liftOf(s, 0, 2, 2*time.Second, NoUntil); !withinTol(v, 2*liftEm, 1e-6) {
		t.Errorf("a background letter rests %v em up, want %v", v, 2*liftEm)
	}
}

// When the line stops being sung the letters settle, one after another from the
// left, each over settleDur, from where they were.
func TestLettersSettleLeftToRightWhenTheLineStops(t *testing.T) {
	s := fourLetters(t, 0, 500*ms, false)
	until := 5 * time.Second
	for i := range s.words[0].letters {
		if v, mv := liftOf(s, 0, i, until, until); !withinTol(v, liftEm, 1e-6) || mv {
			t.Errorf("letter %d as the line stops: %v up, moving %v", i, v, mv)
		}
	}
	first := s.words[0].letters[0].delay
	last := s.words[0].letters[3].delay
	if first != 0 || last <= first || last > settleStaggerMax {
		t.Fatalf("settle delays %v … %v", first, last)
	}
	// Half way through the first letter's settle, it is down a way while the
	// last has not begun.
	mid := until + settleDur/2
	v0, mv0 := liftOf(s, 0, 0, mid, until)
	if v0 <= 0 || v0 >= liftEm || !mv0 {
		t.Errorf("the first letter half way down: %v moving %v", v0, mv0)
	}
	if want, _ := liftOf(s, 0, 3, until+last/2, until); !withinTol(want, liftEm, 1e-6) {
		t.Errorf("the last letter before its turn: %v, want it still up", want)
	}
	// Each is back at 0 at its own time, and the line is at rest after maxSettle.
	for i := range s.words[0].letters {
		d := s.words[0].letters[i].delay
		if v, mv := liftOf(s, 0, i, until+d+settleDur, until); v != 0 || mv {
			t.Errorf("letter %d after its settle: %v moving %v", i, v, mv)
		}
	}
	if s.lifts(SweepClock{T: until + maxSettle + ms, Until: until}) {
		t.Error("a settled line is not painted whole")
	}
	if !s.lifts(SweepClock{T: until + maxSettle - 50*ms, Until: until}) {
		t.Error("a line still settling is painted whole")
	}
	// Quick: most of the way down in the first third of the settle, then a
	// soft landing.
	early, _ := liftOf(s, 0, 0, until+settleDur/3, until)
	if early > liftEm/2 {
		t.Errorf("the settle is slow: %v of %v left after a third", early, liftEm)
	}
}

// Stopping partway through a word: the letters not yet risen stay down, those
// risen settle from where they are.
func TestSettlingStartsFromWhereTheLettersWere(t *testing.T) {
	s := fourLetters(t, time.Second, 1900*ms, false)
	until := 1050 * ms // the first letter is part way up
	v0, _ := liftOf(s, 0, 0, until, until)
	if v0 <= 0 || v0 >= liftEm {
		t.Fatalf("the first letter at the stop: %v", v0)
	}
	if v, _ := liftOf(s, 0, 3, until+200*ms, until); v != 0 {
		t.Errorf("a letter the front never reached rises (%v) after the line stopped", v)
	}
	if v, _ := liftOf(s, 0, 0, until+100*ms, until); v <= 0 || v >= v0 {
		t.Errorf("settling from %v: %v after 100 ms", v0, v)
	}
	// A word that never began never moves.
	if v, mv := liftOf(s, 0, 0, 3*time.Second, 500*ms); v != 0 || mv {
		t.Errorf("a word the line stopped before: %v moving %v", v, mv)
	}
}

// A swelling letter's lift is the front's plus the swell's own offset, and
// the swell finishes after the line stops, the lift settling then.
func TestSwellComesOnTopOfTheLift(t *testing.T) {
	useAppFont(t)
	s := NewSweepText([]lyrics.Word{{Begin: 0, End: 2 * time.Second, Text: "tide"}}, "", 40, 600, 48, 400)
	w := &s.words[0]
	if w.emp == nil {
		t.Fatal("no swell")
	}
	run := w.emp.run
	// Half way through the word the first letters are up by the front, and
	// the swell adds its own rise on top.
	if v, _ := liftOf(s, 0, 0, time.Second, NoUntil); !withinTol(v, liftEm, 1e-6) {
		t.Errorf("the first letter's front lift %v", v)
	}
	if pose := w.emp.pose(0, time.Second); pose.dy >= 0 || pose.scale <= 1 {
		t.Errorf("the swell's own pose %+v", pose)
	}
	// The line stops at 1 s: the swell runs on, the letters stay up until it is
	// over, then settle.
	until := time.Second
	if v, _ := liftOf(s, 0, 0, run-10*ms, until); !withinTol(v, liftEm, 1e-6) {
		t.Errorf("before the swell's end: lift %v", v)
	}
	if v, mv := liftOf(s, 0, 0, run+settleDur/2, until); v <= 0 || v >= liftEm || !mv {
		t.Errorf("settling after the swell: %v moving %v", v, mv)
	}
	if v, mv := liftOf(s, 0, 3, run+maxSettle, until); v != 0 || mv {
		t.Errorf("settled: %v moving %v", v, mv)
	}
	if !s.lifts(SweepClock{T: run + maxSettle - 10*ms, Until: until}) || s.lifts(SweepClock{T: run + maxSettle + 10*ms, Until: until}) {
		t.Error("the text is not painted whole exactly once the swollen word has settled")
	}
}

// Letters are told apart in every word, and a word whose letters join keeps
// one unit per box that rises with the same front.
func TestWordsWhoseLettersJoinRiseAsOneUnit(t *testing.T) {
	useAppFont(t)
	s := NewSweepText([]lyrics.Word{{Begin: 0, End: time.Second, Text: "שלום"}}, "he", 30, 600, 38, 400)
	w := &s.words[0]
	if len(w.letters) != len(w.boxes) || w.letters[0].glyphs != nil || w.emp != nil {
		t.Fatalf("%d units for %d boxes, glyphs %v, swell %v", len(w.letters), len(w.boxes), w.letters[0].glyphs != nil, w.emp != nil)
	}
	if v, _ := liftOf(s, 0, 0, 2*time.Second, NoUntil); !withinTol(v, liftEm, 1e-6) {
		t.Errorf("a right-to-left word rests %v up", v)
	}
}

// The wave reads right to left under rtl: the letter at the right goes first.
func TestRightToLeftFillRaisesFromTheRight(t *testing.T) {
	s := fourLetters(t, 0, time.Second, false)
	w := &s.words[0]
	edge := s.size * 0.5
	early := 150 * ms
	first := w.rise(&w.letters[0], early, edge, true)
	last := w.rise(&w.letters[3], early, edge, true)
	if !(last > first) {
		t.Errorf("rise at the left %v, at the right %v", first, last)
	}
}

// Under reduced motion nothing lifts: the painted text is the whole text at
// every time, and nothing moves.
func TestStillTextNeverLiftsALetter(t *testing.T) {
	s := fourLetters(t, 0, time.Second, false)
	for _, at := range []time.Duration{0, 300 * ms, time.Second, 10 * time.Second} {
		if s.lifts(SweepClock{T: at, Until: NoUntil, Still: true}) {
			t.Errorf("a still text lifts at %v", at)
		}
	}
}

// While a letter rises it is drawn at fractional offsets, so its ink moves in
// sub-pixel steps: more distinct positions than the whole pixels it climbs.
func TestARisingLetterMovesInSubPixelSteps(t *testing.T) {
	useAppFont(t)
	theme.SetMode(theme.ModeLight)
	defer theme.SetMode(theme.ModeSystem)
	const size = 56 // 0.05 em is 2.8 px
	s := NewSweepText([]lyrics.Word{{Begin: 0, End: 900 * ms, Text: "HHHH"}}, "", size, 700, 70, 400)
	w := &s.words[0]
	at := time.Duration(0)
	tt := ui.NewTester(func(c *ui.Context) {
		theme.Apply(c)
		ui.Box(c).Padding(10).Children(func() {
			ui.Box(c.Key("s")).Size(s.Width(), s.Height()).Draw(func(p *ui.Painter, r ui.Rect) {
				s.Paint(p, r.X, r.Y, ui.RGB(0, 0, 0), ui.Color{}, SweepClock{T: at, Until: NoUntil}, false)
			})
		})
	}, 420, 100)
	// The ink-weighted vertical centre of the first letter.
	l := w.letters[0]
	x0, x1 := 10+int(l.x), 10+int(l.x+l.w)
	centre := func() float64 {
		img := tt.Image()
		var sum, mass float64
		for y := 0; y < img.Bounds().Dy(); y++ {
			for x := x0; x < x1; x++ {
				d := 255 - float64(img.RGBAAt(x, y).R)
				sum += d * float64(y)
				mass += d
			}
		}
		return sum / mass
	}
	at = -time.Second
	tt.Frame()
	rest := centre()
	seen := map[float64]bool{}
	for s := time.Duration(0); s <= 600*ms; s += 10 * ms {
		at = s
		tt.Frame()
		seen[math.Round((rest-centre())*100)/100] = true
	}
	at = 2 * time.Second
	tt.Frame()
	if up := rest - centre(); up < 2 || up > 3.6 {
		t.Errorf("the letter rests %.2f px up, want about 2.8", up)
	}
	if len(seen) < 8 {
		t.Errorf("the letter took %d distinct heights on its way up, want sub-pixel steps (more than the 3 whole pixels it climbs)", len(seen))
	}
}

// At rest, raised or not, a letter is as sharp as ever: the share of
// half-covered pixels of a sung line at rest is that of the whole text.
func TestLettersAtRestStaySharp(t *testing.T) {
	useAppFont(t)
	theme.SetMode(theme.ModeDark)
	defer theme.SetMode(theme.ModeSystem)
	s := NewSweepText([]lyrics.Word{{Begin: 0, End: 400 * ms, Text: "Hamburg "}, {Begin: 5 * time.Second, End: 5400 * ms, Text: "fonts"}}, "", 40, 650, 50, 400)
	at := time.Duration(-time.Second)
	tt := ui.NewTester(func(c *ui.Context) {
		theme.Apply(c)
		ui.Box(c).Padding(10).Children(func() {
			ui.Box(c.Key("s")).Size(s.Width(), s.Height()).Draw(func(p *ui.Painter, r ui.Rect) {
				p.Fill(r, ui.RGB(0, 0, 0), 0)
				s.Paint(p, r.X, r.Y, ui.RGB(255, 255, 255), ui.Color{}, SweepClock{T: at, Until: NoUntil}, false)
			})
		})
	}, 420, 80)
	soft := func(t0 time.Duration) float64 {
		at = t0
		tt.Frame()
		img := tt.Image()
		ink, edge := 0, 0
		for i := 0; i < len(img.Pix); i += 4 {
			switch l := img.Pix[i]; {
			case l > 200:
				ink++
			case l > 40:
				edge++
			}
		}
		return float64(edge) / float64(ink)
	}
	flat := soft(-time.Second)
	for _, c := range []struct {
		name string
		at   time.Duration
	}{{"every letter raised", 6 * time.Second}, {"one word raised, one not begun", 2 * time.Second}} {
		if got := soft(c.at); got > flat*1.1 {
			t.Errorf("%s: %.3f of the ink is half-covered, %.3f when flat", c.name, got, flat)
		}
	}
}
