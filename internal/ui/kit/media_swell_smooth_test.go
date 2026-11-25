package kit

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/lyrics"
)

// smoothPad is the room round the text in the smoothness tests' window, in
// DIPs: a swollen letter rises and grows past its box.
const smoothPad = 40

// smoothShot is a tester showing s in base and fill at clock *at, over a white
// ground, at scale.
func smoothShot(s *SweepText, base, fill ui.Color, scale float32, at *time.Duration) *ui.Tester {
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Background(ui.RGB(255, 255, 255)).Padding(smoothPad).Children(func() {
			ui.Box(c.Key("s")).Size(s.Width(), s.Height()).Draw(func(p *ui.Painter, r ui.Rect) {
				s.Paint(p, r.X, r.Y, base, fill, SweepClock{T: *at, Sung: 1}, false)
			})
		})
	}, int(s.Width())+2*smoothPad, int(math.Ceil(float64(s.Height())))+2*smoothPad)
	tt.SetScale(scale)
	return tt
}

// A swelling letter grows smoothly. Its glyphs are shaped at a few sizes of
// the swell (swellStep apart: 2%, a pixel or two at the sizes of the stage),
// and a letter drawn at the nearest of them alone would grow in steps while
// the word sings. The stem of the l (the middle letter of "hold", which the
// swell spreads nowhere) is one column of ink: its coverage down the column
// is its height in device pixels. That height follows the swell's scale to
// within half a pixel and never moves by half a pixel from one frame to the
// next, at 60 frames a second, at 1× and at 2× and at both sizes of text.
func TestASwellingLettersHeightGrowsWithoutSteps(t *testing.T) {
	useAppFont(t)
	ink := ui.RGB(0, 0, 0)
	for _, size := range []float32{48, 72} {
		s := NewSweepText([]lyrics.Word{{Begin: 0, End: 3 * time.Second, Text: "hold"}}, "", size, 700, size*1.25, 500)
		w := &s.words[0]
		if w.emp == nil {
			t.Fatal("a word held for 3 s does not swell")
		}
		const letter = 2 // the l
		l := &w.letters[letter]
		for _, scale := range []float32{1, 2} {
			var at time.Duration
			tt := smoothShot(s, ink, ui.Color{}, scale, &at)
			// The ink of the stem's middle column, in device pixels.
			col := int(math.Round(float64((smoothPad + l.x + l.w/2) * scale)))
			height := func() float64 {
				img := tt.Image()
				sum := 0.0
				for y := range img.Bounds().Dy() {
					sum += float64(255-img.RGBAAt(col, y).R) / 255
				}
				return sum
			}
			at = -time.Second
			tt.Frame()
			rest := height()
			if rest < float64(size*scale)*0.5 {
				t.Fatalf("size %v at %v×: the l's column holds %.1f px of ink: it is not on the stem", size, scale, rest)
			}
			prev, biggest, worst := rest, rest, 0.0
			step := time.Second / 60
			for at = 0; at <= w.emp.total()+200*ms; at += step {
				tt.Frame()
				h := height()
				if d := math.Abs(h - prev); d >= 0.5 {
					t.Errorf("size %v at %v×, %v: the l's height moved %.2f px in a frame (%.2f to %.2f)", size, scale, at, d, prev, h)
				}
				if f := at - w.begin; f >= 0 && f < w.emp.run {
					pose := w.emp.pose(letter, f)
					if d := math.Abs(h - rest*float64(pose.scale)); d > 0.6 {
						t.Errorf("size %v at %v×, %v: the l is %.2f px tall, the swell's scale %.4f says %.2f", size, scale, at, h, pose.scale, rest*float64(pose.scale))
					}
				}
				worst = math.Max(worst, math.Abs(h-prev))
				prev, biggest = h, math.Max(biggest, h)
			}
			if biggest < rest*1.03 {
				t.Errorf("size %v at %v×: the l grew from %.1f to %.1f px at the most: the swell did not show", size, scale, rest, biggest)
			}
			t.Logf("size %v at %v×: the l grows from %.1f to %.1f px, %.2f px a frame at the most", size, scale, rest, biggest, worst)
		}
	}
}

// The fill's front moves by parts of a pixel. The stretches of colour are
// clips, and a clip's edges snap to whole device pixels, so a soft edge cut
// into slices that travel with the front would step by a pixel at a time and
// judder across a slow word. The slices stay where they are and change colour
// as the front passes: the red ink of the word, counted in pixels of coverage
// (red less blue, over 255), grows in every 4 ms step of a word that takes
// 4 s to cross 300 DIPs, by about the same each time (at 1× the front moves
// 0.3 px in a step, at 2× 0.6).
func TestTheFillFrontMovesByPartsOfAPixel(t *testing.T) {
	useAppFont(t)
	base, fill := ui.RGB(0, 0, 0), ui.RGB(255, 0, 0)
	// Eight letters and more do not swell: only the fill and the lift move.
	s := NewSweepText([]lyrics.Word{{Begin: 0, End: 4 * time.Second, Text: "breakwater"}}, "", 48, 700, 60, 600)
	if s.words[0].emp != nil {
		t.Fatal("the word swells")
	}
	for _, scale := range []float32{1, 2} {
		var at time.Duration
		tt := smoothShot(s, base, fill, scale, &at)
		red := func() float64 {
			img := tt.Image()
			sum := 0.0
			for y := range img.Bounds().Dy() {
				for x := range img.Bounds().Dx() {
					c := img.RGBAAt(x, y)
					sum += float64(int(c.R)-int(c.B)) / 255
				}
			}
			return sum
		}
		at = 800 * ms
		tt.Frame()
		prev := red()
		var steps []float64
		for at = 800*ms + 4*ms; at <= 3200*ms; at += 4 * ms {
			tt.Frame()
			now := red()
			steps = append(steps, now-prev)
			prev = now
		}
		sorted := slices.Clone(steps)
		slices.Sort(sorted)
		median := sorted[len(sorted)/2]
		if median <= 0 {
			t.Fatalf("%v×: the red ink grows by %.3f px of coverage in a step: the fill does not move", scale, median)
		}
		stalled, jumped := 0, 0
		for _, d := range steps {
			switch {
			case d < 0.25*median:
				stalled++
			case d > 3*median:
				jumped++
			}
		}
		t.Logf("%v×: %d steps of 4 ms, the red ink grows %.3f px of coverage in a step at the median, from %.3f to %.3f", scale, len(steps), median, sorted[0], sorted[len(sorted)-1])
		if stalled > 0 || jumped > 0 {
			t.Errorf("%v×: of %d steps %d stood still and %d jumped (the median step is %.3f px of coverage): the front moves by whole pixels", scale, len(steps), stalled, jumped, median)
		}
	}
}
