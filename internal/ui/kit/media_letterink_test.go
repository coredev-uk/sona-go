package kit

import (
	"image"
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/lyrics"
)

// glyphShot is a frame of s with every letter drawn on its own from its
// glyphs, in ink, where the sweep puts it at clock t and with nothing
// clipped: what the sweep must come to wherever its clips fall. A swollen
// word's letters are posed as paintLetter poses them (without the glow).
func glyphShot(s *SweepText, bg, ink ui.Color, scale float32, t time.Duration) *image.RGBA {
	return rimShot(s, bg, scale, func(p *ui.Painter, x, y float32) {
		edge, size := s.size*0.5, s.size
		for k := range s.words {
			w := &s.words[k]
			f := t - w.begin
			swelling := w.emp != nil && f >= 0 && f < w.emp.run
			for i := range w.letters {
				l := &w.letters[i]
				if l.glyphs == nil {
					continue
				}
				lift, _ := w.lift(l, t, edge, false)
				pose := letterPose{scale: 1}
				if swelling {
					pose = w.emp.pose(i, f)
				}
				c0, cy := x+l.x+l.w/2, y+l.centre
				draw := func() { p.Glyphs(l.glyphs, x+l.x, y+l.baseline, ink) }
				p.Transform(ui.Translation(pose.dx*size, (pose.dy-lift)*size), 0, 0, func() {
					if pose.scale == 1 {
						draw()
						return
					}
					p.Transform(ui.Scaling(pose.scale, pose.scale), c0, cy, draw)
				})
			}
		}
	})
}

// inkOff counts the device pixels of a and b that differ by more than a
// shade in a channel (antialiasing differs by less), and the first of them.
func inkOff(a, b *image.RGBA) (n int, first image.Point) {
	for y := 0; y < a.Rect.Dy(); y++ {
		for x := 0; x < a.Rect.Dx(); x++ {
			p, q := a.RGBAAt(x, y), b.RGBAAt(x, y)
			d := max(absInt(int(p.R)-int(q.R)), absInt(int(p.G)-int(q.G)), absInt(int(p.B)-int(q.B)))
			if d > 40 {
				if n == 0 {
					first = image.Pt(x, y)
				}
				n++
			}
		}
	}
	return n, first
}

func absInt(v int) int { return max(v, -v) }

// A letter on the move keeps all its ink, and a letter at rest beside it all
// of its: the text is, at every time, what its letters drawn unclipped come
// to. Clipping each to its advance cut the ink that overhangs it (a kerned
// pair, an italic) from the neighbour at rest and painted it twice from the
// one on the move; ligatures came apart as their first letter lifted.
func TestLiftedLettersKeepAllTheirInk(t *testing.T) {
	useAppFont(t)
	type word struct {
		text           string
		italic, bg, em bool
		line           float32 // line height, in sizes
	}
	for _, c := range []word{
		{text: "fjord"},
		{text: "Ty"},
		{text: "AVA"},
		{text: "office"},
		{text: "Éclat", line: 1},
		{text: "pygmy"},
		{text: "ſaffron"},
		{text: "fjord", italic: true},
		{text: "Typhoon", italic: true},
		{text: "office", bg: true},
		{text: "Éclat", em: true},
	} {
		end := 800 * ms
		if c.em {
			end = 2 * time.Second
		}
		lh := c.line
		if lh == 0 {
			lh = 1.25
		}
		words := []lyrics.Word{{Begin: 0, End: end, Text: c.text, Background: c.bg}}
		for _, dark := range []bool{false, true} {
			bg, ink := ui.RGB(0xfa, 0xf8, 0xf4), ui.RGB(0x18, 0x18, 0x18)
			if dark {
				bg, ink = ui.RGB(0x18, 0x18, 0x18), ui.RGB(0xf0, 0xf0, 0xf0)
			}
			var s *SweepText
			if c.italic {
				s = NewSweepTextItalic(words, "", 48, 700, 48*lh, 400)
			} else {
				s = NewSweepText(words, "", 48, 700, 48*lh, 400)
			}
			for _, scale := range []float32{1, 2} {
				for at := 10 * ms; at < end+400*ms; at += 30 * ms {
					got := rimShot(s, bg, scale, func(p *ui.Painter, x, y float32) {
						s.Paint(p, x, y, ink, ui.Color{}, SweepClock{T: at, Sung: 1}, false)
					})
					if n, first := inkOff(got, glyphShot(s, bg, ink, scale, at)); n > 0 {
						t.Errorf("%q italic=%v bg=%v swell=%v dark=%v at %v×, %v: %d pixels differ from the letters drawn unclipped, the first at %v", c.text, c.italic, c.bg, c.em, dark, scale, at, n, first)
						break
					}
				}
			}
		}
	}
}

// A letter's lift moves its ink by a transform, a fraction of a pixel at a
// time, for the glyphs of a real word as for a test's: the ink of the letter
// stands where the lift says (within a quarter of a device pixel, which a
// pixel-snapped glyph would not) and moves by less than a DIP between two
// frames, at 60 and at 30 frames a second.
func TestALettersInkFollowsItsLiftSmoothly(t *testing.T) {
	useAppFont(t)
	bg, ink := ui.RGB(255, 255, 255), ui.RGB(0, 0, 0)
	for _, c := range []struct {
		name string
		size float32
		bg   bool
	}{{"lead 48", 48, false}, {"lead 64", 64, false}, {"background 40", 40, true}} {
		s := NewSweepText([]lyrics.Word{{Begin: 0, End: 900 * ms, Text: "fjordly", Background: c.bg}}, "", c.size, 700, c.size*1.25, 500)
		w := &s.words[0]
		const letter = 2 // the o
		l := &w.letters[letter]
		for _, scale := range []float32{1, 2} {
			var at time.Duration
			tt := ui.NewTester(func(ctx *ui.Context) {
				ui.Box(ctx).Background(bg).Padding(rimPad).Children(func() {
					ui.Box(ctx.Key("s")).Size(s.Width(), s.Height()).Draw(func(p *ui.Painter, r ui.Rect) {
						s.Paint(p, r.X, r.Y, ink, ui.Color{}, SweepClock{T: at, Sung: 1}, false)
					})
				})
			}, int(s.Width())+2*rimPad, int(math.Ceil(float64(s.Height())))+2*rimPad)
			tt.SetScale(scale)
			// The vertical centre of the o's ink, over its box's columns.
			x0, x1 := int((rimPad+l.x)*scale), int((rimPad+l.x+l.w)*scale)
			centre := func() float64 {
				img := tt.Image()
				var sum, mass float64
				for y := 0; y < img.Bounds().Dy(); y++ {
					for x := x0; x < x1; x++ {
						d := float64(255 - img.RGBAAt(x, y).R)
						sum += d * float64(y)
						mass += d
					}
				}
				return sum / mass
			}
			at = -time.Second
			tt.Frame()
			rest := centre()
			for _, fps := range []float64{60, 30} {
				step := time.Duration(float64(time.Second) / fps)
				prev := 0.0
				for at = 0; at <= 1500*ms; at += step {
					tt.Frame()
					got := (rest - centre()) / float64(scale) // DIPs up
					want, _ := w.lift(l, at, s.size*0.5, false)
					if d := math.Abs(got - float64(want*s.size)); d > 0.25/float64(scale)+0.05 {
						t.Errorf("%s at %v×, %v fps, %v: the o stands %.2f DIP up, its lift is %.2f", c.name, scale, fps, at, got, want*s.size)
					}
					// Under a pixel in a frame at 60 fps; at 30 (the CPU
					// rasteriser's) a third of the whole lift, where that is more.
					limit := 1.0
					if fps == 30 {
						limit = max(1, 0.3*float64(w.up*s.size))
					}
					if d := math.Abs(got - prev); d >= limit {
						t.Errorf("%s at %v×, %v fps, %v: the o moved %.2f DIP in a frame", c.name, scale, fps, at, d)
					}
					prev = got
				}
			}
		}
	}
}

// Paint asks for the next frame whenever anything is about to move: a letter
// whose lift changes in the next 5 ms, a swell, a fill. A line that stopped
// asking while a letter still moved would freeze it and start it again.
func TestPaintAsksForFramesWhileAnythingMoves(t *testing.T) {
	useAppFont(t)
	words := []lyrics.Word{
		{Begin: 0, End: 400 * ms, Text: "one "},
		{Begin: 400 * ms, End: 900 * ms, Text: "fjord ", Background: true},
		{Begin: 1500 * ms, End: 3500 * ms, Text: "tide "},
		{Begin: 4000 * ms, End: 4300 * ms, Text: "end"},
	}
	s := NewSweepText(words, "", 32, 700, 40, 600)
	var at time.Duration
	var moving bool
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Padding(10).Children(func() {
			ui.Box(c.Key("s")).Size(s.Width(), s.Height()).Draw(func(p *ui.Painter, r ui.Rect) {
				moving = s.Paint(p, r.X, r.Y, ui.RGB(0, 0, 0), ui.RGB(200, 0, 0), SweepClock{T: at, Sung: 1}, false)
			})
		})
	}, 620, 60)
	edge := s.size * 0.5
	for at = 0; at < 5500*ms; at += 5 * ms {
		tt.Frame()
		if moving {
			continue
		}
		for k := range s.words {
			w := &s.words[k]
			f := at - w.begin
			if w.emp != nil && f >= 0 && f < w.emp.run {
				t.Fatalf("not moving at %v, in the swell of %q", at, w.text)
			}
			for i := range w.letters {
				a, _ := w.lift(&w.letters[i], at, edge, false)
				b, _ := w.lift(&w.letters[i], at+5*ms, edge, false)
				if a != b {
					t.Fatalf("not moving at %v, but letter %d of %q goes from %v to %v em", at, i, w.text, a, b)
				}
			}
		}
	}
}

// Where the line stops being painted by letters (its last letter is down)
// nothing shows: the frame before and the frame after are the same pixels.
func TestTheHandOverToThePlainTextShowsNothing(t *testing.T) {
	useAppFont(t)
	bg, ink := ui.RGB(255, 255, 255), ui.RGB(0, 0, 0)
	for _, text := range []string{"office fjord", "Éclat"} {
		s := NewSweepText([]lyrics.Word{{Begin: 0, End: 800 * ms, Text: text}}, "", 48, 700, 60, 500)
		shot := func(at time.Duration) *image.RGBA {
			return rimShot(s, bg, 2, func(p *ui.Painter, x, y float32) {
				s.Paint(p, x, y, ink, ui.Color{}, SweepClock{T: at, Sung: 1}, false)
			})
		}
		if !s.lifts(SweepClock{T: s.end - ms}) || s.lifts(SweepClock{T: s.end}) {
			t.Fatalf("%q: the hand over is not at %v", text, s.end)
		}
		if n, first := inkOff(shot(s.end-ms), shot(s.end)); n > 0 {
			t.Errorf("%q: %d pixels change as the last letter lands, the first at %v", text, n, first)
		}
	}
}
