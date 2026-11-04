package kit

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/lyrics"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

var (
	mediaGradBase = ui.RGB(255, 0, 0) // the text
	mediaGradFill = ui.RGB(0, 0, 255) // the fill
)

// glyphs counts the pixels of the window's x range that are mostly red (the
// base) and mostly blue (the fill).
func mediaGlyphs(tt *ui.Tester, x0, x1 int) (base, fill int) {
	img := tt.Image()
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := x0; x < x1; x++ {
			c := img.RGBAAt(x, y)
			switch {
			case c.R > 200 && c.B < 80:
				base++
			case c.B > 200 && c.R < 80:
				fill++
			}
		}
	}
	return base, fill
}

func mediaGradientView(o GradientOpts, text string) func(*ui.Context) {
	return func(c *ui.Context) {
		theme.Apply(c)
		ui.Box(c).Padding(10).Children(func() {
			GradientText(c.Key("g"), text, o)
		})
	}
}

func TestGradientTextFillsFromTheStartOverTheBase(t *testing.T) {
	theme.SetMode(theme.ModeLight)
	defer theme.SetMode(theme.ModeSystem)
	o := GradientOpts{Size: 40, Weight: 700, Base: mediaGradBase, Fill: mediaGradFill}
	text := "HHHHHHHH"
	img := func(p float32, rtl bool) (left, right [2]int) {
		o := o
		o.Progress, o.RTL = p, rtl
		tt := ui.NewTester(mediaGradientView(o, text), 300, 70)
		tt.Frame()
		l0, l1 := mediaGlyphs(tt, 0, 150)
		r0, r1 := mediaGlyphs(tt, 150, 300)
		return [2]int{l0, l1}, [2]int{r0, r1}
	}
	// Nothing reached: all base.
	l, r := img(0, false)
	if l[1]+r[1] != 0 || l[0]+r[0] < 100 {
		t.Errorf("progress 0: left %v right %v (base, fill)", l, r)
	}
	// All reached: all fill (the soft edge is past the end).
	l, r = img(1, false)
	if l[0]+r[0] > 20 || l[1]+r[1] < 100 {
		t.Errorf("progress 1: left %v right %v", l, r)
	}
	// Half: the left is fill, the right is base.
	l, r = img(0.5, false)
	if l[1] < 50 || r[0] < 50 || r[1] > l[1]/2 || l[0] > r[0]/2 {
		t.Errorf("progress .5: left %v right %v", l, r)
	}
	// Right to left fills from the right.
	l, r = img(0.5, true)
	if r[1] < 50 || l[0] < 50 || l[1] > r[1]/2 || r[0] > l[0]/2 {
		t.Errorf("rtl progress .5: left %v right %v", l, r)
	}
}

func TestGradientTextHasASoftEdge(t *testing.T) {
	theme.SetMode(theme.ModeLight)
	defer theme.SetMode(theme.ModeSystem)
	o := GradientOpts{Size: 60, Weight: 900, Base: mediaGradBase, Fill: mediaGradFill, Progress: 0.4, Edge: 60}
	tt := ui.NewTester(mediaGradientView(o, "IIIIIIIIIIII"), 600, 90)
	tt.Frame()
	// Along the glyphs' row, the fill's strength falls off through the edge:
	// blue in the first part, then purples (the fill's alpha over red), then red.
	img := tt.Image()
	var blues, purples, reds int
	for x := 10; x < 590; x++ {
		for y := 20; y < 70; y++ {
			c := img.RGBAAt(x, y)
			switch {
			case c.B > 200 && c.R < 60:
				blues++
			case c.B > 60 && c.R > 60 && c.G < 60:
				purples++
			case c.R > 200 && c.B < 60:
				reds++
			}
		}
	}
	if blues == 0 || purples == 0 || reds == 0 {
		t.Errorf("blue %d, purple %d, red %d pixels: want all three", blues, purples, reds)
	}
}

func TestGradientTextWrapsAtItsWidth(t *testing.T) {
	theme.SetMode(theme.ModeLight)
	defer theme.SetMode(theme.ModeSystem)
	text := "a text that wraps over a few lines"
	narrow := shapeGradient(text, GradientOpts{Size: 20, Width: 100})
	wide := shapeGradient(text, GradientOpts{Size: 20, Width: 400})
	if narrow.h <= wide.h || len(narrow.rects) <= len(wide.rects) {
		t.Errorf("wrapping at 100 gives %v high in %d boxes, at 400 %v in %d", narrow.h, len(narrow.rects), wide.h, len(wide.rects))
	}
	if narrow.key != (gradientKey{text, 20, 0, 0, 100}) {
		t.Errorf("key %+v", narrow.key)
	}
	// A wrapped text fills line after line: half the progress is the first
	// lines, not half of each.
	o := GradientOpts{Size: 20, Weight: 700, Base: mediaGradBase, Fill: mediaGradFill, Width: 100, Progress: 0.5}
	tt := ui.NewTester(mediaGradientView(o, "HHHHHHHHHHHHHHHHHHHHHHHH HHHHHHHHHHHHHHHHHHHHHHHH"), 140, 140)
	tt.Frame()
	img := tt.Image()
	topFill, bottomBase := 0, 0
	for y := 10; y < 140; y++ {
		for x := 0; x < 140; x++ {
			c := img.RGBAAt(x, y)
			if y < 40 && c.B > 200 && c.R < 80 {
				topFill++
			}
			if y > 70 && c.R > 200 && c.B < 80 {
				bottomBase++
			}
		}
	}
	if topFill < 50 || bottomBase < 50 {
		t.Errorf("top lines fill %d, bottom lines base %d", topFill, bottomBase)
	}
}

func TestSweepTextFillsEachWordAsItIsSung(t *testing.T) {
	theme.SetMode(theme.ModeLight)
	defer theme.SetMode(theme.ModeSystem)
	words := []lyrics.Word{
		{Begin: 0, End: time.Second, Text: "HHHH "},
		{Begin: time.Second, End: 2 * time.Second, Text: "HHHH"},
	}
	s := NewSweepText(words, 40, 700, 48, 400)
	if s.Width() != 400 || s.Height() < 40 {
		t.Errorf("shaped %v×%v", s.Width(), s.Height())
	}
	var moving bool
	at := time.Duration(0)
	tt := ui.NewTester(func(c *ui.Context) {
		theme.Apply(c)
		ui.Box(c).Padding(10).Children(func() {
			ui.Box(c.Key("s")).Size(s.Width(), s.Height()).Draw(func(p *ui.Painter, r ui.Rect) {
				moving = s.Paint(p, r.X, r.Y, mediaGradBase, mediaGradFill, 1, at, false)
			})
		})
	}, 420, 70)
	count := func(t0 time.Duration) (base, fill int, mv bool) {
		at = t0
		tt.Frame()
		b, f := mediaGlyphs(tt, 0, 420)
		return b, f, moving
	}
	if b, f, mv := count(-time.Second); f != 0 || b < 50 || mv {
		t.Errorf("before the line: base %d fill %d moving %v", b, f, mv)
	}
	if b, f, mv := count(500 * time.Millisecond); f < 20 || b < 50 || !mv {
		t.Errorf("first word half sung: base %d fill %d moving %v", b, f, mv)
	}
	if b, f, mv := count(5 * time.Second); b > 30 || f < 100 || mv {
		t.Errorf("all sung: base %d fill %d moving %v", b, f, mv)
	}
}

func TestWithAlphaScalesTheAlpha(t *testing.T) {
	if got := mediaAlpha(ui.RGB(1, 2, 3), 0.5); got.A != 128 || got.R != 1 {
		t.Errorf("mediaAlpha = %v", got)
	}
	if got := mediaAlpha(ui.RGB(1, 2, 3), 2); got.A != 255 {
		t.Errorf("alpha above 1: %v", got)
	}
}
