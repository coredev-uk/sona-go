package artwork

import (
	"bytes"
	"errors"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"math"
	"runtime"
	"sync"
)

// ErrFormat reports artwork bytes that are neither JPEG nor PNG. Apple's
// CDN serves those two for every template Sona fills ({f} becomes jpg), so
// nothing else is handed to a decoder: in particular no TIFF, BMP, GIF or
// WebP parser ever runs on downloaded bytes.
var ErrFormat = errors.New("artwork: not a JPEG or PNG image")

var pngMagic = []byte("\x89PNG\r\n\x1a\n")

func isJPEG(b []byte) bool { return len(b) > 2 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF }

func isPNG(b []byte) bool { return bytes.HasPrefix(b, pngMagic) }

// decodeConfig reads the header of a JPEG or PNG; any other format fails
// with ErrFormat before a decoder sees it.
func decodeConfig(b []byte) (image.Config, error) {
	switch {
	case isJPEG(b):
		return jpeg.DecodeConfig(bytes.NewReader(b))
	case isPNG(b):
		return png.DecodeConfig(bytes.NewReader(b))
	}
	return image.Config{}, ErrFormat
}

// decodeImage decodes a JPEG or PNG; any other format fails with ErrFormat
// before a decoder sees it.
func decodeImage(b []byte) (image.Image, error) {
	switch {
	case isJPEG(b):
		return jpeg.Decode(bytes.NewReader(b))
	case isPNG(b):
		return png.Decode(bytes.NewReader(b))
	}
	return nil, ErrFormat
}

// kernel is a resampling filter: at(t) for |t| < support, zero beyond.
type kernel struct {
	support float64
	at      func(t float64) float64
}

// lanczos is the Lanczos-3 windowed sinc, used for variants and backdrops.
var lanczos = kernel{3, func(t float64) float64 {
	if t == 0 {
		return 1
	}
	x := math.Pi * t
	return 3 * math.Sin(x) * math.Sin(x/3) / (x * x)
}}

// box averages the source pixels each destination pixel covers.
var box = kernel{0.5, func(float64) float64 { return 1 }}

// taps is one output pixel's weights over consecutive input pixels from
// start. The weights sum to 1.
type taps struct {
	start int
	w     []float64
}

// scaleTaps maps n input pixels onto m output pixels with k, widening k by
// the ratio when scaling down so every input pixel contributes.
func scaleTaps(m, n int, k kernel) []taps {
	ratio := float64(n) / float64(m)
	widen := max(ratio, 1)
	radius := math.Ceil(widen * k.support)
	out := make([]taps, m)
	for i := range out {
		centre := (float64(i)+0.5)*ratio - 0.5
		lo := max(0, int(math.Ceil(centre-radius)))
		hi := min(n-1, int(math.Floor(centre+radius)))
		w := make([]float64, hi-lo+1)
		for j := range w {
			if t := math.Abs(float64(lo+j)-centre) / widen; t < k.support {
				w[j] = k.at(t)
			}
		}
		out[i] = taps{lo, normalise(w)}
	}
	return out
}

// blurTaps is a Gaussian of sigma over n pixels with radius ⌈3σ⌉. Taps
// beyond the edge are dropped and the rest renormalised, so edges keep
// their colour instead of darkening.
func blurTaps(n int, sigma float64) []taps {
	r := int(math.Ceil(sigma * 3))
	g := make([]float64, r+1)
	for i := range g {
		g[i] = math.Exp(-float64(i*i) / (2 * sigma * sigma))
	}
	out := make([]taps, n)
	for p := range out {
		lo, hi := max(0, p-r), min(n-1, p+r)
		w := make([]float64, hi-lo+1)
		for j := range w {
			w[j] = g[abs(lo+j-p)]
		}
		out[p] = taps{lo, normalise(w)}
	}
	return out
}

func normalise(w []float64) []float64 {
	var sum float64
	for _, v := range w {
		sum += v
	}
	if sum != 0 {
		for j := range w {
			w[j] /= sum
		}
	}
	return w
}

// convolveX filters each row of src into dst, whose width is len(tp).
// Rows are spread over GOMAXPROCS goroutines.
func convolveX(dst, src *image.RGBA, tp []taps) {
	n := src.Rect.Dx() * 4
	parallel(dst.Rect.Dy(), func(lo, hi int) {
		row := make([]float64, n)
		for y := lo; y < hi; y++ {
			for i, v := range src.Pix[y*src.Stride:][:n] {
				row[i] = float64(v)
			}
			d := dst.Pix[y*dst.Stride:]
			for x, t := range tp {
				var r, g, b, a float64
				s := row[t.start*4:]
				for j, wt := range t.w {
					p := s[j*4 : j*4+4]
					r += wt * p[0]
					g += wt * p[1]
					b += wt * p[2]
					a += wt * p[3]
				}
				store(d[x*4:x*4+4], r, g, b, a)
			}
		}
	})
}

// convolveY filters each column of src into dst, whose height is len(tp).
// It walks whole rows so memory is read in order; output rows are spread
// over GOMAXPROCS goroutines.
func convolveY(dst, src *image.RGBA, tp []taps) {
	n := dst.Rect.Dx() * 4
	parallel(len(tp), func(lo, hi int) {
		acc := make([]float64, n)
		for y := lo; y < hi; y++ {
			clear(acc)
			t := tp[y]
			for j, wt := range t.w {
				for i, v := range src.Pix[(t.start+j)*src.Stride:][:n] {
					acc[i] += wt * float64(v)
				}
			}
			d := dst.Pix[y*dst.Stride:][:n]
			for i := 0; i < n; i += 4 {
				store(d[i:i+4], acc[i], acc[i+1], acc[i+2], acc[i+3])
			}
		}
	})
}

// store writes one filtered pixel. Negative lobes can overshoot, so colour
// is capped at alpha to keep the result premultiplied.
func store(d []uint8, r, g, b, a float64) {
	d[3] = clamp8(a)
	d[0] = min(clamp8(r), d[3])
	d[1] = min(clamp8(g), d[3])
	d[2] = min(clamp8(b), d[3])
}

// parallel splits [0, n) into up to GOMAXPROCS contiguous chunks and runs
// fn on each in its own goroutine.
func parallel(n int, fn func(lo, hi int)) {
	chunks := min(n, runtime.GOMAXPROCS(0))
	var wg sync.WaitGroup
	for c := range chunks {
		wg.Go(func() { fn(c*n/chunks, (c+1)*n/chunks) })
	}
	wg.Wait()
}

// toRGBA copies sr of img into a new RGBA image at the origin, in parallel
// bands. JPEGs decode to YCbCr; converting once up front keeps the filters
// on plain bytes.
func toRGBA(img image.Image, sr image.Rectangle) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, sr.Dx(), sr.Dy()))
	parallel(out.Rect.Dy(), func(lo, hi int) {
		r := image.Rect(0, lo, out.Rect.Dx(), hi)
		draw.Draw(out, r, img, sr.Min.Add(r.Min), draw.Src)
	})
	return out
}

// resample scales sr of src to a new w×h image with k, horizontally then
// vertically.
func resample(src image.Image, sr image.Rectangle, w, h int, k kernel) *image.RGBA {
	in := toRGBA(src, sr)
	sw, sh := sr.Dx(), sr.Dy()
	if w != sw {
		tmp := image.NewRGBA(image.Rect(0, 0, w, sh))
		convolveX(tmp, in, scaleTaps(w, sw, k))
		in = tmp
	}
	if h != sh {
		out := image.NewRGBA(image.Rect(0, 0, w, h))
		convolveY(out, in, scaleTaps(h, sh, k))
		in = out
	}
	return in
}

// fit scales img down, keeping its aspect ratio, to fit within size×size.
// Images that already fit are returned unchanged.
func fit(img image.Image, size int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= size && h <= size {
		return img
	}
	if w >= h {
		w, h = size, max(1, h*size/w)
	} else {
		w, h = max(1, w*size/h), size
	}
	return resample(img, b, w, h, lanczos)
}

// cover scales the centred square crop of img to size×size.
func cover(img image.Image, size int) *image.RGBA {
	b := img.Bounds()
	side := min(b.Dx(), b.Dy())
	x0 := b.Min.X + (b.Dx()-side)/2
	y0 := b.Min.Y + (b.Dy()-side)/2
	return resample(img, image.Rect(x0, y0, x0+side, y0+side), size, size, lanczos)
}

// blur applies a Gaussian blur of sigma to img, which must sit at the
// origin, in place.
func blur(img *image.RGBA, sigma float64) {
	if sigma <= 0 {
		return
	}
	w, h := img.Rect.Dx(), img.Rect.Dy()
	tmp := image.NewRGBA(img.Rect)
	convolveX(tmp, img, blurTaps(w, sigma))
	convolveY(img, tmp, blurTaps(h, sigma))
}

// adjust changes saturation (HSL, by percent, capped at full saturation)
// and then brightness (by percent of full scale) of every pixel of img,
// which must sit at the origin, in place. Both percentages are clamped to
// [-100, 100].
func adjust(img *image.RGBA, saturation, brightness float64) {
	m := 1 + min(max(saturation, -100), 100)/100
	shift := 255 * min(max(brightness, -100), 100) / 100
	w, h := img.Rect.Dx(), img.Rect.Dy()
	for y := range h {
		row := img.Pix[y*img.Stride : y*img.Stride+w*4]
		for i := 0; i < len(row); i += 4 {
			a := float64(row[i+3])
			if a == 0 {
				continue
			}
			// Un-premultiply to 0..1.
			r, g, b := float64(row[i])/a, float64(row[i+1])/a, float64(row[i+2])/a
			hi, lo := max(r, g, b), min(r, g, b)
			if d := hi - lo; d > 0 {
				l := (hi + lo) / 2
				s := d / (hi + lo)
				if l > 0.5 {
					s = d / (2 - hi - lo)
				}
				// Scaling HSL saturation with hue and lightness fixed moves
				// each channel linearly away from the lightness.
				f := min(m, 1/s)
				r, g, b = l+(r-l)*f, l+(g-l)*f, l+(b-l)*f
			}
			row[i] = premul(r*255+shift, a)
			row[i+1] = premul(g*255+shift, a)
			row[i+2] = premul(b*255+shift, a)
		}
	}
}

// premul clamps a straight 0..255 channel and premultiplies it by alpha a.
func premul(v, a float64) uint8 {
	return clamp8(min(max(v, 0), 255) * a / 255)
}

func clamp8(v float64) uint8 { return uint8(min(max(v+0.5, 0), 255)) }

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
