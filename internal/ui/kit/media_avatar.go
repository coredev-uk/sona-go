package kit

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"golang.org/x/text/unicode/norm"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// AvatarTones is how many tints the theme defines (avatar-<n>-bg and -fg).
const AvatarTones = 8

// Initials are the letters that stand for name, one or two: the first letter
// of the first word and of the last ("Thom Yorke" is "TY", "Dan
// Grech-Marguerat" "DG"), or of the only word ("Madonna" is "M", "米津玄師"
// "米"). Words split on white space; punctuation before a word's letter
// ("O'Brien" keeps its O, "(Strings)" gives S) and words with no letter or
// number ("&") are skipped, and a leading "The" is when other words follow
// ("The Bad Plus" is "BP"). A letter is a whole user-perceived character in
// any script, so marks, Hangul syllables and characters outside the BMP are
// never cut; it is upper-cased unless that makes more than one character
// ("ß"). A name with no letter or number gives its first character, an empty
// one "" (v0.2.0's initialsOf).
func Initials(name string) string {
	text := strings.TrimSpace(norm.NFC.String(name))
	var words []string
	for _, w := range strings.Fields(text) {
		w = strings.TrimLeftFunc(w, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
		if w != "" {
			words = append(words, w)
		}
	}
	if len(words) > 1 && strings.EqualFold(words[0], "the") {
		words = words[1:]
	}
	if len(words) == 0 {
		return upperGrapheme(firstGrapheme(text))
	}
	first := upperGrapheme(firstGrapheme(words[0]))
	if len(words) == 1 {
		return first
	}
	return first + upperGrapheme(firstGrapheme(words[len(words)-1]))
}

// AvatarTone is the tint (0 to AvatarTones-1) of name: FNV-1a of its
// lower-case NFC text over UTF-16 code units, so it never changes between
// sessions or Unicode forms (v0.2.0's avatarTone).
func AvatarTone(name string) int {
	text := strings.ToLower(strings.TrimSpace(norm.NFC.String(name)))
	h := uint32(0x811c9dc5)
	for _, u := range utf16.Encode([]rune(text)) {
		h = (h ^ uint32(u)) * 0x01000193
	}
	return int(h % AvatarTones)
}

// upperGrapheme upper-cases g unless that makes more than one character
// ("ß" would be "SS").
func upperGrapheme(g string) string {
	u := strings.ToUpper(g)
	if graphemeCount(u) == 1 && u != "" {
		return u
	}
	return g
}

func graphemeCount(s string) (n int) {
	for s != "" {
		s = s[len(firstGrapheme(s)):]
		n++
	}
	return n
}

// firstGrapheme is the first user-perceived character of s: a base with its
// marks, an emoji sequence (joiners, modifiers, variation selectors, tags),
// a flag (two regional indicators), a Hangul syllable made of jamo. It is the
// subset of UAX #29's grapheme clusters that names need; the standard library
// has no segmenter.
func firstGrapheme(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	if n == 0 {
		return ""
	}
	end := n
	next := func() (rune, int) { return utf8.DecodeRuneInString(s[end:]) }
	switch {
	case r >= 0x1F1E6 && r <= 0x1F1FF: // a flag
		if q, m := next(); q >= 0x1F1E6 && q <= 0x1F1FF {
			end += m
		}
	case isHangul(r):
		for {
			q, m := next()
			if m == 0 || !isJamo(q) {
				break
			}
			end += m
		}
	}
	for {
		q, m := next()
		if m == 0 {
			break
		}
		switch {
		case unicode.In(q, unicode.Mn, unicode.Me, unicode.Mc),
			q == 0x200C,                  // ZWNJ
			q >= 0x1F3FB && q <= 0x1F3FF, // emoji modifiers
			q >= 0xE0020 && q <= 0xE007F: // tags (subdivision flags)
			end += m
			continue
		case q == 0x200D: // ZWJ: joins a following pictograph
			end += m
			if p, pm := next(); isPictograph(p) {
				end += pm
			}
			continue
		}
		break
	}
	return s[:end]
}

func isHangul(r rune) bool {
	return isJamo(r) || r >= 0xAC00 && r <= 0xD7A3
}

func isJamo(r rune) bool {
	return r >= 0x1100 && r <= 0x11FF || r >= 0xA960 && r <= 0xA97F || r >= 0xD7B0 && r <= 0xD7FF
}

func isPictograph(r rune) bool {
	return r >= 0x1F000 && r <= 0x1FAFF || r >= 0x2190 && r <= 0x2BFF || r == 0x2640 || r == 0x2642
}

// PersonAvatar is a person's picture, round, or their initials on one of
// eight tints picked from their name when Apple has no picture (or it fails
// to load): the initials are 38% of the diameter, semibold. It is decorative,
// hidden from assistive technology: the name is always written beside it.
// src is a DTO's art URL (decoded at size × scale like Artwork's), "" for
// none.
func PersonAvatar(c *ui.Context, l *ArtLoader, name, src string, size float32) ui.Element {
	tone := AvatarTone(name)
	initials := Initials(name)
	bg := theme.C(fmt.Sprintf("avatar-%d-bg", tone))
	fg := theme.C(fmt.Sprintf("avatar-%d-fg", tone))
	return Artwork(c, l, src, size, ArtOpts{Round: true, Edge: true, Fallback: func(c *ui.Context) {
		ui.Row(c).Size(size, size).Center().Background(bg).Children(func() {
			ui.Text(c, initials).FontSize(float32(int(size*0.38 + 0.5))).FontWeight(600).TextColor(fg).SingleLine().
				Role(ui.RoleNone) // decoration
		})
	}})
}

// AvatarSize is the size of an Avatar (shadcn's avatar.tsx).
type AvatarSize int

const (
	// AvatarDefault is 32 DIPs.
	AvatarDefault AvatarSize = iota
	// AvatarSmall is 24.
	AvatarSmall
	// AvatarLarge is 40.
	AvatarLarge
)

// Diameter is the avatar's size in DIPs.
func (s AvatarSize) Diameter() float32 {
	switch s {
	case AvatarSmall:
		return 24
	case AvatarLarge:
		return 40
	}
	return 32
}

// AvatarOpts are the options of Avatar.
type AvatarOpts struct {
	// Src is the picture's art URL ("" for none); Fallback the text shown
	// without it (initials), centred on the muted fill.
	Src      string
	Fallback string
	Size     AvatarSize
	// Badge is the small primary dot at the bottom-right with a ring in the
	// background colour (2 DIPs at small, 2.5 default, 3 large; the dot is
	// 8, 10 and 12 DIPs).
	Badge bool
	// Ring draws the 2 DIP ring in the background colour round the avatar,
	// as AvatarGroup's do.
	Ring bool
	// A11yLabel names the avatar for assistive technology (decoration
	// without it).
	A11yLabel string
}

// Avatar is a round picture (32, 24 or 40 DIPs) that falls back to text on
// the muted fill (text-sm, xs at the small size), as shadcn's avatar.
func Avatar(c *ui.Context, l *ArtLoader, o AvatarOpts) ui.Element {
	d := o.Size.Diameter()
	font := float32(14)
	if o.Size == AvatarSmall {
		font = 12
	}
	root := ui.Box(c).Size(d, d)
	root.Children(func() {
		Artwork(c.Key("picture"), l, o.Src, d, ArtOpts{Round: true, A11yLabel: o.A11yLabel, Fallback: func(c *ui.Context) {
			ui.Row(c).Size(d, d).Center().Background(theme.C("muted")).Children(func() {
				ui.Text(c, o.Fallback).FontSize(font).TextColor(theme.C("muted-foreground")).SingleLine().Role(ui.RoleNone)
			})
		}})
		if o.Badge {
			dot, ring := float32(10), float32(2.5)
			switch o.Size {
			case AvatarSmall:
				dot, ring = 8, 2
			case AvatarLarge:
				dot, ring = 12, 3
			}
			ui.Box(c.Key("badge")).Absolute().Right(0).Bottom(0).Size(dot, dot).Radius(dot/2).
				Background(theme.C("primary")).Border(ring, theme.C("background")).IgnoreInput(true)
		}
	})
	if o.Ring {
		bg := theme.C("background")
		root.DrawOver(func(p *ui.Painter, r ui.Rect) { p.Stroke(r, bg, r.W/2, 2) })
	}
	return root
}

// AvatarGroup lays avatars out overlapping by 8 DIPs, each with a ring in the
// background colour, and a "+n" circle for what is not shown (more > 0).
func AvatarGroup(c *ui.Context, l *ArtLoader, avatars []AvatarOpts, more int) ui.Element {
	row := ui.Row(c)
	size := AvatarDefault
	if len(avatars) > 0 {
		size = avatars[0].Size
	}
	row.Children(func() {
		for i, a := range avatars {
			a.Ring = true
			e := Avatar(c.Key(fmt.Sprintf("avatar-%d", i)), l, a)
			if i > 0 {
				e.MarginStart(-8)
			}
		}
		if more > 0 {
			e := AvatarGroupCount(c.Key("count"), more, size)
			if len(avatars) > 0 {
				e.MarginStart(-8)
			}
		}
	})
	return row
}

// AvatarGroupCount is the "+n" circle that ends a group.
func AvatarGroupCount(c *ui.Context, n int, size AvatarSize) ui.Element {
	d := size.Diameter()
	font := float32(14)
	if size == AvatarSmall {
		font = 12
	}
	bg := theme.C("background")
	e := ui.Row(c).Size(d, d).Center().Radius(d / 2).Background(theme.C("muted"))
	e.DrawOver(func(p *ui.Painter, r ui.Rect) { p.Stroke(r, bg, r.W/2, 2) })
	e.Children(func() {
		ui.Text(c, fmt.Sprintf("+%d", n)).FontSize(font).TextColor(theme.C("muted-foreground")).SingleLine()
	})
	return e
}

// SonaMark is the Sona mark: a record label seen from above, an amber disc
// with a spindle hole in the surface colour. Decorative; 20 DIPs when size is
// 0.
func SonaMark(c *ui.Context, size float32) ui.Element {
	if size <= 0 {
		size = 20
	}
	return ui.Box(c).Size(size, size).Draw(func(p *ui.Painter, r ui.Rect) {
		// The 32-unit viewBox: a disc of radius 15 and a hole of radius 3.
		k := r.W / 32
		cx, cy := r.X+r.W/2, r.Y+r.H/2
		p.Fill(ui.Rect{X: cx - 15*k, Y: cy - 15*k, W: 30 * k, H: 30 * k}, theme.C("amber"), 15*k)
		p.Fill(ui.Rect{X: cx - 3*k, Y: cy - 3*k, W: 6 * k, H: 6 * k}, theme.C("surface"), 3*k)
	})
}
