package main

import (
	"bytes"
	"context"
	"errors"
	"hash/fnv"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"net/url"
	"strconv"
	"time"
)

// coverSource is the gallery's artwork: it makes a cover for any src, so a
// component can be shown with pictures without the network or a library. The
// cover is a gradient and rings in hues taken from the src; it is made at
// the exact size asked for. Two query parameters play the loader's states:
// delay=<ms> holds the cover back (the placeholder shows meanwhile) and
// fail=1 fails it.
type coverSource struct{}

var errCover = errors.New("uigallery: this cover is made to fail")

// coverURL is the src of generated cover n. delay holds it back; fail makes
// it fail; nonce makes a src of its own so that the loader asks again.
func coverURL(n int, delay time.Duration, fail bool, nonce int) string {
	q := url.Values{}
	if delay > 0 {
		q.Set("delay", strconv.Itoa(int(delay.Milliseconds())))
	}
	if fail {
		q.Set("fail", "1")
	}
	if nonce > 0 {
		q.Set("n", strconv.Itoa(nonce))
	}
	s := "/art/gallery-" + strconv.Itoa(n) + "/{w}x{h}.jpg"
	if len(q) > 0 {
		s += "?" + q.Encode()
	}
	return s
}

// Template accepts every non-empty src.
func (coverSource) Template(src string) (string, bool) { return src, src != "" }

// Get makes the cover of src at w×h pixels.
func (coverSource) Get(ctx context.Context, src string, w, h int) ([]byte, error) {
	path, q := src, url.Values{}
	if u, err := url.Parse(src); err == nil {
		path, q = u.Path, u.Query()
	}
	if ms, err := strconv.Atoi(q.Get("delay")); err == nil && ms > 0 {
		select {
		case <-time.After(time.Duration(ms) * time.Millisecond):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if q.Get("fail") == "1" {
		return nil, errCover
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, cover(path, w, h), &jpeg.Options{Quality: 88}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Backdrop makes the pre-blurred backdrop of src: a small soft picture the
// backdrop scales up. It honours delay and fail like Get.
func (c coverSource) Backdrop(ctx context.Context, src string) ([]byte, error) {
	return c.Get(ctx, src, 64, 64)
}

// cover paints the picture of a path: a diagonal gradient between two hues,
// with concentric rings off the middle.
func cover(path string, w, h int) image.Image {
	f := fnv.New32a()
	f.Write([]byte(path))
	seed := f.Sum32()
	h1 := float64(seed % 360)
	h2 := math.Mod(h1+50+float64(seed>>9%90), 360)
	cx := float64(w) * (0.3 + float64(seed>>3%40)/100)
	cy := float64(h) * (0.3 + float64(seed>>5%40)/100)
	freq := 14 / float64(max(w, h)) * (1 + float64(seed>>7%4))
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			t := float64(x+y) / float64(w+h)
			r := math.Hypot(float64(x)-cx, float64(y)-cy)
			ring := 0.5 + 0.5*math.Sin(r*freq)
			v := 0.62 + 0.28*ring*(1-t)
			s := 0.55 + 0.25*ring
			img.SetRGBA(x, y, hsv(h1+(h2-h1)*t, s, v))
		}
	}
	return img
}

// hsv is a colour from hue in degrees, saturation and value in 0…1.
func hsv(h, s, v float64) color.RGBA {
	h = math.Mod(h, 360) / 60
	i := math.Floor(h)
	f := h - i
	p, q, t := v*(1-s), v*(1-s*f), v*(1-s*(1-f))
	var r, g, b float64
	switch int(i) % 6 {
	case 0:
		r, g, b = v, t, p
	case 1:
		r, g, b = q, v, p
	case 2:
		r, g, b = p, v, t
	case 3:
		r, g, b = p, q, v
	case 4:
		r, g, b = t, p, v
	default:
		r, g, b = v, p, q
	}
	return color.RGBA{uint8(r * 255), uint8(g * 255), uint8(b * 255), 255}
}
