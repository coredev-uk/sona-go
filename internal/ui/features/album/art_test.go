package album

import (
	"bytes"
	"context"
	"hash/fnv"
	"image"
	"image/color"
	"image/jpeg"
)

// coverSource serves a generated cover for any art path: the page's artwork
// loads without a network (the covers are invented, as the uitest fixture's).
type coverSource struct{}

func tint(path string) (a, b color.RGBA) {
	h := fnv.New32a()
	_, _ = h.Write([]byte(path))
	v := h.Sum32()
	a = color.RGBA{R: uint8(v), G: uint8(v >> 8), B: uint8(v >> 16), A: 255}
	b = color.RGBA{R: uint8(v >> 8), G: uint8(v >> 16), B: uint8(v >> 24), A: 255}
	return a, b
}

func (coverSource) Template(p string) (string, bool) { return p, true }

func (coverSource) Get(_ context.Context, tpl string, w, h int) ([]byte, error) {
	return gradient(tpl, max(w, 1), max(h, 1)), nil
}

func (coverSource) Backdrop(_ context.Context, tpl string) ([]byte, error) {
	return gradient(tpl, 16, 16), nil
}

func gradient(seed string, w, h int) []byte {
	a, b := tint(seed)
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			t := float64(x+y) / float64(w+h)
			img.Set(x, y, color.RGBA{
				R: uint8(float64(a.R)*(1-t) + float64(b.R)*t),
				G: uint8(float64(a.G)*(1-t) + float64(b.G)*t),
				B: uint8(float64(a.B)*(1-t) + float64(b.B)*t),
				A: 255,
			})
		}
	}
	var out bytes.Buffer
	_ = jpeg.Encode(&out, img, &jpeg.Options{Quality: 80})
	return out.Bytes()
}
