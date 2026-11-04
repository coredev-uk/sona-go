package kit

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
)

// The model tests are v0.2.0's shared/selection/model.test.ts.

// keys: row 2 is still loading (no key).
var collKeys = RowKeys{"a", "b", "", "d", "e", "f"}

var (
	collPlain     = ClickMods{}
	collCtrl      = ClickMods{Toggle: true}
	collShift     = ClickMods{Range: true}
	collCtrlShift = ClickMods{Range: true, Toggle: true}
)

func collAnchor(t *testing.T, s SelectionState) int {
	t.Helper()
	a, ok := s.Anchor()
	if !ok {
		t.Fatal("no anchor")
	}
	return a
}

func collWantKeys(t *testing.T, s SelectionState, want ...string) {
	t.Helper()
	if got := s.Keys(); !slices.Equal(got, want) {
		t.Errorf("selected %v, want %v", got, want)
	}
}

func TestSelectionPlainClickSelectsOneAndMovesTheAnchor(t *testing.T) {
	s := SelectionState{}.Click(collKeys, 1, collPlain).Click(collKeys, 4, collPlain)
	collWantKeys(t, s, "e")
	if collAnchor(t, s) != 4 {
		t.Error("anchor not on the clicked row")
	}
}

func TestSelectionCtrlTogglesAndKeepsTheRest(t *testing.T) {
	s := SelectionState{}.Click(collKeys, 0, collPlain).Click(collKeys, 3, collCtrl)
	collWantKeys(t, s, "a", "d")
	s = s.Click(collKeys, 0, collCtrl)
	collWantKeys(t, s, "d")
	if collAnchor(t, s) != 0 {
		t.Error("a Ctrl-click must move the anchor even when it deselects")
	}
}

func TestSelectionShiftSelectsFromTheAnchorSkippingKeylessRows(t *testing.T) {
	s := SelectionState{}.Click(collKeys, 1, collPlain).Click(collKeys, 4, collShift)
	collWantKeys(t, s, "b", "d", "e")
	if collAnchor(t, s) != 1 {
		t.Error("Shift moved the anchor")
	}
	s = s.Click(collKeys, 0, collShift)
	collWantKeys(t, s, "a", "b")
	if collAnchor(t, s) != 1 {
		t.Error("the second Shift-click did not pivot on the same anchor")
	}
}

func TestSelectionCtrlShiftAddsARange(t *testing.T) {
	s := SelectionState{}.Click(collKeys, 0, collPlain).Click(collKeys, 4, collCtrl).Click(collKeys, 5, collCtrlShift)
	collWantKeys(t, s, "a", "e", "f")
}

func TestSelectionShiftStartsAtTheClickedRowWithoutAnAnchor(t *testing.T) {
	s := SelectionState{}.Click(collKeys, 3, collShift)
	collWantKeys(t, s, "d")
	if collAnchor(t, s) != 3 {
		t.Error("the clicked row did not become the anchor")
	}
}

func TestSelectionIgnoresRowsThatCannotBeSelected(t *testing.T) {
	s := SelectionState{}.Click(collKeys, 0, collPlain)
	if got := s.Click(collKeys, 2, collPlain); !sameSelection(got, s) {
		t.Error("a click on a keyless row changed the selection")
	}
	if got := s.Click(collKeys, 9, collPlain); !sameSelection(got, s) {
		t.Error("a click past the end changed the selection")
	}
}

func TestSelectionArrowsExtendFromTheAnchorAndPlainArrowsMoveOnlyIt(t *testing.T) {
	s := SelectionState{}.Click(collKeys, 3, collPlain).ExtendTo(collKeys, 4).ExtendTo(collKeys, 5)
	collWantKeys(t, s, "d", "e", "f")
	// Back up past the anchor: the range flips to the other side.
	s = s.ExtendTo(collKeys, 1)
	collWantKeys(t, s, "b", "d")
	s = s.MoveAnchor(5)
	collWantKeys(t, s, "b", "d")
	s = s.ExtendTo(collKeys, 4)
	collWantKeys(t, s, "e", "f")
}

func TestSelectionSelectAllAndEscape(t *testing.T) {
	s := SelectionState{}.Click(collKeys, 4, collPlain).SelectAll(collKeys)
	collWantKeys(t, s, "a", "b", "d", "e", "f")
	s = s.Cleared()
	if s.Len() != 0 || collAnchor(t, s) != 4 {
		t.Errorf("Escape left %d selected, anchor kept: %v", s.Len(), s.anchor)
	}
	if again := s.Cleared(); !sameSelection(again, s) {
		t.Error("a second Escape changed the selection")
	}
}

func TestSelectionMenuKeepsASelectedRowsSelectionAndReplacesOtherwise(t *testing.T) {
	s := SelectionState{}.Click(collKeys, 0, collPlain).Click(collKeys, 1, collCtrl)
	if got := s.MenuOn(collKeys, 1); !sameSelection(got, s) {
		t.Error("a menu on a selected row changed the selection")
	}
	other := s.MenuOn(collKeys, 4)
	collWantKeys(t, other, "e")
	if collAnchor(t, other) != 4 {
		t.Error("the menu's row did not become the anchor")
	}
}

func TestSelectionFollowsRowsByKeyWhenTheListResorts(t *testing.T) {
	s := SelectionState{}.Click(collKeys, 0, collPlain).Click(collKeys, 4, collCtrl)
	if got := s.SelectedIndices(collKeys); !slices.Equal(got, []int{0, 4}) {
		t.Errorf("indices %v", got)
	}
	if got := s.SelectedIndices(RowKeys{"e", "x", "a"}); !slices.Equal(got, []int{0, 2}) {
		t.Errorf("indices after sorting %v", got)
	}
}

func TestSelectionDrawsAdjacentRowsAsOneRun(t *testing.T) {
	s := withKeys(0, "b", "c", "d", "f")
	for _, c := range []struct {
		prev, key, next string
		want            Run
	}{
		{"a", "b", "c", RunFirst},
		{"b", "c", "d", RunMiddle},
		{"c", "d", "e", RunLast},
		{"e", "f", "", RunSingle},
		{"d", "e", "f", RunNone},
		{"", "", "b", RunNone},
	} {
		if got := s.RunAt(c.prev, c.key, c.next); got != c.want {
			t.Errorf("RunAt(%q,%q,%q) = %d, want %d", c.prev, c.key, c.next, got, c.want)
		}
	}
}

func TestSelectionRepeatedSongsGetOccurrenceKeys(t *testing.T) {
	ids := []string{"x", "y", "x", "x", ""}
	got := KeysOf(len(ids), func(i int) string { return ids[i] })
	if want := (RowKeys{"x", "y", "x#2", "x#3", ""}); !slices.Equal(got, want) {
		t.Errorf("keys %v, want %v", got, want)
	}
}

func TestSelectionGroupWithNeedsTwoRowsIncludingTheKey(t *testing.T) {
	var sel Selection
	items := []Item{{Kind: KindSong}, {Kind: KindSong}, {}, {Kind: KindSong}, {Kind: KindSong}, {Kind: KindSong}}
	sel.Rows(collKeys, func(i int) (Item, bool) { return items[i], items[i].Kind != "" })
	sel.Click(0, collPlain)
	if _, _, ok := sel.GroupWith("a"); ok {
		t.Error("one selected row is a group")
	}
	sel.Click(1, collCtrl)
	got, idx, ok := sel.GroupWith("b")
	if !ok || len(got) != 2 || !slices.Equal(idx, []int{0, 1}) {
		t.Errorf("GroupWith(b) = %v %v %v", got, idx, ok)
	}
	if _, _, ok := sel.GroupWith("e"); ok {
		t.Error("a row outside the selection is in the group")
	}
}

// collOutsideApp has a list root and a button outside it.
type collOutsideApp struct {
	sel   Selection
	hold  bool
	other int
}

func (a *collOutsideApp) view(c *ui.Context) {
	ui.Row(c).Gap(40).Padding(20).Children(func() {
		root := ui.Box(c.Key("root")).Size(100, 100)
		a.sel.OutsidePress(root)
		root.Children(func() { ui.Text(c, "inside") })
		if ui.Button(c.Key("out"), "outside").Clicked() {
			a.other++
		}
	})
}

func TestSelectionClearsOnAPressOutsideUnlessHeld(t *testing.T) {
	a := &collOutsideApp{}
	a.sel.Rows(collKeys, func(int) (Item, bool) { return Item{}, false })
	a.sel.Hold = func() bool { return a.hold }
	tt := ui.NewTester(a.view, 400, 200)
	a.sel.Click(0, collPlain)
	tt.Frame()
	if err := tt.Click("inside"); err != nil {
		t.Fatal(err)
	}
	if a.sel.Len() != 1 {
		t.Error("a press inside the list cleared the selection")
	}
	a.hold = true
	_ = tt.Click("outside")
	if a.sel.Len() != 1 {
		t.Error("a press outside cleared the selection while a menu was open")
	}
	a.hold = false
	_ = tt.Click("outside")
	if a.sel.Len() != 0 {
		t.Error("a press outside left the selection")
	}
}
