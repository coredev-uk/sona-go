package kit

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// choicesApp is a song with an action whose choices follow its artists, as
// "Go to artist" does.
type choicesApp struct {
	am     *ActionMenus
	item   Item
	artist []services.Artist
	went   []string
}

func newChoicesApp(t *testing.T, artists []services.Artist) (*choicesApp, *ui.Tester) {
	t.Helper()
	theme.SetMode(theme.ModeLight)
	t.Cleanup(func() { theme.SetMode(theme.ModeSystem) })
	a := &choicesApp{item: song("Stay"), artist: artists}
	reg := NewActionRegistry(ActionDef{
		ID: "artist.go-to", Target: KindSong, Group: GroupItem, Order: 20, Icon: "mic-vocal",
		Label: func(*ActionContext) string { return "Go to artist" },
		Choices: func(*ActionContext) []ActionChoice {
			var out []ActionChoice
			for _, ar := range a.artist {
				out = append(out, ActionChoice{Key: ar.ID, Label: ar.Name, Run: func(*ActionContext) { a.went = append(a.went, ar.ID) }})
			}
			return out
		},
		Run: func(*ActionContext) { a.went = append(a.went, "first") },
	})
	a.am = NewActionMenus(reg, nil)
	return a, ui.NewTester(a.view, 560, 300)
}

func (a *choicesApp) view(c *ui.Context) {
	theme.Apply(c)
	tgt := ActionTarget{Item: a.item, Name: a.item.Name(), Key: a.item.ID()}
	ui.Column(c).Padding(20).Children(func() {
		row := ui.Row(c.Key("row")).Height(40).Width(480).AlignItems(ui.Center)
		row.Children(func() {
			ui.Text(c, a.item.Name()).Grow(1)
			a.am.Trigger(c, row, tgt, TriggerOpts{Visible: true})
		})
	})
	a.am.Popup(c)
}

// A song of several artists has "Go to artist" as a submenu with one entry per
// artist, never the first silently: it opens beside the row (Right, or the
// pointer), and each entry opens that artist.
func TestActionChoicesMakeASubmenuForSeveralArtists(t *testing.T) {
	a, tt := newChoicesApp(t, twoArtists)
	if err := tt.Click("More for Stay"); err != nil {
		t.Fatal(err)
	}
	tr := access.Read(t, tt)
	row, ok := tr.Find(ui.RoleMenuItem, "Go to artist")
	if !ok || row.Has(access.Expanded) {
		t.Fatalf("want a collapsed row named Go to artist:\n%s", tr.Dump())
	}
	if _, ok := tr.Find(ui.RoleMenuItem, "Justin Bieber"); ok {
		t.Fatalf("the artists show before the sub-menu opens:\n%s", tr.Dump())
	}
	tt.Key(0, ui.KeyDown) // the row, then its sub-menu
	tt.Key(0, ui.KeyRight)
	tr = access.Read(t, tt)
	if n, ok := tr.Find(ui.RoleMenuItem, "Go to artist"); !ok || !n.Has(access.Expanded) {
		t.Errorf("Right did not open the sub-menu:\n%s", tr.Dump())
	}
	for _, name := range []string{"The Kid LAROI", "Justin Bieber"} {
		if _, ok := tr.Find(ui.RoleMenuItem, name); !ok {
			t.Fatalf("no sub-menu entry %q:\n%s", name, tr.Dump())
		}
	}
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyEnter)
	tt.Advance(50e6)
	tt.Frame()
	tt.Frame()
	if !slices.Equal(a.went, []string{"320569549"}) {
		t.Errorf("went to %v, want the second artist", a.went)
	}
	if a.am.IsOpen() {
		t.Error("the menu stayed open")
	}
}

// One artist, or none the action knows more of, keeps the plain row.
func TestActionChoicesOfOneArtistKeepThePlainRow(t *testing.T) {
	for name, artists := range map[string][]services.Artist{"none": nil, "one": oneArtist} {
		a, tt := newChoicesApp(t, artists)
		if err := tt.Click("More for Stay"); err != nil {
			t.Fatal(err)
		}
		tr := access.Read(t, tt)
		if _, ok := tr.Find(ui.RoleMenuItem, "Go to artist"); !ok {
			t.Fatalf("%s: want a row named Go to artist:\n%s", name, tr.Dump())
		}
		if err := tt.Click("Go to artist"); err != nil {
			t.Fatal(err)
		}
		tt.Advance(50e6)
		tt.Frame()
		tt.Frame()
		if !slices.Equal(a.went, []string{"first"}) {
			t.Errorf("%s: went %v", name, a.went)
		}
	}
}

// A choice with Sub is a submenu of its own (a folder of playlists), opened
// by Right and closed by Left; Icon and Rule shape the rows, and a choice
// that is not an entry of a submenu runs with the menu's items.
func TestActionChoicesNest(t *testing.T) {
	theme.SetMode(theme.ModeLight)
	t.Cleanup(func() { theme.SetMode(theme.ModeSystem) })
	var went []string
	run := func(name string) func(*ActionContext) {
		return func(a *ActionContext) { went = append(went, name+":"+a.Item().Name()) }
	}
	reg := NewActionRegistry(ActionDef{
		ID: "playlists.add-to.song", Target: KindSong, Group: GroupItem, Order: 40, Icon: "list-plus",
		Label: func(*ActionContext) string { return "Add to playlist" },
		Choices: func(*ActionContext) []ActionChoice {
			return []ActionChoice{
				{Key: "new", Label: "New playlist…", Icon: "plus", Run: run("new")},
				{Key: "mixes", Label: "Mixes", Icon: "folder-closed", Rule: true, Sub: []ActionChoice{
					{Key: "gym", Label: "Gym", Run: run("gym")},
					{Key: "old", Label: "Old", Icon: "folder-closed", Sub: []ActionChoice{{Key: "dusty", Label: "Dusty", Run: run("dusty")}}},
				}},
				{Key: "road", Label: "Roadie", Rule: true, Run: run("road")},
			}
		},
		Run: run("plain"),
	})
	am := NewActionMenus(reg, nil)
	item := song("Stay")
	view := func(c *ui.Context) {
		theme.Apply(c)
		tgt := ActionTarget{Item: item, Name: item.Name(), Key: item.ID()}
		ui.Column(c).Padding(20).Children(func() {
			row := ui.Row(c.Key("row")).Height(40).Width(480).AlignItems(ui.Center)
			row.Children(func() {
				ui.Text(c, item.Name()).Grow(1)
				am.Trigger(c, row, tgt, TriggerOpts{Visible: true})
			})
		})
		am.Popup(c)
	}
	tt := ui.NewTester(view, 560, 400)
	if err := tt.Click("More for Stay"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyRight)
	tr := access.Read(t, tt)
	for _, name := range []string{"New playlist…", "Mixes", "Roadie"} {
		if _, ok := tr.Find(ui.RoleMenuItem, name); !ok {
			t.Fatalf("no entry %q:\n%s", name, tr.Dump())
		}
	}
	if n, ok := tr.Find(ui.RoleMenuItem, "Mixes"); !ok || n.Has(access.Expanded) {
		t.Fatalf("Mixes should be a collapsed submenu:\n%s", tr.Dump())
	}
	// New playlist…, then Mixes: Right opens it, and Old inside it.
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyRight)
	tr = access.Read(t, tt)
	if _, ok := tr.Find(ui.RoleMenuItem, "Gym"); !ok {
		t.Fatalf("Right did not open the folder:\n%s", tr.Dump())
	}
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyRight)
	if !tt.HasText("Dusty") {
		t.Fatalf("Right did not open the folder in the folder: %q", tt.Texts())
	}
	tt.Key(0, ui.KeyLeft)
	if tt.HasText("Dusty") || !tt.HasText("Gym") {
		t.Fatalf("Left should close one level: %q", tt.Texts())
	}
	tt.Key(0, ui.KeyRight)
	tt.Key(0, ui.KeyEnter)
	tt.Advance(50e6)
	tt.Frame()
	tt.Frame()
	if !slices.Equal(went, []string{"dusty:Stay"}) {
		t.Errorf("went %v", went)
	}
	if am.IsOpen() {
		t.Error("the menu stayed open")
	}
}
