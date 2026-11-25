package kit

import (
	"bytes"
	"fmt"
	"image"
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/lyrics"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

const ms = time.Millisecond

func withinTol(a, b, tol float32) bool { return float32(math.Abs(float64(a-b))) <= tol }

// useAppFont registers the app's font, as the other kit tests that draw
// text do, so that the boxes and the ink these tests measure are the same
// whichever tests ran before.
func useAppFont(t *testing.T) {
	t.Helper()
	if err := theme.RegisterFonts(); err != nil {
		t.Fatal(err)
	}
}

func TestEmphasizesWordsHeldToSwell(t *testing.T) {
	for _, c := range []struct {
		text string
		dur  time.Duration
		want bool
	}{
		{"tide", time.Second, true},
		{"tide", 999 * ms, false},
		{"tide ", 2 * time.Second, true}, // the trailing space is not counted
		{"  tide  ", time.Second, true},
		{"a", 5 * time.Second, false},        // one character
		{"ab", time.Second, true},            // two
		{"abcdefg", time.Second, true},       // seven
		{"abcdefg ", time.Second, true},      // seven and a space
		{"abcdefgh", 5 * time.Second, false}, // eight
		{"", 5 * time.Second, false},
		{"   ", 5 * time.Second, false},
		// CJK: a second is all it takes, of any length.
		{"愛", time.Second, true},
		{"愛", 999 * ms, false},
		{"愛してるって言ってほしい ", 2 * time.Second, true},
		{"ありがとう", time.Second, true},
		{"カタカナカタカナ", time.Second, true},
		{"사랑해요사랑해요", time.Second, true},
		{"愛、", time.Second, true},
		{"愛 a", time.Second, true},         // mixed: three characters, under seven
		{"愛でるabcdefg", time.Second, false}, // mixed and too long for the plain rule
	} {
		if got := emphasizes(c.text, c.dur); got != c.want {
			t.Errorf("emphasizes(%q, %v) = %v, want %v", c.text, c.dur, got, c.want)
		}
	}
}

func TestSwellParametersFollowTheHeldTime(t *testing.T) {
	const tol = 1e-4
	// A second: the weakest.
	w := newSwell(time.Second, 4, false, false)
	if !withinTol(w.amount, 0.6*0.125, tol) || !withinTol(w.blur, 0.5/27, tol) || w.du != time.Second {
		t.Errorf("1 s: amount %v blur %v du %v", w.amount, w.blur, w.du)
	}
	// Shorter than a second counts as a second.
	if v := newSwell(300*ms, 4, false, false); v != w {
		t.Errorf("300 ms: %+v, want a second's %+v", v, w)
	}
	// Two seconds: amount 1 (cubed below 1, rooted above), so 0.6.
	w = newSwell(2*time.Second, 4, false, false)
	if !withinTol(w.amount, 0.6, tol) || !withinTol(w.blur, 0.5*(2.0/3)*(2.0/3)*(2.0/3), tol) {
		t.Errorf("2 s: amount %v blur %v", w.amount, w.blur)
	}
	// Letters follow du/2.5/n apart.
	for i, want := range []time.Duration{0, 200 * ms, 400 * ms, 600 * ms} {
		if got := w.delay(i); got != want {
			t.Errorf("letter %d starts %v in, want %v", i, got, want)
		}
	}
	if got := w.total(); got != 600*ms+2800*ms {
		t.Errorf("a swell of 4 letters over 2 s runs %v", got)
	}
	// The last word of the line is stronger: ×1.6, ×1.5, and 1.2 as long.
	last := newSwell(2*time.Second, 4, true, false)
	if !withinTol(last.amount, 0.96, tol) || !withinTol(last.blur, 0.75*(2.0/3)*(2.0/3)*(2.0/3), tol) || last.du != 2400*ms {
		t.Errorf("last: amount %v blur %v du %v", last.amount, last.blur, last.du)
	}
	// And the caps: amount 1.2, blur 0.8.
	long := newSwell(8*time.Second, 4, true, false)
	if long.amount != 1.2 || long.blur != 0.8 {
		t.Errorf("long last word: amount %v blur %v, want the caps 1.2 and 0.8", long.amount, long.blur)
	}
	if v := newSwell(8*time.Second, 4, false, false); v.amount > 1.2 || v.blur > 0.8 {
		t.Errorf("long word: %v %v above the caps", v.amount, v.blur)
	}
	if got := long.glowEm(); !withinTol(got, 0.24, tol) {
		t.Errorf("glow radius %v em, want min(0.3, 0.3×0.8) = 0.24", got)
	}
}

func TestEmpEasingRisesToOneAndFallsToZero(t *testing.T) {
	if empEasing(0) != 0 || !withinTol(empEasing(0.5), 1, 1e-4) || !withinTol(empEasing(1), 0, 1e-4) {
		t.Errorf("0 → %v, ½ → %v, 1 → %v", empEasing(0), empEasing(0.5), empEasing(1))
	}
	for x := float32(0.05); x < 1; x += 0.05 {
		if v := empEasing(x); v < 0 || v > 1.0001 {
			t.Errorf("empEasing(%v) = %v outside 0..1", x, v)
		}
	}
}

func TestLettersSwellInTurnAndSpreadFromTheCentre(t *testing.T) {
	w := newSwell(2*time.Second, 4, false, false)
	// Before its delay a letter is at rest; the last one's starts 600 ms in.
	if p := w.pose(3, 500*ms); p != (letterPose{scale: 1}) {
		t.Errorf("letter 3 at 500 ms: %+v", p)
	}
	if p := w.pose(0, 500*ms); p.scale <= 1 || p.glow <= 0 {
		t.Errorf("letter 0 at 500 ms: %+v", p)
	}
	// Half way through a letter's swell: the envelope is 1.
	mid := w.pose(0, w.du/2)
	if !withinTol(mid.scale, 1+0.1*w.amount, 1e-3) || !withinTol(mid.glow, w.blur, 1e-3) {
		t.Errorf("at the peak: %+v (amount %v blur %v)", mid, w.amount, w.blur)
	}
	// The letters left of the middle move left, the right ones right, the
	// middle one not at all.
	a, b, c, d := w.pose(0, w.du/2), w.pose(1, w.delay(1)+w.du/2), w.pose(2, w.delay(2)+w.du/2), w.pose(3, w.delay(3)+w.du/2)
	if !(a.dx < b.dx && b.dx < 0 && withinTol(c.dx, 0, 1e-6) && d.dx > 0) {
		t.Errorf("spread %v %v %v %v", a.dx, b.dx, c.dx, d.dx)
	}
	if !withinTol(a.dx, -0.03*w.amount*2, 1e-3) {
		t.Errorf("the first letter's offset %v em", a.dx)
	}
	// The glow, the scale, the spread and the swell's own offset are gone at
	// the end: what is left of the lift is the fill front's, not the swell's.
	end := w.pose(3, w.total())
	if end.scale != 1 || !withinTol(end.glow, 0, 1e-4) || !withinTol(end.dx, 0, 1e-4) || !withinTol(end.dy, 0, 1e-4) {
		t.Errorf("at the end: %+v", end)
	}
	// Half way, a letter rises a little over the lift the front gives it.
	if mid := w.pose(0, w.du/2); mid.dy >= 0 {
		t.Errorf("a swelling letter at its peak is %v em up, want a rise of its own", -mid.dy)
	}
}

// A held word of two to seven characters gets letters; a short one or a long
// one, a wrapped word and a right-to-left one do not.
func TestSweepTextSwellsOnlyWhatIsHeldToSwell(t *testing.T) {
	useAppFont(t)
	words := []lyrics.Word{
		{Begin: 0, End: 400 * ms, Text: "the "},
		{Begin: 400 * ms, End: 3 * time.Second, Text: "tide "},
		{Begin: 3 * time.Second, End: 5 * time.Second, Text: "a "},
		{Begin: 5 * time.Second, End: 7 * time.Second, Text: "returning"},
	}
	s := NewSweepText(words, "", 30, 600, 38, 600)
	for i, want := range []int{0, 4, 0, 0} {
		got := 0
		if s.words[i].emp != nil {
			got = len(s.words[i].letters)
		}
		if got != want {
			t.Errorf("%q has %d letters, want %d", words[i].Text, got, want)
		}
	}
	// The last word of the line (held, but long) is not a swell; the held
	// "tide" is not the last.
	if s.words[1].emp.n != 4 || s.words[1].emp.amount != newSwell(2600*ms, 4, false, false).amount {
		t.Errorf("tide's swell %+v", s.words[1].emp)
	}
	last := NewSweepText([]lyrics.Word{{Begin: 0, End: 2 * time.Second, Text: "tide"}}, "", 30, 600, 38, 600)
	if e := last.words[0].emp; e == nil || e.amount != newSwell(2*time.Second, 4, true, false).amount {
		t.Errorf("the last word gets the stronger swell: %+v", e)
	}
	// Right to left: no letters, whatever its length.
	rtl := NewSweepText([]lyrics.Word{{Begin: 0, End: 2 * time.Second, Text: "שלום"}}, "he", 30, 600, 38, 600)
	if rtl.words[0].emp != nil {
		t.Error("a Hebrew word swells letter by letter")
	}
	// CJK: a letter each.
	cjk := NewSweepText([]lyrics.Word{{Begin: 0, End: 2 * time.Second, Text: "ありがとう"}}, "ja", 30, 600, 38, 600)
	if e := cjk.words[0].emp; e == nil || len(cjk.words[0].letters) != 5 {
		t.Errorf("five kana: %+v", e)
	}
	// Letters tile the word: each starts where the one before ends.
	letters := s.words[1].letters
	box := s.words[1].boxes[0].r
	if !withinTol(letters[0].x, box.X, 0.5) || !withinTol(letters[3].x+letters[3].w, box.X+box.W, 0.5) {
		t.Errorf("letters %v…%v over a box %v…%v", letters[0].x, letters[3].x+letters[3].w, box.X, box.X+box.W)
	}
	for i := 1; i < len(letters); i++ {
		if !withinTol(letters[i].x, letters[i-1].x+letters[i-1].w, 0.5) {
			t.Errorf("letter %d starts at %v after %v", i, letters[i].x, letters[i-1].x+letters[i-1].w)
		}
	}
	// Every word, swelling or not, has its letters, each with glyphs of its own.
	for i, w := range s.words {
		for _, l := range w.letters {
			if l.glyphs == nil {
				t.Errorf("a letter of %q has no glyphs", words[i].Text)
			}
		}
	}
}

// The ink an unmoved word gives is the same as the plain layout's, so
// painting word by word never loses a pixel to a clip: wrapped, italic, with
// descenders and words set close.
func TestPaintingWordByWordLosesNoInk(t *testing.T) {
	useAppFont(t)
	theme.SetMode(theme.ModeLight)
	defer theme.SetMode(theme.ModeSystem)
	words := []lyrics.Word{
		{Begin: time.Second, End: 1500 * ms, Text: "Gypsy "},
		{Begin: 1500 * ms, End: 2 * time.Second, Text: "jumping "},
		{Begin: 2 * time.Second, End: 2500 * ms, Text: "quickly "},
		{Begin: 2500 * ms, End: 3 * time.Second, Text: "fjord"},
		{Begin: 3 * time.Second, End: 3500 * ms, Text: "ly "},
		{Begin: 3500 * ms, End: 4 * time.Second, Text: "pygmy "},
		{Begin: 4 * time.Second, End: 4500 * ms, Text: "ſaffron"},
	}
	for _, italic := range []bool{false, true} {
		var s *SweepText
		if italic {
			s = NewSweepTextItalic(words, "", 32, 650, 40, 260)
		} else {
			s = NewSweepText(words, "", 32, 650, 40, 260)
		}
		if len(s.words[1].boxes) == 0 {
			t.Fatal("no boxes")
		}
		at := time.Duration(0)
		tt := ui.NewTester(func(c *ui.Context) {
			theme.Apply(c)
			ui.Box(c).Padding(10).Children(func() {
				ui.Box(c.Key("s")).Size(s.Width(), s.Height()).Draw(func(p *ui.Painter, r ui.Rect) {
					// Nothing sung: no fill, and until the line begins nothing lifts.
					s.Paint(p, r.X, r.Y, mediaGradBase, mediaGradFill, SweepClock{T: at}, false)
				})
			})
		}, 300, 260)
		flat := func(at0 time.Duration) []byte {
			at = at0
			tt.Frame()
			return append([]byte(nil), tt.Image().Pix...)
		}
		whole := flat(0)
		// At the first word's begin the line is painted word by word, none
		// of them yet away from its place.
		if s.lifts(SweepClock{T: 0}) {
			t.Fatal("lifts before the line begins")
		}
		piece := flat(time.Second)
		if !s.lifts(SweepClock{T: time.Second}) {
			t.Fatal("does not paint by the word as the line begins")
		}
		// Antialiasing differs by a shade at a few pixels where a clip splits
		// a run of glyphs the text system draws joined; ink a clip cut off is
		// a whole pixel's worth of difference, and in numbers.
		lost, first := 0, ""
		for i := range whole {
			d := int(whole[i]) - int(piece[i])
			if d < 0 {
				d = -d
			}
			if d > 100 {
				if lost == 0 {
					first = fmt.Sprintf("x=%d y=%d: %v then %v", i/4%(tt.Image().Stride/4), i/4/(tt.Image().Stride/4), whole[i/4*4:i/4*4+4], piece[i/4*4:i/4*4+4])
				}
				lost++
			}
		}
		if lost > 0 {
			t.Errorf("italic %v: painting by the word changes %d channel values of the picture by more than a shade, the first at %s", italic, lost, first)
		}
	}
}

func rowsWith(img *image.RGBA, x0, x1 int, pred func(r, g, b uint8) bool) (top, bottom int) {
	top, bottom = -1, -1
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := x0; x < x1; x++ {
			c := img.RGBAAt(x, y)
			if pred(c.R, c.G, c.B) {
				if top < 0 {
					top = y
				}
				bottom = y
				break
			}
		}
	}
	return top, bottom
}

func isRed(r, g, b uint8) bool { return r > 180 && g < 100 && b < 100 }

// The glyphs really move: a word's first letter, lit whole, stands 0.05em
// higher than at rest, the line's other words stay where they are, and the
// word is back where it was once its letters are down.
func TestLiftedInkStandsHigher(t *testing.T) {
	useAppFont(t)
	theme.SetMode(theme.ModeLight)
	defer theme.SetMode(theme.ModeSystem)
	words := []lyrics.Word{
		{Begin: 0, End: 300 * ms, Text: "HHHH "},
		{Begin: 5 * time.Second, End: 5300 * ms, Text: "HHHH"},
	}
	const size = 80 // 0.05 em = 4 px
	s := NewSweepText(words, "", size, 700, 100, 700)
	at := time.Duration(0)
	var moving bool
	tt := ui.NewTester(func(c *ui.Context) {
		theme.Apply(c)
		ui.Box(c).Padding(10).Children(func() {
			ui.Box(c.Key("s")).Size(s.Width(), s.Height()).Draw(func(p *ui.Painter, r ui.Rect) {
				moving = s.Paint(p, r.X, r.Y, mediaGradBase, mediaGradFill, SweepClock{T: at}, false)
			})
		})
	}, 720, 130)
	frame := func(t0 time.Duration) (first, second [2]int, mv bool) {
		at = t0
		tt.Frame()
		img := tt.Image()
		mid := int(10 + s.words[1].boxes[0].r.X)
		a, b := rowsWith(img, 0, mid-2, isRed)
		c, d := rowsWith(img, mid, 720, isRed)
		return [2]int{a, b}, [2]int{c, d}, moving
	}
	f0, s0, _ := frame(-time.Second)
	// The first letter is lit whole 110 ms in, and is up by then.
	f1, s1, mv := frame(110 * ms)
	if !mv {
		t.Error("not moving while the first letter is up")
	}
	if got := f0[0] - f1[0]; got < 3 || got > 5 {
		t.Errorf("the lifted word's top moved by %d px, want 4 (0.05 em of %d)", got, size)
	}
	if s0 != s1 {
		t.Errorf("the unsung word moved: %v then %v", s0, s1)
	}
	// Partway up, between.
	f2, _, _ := frame(30 * ms)
	if f2[0] >= f0[0] || f2[0] <= f1[0] {
		t.Errorf("mid-lift top %d not between %d and %d", f2[0], f1[0], f0[0])
	}
	// Long after, the word is back where it began, and nothing moves.
	f3, s3, mv := frame(2 * time.Second)
	if mv || f3 != f0 || s3 != s0 {
		t.Errorf("long after: moving %v, word at %v then %v, other at %v then %v", mv, f0, f3, s0, s3)
	}
}

// Reduced motion: no word lifts, and nothing is asked of the next frame by it.
func TestStillTextDoesNotMove(t *testing.T) {
	useAppFont(t)
	theme.SetMode(theme.ModeLight)
	defer theme.SetMode(theme.ModeSystem)
	words := []lyrics.Word{{Begin: 0, End: 2 * time.Second, Text: "tide "}, {Begin: 2 * time.Second, End: 2400 * ms, Text: "HHHH"}}
	s := NewSweepText(words, "", 60, 700, 70, 500)
	if s.lifts(SweepClock{T: time.Second, Still: true}) {
		t.Error("a still text lifts")
	}
	at := time.Duration(-time.Second)
	var moving bool
	tt := ui.NewTester(func(c *ui.Context) {
		theme.Apply(c)
		ui.Box(c).Padding(10).Children(func() {
			ui.Box(c.Key("s")).Size(s.Width(), s.Height()).Draw(func(p *ui.Painter, r ui.Rect) {
				moving = s.Paint(p, r.X, r.Y, mediaGradBase, mediaGradFill, SweepClock{T: at, Still: true}, false)
			})
		})
	}, 520, 100)
	tt.Frame()
	before := append([]byte(nil), tt.Image().Pix...)
	for _, at0 := range []time.Duration{500 * ms, time.Second, 2100 * ms, 5 * time.Second} {
		at = at0
		tt.Frame()
		if moving {
			t.Errorf("at %v a still text reports it is moving", at0)
		}
		if !bytes.Equal(before, tt.Image().Pix) {
			t.Errorf("at %v a still text moved", at0)
		}
	}
}

// Paint reports a moving text only while a word fills, rises or falls: a
// line at rest, before it, after it or held, asks for no frame.
func TestSweepPaintReportsWhetherItMoves(t *testing.T) {
	useAppFont(t)
	theme.SetMode(theme.ModeLight)
	defer theme.SetMode(theme.ModeSystem)
	words := []lyrics.Word{
		{Begin: time.Second, End: 1300 * ms, Text: "one "},
		{Begin: 1300 * ms, End: 1600 * ms, Text: "two "},
		{Begin: 6 * time.Second, End: 6300 * ms, Text: "three"},
	}
	s := NewSweepText(words, "", 40, 600, 48, 500)
	var clock SweepClock
	var moving bool
	tt := ui.NewTester(func(c *ui.Context) {
		theme.Apply(c)
		ui.Box(c).Padding(10).Children(func() {
			ui.Box(c.Key("s")).Size(s.Width(), s.Height()).Draw(func(p *ui.Painter, r ui.Rect) {
				moving = s.Paint(p, r.X, r.Y, mediaGradBase, mediaGradFill, clock, false)
			})
		})
	}, 520, 70)
	for _, c := range []struct {
		name string
		at   time.Duration
		sung float32
		want bool
	}{
		{"before the line", 0, 0, false},
		{"as it begins", time.Second, 1, true},
		{"filling the first word", 1100 * ms, 1, true},
		{"the first letters are down again", 1900 * ms, 1, false},
		{"both down, between words", 2400 * ms, 1, false},
		{"at rest", 5 * time.Second, 1, false},
		{"the last word begins", 6 * time.Second, 1, true},
		{"the last letters falling", 6400 * ms, 1, true},
		{"the last word down, the line still current", 8 * time.Second, 1, false},
		{"after the line", 9 * time.Second, 0, false},
		{"long after", 60 * time.Second, 0, false},
	} {
		clock = SweepClock{T: c.at, Sung: c.sung}
		tt.Frame()
		if moving != c.want {
			t.Errorf("%s (%v): moving %v, want %v", c.name, c.at, moving, c.want)
		}
	}
	// Before the line, nothing sung: still, and nothing asked of the next frame.
	clock = SweepClock{T: 500 * ms}
	tt.Frame()
	if moving {
		t.Error("moving before the line")
	}
}

// The letters of a swelling word are drawn at their own sizes, so they stay
// sharp: the share of half-covered pixels at the peak is no more than at rest
// (MyGo v0.9.0 blurs what a transform scales).
func TestSwollenLettersStaySharp(t *testing.T) {
	useAppFont(t)
	theme.SetMode(theme.ModeDark)
	defer theme.SetMode(theme.ModeSystem)
	words := []lyrics.Word{{Begin: 0, End: 4 * time.Second, Text: "Hold"}}
	s := NewSweepText(words, "", 60, 700, 72, 400)
	at := time.Duration(-time.Second)
	tt := ui.NewTester(func(c *ui.Context) {
		theme.Apply(c)
		ui.Box(c).Padding(10).Children(func() {
			ui.Box(c.Key("s")).Size(s.Width(), s.Height()).Draw(func(p *ui.Painter, r ui.Rect) {
				p.Fill(r, ui.RGB(0, 0, 0), 0)
				// No fill colour: the letters and their motion alone.
				s.Paint(p, r.X, r.Y, ui.RGB(255, 255, 255), ui.Color{}, SweepClock{T: at, Sung: 1}, false)
			})
		})
	}, 420, 100)
	softness := func(t0 time.Duration) float64 {
		at = t0
		tt.Frame()
		img := tt.Image()
		ink, edge := 0, 0
		for y := 0; y < img.Bounds().Dy(); y++ {
			for x := 0; x < img.Bounds().Dx(); x++ {
				switch l := img.RGBAAt(x, y).R; {
				case l > 200:
					ink++
				case l > 40:
					edge++
				}
			}
		}
		return float64(edge) / float64(ink)
	}
	rest := softness(-time.Second)
	peak := softness(time.Duration(float64(s.words[0].emp.du) * 0.5)) // the first letter at its biggest
	// A swelling letter moves, so it is drawn at fractional offsets and a
	// little softer than at rest; letters at rest are as sharp as ever
	// (TestLettersAtRestStaySharp).
	if peak > rest*2.2 {
		t.Errorf("swollen letters are soft: %.3f of the ink at the edge, %.3f at rest", peak, rest)
	}
	// And the letters did swell: more ink than at rest.
	inkAt := func(t0 time.Duration) int {
		at = t0
		tt.Frame()
		n := 0
		img := tt.Image()
		for i := 0; i < len(img.Pix); i += 4 {
			if img.Pix[i] > 128 {
				n++
			}
		}
		return n
	}
	if big, small := inkAt(time.Duration(float64(s.words[0].emp.du)*0.5)), inkAt(-time.Second); big <= small {
		t.Errorf("no swell: %d ink pixels at the peak, %d at rest", big, small)
	}
}

// The glow is the fill's colour, and a line not sung has none.
func TestSwellGlowsInTheFillColour(t *testing.T) {
	useAppFont(t)
	theme.SetMode(theme.ModeLight)
	defer theme.SetMode(theme.ModeSystem)
	words := []lyrics.Word{{Begin: 0, End: 4 * time.Second, Text: "Hold"}}
	s := NewSweepText(words, "", 60, 700, 72, 400)
	sung := float32(1)
	at := time.Duration(-time.Second)
	tt := ui.NewTester(func(c *ui.Context) {
		theme.Apply(c)
		ui.Box(c).Padding(10).Children(func() {
			ui.Box(c.Key("s")).Size(s.Width(), s.Height()).Draw(func(p *ui.Painter, r ui.Rect) {
				// A glow of blue round red letters on white: blue pixels not
				// under a letter are the glow.
				s.Paint(p, r.X, r.Y, mediaGradBase, mediaGradFill, SweepClock{T: at, Sung: sung}, false)
			})
		})
	}, 420, 100)
	at = time.Duration(float64(s.words[0].emp.du) * 0.5)
	// bluish counts the pixels the glow tints (blue over white): of those that
	// are white without it.
	bluish := func(img, without *image.RGBA) int {
		n := 0
		for i := 0; i < len(img.Pix); i += 4 {
			r, g, b := int(img.Pix[i]), int(img.Pix[i+1]), int(img.Pix[i+2])
			if without.Pix[i] > 235 && without.Pix[i+1] > 235 && without.Pix[i+2] > 235 && b > r+20 && b > g+20 {
				n++
			}
		}
		return n
	}
	sung = 0
	tt.Frame()
	dark := append([]uint8(nil), tt.Image().Pix...)
	plain := &image.RGBA{Pix: dark, Stride: tt.Image().Stride, Rect: tt.Image().Rect}
	if n := bluish(plain, plain); n != 0 {
		t.Errorf("a line not sung glows: %d pixels", n)
	}
	sung = 1
	tt.Frame()
	if n := bluish(tt.Image(), plain); n < 200 {
		t.Errorf("no glow of the fill's colour round a swelling word: %d tinted pixels", n)
	}
}

// Words set close (the syllables of one word) are painted each within its
// own box: no ink is drawn twice at two heights.
func TestCloseWordsAreNotPaintedOverEachOther(t *testing.T) {
	useAppFont(t)
	words := []lyrics.Word{{Text: "break"}, {Text: "wa"}, {Text: "ter "}, {Text: "now"}}
	s := NewSweepText(words, "", 40, 600, 48, 600)
	for i := range 2 {
		a, b := s.words[i].boxes[0], s.words[i+1].boxes[0]
		if a.r.X+a.r.W+a.right > b.r.X-b.left+0.01 {
			t.Errorf("words %d and %d overlap: %v+%v and %v−%v", i, i+1, a.r.X+a.r.W, a.right, b.r.X, b.left)
		}
	}
	// Apart by a space they keep their overhang: an italic slant is inside.
	w := s.words[2].boxes[0]
	if w.right <= 0 || w.right > 0.15*40+0.01 {
		t.Errorf("a word before a space overhangs %v", w.right)
	}
}
