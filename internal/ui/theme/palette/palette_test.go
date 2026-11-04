package palette

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

var themes = []Theme{Light, Dark}

func name(th Theme) string {
	if th == Dark {
		return "dark"
	}
	return "light"
}

func apple(background string, text ...string) *Cover {
	c := &Cover{Background: background}
	for i, dst := range []*string{&c.Text1, &c.Text2, &c.Text3, &c.Text4} {
		if i < len(text) {
			*dst = text[i]
		}
	}
	return c
}

// covers are at the edges of what Apple sends, with the text colours Apple
// gave or none.
var covers = map[string]*Cover{
	"white":      apple("#ffffff", "#0c0c0c", "#191919", "#3d3d3d", "#474747"),
	"black":      apple("#040a0a", "#dfebe5", "#dfebe5", "#b3beb9", "#b3beb9"),
	"midGrey":    apple("#777777"),
	"cream":      apple("#fffce9", "#050608", "#232429"),
	"lime":       apple("#8acf00", "#050800", "#070a00", "#203000", "#213100"),
	"navyGold":   apple("#3f546a", "#fdc14c", "#fcb92d", "#d7ab52", "#d6a539"),
	"red":        apple("#ff0000"),
	"green":      apple("#00ff00"),
	"blue":       apple("#0000ff", "#ffffff"),
	"yellow":     apple("#ffff00", "#ffff00", "#ffff00"),
	"cyan":       apple("#00ffff"),
	"magenta":    apple("#ff00ff", "#00ff00", "#ff00ff"),
	"clash":      apple("#808080", "#7f7f7f", "#818181"), // Apple's text colours unreadable on its own background
	"ground":     apple(Themes[Light].Ground),
	"darkGround": apple(Themes[Dark].Ground),
}

func atLeast(t *testing.T, what string, got, want float64) {
	t.Helper()
	if got < want {
		t.Errorf("%s: contrast %.3f, want >= %.2f", what, got, want)
	}
}

func TestEveryPairIsReadable(t *testing.T) {
	for _, th := range themes {
		for cname, cover := range covers {
			t.Run(cname+"/"+name(th), func(t *testing.T) {
				a, ok := ForAlbum(cover, nil, th, false)
				if !ok {
					t.Fatal("no colours for a cover with a background")
				}
				s := a.Sleeve
				tones := []string{s.Surface, s.Hover, s.Active}
				for _, bg := range tones {
					atLeast(t, "ink on "+bg, Contrast(s.Ink, bg), TextContrast)
					atLeast(t, "muted on "+bg, Contrast(s.InkMuted, bg), TextContrast)
					if s.AccentInk != "" {
						atLeast(t, "accent ink on "+bg, Contrast(s.AccentInk, bg), TextContrast)
					}
				}
				for _, bg := range []string{s.Surface, s.Hover} {
					atLeast(t, "control line on "+bg, Contrast(s.ControlLine, bg), UIContrast)
				}
				// One tone step off the ground; never a bright slab in the dark theme.
				ground := Lightness(Themes[th].Ground)
				if d := abs(Lightness(s.Surface) - ground); d < 0.045 {
					t.Errorf("sleeve %s is %.3f from the ground, want a tone step", s.Surface, d)
				}
				if th == Dark && Lightness(s.Surface) > 0.37 {
					t.Errorf("dark sleeve %s is lighter than a mid tone", s.Surface)
				}
				if a.Accent == nil {
					return
				}
				tk := Themes[th]
				ac := a.Accent
				atLeast(t, "glyph on fill", Contrast(ac.OnFill, ac.Fill), TextContrast)
				floor := min(UIContrast, Contrast(tk.Amber, tk.Ground))
				atLeast(t, "fill on ground", Contrast(ac.Fill, tk.Ground), floor-1e-9)
				for _, bg := range PageBackgrounds(th) {
					atLeast(t, "accent ink on "+bg, Contrast(ac.Ink, bg), TextContrast)
				}
				// In the narrow layout the play control sits on the sleeve.
				n, _ := ForAlbum(cover, nil, th, true)
				narrow := n.Accent
				atLeast(t, "narrow glyph on fill", Contrast(narrow.OnFill, narrow.Fill), TextContrast)
				if narrow.Fill != tk.Amber {
					atLeast(t, "narrow fill on sleeve", Contrast(narrow.Fill, s.Surface), floor-1e-9)
					atLeast(t, "narrow fill on ground", Contrast(narrow.Fill, tk.Ground), floor-1e-9)
				}
			})
		}
	}
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func TestAppleColoursAreUsedWhenTheyPass(t *testing.T) {
	a, _ := ForAlbum(covers["lime"], nil, Light, false)
	if a.Sleeve.Surface != "#8acf00" {
		t.Errorf("surface %s, want the cover's own colour", a.Sleeve.Surface)
	}
	if a.Sleeve.Ink != "#050800" {
		t.Errorf("ink %s, want Apple's primary", a.Sleeve.Ink)
	}
	// Apple's secondary is the same colour as its primary here; its softer one reads as a step down.
	if a.Sleeve.InkMuted != "#203000" {
		t.Errorf("muted %s, want Apple's softer colour", a.Sleeve.InkMuted)
	}
}

func TestMutedTextIsAStepBelowTheInk(t *testing.T) {
	for _, th := range themes {
		for cname, cover := range covers {
			a, _ := ForAlbum(cover, nil, th, false)
			if c := Contrast(a.Sleeve.Ink, a.Sleeve.InkMuted); c <= 1.1 {
				t.Errorf("%s/%s: ink %s and muted %s are %.2f apart", cname, name(th), a.Sleeve.Ink, a.Sleeve.InkMuted, c)
			}
		}
	}
}

func TestInkTurnsToTheOtherSideWhenTheSleeveLeavesTheCoversColour(t *testing.T) {
	// In the dark theme the cream sleeve darkens, so Apple's near-black text can't stay.
	a, _ := ForAlbum(covers["cream"], nil, Dark, false)
	if Lightness(a.Sleeve.Ink) <= 0.8 {
		t.Errorf("ink %s on the dark cream sleeve is not light", a.Sleeve.Ink)
	}
	atLeast(t, "ink on the pressed tone", Contrast(a.Sleeve.Ink, a.Sleeve.Active), TextContrast)
}

// The sweep runs ForAlbum 432 times on a 6-level grid of cover colours with
// any text colours Apple might send.
func TestEveryPairIsReadableForEveryCoverColourOnA6LevelGrid(t *testing.T) {
	levels := []string{"00", "33", "66", "99", "cc", "ff"}
	var grid []string
	for _, r := range levels {
		for _, g := range levels {
			for _, b := range levels {
				grid = append(grid, "#"+r+g+b)
			}
		}
	}
	failures := 0
	for _, th := range themes {
		for i, bg := range grid {
			// Apple's text colours: none, or arbitrary ones from the grid.
			var text []string
			if i%3 != 0 {
				text = []string{grid[(i*7)%len(grid)], grid[(i*13)%len(grid)]}
			}
			a, _ := ForAlbum(apple(bg, text...), []string{grid[(i*5)%len(grid)]}, th, false)
			s := a.Sleeve
			texts := []string{s.Ink, s.InkMuted}
			if s.AccentInk != "" {
				texts = append(texts, s.AccentInk)
			}
			for _, c := range texts {
				for _, tone := range []string{s.Surface, s.Hover, s.Active} {
					if Contrast(c, tone) < TextContrast {
						failures++
						t.Errorf("%s %s: %s on %s is %.2f", bg, name(th), c, tone, Contrast(c, tone))
					}
				}
			}
			if ac := a.Accent; ac != nil {
				if Contrast(ac.OnFill, ac.Fill) < TextContrast {
					failures++
					t.Errorf("%s %s: glyph %s on %s", bg, name(th), ac.OnFill, ac.Fill)
				}
				for _, b := range PageBackgrounds(th) {
					if Contrast(ac.Ink, b) < TextContrast {
						failures++
						t.Errorf("%s %s: accent ink %s on %s", bg, name(th), ac.Ink, b)
					}
				}
			}
		}
	}
	if failures != 0 {
		t.Errorf("%d unreadable pairs", failures)
	}
}

func TestAccentTakesAmbersJobOnlyForACoverWithColour(t *testing.T) {
	if a, _ := ForAlbum(covers["white"], nil, Light, false); a.Accent != nil {
		t.Errorf("a white cover has an accent: %+v", a.Accent)
	}
	if a, _ := ForAlbum(covers["black"], nil, Dark, false); a.Accent != nil {
		t.Errorf("a black cover has an accent: %+v", a.Accent)
	}
	if a, _ := ForAlbum(covers["lime"], nil, Light, false); a.Accent == nil {
		t.Error("a lime cover has no accent")
	}
	// Gold text on navy: the most colourful gold is the accent.
	a, _ := ForAlbum(covers["navyGold"], nil, Dark, false)
	if a.Accent == nil || a.Accent.Ink != "#fcb92d" {
		t.Errorf("gold on navy: accent %+v, want ink #fcb92d", a.Accent)
	}
}

func TestFallsBackToTheDominantColoursThenToNothing(t *testing.T) {
	a, ok := ForAlbum(nil, []string{"#3366cc", "#ffcc00"}, Light, false)
	if !ok {
		t.Fatal("no colours from the dominant ones")
	}
	// The first dominant colour, darkened so white text and a muted step under it pass on its pressed tone too.
	if c := Contrast(a.Sleeve.Surface, "#3366cc"); c >= 1.5 {
		t.Errorf("sleeve %s is %.2f from the dominant colour", a.Sleeve.Surface, c)
	}
	if Lightness(a.Sleeve.Surface) >= Lightness("#3366cc") {
		t.Errorf("sleeve %s is not darker than the dominant colour", a.Sleeve.Surface)
	}
	atLeast(t, "ink on the pressed tone", Contrast(a.Sleeve.Ink, a.Sleeve.Active), TextContrast)
	if _, ok := ForAlbum(apple(""), []string{"not a colour"}, Light, false); ok {
		t.Error("colours from no colour")
	}
	if _, ok := ForAlbum(nil, nil, Dark, false); ok {
		t.Error("colours from nothing")
	}
}

func TestSleeveSurfaceStepsACoverTheColourOfTheGroundOffIt(t *testing.T) {
	if got := Lightness(SleeveSurface(Themes[Light].Ground, Light)); got >= Lightness(Themes[Light].Ground) {
		t.Errorf("light: %.3f, want darker than the ground", got)
	}
	if got := Lightness(SleeveSurface(Themes[Dark].Ground, Dark)); got <= Lightness(Themes[Dark].Ground) {
		t.Errorf("dark: %.3f, want lighter than the ground", got)
	}
}

func TestEnsureContrast(t *testing.T) {
	if got := EnsureContrast("#000000", []string{"#ffffff"}, 4.5); got != "#000000" {
		t.Errorf("a colour that passes became %s", got)
	}
	// A mid grey is the hardest background.
	for _, fg := range []string{"#7f7f7f", "#ff0000", "#00ff00", "#0000ff"} {
		c := EnsureContrast(fg, []string{"#767676", "#777777", "#787878"}, 4.5)
		atLeast(t, fmt.Sprintf("%s -> %s on #777777", fg, c), Contrast(c, "#777777"), 4.5)
	}
}

func TestAccentOf(t *testing.T) {
	if got := AccentOf([]string{"#777777", "#ff0000", "#cc4444", "x"}); got != "#ff0000" {
		t.Errorf("most colourful: %q", got)
	}
	if got := AccentOf([]string{"#ffffff", "#0c0c0c", "#191919"}); got != "" {
		t.Errorf("greys gave %q", got)
	}
}

func TestOverridesDeclareTheRolesBuiltOnTheSleeve(t *testing.T) {
	a, _ := ForAlbum(covers["lime"], nil, Dark, false)
	m := a.Sleeve.Overrides()
	if m["input"] != ui.Hex(a.Sleeve.ControlLine) || m["ring"] != ui.Hex(a.Sleeve.Ink) || m["accent"] != ui.Hex(a.Sleeve.Hover) {
		t.Errorf("stock roles do not follow the sleeve: %v", m)
	}
	if m["amber-ink"] != ui.Hex(a.Sleeve.AccentInk) || m["play-ink"] != m["amber-ink"] {
		t.Error("the accent as text on the sleeve is not overridden")
	}
	g, _ := ForAlbum(covers["white"], nil, Light, false)
	if _, ok := g.Sleeve.Overrides()["amber-ink"]; ok {
		t.Error("a greyscale sleeve overrides amber's ink")
	}
	acc := a.Accent.Overrides()
	if acc["amber"] != ui.Hex(a.Accent.Fill) || acc["play"] != acc["amber"] || acc["on-play"] != ui.Hex(a.Accent.OnFill) {
		t.Errorf("accent overrides: %v", acc)
	}
}

func TestThemesMatchTheTokens(t *testing.T) {
	hex := func(c ui.Color) string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }
	for _, th := range themes {
		tk := theme.TokensFor(th == Dark)
		want := Themes[th]
		for token, got := range map[string]string{
			"ground": want.Ground, "hover": want.Hover, "active": want.Active,
			"ink": want.Ink, "amber": want.Amber, "on-amber": want.OnAmber,
		} {
			if have := hex(tk.Color[token]); have != got {
				t.Errorf("%s %s: tokens.json has %s, palette.Themes %s", name(th), token, have, got)
			}
		}
	}
}
