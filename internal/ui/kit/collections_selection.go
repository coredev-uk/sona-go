package kit

import (
	"slices"
	"strconv"

	"github.com/egoist/mygo/ui"
)

// The multi-select model shared by track lists and cover grids (v0.2.0's
// shared/selection/model.ts): which rows are selected, by key so a
// selection survives re-sorting and paged loading, and the anchor that
// Shift ranges start from, a position in the list as shown. A row without a
// key (still loading, not selectable) is never selected. SelectionState's
// operations are pure; Selection keeps one per list.

// Rows are a list's rows as the selection sees them: how many, and each row's
// key in display order ("" for a row that cannot be selected: still loading,
// no target). A long list implements it over its own data so no frame
// builds a key slice.
type Rows interface {
	Len() int
	Key(i int) string
}

// RowKeys is Rows over a slice of keys.
type RowKeys []string

// Len is the number of rows.
func (k RowKeys) Len() int { return len(k) }

// Key is row i's key.
func (k RowKeys) Key(i int) string { return k[i] }

// SelectionState is the set of selected row keys and the anchor. The zero
// value is "nothing selected, no anchor" once Anchor is read through
// HasAnchor; methods return a new state and never change the receiver.
type SelectionState struct {
	keys   map[string]struct{}
	anchor int // position of the anchor + 1; 0 is none (so the zero value is empty)
}

// Len is how many rows are selected.
func (s SelectionState) Len() int { return len(s.keys) }

// Has reports whether key is selected.
func (s SelectionState) Has(key string) bool {
	if key == "" {
		return false
	}
	_, ok := s.keys[key]
	return ok
}

// Anchor is where Shift+click and Shift+arrow ranges start, and whether
// there is one (false before any choice).
func (s SelectionState) Anchor() (int, bool) { return s.anchor - 1, s.anchor > 0 }

// Keys are the selected keys, sorted (a stable order for tests; list order
// is SelectedIndices).
func (s SelectionState) Keys() []string {
	out := make([]string, 0, len(s.keys))
	for k := range s.keys {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func withKeys(anchor int, keys ...string) SelectionState {
	m := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		m[k] = struct{}{}
	}
	return SelectionState{keys: m, anchor: anchor + 1}
}

// ClickMods are the modifiers of a click on a row.
type ClickMods struct {
	// Range is Shift: select the range from the anchor.
	Range bool
	// Toggle is the command key (Ctrl, Cmd on macOS): toggle one row, or add a range to what is
	// selected.
	Toggle bool
}

// ModsOf reads a click's modifier keys: Shift is Range, the platform command
// key (ui.Cmd: Ctrl, Cmd on macOS) is Toggle.
func ModsOf(m ui.Modifiers) ClickMods {
	return ClickMods{Range: m&ui.Shift != 0, Toggle: m&ui.Cmd != 0}
}

// rangeKeys are the keys of rows a…b inclusive (either order), skipping
// keyless rows.
func rangeKeys(rows Rows, a, b int) []string {
	lo, hi := max(0, min(a, b)), min(rows.Len()-1, max(a, b))
	var out []string
	for i := lo; i <= hi; i++ {
		if k := rows.Key(i); k != "" {
			out = append(out, k)
		}
	}
	return out
}

// Click is a click on row index: alone it selects just that row; Ctrl
// toggles it; Shift selects anchor…index (Ctrl+Shift adds that range to
// what is selected). The anchor moves to a clicked row except on Shift, so
// successive Shift-clicks pivot on it. A click on a keyless row returns the
// same selection.
func (s SelectionState) Click(keys Rows, index int, mods ClickMods) SelectionState {
	if index < 0 || index >= keys.Len() || keys.Key(index) == "" {
		return s
	}
	key := keys.Key(index)
	if mods.Range {
		anchor, ok := s.Anchor()
		if !ok {
			anchor = index
		}
		var base []string
		if mods.Toggle {
			base = s.Keys()
		}
		return withKeys(anchor, append(base, rangeKeys(keys, anchor, index)...)...)
	}
	if mods.Toggle {
		next := s.Keys()
		if i := slices.Index(next, key); i >= 0 {
			next = slices.Delete(next, i, i+1)
		} else {
			next = append(next, key)
		}
		return withKeys(index, next...)
	}
	return withKeys(index, key)
}

// ExtendTo is Shift+arrow onto row index: the selection becomes
// anchor…index, flipping to the other side of the anchor when the arrow
// goes back past it.
func (s SelectionState) ExtendTo(keys Rows, index int) SelectionState {
	anchor, ok := s.Anchor()
	if !ok {
		anchor = index
	}
	return withKeys(anchor, rangeKeys(keys, anchor, index)...)
}

// MoveAnchor is a plain arrow onto row index: the selection stays and the
// next range starts there.
func (s SelectionState) MoveAnchor(index int) SelectionState {
	if a, ok := s.Anchor(); ok && a == index {
		return s
	}
	return SelectionState{keys: s.keys, anchor: index + 1}
}

// SelectAll is Ctrl+A (Cmd+A on macOS): every selectable row, the anchor where it was.
func (s SelectionState) SelectAll(keys Rows) SelectionState {
	next := make([]string, 0, keys.Len())
	for i := range keys.Len() {
		if k := keys.Key(i); k != "" {
			next = append(next, k)
		}
	}
	out := withKeys(0, next...)
	out.anchor = s.anchor
	return out
}

// MenuOn is a right-click, or the menu key, on row index: outside the
// selection it selects just that row and moves the anchor; inside, it
// keeps the selection.
func (s SelectionState) MenuOn(keys Rows, index int) SelectionState {
	if index < 0 || index >= keys.Len() || keys.Key(index) == "" || s.Has(keys.Key(index)) {
		return s
	}
	return withKeys(index, keys.Key(index))
}

// Cleared is Escape: nothing selected, the anchor kept. An empty selection
// is returned as it is.
func (s SelectionState) Cleared() SelectionState {
	if len(s.keys) == 0 {
		return s
	}
	return SelectionState{anchor: s.anchor}
}

// SelectedIndices are the positions of the selected rows in the list as it
// is now, in list order: a selection follows its rows by key when the list
// sorts again.
func (s SelectionState) SelectedIndices(keys Rows) []int {
	if len(s.keys) == 0 {
		return nil
	}
	var out []int
	for i := range keys.Len() {
		if k := keys.Key(i); k != "" && s.Has(k) {
			out = append(out, i)
		}
	}
	return out
}

// Run is where a selected row sits in its run of selected neighbours, so a
// run is drawn as one block.
type Run uint8

// The positions in a run; RunNone is not selected (or keyless).
const (
	RunNone Run = iota
	RunSingle
	RunFirst
	RunMiddle
	RunLast
)

// RunAt is the Run of the row with key between prev and next ("" for a
// neighbour that does not exist or cannot be selected).
func (s SelectionState) RunAt(prev, key, next string) Run {
	if !s.Has(key) {
		return RunNone
	}
	before, after := s.Has(prev), s.Has(next)
	switch {
	case before && after:
		return RunMiddle
	case before:
		return RunLast
	case after:
		return RunFirst
	}
	return RunSingle
}

// Selection is one list's selection: the state and the rows it is over as
// the list last built them (v0.2.0's SelectionScope). A list builds one
// as a field of its state, calls Rows every frame with its rows, and its
// rows ask it (Selected, Run); menus and drags ask GroupWith. Main thread
// only.
type Selection struct {
	state SelectionState
	keys  Rows
	item  func(i int) (Item, bool)
	// version counts changes, for the owner to know a frame is due.
	version uint64

	// Hold, when set and true, keeps a press outside the list from
	// clearing the selection: a menu or dialog is open (the press belongs
	// to it).
	Hold func() bool
}

// Rows points the selection at the list's rows as it builds them now: keys
// in display order ("" for a row that cannot be selected) and itemAt, which
// returns a row's item (false for none).
func (s *Selection) Rows(keys Rows, itemAt func(i int) (Item, bool)) {
	s.keys, s.item = keys, itemAt
}

// State is the selection as a value.
func (s *Selection) State() SelectionState { return s.state }

// Version changes whenever the selection does.
func (s *Selection) Version() uint64 { return s.version }

func (s *Selection) set(next SelectionState) {
	if sameSelection(s.state, next) {
		return
	}
	s.state = next
	s.version++
}

func sameSelection(a, b SelectionState) bool {
	if a.anchor != b.anchor || len(a.keys) != len(b.keys) {
		return false
	}
	for k := range a.keys {
		if _, ok := b.keys[k]; !ok {
			return false
		}
	}
	return true
}

// Len is how many rows are selected.
func (s *Selection) Len() int { return s.state.Len() }

// Selected reports whether the row with key is selected.
func (s *Selection) Selected(key string) bool { return s.state.Has(key) }

// Run is where row index sits in its run of selected rows.
func (s *Selection) Run(index int) Run {
	if s.keys == nil || index < 0 || index >= s.keys.Len() {
		return RunNone
	}
	var prev, next string
	if index > 0 {
		prev = s.keys.Key(index - 1)
	}
	if index+1 < s.keys.Len() {
		next = s.keys.Key(index + 1)
	}
	return s.state.RunAt(prev, s.keys.Key(index), next)
}

// Click applies a click on row index.
func (s *Selection) Click(index int, mods ClickMods) {
	if s.keys != nil {
		s.set(s.state.Click(s.keys, index, mods))
	}
}

// MenuOn applies a context menu opened on row index.
func (s *Selection) MenuOn(index int) {
	if s.keys != nil {
		s.set(s.state.MenuOn(s.keys, index))
	}
}

// Clear is Escape.
func (s *Selection) Clear() { s.set(s.state.Cleared()) }

// Reset forgets the selection and the anchor (the list changed).
func (s *Selection) Reset() { s.set(SelectionState{}) }

// ArrowTo is what an arrow key that moved the focus to row index does to the
// selection: Shift extends it to index, a plain arrow moves the anchor.
func (s *Selection) ArrowTo(index int, shift bool) {
	if s.keys == nil {
		return
	}
	if shift {
		s.set(s.state.ExtendTo(s.keys, index))
		return
	}
	s.set(s.state.MoveAnchor(index))
}

// Indices are the selected rows' positions in list order.
func (s *Selection) Indices() []int {
	if s.keys == nil {
		return nil
	}
	return s.state.SelectedIndices(s.keys)
}

// Items are the selected rows' items in list order with their positions
// (rows without an item are left out).
func (s *Selection) Items() (items []Item, indices []int) {
	for _, i := range s.Indices() {
		if s.item == nil {
			break
		}
		if it, ok := s.item(i); ok {
			items = append(items, it)
			indices = append(indices, i)
		}
	}
	return items, indices
}

// GroupWith is the selection as menus and drags use it: the selected items
// when two or more rows are selected including key, else ok is false (the
// row acts alone). It makes Selection a SelectionScope.
func (s *Selection) GroupWith(key string) (items []Item, indices []int, ok bool) {
	if s.state.Len() < 2 || !s.state.Has(key) {
		return nil, nil, false
	}
	items, indices = s.Items()
	return items, indices, true
}

// Keys handles the selection's keys on the item s, which is row focus and has
// the focus: Escape (with a selection), Ctrl+A and Ctrl+Space. Shift+F10 and
// the menu key belong to the menus (ActionMenus.Row). It reports whether it
// took a key.
func (s *Selection) Keys(el ui.Element, focus int) bool {
	if !el.Focused() {
		return false
	}
	switch {
	case s.state.Len() > 0 && el.Shortcut(0, ui.KeyEscape):
		s.Clear()
	case s.keys != nil && el.Shortcut(ui.Cmd, ui.KeyA):
		s.set(s.state.SelectAll(s.keys))
	case el.Shortcut(ui.Cmd, ui.KeySpace):
		s.Click(focus, ClickMods{Toggle: true})
	default:
		return false
	}
	return true
}

// OutsidePress clears the selection when a press landed outside the list
// (root.PressedOutside), unless Hold says a menu or dialog is open.
func (s *Selection) OutsidePress(root ui.Element) {
	if s.state.Len() == 0 || !root.PressedOutside() {
		return
	}
	if s.Hold != nil && s.Hold() {
		return
	}
	s.set(SelectionState{})
}

// KeysOf builds row keys from per-row keys where a repeated key gets "#n"
// (a playlist's second copy of a song), as TrackList does for songs.
func KeysOf(n int, key func(i int) string) RowKeys {
	out := make(RowKeys, n)
	seen := map[string]int{}
	for i := range out {
		k := key(i)
		if k == "" {
			continue
		}
		seen[k]++
		if seen[k] > 1 {
			k += "#" + strconv.Itoa(seen[k])
		}
		out[i] = k
	}
	return out
}
