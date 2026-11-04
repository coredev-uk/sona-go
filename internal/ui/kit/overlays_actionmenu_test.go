package kit

import (
	"context"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// fakeScope is a selection of row keys.
type fakeScope struct {
	rows []Item
	sel  []int
}

func (f *fakeScope) GroupWith(key string) ([]Item, []int, bool) {
	at := -1
	for i, it := range f.rows {
		if it.ID() == key {
			at = i
		}
	}
	if at < 0 || !slices.Contains(f.sel, at) || len(f.sel) < 2 {
		return nil, nil, false
	}
	items := make([]Item, len(f.sel))
	for i, s := range f.sel {
		items[i] = f.rows[s]
	}
	return items, slices.Clone(f.sel), true
}

// listApp is a small list of rows with an ActionMenus, and a dialog some
// action opens.
type listApp struct {
	reg      *ActionRegistry
	am       *ActionMenus
	scope    *fakeScope
	rows     []Item
	ran      []string
	bulkRuns [][]string
	bulkIdx  [][]int
	loads    atomic.Int32
	dialog   bool
	posted   chan func()
	rated    int
	gate     chan struct{} // when set, the rating loads wait for it
}

func newListApp(t *testing.T) (*listApp, *ui.Tester) {
	t.Helper()
	theme.SetMode(theme.ModeLight)
	t.Cleanup(func() { theme.SetMode(theme.ModeSystem) })
	a := &listApp{posted: make(chan func(), 16)}
	a.rows = []Item{song("Airbag"), song("Paranoid Android"), song("Karma Police")}
	rd := MenuData{ID: "rating", Load: func(ctx context.Context, items []Item) any {
		a.loads.Add(1)
		if g := a.gate; g != nil {
			select {
			case <-g:
			case <-ctx.Done():
			}
		}
		return 1
	}}
	a.reg = NewActionRegistry(
		ActionDef{
			ID: "player.play-next", Target: KindSong, Group: GroupQueue, Order: 10, Icon: "list-start",
			Label: func(*ActionContext) string { return "Play next" },
			Run:   func(x *ActionContext) { a.ran = append(a.ran, "next:"+x.Item().Name()) },
		},
		ActionDef{
			ID: "player.add-to-queue", Target: KindSong, Group: GroupQueue, Order: 20, Icon: "list-end",
			Label:          func(*ActionContext) string { return "Play last" },
			DisabledReason: func(*ActionContext) string { return "Not available while listening to a live station" },
			Run:            func(x *ActionContext) { a.ran = append(a.ran, "last") },
		},
		ActionDef{
			ID: "playlists.rename", Target: KindSong, Group: GroupItem, Order: 40, Icon: "pencil",
			Label: func(*ActionContext) string { return "Rename…" },
			Run:   func(x *ActionContext) { a.dialog = true },
		},
		ActionDef{
			ID: "library.favorite.song", Target: KindSong, Group: GroupRating, Order: 10, Icon: "star",
			Data: []MenuData{rd},
			Label: func(x *ActionContext) string {
				if v, _ := DataOf[int](x, rd); v == 1 {
					return "Undo favourite"
				}
				return "Favourite"
			},
			Filled:  func(x *ActionContext) bool { v, _ := DataOf[int](x, rd); return v == 1 },
			Enabled: func(x *ActionContext) bool { _, ok := DataOf[int](x, rd); return ok },
			Run:     func(*ActionContext) { a.rated++ },
		},
		ActionDef{
			ID: "library.suggest-less.song", Target: KindSong, Group: GroupRating, Order: 20, Icon: "thumbs-down",
			Data:  []MenuData{rd},
			Label: func(*ActionContext) string { return "Suggest less" },
			Run:   func(*ActionContext) {},
		},
		ActionDef{
			ID: "player.play-next.many", Target: KindSelection, Kinds: []ItemKind{KindSong, KindAlbum}, Group: GroupQueue, Order: 10, Icon: "list-start",
			Label: func(*ActionContext) string { return "Play next" },
			Run: func(x *ActionContext) {
				var n []string
				for _, it := range x.Items {
					n = append(n, it.Name())
				}
				a.bulkRuns = append(a.bulkRuns, n)
				a.bulkIdx = append(a.bulkIdx, x.Container.Indices)
			},
		},
	)
	a.reg.SetPoster(func(fn func()) { a.posted <- fn })
	a.scope = &fakeScope{rows: a.rows}
	a.am = NewActionMenus(a.reg, a.scope)
	return a, ui.NewTester(a.view, 560, 420)
}

func (a *listApp) view(c *ui.Context) {
	theme.Apply(c)
	ui.Column(c).Padding(20).Gap(4).Children(func() {
		for i, it := range a.rows {
			tgt := ActionTarget{
				Item: it, Name: it.Name(), Key: it.ID(),
				Container: &ActionContainer{Kind: "album", ID: "al-1", Index: i, Editable: true},
			}
			row := ui.Row(c.Key("row:"+it.ID())).Focusable().Height(40).Width(480).Padding(0, 8).AlignItems(ui.Center).Label(it.Name())
			if slices.Contains(a.scope.sel, i) {
				row.Background(theme.C("active"))
			}
			a.am.Row(row, tgt)
			row.Children(func() {
				ui.Text(c, it.Name()).Grow(1)
				a.am.Trigger(c, tgt, TriggerOpts{Visible: row.Hovered() || row.FocusWithin()})
			})
		}
	})
	a.am.Popup(c)
	Dialog(c, &a.dialog, DialogOpts{Title: "Rename song"}, func() {})
}

// drain runs what the registry's poster queued (menu data loaded off the
// main thread) and redraws.
func (a *listApp) drain(t *testing.T, tt *ui.Tester, want int) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for range want {
		select {
		case fn := <-a.posted:
			fn()
		case <-deadline:
			t.Fatal("menu data never came back")
		}
	}
	tt.Frame()
}

func TestTriggerOpensMenuGroupedWithSeparatorsAndRunsAction(t *testing.T) {
	a, tt := newListApp(t)
	if err := tt.Click("More for Airbag"); err != nil {
		t.Fatal(err)
	}
	a.drain(t, tt, 1)
	for _, s := range []string{
		"Play next", "Play last", "Rename…", "Undo favourite", "Suggest less",
		"Not available while listening to a live station",
	} {
		if !tt.HasText(s) {
			t.Errorf("%q missing from the menu", s)
		}
	}
	if err := tt.Click("Play next"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if !slices.Equal(a.ran, []string{"next:Airbag"}) {
		t.Errorf("ran %v", a.ran)
	}
	if a.am.IsOpen() {
		t.Error("the menu stayed open")
	}
}

func TestRowsMenuDataLoadsOncePerIDWhenOpened(t *testing.T) {
	a, tt := newListApp(t)
	tt.Frame()
	if n := a.loads.Load(); n != 0 {
		t.Fatalf("%d loads before any menu opened", n)
	}
	_ = tt.Click("More for Airbag")
	a.drain(t, tt, 1)
	if n := a.loads.Load(); n != 1 {
		t.Errorf("%d loads for two actions sharing one MenuData, want 1", n)
	}
	// Until the data resolved, Favourite was greyed; now it is not.
	if err := tt.Click("Undo favourite"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if a.rated != 1 {
		t.Errorf("rated %d", a.rated)
	}
}

func TestFavouriteIsDisabledUntilItsDataLoads(t *testing.T) {
	a, tt := newListApp(t)
	a.gate = make(chan struct{})
	_ = tt.Click("More for Airbag")
	if !tt.HasText("Favourite") {
		t.Fatal("no Favourite row")
	}
	_ = tt.Click("Favourite")
	tt.Frame()
	if a.rated != 0 || !a.am.IsOpen() {
		t.Errorf("a disabled row ran: rated %d, open %v", a.rated, a.am.IsOpen())
	}
	close(a.gate)
	a.drain(t, tt, 1)
	if !tt.HasText("Undo favourite") {
		t.Error("the row did not follow its data")
	}
}

func TestRowTriggerIsHiddenUntilHoverButStaysNamed(t *testing.T) {
	_, tt := newListApp(t)
	tt.Frame()
	if _, ok := tt.Find("More for Airbag"); !ok {
		t.Error("the trigger is not in the tree (assistive technology must find it)")
	}
}

func TestRightClickOpensMenuAtPointAndChoosing(t *testing.T) {
	a, tt := newListApp(t)
	if err := tt.RightClick("Karma Police"); err != nil {
		t.Fatal(err)
	}
	if !a.am.IsOpen() || !tt.HasText("Play next") {
		t.Fatal("right-click did not open the menu")
	}
	a.drain(t, tt, 1)
	if err := tt.Click("Play next"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if !slices.Equal(a.ran, []string{"next:Karma Police"}) {
		t.Errorf("ran %v", a.ran)
	}
}

func TestShiftF10OpensFromKeyboardOnFirstRowAndEscapeReturnsFocusToRow(t *testing.T) {
	a, tt := newListApp(t)
	_ = tt.Click("Paranoid Android") // focuses the row
	tt.Key(ui.Shift, ui.KeyF10)
	if !a.am.IsOpen() {
		t.Fatal("Shift+F10 did not open the menu")
	}
	tt.Key(0, ui.KeyEnter) // the first row
	tt.Frame()
	tt.Frame()
	if !slices.Equal(a.ran, []string{"next:Paranoid Android"}) {
		t.Fatalf("keyboard open did not start on the first row: %v", a.ran)
	}
	tt.Key(ui.Shift, ui.KeyF10)
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.am.IsOpen() {
		t.Fatal("Escape did not close the menu")
	}
	if !tt.Focused("Paranoid Android") {
		t.Error("focus did not return to the row")
	}
}

func TestMenuKeyOpensToo(t *testing.T) {
	a, tt := newListApp(t)
	_ = tt.Click("Airbag")
	tt.Key(0, ui.KeyContextMenu)
	if !a.am.IsOpen() {
		t.Error("the Menu key did not open the menu")
	}
}

func TestChosenActionThatOpensADialogReturnsFocusToTheRow(t *testing.T) {
	a, tt := newListApp(t)
	_ = tt.Click("Karma Police")
	tt.Key(ui.Shift, ui.KeyF10)
	a.drain(t, tt, 1)
	_ = tt.Click("Rename…")
	tt.Frame()
	tt.Frame()
	if !a.dialog || !tt.HasText("Rename song") {
		t.Fatalf("the action did not open its dialog (dialog %v)", a.dialog)
	}
	tt.Key(0, ui.KeyEscape)
	tt.Advance(300 * time.Millisecond)
	if a.dialog {
		t.Fatal("Escape did not close the dialog")
	}
	if !tt.Focused("Karma Police") {
		t.Error("focus did not return to the page row after the dialog")
	}
}

func TestMultiSelectionMenuListsBulkActionsAndRunsOnce(t *testing.T) {
	a, tt := newListApp(t)
	a.scope.sel = []int{0, 2}
	tt.Frame()
	if err := tt.RightClick("Karma Police"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("2 songs selected") {
		t.Fatal(`no "2 songs selected" label`)
	}
	if tt.HasText("Favourite") || tt.HasText("Rename…") {
		t.Error("single actions listed for a bulk menu")
	}
	if err := tt.Click("Play next"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if len(a.bulkRuns) != 1 || !slices.Equal(a.bulkRuns[0], []string{"Airbag", "Karma Police"}) {
		t.Errorf("bulk runs %v", a.bulkRuns)
	}
	if !slices.Equal(a.bulkIdx[0], []int{0, 2}) {
		t.Errorf("indices %v", a.bulkIdx[0])
	}
	if len(a.ran) != 0 {
		t.Errorf("the single action ran too: %v", a.ran)
	}
}

func TestRowOutsideTheSelectionActsOnItself(t *testing.T) {
	a, tt := newListApp(t)
	a.scope.sel = []int{0, 2}
	tt.Frame()
	if err := tt.RightClick("Paranoid Android"); err != nil {
		t.Fatal(err)
	}
	if tt.HasText("selected") {
		t.Error("a row outside the selection got the bulk menu")
	}
	_ = tt.Click("Play next")
	tt.Frame()
	tt.Frame()
	if !slices.Equal(a.ran, []string{"next:Paranoid Android"}) || len(a.bulkRuns) != 0 {
		t.Errorf("ran %v bulk %v", a.ran, a.bulkRuns)
	}
}

func TestTriggerPressedAgainClosesItsMenu(t *testing.T) {
	a, tt := newListApp(t)
	_ = tt.Click("More for Airbag")
	if !a.am.IsOpen() {
		t.Fatal("not open")
	}
	_ = tt.Click("More for Airbag")
	tt.Frame()
	if a.am.IsOpen() {
		t.Error("the second press did not close the menu")
	}
}

func TestKindsWithoutActionsGetNoTrigger(t *testing.T) {
	a, tt := newListApp(t)
	a.rows = append(a.rows, StationItem(services.Station{ID: "st", Name: "Live"}))
	a.scope.rows = a.rows
	tt.Frame()
	if _, ok := tt.Find("More for Live"); ok {
		t.Error("a trigger for a kind with no registered actions")
	}
}

func TestMenuDataWithASynchronousPosterDoesNotRace(t *testing.T) {
	a, tt := newListApp(t)
	a.reg.SetPoster(func(fn func()) { fn() }) // as a test Poster does: on the load goroutine
	_ = tt.Click("More for Airbag")
	deadline := time.Now().Add(2 * time.Second)
	for !tt.HasText("Undo favourite") {
		if time.Now().After(deadline) {
			t.Fatal("the data never arrived")
		}
		time.Sleep(2 * time.Millisecond)
		tt.Frame()
	}
}
