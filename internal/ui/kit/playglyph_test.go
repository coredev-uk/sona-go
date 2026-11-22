package kit

import (
	"image"
	"image/color"
	"math"
	"runtime"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// playDisc renders PlayButton at size and returns its picture and box.
func playDisc(t *testing.T, size, scale float32) (*image.RGBA, ui.Rect) {
	t.Helper()
	theme.SetMode(theme.ModeLight)
	t.Cleanup(func() { theme.SetMode(theme.ModeSystem) })
	tt := ui.NewTester(func(c *ui.Context) {
		theme.Apply(c)
		ui.Row(c).Padding(10).Children(func() { PlayButton(c, "Play", size) })
	}, 80, 80)
	tt.SetScale(scale)
	tt.Frame()
	r, ok := tt.Find("Play")
	if !ok {
		t.Fatal("no Play button")
	}
	return tt.Image(), r
}

// playMarks are the middle of the play triangle's box and its centre of mass
// in the disc r, relative to the disc's middle, in DIPs: the triangle is
// whatever differs from the disc's colour.
func playMarks(im *image.RGBA, sc float32, r ui.Rect) (box, mass [2]float64) {
	bgp := im.RGBAAt(int((r.X+r.W/2)*sc), int((r.Y+r.H*0.12)*sc))
	dist := func(c color.RGBA) float64 {
		return math.Abs(float64(c.R)-float64(bgp.R)) + math.Abs(float64(c.G)-float64(bgp.G)) + math.Abs(float64(c.B)-float64(bgp.B))
	}
	var sw, sx, sy float64
	minx, miny, maxx, maxy := math.MaxInt, math.MaxInt, -1, -1
	for y := int((r.Y + r.H*0.2) * sc); y < int((r.Y+r.H*0.8)*sc); y++ {
		for x := int((r.X + r.W*0.2) * sc); x < int((r.X+r.W*0.8)*sc); x++ {
			if d := dist(im.RGBAAt(x, y)); d > 120 {
				w := d / 765
				sw, sx, sy = sw+w, sx+w*(float64(x)+.5), sy+w*(float64(y)+.5)
				minx, miny, maxx, maxy = min(minx, x), min(miny, y), max(maxx, x), max(maxy, y)
			}
		}
	}
	s, cx, cy := float64(sc), float64(r.X+r.W/2), float64(r.Y+r.H/2)
	return [2]float64{float64(minx+maxx+1)/2/s - cx, float64(miny+maxy+1)/2/s - cy}, [2]float64{sx/sw/s - cx, sy/sw/s - cy}
}

// The Play disc's triangle is optically centred: Lucide's play glyph sits
// right in its viewBox, so its box is a little right of the disc's middle
// and its mass a little left, by the same amount (1/24 of the glyph), and
// no nudge is added to either.
func TestPlayButtonsTriangleIsOpticallyCentred(t *testing.T) {
	// The triangle is read in Lucide's glyph, which Linux and Windows draw;
	// macOS draws the SF Symbol play.fill, whose box sits in the middle and
	// whose shape the system sets (docs/design.md, "Icons").
	if runtime.GOOS != "linux" {
		t.Skip("pixel readings are asserted on Linux only: macOS draws an SF Symbol here")
	}
	for _, size := range []float32{26, 40, 56} {
		for _, sc := range []float32{2, 1} {
			im, r := playDisc(t, size, sc)
			box, mass := playMarks(im, sc, r)
			g := float64(max(size*0.45, 12))
			lim := g/24 + 0.15
			if sc == 1 {
				lim += 0.5 // a pixel snaps the box
			}
			t.Logf("size %v scale %v: box (%.2f, %.2f) mass (%.2f, %.2f)", size, sc, box[0], box[1], mass[0], mass[1])
			if box[0] < -0.1 || box[0] > lim || math.Abs(box[1]) > 0.1 || math.Abs(mass[0]) > lim || math.Abs(mass[1]) > 0.2 {
				t.Errorf("size %v scale %v: triangle box off by (%.2f, %.2f), mass by (%.2f, %.2f); allowed %.2f", size, sc, box[0], box[1], mass[0], mass[1], lim)
			}
		}
	}
}
