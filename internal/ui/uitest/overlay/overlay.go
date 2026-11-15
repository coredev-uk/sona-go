// Package overlay asserts where one mark lies against what it sits on: a
// play wash that must cover its artwork exactly, a disc inside a cover's
// corner, a glyph centred in its button. A one-pixel slip of such a mark is
// easy to miss at 1x and in a golden over a flat fixture cover, so these
// checks measure the layout, and the picture where the cover really is.
//
// It lives beside uitest, not in it, so that the kit's own tests (which
// uitest imports) can use it, as they use uitest/access.
//
// Rectangles are a ui.Tester's: DIPs, from Tester.Find. Every check allows
// 1/100 DIP, the float error of a layout, and no more.
package overlay

import (
	"image"
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
)

// Tol is how far two edges may differ and still be one: the float error of a
// layout, in DIPs.
const Tol = 0.01

func near(a, b float32) bool { return math.Abs(float64(a-b)) <= Tol }

// Same fails the test unless overlay's bounds equal target's: "this wash
// covers exactly that artwork". what names the pair in the failure.
func Same(t testing.TB, what string, overlay, target ui.Rect) {
	t.Helper()
	if !near(overlay.X, target.X) || !near(overlay.Y, target.Y) || !near(overlay.W, target.W) || !near(overlay.H, target.H) {
		t.Errorf("%s: the overlay is %v, its target %v (differs by x %.2f, y %.2f, w %.2f, h %.2f DIPs)",
			what, overlay, target, overlay.X-target.X, overlay.Y-target.Y, overlay.W-target.W, overlay.H-target.H)
	}
}

// Inside fails the test unless inner lies within outer.
func Inside(t testing.TB, what string, inner, outer ui.Rect) {
	t.Helper()
	if inner.X < outer.X-Tol || inner.Y < outer.Y-Tol || inner.X+inner.W > outer.X+outer.W+Tol || inner.Y+inner.H > outer.Y+outer.H+Tol {
		t.Errorf("%s: %v is not inside %v", what, inner, outer)
	}
}

// InsetBottomLeft fails the test unless inner sits inside outer at the given distances
// from its left and bottom edges (a play disc in a cover's bottom-left
// corner), in DIPs.
func InsetBottomLeft(t testing.TB, what string, inner, outer ui.Rect, left, bottom float32) {
	t.Helper()
	if l, b := inner.X-outer.X, outer.Y+outer.H-(inner.Y+inner.H); !near(l, left) || !near(b, bottom) {
		t.Errorf("%s: %v sits %.2f from the left and %.2f from the bottom of %v; want %.2f and %.2f", what, inner, l, b, outer, left, bottom)
	}
}

// Centred fails the test unless inner's middle is outer's within slack DIPs
// on each axis (the box of a glyph; its optical centre is the picture's
// business, see GlyphMarks).
func Centred(t testing.TB, what string, inner, outer ui.Rect, slack float32) {
	t.Helper()
	dx := (inner.X + inner.W/2) - (outer.X + outer.W/2)
	dy := (inner.Y + inner.H/2) - (outer.Y + outer.H/2)
	if math.Abs(float64(dx)) > float64(slack)+Tol || math.Abs(float64(dy)) > float64(slack)+Tol {
		t.Errorf("%s: %v is off the middle of %v by (%.2f, %.2f) DIPs; allowed %.2f", what, inner, outer, dx, dy, slack)
	}
}

// WholePixels fails the test unless every edge of r falls on a device pixel
// at scale (device pixels per DIP): a mark on a pixel boundary is drawn
// crisp, a half-pixel one blurred at both edges.
func WholePixels(t testing.TB, what string, r ui.Rect, scale float32) {
	t.Helper()
	for _, e := range []struct {
		name string
		v    float32
	}{{"left", r.X}, {"top", r.Y}, {"right", r.X + r.W}, {"bottom", r.Y + r.H}} {
		px := e.v * scale
		if d := math.Abs(float64(px) - math.Round(float64(px))); d > Tol {
			t.Errorf("%s: the %s edge of %v is at %.3f device pixels at scale %v, not a whole pixel", what, e.name, r, px, scale)
		}
	}
}

// ArtBounds is the box, in DIPs, of the saturated pixels of im inside
// search: where a fixture cover (a gradient between two hues; the ground, the
// washes and the glyphs are grey) really is in the picture, found there and
// not in the layout. ok is false when there is none. A mark that lies on the
// artwork by the layout but not by the picture is the bug this is for. search
// must hold the one cover and none beside it.
func ArtBounds(im *image.RGBA, scale float32, search ui.Rect) (r ui.Rect, ok bool) {
	b := im.Bounds()
	x0, y0 := max(b.Min.X, int(search.X*scale)), max(b.Min.Y, int(search.Y*scale))
	x1, y1 := min(b.Max.X, int((search.X+search.W)*scale+0.5)), min(b.Max.Y, int((search.Y+search.H)*scale+0.5))
	minx, miny, maxx, maxy := math.MaxInt, math.MaxInt, -1, -1
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			c := im.RGBAAt(x, y)
			if max(c.R, c.G, c.B)-min(c.R, c.G, c.B) > 60 {
				minx, miny, maxx, maxy = min(minx, x), min(miny, y), max(maxx, x), max(maxy, y)
			}
		}
	}
	if maxx < 0 {
		return ui.Rect{}, false
	}
	return ui.Rect{X: float32(minx) / scale, Y: float32(miny) / scale, W: float32(maxx+1-minx) / scale, H: float32(maxy+1-miny) / scale}, true
}

// OnArtwork fails the test unless r lies exactly on the artwork drawn at it
// in im: the box of the cover's saturated pixels within 6 DIPs of r equals r.
// Use it for a wash or a mark that must cover its cover whole (a row's play
// wash), measured with the mark at rest.
func OnArtwork(t testing.TB, what string, im *image.RGBA, scale float32, r ui.Rect) {
	t.Helper()
	art, ok := ArtBounds(im, scale, ui.Rect{X: r.X - 6, Y: r.Y - 6, W: r.W + 12, H: r.H + 12})
	if !ok {
		t.Errorf("%s: no artwork is drawn near %v (is the cover loaded, and the mark at rest?)", what, r)
		return
	}
	if d := max(abs32(art.X-r.X), abs32(art.Y-r.Y), abs32(art.W-r.W), abs32(art.H-r.H)); d > Tol {
		t.Errorf("%s: the mark is %v, the artwork in the picture %v (off by %.2f DIPs at scale %v)", what, r, art, d, scale)
	}
}

func abs32(v float32) float32 { return float32(math.Abs(float64(v))) }
