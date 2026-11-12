package theme

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func hex(s string) ui.Color { return ui.Hex(s) }

// The custom properties of v0.2.0's shared/styles/index.css, light and dark.
var cssColors = map[string][2]string{
	"ground":       {"#f4f5f3", "#121513"},
	"surface":      {"#eceeeb", "#171a18"},
	"raised":       {"#ffffff", "#1e2220"},
	"hover":        {"#e4e7e3", "#222724"},
	"active":       {"#dbdfda", "#2a302c"},
	"line":         {"#dadcd7", "#262b28"},
	"control-line": {"#838984", "#6b736d"},
	"ink":          {"#161917", "#eef1ee"},
	"ink-muted":    {"#545a55", "#a9b0aa"},
	"ink-faint":    {"#5a605b", "#a0a7a1"},
	"amber":        {"#e5a31b", "#f0b43c"},
	"amber-ink":    {"#845500", "#f0b43c"},
	"on-amber":     {"#1a1405", "#1a1405"},
	"danger":       {"#b42318", "#f0705f"},
	"positive":     {"#1c6943", "#5cc48c"},
	"avatar-0-bg":  {"#f5d6d2", "#503532"}, "avatar-0-fg": {"#492826", "#f5dfdc"},
	"avatar-1-bg": {"#eedbc6", "#4b3926"}, "avatar-1-fg": {"#432e14", "#f0e2d3"},
	"avatar-2-bg": {"#dde1c7", "#3c4027"}, "avatar-2-fg": {"#323616", "#e4e7d4"},
	"avatar-3-bg": {"#cde5d4", "#2c4334"}, "avatar-3-fg": {"#1c3a27", "#d8eadd"},
	"avatar-4-bg": {"#c4e6e5", "#214444"}, "avatar-4-fg": {"#093b3b", "#d2eaea"},
	"avatar-5-bg": {"#c9e2f2", "#28414f"}, "avatar-5-fg": {"#163749", "#d5e8f3"},
	"avatar-6-bg": {"#dddbf4", "#3d3a51"}, "avatar-6-fg": {"#322e4b", "#e4e2f5"},
	"avatar-7-bg": {"#f0d5e3", "#4d3542"}, "avatar-7-fg": {"#452839", "#f2dee8"},
}

// The shadcn roles index.css maps onto the tokens (var(--x) targets).
var cssRoles = map[string]string{
	"background": "ground", "foreground": "ink", "card": "raised", "card-foreground": "ink",
	"popover": "raised", "popover-foreground": "ink", "primary": "ink", "primary-foreground": "ground",
	"secondary": "hover", "secondary-foreground": "ink", "muted": "hover", "muted-foreground": "ink-muted",
	"accent": "hover", "accent-foreground": "ink", "destructive": "danger", "border": "line", "input": "control-line",
	"play": "amber", "play-ink": "amber-ink", "on-play": "on-amber",
}

func TestEmbeddedTokensHoldEveryCSSColourInBothModes(t *testing.T) {
	for i, dark := range []bool{false, true} {
		tk, err := state.doc.Tokens(dark)
		if err != nil {
			t.Fatal(err)
		}
		for name, vals := range cssColors {
			if got, want := tk.Color[name], hex(vals[i]); got != want {
				t.Errorf("%s %s = %v, want %v", modeName(dark), name, got, want)
			}
		}
		for role, target := range cssRoles {
			if got, want := tk.Color[role], tk.Color[target]; got != want {
				t.Errorf("%s %s = %v, want %s's %v", modeName(dark), role, got, target, want)
			}
		}
		// Derived: ring is ink at 60%, edge ink at 6% (docs/design.md).
		ink := tk.Color["ink"]
		if got := tk.Color["ring"]; got != ui.RGBA(ink.R, ink.G, ink.B, 0.6) {
			t.Errorf("%s ring = %v", modeName(dark), got)
		}
		if got := tk.Color["edge"]; got.R != ink.R || got.A != ui.RGBA(0, 0, 0, 0.06).A {
			t.Errorf("%s edge = %v", modeName(dark), got)
		}
		if got := tk.Color["selection"]; got.A != ui.RGBA(0, 0, 0, 0.32).A || got.R != tk.Color["amber"].R {
			t.Errorf("%s selection = %v", modeName(dark), got)
		}
	}
}

func TestScalesFollowDesignMD(t *testing.T) {
	light, _ := state.doc.Tokens(false)
	dark, _ := state.doc.Tokens(true)
	want := map[string]TypeStyle{
		"display":   {32, 36, 700, -0.64},
		"title":     {24, 30, 650, -0.36},
		"heading":   {16, 22, 600, 0},
		"body":      {14, 20, 400, 0},
		"label":     {14, 20, 500, 0},
		"secondary": {13, 18, 400, 0},
		"meta":      {12, 16, 450, 0},
	}
	for name, ts := range want {
		if light.Type[name] != ts {
			t.Errorf("type %s = %+v, want %+v", name, light.Type[name], ts)
		}
	}
	for alias, name := range map[string]string{"page": "title", "ui": "body", "sub": "secondary", "caption": "secondary"} {
		if light.Type[alias] != want[name] {
			t.Errorf("type %s is not %s", alias, name)
		}
	}
	for name, v := range map[string]float32{"sm": 4, "md": 6, "lg": 8} {
		if light.Radius[name] != v {
			t.Errorf("radius %s = %v", name, light.Radius[name])
		}
	}
	if light.Space["1"] != 4 || light.Space["8"] != 32 {
		t.Errorf("space %v", light.Space)
	}
	for name, v := range map[string]float32{"sidebar": 232, "top-bar": 48, "player-bar": 72, "control": 32, "window-min-width": 940, "window-min-height": 600} {
		if light.Metric[name] != v {
			t.Errorf("metric %s = %v", name, light.Metric[name])
		}
	}
	if light.Scalar["backdrop-scrim"] != 0.92 || dark.Scalar["backdrop-scrim"] != 0.86 {
		t.Errorf("backdrop scrim %v %v", light.Scalar, dark.Scalar)
	}
	if m := light.Motion["feedback"]; m.Duration != 120*time.Millisecond || m.Ease == nil {
		t.Errorf("feedback motion %+v", m)
	}
	// The exponential-out curve: slow at the end, fast at the start.
	if e := light.Motion["feedback"].Ease; e(0.1) < 0.3 || e(1) != 1 {
		t.Errorf("ease-out curve: f(0.1) = %v, f(1) = %v", e(0.1), e(1))
	}
	if s := light.Shadow["float"]; s.Y != 8 || s.Blur != 24 || s.Color.A >= dark.Shadow["float"].Color.A {
		t.Errorf("float shadow light %+v dark %+v", s, dark.Shadow["float"])
	}
}

// Sona's own tokens keep every floor Validate holds a theme to
// (docs/accessibility.md: text 4.5:1 up to --active, control-line 3:1).
func TestSonasOwnTokensValidate(t *testing.T) {
	for _, dark := range []bool{false, true} {
		tk, err := state.base.Tokens(dark)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range Validate(tk) {
			t.Errorf("%s", p)
		}
	}
}

func TestParseRejectsBrokenTokens(t *testing.T) {
	const ok = `{"color":{"light":{"a":"#000000","b":"a/50"},"dark":{"a":"#ffffff","b":"a/50"}},"type":{},"motion":{}}`
	if _, err := Parse([]byte(ok)); err != nil {
		t.Fatal(err)
	}
	for name, src := range map[string]struct{ from, to string }{
		"a cycle":             {`"b":"a/50"},"dark"`, `"b":"c","c":"b"},"dark"`},
		"an unknown ref":      {`"b":"a/50"},"dark"`, `"b":"nope"},"dark"`},
		"a bad hex":           {`"a":"#000000"`, `"a":"#00000g"`},
		"a bad opacity":       {`"b":"a/50"},"dark"`, `"b":"a/150"},"dark"`},
		"a missing mode":      {`"dark":{"a":"#ffffff","b":"a/50"}}`, `"dark":{"a":"#ffffff"}},"scalar":{"x":{"light":1}}`},
		"a bad duration":      {`"motion":{}`, `"motion":{"m":{"duration":"fast","ease":"out"}},"ease":{"out":[0,0,1,1]}`},
		"an unknown easing":   {`"motion":{}`, `"motion":{"m":{"duration":"1s","ease":"zip"}}`},
		"a type with no size": {`"type":{}`, `"type":{"x":{"weight":400}}`},
		"a type alias loop":   {`"type":{}`, `"type":{"x":"y","y":"x"}`},
		"not JSON":            {ok, `{`},
	} {
		in := strings.Replace(ok, src.from, src.to, 1)
		if name == "not JSON" {
			in = src.to
		}
		if _, err := Parse([]byte(in)); err == nil {
			t.Errorf("%s accepted: %s", name, in)
		}
	}
}

func TestLoadFileAndReload(t *testing.T) {
	defer ReloadEmbedded()
	dir := t.TempDir()
	path := dir + "/tokens.json"
	custom := strings.Replace(string(embeddedTokens), `"ground": "#f4f5f3"`, `"ground": "#010203"`, 1)
	if custom == string(embeddedTokens) {
		t.Fatal("test edit did not apply")
	}
	writeFile(t, path, custom)
	SetMode(ModeLight)
	defer SetMode(ModeSystem)
	if err := Reload(path); err != nil {
		t.Fatal(err)
	}
	if got := C("ground"); got != ui.RGB(1, 2, 3) {
		t.Errorf("reloaded ground = %v", got)
	}
	writeFile(t, path, "{")
	if err := Reload(path); err == nil {
		t.Error("a broken file was accepted")
	}
	if got := C("ground"); got != ui.RGB(1, 2, 3) {
		t.Errorf("a failed reload changed the tokens: %v", got)
	}
	ReloadEmbedded()
	if got := C("ground"); got != hex("#f4f5f3") {
		t.Errorf("embedded ground = %v", got)
	}
}

func TestCPanicsOnAnUnknownNameInTests(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("an unknown token did not panic under test")
		}
	}()
	C("no-such-token")
}

func TestApplyFollowsModeAndDesktop(t *testing.T) {
	defer SetMode(ModeSystem)
	var seen *ui.Theme
	var dark bool
	tt := ui.NewTester(func(c *ui.Context) {
		Apply(c)
		seen, dark = c.Theme(), Dark()
	}, 200, 100)

	SetMode(ModeSystem)
	tt.SetDark(false)
	tt.Frame()
	if dark || seen.Background != hex("#f4f5f3") || seen.Text != hex("#161917") || seen.Font != FontFamily {
		t.Errorf("light system: dark %v, theme %+v", dark, seen)
	}
	tt.SetDark(true)
	tt.Frame()
	if !dark || seen.Background != hex("#121513") || seen.Text != hex("#eef1ee") || !seen.Dark {
		t.Errorf("dark system: dark %v, bg %v", dark, seen.Background)
	}
	if seen.Accent != hex("#f0b43c") || seen.AccentText != hex("#1a1405") || seen.Radius != 6 || seen.FontSize != 14 {
		t.Errorf("dark theme: %+v", seen)
	}

	SetMode(ModeLight)
	tt.Frame()
	if dark || seen.Background != hex("#f4f5f3") {
		t.Errorf("forced light on a dark desktop: dark %v bg %v", dark, seen.Background)
	}
	SetMode(ModeDark)
	tt.SetDark(false)
	tt.Frame()
	if !dark || seen.Background != hex("#121513") {
		t.Errorf("forced dark on a light desktop: dark %v bg %v", dark, seen.Background)
	}

	// The same theme object is reused while nothing changes.
	first := seen
	tt.Frame()
	if seen != first {
		t.Error("Apply built a new theme with nothing changed")
	}
	// A token change shows on the next frame.
	defer ReloadEmbedded()
	d, err := Parse([]byte(strings.Replace(string(embeddedTokens), `"ground": "#121513"`, `"ground": "#0a0b0c"`, 1)))
	if err != nil {
		t.Fatal(err)
	}
	SetDocument(d)
	tt.Frame()
	if seen.Background != hex("#0a0b0c") {
		t.Errorf("after reload bg = %v", seen.Background)
	}
}

func TestApplyScalesTextWithThePreference(t *testing.T) {
	var fs float32
	tt := ui.NewTester(func(c *ui.Context) { Apply(c); fs = c.Theme().FontSize }, 100, 100)
	tt.SetPreferences(ui.Preferences{TextScale: 1.25})
	tt.Frame()
	if fs != 17.5 {
		t.Errorf("font size %v at a text scale of 1.25", fs)
	}
}

// themeUnder is the ui.Theme Apply sets for a frame under the desktop's prefs.
func themeUnder(t *testing.T, prefs ui.Preferences) *ui.Theme {
	t.Helper()
	var th *ui.Theme
	tt := ui.NewTester(func(c *ui.Context) { Apply(c); th = c.Theme() }, 100, 100)
	tt.SetPreferences(prefs)
	tt.Frame()
	return th
}

// MyGo's high contrast (stronger borders and muted text, an opaque focus ring,
// bolder scroll bars) applies to Sona's theme, and the desktop's accent does
// not: amber is Sona's.
func TestApplyFollowsHighContrastButNotTheDesktopsAccent(t *testing.T) {
	rest := themeUnder(t, ui.Preferences{})
	hc := themeUnder(t, ui.Preferences{HighContrast: true, Accent: hex("#ff0000")})
	if hc.Border == rest.Border || hc.TextMuted == rest.TextMuted || hc.Scrollbar == rest.Scrollbar {
		t.Errorf("high contrast left the border %v, muted text %v or scroll bar %v as they were", hc.Border, hc.TextMuted, hc.Scrollbar)
	}
	if hc.Focus.A != 255 {
		t.Errorf("focus ring alpha %d under high contrast, want opaque", hc.Focus.A)
	}
	if hc.ScrollbarStyle.Hover.A <= rest.ScrollbarStyle.Hover.A {
		t.Errorf("the hovered scroll bar thumb is %v, no bolder than %v", hc.ScrollbarStyle.Hover, rest.ScrollbarStyle.Hover)
	}
	if hc.Accent != rest.Accent || hc.Accent != Current().Color["amber"] || hc.Selection != rest.Selection {
		t.Errorf("the desktop's accent changed the theme: accent %v, selection %v", hc.Accent, hc.Selection)
	}
}

// What the desktop leaves at its usual setting changes nothing in the theme
// (so the goldens, which run at them, hold).
func TestApplyUsualPreferencesLeaveTheThemeAlone(t *testing.T) {
	a := themeUnder(t, ui.Preferences{})
	b := themeUnder(t, ui.Preferences{TextScale: 1})
	if a.FontSize != b.FontSize || a.Border != b.Border || a.TextMuted != b.TextMuted || a.Focus != b.Focus ||
		a.Scrollbar != b.Scrollbar || a.ScrollbarStyle != b.ScrollbarStyle || a.Accent != b.Accent || a.Selection != b.Selection {
		t.Errorf("a text scale of 1 changed the theme:\n%+v\n%+v", a, b)
	}
	if want := Current().Type["body"].Size; a.FontSize != want {
		t.Errorf("font size %v at the usual scale, want the body's %v", a.FontSize, want)
	}
}

// The scroll bar thumb has its own colour while the pointer is over the bar.
func TestApplyColoursTheHoveredScrollbarThumb(t *testing.T) {
	th := themeUnder(t, ui.Preferences{})
	tk := Current()
	if th.ScrollbarStyle.Hover != tk.Color["scrollbar-hover"] || th.Scrollbar != tk.Color["scrollbar"] {
		t.Errorf("scroll bar %v, hovered %v; want the tokens' %v and %v", th.Scrollbar, th.ScrollbarStyle.Hover, tk.Color["scrollbar"], tk.Color["scrollbar-hover"])
	}
	if th.ScrollbarStyle.Hover == th.Scrollbar {
		t.Error("the hovered thumb is the resting colour")
	}
}

func TestReducedMotionAndEffects(t *testing.T) {
	defer func() { SetReduceEffects(false); SetReducedMotion(false) }()
	tt := ui.NewTester(func(c *ui.Context) { Apply(c) }, 100, 100)
	tt.Frame()
	if ReduceEffects() || ReducedMotion() {
		t.Fatal("reduced by default")
	}
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tt.Frame()
	if ReduceEffects() || !ReducedMotion() {
		t.Errorf("desktop's reduced motion: effects %v motion %v", ReduceEffects(), ReducedMotion())
	}
	tt.SetPreferences(ui.Preferences{})
	tt.Frame()
	SetReducedMotion(true)
	if ReduceEffects() || !ReducedMotion() {
		t.Errorf("motion setting: effects %v motion %v", ReduceEffects(), ReducedMotion())
	}
	SetReducedMotion(false)
	SetReduceEffects(true)
	if !ReduceEffects() || !ReducedMotion() {
		t.Errorf("reduce effects stops motion too: effects %v motion %v", ReduceEffects(), ReducedMotion())
	}
}

func TestScopeReplacesTokensOnlyWhileItsChildrenBuild(t *testing.T) {
	outer := map[string]ui.Color{"ink": ui.RGB(1, 2, 3), "amber": ui.RGB(4, 5, 6)}
	inner := map[string]ui.Color{"ink": ui.RGB(7, 8, 9)}
	base, ground := C("ink"), C("ground")
	Scope(outer, func() {
		if got := C("ink"); got != outer["ink"] {
			t.Errorf("ink in scope %v", got)
		}
		if got := C("ground"); got != ground {
			t.Errorf("a token the scope does not name changed: %v", got)
		}
		Scope(inner, func() {
			if got := C("ink"); got != inner["ink"] {
				t.Errorf("ink in the inner scope %v", got)
			}
			if got := C("amber"); got != outer["amber"] {
				t.Errorf("amber from the outer scope: %v", got)
			}
		})
		if got := C("ink"); got != outer["ink"] {
			t.Errorf("ink after the inner scope %v", got)
		}
	})
	if got := C("ink"); got != base {
		t.Errorf("ink after the scope %v, want %v", got, base)
	}
	Scope(nil, func() {
		if got := C("ink"); got != base {
			t.Errorf("an empty scope changed ink: %v", got)
		}
	})
}
