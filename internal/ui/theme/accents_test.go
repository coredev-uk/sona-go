package theme

import (
	"math"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
)

// sonaOwn is Sona's own themes and the accent each already has as its
// colour, which it does not offer again ("" for none).
var sonaOwn = map[string]string{"sona": "amber", "graphite": "sky", "paper": "teal", "contrast": "", "oled": "sky"}

func fillOf(th *Theme, accent string, dark bool) ui.Color {
	fill, _ := th.AccentColors(accent, dark)
	return fill
}

// hueOf is a colour's OKLCH hue in degrees.
func hueOf(c ui.Color) float64 {
	lin := func(v uint8) float64 {
		x := float64(v) / 255
		if x <= 0.04045 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	r, g, b := lin(c.R), lin(c.G), lin(c.B)
	l := math.Cbrt(0.4122214708*r + 0.5363325363*g + 0.0514459929*b)
	m := math.Cbrt(0.2119034982*r + 0.6806995451*g + 0.1073969566*b)
	s := math.Cbrt(0.0883024619*r + 0.2817188376*g + 0.6299787005*b)
	a := 1.9779984951*l - 2.4285922050*m + 0.4505937099*s
	bb := 0.0259040371*l + 0.7827717662*m - 0.8086757660*s
	return math.Mod(math.Atan2(bb, a)*180/math.Pi+360, 360)
}

// Every accent Sona's own themes were designed with is offered, in the
// display order, and each combination of theme, accent and variant passes
// Validate: an accent that failed would have been left out, so this fails
// when a designed value drifts out of the floors.
func TestSonasOwnThemesOfferEveryDesignedAccentAndEachValidates(t *testing.T) {
	for id, own := range sonaOwn {
		th := Lookup(id)
		var want []string
		for _, a := range AccentIDs() {
			if a != own {
				want = append(want, a)
			}
		}
		if got := th.Accents(); !slices.Equal(got, want) {
			t.Errorf("%s offers %v, want %v", id, got, want)
		}
		for _, a := range th.Accents() {
			for _, dark := range []bool{false, true} {
				if (dark && !th.HasDark()) || (!dark && !th.HasLight()) {
					continue
				}
				tk := th.WithAccent(a).Tokens(dark)
				for _, p := range Validate(tk) {
					t.Errorf("%s %s %s: %s", id, a, modeName(dark), p)
				}
			}
		}
	}
}

// An accent changes the spot colour and nothing else: amber, amber-ink and
// on-amber, the play roles and the selection that follow them, and no other
// colour token.
func TestAnAccentChangesOnlyTheSpotColour(t *testing.T) {
	spot := map[string]bool{"amber": true, "amber-ink": true, "on-amber": true, "play": true, "play-ink": true, "on-play": true, "selection": true, "caret": true}
	for id := range sonaOwn {
		th := Lookup(id)
		for _, a := range th.Accents() {
			for _, dark := range []bool{false, true} {
				if (dark && !th.HasDark()) || (!dark && !th.HasLight()) {
					continue
				}
				base, got := th.Tokens(dark), th.WithAccent(a).Tokens(dark)
				if got.Color["amber"] == base.Color["amber"] {
					t.Errorf("%s %s %s: the fill did not change", id, a, modeName(dark))
				}
				for name, c := range base.Color {
					if !spot[name] && got.Color[name] != c {
						t.Errorf("%s %s %s: %s changed", id, a, modeName(dark), name)
					}
				}
				for role, spotName := range map[string]string{"play": "amber", "play-ink": "amber-ink", "on-play": "on-amber"} {
					if got.Color[role] != got.Color[spotName] {
						t.Errorf("%s %s %s: %s does not follow %s", id, a, modeName(dark), role, spotName)
					}
				}
			}
		}
	}
}

// Gold is yellow, Amber is orange: the two read as different colours.
func TestGoldIsClearlyDistinctFromAmber(t *testing.T) {
	th := Lookup("graphite")
	for _, dark := range []bool{false, true} {
		amber, gold := fillOf(th, "amber", dark), fillOf(th, "gold", dark)
		if d := hueOf(gold) - hueOf(amber); d < 18 {
			t.Errorf("%s: gold is %.0f° from amber, want at least 18", modeName(dark), d)
		}
	}
}

// The accent chosen is laid over the active theme, restored with the theme,
// and ignored by a theme that does not offer it.
func TestSetAccentLaysItOverTheActiveTheme(t *testing.T) {
	t.Cleanup(func() { SetTheme(DefaultThemeID); SetAccent("") })
	SetTheme("oled")
	SetAccent("lime")
	if got, want := ActiveAccent(), "lime"; got != want {
		t.Fatalf("accent %q, want %q", got, want)
	}
	if got, want := C("amber"), fillOf(Lookup("oled"), "lime", true); got != want {
		t.Errorf("amber %v, want %v", got, want)
	}
	SetTheme("nord") // a community palette offers none
	if ActiveAccent() != "" || C("amber") == fillOf(Lookup("oled"), "lime", true) {
		t.Errorf("nord took an accent: %q", ActiveAccent())
	}
	SetTheme("oled")
	if ActiveAccent() != "lime" {
		t.Errorf("lime was not kept for oled: %q", ActiveAccent())
	}
	SetAccent("")
	if C("amber") != ui.Hex("#5492ff") {
		t.Errorf("the theme's own colour is %v", C("amber"))
	}
}

// A user's theme gives its own amber: a file with accents is refused.
func TestAUserThemeCannotGiveAccents(t *testing.T) {
	th, ps := ParseTheme([]byte(`{"format":1,"id":"mine","name":"Mine","dark":{},"accents":{"sky":{"dark":{"amber":"#6cc9ff","amber-ink":"#6cc9ff","on-amber":"#111111"}}}}`))
	if th != nil || !has(ps, ProblemBadValue, "accents") {
		t.Errorf("accepted or wrong problem: %v %v", th, ps)
	}
}

// A malformed accent in a Sona file is a bug the tests catch: an unknown
// accent, a missing token or a variant the theme lacks is refused.
func TestMalformedAccentsAreRefused(t *testing.T) {
	head := `{"format":1,"id":"mine","name":"Mine","dark":{},"accents":`
	for name, accents := range map[string]string{
		"an unknown accent":   `{"purple":{"dark":{"amber":"#6cc9ff","amber-ink":"#6cc9ff","on-amber":"#111111"}}}`,
		"a missing token":     `{"sky":{"dark":{"amber":"#6cc9ff","amber-ink":"#6cc9ff"}}}`,
		"an extra token":      `{"sky":{"dark":{"amber":"#6cc9ff","amber-ink":"#6cc9ff","on-amber":"#111111","ink":"#ffffff"}}}`,
		"a missing variant":   `{"sky":{}}`,
		"a variant not given": `{"sky":{"dark":{"amber":"#6cc9ff","amber-ink":"#6cc9ff","on-amber":"#111111"},"light":{"amber":"#0c6a9e","amber-ink":"#0c6a9e","on-amber":"#ffffff"}}}`,
	} {
		if th, ps := parseTheme([]byte(head+accents+`}`), state.base, true); th != nil || len(ps) == 0 {
			t.Errorf("%s: accepted (%v)", name, ps)
		}
	}
}

// An accent that fails the floors is left out, never offered.
func TestAnAccentThatFailsValidateIsNotOffered(t *testing.T) {
	th, ps := parseTheme([]byte(`{"format":1,"id":"mine","name":"Mine","dark":{},"accents":{
		"sky":{"dark":{"amber":"#6cc9ff","amber-ink":"#6cc9ff","on-amber":"#111111"}},
		"rose":{"dark":{"amber":"#2a2f2c","amber-ink":"#2a2f2c","on-amber":"#111111"}}}}`), state.base, true)
	if th == nil {
		t.Fatal(ps)
	}
	if got := th.Accents(); !slices.Equal(got, []string{"sky", "rose"}[:1]) {
		t.Errorf("offers %v, want [sky]", got)
	}
}

// High contrast keeps its 7:1 for the accent's text too.
func TestHighContrastAccentsKeep7To1(t *testing.T) {
	th := Lookup("contrast")
	for _, a := range th.Accents() {
		for _, dark := range []bool{false, true} {
			tk := th.WithAccent(a).Tokens(dark)
			for _, bg := range []string{"ground", "surface", "raised", "hover", "active"} {
				if c := Contrast(tk.Color["amber-ink"], tk.Color[bg]); c < 7 {
					t.Errorf("%s %s: amber-ink on %s is %.2f:1", a, modeName(dark), bg, c)
				}
			}
		}
	}
}
