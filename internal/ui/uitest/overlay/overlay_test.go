package overlay

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"testing"

	"github.com/egoist/mygo/ui"
)

// rec is a testing.TB that keeps what a check reports instead of failing.
type rec struct {
	testing.TB
	errs []string
}

func (r *rec) Helper()                      {}
func (r *rec) Errorf(f string, args ...any) { r.errs = append(r.errs, fmt.Sprintf(f, args...)) }

// failing reports whether check fails.
func failing(check func(t testing.TB)) bool {
	r := &rec{}
	check(r)
	return len(r.errs) > 0
}

func TestSameIsOneRectToTheFloatError(t *testing.T) {
	r := ui.Rect{X: 10, Y: 20, W: 40, H: 40}
	for _, tc := range []struct {
		name string
		o    ui.Rect
		fail bool
	}{
		{"equal", r, false},
		{"float error", ui.Rect{X: 10.004, Y: 19.996, W: 40.003, H: 40}, false},
		{"half a DIP right", ui.Rect{X: 10.5, Y: 20, W: 40, H: 40}, true},
		{"a DIP short", ui.Rect{X: 10, Y: 20, W: 39, H: 40}, true},
		{"a DIP low", ui.Rect{X: 10, Y: 21, W: 40, H: 40}, true},
	} {
		if got := failing(func(t testing.TB) { Same(t, tc.name, tc.o, r) }); got != tc.fail {
			t.Errorf("Same(%s): fails %v, want %v", tc.name, got, tc.fail)
		}
	}
}

func TestPlacementChecks(t *testing.T) {
	outer := ui.Rect{X: 0, Y: 0, W: 100, H: 100}
	if failing(func(t testing.TB) { Inside(t, "in", ui.Rect{X: 8, Y: 56, W: 36, H: 36}, outer) }) {
		t.Error("Inside fails a disc inside")
	}
	if !failing(func(t testing.TB) { Inside(t, "out", ui.Rect{X: 80, Y: 56, W: 36, H: 36}, outer) }) {
		t.Error("Inside passes a disc past the edge")
	}
	if failing(func(t testing.TB) { InsetBottomLeft(t, "corner", ui.Rect{X: 8, Y: 56, W: 36, H: 36}, outer, 8, 8) }) {
		t.Error("InsetBottomLeft fails a disc 8 and 8 from the corner")
	}
	if !failing(func(t testing.TB) { InsetBottomLeft(t, "corner", ui.Rect{X: 8, Y: 55, W: 36, H: 36}, outer, 8, 8) }) {
		t.Error("InsetBottomLeft passes a disc 9 from the bottom")
	}
	if failing(func(t testing.TB) { Centred(t, "mid", ui.Rect{X: 40, Y: 40, W: 20, H: 20}, outer, 0) }) {
		t.Error("Centred fails a centred box")
	}
	if !failing(func(t testing.TB) { Centred(t, "off", ui.Rect{X: 41, Y: 40, W: 20, H: 20}, outer, 0.5) }) {
		t.Error("Centred passes a box 1 DIP off with half a DIP of slack")
	}
}

func TestWholePixelsFollowsTheScale(t *testing.T) {
	r := ui.Rect{X: 45.5, Y: 142, W: 13, H: 12} // the equaliser in a 40 DIP cover, once
	if !failing(func(t testing.TB) { WholePixels(t, "1x", r, 1) }) {
		t.Error("a half-DIP left edge passes at 1x")
	}
	if failing(func(t testing.TB) { WholePixels(t, "2x", r, 2) }) {
		t.Error("a half-DIP left edge fails at 2x")
	}
}

// A cover is found by its colour, in the picture, wherever the layout says.
func TestArtBoundsFindsTheCoverInThePicture(t *testing.T) {
	im := image.NewRGBA(image.Rect(0, 0, 200, 100))
	draw.Draw(im, im.Bounds(), image.NewUniform(color.RGBA{240, 240, 240, 255}), image.Point{}, draw.Src)
	// A 40×40 DIP cover at (20, 10) at scale 2.
	draw.Draw(im, image.Rect(40, 20, 120, 100), image.NewUniform(color.RGBA{200, 60, 90, 255}), image.Point{}, draw.Src)
	// A grey wash over its middle does not move the box.
	draw.Draw(im, image.Rect(60, 40, 100, 80), image.NewUniform(color.RGBA{200, 200, 200, 255}), image.Point{}, draw.Src)

	got, ok := ArtBounds(im, 2, ui.Rect{X: 0, Y: 0, W: 100, H: 50})
	if want := (ui.Rect{X: 20, Y: 10, W: 40, H: 40}); !ok || got != want {
		t.Errorf("ArtBounds = %v, %v; want %v", got, ok, want)
	}
	if failing(func(t testing.TB) { OnArtwork(t, "exact", im, 2, ui.Rect{X: 20, Y: 10, W: 40, H: 40}) }) {
		t.Error("OnArtwork fails a mark exactly on the cover")
	}
	if !failing(func(t testing.TB) { OnArtwork(t, "shifted", im, 2, ui.Rect{X: 20.5, Y: 10, W: 40, H: 40}) }) {
		t.Error("OnArtwork passes a mark half a DIP off the cover")
	}
	if _, ok := ArtBounds(im, 2, ui.Rect{X: 80, Y: 0, W: 20, H: 50}); ok {
		t.Error("ArtBounds finds a cover where there is none")
	}
}
