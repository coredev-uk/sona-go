package theme

import (
	"github.com/egoist/mygo/plugins/sfsymbols"
	"github.com/egoist/mygo/ui"
)

// symbolOf maps the Lucide icons of the transport and the common controls,
// by canonical name, to the SF Symbol drawn for each on macOS (docs/design.md,
// "Icons"). The transport and the speaker are solid, as macOS draws them;
// what has a solid form for "on" (the favourite star) takes it with
// GlyphOpts.Filled. Sona's own glyphs (hat-glasses, disc-3, the brand mark)
// and every other icon stay Lucide everywhere.
var symbolOf = map[string]string{
	"play":          "play.fill",
	"pause":         "pause.fill",
	"skip-back":     "backward.fill",
	"skip-forward":  "forward.fill",
	"shuffle":       "shuffle",
	"repeat":        "repeat",
	"repeat-1":      "repeat.1",
	"volume-1":      "speaker.wave.1.fill",
	"volume-2":      "speaker.wave.2.fill",
	"volume-x":      "speaker.slash.fill",
	"list-music":    "music.note.list",
	"search":        "magnifyingglass",
	"x":             "xmark",
	"ellipsis":      "ellipsis",
	"plus":          "plus",
	"trash":         "trash",
	"star":          "star",
	"chevron-down":  "chevron.down",
	"chevron-up":    "chevron.up",
	"chevron-left":  "chevron.left",
	"chevron-right": "chevron.right",
}

// symbolWeight is the symbols' stroke: Regular is the 1.5 DIP stroke of the
// 16 DIP Lucide icons beside them.
const symbolWeight = sfsymbols.Regular

// symbolScale is how much larger than the Lucide icon's box a symbol is
// drawn: at its natural scale a symbol's ink is about a fifth smaller than
// the Lucide icon's in the same box (measured on macOS 26: search 10 DIPs
// against 13, the star 12 against 14), and the two sit side by side.
const symbolScale = 1.2

// symbols reports whether Glyph draws SF Symbols: on macOS only
// (symbols_darwin.go), and tests may turn it on elsewhere, where every
// symbol draws its Lucide fallback.
var symbols = symbolsOS

// SymbolName returns the SF Symbol Glyph draws on macOS for the icon name
// (any name Icon takes), and false for an icon that stays Lucide.
func SymbolName(name string) (string, bool) {
	canon, ok := IconName(name)
	if !ok {
		return "", false
	}
	s, ok := symbolOf[canon]
	return s, ok
}

// GlyphOpts are the options of Glyph.
type GlyphOpts struct {
	// Color is the glyph's colour. An SF Symbol is drawn in it, as it does
	// not take the text colour around it.
	Color ui.Color
	// Inherit leaves the Lucide icon in the text colour around it, which
	// Color then only repeats for the symbol: a button's icon keeps easing
	// with its face.
	Inherit bool
	// Filled draws the solid form: the symbol's .fill variant, and
	// IconFilled for Lucide.
	Filled bool
}

// Glyph shows the icon name, size DIPs square: on macOS the SF Symbol
// SymbolName gives where there is one, and the Lucide icon (Icon) for every
// other name and on every other system. Like an icon it is decoration that
// assistive technology does not see; name one that means something with
// Role(ui.RoleImage).Label(…).
//
//	theme.Glyph(c, "search", 16, theme.GlyphOpts{Color: theme.C("ink-faint")})
func Glyph(c *ui.Context, name string, size float32, o GlyphOpts) ui.Element {
	svg := Icon(name)
	if o.Filled {
		svg = IconFilled(name)
	}
	if sym, ok := SymbolName(name); symbols && ok {
		// The glyph keeps the Lucide icon's box; the symbol is drawn over
		// symbolScale of it, centred, so that its ink is as large.
		box := ui.Box(c).Size(size, size).Shrink(0)
		box.Children(func() {
			side := size * symbolScale
			s := sfsymbols.Symbol(c, sym).Size(side).Color(o.Color).Weight(symbolWeight).Natural().Fallback(svg)
			if o.Filled {
				s = s.Fill()
			}
			// A symbol names itself ("play.fill") as an image, which a
			// button with a label would read out with it ("plus Add"): it is
			// cleared, so the symbol is the decoration a Lucide icon is.
			s.Role(ui.RoleAuto).Label("").Absolute().Left((size - side) / 2).Top((size - side) / 2)
		})
		return box
	}
	e := ui.Icon(c, svg).FontSize(size)
	if !o.Inherit {
		e.TextColor(o.Color)
	}
	return e
}
