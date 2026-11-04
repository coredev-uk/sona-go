package home

import (
	"fmt"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// rowsRig is the item rows alone in a window: what v0.2.0's ItemRows.test.tsx
// rendered.
type rowsRig struct {
	t      *testing.T
	home   *home
	router *ui.Router
	rec    *recorder
	tt     *uitest.Session
	st     rowsState
	items  []services.Item
	o      rowsOpts
	// tweak changes the context of the frame (a restricted account).
	tweak func(*kit.ItemsCtx)
	// cols is the last columns count reported.
	cols int
}

func newRowsRig(t *testing.T, items []services.Item, o rowsOpts, w, hgt int) *rowsRig {
	t.Helper()
	h := uitest.New(t, uitest.Empty())
	r := &rowsRig{t: t, router: ui.NewRouter("/"), rec: &recorder{}, items: items, o: o}
	env := new(module.Env)
	*env = *h.Env
	env.Nav = r.router
	env.HasRoute = func(p string) bool { return true }
	env.Surfaces = store.NewSurfaces()
	for _, kind := range []string{"album", "playlist", "station", "song"} {
		env.Players.Register(kind, func(it services.Item) { r.rec.add(it.Kind + ":" + it.ID) })
	}
	r.home = newHome(env, &fakeDiscover{}, &fakeLib{})
	prev := r.o.OnColumns
	r.o.OnColumns = func(n int) {
		r.cols = n
		if prev != nil {
			prev(n)
		}
	}
	r.tt = h.Tester(t, func(c *ui.Context) {
		c.DisableAnimations()
		ui.Column(c).Fill().Padding(24).Children(func() {
			ctx := env.ItemsCtx(nil)
			if r.tweak != nil {
				r.tweak(&ctx)
			}
			r.home.itemRows(c, &r.st, r.items, ctx, r.o)
			// Something after the list to leave it for.
			kit.Button(c.Key("after"), "After", kit.ButtonOpts{Variant: kit.Outline})
		})
	}, w, hgt)
	r.tt.Frame()
	r.tt.Frame()
	return r
}

// moodStations is three stations with pages, as v0.2.0's test had.
func moodStations() []services.Item {
	return []services.Item{stationItem("ra.1", "Energy", 1, true), stationItem("ra.2", "Focus", 2, true), stationItem("ra.3", "Relax", 3, true)}
}

func (r *rowsRig) key(k ui.Key) {
	r.t.Helper()
	r.tt.Key(0, k)
	r.tt.Frame()
}

func (r *rowsRig) focused(name string) {
	r.t.Helper()
	if !r.tt.Focused(name) {
		r.t.Errorf("the focus is not on %q; texts %q", name, r.tt.Texts())
	}
}

// Narrow windows give one column, wide ones two (264 wide at least, 24 apart).
const (
	oneColumn  = 320
	twoColumns = 640
)

func TestRowsAreOneTabStopAndArrowsHomeAndEndMoveBetweenThem(t *testing.T) {
	// One column: Down is the next row.
	r := newRowsRig(t, moodStations(), rowsOpts{Label: "Find Your Mood", Rows: 3}, oneColumn, 600)
	r.key(ui.KeyTab)
	r.focused("Energy")
	r.key(ui.KeyDown)
	r.focused("Focus")
	r.key(ui.KeyEnd)
	r.focused("Relax")
	r.key(ui.KeyDown) // clamped at the end
	r.focused("Relax")
	r.key(ui.KeyHome)
	r.focused("Energy")
	r.key(ui.KeyUp) // clamped at the start
	r.focused("Energy")
	// Left and Right move by one in a single column, as Up and Down.
	r.key(ui.KeyRight)
	r.focused("Focus")
	r.key(ui.KeyLeft)
	r.focused("Energy")
}

func TestRowsRoveTheirTabStopToTheRowFocusedLast(t *testing.T) {
	r := newRowsRig(t, moodStations(), rowsOpts{Label: "Find Your Mood", Rows: 3}, oneColumn, 600)
	r.key(ui.KeyTab)
	r.key(ui.KeyDown)
	r.focused("Focus")
	if f := r.st.nav.Focus(); f != 1 {
		t.Errorf("the roving stop is row %d, want 1", f)
	}
	// Tab leaves the list: it has no second stop (the play buttons and
	// menus are not stops).
	r.key(ui.KeyTab)
	r.focused("After")
	// Shift+Tab comes back to the row focused last.
	r.tt.Key(ui.Shift, ui.KeyTab)
	r.tt.Frame()
	r.focused("Focus")
}

func TestRowsArrowsMoveAcrossColumnsAndDownJumpsAColumnCount(t *testing.T) {
	items := moodStations()
	items = append(items, stationItem("ra.4", "Calm", 4, true), stationItem("ra.5", "Dream", 5, true))
	r := newRowsRig(t, items, rowsOpts{Label: "Moods", Rows: 3}, twoColumns, 600)
	if r.cols != 2 {
		t.Fatalf("%d columns at %d DIPs, want 2", r.cols, twoColumns)
	}
	r.key(ui.KeyTab)
	r.focused("Energy")
	r.key(ui.KeyRight)
	r.focused("Focus")
	r.key(ui.KeyDown) // one row down is two items on
	r.focused("Calm")
	r.key(ui.KeyDown) // the last row has no item under Calm: clamped
	r.focused("Calm")
	r.key(ui.KeyUp)
	r.focused("Focus")
	r.key(ui.KeyLeft)
	r.focused("Energy")
	r.key(ui.KeyEnd)
	r.focused("Dream")
	r.key(ui.KeyHome)
	r.focused("Energy")
}

func TestRowsPlayTheFocusedRowWithSpaceAndOpenItWithEnter(t *testing.T) {
	r := newRowsRig(t, moodStations(), rowsOpts{Label: "Find Your Mood", Rows: 3}, oneColumn, 600)
	r.key(ui.KeyTab)
	r.key(ui.KeyDown)
	r.key(ui.KeySpace)
	if got := r.rec.all(); !slices.Equal(got, []string{"station:ra.2"}) {
		t.Errorf("Space played %q, want the focused station", got)
	}
	if got := r.router.Path(); got != "/" {
		t.Errorf("Space navigated to %q", got)
	}
	r.key(ui.KeyEnter)
	if got := r.router.Path(); got != "/station/ra.2" {
		t.Errorf("Enter opened %q, want the station's page", got)
	}
}

func TestRowsShowTheRowsAskedForUntilOpened(t *testing.T) {
	items := moodStations()
	items = append(items, stationItem("ra.4", "Calm", 4, true), stationItem("ra.5", "Dream", 5, true))
	r := newRowsRig(t, items, rowsOpts{Label: "Moods", Rows: 2}, twoColumns, 800)
	if !r.tt.HasText("Calm") || r.tt.HasText("Dream") {
		t.Errorf("two rows of two should show four; texts %q", r.tt.Texts())
	}
	r.o.Open = true
	r.tt.Frame()
	r.tt.Frame()
	if !r.tt.HasText("Dream") {
		t.Errorf("opened, the fifth is missing; texts %q", r.tt.Texts())
	}
}

func TestRowsAFocusOnAHiddenRowMovesBackWhenTheListShrinks(t *testing.T) {
	items := moodStations()
	r := newRowsRig(t, items, rowsOpts{Label: "Moods", Rows: 3, Open: true}, oneColumn, 600)
	r.key(ui.KeyTab)
	r.key(ui.KeyEnd)
	r.focused("Relax")
	r.items = items[:2]
	r.tt.Frame()
	r.tt.Frame()
	if f := r.st.nav.Focus(); f > 1 {
		t.Errorf("the roving stop is row %d of two", f)
	}
}

func TestRowsShowAnExplicitMarkAndWithholdThePlayOfExplicitRowsForARestrictedAccount(t *testing.T) {
	items := moodStations()
	items[1].Explicit = true
	r := newRowsRig(t, items, rowsOpts{Label: "Moods", Rows: 3}, oneColumn, 600)
	r.tweak = func(c *kit.ItemsCtx) { c.Restricted = true }
	r.tt.Frame()
	r.key(ui.KeyTab)
	r.key(ui.KeyDown)
	r.key(ui.KeySpace)
	if got := r.rec.all(); len(got) != 0 {
		t.Errorf("a withheld row played: %q", got)
	}
	// Open still works: the page tells why it cannot play.
	r.key(ui.KeyUp)
	r.key(ui.KeySpace)
	if got := r.rec.all(); !slices.Equal(got, []string{"station:ra.1"}) {
		t.Errorf("played %q, want the unrestricted row", got)
	}
}

func TestRowsPageLessStationsPlayWhenOpened(t *testing.T) {
	items := []services.Item{stationItem("ra.1", "Local FM", 1, false), stationItem("ra.2", "Focus", 2, true)}
	r := newRowsRig(t, items, rowsOpts{Label: "Moods", Rows: 3}, oneColumn, 600)
	r.key(ui.KeyTab)
	r.key(ui.KeyEnter)
	if got := r.rec.all(); !slices.Equal(got, []string{"station:ra.1"}) {
		t.Errorf("Enter on a page-less station played %q", got)
	}
	if got := r.router.Path(); got != "/" {
		t.Errorf("a page-less station navigated to %q", got)
	}
	// A click does the same, and the play mark is no second control.
	r.rec.plays = nil
	if err := r.tt.Click("Local FM"); err != nil {
		t.Fatal(err)
	}
	r.tt.Frame()
	if got := r.rec.all(); !slices.Equal(got, []string{"station:ra.1"}) {
		t.Errorf("a click played %q", got)
	}
}

func TestRowsHoverRevealsAPlayButtonNamedForTheRow(t *testing.T) {
	r := newRowsRig(t, []services.Item{albumItem("a1", "Some Album", "Some Artist", 1)}, rowsOpts{Label: "Albums", Rows: 2}, oneColumn, 400)
	// At rest the button is built, for assistive technology, but takes
	// neither the pointer nor the Tab key.
	rest, ok := access.Read(t, r.tt.Tester).Find(ui.RoleButton, "Play Some Album")
	if !ok {
		t.Fatal("the play button is not on the accessibility tree at rest")
	}
	if rest.Has(access.Focusable) {
		t.Error("the play button is a Tab stop at rest")
	}
	name, _ := r.tt.Find("Some Album")
	r.tt.Move(name.X+4, name.Y+4)
	r.tt.Frame()
	r.tt.Frame()
	play, ok := r.tt.Find("Play Some Album")
	if !ok {
		t.Fatalf("hovering shows no play button; texts %q", r.tt.Texts())
	}
	r.tt.ClickAt(play.X+play.W/2, play.Y+play.H/2)
	r.tt.Frame()
	if got := r.rec.all(); !slices.Equal(got, []string{"album:a1"}) {
		t.Errorf("played %q", got)
	}
	if got := r.router.Path(); got != "/" {
		t.Errorf("the play button also opened the album: %q", got)
	}
}

func registerStationActions(r *rowsRig, ran *[]string) {
	r.home.env.Actions.Register(
		kit.ActionDef{
			ID: "test.station", Target: kit.KindStation, Group: kit.GroupItem,
			Label: func(*kit.ActionContext) string { return "Do something" },
			Run:   func(a *kit.ActionContext) { *ran = append(*ran, fmt.Sprint("one:", a.Item().Name())) },
		},
		kit.ActionDef{
			ID: "test.stations", Target: kit.KindSelection, Kinds: []kit.ItemKind{kit.KindStation}, Group: kit.GroupItem,
			Label: func(*kit.ActionContext) string { return "Do all of them" },
			Run:   func(a *kit.ActionContext) { *ran = append(*ran, "all") },
		},
	)
}

func TestRowsSelectWithTheCommandKeyAndMenuOnARightClick(t *testing.T) {
	r := newRowsRig(t, moodStations(), rowsOpts{Label: "Find Your Mood", Rows: 3}, oneColumn, 600)
	var ran []string
	registerStationActions(r, &ran)
	r.tt.Frame()

	// Cmd+click selects, and does not open.
	if err := r.tt.ClickWith(ui.Cmd, "Energy"); err != nil {
		t.Fatal(err)
	}
	if err := r.tt.ClickWith(ui.Cmd, "Focus"); err != nil {
		t.Fatal(err)
	}
	r.tt.Frame()
	if n := r.st.sel.Len(); n != 2 {
		t.Fatalf("%d rows selected, want 2", n)
	}
	if got := r.router.Path(); got != "/" {
		t.Errorf("a Cmd+click opened %q", got)
	}
	// A right-click on a selected row lists the actions for the selection.
	if err := r.tt.RightClick("Focus"); err != nil {
		t.Fatal(err)
	}
	r.tt.Frame()
	r.tt.Frame()
	if !r.tt.HasText("Do all of them") {
		t.Fatalf("no bulk menu; texts %q", r.tt.Texts())
	}
	// Escape closes it.
	r.key(ui.KeyEscape)
	r.tt.Advance(200e6)
	r.tt.Frame()
	if r.tt.HasText("Do all of them") {
		t.Error("Escape did not close the menu")
	}
	// With the focus on a row, Escape clears the selection.
	if err := r.tt.Click("Relax"); err != nil {
		t.Fatal(err)
	}
	r.tt.Frame()
	r.tt.Frame()
	if n := r.st.sel.Len(); n != 2 {
		t.Fatalf("a plain click on a row changed the selection to %d rows", n)
	}
	r.key(ui.KeyEscape)
	if n := r.st.sel.Len(); n != 0 {
		t.Errorf("%d rows still selected after Escape", n)
	}
}

func TestRowsRightClickOnARowOutsideTheSelectionMenusItAlone(t *testing.T) {
	r := newRowsRig(t, moodStations(), rowsOpts{Label: "Find Your Mood", Rows: 3}, oneColumn, 600)
	var ran []string
	registerStationActions(r, &ran)
	r.tt.Frame()
	if err := r.tt.RightClick("Relax"); err != nil {
		t.Fatal(err)
	}
	r.tt.Frame()
	r.tt.Frame()
	if !r.tt.HasText("Do something") {
		t.Fatalf("no menu; texts %q", r.tt.Texts())
	}
	if n := r.st.sel.Len(); n != 1 {
		t.Errorf("%d rows selected, want the right-clicked one", n)
	}
	if err := r.tt.Click("Do something"); err != nil {
		t.Fatal(err)
	}
	r.tt.Advance(300e6)
	r.tt.Frame()
	if !slices.Equal(ran, []string{"one:Relax"}) {
		t.Errorf("ran %q", ran)
	}
}

func TestRowsOpenTheirMenuFromTheKeyboard(t *testing.T) {
	r := newRowsRig(t, moodStations(), rowsOpts{Label: "Find Your Mood", Rows: 3}, oneColumn, 600)
	var ran []string
	registerStationActions(r, &ran)
	r.tt.Frame()
	r.key(ui.KeyTab)
	r.key(ui.KeyDown)
	r.tt.Key(ui.Shift, ui.KeyF10)
	r.tt.Frame()
	r.tt.Frame()
	if !r.tt.HasText("Do something") {
		t.Fatalf("Shift+F10 opened no menu; texts %q", r.tt.Texts())
	}
	r.key(ui.KeyEnter) // the menu's first row
	r.tt.Advance(300e6)
	r.tt.Frame()
	if !slices.Equal(ran, []string{"one:Focus"}) {
		t.Errorf("ran %q", ran)
	}
}

func TestRowsAreNamedForAssistiveTechnology(t *testing.T) {
	r := newRowsRig(t, moodStations(), rowsOpts{Label: "Find Your Mood", Rows: 3}, oneColumn, 600)
	if _, ok := r.tt.Find("Find Your Mood"); !ok {
		t.Errorf("the grid has no name; texts %q", r.tt.Texts())
	}
}
