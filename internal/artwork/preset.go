package artwork

import (
	"context"
	"errors"
	"fmt"
	"image"
	"math"
	"strconv"
	"strings"
)

// presetPrefix starts a preset template: the prefix, a style name and two
// colours as six hex digits each, separated by colons
// ("preset:aurora:3a7bd5:d5b43a"). A preset is drawn in Go from its
// template alone, square, and is addressed, cached and served like any
// other template, so every surface that shows artwork shows it unchanged.
const presetPrefix = "preset:"

// PresetStyles are the preset covers' styles, in the order Presets offers
// them. The names are stable: a template stores one.
var PresetStyles = []string{"aurora", "dusk", "horizon", "moon", "glow", "duo"}

// ErrPreset reports a template that is not a valid preset.
var ErrPreset = errors.New("artwork: invalid preset template")

// defaultPalette is what Presets draws from when a playlist has no artwork
// to take colours from: a warm, a cool and a violet accent.
var defaultPalette = []Color{{0xe0, 0x65, 0x3a}, {0x1f, 0x8a, 0x9c}, {0x7a, 0x55, 0xc8}}

// Presets returns the preset covers on offer for a playlist whose artwork
// has the dominant colours palette (DominantColors), one per style in
// PresetStyles order, as templates. A palette with fewer than three
// usable colours is completed by turning the hue of the first, and an
// empty one by a default set, so there are always six distinct covers. The
// colours are the playlist's own; each style sets its own lightness, so a
// dark or washed-out cover still gives a cover with depth.
func Presets(palette []Color) []string {
	var p []Color
	for _, c := range palette {
		// Near-black is what DominantColors answers for an image it could not
		// read: nothing to draw from.
		if len(p) < 3 && toOKLCH(c).l >= 0.1 {
			p = append(p, c)
		}
	}
	if len(p) == 0 {
		p = append(p, defaultPalette...)
	}
	for len(p) < 3 {
		next := toOKLCH(p[0])
		next.h = math.Mod(next.h+float64(len(p))*80, 360)
		p = append(p, fromOKLCH(next, math.Max(next.l, 0.5)))
	}
	pair := [][2]int{{0, 1}, {1, 2}, {2, 0}, {0, 2}, {1, 0}, {2, 1}}
	out := make([]string, len(PresetStyles))
	for i, style := range PresetStyles {
		out[i] = presetPrefix + style + ":" + p[pair[i][0]].hex6() + ":" + p[pair[i][1]].hex6()
	}
	return out
}

// IsPreset reports whether tmpl is a valid preset template.
func IsPreset(tmpl string) bool {
	_, _, _, err := parsePreset(strings.TrimPrefix(tmpl, presetPrefix))
	return strings.HasPrefix(tmpl, presetPrefix) && err == nil
}

// PresetStyle returns the style of a valid preset template, "" for any
// other.
func PresetStyle(tmpl string) string {
	if !IsPreset(tmpl) {
		return ""
	}
	style, _, _, _ := parsePreset(strings.TrimPrefix(tmpl, presetPrefix))
	return style
}

func (c Color) hex6() string { return fmt.Sprintf("%02x%02x%02x", c.R, c.G, c.B) }

// parsePreset validates the part of a preset template after its prefix.
func parsePreset(rest string) (style string, a, b Color, err error) {
	parts := strings.Split(rest, ":")
	if len(parts) != 3 {
		return "", a, b, ErrPreset
	}
	known := false
	for _, s := range PresetStyles {
		known = known || s == parts[0]
	}
	if !known {
		return "", a, b, ErrPreset
	}
	if a, err = parseHex6(parts[1]); err != nil {
		return "", a, b, err
	}
	if b, err = parseHex6(parts[2]); err != nil {
		return "", a, b, err
	}
	return parts[0], a, b, nil
}

func parseHex6(s string) (Color, error) {
	if len(s) != 6 || strings.ToLower(s) != s {
		return Color{}, ErrPreset
	}
	n, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return Color{}, ErrPreset
	}
	return Color{uint8(n >> 16), uint8(n >> 8), uint8(n)}, nil
}

// parsePresetTemplate normalises a preset template and returns its cache key.
func parsePresetTemplate(rest string) (string, string, error) {
	style, a, b, err := parsePreset(rest)
	if err != nil {
		return "", "", err
	}
	tmpl := presetPrefix + style + ":" + a.hex6() + ":" + b.hex6()
	return tmpl, keyOf(tmpl), nil
}

// renderPreset draws a preset template as a side×side JPEG. Rows are drawn
// in parallel; ctx is checked before the work starts.
func renderPreset(ctx context.Context, tmpl string, side int) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	style, a, b, err := parsePreset(strings.TrimPrefix(tmpl, presetPrefix))
	if err != nil {
		return nil, err
	}
	paint := presetPainters[style](toOKLCH(a), toOKLCH(b))
	img := image.NewRGBA(image.Rect(0, 0, side, side))
	inv := 1 / float64(side)
	parallel(side, func(lo, hi int) {
		for y := lo; y < hi; y++ {
			row := img.Pix[y*img.Stride:]
			v := (float64(y) + 0.5) * inv
			for x := range side {
				u := (float64(x) + 0.5) * inv
				c := paint(u, v, inv)
				// Interleaved gradient noise (±0.5 level) hides the bands a
				// smooth gradient makes in 8 bits.
				n := math.Mod(52.9829189*math.Mod(0.06711056*float64(x)+0.00583715*float64(y), 1), 1) - 0.5
				row[x*4+0] = clamp8(c[0]*255 + n)
				row[x*4+1] = clamp8(c[1]*255 + n)
				row[x*4+2] = clamp8(c[2]*255 + n)
				row[x*4+3] = 255
			}
		}
	})
	return encodeJPEG(img, variantQuality)
}

// rgb is an sRGB triple in [0, 1].
type rgb [3]float64

// painter returns the colour at (u, v) in the unit square; px is the size
// of a pixel in it, for edges that are one pixel soft.
type painter func(u, v, px float64) rgb

// presetPainters maps a style to its painter, given the two colours.
var presetPainters = map[string]func(a, b oklch) painter{
	// aurora: a deep ground with two soft glows.
	"aurora": func(a, b oklch) painter {
		ground, glow, haze := tone(b, 0.24, 0.8), tone(a, 0.72, 1), tone(b, 0.5, 1)
		return func(u, v, _ float64) rgb {
			c := ground
			c = over(c, glow, 0.95*falloff(dist(u, v, 0.18, 0.88), 0.95))
			c = over(c, haze, 0.8*falloff(dist(u, v, 0.86, 0.14), 0.7))
			return c
		}
	},
	// dusk: a slanted gradient from a bright top left to a deep bottom right.
	"dusk": func(a, b oklch) painter {
		lut := gradient(tone(a, 0.8, 1), tone(a, 0.56, 1), tone(b, 0.3, 0.9), tone(b, 0.2, 0.7))
		return func(u, v, _ float64) rgb { return lut(0.62*v + 0.38*u) }
	},
	// horizon: a sky over three ridges, each darker than the one behind.
	"horizon": func(a, b oklch) painter {
		sky := gradient(tone(a, 0.84, 1), tone(a, 0.66, 1), tone(b, 0.46, 1))
		ridges := []struct {
			base, amp, freq, phase float64
			col                    rgb
		}{
			{0.58, 0.05, 1.3, 0.4, tone(b, 0.42, 0.9)},
			{0.7, 0.06, 1.0, 2.2, tone(b, 0.32, 0.8)},
			{0.82, 0.05, 1.7, 4.1, tone(b, 0.22, 0.7)},
		}
		return func(u, v, px float64) rgb {
			c := sky(v / 0.8)
			for _, r := range ridges {
				edge := r.base + r.amp*math.Sin(2*math.Pi*r.freq*u+r.phase) + 0.02*math.Sin(2*math.Pi*3.1*u+r.phase)
				c = over(c, r.col, smooth(v-edge, px*1.5))
			}
			return c
		}
	},
	// moon: a large disc, half off the cover, over a graded night.
	"moon": func(a, b oklch) painter {
		night := gradient(tone(b, 0.34, 0.9), tone(b, 0.2, 0.8))
		disc := gradient(tone(a, 0.86, 0.9), tone(a, 0.66, 1))
		return func(u, v, px float64) rgb {
			c := night(v)
			d := dist(u, v, 0.66, 0.4)
			c = over(c, tone(a, 0.5, 0.9), 0.35*falloff(d, 0.62))
			return over(c, disc((v-0.1)/0.6), smooth(0.29-d, px*1.5))
		}
	},
	// glow: a bright core fading to a dark edge.
	"glow": func(a, b oklch) painter {
		lut := gradient(tone(a, 0.86, 0.9), tone(a, 0.56, 1), tone(b, 0.2, 0.7))
		return func(u, v, _ float64) rgb { return lut(dist(u, v, 0.5, 0.44) / 0.78) }
	},
	// duo: two tones cut by a diagonal, each with a little light of its own.
	"duo": func(a, b oklch) painter {
		top := gradient(tone(a, 0.74, 1), tone(a, 0.58, 1))
		bottom := gradient(tone(b, 0.3, 0.9), tone(b, 0.2, 0.8))
		return func(u, v, px float64) rgb {
			t := (u + v) / 2
			// The cut runs from the lower left to the upper right: u+v=0.95.
			return over(top(t/0.95), bottom((t-0.45)/0.55), smooth((u+v)-0.95, px*1.5))
		}
	},
}

// dist is the distance from (u, v) to (cx, cy).
func dist(u, v, cx, cy float64) float64 { return math.Hypot(u-cx, v-cy) }

// falloff is 1 at distance 0 and 0 at r, eased.
func falloff(d, r float64) float64 {
	t := math.Min(math.Max(1-d/r, 0), 1)
	return t * t * (3 - 2*t)
}

// smooth is 0 for x <= -w, 1 for x >= w and eased between: an antialiased
// edge.
func smooth(x, w float64) float64 {
	t := math.Min(math.Max((x+w)/(2*w), 0), 1)
	return t * t * (3 - 2*t)
}

// over blends top onto bottom with opacity a.
func over(bottom, top rgb, a float64) rgb {
	return rgb{bottom[0] + (top[0]-bottom[0])*a, bottom[1] + (top[1]-bottom[1])*a, bottom[2] + (top[2]-bottom[2])*a}
}

// gradient returns the colour at t in [0, 1] (clamped) of stops spread
// evenly, mixed in OKLab, which keeps the middle of a gradient between two
// hues from going grey.
func gradient(stops ...rgb) func(t float64) rgb {
	const n = 512
	lab := make([][3]float64, len(stops))
	for i, s := range stops {
		lab[i] = srgbToOKLab(s)
	}
	lut := make([]rgb, n+1)
	for i := range lut {
		t := float64(i) / n * float64(len(stops)-1)
		k := min(int(t), len(stops)-2)
		f := t - float64(k)
		var m [3]float64
		for j := range m {
			m[j] = lab[k][j] + (lab[k+1][j]-lab[k][j])*f
		}
		lut[i] = oklabToSRGB(m)
	}
	return func(t float64) rgb { return lut[int(math.Min(math.Max(t, 0), 1)*n+0.5)] }
}

// oklch is a colour in OKLab's polar form: lightness 0…1, chroma, hue in
// degrees.
type oklch struct{ l, c, h float64 }

// Chroma bounds of a preset's tones: enough to read as the playlist's
// colour, never neon.
const (
	minChroma = 0.035
	maxChroma = 0.17
)

func toOKLCH(c Color) oklch {
	lab := srgbToOKLab(rgb{float64(c.R) / 255, float64(c.G) / 255, float64(c.B) / 255})
	h := math.Atan2(lab[2], lab[1]) * 180 / math.Pi
	if h < 0 {
		h += 360
	}
	return oklch{l: lab[0], c: math.Hypot(lab[1], lab[2]), h: h}
}

// fromOKLCH returns the colour of c's hue and chroma at lightness l.
func fromOKLCH(c oklch, l float64) Color {
	t := tone(c, l, 1)
	return Color{clamp8(t[0] * 255), clamp8(t[1] * 255), clamp8(t[2] * 255)}
}

// tone is c's hue at lightness l with its chroma (clamped to the bounds
// above) times k, brought into the sRGB gamut by lowering the chroma.
func tone(c oklch, l, k float64) rgb {
	ch := math.Min(math.Max(c.c, minChroma), maxChroma) * k
	for range 24 {
		hr := c.h * math.Pi / 180
		out := oklabToSRGBRaw([3]float64{l, ch * math.Cos(hr), ch * math.Sin(hr)})
		if inGamut(out) {
			return out
		}
		ch *= 0.9
	}
	return oklabToSRGB([3]float64{l, 0, 0})
}

func inGamut(c rgb) bool {
	for _, v := range c {
		if v < -0.0005 || v > 1.0005 {
			return false
		}
	}
	return true
}

func srgbToLinear(v float64) float64 {
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

func linearToSRGB(v float64) float64 {
	if v <= 0.0031308 {
		return 12.92 * v
	}
	return 1.055*math.Pow(v, 1/2.4) - 0.055
}

func srgbToOKLab(c rgb) [3]float64 {
	r, g, b := srgbToLinear(c[0]), srgbToLinear(c[1]), srgbToLinear(c[2])
	l := math.Cbrt(0.4122214708*r + 0.5363325363*g + 0.0514459929*b)
	m := math.Cbrt(0.2119034982*r + 0.6806995451*g + 0.1073969566*b)
	s := math.Cbrt(0.0883024619*r + 0.2817188376*g + 0.6299787005*b)
	return [3]float64{
		0.2104542553*l + 0.7936177850*m - 0.0040720468*s,
		1.9779984951*l - 2.4285922050*m + 0.4505937099*s,
		0.0259040371*l + 0.7827717662*m - 0.8086757660*s,
	}
}

// oklabToSRGBRaw converts without clamping, so the gamut can be checked.
func oklabToSRGBRaw(lab [3]float64) rgb {
	l := lab[0] + 0.3963377774*lab[1] + 0.2158037573*lab[2]
	m := lab[0] - 0.1055613458*lab[1] - 0.0638541728*lab[2]
	s := lab[0] - 0.0894841775*lab[1] - 1.2914855480*lab[2]
	l, m, s = l*l*l, m*m*m, s*s*s
	return rgb{
		linearToSRGB(+4.0767416621*l - 3.3077115913*m + 0.2309699292*s),
		linearToSRGB(-1.2684380046*l + 2.6097574011*m - 0.3413193965*s),
		linearToSRGB(-0.0041960863*l - 0.7034186147*m + 1.7076147010*s),
	}
}

func oklabToSRGB(lab [3]float64) rgb {
	c := oklabToSRGBRaw(lab)
	for i, v := range c {
		c[i] = math.Min(math.Max(v, 0), 1)
	}
	return c
}
