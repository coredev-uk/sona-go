package theme

import (
	"runtime"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

func TestEveryMappedNameIsALucideIcon(t *testing.T) {
	if len(symbolOf) < 20 {
		t.Fatalf("%d icons mapped", len(symbolOf))
	}
	for name, sym := range symbolOf {
		canon, ok := IconName(name)
		if !ok || canon != name {
			t.Errorf("%s (%s) is no canonical Lucide name: %q, %v", name, sym, canon, ok)
			continue
		}
		Icon(name) // panics under test for a name that is no icon
	}
}

func TestSymbolNameTakesWhatIconTakes(t *testing.T) {
	for in, want := range map[string]string{
		"play": "play.fill", "Play": "play.fill", "trash-2": "trash", "Trash2": "trash",
		"ChevronRight": "chevron.right", "volume-x": "speaker.slash.fill", "repeat-1": "repeat.1",
	} {
		if got, ok := SymbolName(in); !ok || got != want {
			t.Errorf("SymbolName(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	// Sona's own glyphs, the rest and what is no icon stay Lucide.
	for _, in := range []string{"hat-glasses", "disc-3", "circle-alert", "not-an-icon"} {
		if got, ok := SymbolName(in); ok {
			t.Errorf("SymbolName(%q) = %q", in, got)
		}
	}
}

func TestSymbolsAreMacOSOnly(t *testing.T) {
	if symbols != (runtime.GOOS == "darwin") {
		t.Errorf("symbols is %v on %s", symbols, runtime.GOOS)
	}
}

// withSymbols runs the test with Glyph drawing SF Symbols, or Lucide. Off
// macOS a symbol draws its Lucide fallback in its own colour.
func withSymbols(t *testing.T, on bool) {
	t.Helper()
	was := symbols
	symbols = on
	t.Cleanup(func() { symbols = was })
}

// glyphApp is a button named by its text, with a glyph in a text colour
// the glyph is told about.
func glyphApp(name string, o GlyphOpts) func(c *ui.Context) {
	return func(c *ui.Context) {
		Apply(c)
		ui.Row(c).Padding(20).Children(func() {
			b := ui.ButtonBase(c.Key("b")).Gap(8).Padding(8).TextColor(ui.RGB(0, 0, 255))
			b.Children(func() {
				Glyph(c.Key("g"), name, 32, o)
				ui.Text(c.Key("t"), "Add")
			})
		})
	}
}

func TestGlyphIsNotReadBesideTheButtonsName(t *testing.T) {
	for _, on := range []bool{false, true} {
		withSymbols(t, on)
		tt := ui.NewTester(glyphApp("plus", GlyphOpts{Color: ui.RGB(0, 0, 255), Inherit: true}), 240, 120)
		tr := access.Read(t, tt)
		if _, ok := tr.Find(ui.RoleButton, "Add"); !ok {
			t.Errorf("symbols %v: no button named Add:\n%s", on, tr.Dump())
		}
		if n := tr.Roles(ui.RoleImage); len(n) != 0 {
			t.Errorf("symbols %v: the glyph is on the tree: %v", on, n)
		}
	}
}

func TestNamedGlyphIsAnImage(t *testing.T) {
	for _, on := range []bool{false, true} {
		withSymbols(t, on)
		tt := ui.NewTester(func(c *ui.Context) {
			Apply(c)
			Glyph(c, "star", 16, GlyphOpts{Color: C("ink")}).Role(ui.RoleImage).Label("Favourite")
		}, 120, 60)
		if tr := access.Read(t, tt); len(tr.FindAll(ui.RoleImage, "Favourite")) != 1 {
			t.Errorf("symbols %v: no one image named Favourite:\n%s", on, tr.Dump())
		}
	}
}

// inkOf counts the pixels of the picture in col, near enough.
func inkOf(tt *ui.Tester, col [3]uint8) int {
	img := tt.Image()
	n := 0
	for i := 0; i+3 < len(img.Pix); i += 4 {
		px := img.Pix[i : i+3]
		near := true
		for k := range 3 {
			if d := int(px[k]) - int(col[k]); d < -40 || d > 40 {
				near = false
			}
		}
		if near {
			n++
		}
	}
	return n
}

func TestGlyphColour(t *testing.T) {
	red, blue := [3]uint8{255, 0, 0}, [3]uint8{0, 0, 255}
	o := GlyphOpts{Color: ui.RGB(255, 0, 0), Inherit: true}
	// Lucide inherits the button's blue (and its colour transition); a
	// symbol takes no text colour, so Color is what it is drawn in.
	withSymbols(t, false)
	tt := ui.NewTester(glyphApp("plus", o), 240, 120)
	if inkOf(tt, red) > 0 || inkOf(tt, blue) < 40 {
		t.Errorf("Lucide with Inherit: %d red, %d blue pixels", inkOf(tt, red), inkOf(tt, blue))
	}
	withSymbols(t, true)
	tt = ui.NewTester(glyphApp("plus", o), 240, 120)
	// A real symbol at its natural scale is lighter than Lucide's plus.
	if inkOf(tt, red) < 10 {
		t.Errorf("symbol: %d red pixels", inkOf(tt, red))
	}
	// Without Inherit, Lucide is drawn in Color too.
	withSymbols(t, false)
	tt = ui.NewTester(glyphApp("plus", GlyphOpts{Color: ui.RGB(255, 0, 0)}), 240, 120)
	if inkOf(tt, red) < 40 {
		t.Errorf("Lucide in Color: %d red pixels", inkOf(tt, red))
	}
}

func TestGlyphIsSizeSquare(t *testing.T) {
	for _, on := range []bool{false, true} {
		withSymbols(t, on)
		var r ui.Rect
		tt := ui.NewTester(func(c *ui.Context) {
			Apply(c)
			ui.Row(c).AlignItems(ui.Start).Children(func() {
				r = Glyph(c, "search", 20, GlyphOpts{Color: C("ink")}).Bounds()
			})
		}, 120, 60)
		tt.Frame()
		if r.W != 20 || r.H != 20 {
			t.Errorf("symbols %v: the glyph is %v×%v", on, r.W, r.H)
		}
	}
}
