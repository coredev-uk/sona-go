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
