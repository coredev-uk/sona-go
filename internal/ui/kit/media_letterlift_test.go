package kit

import (
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/lyrics"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// liftOf is the lift, in em, of letter i of word k of s at t.
func liftOf(s *SweepText, k, i int, t time.Duration) (float32, bool) {
	return s.words[k].lift(&s.words[k].letters[i], t, s.size*0.5, false)
}

// eightLetters is a word of eight letters, too long to swell however long it
// is held (its letters lift plainly); fourLetters one of four, which swells
// when held for a second or more.
func eightLetters(t *testing.T, begin, end time.Duration, bg bool) *SweepText {
	t.Helper()
	useAppFont(t)
	return NewSweepText([]lyrics.Word{{Begin: begin, End: end, Text: "HHHHHHHH", Background: bg}}, "", 40, 700, 48, 400)
}

func fourLetters(t *testing.T, begin, end time.Duration, bg bool) *SweepText {
	t.Helper()
	useAppFont(t)
	return NewSweepText([]lyrics.Word{{Begin: begin, End: end, Text: "HHHH", Background: bg}}, "", 40, 700, 48, 400)
}

// A letter is at 0 before the colour reaches it, rises to the full lift while
// it is lit and is back at exactly 0 within fallDur of being lit whole: long
// before the line's end.
func TestALetterRisesAsTheColourReachesItAndComesStraightBackDown(t *testing.T) {
	s := eightLetters(t, time.Second, 3*time.Second, false)
	w := &s.words[0]
	edge := s.size * 0.5
	if len(w.letters) != 8 {
		t.Fatalf("%d letters", len(w.letters))
	}
	for i := range w.letters {
		l := &w.letters[i]
		reach := time.Second + w.reach(l, edge, false)
		lit := time.Second + w.lit(l, edge, false)
		if lit <= reach {
			t.Fatalf("letter %d is lit (%v) no later than reached (%v)", i, lit, reach)
		}
		if v, mv := liftOf(s, 0, i, reach-5*ms); v != 0 || mv {
			t.Errorf("letter %d is %v em up (moving %v) before the colour reaches it", i, v, mv)
		}
		if v, _ := liftOf(s, 0, i, reach+riseDur/4); v < liftEm/2 {
			t.Errorf("letter %d is only %v of %v up a quarter into its rise: too slow", i, v, liftEm)
		}
		if v, mv := liftOf(s, 0, i, lit); !withinTol(v, liftEm, 1e-6) || !mv {
			t.Errorf("letter %d is %v em up (moving %v) as it is lit whole, want %v", i, v, mv, liftEm)
		}
		if v, mv := liftOf(s, 0, i, lit+fallDur); v != 0 || mv {
			t.Errorf("letter %d is %v em up (moving %v) fallDur after it is lit", i, v, mv)
		}
		if v, _ := liftOf(s, 0, i, lit+fallDur/3); v <= 0 || v >= liftEm/2 {
			t.Errorf("letter %d is %v em up a third into its fall: not on its way down", i, v)
		}
		if lit+fallDur >= 3*time.Second+fallDur+ms {
			t.Errorf("letter %d is down at %v, after the word's end plus a fall", i, lit+fallDur)
		}
	}
	// Rest is exactly 0, at every time before and after the word.
	for _, at := range []time.Duration{0, 999 * ms, 3*time.Second + fallDur, 20 * time.Second} {
		for i := range w.letters {
			if v, mv := liftOf(s, 0, i, at); v != 0 || mv {
				t.Errorf("letter %d at %v: %v up, moving %v", i, at, v, mv)
			}
		}
	}
}

// The stagger comes from the colour's reach: a letter further along the word
// peaks later, and a letter never rises above the lift.
func TestLettersLaterInAWordPeakLater(t *testing.T) {
	s := eightLetters(t, time.Second, 3*time.Second, false)
	w := &s.words[0]
	peak := func(i int) time.Duration {
		best, at := float32(-1), time.Duration(0)
		for d := time.Duration(0); d <= 3*time.Second+fallDur; d += ms {
			v, _ := liftOf(s, 0, i, time.Second+d)
			if v > liftEm+1e-6 {
				t.Fatalf("letter %d is %v em up: above %v", i, v, liftEm)
			}
			if v > best {
				best, at = v, d
			}
		}
		return at
	}
	prev := time.Duration(-1)
	for i := range w.letters {
		p := peak(i)
		if p <= prev {
			t.Errorf("letter %d peaks at %v, not after the one before (%v)", i, p, prev)
		}
		prev = p
	}
}

// A quick word's letters are all up and down again within riseDur + fallDur
// of its colour finishing: no lag behind the voice.
func TestAQuickWordIsDownRightAfterItsColour(t *testing.T) {
	s := fourLetters(t, time.Second, 1250*ms, false)
	for i := range s.words[0].letters {
		if v, mv := liftOf(s, 0, i, 1250*ms+fallDur); v != 0 || mv {
			t.Errorf("letter %d fallDur after a 250 ms word: %v up, moving %v", i, v, mv)
		}
	}
	if _, mv := liftOf(s, 0, 0, 1100*ms); !mv {
		t.Error("not moving while the colour sweeps")
	}
}

// A slow word's letters do not creep up with its fill: each comes up in
// riseDur once the colour reaches it, so the first letter of a word held 3 s
// is up long before the last begins.
func TestASlowWordsLettersComeUpQuickly(t *testing.T) {
	s := eightLetters(t, 0, 3*time.Second, false)
	if v, _ := liftOf(s, 0, 0, riseDur); !withinTol(v, liftEm, 1e-3) {
		t.Errorf("the first letter riseDur into a 3 s word: %v, want up", v)
	}
	if v, _ := liftOf(s, 0, 3, 700*ms); v != 0 {
		t.Errorf("the last letter is up (%v) before the colour reaches it", v)
	}
}

func TestBackgroundLettersRiseTwiceAsFar(t *testing.T) {
	s := eightLetters(t, 0, 2*time.Second, true)
	l := &s.words[0].letters[0]
	if v, _ := liftOf(s, 0, 0, s.words[0].lit(l, s.size*0.5, false)); !withinTol(v, 2*liftEm, 1e-6) {
		t.Errorf("a background letter peaks %v em up, want %v", v, 2*liftEm)
	}
}

// A letter lit before it has quite come up falls from where it got to: a very
// short word peaks lower.
func TestAVeryShortWordPeaksLower(t *testing.T) {
	s := fourLetters(t, 0, 40*ms, false)
	best := float32(0)
	for d := time.Duration(0); d <= 400*ms; d += ms {
		v, _ := liftOf(s, 0, 0, d)
		best = max(best, v)
	}
	if best <= 0 || best >= liftEm*0.9 {
		t.Errorf("a letter of a 40 ms word peaks at %v em, want under %v", best, liftEm)
	}
}

// A swelling word's letters float in the swell's bump alone, which comes back
// to 0: the front's lift is not on top of it, and nothing is left up when the
// swell has run.
func TestASwollenLettersFloatReturnsToRest(t *testing.T) {
	useAppFont(t)
	s := NewSweepText([]lyrics.Word{{Begin: 0, End: 2 * time.Second, Text: "tide"}}, "", 40, 600, 48, 400)
	w := &s.words[0]
	if w.emp == nil {
		t.Fatal("no swell")
	}
	run := w.emp.run
	for i := range w.letters {
		for _, at := range []time.Duration{0, time.Second, run - ms, run + ms} {
			if v, mv := liftOf(s, 0, i, at); v != 0 || mv {
				t.Errorf("letter %d has a lift of %v (moving %v) at %v besides the swell's", i, v, mv, at)
			}
		}
		d := w.emp.delay(i)
		if p := w.emp.pose(i, d+time.Duration(float64(w.emp.du)*0.7)); p.dy > -liftEm {
			t.Errorf("letter %d at the bump's peak: dy %v, want up by at least %v", i, p.dy, liftEm)
		}
		if p := w.emp.pose(i, d+time.Duration(float64(w.emp.du)*1.4)); !withinTol(p.dy, 0, 1e-6) {
			t.Errorf("letter %d is %v em up when its float has run", i, -p.dy)
		}
	}
	if !s.lifts(SweepClock{T: run - 10*ms}) || s.lifts(SweepClock{T: run + 10*ms}) {
		t.Error("the text is not painted whole exactly once the swell has run")
	}
}

// lifts turns false shortly after the last letter is down, while the line is
// still the current one: no frames are asked for a finished line.
func TestLiftsStopsOnceTheLastLetterIsDown(t *testing.T) {
	s := eightLetters(t, time.Second, 2*time.Second, false)
	w := &s.words[0]
	edge := s.size * 0.5
	last := time.Second + w.lit(&w.letters[7], edge, false) + fallDur
	if !s.lifts(SweepClock{T: last - 20*ms, Sung: 1}) {
		t.Error("a letter still falling is painted whole")
	}
	if s.lifts(SweepClock{T: last + 20*ms, Sung: 1}) {
		t.Error("a line whose letters are down still lifts")
	}
	if s.lifts(SweepClock{T: 30 * time.Second, Sung: 1}) {
		t.Error("a finished line lifts long after")
	}
	if s.lifts(SweepClock{T: 900 * ms, Sung: 1}) {
		t.Error("a line lifts before it begins")
	}
}

// Letters are told apart in every word, and a word whose letters join keeps
// one unit per box that rises with the same front and falls with it.
func TestWordsWhoseLettersJoinRiseAsOneUnit(t *testing.T) {
	useAppFont(t)
	s := NewSweepText([]lyrics.Word{{Begin: 0, End: time.Second, Text: "שלום"}}, "he", 30, 600, 38, 400)
	w := &s.words[0]
	if len(w.letters) != len(w.boxes) || w.letters[0].glyphs != nil || w.emp != nil {
		t.Fatalf("%d units for %d boxes, glyphs %v, swell %v", len(w.letters), len(w.boxes), w.letters[0].glyphs != nil, w.emp != nil)
	}
	if v, _ := w.lift(&w.letters[0], 500*ms, s.size*0.5, true); v <= 0 {
		t.Errorf("a right-to-left word is %v up mid-word", v)
	}
	if v, mv := w.lift(&w.letters[0], 2*time.Second, s.size*0.5, true); v != 0 || mv {
		t.Errorf("a right-to-left word rests %v up (moving %v)", v, mv)
	}
}

// The wave reads right to left under rtl: the letter at the right goes first.
func TestRightToLeftFillRaisesFromTheRight(t *testing.T) {
	s := fourLetters(t, 0, 900*ms, false)
	w := &s.words[0]
	edge := s.size * 0.5
	early := 150 * ms
	first, _ := w.lift(&w.letters[0], early, edge, true)
	last, _ := w.lift(&w.letters[3], early, edge, true)
	if !(last > first) {
		t.Errorf("lift at the left %v, at the right %v", first, last)
	}
}

// Under reduced motion nothing lifts: the painted text is the whole text at
// every time, and nothing moves.
func TestStillTextNeverLiftsALetter(t *testing.T) {
	s := fourLetters(t, 0, 900*ms, false)
	for _, at := range []time.Duration{0, 300 * ms, time.Second, 10 * time.Second} {
		if s.lifts(SweepClock{T: at, Still: true}) {
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
				s.Paint(p, r.X, r.Y, ui.RGB(0, 0, 0), ui.Color{}, SweepClock{T: at}, false)
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
	peak := 0.0
	for h := range seen {
		peak = max(peak, h)
	}
	if peak < 2 || peak > 3.6 {
		t.Errorf("the letter peaks %.2f px up, want about 2.8", peak)
	}
	at = 2 * time.Second
	tt.Frame()
	if up := rest - centre(); math.Abs(up) > 0.01 {
		t.Errorf("the letter rests %.2f px up, want 0", up)
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
				s.Paint(p, r.X, r.Y, ui.RGB(255, 255, 255), ui.Color{}, SweepClock{T: at}, false)
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
