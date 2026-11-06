// Package palette works out the colours of a cover-coloured page (the album
// page) from the cover's own colours: Apple's (the background the cover sits
// on and the text colours Apple sets over it, services.ArtworkColors) and
// Go's dominant colours (services.Header.Colors). It is v0.2.0's
// features/album/palette.ts.
//
//   - The sleeve is printed in the cover's background colour, kept one tone
//     step off the ground, and no lighter than a mid tone in the dark theme
//     (a white sleeve would be the brightest thing in a dark window).
//   - Text on the sleeve starts from Apple's text colours and is moved in
//     lightness, hue kept, until it reaches 4.5:1 on the sleeve and on its
//     hover and pressed tones; Apple's colours are used as they are when they
//     already do.
//   - The cover's most colourful colour, when it has one, takes amber's job on
//     the page: the play control and the playing row, and the artist's link.
//     Its fill is never less distinct from the ground than amber's, its glyph
//     keeps 4.5:1, and as text it keeps 4.5:1 on every row tone. A greyscale
//     cover keeps Sona's amber.
//
// Everything is worked out per theme from the theme's tokens (Themes, kept
// equal to tokens.json by a test), so the result is plain colours, applied
// with theme.Scope: nothing is blended at paint time and nothing depends on
// the backdrop image. A palette is built once per cover, theme and layout,
// never per frame.
package palette

import (
	"math"
	"strconv"
	"strings"
)

// rgb is sRGB, each channel 0–1.
type rgb [3]float64

type lch struct{ l, c, h float64 }

// Theme names the two appearances.
type Theme int

// The themes.
const (
	Light Theme = iota
	Dark
)

// Tokens are the theme tokens the palette is checked against: the same
// colours as tokens.json (a test keeps them equal).
type Tokens struct {
	Ground, Hover, Active, Ink, Amber, OnAmber string
}

// Themes is the tokens of each theme.
var Themes = [2]Tokens{
	Light: {Ground: "#f4f5f3", Hover: "#e4e7e3", Active: "#dbdfda", Ink: "#161917", Amber: "#e5a31b", OnAmber: "#1a1405"},
	Dark:  {Ground: "#121513", Hover: "#222724", Active: "#2a302c", Ink: "#eef1ee", Amber: "#f0b43c", OnAmber: "#1a1405"},
}

// The contrasts and steps the palette holds to.
const (
	// TextContrast is text against its background (WCAG AA, body text).
	TextContrast = 4.5
	// UIContrast is controls and their edges against their background
	// (WCAG AA, non-text).
	UIContrast = 3
	// sleeveStep: the sleeve stays at least this far from the ground in OKLCH
	// lightness (one tone step).
	sleeveStep = 0.05
	// darkSleeveMax is the sleeve's lightest tone in the dark theme.
	darkSleeveMax = 0.36
	// accentMinChroma: below this OKLCH chroma a colour reads as grey and is
	// no accent.
	accentMinChroma = 0.06
	// mutedStep: muted text must still read as a step below the ink, not the
	// same colour.
	mutedStep = 1.15
	// sleeveHeadroom is the contrast black or white keeps on a sleeve and its
	// pressed tone, so that ink and a muted step below it both reach 4.5:1
	// there.
	sleeveHeadroom = 5.8

	white = "#ffffff"
	black = "#000000"
)

// Lightness is a colour's OKLCH lightness, 0–1.
func Lightness(hex string) float64 { return oklch(mustRGB(hex)).l }

// ParseHex reads "#rrggbb" (the # optional).
func ParseHex(s string) (c [3]float64, ok bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "#")
	if len(s) != 6 {
		return c, false
	}
	n, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return c, false
	}
	return [3]float64{float64(n>>16&255) / 255, float64(n>>8&255) / 255, float64(n&255) / 255}, true
}

func mustRGB(hex string) rgb {
	c, ok := ParseHex(hex)
	if !ok {
		panic("palette: not a colour: " + hex)
	}
	return c
}

func toHex(c rgb) string {
	const digits = "0123456789abcdef"
	b := make([]byte, 1, 7)
	b[0] = '#'
	for _, v := range c {
		n := int(math.Round(math.Min(1, math.Max(0, v)) * 255))
		b = append(b, digits[n>>4], digits[n&15])
	}
	return string(b)
}

func toLinear(v float64) float64 {
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

func fromLinear(v float64) float64 {
	if v <= 0.0031308 {
		return v * 12.92
	}
	return 1.055*math.Pow(v, 1/2.4) - 0.055
}

func clamp01(v float64) float64 { return math.Min(1, math.Max(0, v)) }

// Luminance is WCAG relative luminance.
func Luminance(hex string) float64 {
	c := mustRGB(hex)
	return 0.2126*toLinear(c[0]) + 0.7152*toLinear(c[1]) + 0.0722*toLinear(c[2])
}

// Contrast is the WCAG contrast ratio of two colours, 1–21.
func Contrast(a, b string) float64 {
	la, lb := Luminance(a), Luminance(b)
	return (math.Max(la, lb) + 0.05) / (math.Min(la, lb) + 0.05)
}

// toOklab is OKLab (Björn Ottosson) from sRGB.
func toOklab(c rgb) [3]float64 {
	lr, lg, lb := toLinear(c[0]), toLinear(c[1]), toLinear(c[2])
	l := math.Cbrt(0.4122214708*lr + 0.5363325363*lg + 0.0514459929*lb)
	m := math.Cbrt(0.2119034982*lr + 0.6806995451*lg + 0.1073969566*lb)
	s := math.Cbrt(0.0883024619*lr + 0.2817188376*lg + 0.6299787005*lb)
	return [3]float64{
		0.2104542553*l + 0.793617785*m - 0.0040720468*s,
		1.9779984951*l - 2.428592205*m + 0.4505937099*s,
		0.0259040371*l + 0.7827717662*m - 0.808675766*s,
	}
}

// fromOklabLinear is linear sRGB from OKLab; channels may fall outside 0–1
// (out of gamut).
func fromOklabLinear(lab [3]float64) [3]float64 {
	L, a, b := lab[0], lab[1], lab[2]
	cube := func(v float64) float64 { return v * v * v }
	l := cube(L + 0.3963377774*a + 0.2158037573*b)
	m := cube(L - 0.1055613458*a - 0.0638541728*b)
	s := cube(L - 0.0894841775*a - 1.291485548*b)
	return [3]float64{
		4.0767416621*l - 3.3077115913*m + 0.2309699292*s,
		-1.2684380046*l + 2.6097574011*m - 0.3413193965*s,
		-0.0041960863*l - 0.7034186147*m + 1.707614701*s,
	}
}

func oklch(c rgb) lch {
	lab := toOklab(c)
	return lch{l: lab[0], c: math.Hypot(lab[1], lab[2]), h: math.Atan2(lab[2], lab[1])}
}

func inGamut(c [3]float64) bool {
	for _, v := range c {
		if v < -1e-4 || v > 1+1e-4 {
			return false
		}
	}
	return true
}

func linearToHex(lin [3]float64) string {
	return toHex(rgb{fromLinear(clamp01(lin[0])), fromLinear(clamp01(lin[1])), fromLinear(clamp01(lin[2]))})
}

// fromOklch is the colour at OKLCH x, its chroma reduced until it fits sRGB.
func fromOklch(x lch) string {
	L := clamp01(x.l)
	at := func(chroma float64) [3]float64 {
		return fromOklabLinear([3]float64{L, chroma * math.Cos(x.h), chroma * math.Sin(x.h)})
	}
	lin := at(x.c)
	if !inGamut(lin) {
		lo, hi := 0.0, x.c
		for range 24 {
			mid := (lo + hi) / 2
			if inGamut(at(mid)) {
				lo = mid
			} else {
				hi = mid
			}
		}
		lin = at(lo)
	}
	return linearToHex(lin)
}

// Mix is b mixed into a by t (0–1) in OKLab, as CSS color-mix(in oklab, a, b t).
func Mix(a, b string, t float64) string {
	x, y := toOklab(mustRGB(a)), toOklab(mustRGB(b))
	var lab [3]float64
	for i := range lab {
		lab[i] = x[i] + (y[i]-x[i])*t
	}
	return linearToHex(fromOklabLinear(lab))
}

// Over is fg at alpha painted over bg, as a browser composites it (in sRGB).
func Over(bg, fg string, alpha float64) string {
	b, f := mustRGB(bg), mustRGB(fg)
	var out rgb
	for i := range out {
		out[i] = b[i] + (f[i]-b[i])*alpha
	}
	return toHex(out)
}

// nearest is the colour nearest hex in OKLCH lightness (hue and chroma kept,
// as far as sRGB allows) for which ok holds; false when no lightness does.
func nearest(hex string, ok func(string) bool) (string, bool) {
	if ok(hex) {
		return hex, true
	}
	x := oklch(mustRGB(hex))
	const steps = 200
	for i := 1; i <= steps; i++ {
		d := float64(i) / steps
		if x.l+d > 1+1.0/steps && x.l-d < -1.0/steps {
			break
		}
		for _, L := range [2]float64{x.l - d, x.l + d} {
			if L < 0 || L > 1 {
				continue
			}
			if cand := fromOklch(lch{l: L, c: x.c, h: x.h}); ok(cand) {
				return cand, true
			}
		}
	}
	for _, end := range [2]string{black, white} {
		if ok(end) {
			return end, true
		}
	}
	return "", false
}

// worst is the lowest contrast c has against any of bgs.
func worst(c string, bgs []string) float64 {
	w := math.Inf(1)
	for _, b := range bgs {
		w = math.Min(w, Contrast(c, b))
	}
	return w
}

// EnsureContrast is fg, or the nearest colour of its hue that keeps min:1
// against every background; black or white, whichever does better, when none
// does.
func EnsureContrast(fg string, bgs []string, minimum float64) string {
	if c, ok := nearest(fg, func(c string) bool { return worst(c, bgs) >= minimum }); ok {
		return c
	}
	if worst(black, bgs) >= worst(white, bgs) {
		return black
	}
	return white
}

// AccentOf is the most colourful of candidates, or "" when all of them read
// as grey (or are no colour).
func AccentOf(candidates []string) string {
	best, chroma := "", accentMinChroma
	for _, c := range candidates {
		rgb, ok := ParseHex(c)
		if !ok {
			continue
		}
		if ch := oklch(rgb).c; ch >= chroma {
			best, chroma = c, ch
		}
	}
	return best
}

// poleOf is the black or white that text on surface sits towards: whichever
// contrasts more.
func poleOf(surface string) string {
	if Contrast(black, surface) >= Contrast(white, surface) {
		return black
	}
	return white
}

// tonesOf is a surface with its hover and pressed tones: steps towards the
// side its text sits on, as the theme's own.
func tonesOf(surface string) []string {
	pole := poleOf(surface)
	return []string{surface, Mix(surface, pole, 0.06), Mix(surface, pole, 0.1)}
}

// SleeveSurface is the sleeve's surface for a cover background bg in th: the
// cover's colour, one tone step off the ground, no lighter than a mid tone in
// the dark theme, and far enough from a mid grey that text on it and on its
// hover and pressed tones can reach 4.5:1 with room for a muted step.
func SleeveSurface(bg string, th Theme) string {
	x := oklch(mustRGB(bg))
	ground := Lightness(Themes[th].Ground)
	l := x.l
	if th == Dark {
		l = math.Min(l, darkSleeveMax)
	}
	if math.Abs(l-ground) < sleeveStep {
		if th == Dark {
			l = ground + sleeveStep
		} else {
			l = ground - sleeveStep
		}
	}
	fits := func(s string) bool {
		sl := Lightness(s)
		return math.Abs(sl-ground) >= sleeveStep-0.005 &&
			(th == Light || sl <= darkSleeveMax+0.005) &&
			worst(poleOf(s), tonesOf(s)) >= sleeveHeadroom
	}
	start := fromOklch(lch{l: l, c: x.c, h: x.h})
	if c, ok := nearest(start, fits); ok {
		return c
	}
	return start
}

// Sleeve is the sleeve's colours: surface, its row tones, and the text and
// lines on it. Every colour is "#rrggbb".
type Sleeve struct {
	Surface, Hover, Active string
	Ink, InkMuted          string
	Line, ControlLine      string
	// AccentInk is the accent as text on the sleeve (links); "" without an
	// accent.
	AccentInk string
}

// Accent is the accent's colours on the page: amber's three roles.
type Accent struct {
	// Fill is the play control's fill and the equaliser (amber).
	Fill string
	// OnFill is the glyph on the fill (on-amber).
	OnFill string
	// Ink is the accent as text on the ground and on rows (amber-ink).
	Ink string
}

// Album is a cover's colours on the album page.
type Album struct {
	Sleeve Sleeve
	// Accent is nil for a greyscale cover: it keeps Sona's amber.
	Accent *Accent
}

// Cover is Apple's colours for a cover ("#rrggbb", each "" when Apple gives
// none; services.ArtworkColors converts to it). Apple's text colours are its
// choices, not checked for contrast: this package checks them.
type Cover struct {
	Background                 string
	Text1, Text2, Text3, Text4 string
}

// PageBackgrounds are the backgrounds page text sits on: ground, row hover,
// pressed, and a selected row (ink at 12%).
func PageBackgrounds(th Theme) []string {
	t := Themes[th]
	return []string{t.Ground, t.Hover, t.Active, Over(t.Ground, t.Ink, 0.12)}
}

// sleeveColors: primary is Apple's primary text colour; softer its secondary
// and softer ones, in order of preference.
func sleeveColors(bg, primary string, softer []string, th Theme) Sleeve {
	tones := tonesOf(SleeveSurface(bg, th))
	surface, hover, active := tones[0], tones[1], tones[2]
	readable := func(c string, minimum float64) bool { return worst(c, tones) >= minimum }
	// Apple's primary when it reads on the sleeve as it is, with room for a
	// muted step under it; else (the sleeve moved off the cover's colour) black
	// or white with a trace of the sleeve.
	ink := primary
	if primary == "" || !readable(primary, TextContrast*mutedStep) {
		ink = EnsureContrast(Mix(poleOf(surface), surface, 0.03), tones, TextContrast)
	}
	derived := Mix(ink, surface, 0.35)
	muted := ""
	for _, c := range append(append([]string{}, softer...), derived) {
		if readable(c, TextContrast) && Contrast(c, ink) >= mutedStep {
			muted = c
			break
		}
	}
	if muted == "" {
		muted = EnsureContrast(derived, tones, TextContrast)
	}
	return Sleeve{
		Surface: surface, Hover: hover, Active: active,
		Ink: ink, InkMuted: muted,
		Line:        Mix(surface, ink, 0.14),
		ControlLine: EnsureContrast(Mix(surface, ink, 0.5), []string{surface, hover}, UIContrast),
	}
}

func accentColors(accent string, th Theme, under []string) *Accent {
	t := Themes[th]
	// The fill (play control, equaliser) is never less distinct from what it
	// sits on than amber is from the ground, nor than 3:1 where amber is.
	floor := math.Min(UIContrast, Contrast(t.Amber, t.Ground))
	glyph := func(c string) string {
		if Contrast(white, c) > Contrast(t.OnAmber, c) {
			return white
		}
		return t.OnAmber
	}
	fillOK := func(c string) bool { return worst(c, under) >= floor && Contrast(glyph(c), c) >= TextContrast }
	fill, ok := nearest(accent, fillOK)
	if !ok {
		fill = t.Amber
	}
	on := glyph(fill)
	if fill == t.Amber {
		on = t.OnAmber
	}
	return &Accent{Fill: fill, OnFill: on, Ink: EnsureContrast(accent, PageBackgrounds(th), TextContrast)}
}

func validHex(c string) bool {
	_, ok := ParseHex(c)
	return ok
}

// ForAlbum is the album's colours in th, from Apple's colours for the cover
// and Go's dominant colours (Header.Colors). playOnSleeve: the play control
// sits on the sleeve (the narrow layout), not beside it. It reports false
// when neither gives a background: the sleeve then keeps the plain wash of its
// backdrop.
func ForAlbum(apple *Cover, dominant []string, th Theme, playOnSleeve bool) (Album, bool) {
	var a Cover
	if apple != nil {
		a = *apple
	}
	bg := ""
	for _, c := range append([]string{a.Background}, dominant...) {
		if validHex(c) {
			bg = c
			break
		}
	}
	if bg == "" {
		return Album{}, false
	}
	// Apple's text colours are picked for its background, so only with it.
	own := Cover{}
	if bg == a.Background {
		own = a
	}
	primary := ""
	if validHex(own.Text1) {
		primary = own.Text1
	}
	var softer []string
	for _, c := range []string{own.Text2, own.Text3} {
		if validHex(c) {
			softer = append(softer, c)
		}
	}
	sleeve := sleeveColors(bg, primary, softer, th)
	var candidates []string
	for _, c := range append([]string{a.Background, a.Text1, a.Text2, a.Text3, a.Text4}, dominant...) {
		if validHex(c) {
			candidates = append(candidates, c)
		}
	}
	hue := AccentOf(candidates)
	if hue == "" {
		return Album{Sleeve: sleeve}, true
	}
	sleeve.AccentInk = EnsureContrast(hue, []string{sleeve.Surface, sleeve.Hover, sleeve.Active}, TextContrast)
	under := []string{Themes[th].Ground}
	if playOnSleeve {
		under = append(under, sleeve.Surface)
	}
	return Album{Sleeve: sleeve, Accent: accentColors(hue, th, under)}, true
}
