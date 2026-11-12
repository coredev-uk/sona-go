package theme

import (
	"fmt"
	"io/fs"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// user is a theme file that overrides the light ground and leaves the dark
// appearance as Sona's.
func user(id, name, ground string) []byte {
	return []byte(fmt.Sprintf(`{"format":1,"id":%q,"name":%q,"light":{"color":{"ground":%q}},"dark":{}}`, id, name, ground))
}

func TestBuiltinThemesAreEmbeddedAndValid(t *testing.T) {
	files, err := fs.Glob(builtinFiles, "themes/*.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != len(builtinOrder) {
		t.Errorf("themes/ has %v, builtinOrder %v", files, builtinOrder)
	}
	for i, id := range builtinOrder {
		data, err := builtinFiles.ReadFile("themes/" + id + ".json")
		if err != nil {
			t.Fatal(err)
		}
		th, problems := ParseTheme(data)
		for _, p := range problems {
			t.Errorf("%s: %s", id, p)
		}
		if th == nil {
			continue
		}
		if th.ID != id || th.Name == "" || !builtinThemes[i].Builtin || builtinThemes[i].ID != id {
			t.Errorf("%s: id %q name %q", id, th.ID, th.Name)
		}
	}
	if builtinThemes[0].ID != DefaultThemeID {
		t.Errorf("the first theme is %q, the default is %q", builtinThemes[0].ID, DefaultThemeID)
	}
}

func TestBuiltinThemesHaveTheirPolarity(t *testing.T) {
	darkOnly := map[string]bool{"oled": true, "dracula": true}
	for _, th := range builtinThemes {
		if th.HasLight() == darkOnly[th.ID] || !th.HasDark() {
			t.Errorf("%s: light %v dark %v", th.ID, th.HasLight(), th.HasDark())
		}
	}
}

// The community palettes credit their project.
func TestCommunityPalettesCreditTheirProject(t *testing.T) {
	for _, id := range []string{"catppuccin", "nord", "gruvbox", "solarized", "dracula", "rose-pine", "tokyo-night"} {
		th := Lookup(id)
		if th == nil || !th.Builtin || th.Author == "" || th.Name == "" {
			t.Errorf("%s: %+v", id, th)
		}
	}
}

// Sona's own themes keep the chrome a neutral board, so the cover is the
// colour (docs/design.md). The community palettes keep their own hues.
func TestSonasOwnThemesHaveANeutralBoard(t *testing.T) {
	for _, id := range []string{"sona", "graphite", "paper", "contrast", "oled"} {
		th := Lookup(id)
		for _, dark := range []bool{false, true} {
			tk := th.Tokens(dark)
			// The chrome is a neutral board and the cover is the colour
			// (docs/design.md): no board tone strays far from grey.
			for _, name := range []string{"ground", "surface", "raised", "hover", "active", "line"} {
				c := tk.Color[name]
				hi, lo := max(c.R, c.G, c.B), min(c.R, c.G, c.B)
				if hi-lo > 12 {
					t.Errorf("%s %s %s = %v is tinted, not neutral", th.ID, modeName(dark), name, c)
				}
			}
		}
	}
}

// High contrast keeps every text colour at 7:1 on every tone up to the
// pressed one and on a selected row.
func TestHighContrastTextKeeps7To1(t *testing.T) {
	th := Lookup("contrast")
	if th == nil || !th.HasLight() || !th.HasDark() {
		t.Fatal("no high contrast theme with both variants")
	}
	for _, dark := range []bool{false, true} {
		tk := th.Tokens(dark)
		ground := tk.Color["ground"]
		tones := []ui.Color{ground, tk.Color["surface"], tk.Color["raised"], tk.Color["hover"], tk.Color["active"], tk.Color["ink"].Alpha(0.12).Over(ground)}
		for _, fg := range textTokens {
			for i, bg := range tones {
				if c := Contrast(tk.Color[fg], bg); c < 7 {
					t.Errorf("%s: %s on tone %d is %.2f:1", modeName(dark), fg, i, c)
				}
			}
		}
	}
}

func TestMissingTokensComeFromSona(t *testing.T) {
	th, problems := ParseTheme(user("quiet", "Quiet", "#f6f6f6"))
	if th == nil {
		t.Fatal(problems)
	}
	light, dark := th.Tokens(false), th.Tokens(true)
	if light.Color["ground"] != ui.Hex("#f6f6f6") {
		t.Errorf("ground = %v", light.Color["ground"])
	}
	base, _ := state.base.Tokens(false)
	for name, c := range base.Color {
		// Only the ground and the roles built on it change.
		if c == base.Color["ground"] {
			continue
		}
		if light.Color[name] != c {
			t.Errorf("light %s = %v, Sona has %v", name, light.Color[name], c)
		}
	}
	if light.Type["body"] != base.Type["body"] || light.Radius["md"] != base.Radius["md"] || dark.Scalar["backdrop-scrim"] != 0.86 {
		t.Error("non-colour tokens did not come from Sona")
	}
	// A theme with one variant: the other is Sona's, and the theme pins its own.
	one, problems := ParseTheme([]byte(`{"format":1,"id":"one","name":"One","dark":{"color":{"ground":"#0b0b0b"}}}`))
	if one == nil {
		t.Fatal(problems)
	}
	if d, pinned := one.Pinned(); !pinned || !d {
		t.Errorf("dark-only theme pins %v %v", d, pinned)
	}
	if got, want := one.Tokens(false).Color["ground"], base.Color["ground"]; got != want {
		t.Errorf("the light variant a dark-only theme lacks is %v, want Sona's %v", got, want)
	}
	if _, pinned := th.Pinned(); pinned {
		t.Error("a theme with both variants pins")
	}
}

func TestParseThemeRefusesWhatItShould(t *testing.T) {
	const head = `{"format":1,"id":"t","name":"T",`
	for name, c := range map[string]struct {
		file string
		kind ProblemKind
		tok  string
	}{
		"not JSON":              {`{`, ProblemSyntax, ""},
		"trailing data":         {head + `"dark":{}} {}`, ProblemSyntax, ""},
		"an unknown field":      {head + `"lite":{}}`, ProblemSyntax, ""},
		"a wrong group":         {head + `"dark":{"colour":{}}}`, ProblemSyntax, ""},
		"another format":        {`{"format":2,"id":"t","name":"T","dark":{}}`, ProblemFormat, ""},
		"no format":             {`{"id":"t","name":"T","dark":{}}`, ProblemFormat, ""},
		"an id with capitals":   {`{"format":1,"id":"Tee","name":"T","dark":{}}`, ProblemID, ""},
		"no id":                 {`{"format":1,"name":"T","dark":{}}`, ProblemID, ""},
		"no name":               {`{"format":1,"id":"t","dark":{}}`, ProblemName, ""},
		"a long name":           {`{"format":1,"id":"t","name":"` + strings.Repeat("n", 41) + `","dark":{}}`, ProblemName, ""},
		"no variant":            {`{"format":1,"id":"t","name":"T"}`, ProblemNoVariant, ""},
		"an unknown colour":     {head + `"dark":{"color":{"magenta":"#ff00ff"}}}`, ProblemUnknownToken, "magenta"},
		"an unknown scalar":     {head + `"dark":{"scalar":{"zoom":1}}}`, ProblemUnknownToken, "zoom"},
		"an unknown shadow":     {head + `"dark":{"shadow":{"huge":{"x":0,"y":0,"blur":1,"spread":0,"color":"#000000"}}}}`, ProblemUnknownToken, "huge"},
		"a bad hex":             {head + `"dark":{"color":{"ground":"#12345"}}}`, ProblemBadValue, "ground"},
		"a reference loop":      {head + `"dark":{"color":{"ground":"surface","surface":"ground"}}}`, ProblemBadValue, "ground"},
		"an unknown reference":  {head + `"dark":{"color":{"ground":"nope"}}}`, ProblemBadValue, "ground"},
		"a bad opacity":         {head + `"dark":{"color":{"ground":"ink/150"}}}`, ProblemBadValue, "ground"},
		"a bad shadow colour":   {head + `"dark":{"shadow":{"sm":{"x":0,"y":0,"blur":1,"spread":0,"color":"red"}}}}`, ProblemBadValue, "sm"},
		"a huge shadow":         {head + `"dark":{"shadow":{"sm":{"x":0,"y":0,"blur":9000,"spread":0,"color":"#000000"}}}}`, ProblemBadValue, "sm"},
		"faint text":            {head + `"light":{"color":{"ink-muted":"#dddddd"}}}`, ProblemContrast, "ink-muted"},
		"a spot colour as ring": {head + `"light":{"color":{"ring":"amber"}}}`, ProblemSpotRole, "ring"},
		"a dim spot colour":     {head + `"dark":{"color":{"amber":"#2a2f2c","amber-ink":"#f0b43c"}}}`, ProblemContrast, "amber"},
		"a faint scrim":         {head + `"dark":{"scalar":{"backdrop-scrim":0.2}}}`, ProblemRange, "backdrop-scrim"},
	} {
		th, ps := ParseTheme([]byte(c.file))
		if th != nil {
			t.Errorf("%s: accepted", name)
			continue
		}
		if !has(ps, c.kind, c.tok) {
			t.Errorf("%s: want kind %d token %q, got %v", name, c.kind, c.tok, ps)
		}
	}
}

func TestAThemeMayPickItsOwnSpotColour(t *testing.T) {
	th, ps := ParseTheme([]byte(`{"format":1,"id":"coral","name":"Coral","light":{"color":{"amber":"#e8590c","amber-ink":"#a33d00","on-amber":"#1a0a00"}},"dark":{"color":{"amber":"#ff8a5c","amber-ink":"#ff8a5c","on-amber":"#1a0a00"}}}`))
	if th == nil {
		t.Fatal(ps)
	}
	tk := th.Tokens(false)
	if tk.Color["play"] != ui.Hex("#e8590c") || tk.Color["on-play"] != ui.Hex("#1a0a00") || tk.Color["selection"].R != 0xe8 {
		t.Errorf("the play roles and the selection did not follow: %v %v %v", tk.Color["play"], tk.Color["on-play"], tk.Color["selection"])
	}
}
