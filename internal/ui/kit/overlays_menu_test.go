package kit

import (
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// menuApp is a view with a trigger button and the menu under test.
type menuApp struct {
	menu   Menu
	items  []MenuItem
	chosen []string
	ctx    *ui.Context
}

func (a *menuApp) view(c *ui.Context) {
	a.ctx = c
	theme.Apply(c)
	ui.Column(c).Padding(20).Gap(12).AlignItems(ui.Start).Children(func() {
		trigger := ui.ButtonBase(c.Key("trigger")).Label("Open").Padding(8)
		trigger.Children(func() { ui.Text(c, "Open") })
		if trigger.Clicked() {
			if a.menu.IsOpen() {
				a.menu.Close()
			} else {
				a.menu.OpenFrom(trigger.FocusVisible())
			}
		}
		ui.Button(c, "Elsewhere")
		a.menu.Show(c, MenuOpts{ID: "m", Label: "Test menu", Width: 224}, trigger, a.items)
	})
}

func (a *menuApp) pick(key string) func() { return func() { a.chosen = append(a.chosen, key) } }

func newMenuApp(t *testing.T) (*menuApp, *ui.Tester) {
	t.Helper()
	theme.SetMode(theme.ModeLight)
	t.Cleanup(func() { theme.SetMode(theme.ModeSystem) })
	a := &menuApp{}
	a.items = []MenuItem{
		{Key: "play", Label: "Play next", Icon: "list-start", Run: a.pick("play")},
		{Key: "last", Label: "Play last", Icon: "list-end", Run: a.pick("last")},
		{Key: "off", Label: "Station", Icon: "radio", Disabled: true, Run: a.pick("off")},
		{Key: "sep", Kind: MenuSeparator},
		{Key: "fav", Label: "Favourite", Icon: "star", Filled: true, Run: a.pick("fav")},
		{Key: "reason", Label: "Add to queue", Icon: "list-plus", Reason: "Not available while listening to a live station", Run: a.pick("reason")},
		{Key: "del", Label: "Delete", Icon: "trash", Destructive: true, Run: a.pick("del")},
	}
	return a, ui.NewTester(a.view, 480, 420)
}

func TestMenuOpensFromTriggerAndChoosesByClick(t *testing.T) {
	a, tt := newMenuApp(t)
	if err := tt.Click("Open"); err != nil {
		t.Fatal(err)
	}
	if !a.menu.IsOpen() || !tt.HasText("Play next") {
		t.Fatal("menu did not open")
	}
	if err := tt.Click("Play last"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(50e6)
	if a.menu.IsOpen() {
		t.Error("menu stayed open after a choice")
	}
	if !slices.Equal(a.chosen, []string{"last"}) {
		t.Errorf("chosen %v", a.chosen)
	}
}

func TestMenuKeyboardOpenStartsOnFirstRowAndSkipsDisabled(t *testing.T) {
	a, tt := newMenuApp(t)
	tt.Key(0, ui.KeyTab)
	if !tt.Focused("Open") {
		t.Fatal("trigger not focused")
	}
	tt.Key(0, ui.KeyEnter)
	if !a.menu.IsOpen() {
		t.Fatal("Enter did not open the menu")
	}
	if !tt.Focused("Test menu") {
		t.Error("the menu did not take the focus")
	}
	tt.Key(0, ui.KeyEnter) // the first row: Play next
	tt.Advance(50e6)
	if !slices.Equal(a.chosen, []string{"play"}) {
		t.Fatalf("keyboard open did not start on the first row: %v", a.chosen)
	}
	if !tt.Focused("Open") {
		t.Error("focus did not return to the trigger")
	}

	a.chosen = nil
	tt.Key(0, ui.KeyEnter)
	tt.Key(0, ui.KeyDown) // Play last
	tt.Key(0, ui.KeyDown) // skips the disabled Station: Favourite
	tt.Key(0, ui.KeyDown) // skips the reason row: Delete
	tt.Key(0, ui.KeyDown) // wraps to Play next
	tt.Key(0, ui.KeyUp)   // back to Delete
	tt.Key(0, ui.KeyEnter)
	tt.Advance(50e6)
	if !slices.Equal(a.chosen, []string{"del"}) {
		t.Errorf("chosen %v, want Delete", a.chosen)
	}
}

func TestMenuHomeEndAndTypeAhead(t *testing.T) {
	a, tt := newMenuApp(t)
	tt.Key(0, ui.KeyTab)
	tt.Key(0, ui.KeyEnter)
	tt.Key(0, ui.KeyEnd)
	tt.Key(0, ui.KeyEnter)
	tt.Advance(50e6)
	if !slices.Equal(a.chosen, []string{"del"}) {
		t.Fatalf("End: chosen %v", a.chosen)
	}
	a.chosen = nil
	tt.Key(0, ui.KeyEnter)
	tt.Key(0, ui.KeyEnd)
	tt.Key(0, ui.KeyHome)
	tt.TypeKey(0, ui.KeyF, "f")
	tt.TypeKey(0, ui.KeyA, "a")
	tt.Key(0, ui.KeyEnter)
	tt.Advance(50e6)
	if !slices.Equal(a.chosen, []string{"fav"}) {
		t.Errorf("type-ahead: chosen %v", a.chosen)
	}
}

func TestMenuEscapeClosesAndReturnsFocus(t *testing.T) {
	a, tt := newMenuApp(t)
	tt.Key(0, ui.KeyTab)
	tt.Key(0, ui.KeyEnter)
	tt.Key(0, ui.KeyEscape)
	tt.Advance(50e6)
	if a.menu.IsOpen() {
		t.Fatal("Escape did not close the menu")
	}
	if !tt.Focused("Open") {
		t.Error("focus did not return to the trigger")
	}
	if len(a.chosen) != 0 {
		t.Errorf("Escape chose %v", a.chosen)
	}
}

func TestMenuDisabledAndReasonRowsCannotBeChosen(t *testing.T) {
	a, tt := newMenuApp(t)
	_ = tt.Click("Open")
	_ = tt.Click("Station")
	_ = tt.Click("Add to queue")
	if !a.menu.IsOpen() || len(a.chosen) != 0 {
		t.Errorf("a disabled row was chosen: %v (open %v)", a.chosen, a.menu.IsOpen())
	}
	if !tt.HasText("Not available while listening to a live station") {
		t.Error("the reason line is missing")
	}
}

func TestMenuPressOutsideCloses(t *testing.T) {
	a, tt := newMenuApp(t)
	_ = tt.Click("Open")
	// The open menu covers the page, so the Tester refuses to click the
	// button under it by name: press where it shows, as the pointer does.
	r, ok := tt.Find("Elsewhere")
	if !ok {
		t.Fatal("no Elsewhere button")
	}
	tt.ClickAt(r.X+r.W/2, r.Y+r.H/2)
	tt.Advance(50e6)
	if a.menu.IsOpen() {
		t.Error("a press outside did not close the menu")
	}
}

func TestMenuSubmenuOpensOnRightClosesOnLeft(t *testing.T) {
	a, tt := newMenuApp(t)
	a.items = []MenuItem{
		{Key: "one", Label: "One", Run: a.pick("one")},
		{Key: "more", Label: "More", Kind: MenuSub, Sub: []MenuItem{
			{Key: "a", Label: "Alpha", Run: a.pick("a")},
			{Key: "b", Label: "Beta", Run: a.pick("b")},
		}},
	}
	tt.Key(0, ui.KeyTab)
	tt.Key(0, ui.KeyEnter)
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyRight)
	if !tt.HasText("Alpha") {
		t.Fatal("Right did not open the sub-menu")
	}
	tt.Key(0, ui.KeyLeft)
	if tt.HasText("Alpha") {
		t.Fatal("Left did not close the sub-menu")
	}
	tt.Key(0, ui.KeyRight)
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyEnter)
	tt.Advance(50e6)
	if !slices.Equal(a.chosen, []string{"b"}) {
		t.Errorf("sub-menu choice: %v", a.chosen)
	}
}

func TestMenuEscapeClosesTheSubmenuFirst(t *testing.T) {
	a, tt := newMenuApp(t)
	a.items = []MenuItem{{Key: "more", Label: "More", Kind: MenuSub, Sub: []MenuItem{{Key: "a", Label: "Alpha"}}}}
	tt.Key(0, ui.KeyTab)
	tt.Key(0, ui.KeyEnter)
	tt.Key(0, ui.KeyRight)
	tt.Key(0, ui.KeyEscape)
	if !a.menu.IsOpen() || tt.HasText("Alpha") {
		t.Fatalf("first Escape: open %v, sub shown %v", a.menu.IsOpen(), tt.HasText("Alpha"))
	}
	tt.Key(0, ui.KeyEscape)
	if a.menu.IsOpen() {
		t.Error("second Escape did not close the menu")
	}
}

func TestMenuCheckboxAndRadioRolesAndLabels(t *testing.T) {
	a, tt := newMenuApp(t)
	on := true
	a.items = []MenuItem{
		{Key: "lbl", Label: "View", Kind: MenuLabel},
		{Key: "sh", Label: "Show queue", Kind: MenuCheckbox, Checked: on, Hint: "Ctrl+Q"},
		{Key: "r1", Label: "Compact", Kind: MenuRadio, Checked: true},
		{Key: "r2", Label: "Roomy", Kind: MenuRadio},
	}
	_ = tt.Click("Open")
	for _, s := range []string{"View", "Show queue", "Compact", "Roomy", "Ctrl+Q"} {
		if !tt.HasText(s) {
			t.Errorf("%q missing", s)
		}
	}
}

func TestMenuIsCappedToTheRoomAndScrolls(t *testing.T) {
	a, _ := newMenuApp(t)
	for i := range 40 {
		a.items = append(a.items, MenuItem{Key: "x" + string(rune('A'+i%26)) + string(rune('a'+i/26)), Label: "Row " + string(rune('A'+i%26)) + string(rune('a'+i/26))})
	}
	tt := ui.NewTester(a.view, 480, 300)
	_ = tt.Click("Open")
	r, ok := tt.Find("Test menu")
	if !ok {
		t.Fatal("no menu panel")
	}
	if r.H > 300 || r.Y < 0 || r.Y+r.H > 300 {
		t.Errorf("the panel is %v in a 300 high window", r)
	}
	// A row below the fold is reached by the keys and scrolled into view.
	tt.Key(0, ui.KeyEnd)
	tt.Key(0, ui.KeyEnter)
	tt.Advance(50e6)
	if a.menu.IsOpen() {
		t.Error("End then Enter did not choose the last row")
	}
}

func TestMenuOpenedFromTheKeyboardUnderARestingPointerKeepsTheFirstRow(t *testing.T) {
	a, tt := newMenuApp(t)
	tt.Move(60, 117) // over Play last, where the menu's second row will be
	tt.Key(0, ui.KeyTab)
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	tt.Advance(50e6)
	if !slices.Equal(a.chosen, []string{"play"}) {
		t.Errorf("chosen %v: the resting pointer took the highlight from the keys", a.chosen)
	}
}

func TestMenuTypeAheadOfTheSameLetterGoesToTheNextRow(t *testing.T) {
	a, tt := newMenuApp(t)
	tt.Key(0, ui.KeyTab)
	tt.Key(0, ui.KeyEnter) // on Play next
	tt.TypeKey(0, ui.KeyP, "p")
	tt.TypeKey(0, ui.KeyP, "p")
	tt.Key(0, ui.KeyEnter)
	tt.Advance(50e6)
	// p -> Play last, p again -> on and round to Play next.
	if !slices.Equal(a.chosen, []string{"play"}) {
		t.Errorf("chosen %v, want Play next", a.chosen)
	}
}

func TestMenuSpaceChoosesAgainOnceTheTypeAheadHasExpired(t *testing.T) {
	a, tt := newMenuApp(t)
	tt.Key(0, ui.KeyTab)
	tt.Key(0, ui.KeyEnter) // on Play next
	tt.TypeKey(0, ui.KeyP, "p")
	tt.TypeKey(0, ui.KeyP, "p") // on, round to Play next
	tt.TypeKey(0, ui.KeyP, "p") // Play last
	tt.Advance(2 * time.Second)
	tt.Key(0, ui.KeySpace)
	tt.Advance(50e6)
	if !slices.Equal(a.chosen, []string{"last"}) {
		t.Errorf("chosen %v: Space after the search expired should choose the highlighted row", a.chosen)
	}
}

func TestMenuTypeAheadWorksFromKeyPressesAlone(t *testing.T) {
	a, tt := newMenuApp(t)
	tt.Key(0, ui.KeyTab)
	tt.Key(0, ui.KeyEnter)
	tt.Key(0, ui.KeyF) // GTK and AppKit send no text to a menu: only the key
	tt.Key(0, ui.KeyEnter)
	tt.Advance(50e6)
	if !slices.Equal(a.chosen, []string{"fav"}) {
		t.Errorf("chosen %v", a.chosen)
	}
}

func TestMenuKeepOpenRunsAtOnceAndLeavesTheMenuOpen(t *testing.T) {
	a, tt := newMenuApp(t)
	on := false
	a.items = []MenuItem{{Key: "priv", Kind: MenuCheckbox, Label: "Private listening", KeepOpen: true, Run: func() { on = !on }}}
	if err := tt.Click("Open"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Private listening"); err != nil {
		t.Fatal(err)
	}
	if !on || !a.menu.IsOpen() {
		t.Fatalf("after one choice: on %v, open %v", on, a.menu.IsOpen())
	}
	if err := tt.Click("Private listening"); err != nil {
		t.Fatal(err)
	}
	if on || !a.menu.IsOpen() {
		t.Fatalf("after two choices: on %v, open %v", on, a.menu.IsOpen())
	}
}

// aimMenu opens a menu at a fixed point with a sub-menu taller than the rows
// under its row open from the pointer on that row, the pointer there. It
// returns where the rows are: the middle of the menu's panel, the sub-menu
// row's and the row after it's, and the panel's right edge.
func aimMenu(t *testing.T) (a *menuApp, tt *ui.Tester, midX, moreY, nextY, right float32) {
	t.Helper()
	return aimMenuMotion(t, false)
}

// aimMenuMotion is aimMenu with the motion reduced or not: reduced, the panels
// do not fade in, so no animation frames pass before the pointer moves on.
func aimMenuMotion(t *testing.T, reduced bool) (a *menuApp, tt *ui.Tester, midX, moreY, nextY, right float32) {
	t.Helper()
	theme.SetReducedMotion(reduced)
	t.Cleanup(func() { theme.SetReducedMotion(false) })
	a, tt = newMenuApp(t)
	a.items = []MenuItem{
		{Key: "one", Label: "One", Run: a.pick("one")},
		{Key: "more", Label: "More", Kind: MenuSub, Sub: []MenuItem{
			{Key: "a", Label: "Alpha", Run: a.pick("a")},
			{Key: "b", Label: "Beta", Run: a.pick("b")},
			{Key: "c", Label: "Gamma", Run: a.pick("c")},
			{Key: "d", Label: "Delta", Run: a.pick("d")},
			{Key: "e", Label: "Epsilon", Run: a.pick("e")},
		}},
		{Key: "three", Label: "Three", Run: a.pick("three")},
		{Key: "four", Label: "Four", Run: a.pick("four")},
	}
	a.menu.OpenAt(40, 60, false)
	tt.Frame()
	tt.Frame()
	box := panelBox(a, 0)
	more, ok := tt.Find("More")
	if !ok {
		t.Fatalf("no More row: %q", tt.Texts())
	}
	next, ok := tt.Find("Three")
	if !ok {
		t.Fatalf("no Three row: %q", tt.Texts())
	}
	midX, moreY, nextY, right = box.X+box.W/2, more.Y+more.H/2, next.Y+next.H/2, box.X+box.W
	tt.Move(midX, moreY)
	if !tt.HasText("Alpha") || a.menu.levels[0].hi != "more" {
		t.Fatalf("the sub-menu did not open on the pointer: hi %q", a.menu.levels[0].hi)
	}
	return a, tt, midX, moreY, nextY, right
}

// panelBox is the box of the menu's panel at level lv, as last laid out.
func panelBox(a *menuApp, lv int) ui.Rect { return a.menu.subs[lv].Bounds(a.ctx) }

// menuHi is the highlighted row of the menu at level lv, "" when that level is
// not open.
func menuHi(a *menuApp, lv int) string {
	if lv >= len(a.menu.levels) {
		return ""
	}
	return a.menu.levels[lv].hi
}

func TestMenuPointerHeadingForTheSubmenuKeepsItOpenAcrossASiblingRow(t *testing.T) {
	a, tt, _, _, nextY, right := aimMenu(t)
	// Down and across onto Three, toward the sub-menu: no highlight moves.
	tt.Move(right-10, nextY)
	if !tt.HasText("Alpha") || menuHi(a, 0) != "more" {
		t.Fatalf("crossing Three toward the sub-menu: Alpha shown %v, highlighted %q", tt.HasText("Alpha"), menuHi(a, 0))
	}
	tt.Advance(menuAimGrace - time.Millisecond)
	if !tt.HasText("Alpha") || menuHi(a, 0) != "more" {
		t.Fatalf("1 ms before the grace ends: Alpha shown %v, highlighted %q", tt.HasText("Alpha"), menuHi(a, 0))
	}
	// Resting there, Three takes the highlight once the grace ends, and the
	// sub-menu closes.
	tt.Advance(2 * time.Millisecond)
	if tt.HasText("Alpha") || menuHi(a, 0) != "three" {
		t.Fatalf("resting on Three past the grace: Alpha shown %v, highlighted %q", tt.HasText("Alpha"), menuHi(a, 0))
	}
}

func TestMenuPointerHeadingOnKeepsTheSubmenuAndReachesIt(t *testing.T) {
	a, tt, midX, _, nextY, right := aimMenu(t)
	tt.Move(midX+(right-midX)/2, nextY-2) // onto Three, halfway
	tt.Advance(menuAimGrace / 2)
	tt.Move(right-10, nextY) // on toward the sub-menu: the grace starts again
	tt.Advance(menuAimGrace * 3 / 4)
	if !tt.HasText("Alpha") || menuHi(a, 0) != "more" {
		t.Fatalf("moving on toward the sub-menu: Alpha shown %v, highlighted %q", tt.HasText("Alpha"), menuHi(a, 0))
	}
	alpha, ok := tt.Find("Alpha")
	if !ok {
		t.Fatal("no Alpha")
	}
	tt.Move(alpha.X+alpha.W/2, alpha.Y+alpha.H/2)
	tt.Advance(time.Second)
	if !tt.HasText("Alpha") || menuHi(a, 0) != "more" || menuHi(a, 1) != "a" {
		t.Fatalf("reaching the sub-menu: Alpha shown %v, highlighted %q and %q", tt.HasText("Alpha"), menuHi(a, 0), menuHi(a, 1))
	}
}

func TestMenuPointerMovingAwayFromTheSubmenuSwitchesAtOnce(t *testing.T) {
	a, tt, _, _, nextY, _ := aimMenu(t)
	tt.Move(panelBox(a, 0).X+10, nextY) // away from it, onto Three
	if tt.HasText("Alpha") || menuHi(a, 0) != "three" {
		t.Fatalf("moving away: Alpha shown %v, highlighted %q", tt.HasText("Alpha"), menuHi(a, 0))
	}

	a, tt, midX, _, nextY, _ := aimMenu(t)
	tt.Move(midX, nextY) // straight down: outside the triangle
	if tt.HasText("Alpha") || menuHi(a, 0) != "three" {
		t.Fatalf("moving straight down: Alpha shown %v, highlighted %q", tt.HasText("Alpha"), menuHi(a, 0))
	}
}

// aimedAndResting is a pointer that crossed Three toward the sub-menu and
// rests there, inside the grace.
func aimedAndResting(t *testing.T) (*menuApp, *ui.Tester) {
	t.Helper()
	a, tt, _, _, nextY, right := aimMenu(t)
	tt.Move(right-10, nextY)
	tt.Advance(menuAimGrace / 2)
	if !tt.HasText("Alpha") || menuHi(a, 0) != "more" {
		t.Fatalf("aiming: Alpha shown %v, highlighted %q", tt.HasText("Alpha"), menuHi(a, 0))
	}
	return a, tt
}

func TestMenuKeysDuringTheAimGraceAreNotOverriddenByThePointer(t *testing.T) {
	t.Run("Right goes into the sub-menu", func(t *testing.T) {
		a, tt := aimedAndResting(t)
		tt.Key(0, ui.KeyRight)
		tt.Advance(menuAimGrace + time.Millisecond)
		if !tt.HasText("Alpha") || menuHi(a, 0) != "more" || menuHi(a, 1) != "a" {
			t.Fatalf("Right: Alpha shown %v, highlighted %q and %q", tt.HasText("Alpha"), menuHi(a, 0), menuHi(a, 1))
		}
	})
	t.Run("Up moves to the row above", func(t *testing.T) {
		a, tt := aimedAndResting(t)
		tt.Key(0, ui.KeyUp)
		tt.Advance(menuAimGrace + time.Millisecond)
		if tt.HasText("Alpha") || menuHi(a, 0) != "one" {
			t.Fatalf("Up: Alpha shown %v, highlighted %q", tt.HasText("Alpha"), menuHi(a, 0))
		}
	})
	t.Run("type-ahead moves to the row typed", func(t *testing.T) {
		a, tt := aimedAndResting(t)
		tt.TypeKey(0, ui.KeyO, "o")
		tt.Advance(menuAimGrace + time.Millisecond)
		if tt.HasText("Alpha") || menuHi(a, 0) != "one" {
			t.Fatalf("type-ahead: Alpha shown %v, highlighted %q", tt.HasText("Alpha"), menuHi(a, 0))
		}
	})
}

func TestMenuPointerHeadingRightAfterTheSubmenuOpensKeepsIt(t *testing.T) {
	for _, reduced := range []bool{false, true} {
		a, tt, _, _, nextY, right := aimMenuMotion(t, reduced)
		tt.Move(right-10, nextY)
		if !tt.HasText("Alpha") || menuHi(a, 0) != "more" {
			t.Errorf("reduced motion %v: crossing Three right after the sub-menu opened: Alpha shown %v, highlighted %q", reduced, tt.HasText("Alpha"), menuHi(a, 0))
		}
		tt.Advance(menuAimGrace + time.Millisecond)
		if tt.HasText("Alpha") || menuHi(a, 0) != "three" {
			t.Errorf("reduced motion %v: resting on Three past the grace: Alpha shown %v, highlighted %q", reduced, tt.HasText("Alpha"), menuHi(a, 0))
		}
	}
}

func TestMenuAimTowardTriangle(t *testing.T) {
	sub := ui.Rect{X: 200, Y: 100, W: 100, H: 150}
	cases := []struct {
		name     string
		from, to [2]float32
		want     bool
	}{
		{"diagonally down toward it", [2]float32{100, 110}, [2]float32{190, 140}, true},
		{"straight across", [2]float32{100, 110}, [2]float32{190, 110}, true},
		{"straight down", [2]float32{100, 110}, [2]float32{100, 140}, false},
		{"away", [2]float32{100, 110}, [2]float32{60, 120}, false},
		{"below the edge's far corner", [2]float32{100, 110}, [2]float32{190, 400}, false},
		{"over it", [2]float32{250, 110}, [2]float32{260, 140}, false},
		{"flipped, toward it from the right", [2]float32{400, 110}, [2]float32{310, 140}, true},
		{"flipped, away", [2]float32{400, 110}, [2]float32{420, 140}, false},
	}
	for _, tc := range cases {
		if got := menuAimToward(sub, tc.from, tc.to); got != tc.want {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
	if menuAimToward(ui.Rect{}, [2]float32{0, 0}, [2]float32{5, 5}) {
		t.Error("an empty panel is aimed at")
	}
}
