package kit

import (
	"strconv"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// centre is the middle of the first element showing text (window DIPs).
func centre(t *testing.T, tt *ui.Tester, text string) func() (float32, float32) {
	t.Helper()
	return func() (float32, float32) {
		r, ok := tt.Find(text)
		if !ok {
			t.Fatalf("no %q on screen: %q", text, tt.Texts())
		}
		return r.X + r.W/2, r.Y + r.H/2
	}
}

// Where the kit draws a "More for <name>" button, a right-click on the item
// opens the same menu at the pointer, and Shift+F10 or the Menu key open it
// from the keyboard. Trigger takes the row it belongs to, so the compact
// items cannot draw the button without the wiring.
func TestKitItemsOpenTheirMenuWhereTheirMoreButtonDoes(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T) (*ui.Tester, access.MenuWays)
	}{
		{"compact track row", func(t *testing.T) (*ui.Tester, access.MenuWays) {
			_, tt := trackApp(t, trackItems(4), "", nil)
			return tt, access.MenuWays{
				More:  "More for Song 1",
				Point: centre(t, tt, "Song 1"),
				Focus: func() { access.FocusByTab(t, tt, "Song 1") },
			}
		}},
		{"station row", func(t *testing.T) (*ui.Tester, access.MenuWays) {
			_, tt, _ := stationApp(t, []services.Station{capital, heart}, "", nil)
			return tt, access.MenuWays{
				More:  "More for Capital",
				Point: centre(t, tt, "Capital"),
				Focus: func() { access.FocusByTab(t, tt, "Capital") },
			}
		}},
		{"wide-row card", func(t *testing.T) (*ui.Tester, access.MenuWays) {
			items := []services.Item{wideItem("1", artPath("1", 1000, 250), "")}
			st := &WideRowState{}
			a, tt := newItemsApp(t, 2*240+24+48, 400, nil, func(c *ui.Context, ctx ItemsCtx) {
				WideRow(c, st, items, ctx, WideRowOpts{Title: "Row", Min: 240, Ratio: 2, First: true})
			})
			a.load(t, tt, 0)
			return tt, access.MenuWays{
				More:  "More for Item 1",
				Point: centre(t, tt, "Item 1"),
				Focus: func() { access.FocusByTab(t, tt, "Item 1") },
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tt, ways := tc.setup(t)
			access.CheckMenuWays(t, tt, ways)
		})
	}
}

// wiringApp draws one Trigger in each of the shapes a surface gives it: a
// list row with a selection, a page's head (a right-click on its non-interactive
// artwork or title), and a row whose title is a link of its own.
type wiringApp struct {
	*listApp
	sel   Selection
	title bool // the head's title is a link
}

func (w *wiringApp) view(c *ui.Context) {
	theme.Apply(c)
	rows := w.rows
	ui.Column(c).Padding(20).Gap(12).Children(func() {
		// A head: the cover and the name are plain, the button is in its row of
		// actions.
		it := rows[0]
		tgt := ActionTarget{Item: it, Name: it.Name(), Key: it.ID()}
		head := ui.Row(c.Key("head")).Height(120).Width(480).Gap(16).Padding(8)
		head.Children(func() {
			ui.Box(c.Key("cover")).Size(100, 100).Background(theme.C("hover"))
			ui.Column(c.Key("text")).Children(func() {
				var name ui.Element
				if w.title {
					name = ui.Link(c.Key("name"), "Head "+it.Name(), "/x")
				} else {
					name = ui.Text(c, "Head "+it.Name())
				}
				ui.Row(c.Key("actions")).Margin(12, 0, 0, 0).Children(func() {
					w.am.Trigger(c, head, tgt, TriggerOpts{Header: true}, name)
				})
			})
		})
		// A list row, its selection and its title link.
		for i, r := range rows[1:] {
			i++
			t := ActionTarget{Item: r, Name: r.Name(), Key: r.ID()}
			row := ui.Row(c.Key("row:"+strconv.Itoa(i))).Focusable().Height(40).Width(480).Padding(0, 8).AlignItems(ui.Center).Label(r.Name())
			row.Children(func() {
				ui.Text(c, r.Name()).Grow(1)
				w.am.Trigger(c, row, t, TriggerOpts{Visible: row.Hovered() || row.FocusWithin(), Select: &w.sel, Index: i})
			})
		}
	})
	w.am.Popup(c)
}

func TestTriggerWiresTheHeadItBelongsTo(t *testing.T) {
	for _, linked := range []bool{false, true} {
		name := "plain title"
		if linked {
			name = "link title"
		}
		t.Run(name, func(t *testing.T) {
			a, _ := newListApp(t)
			w := &wiringApp{listApp: a, title: linked}
			tt := ui.NewTester(w.view, 560, 420)
			tt.Frame()
			access.CheckMenuWays(t, tt, access.MenuWays{
				More:  "More for Airbag",
				Point: centre(t, tt, "Head Airbag"),
				Focus: func() { access.FocusByTab(t, tt, "More for Airbag") },
			})
			// The cover (no text) is the head's too.
			tt.RightClickAt(70, 70)
			tt.Frame()
			tt.Frame()
			if m, ok := access.Read(t, tt).OpenMenu(); !ok || m.Label != "More for Airbag" {
				t.Errorf("a right-click on the head's cover opened %v (%v)", m, ok)
			}
		})
	}
}

// A right-click on a row outside the selection selects it alone, so the menu
// acts on it, as the lists' own rows do.
func TestTriggerWithASelectionSelectsTheRightClickedRowAlone(t *testing.T) {
	a, _ := newListApp(t)
	w := &wiringApp{listApp: a}
	w.sel.Rows(KeysOf(len(a.rows), func(i int) string { return a.rows[i].ID() }), func(i int) (Item, bool) { return a.rows[i], true })
	tt := ui.NewTester(w.view, 560, 420)
	tt.Frame()
	if err := tt.RightClick("Karma Police"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if !a.am.IsOpen() {
		t.Fatal("a right-click opened no menu")
	}
	if got := w.sel.Len(); got != 1 || !w.sel.Selected(a.rows[2].ID()) {
		t.Errorf("%d rows selected after a right-click on Karma Police, want it alone", got)
	}
}
