package kit

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/lyrics"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// fortyLetters is a line of 8 words of 5 letters (40 letters), each sung for
// 400 ms, 3.2 s in all.
func fortyLetters() []lyrics.Word {
	var words []lyrics.Word
	for i := range 8 {
		words = append(words, lyrics.Word{
			Begin: time.Duration(i) * 400 * time.Millisecond, End: time.Duration(i+1) * 400 * time.Millisecond, Text: "Abcde ",
		})
	}
	return words
}

// benchSweep paints a 40-letter line (nil: nothing) at clock on the Tester,
// one frame an iteration, drawing the sweep alone.
func benchSweep(b *testing.B, s *SweepText, clock SweepClock) {
	b.Helper()
	if err := theme.RegisterFonts(); err != nil {
		b.Fatal(err)
	}
	theme.SetMode(theme.ModeDark)
	b.Cleanup(func() { theme.SetMode(theme.ModeSystem) })
	tt := ui.NewTester(func(c *ui.Context) {
		theme.Apply(c)
		ui.Box(c.Key("s")).Size(900, 120).Draw(func(p *ui.Painter, r ui.Rect) {
			if s != nil {
				s.Paint(p, r.X, r.Y, ui.RGB(235, 232, 240), ui.RGB(240, 180, 60), clock, false)
			}
		})
	}, 900, 120)
	tt.Frame()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		tt.Frame()
	}
}

// BenchmarkSweepPaint is a frame of a 40-letter line mid-sweep: the front is
// in the fourth word, so some letters are rising, those before are up and
// those after are not begun. BenchmarkSweepPaintBaseline is the same frame
// drawing nothing: the Tester's own cost, to take off.
func BenchmarkSweepPaint(b *testing.B) {
	s := NewSweepText(fortyLetters(), "", 40, 650, 50, 900)
	benchSweep(b, s, SweepClock{T: 1300 * time.Millisecond, Sung: 1})
}

func BenchmarkSweepPaintBaseline(b *testing.B) {
	benchSweep(b, nil, SweepClock{})
}

// An idle line, painted whole, and one at rest all raised.
func BenchmarkSweepPaintIdle(b *testing.B) {
	s := NewSweepText(fortyLetters(), "", 40, 650, 50, 900)
	benchSweep(b, s, SweepClock{T: -time.Second})
}

func BenchmarkSweepPaintAtRest(b *testing.B) {
	s := NewSweepText(fortyLetters(), "", 40, 650, 50, 900)
	benchSweep(b, s, SweepClock{T: 5 * time.Second, Sung: 1})
}
