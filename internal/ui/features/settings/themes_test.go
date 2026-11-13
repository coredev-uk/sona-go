package settings

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// themeRig is the Appearance section with the themes service faked.
func themeRig(t *testing.T) (*rig, *fakeThemes) {
	t.Helper()
	var f *fakeThemes
	// Tall, so that every card and the rows under them are in the window.
	r := sectionRigSized(t, "appearance", 2400, func(s *settings) { f = withFakeThemes(s) })
	r.tt.Until("the gallery", r.tt.Shows("Community palettes", "Open themes folder"))
	return r, f
}

var builtinCards = []string{
	"Sona", "Graphite", "Paper", "High contrast", "OLED",
	"Catppuccin", "Nord", "Gruvbox", "Solarized", "Dracula", "Rosé Pine", "Tokyo Night",
}

// The gallery is one radio group named Theme: every built-in is a radio
// named by the theme and described by what it looks like, the chosen one is
// checked, a theme with one look says so, the group is one stop of Tab, and
// the arrows choose.
func TestA11yThemeGalleryIsOneRadioGroup(t *testing.T) {
	r, _ := themeRig(t)
	access.NoNestedControls(t, r.tt.Tester)
	tr := access.Read(t, r.tt.Tester)
	group, ok := tr.Find(ui.RoleRadioGroup, "Theme")
	if !ok {
		t.Fatalf("no radio group named Theme:\n%s", tr.Dump())
	}
	var names []string
	for _, n := range tr.Children(group) {
		if n.Role == ui.RoleRadio {
			names = append(names, n.Label)
		}
	}
	if !slices.Equal(names, builtinCards) {
		t.Errorf("radios %q, want %q", names, builtinCards)
	}
	for _, name := range builtinCards {
		n, ok := tr.Find(ui.RoleRadio, name)
		if !ok || n.Description == "" {
			t.Errorf("%s: %+v", name, n)
			continue
		}
		if want := name == "Sona"; n.Has(access.Checked) != want {
			t.Errorf("%s checked = %v", name, !want)
		}
	}
	for _, name := range []string{"OLED", "Dracula"} {
		if n, _ := tr.Find(ui.RoleRadio, name); !strings.Contains(n.Description, "Dark only") {
			t.Errorf("%s does not say it is dark only: %q", name, n.Description)
		}
	}
	// The headings that group the cards.
	var headings []string
	for _, n := range tr.Roles(ui.RoleHeading) {
		if n.Level == 4 {
			headings = append(headings, n.Label)
		}
	}
	if !slices.Equal(headings, []string{"Sona's own", "Community palettes"}) {
		t.Errorf("headings %q", headings)
	}
	// One stop of Tab for the whole group.
	inGroup := 0
	for _, label := range access.StopLabels(access.TabWalk(t, r.tt.Tester, 120)) {
		if slices.Contains(builtinCards, label) {
			inGroup++
		}
	}
	if inGroup != 1 {
		t.Errorf("the gallery is %d stops of Tab, want 1", inGroup)
	}
	// Arrows choose and save.
	if err := r.tt.Click("Graphite"); err != nil {
		t.Fatal(err)
	}
	r.saved("graphite saved", func(s config.Settings) bool { return s.ThemeID == "graphite" })
	r.tt.Key(0, ui.KeyRight)
	r.saved("paper saved", func(s config.Settings) bool { return s.ThemeID == "paper" })
	// The next key goes to the card the view shows checked: wait for the
	// view, not only the store, or a loaded machine sends it too early.
	r.tt.Until("paper checked", func() bool {
		n, _ := access.Read(t, r.tt.Tester).Find(ui.RoleRadio, "Paper")
		return n.Has(access.Checked)
	})
	r.tt.Key(0, ui.KeyLeft)
	r.saved("graphite again", func(s config.Settings) bool { return s.ThemeID == "graphite" })
	tr = access.Read(t, r.tt.Tester)
	if n, _ := tr.Find(ui.RoleRadio, "Graphite"); !n.Has(access.Checked) {
		t.Error("the chosen card is not checked")
	}
}

func TestChoosingAThemeAppliesItAtOnceAndSaves(t *testing.T) {
	r, _ := themeRig(t)
	if err := r.tt.Click("Nord"); err != nil {
		t.Fatal(err)
	}
	if theme.Active().ID != "nord" {
		t.Errorf("active %q before Go answered", theme.Active().ID)
	}
	r.saved("nord saved", func(s config.Settings) bool { return s.ThemeID == "nord" })
	r.tt.Until("the summary", r.tt.Shows("Nord · Light"))
}

// A theme with one look holds it: the mode row is off and says why, and the
// summary names the theme and its look. Another theme gives the mode back.
func TestADarkOnlyThemePinsTheMode(t *testing.T) {
	r, _ := themeRig(t)
	if err := r.tt.Click("OLED"); err != nil {
		t.Fatal(err)
	}
	r.saved("oled saved", func(s config.Settings) bool { return s.ThemeID == "oled" })
	r.tt.Until("the row says so", r.tt.Shows("OLED has a dark look only, so Sona stays dark with it.", "OLED · Dark"))
	if !theme.Dark() {
		t.Error("a dark-only theme drew light")
	}
	tr := access.Read(t, r.tt.Tester)
	if n, ok := tr.Find(ui.RoleRadio, "Light"); !ok || !n.Has(access.Disabled) {
		t.Errorf("the Light mode is not off: %+v", n)
	}
	if err := r.tt.Click("Sona"); err != nil {
		t.Fatal(err)
	}
	r.saved("sona saved", func(s config.Settings) bool { return s.ThemeID == "sona" })
	r.tt.Until("the mode is back", r.tt.Shows("System follows your desktop's light or dark preference."))
	tr = access.Read(t, r.tt.Tester)
	if n, _ := tr.Find(ui.RoleRadio, "Light"); n.Has(access.Disabled) {
		t.Error("the Light mode stayed off")
	}
}

func userFile(name, id, ground string) theme.UserFile {
	return theme.UserFile{Name: name, Data: []byte(`{"format":1,"id":"` + id + `","name":"` + strings.ToUpper(id[:1]) + id[1:] + `","author":"Ada","light":{"color":{"ground":"` + ground + `"}},"dark":{}}`)}
}

// The user's files come after the built-ins: a good one as a card of its
// own, a refused one disabled with why, a refused edit of a good file as the
// earlier version with a note.
func TestUserThemesAreListedWithTheirProblems(t *testing.T) {
	r, _ := themeRig(t)
	theme.LoadUser([]theme.UserFile{
		userFile("mine.json", "mine", "#f7f7f7"),
		{Name: "broken.json", Data: []byte("{")},
		{Name: "faint.json", Data: []byte(`{"format":1,"id":"faint","name":"Faint","light":{"color":{"ink-muted":"#dddddd"}}}`)},
		{Name: "big.json", TooLarge: true},
	})
	r.tt.Frame()
	r.tt.Until("your themes", r.tt.Shows("Your themes", "Mine", "By Ada", "broken.json", "big.json"))
	tr := access.Read(t, r.tt.Tester)
	for name, want := range map[string]string{
		"broken.json": "Not a valid theme file",
		"faint.json":  "ink-muted on ground",
		"big.json":    "larger than 256 KiB",
	} {
		n, ok := tr.Find(ui.RoleRadio, name)
		if !ok || !n.Has(access.Disabled) || !strings.Contains(n.Description, want) {
			t.Errorf("%s: want a disabled radio described with %q: %+v", name, want, n)
		}
	}
	if err := r.tt.Click("Mine"); err != nil {
		t.Fatal(err)
	}
	r.saved("mine saved", func(s config.Settings) bool { return s.ThemeID == "mine" })
	if theme.Active().ID != "mine" {
		t.Errorf("active %q", theme.Active().ID)
	}
	// Edit it badly: the earlier version stays and the card says so.
	theme.LoadUser([]theme.UserFile{{Name: "mine.json", Data: []byte(`{"format":1,"id":"mine","name":"Mine","light":{"color":{"ink-muted":"#dddddd"}},"dark":{}}`)}})
	r.tt.Frame()
	r.tt.Until("the note", r.tt.Shows("Your last edit was refused. This earlier version stays."))
	if theme.Active().ID != "mine" {
		t.Error("the refused edit dropped the theme")
	}
}

func TestAMissingChosenThemeFallsBackToSonaAndSaysSo(t *testing.T) {
	r, _ := themeRig(t)
	// What the shell does when the saved ID is one no theme has.
	if theme.SetTheme("gone-theme") {
		t.Fatal("found a theme with that ID")
	}
	r.tt.Frame()
	r.tt.Until("the note", r.tt.Shows("The theme “gone-theme” isn't available, so Sona's own is showing. It comes back with its file."))
	tr := access.Read(t, r.tt.Tester)
	if n, _ := tr.Find(ui.RoleRadio, "Sona"); !n.Has(access.Checked) {
		t.Error("Sona's own theme is not the checked card while it shows")
	}
	if theme.Requested() != "gone-theme" {
		t.Errorf("the unknown ID was not kept: %q", theme.Requested())
	}
}

func TestThemesFolderRowSaysWhereAndOpens(t *testing.T) {
	r, f := themeRig(t)
	r.tt.Until("the folder", r.tt.Shows(fakeThemesFolder))
	if err := r.tt.Click("Open themes folder"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the folder opened", func() bool { return f.opens() == 1 })
	f.err = errors.New("no file manager")
	if err := r.tt.Click("Open themes folder"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the failure", r.tt.Shows("Couldn't open the themes folder"))
}

