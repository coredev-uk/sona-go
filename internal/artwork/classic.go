package artwork

import (
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"math"
	"net/url"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	fx26 "golang.org/x/image/math/fixed"
)

// ClassicStyle is the preset in the look of the cover Apple makes for a new
// playlist: a diagonal gradient of four colours with the playlist's name at
// the top left. Its template is
// "preset:classic:<c1>:<c2>:<c3>:<c4>:<name>", the name in unpadded URL-safe
// base64 (empty for none), so it is drawn from the template alone; a rename
// has to rewrite the template (RenamePreset).
const ClassicStyle = "classic"

// maxClassicName bounds the name a classic template carries, in bytes.
const maxClassicName = 400

type classicSpec struct {
	cols [4]Color
	name string
}

// parseClassic reads the part of a preset template after "classic:".
func parseClassic(rest string) (classicSpec, error) {
	parts := strings.Split(rest, ":")
	if len(parts) != 5 || len(parts[4]) > maxClassicName*2 {
		return classicSpec{}, ErrPreset
	}
	var s classicSpec
	for i := range s.cols {
		c, err := parseHex6(parts[i])
		if err != nil {
			return classicSpec{}, err
		}
		s.cols[i] = c
	}
	b, err := base64.RawURLEncoding.DecodeString(parts[4])
	if err != nil || !utf8.Valid(b) || len(b) > maxClassicName {
		return classicSpec{}, ErrPreset
	}
	s.name = string(b)
	return s, nil
}

func (s classicSpec) template() string {
	var b strings.Builder
	b.WriteString(presetPrefix + ClassicStyle)
	for _, c := range s.cols {
		b.WriteString(":" + c.hex6())
	}
	return b.String() + ":" + base64.RawURLEncoding.EncodeToString([]byte(s.name))
}

// classicTemplate is the classic preset of four colours and a name (cut to
// what a template carries).
func classicTemplate(cols [4]Color, name string) string {
	name = strings.TrimSpace(name)
	for len(name) > maxClassicName {
		_, n := utf8.DecodeLastRuneInString(name)
		name = name[:len(name)-n]
	}
	return classicSpec{cols, name}.template()
}

// RenamePreset returns the preset template tmpl with the name a playlist now
// has: a classic one carries it, any other (and any other template) is
// returned as it is.
func RenamePreset(tmpl, name string) string {
	rest, ok := strings.CutPrefix(tmpl, presetPrefix+ClassicStyle+":")
	if !ok {
		return tmpl
	}
	s, err := parseClassic(rest)
	if err != nil {
		return tmpl
	}
	return classicTemplate(s.cols, name)
}

// GeneratedColors returns the four colours (c1…c4) of the cover Apple
// generated for a playlist, from its template, and whether tmpl is one.
func GeneratedColors(tmpl string) ([]Color, bool) {
	u, err := url.Parse(tmpl)
	if err != nil || !strings.HasPrefix(u.Path, "/image/thumb/gen/") {
		return nil, false
	}
	q := u.Query()
	var out []Color
	for _, k := range []string{"c1", "c2", "c3", "c4"} {
		c, err := parseHex6(strings.ToLower(q.Get(k)))
		if err != nil {
			return nil, false
		}
		out = append(out, c)
	}
	return out, true
}

// classicColours are four colours for the classic preset from a palette
// that has none of Apple's: the first colour's hue from light to near-black,
// turning a little towards warmer or cooler in the middle as Apple's do.
func classicColours(p []oklch) [4]Color {
	a := p[0]
	mid := a
	mid.h = math.Mod(a.h+28, 360)
	return [4]Color{fromOKLCH(a, 0.86), fromOKLCH(a, 0.68), fromOKLCH(mid, 0.4), fromOKLCH(mid, 0.15)}
}

// paintClassic is the classic gradient: light at the top left through the
// two middle colours to the darkest in the bottom right corner.
func paintClassic(c [4]Color) painter {
	lut := gradient(srgbOf(c[0]), srgbOf(c[1]), srgbOf(c[2]), srgbOf(c[3]))
	return func(u, v, _ float64) rgb { return lut(math.Min((u+v)/2*1.08, 1)) }
}

func srgbOf(c Color) rgb { return rgb{float64(c.R) / 255, float64(c.G) / 255, float64(c.B) / 255} }

var (
	nameFontOnce sync.Once
	nameFont     *sfnt.Font
)

// drawName sets name at the top left of img, as Apple's generated cover
// does: bold, black on a light corner and white on a dark one, wrapped to
// three lines. A name with a character the font lacks (other scripts) is
// left off, never drawn as empty boxes.
func drawName(img *image.RGBA, name string, corner Color) {
	name = strings.TrimSpace(name)
	nameFontOnce.Do(func() { nameFont, _ = opentype.Parse(gobold.TTF) })
	if name == "" || nameFont == nil {
		return
	}
	var buf sfnt.Buffer
	for _, r := range name {
		if r == ' ' {
			continue
		}
		if g, err := nameFont.GlyphIndex(&buf, r); err != nil || g == 0 {
			return
		}
	}
	side := float64(img.Rect.Dx())
	size := side * 0.115
	face, err := opentype.NewFace(nameFont, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return
	}
	defer func() { _ = face.Close() }()
	ink := color.RGBA{0, 0, 0, 255}
	if toOKLCH(corner).l < 0.6 {
		ink = color.RGBA{255, 255, 255, 255}
	}
	margin := side * 0.11
	maxW := fx26.I(int(side - 2*margin))
	d := &font.Drawer{Dst: img, Src: image.NewUniform(ink), Face: face}
	lines := wrapName(d, name, maxW, 3)
	lead := fx26.I(int(math.Round(size * 1.12)))
	y := fx26.I(int(margin + size*0.82))
	for _, l := range lines {
		d.Dot = fx26.Point26_6{X: fx26.I(int(margin)), Y: y}
		d.DrawString(l)
		y += lead
	}
}

// wrapName breaks s at spaces into at most max lines no wider than w; a
// word wider than a line is cut, and what does not fit ends in "…".
func wrapName(d *font.Drawer, s string, w fx26.Int26_6, max int) []string {
	var lines []string
	cur := ""
	fits := func(t string) bool { return d.MeasureString(t) <= w }
	for _, word := range strings.Fields(s) {
		next := word
		if cur != "" {
			next = cur + " " + word
		}
		if fits(next) {
			cur = next
			continue
		}
		if cur != "" {
			lines = append(lines, cur)
		}
		for !fits(word) && utf8.RuneCountInString(word) > 1 {
			// Cut a long word at the last rune that fits.
			rs := []rune(word)
			n := len(rs)
			for n > 1 && !fits(string(rs[:n])) {
				n--
			}
			lines = append(lines, string(rs[:n]))
			word = string(rs[n:])
		}
		cur = word
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	if len(lines) > max {
		lines = lines[:max]
		l := []rune(lines[max-1])
		for len(l) > 1 && !fits(string(l)+"…") {
			l = l[:len(l)-1]
		}
		lines[max-1] = string(l) + "…"
	}
	return lines
}

// renderClassic draws a classic preset as a side×side picture.
func renderClassic(ctx context.Context, s classicSpec, side int) (*image.RGBA, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	img := paintImage(side, paintClassic(s.cols))
	drawName(img, s.name, s.cols[0])
	return img, nil
}
