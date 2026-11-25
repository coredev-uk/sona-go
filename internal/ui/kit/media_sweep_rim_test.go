package kit

import (
	"fmt"
	"image"
	"math"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/lyrics"
)

// rimRegion is a stretch of a SweepText's layout (DIPs, in its coordinates)
// that must come out as the text drawn once in c, raised dy DIPs.
type rimRegion struct {
	name           string
	x0, x1, y0, y1 float32
	c              ui.Color
	dy             float32
}

// The sung part of a word-timed line is painted in the fill alone, the part
// not yet sung in the base alone: never the fill drawn over the base, whose
// edge pixels keep (1−c)·c of the near-white base and ring the amber glyphs
// with a pale rim (issue #385). Each stretch is pixel for pixel the text drawn
// once in its colour, at 1×, 1.25× and 2×: on one line, wrapped, right to
// left, still (reduced motion, the line painted whole) and by letters, the
// first word sung and its letters down again.
func TestSweepTextPaintsEveryPixelInOneColour(t *testing.T) {
	useAppFont(t)
	bg := ui.RGB(0x18, 0x18, 0x18)
	base := ui.RGB(0xf0, 0xf0, 0xf0) // the dark theme's ink
	fill := ui.RGB(0xf0, 0xb4, 0x3c) // and amber-ink
	// Words sung for under a second, which do not swell.
	latin := []lyrics.Word{{Begin: 0, End: 800 * ms, Text: "Holding "}, {Begin: 1500 * ms, End: 2300 * ms, Text: "water"}}
	hebrew := []lyrics.Word{{Begin: 0, End: 800 * ms, Text: "שלום "}, {Begin: 1500 * ms, End: 2300 * ms, Text: "עולם"}}
	for _, c := range []struct {
		name  string
		words []lyrics.Word
		lang  string
		width float32
		rtl   bool
		clock SweepClock
	}{
		// Still: the first word sung, the second 60% of the way.
		{"still", latin, "", 400, false, SweepClock{T: 1980 * ms, Sung: 1, Still: true}},
		{"still wrapped", latin, "", 190, false, SweepClock{T: 1980 * ms, Sung: 1, Still: true}},
		{"still rtl", hebrew, "he", 400, true, SweepClock{T: 1980 * ms, Sung: 1, Still: true}},
		// Down: the first word sung, its letters risen and back at rest, the
		// second not begun.
		{"down", latin, "", 400, false, SweepClock{T: 1300 * ms, Sung: 1}},
		{"down wrapped", latin, "", 190, false, SweepClock{T: 1300 * ms, Sung: 1}},
		{"down rtl", hebrew, "he", 400, true, SweepClock{T: 1300 * ms, Sung: 1}},
	} {
		s := NewSweepText(c.words, c.lang, 40, 700, 48, c.width)
		edge := s.size * 0.5
		var regions []rimRegion
		for k := range s.words {
			w := &s.words[k]
			if len(w.boxes) != 1 {
				t.Fatalf("%s: word %d is in %d boxes", c.name, k, len(w.boxes))
			}
			b := w.boxes[0]
			left, right := b.r.X-b.left, b.r.X+b.r.W+b.right
			y0, y1 := b.r.Y, b.r.Y+b.r.H
			if c.clock.T < w.begin {
				regions = append(regions, rimRegion{fmt.Sprintf("word %d, not begun", k), left, right, y0, y1, base, 0})
				continue
			}
			prog, _ := w.progress(c.clock.T)
			dy := float32(0)
			if !c.clock.Still && prog < 1 {
				t.Fatalf("%s: word %d is moving", c.name, k)
			}
			if prog >= 1 {
				regions = append(regions, rimRegion{fmt.Sprintf("word %d, sung", k), left, right, y0, y1, fill, dy})
				continue
			}
			// The fade's slices lie on a grid and take the colour of the fade at
			// their middle (mediaSegments): the slice the front is in, and the one
			// the fade ends in, are mixed, so the pure stretches end a slice
			// short of them.
			solid, slice := prog*(w.total+edge)-edge, edge/gradientSteps
			reached := rimRegion{fmt.Sprintf("word %d, reached", k), left, b.r.X + solid - slice, y0, y1, fill, dy}
			ahead := rimRegion{fmt.Sprintf("word %d, ahead", k), b.r.X + solid + edge + slice, right, y0, y1, base, dy}
			if c.rtl {
				reached.x0, reached.x1 = b.r.X+b.r.W-solid+slice, right
				ahead.x0, ahead.x1 = left, b.r.X+b.r.W-solid-edge-slice
			}
			regions = append(regions, reached, ahead)
		}
		for _, scale := range []float32{1, 1.25, 2} {
			name := fmt.Sprintf("%s at %v×", c.name, scale)
			sweep := rimShot(s, bg, scale, func(p *ui.Painter, x, y float32) { s.Paint(p, x, y, base, fill, c.clock, c.rtl) })
			for _, r := range regions {
				once := rimShot(s, bg, scale, func(p *ui.Painter, x, y float32) { p.TextLayout(s.layout, x, y+r.dy, r.c) })
				ink, off, lighter, worst := rimCompare(sweep, once, r, scale)
				if ink < 20 {
					t.Errorf("%s, %s: only %d pixels of ink to compare", name, r.name, ink)
				}
				if off > 0 {
					t.Errorf("%s, %s: %d of %d pixels differ from the text drawn once in its colour (by up to %d), %d of them lighter (a rim)",
						name, r.name, off, ink, worst, lighter)
				}
			}
		}
	}
}

// rimPad is the padding round the text in rimShot's window, in DIPs.
const rimPad = 10

// rimShot is a frame of draw (at the text's origin) over an opaque bg, at
// scale, copied.
func rimShot(s *SweepText, bg ui.Color, scale float32, draw func(p *ui.Painter, x, y float32)) *image.RGBA {
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Background(bg).Padding(rimPad).Children(func() {
			ui.Box(c.Key("s")).Size(s.Width(), s.Height()).Draw(func(p *ui.Painter, r ui.Rect) { draw(p, r.X, r.Y) })
		})
	}, int(s.Width())+2*rimPad, int(math.Ceil(float64(s.Height())))+2*rimPad)
	tt.SetScale(scale)
	tt.Frame()
	img := tt.Image()
	out := image.NewRGBA(img.Rect)
	copy(out.Pix, img.Pix)
	return out
}

// rimCompare compares got with want over the device pixels well inside r: how
// many are ink (differ from the background) in want or got, how many of them
// differ by more than one level, how many are lighter than want in a channel
// and the largest difference.
func rimCompare(got, want *image.RGBA, r rimRegion, scale float32) (ink, off, lighter, worst int) {
	bg := want.RGBAAt(0, 0)
	px := func(v float32) float64 { return float64((v + rimPad) * scale) }
	x0, x1 := int(math.Ceil(px(r.x0)))+1, int(math.Floor(px(r.x1)))-1
	y0, y1 := int(math.Ceil(px(r.y0)))+1, int(math.Floor(px(r.y1)))-1
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			g, w := got.RGBAAt(x, y), want.RGBAAt(x, y)
			if g == bg && w == bg {
				continue
			}
			ink++
			d, up := 0, false
			for i, gv := range [3]uint8{g.R, g.G, g.B} {
				wv := [3]uint8{w.R, w.G, w.B}[i]
				d = max(d, int(gv)-int(wv), int(wv)-int(gv))
				up = up || int(gv) > int(wv)+1
			}
			if d > 1 {
				off++
				worst = max(worst, d)
				if up {
					lighter++
				}
			}
		}
	}
	return ink, off, lighter, worst
}

// GradientText paints the part its fill has reached in the fill alone too: at
// progress 1 it is pixel for pixel the text in the fill.
func TestGradientTextPaintsTheFillWithoutTheBase(t *testing.T) {
	useAppFont(t)
	bg := ui.RGB(0x18, 0x18, 0x18)
	base, fill := ui.RGB(0xf0, 0xf0, 0xf0), ui.RGB(0xf0, 0xb4, 0x3c)
	shot := func(o GradientOpts) *image.RGBA {
		tt := ui.NewTester(func(c *ui.Context) {
			ui.Box(c).Background(bg).Padding(rimPad).Children(func() {
				GradientText(c.Key("g"), "Holding water", o)
			})
		}, 300, 60)
		tt.SetScale(1.25)
		tt.Frame()
		img := tt.Image()
		out := image.NewRGBA(img.Rect)
		copy(out.Pix, img.Pix)
		return out
	}
	o := GradientOpts{Size: 32, Weight: 700, Base: base, Fill: fill, Progress: 1}
	got := shot(o)
	o.Base, o.Progress = fill, 0
	want := shot(o)
	ink, off, lighter, worst := rimCompare(got, want, rimRegion{x0: 0, x1: 280, y0: 0, y1: 40}, 1.25)
	if ink < 100 || off > 0 {
		t.Errorf("%d of %d pixels differ from the text in the fill (by up to %d), %d of them lighter", off, ink, worst, lighter)
	}
}
