// Package access reads what assistive technology sees of a ui.Tester window:
// the roles, names and states of the tree, and the Tab stops on it. It lives
// beside uitest, not in it, so that the kit's own tests (which uitest
// imports) can use it.
package access

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"unsafe"

	"github.com/egoist/mygo/ui"
)

// What assistive technology sees of a window, read from a ui.Tester: the
// tree MyGo describes for it (roles, names, states), which the Tester does
// not expose. MyGo's engine builds the tree only once assistive technology
// asked, so Read asks, as a screen reader does, and reads the tree the
// headless window kept of the last frame.
//
// It reaches the Tester's unexported fields by name (rt, h, access): MyGo is
// pinned exactly, and TestPinnedMyGo fails at once on a bump so that the
// constants below (the flag bits of ui/element.go, the element kinds, the
// order of internal/platform's states and actions, the leaf roles of
// ui/access.go) are re-checked; the canaries in access_test.go check them
// against what the engine builds.

// State is a bit of a node's states.
type State uint32

// The states of a node, in MyGo's order (internal/platform.AccessStates).
const (
	Focusable State = 1 << iota
	Disabled
	Checked
	Mixed
	Selected
	Expanded
	Multiline
	Password
	ReadOnly
	Selectable
	Offscreen
	Multiselectable
	Segment
	Search
	Invalid
	SortAscending
	SortDescending
	Expandable
	Vertical
	Busy
	SortOther
	Horizontal
)

// Action is an action assistive technology can ask of a node.
type Action uint8

// The actions of a node, in MyGo's order (internal/platform.AccessActions).
const (
	Press Action = 1 << iota
	Focus
	Increment
	Decrement
	SetValue
	ScrollIntoView
	Expand
)

// Node is an element of the tree assistive technology reads.
type Node struct {
	// Index is the node's place in Tree.Nodes; Parent its parent's, -1 at the top.
	Index, Parent int
	ID            uint64
	// Role is MyGo's role (ui.RoleButton, ui.RoleLink …).
	Role                      ui.Role
	Label, Value, Description string
	States                    State
	Actions                   Action
	Level, PosInSet, SetSize  int
	// RowCount and ColCount are how many rows and columns a table, grid or
	// tree grid says it has, built or not (aria-rowcount, aria-colcount);
	// RowIndex and ColIndex the place of a row or a cell among them, from
	// 1, header rows counted (aria-rowindex, aria-colindex). 0 when not
	// given.
	RowCount, ColCount, RowIndex, ColIndex int
	Min, Max, Now                          float64
	// Current is the kind of current item the node is (aria-current),
	// ui.CurrentNone for none; HasPopup the kind of popup it opens
	// (aria-haspopup), ui.PopupNone for none.
	Current  ui.Current
	HasPopup ui.Popup
	// Controls are the IDs of the nodes it controls (aria-controls).
	Controls []uint64
}

// Has reports whether every state in s is set.
func (n Node) Has(s State) bool { return n.States&s == s }

// Can reports whether assistive technology can ask the node for a.
func (n Node) Can(a Action) bool { return n.Actions&a == a }

func (n Node) String() string {
	return fmt.Sprintf("%s %q", roleName(n.Role), n.Label)
}

// Tree is the nodes assistive technology reads, parents before children.
type Tree struct {
	Nodes []Node
	// Focus is the node with the keyboard focus: nil for none, and for a
	// focused element assistive technology cannot see (inside a button, a
	// link, or any element it reads as one piece).
	Focus *Node
	// FocusID is the focused element's ID, 0 when the window has no focus.
	FocusID uint64
	// Announcements are what the frame asked screen readers to read.
	Announcements []string
}

// Read asks for the accessibility tree, draws a frame and reads it.
func Read(t testing.TB, tt *ui.Tester) Tree {
	t.Helper()
	tree, err := readTree(tt)
	if err != nil {
		t.Fatalf("access.Read: %v", err)
	}
	return tree
}

func readTree(tt *ui.Tester) (Tree, error) {
	tv := reflect.ValueOf(tt).Elem()
	rt, h := tv.FieldByName("rt"), tv.FieldByName("h")
	if !rt.IsValid() || !h.IsValid() {
		return Tree{}, fmt.Errorf("ui.Tester has no fields rt and h any more")
	}
	on := rt.Elem().FieldByName("access")
	if !on.IsValid() || on.Kind() != reflect.Bool {
		return Tree{}, fmt.Errorf("the engine has no access flag any more")
	}
	reflect.NewAt(on.Type(), unsafe.Pointer(on.UnsafeAddr())).Elem().SetBool(true)
	tt.Frame()
	tree := h.Elem().FieldByName("access")
	if !tree.IsValid() {
		return Tree{}, fmt.Errorf("the headless window keeps no access tree any more")
	}
	if tree.IsNil() {
		return Tree{}, fmt.Errorf("no accessibility tree after a frame")
	}
	tree = tree.Elem()
	given := givenRoles(rt)
	var out Tree
	nodes := tree.FieldByName("Nodes")
	focus := tree.FieldByName("Focus").Uint()
	out.FocusID = focus
	for i := range nodes.Len() {
		v := nodes.Index(i)
		id := v.FieldByName("ID").Uint()
		role, ok := given[id]
		if !ok {
			role = ui.Role(v.FieldByName("Role").Uint()) + ui.RoleGroup
		}
		n := Node{
			Index:       i,
			Parent:      int(v.FieldByName("Parent").Int()),
			ID:          id,
			Role:        role,
			Label:       v.FieldByName("Label").String(),
			Value:       v.FieldByName("Value").String(),
			Description: v.FieldByName("Description").String(),
			States:      State(v.FieldByName("States").Uint()),
			Actions:     Action(v.FieldByName("Actions").Uint()),
			Level:       int(v.FieldByName("Level").Int()),
			PosInSet:    int(v.FieldByName("PosInSet").Int()),
			SetSize:     int(v.FieldByName("SetSize").Int()),
			RowCount:    int(v.FieldByName("RowCount").Int()),
			ColCount:    int(v.FieldByName("ColCount").Int()),
			RowIndex:    int(v.FieldByName("RowIndex").Int()),
			ColIndex:    int(v.FieldByName("ColIndex").Int()),
			Min:         v.FieldByName("Min").Float(),
			Max:         v.FieldByName("Max").Float(),
			Now:         v.FieldByName("Now").Float(),
			Current:     ui.Current(v.FieldByName("Current").Uint()),
			HasPopup:    ui.Popup(v.FieldByName("HasPopup").Uint()),
		}
		ctl := v.FieldByName("Controls")
		for j := range ctl.Len() {
			n.Controls = append(n.Controls, ctl.Index(j).Uint())
		}
		out.Nodes = append(out.Nodes, n)
	}
	if focus != 0 {
		for i := range out.Nodes {
			if out.Nodes[i].ID == focus {
				out.Focus = &out.Nodes[i]
			}
		}
	}
	anns := tree.FieldByName("Announcements")
	for i := range anns.Len() {
		out.Announcements = append(out.Announcements, anns.Index(i).String())
	}
	return out, nil
}

// givenRoles are the roles the elements of the last frame were given, by
// their IDs, as MyGo's Tester.Access reads them (ui/tester_access.go): the
// platform's roles follow ui's in order only up to RoleTreeGrid, a list box
// is a list and an option a list item to the platform, and a List's rows
// shown as a table are its rows. Elements of no role of their own (a text,
// a button…) are read from the node's role.
func givenRoles(rt reflect.Value) map[uint64]ui.Role {
	given := map[uint64]ui.Role{}
	root := rt.Elem().FieldByName("c").FieldByName("root")
	if !root.IsValid() || root.IsNil() {
		return given
	}
	var walk func(v reflect.Value)
	walk = func(v reflect.Value) {
		if r := ui.Role(v.FieldByName("role").Uint()); r > ui.RoleNone && r != ui.RoleListItem {
			given[v.FieldByName("id").Uint()] = r
		}
		for _, ch := range children(v) {
			walk(ch)
		}
	}
	walk(root.Elem())
	return given
}

// Find is the first node of role whose label is exactly label.
func (tr Tree) Find(role ui.Role, label string) (Node, bool) {
	for _, n := range tr.Nodes {
		if n.Role == role && n.Label == label {
			return n, true
		}
	}
	return Node{}, false
}

// FindAll are the nodes of role whose label is exactly label.
func (tr Tree) FindAll(role ui.Role, label string) []Node {
	var out []Node
	for _, n := range tr.Nodes {
		if n.Role == role && n.Label == label {
			out = append(out, n)
		}
	}
	return out
}

// Labelled are the nodes of role whose label starts with prefix.
func (tr Tree) Labelled(role ui.Role, prefix string) []Node {
	var out []Node
	for _, n := range tr.Nodes {
		if n.Role == role && strings.HasPrefix(n.Label, prefix) {
			out = append(out, n)
		}
	}
	return out
}

// ControlledBy are the nodes n controls (aria-controls), in its order.
func (tr Tree) ControlledBy(n Node) []Node {
	var out []Node
	for _, id := range n.Controls {
		for _, m := range tr.Nodes {
			if m.ID == id {
				out = append(out, m)
				break
			}
		}
	}
	return out
}

// Roles are the nodes of role.
func (tr Tree) Roles(role ui.Role) []Node {
	var out []Node
	for _, n := range tr.Nodes {
		if n.Role == role {
			out = append(out, n)
		}
	}
	return out
}

// Children are the nodes directly below n.
func (tr Tree) Children(n Node) []Node {
	var out []Node
	for _, c := range tr.Nodes[n.Index+1:] {
		if c.Parent == n.Index {
			out = append(out, c)
		}
	}
	return out
}

// Ancestors are the nodes above n, nearest first.
func (tr Tree) Ancestors(n Node) []Node {
	var out []Node
	for p := n.Parent; p >= 0; p = tr.Nodes[p].Parent {
		out = append(out, tr.Nodes[p])
	}
	return out
}

// Within reports whether n has an ancestor of role.
func (tr Tree) Within(n Node, role ui.Role) bool {
	return slices.ContainsFunc(tr.Ancestors(n), func(a Node) bool { return a.Role == role })
}

// Texts are the labels of every node, for a failure message.
func (tr Tree) Texts() []string {
	out := make([]string, len(tr.Nodes))
	for i, n := range tr.Nodes {
		out[i] = n.String()
	}
	return out
}

// Dump is the tree as indented "role label [states]" lines.
func (tr Tree) Dump() string {
	var b strings.Builder
	depth := make([]int, len(tr.Nodes))
	for i, n := range tr.Nodes {
		if n.Parent >= 0 {
			depth[i] = depth[n.Parent] + 1
		}
		fmt.Fprintf(&b, "%s%s", strings.Repeat("  ", depth[i]), n)
		if n.States != 0 {
			fmt.Fprintf(&b, " %s", n.States)
		}
		if n.Value != "" {
			fmt.Fprintf(&b, " value=%q", n.Value)
		}
		if n.Description != "" {
			fmt.Fprintf(&b, " description=%q", n.Description)
		}
		if n.Level != 0 {
			fmt.Fprintf(&b, " level=%d", n.Level)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func (s State) String() string {
	names := []string{"focusable", "disabled", "checked", "mixed", "selected", "expanded", "multiline", "password", "readonly", "selectable", "offscreen", "multiselectable", "segment", "search", "invalid", "ascending", "descending", "expandable", "vertical", "busy", "sorted", "horizontal"}
	var on []string
	for i, n := range names {
		if s&(1<<i) != 0 {
			on = append(on, n)
		}
	}
	return "[" + strings.Join(on, " ") + "]"
}

func roleName(r ui.Role) string {
	names := map[ui.Role]string{
		ui.RoleGroup: "group", ui.RoleText: "text", ui.RoleButton: "button", ui.RoleLink: "link",
		ui.RoleCheckBox: "checkbox", ui.RoleRadio: "radio", ui.RoleSwitch: "switch", ui.RoleSlider: "slider",
		ui.RoleProgress: "progress", ui.RoleTextField: "textfield", ui.RoleImage: "image", ui.RoleList: "list",
		ui.RoleScroll: "scroll", ui.RoleDialog: "dialog", ui.RolePopup: "popup", ui.RoleTooltip: "tooltip",
		ui.RolePopUpButton: "popupbutton", ui.RoleTabList: "tablist", ui.RoleTab: "tab", ui.RoleSplitter: "splitter",
		ui.RoleStatus: "status", ui.RoleTable: "table", ui.RoleRow: "row", ui.RoleCell: "cell",
		ui.RoleColumnHeader: "columnheader", ui.RoleTree: "tree", ui.RoleTreeItem: "treeitem", ui.RoleListItem: "listitem",
		ui.RoleMenuButton: "menubutton", ui.RoleToolbar: "toolbar", ui.RoleRadioGroup: "radiogroup",
		ui.RoleToggleButton: "togglebutton", ui.RoleComboBox: "combobox", ui.RoleDisclosure: "disclosure",
		ui.RoleMeter: "meter", ui.RoleStepper: "stepper", ui.RoleColorWell: "colorwell", ui.RoleAlertDialog: "alertdialog",
		ui.RoleMenu: "menu", ui.RoleMenuBar: "menubar", ui.RoleMenuItem: "menuitem",
		ui.RoleMenuItemCheckBox: "menuitemcheckbox", ui.RoleMenuItemRadio: "menuitemradio", ui.RoleHeading: "heading",
		ui.RoleRegion: "region", ui.RoleLog: "log", ui.RoleTimer: "timer", ui.RoleAlert: "alert",
		ui.RoleSeparator: "separator", ui.RoleRowHeader: "rowheader", ui.RoleSearch: "search", ui.RoleArticle: "article",
		ui.RoleNavigation: "navigation", ui.RoleGrid: "grid", ui.RoleTreeGrid: "treegrid", ui.RoleListBox: "listbox",
		ui.RoleOption: "option", ui.RoleDeletion: "deletion", ui.RoleInsertion: "insertion",
	}
	if n, ok := names[r]; ok {
		return n
	}
	return fmt.Sprintf("role%d", r)
}

// Stop is a Tab stop.
type Stop struct {
	Node Node
	// Seen is false when the focused element is not on the tree assistive
	// technology reads: a control nested inside a button or a link.
	Seen bool
}

// TabWalk presses Tab from where the focus is, up to limit times, and returns
// the stops in order until the focus comes back to a stop it has seen. It
// fails the test when a stop is not on the tree (assistive technology
// cannot reach it: a control nested inside a button or a link), and when
// the walk never comes back (a trap, or more stops than limit).
func TabWalk(t testing.TB, tt *ui.Tester, limit int) []Stop {
	t.Helper()
	tt.SetFocused(true)
	var stops []Stop
	seen := map[uint64]bool{}
	for range limit {
		tt.Key(0, ui.KeyTab)
		tr := Read(t, tt)
		id := focusedID(tt)
		if seen[id] {
			return stops
		}
		seen[id] = true
		st := stop(tr)
		if !st.Seen {
			t.Errorf("Tab stop %d is not on the accessibility tree (inside a button or a link, or the window has no focus): %s; stops so far %q", len(stops)+1, where(tt), labels(stops))
		}
		stops = append(stops, st)
	}
	t.Errorf("Tab never came back to a stop it had seen in %d presses: %q", limit, labels(stops))
	return stops
}

// focusedID is the ID of the element with the keyboard focus, 0 for none.
func focusedID(tt *ui.Tester) uint64 {
	return reflect.ValueOf(tt).Elem().FieldByName("rt").Elem().FieldByName("focused").Uint()
}

func stop(tr Tree) Stop {
	if tr.Focus == nil {
		return Stop{}
	}
	return Stop{Node: *tr.Focus, Seen: true}
}

func labels(stops []Stop) []string {
	out := make([]string, len(stops))
	for i, s := range stops {
		out[i] = s.Node.String()
	}
	return out
}

// Nested is a control that assistive technology cannot reach: it sits
// inside an element MyGo reads as one piece (a button, a link, a text, any
// leaf role), so it is not on the tree although it takes clicks or the
// focus. Every control is a sibling of the opener, never inside it.
type Nested struct {
	// Opener is the leaf the control is inside, Inner the control.
	Opener, Inner string
}

func (n Nested) String() string { return fmt.Sprintf("%s inside %s", n.Inner, n.Opener) }

// Flags of MyGo's elements (ui/element.go), which Nested reads off the
// engine's element tree: TestReadSeesTheTree breaks first when a bump
// moves them.
const (
	flagClickable uint32 = 1 << 0
	flagFocusable uint32 = 1 << 1
	flagEditable  uint32 = 1 << 2
	flagScrollX   uint32 = 1 << 4
	flagScrollY   uint32 = 1 << 5
	flagInvisible uint32 = 1 << 18
	flagInert     uint32 = 1 << 27
)

const (
	kindBox   = 0
	kindText  = 1
	kindImage = 2
	kindIcon  = 4
)

// NestedControls reads the elements of the last frame and returns the
// controls that are inside a leaf of the accessibility tree.
func NestedControls(t testing.TB, tt *ui.Tester) []Nested {
	t.Helper()
	tt.Frame()
	rt := reflect.ValueOf(tt).Elem().FieldByName("rt")
	if !rt.IsValid() {
		t.Fatal("ui.Tester has no field rt any more")
	}
	root := rt.Elem().FieldByName("c").FieldByName("root")
	if !root.IsValid() || root.IsNil() {
		t.Fatal("the engine has no root element any more")
	}
	var out []Nested
	visited = 0
	walkElements(root, &out)
	t.Logf("nested-control check read %d elements", visited)
	return out
}

type element struct {
	flags       uint32
	role        ui.Role
	kind        uint8
	label, text string
	hidden      bool
}

func readElement(v reflect.Value) element {
	return element{
		flags:  uint32(v.FieldByName("flags").Uint()),
		role:   ui.Role(v.FieldByName("role").Uint()),
		kind:   uint8(v.FieldByName("kind").Uint()),
		label:  v.FieldByName("label").String(),
		text:   v.FieldByName("text").String(),
		hidden: v.FieldByName("hidden").Bool() || v.FieldByName("accessHidden").Bool(),
	}
}

// leafOf says whether MyGo reads the element as one piece, and as which
// role (mirroring ui/access.go: accessRole and leafRole).
func (e element) leafOf() (ui.Role, bool) {
	role := e.role
	switch role {
	case ui.RoleNone:
		return 0, false
	case ui.RoleAuto:
		switch {
		case e.flags&flagEditable != 0:
			role = ui.RoleTextField
		case e.kind == kindText:
			if e.text == "" {
				return 0, false
			}
			return ui.RoleText, true
		case e.kind == kindImage || e.kind == kindIcon:
			if e.label == "" {
				return 0, false
			}
			return ui.RoleImage, true
		case e.flags&flagClickable != 0 && e.flags&flagFocusable != 0:
			role = ui.RoleButton
		case e.flags&(flagScrollX|flagScrollY) != 0:
			return ui.RoleScroll, false
		default:
			return ui.RoleGroup, false
		}
	}
	switch role {
	case ui.RoleGroup, ui.RoleList, ui.RoleScroll, ui.RoleDialog, ui.RolePopup, ui.RoleStatus, ui.RoleTabList, ui.RoleTable,
		ui.RoleRow, ui.RoleCell, ui.RoleColumnHeader, ui.RoleRowHeader, ui.RoleTree, ui.RoleListItem, ui.RoleToolbar,
		ui.RoleRadioGroup, ui.RoleMenu, ui.RoleMenuBar, ui.RoleAlertDialog, ui.RoleHeading, ui.RoleNavigation,
		ui.RoleRegion, ui.RoleLog, ui.RoleTimer, ui.RoleAlert, ui.RoleSearch, ui.RoleArticle, ui.RoleGrid,
		ui.RoleTreeGrid, ui.RoleListBox, ui.RoleOption, ui.RoleDeletion, ui.RoleInsertion:
		return role, false
	}
	return role, true
}

func children(v reflect.Value) []reflect.Value {
	var out []reflect.Value
	for ch := v.FieldByName("first"); !ch.IsNil(); ch = ch.Elem().FieldByName("next") {
		out = append(out, ch.Elem())
	}
	return out
}

// visited counts the elements the last check read.
var visited int

func walkElements(ptr reflect.Value, out *[]Nested) {
	visited++
	v := ptr.Elem()
	e := readElement(v)
	if e.flags&(flagInvisible|flagInert) != 0 || e.hidden {
		return
	}
	if _, leaf := e.leafOf(); leaf {
		if e.kind == kindText {
			return // MyGo puts a text's inline links on the tree (accessInline)
		}
		opener := fmt.Sprintf("%s %q", roleName(leafRole(e)), nameOf(v))
		for _, ch := range children(v) {
			findControls(ch, opener, out)
		}
		return
	}
	for _, ch := range children(v) {
		walkElements(ch.Addr(), out)
	}
}

func leafRole(e element) ui.Role {
	r, _ := e.leafOf()
	return r
}

// findControls collects the controls at or below v, inside the leaf opener.
func findControls(v reflect.Value, opener string, out *[]Nested) {
	e := readElement(v)
	if e.flags&(flagInvisible|flagInert) != 0 || e.hidden {
		return
	}
	if e.flags&(flagClickable|flagFocusable) != 0 {
		*out = append(*out, Nested{Opener: opener, Inner: nameOf(v)})
	}
	for _, ch := range children(v) {
		findControls(ch, opener, out)
	}
}

// nameOf is an element's label, or the first text inside it.
func nameOf(v reflect.Value) string {
	e := readElement(v)
	if e.label != "" {
		return e.label
	}
	if e.kind == kindText {
		return e.text
	}
	for _, ch := range children(v) {
		if s := nameOf(ch); s != "" {
			return s
		}
	}
	return ""
}

// NoNestedControls fails the test for every control inside a leaf.
func NoNestedControls(t testing.TB, tt *ui.Tester) {
	t.Helper()
	for _, n := range NestedControls(t, tt) {
		t.Errorf("not on the accessibility tree: %s", n)
	}
}

// clickRoles are the controls Click looks for by name when the Tester's own
// click refuses the element it found first.
var clickRoles = []ui.Role{
	ui.RoleLink, ui.RoleButton, ui.RoleOption, ui.RoleMenuItem, ui.RoleMenuItemRadio, ui.RoleMenuItemCheckBox,
	ui.RoleTab, ui.RoleCheckBox, ui.RoleRadio, ui.RoleSwitch, ui.RoleToggleButton, ui.RolePopUpButton, ui.RoleMenuButton,
}

// Click clicks the control named name, as tt.Click does. MyGo v0.8.1's
// Tester.Click takes the first element showing name, and refuses it as
// not the input target when that element is not itself the control but
// holds it, under an ancestor that handles input (a scrolling page): an
// inline link in a text (the text is found before its link), or a button
// in a list item labelled as it is. Click then clicks the middle of the
// control of that name assistive technology sees, as a user would. The
// refusals it does not retry (hidden, outside the window, under a scroll
// bar or a web view) stand; a control another element covers gets the
// click on that element, and the test fails on what it expected.
func Click(tt *ui.Tester, name string) error {
	err := tt.Click(name)
	if err == nil || !strings.Contains(err.Error(), "is not the input target") {
		return err
	}
	for _, r := range clickRoles {
		if n, ok := tt.AccessByRole(r, name); ok && n.Bounds.W > 0 && n.Bounds.H > 0 {
			tt.ClickAt(n.Bounds.X+n.Bounds.W/2, n.Bounds.Y+n.Bounds.H/2)
			return nil
		}
	}
	return err
}

// where describes the element with the keyboard focus by the elements it is
// inside, for the failure message of a stop that is not on the tree.
func where(tt *ui.Tester) string {
	rt := reflect.ValueOf(tt).Elem().FieldByName("rt").Elem()
	focused := rt.FieldByName("focused").Uint()
	root := rt.FieldByName("c").FieldByName("root")
	if focused == 0 || !root.IsValid() || root.IsNil() {
		return "no focused element"
	}
	var path []string
	var find func(v reflect.Value) bool
	find = func(v reflect.Value) bool {
		e := readElement(v)
		name := fmt.Sprintf("%s %q", roleName(e.role), nameOf(v))
		path = append(path, name)
		if v.FieldByName("id").Uint() == focused {
			return true
		}
		for _, ch := range children(v) {
			if find(ch) {
				return true
			}
		}
		path = path[:len(path)-1]
		return false
	}
	if !find(root.Elem()) {
		return "a focused element that is not built"
	}
	return strings.Join(path[max(0, len(path)-4):], " > ")
}
