package artwork

import (
	"context"
	"fmt"
	"image"
	"math"
	"slices"
)

// Color is an sRGB swatch. It marshals as "#rrggbb".
type Color struct{ R, G, B uint8 }

// Hex returns c as "#rrggbb".
func (c Color) Hex() string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }

func (c Color) String() string { return c.Hex() }

// MarshalText implements encoding.TextMarshaler.
func (c Color) MarshalText() ([]byte, error) { return []byte(c.Hex()), nil }

// DominantColors returns 2–3 swatches for theming, most prominent first,
// derived from the 64 px source. Prominence weighs pixel share and
// saturation, so a small vivid accent can outrank a large grey area.
func (c *Cache) DominantColors(ctx context.Context, template string) ([]Color, error) {
	key, err := c.register(template)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	cached, ok := c.colors[key]
	c.mu.Unlock()
	if ok {
		return slices.Clone(cached), nil
	}
	c.acquire(key)
	defer c.release(key)
	src, _, err := c.source(ctx, key, c.templateOf(key), Sizes[0])
	if err != nil {
		return nil, err
	}
	img, err := c.decode(src)
	if err != nil {
		return nil, err
	}
	cols := dominant(img)
	c.mu.Lock()
	c.colors[key] = cols
	c.mu.Unlock()
	return slices.Clone(cols), nil
}

// dominant buckets pixels into a 4-bit-per-channel histogram and picks the
// highest-scoring bucket averages that are visibly distinct.
func dominant(img image.Image) []Color {
	small := resample(img, img.Bounds(), 32, 32, box)
	type bucket struct {
		n       int
		r, g, b int
		score   float64
		mean    Color
	}
	var hist [4096]bucket
	for i := 0; i+3 < len(small.Pix); i += 4 {
		r, g, b := int(small.Pix[i]), int(small.Pix[i+1]), int(small.Pix[i+2])
		h := &hist[r>>4<<8|g>>4<<4|b>>4]
		h.n++
		h.r += r
		h.g += g
		h.b += b
	}
	var bs []bucket
	for _, h := range hist {
		if h.n == 0 {
			continue
		}
		h.mean = Color{uint8(h.r / h.n), uint8(h.g / h.n), uint8(h.b / h.n)}
		h.score = float64(h.n) * (0.35 + saturation(h.mean))
		bs = append(bs, h)
	}
	slices.SortFunc(bs, func(a, b bucket) int {
		switch {
		case a.score > b.score:
			return -1
		case a.score < b.score:
			return 1
		}
		return 0
	})
	var out []Color
	for _, b := range bs {
		if len(out) == 3 {
			break
		}
		if !slices.ContainsFunc(out, func(o Color) bool { return distance(o, b.mean) < 80 }) {
			out = append(out, b.mean)
		}
	}
	if len(out) == 0 {
		return []Color{{}, {}}
	}
	if len(out) == 1 {
		// A flat image: pair it with a darker shade for contrast.
		p := out[0]
		out = append(out, Color{p.R / 2, p.G / 2, p.B / 2})
	}
	return out
}

// saturation is HSV saturation in [0, 1].
func saturation(c Color) float64 {
	hi := max(c.R, c.G, c.B)
	lo := min(c.R, c.G, c.B)
	if hi == 0 {
		return 0
	}
	return float64(hi-lo) / float64(hi)
}

// distance is Euclidean RGB distance.
func distance(a, b Color) float64 {
	dr := float64(a.R) - float64(b.R)
	dg := float64(a.G) - float64(b.G)
	db := float64(a.B) - float64(b.B)
	return math.Sqrt(dr*dr + dg*dg + db*db)
}
