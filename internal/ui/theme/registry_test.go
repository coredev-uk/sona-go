package theme

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// reset puts the registry back as a fresh process has it.
func reset(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		LoadUser(nil)
		SetTheme(DefaultThemeID)
		SetMode(ModeSystem)
	})
}

func TestSetThemeChangesTheColours(t *testing.T) {
	reset(t)
	SetMode(ModeDark)
	if got := C("ground"); got != ui.Hex("#121513") {
		t.Fatalf("Sona's dark ground = %v", got)
	}
	rev := Revision()
	if !SetTheme("graphite") {
		t.Fatal("graphite is not a theme")
	}
	g := Lookup("graphite").Tokens(true)
	if got := C("ground"); got != g.Color["ground"] || got == ui.Hex("#121513") {
		t.Errorf("ground after graphite = %v", got)
	}
	if C("amber") != g.Color["amber"] || C("play") != g.Color["amber"] {
		t.Errorf("the spot colour did not change: amber %v play %v", C("amber"), C("play"))
	}
	if Revision() == rev || Active().ID != "graphite" || Requested() != "graphite" {
		t.Errorf("revision %d, active %q, requested %q", Revision(), Active().ID, Requested())
	}
	// The same choice again leaves what was worked out from the tokens alone.
	rev = Revision()
	SetTheme("graphite")
	if Revision() != rev {
		t.Error("choosing the active theme again changed the revision")
	}
}

func TestAOneVariantThemePinsTheAppearance(t *testing.T) {
	reset(t)
	SetTheme("oled")
	for _, m := range []Mode{ModeLight, ModeSystem, ModeDark} {
		SetMode(m)
		if !Dark() {
			t.Errorf("mode %d: a dark-only theme drew light", m)
		}
	}
	if got := C("ground"); got != ui.RGB(0, 0, 0) {
		t.Errorf("oled's ground = %v", got)
	}
	SetTheme("sona")
	SetMode(ModeLight)
	if Dark() {
		t.Error("Sona under Light drew dark")
	}
}

func TestAnUnknownThemeShowsSonaAndIsKept(t *testing.T) {
	reset(t)
	SetMode(ModeLight)
	if SetTheme("no-such-theme") {
		t.Error("an unknown theme was found")
	}
	if Active().ID != DefaultThemeID || Requested() != "no-such-theme" || C("ground") != ui.Hex("#f4f5f3") {
		t.Errorf("active %q requested %q ground %v", Active().ID, Requested(), C("ground"))
	}
	// Its file turns up: the theme is there without choosing it again.
	LoadUser([]UserFile{{Name: "x.json", Data: user("no-such-theme", "Later", "#f7f7f7")}})
	if Active().ID != "no-such-theme" || C("ground") != ui.Hex("#f7f7f7") {
		t.Errorf("after its file arrived: active %q ground %v", Active().ID, C("ground"))
	}
	if SetTheme("") && Active().ID != DefaultThemeID {
		t.Error("an empty ID is not Sona's")
	}
}

func TestThemesListsBuiltInsThenTheUsers(t *testing.T) {
	reset(t)
	LoadUser([]UserFile{
		{Name: "b.json", Data: user("zeta", "Zeta", "#f7f7f7")},
		{Name: "a.json", Data: user("alpha", "Alpha", "#f8f8f8")},
	})
	var ids []string
	for _, th := range Themes() {
		ids = append(ids, th.ID)
	}
	if got, want := strings.Join(ids, " "), "sona graphite paper contrast oled catppuccin nord gruvbox solarized dracula rose-pine tokyo-night alpha zeta"; got != want {
		t.Errorf("themes = %s, want %s", got, want)
	}
	if Lookup("alpha").File != "a.json" || Lookup("alpha").Builtin || !Lookup("paper").Builtin {
		t.Error("files or built-in flags are wrong")
	}
}

func TestUserThemesReloadKeepTheLastGoodAndSayWhy(t *testing.T) {
	reset(t)
	SetMode(ModeLight)
	SetTheme("mine")
	LoadUser([]UserFile{{Name: "mine.json", Data: user("mine", "Mine", "#f7f7f7")}})
	if C("ground") != ui.Hex("#f7f7f7") {
		t.Fatalf("ground = %v", C("ground"))
	}
	// An edit shows at once.
	es := LoadUser([]UserFile{{Name: "mine.json", Data: user("mine", "Mine", "#f9f9f9")}})
	if C("ground") != ui.Hex("#f9f9f9") || len(es) != 1 || es[0].Problems != nil || es[0].Kept {
		t.Errorf("after an edit: ground %v, %+v", C("ground"), es)
	}
	// A broken edit keeps the last good theme and lists the problems.
	es = LoadUser([]UserFile{{Name: "mine.json", Data: []byte(`{"format":1,"id":"mine","name":"Mine","light":{"color":{"ink-muted":"#eeeeee"}}}`)}})
	if C("ground") != ui.Hex("#f9f9f9") || len(es) != 1 || !es[0].Kept || es[0].Theme == nil || len(es[0].Problems) == 0 {
		t.Errorf("after a broken edit: ground %v, %+v", C("ground"), es)
	}
	if got := UserEntries(); len(got) != 1 || !got[0].Kept {
		t.Errorf("UserEntries = %+v", got)
	}
	// A file that was never good is listed with its problems and offers no theme.
	es = LoadUser([]UserFile{{Name: "new.json", Data: []byte("{")}})
	if len(es) != 1 || es[0].Theme != nil || len(es[0].Problems) == 0 || es[0].Problems[0].Kind != ProblemSyntax {
		t.Errorf("a broken new file: %+v", es)
	}
	// Deleting the file drops its theme, and the choice falls back to Sona.
	if Active().ID != DefaultThemeID || C("ground") != ui.Hex("#f4f5f3") {
		t.Errorf("after the file went: active %q ground %v", Active().ID, C("ground"))
	}
}

func TestUserThemeIDsAreUniqueAndSizeCapped(t *testing.T) {
	reset(t)
	es := LoadUser([]UserFile{
		{Name: "a.json", Data: user("sona", "Imposter", "#f7f7f7")},
		{Name: "b.json", Data: user("dup", "First", "#f8f8f8")},
		{Name: "c.json", Data: user("dup", "Second", "#f9f9f9")},
		{Name: "d.json", TooLarge: true},
	})
	want := []ProblemKind{ProblemIDTaken, -1, ProblemIDTaken, ProblemTooLarge}
	for i, e := range es {
		if want[i] < 0 {
			if len(e.Problems) != 0 || e.Theme == nil {
				t.Errorf("%s: %+v", e.File, e)
			}
			continue
		}
		if len(e.Problems) != 1 || e.Problems[0].Kind != want[i] || e.Theme != nil {
			t.Errorf("%s: %+v", e.File, e)
		}
	}
	if Lookup("sona").Builtin != true || Lookup("dup").Name != "First" {
		t.Error("a built-in or an earlier file lost its ID")
	}
}

func TestThemeTokensAreCachedUntilTheTokensChange(t *testing.T) {
	reset(t)
	g := Lookup("graphite")
	before := g.Tokens(true)
	if g.Tokens(true) != before {
		t.Error("a theme's tokens are worked out each time")
	}
	SetTheme("paper")
	if g.Tokens(true) == before {
		t.Error("a new revision kept the old tokens")
	}
}

func TestApplyKeepsOneUITheme(t *testing.T) {
	reset(t)
	var seen *ui.Theme
	tt := ui.NewTester(func(c *ui.Context) { Apply(c); seen = c.Theme() }, 200, 100)
	SetMode(ModeDark)
	SetTheme("graphite")
	tt.Frame()
	if seen.Background != Lookup("graphite").Tokens(true).Color["ground"] || seen.Accent != Lookup("graphite").Tokens(true).Color["amber"] {
		t.Errorf("graphite: background %v accent %v", seen.Background, seen.Accent)
	}
	first := seen
	tt.Frame()
	if seen != first {
		t.Error("Apply built a new ui theme with nothing changed")
	}
}
