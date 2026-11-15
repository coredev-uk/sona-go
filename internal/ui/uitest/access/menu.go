package access

import (
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// Menu is the item menu open in a window, as assistive technology reads it:
// its name ("More for Airbag") and its entries, in order.
type Menu struct {
	Label   string
	Entries []string
}

// OpenMenu returns the item menu open in the tree: the menu whose name starts
// with "More for", and the labels of its entries.
func (tr Tree) OpenMenu() (Menu, bool) {
	menus := tr.Labelled(ui.RoleMenu, "More for")
	if len(menus) == 0 {
		return Menu{}, false
	}
	m := Menu{Label: menus[0].Label}
	for _, n := range tr.Nodes {
		switch n.Role {
		case ui.RoleMenuItem, ui.RoleMenuItemCheckBox, ui.RoleMenuItemRadio:
			m.Entries = append(m.Entries, n.Label)
		}
	}
	return m, true
}

// MenuWays are the ways to open one item's menu, for CheckMenuWays: the rule
// is that wherever an item has a "More" button, a right-click on the item and
// the keyboard's menu keys open the same menu.
type MenuWays struct {
	// More is the accessible name of the item's "More for <name>" button.
	More string
	// Point is where a right-click lands on the item: its artwork, its title,
	// its body (window DIPs). The menu must open there.
	Point func() (x, y float32)
	// AlsoPoints are other places on the item (its artwork when Point is the
	// title): a right-click on each opens the menu there too.
	AlsoPoints []func() (x, y float32)
	// Focus puts the keyboard focus on the item or inside it (Shift+F10 and
	// the Menu key answer to it).
	Focus func()
	// Settle draws what a press left to draw (frames, posted work). Without
	// it the window draws three frames.
	Settle func()
}

// pointerReach is how far from the pointer the first entry's text may start:
// the menu's panel padding, an entry's own padding and its icon.
const pointerReach = 56

// CheckMenuWays opens the item's menu with its More button, with a
// right-click at Point, and with Shift+F10 and the Menu key after Focus, and
// reports a way that opens no menu, a different one, or (for the right-click)
// one that is not at the pointer. Each menu is closed with Escape before the
// next.
func CheckMenuWays(t testing.TB, tt *ui.Tester, w MenuWays) {
	t.Helper()
	settle := w.Settle
	if settle == nil {
		settle = func() { tt.Frame(); tt.Frame(); tt.Frame() }
	}
	read := func(how string) Menu {
		t.Helper()
		settle()
		m, ok := Read(t, tt).OpenMenu()
		if !ok {
			t.Fatalf("%s opened no item menu; texts %q", how, tt.Texts())
		}
		if len(m.Entries) == 0 {
			t.Fatalf("%s opened a menu with no entries", how)
		}
		return m
	}
	closeMenu := func(how string) {
		t.Helper()
		tt.Key(0, ui.KeyEscape)
		tt.Advance(300 * time.Millisecond)
		settle()
		if _, ok := Read(t, tt).OpenMenu(); ok {
			t.Fatalf("Escape did not close the menu %s opened", how)
		}
	}

	if err := tt.Click(w.More); err != nil {
		t.Fatalf("the More button %q: %v", w.More, err)
	}
	want := read("the More button")
	closeMenu("the More button")

	for _, at := range append([]func() (float32, float32){w.Point}, w.AlsoPoints...) {
		x, y := at()
		tt.RightClickAt(x, y)
		got := read("a right-click")
		if got.Label != want.Label || !slices.Equal(got.Entries, want.Entries) {
			t.Errorf("a right-click at (%.0f, %.0f) opened %q %q, the More button %q %q", x, y, got.Label, got.Entries, want.Label, want.Entries)
		}
		if r, ok := tt.Find(got.Entries[0]); !ok {
			t.Errorf("the right-click menu's first entry %q is not on screen", got.Entries[0])
		} else if r.X < x-1 || r.X > x+pointerReach || r.Y < y-1 || r.Y > y+pointerReach {
			t.Errorf("a right-click at (%.0f, %.0f) opened the menu with its first entry at (%.0f, %.0f)", x, y, r.X, r.Y)
		}
		closeMenu("a right-click")
	}

	for _, k := range []struct {
		name string
		mods ui.Modifiers
		key  ui.Key
	}{{"Shift+F10", ui.Shift, ui.KeyF10}, {"the Menu key", 0, ui.KeyContextMenu}} {
		if w.Focus != nil {
			w.Focus()
		}
		tt.Key(k.mods, k.key)
		got := read(k.name)
		if got.Label != want.Label || !slices.Equal(got.Entries, want.Entries) {
			t.Errorf("%s opened %q %q, the More button %q %q", k.name, got.Label, got.Entries, want.Label, want.Entries)
		}
		closeMenu(k.name)
	}
}

// FocusByTab presses Tab until the element named label has the focus.
func FocusByTab(t testing.TB, tt *ui.Tester, label string) {
	t.Helper()
	for range 60 {
		if tt.Focused(label) {
			return
		}
		tt.Key(0, ui.KeyTab)
	}
	t.Fatalf("Tab never reached %q; texts %q", label, tt.Texts())
}
