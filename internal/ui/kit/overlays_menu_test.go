package kit

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// menuApp is a view with a trigger button and the menu under test.
type menuApp struct {
	menu   Menu
	items  []MenuItem
	chosen []string
}

func (a *menuApp) view(c *ui.Context) {
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
	_ = tt.Click("Elsewhere")
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
	tt.Move(60, 85) // where the menu's rows will be
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

func TestMenuReasonRowSaysWhyToAssistiveTechnology(t *testing.T) {
	_, tt := newMenuApp(t)
	_ = tt.Click("Open")
	if !tt.HasText("Not available while listening to a live station") {
		t.Error("the reason is not shown")
	}
}
