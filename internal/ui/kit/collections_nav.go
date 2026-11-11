package kit

import (
	"github.com/egoist/mygo/ui"
)

// GridNav is the keyboard navigation of a list or a grid of focusable items
// (v0.2.0's arrow handling in CoverGrid and TrackList, with useItemList's
// arrowTo): arrows move the focus to the neighbouring item (Left and Right ±1,
// Up and Down ± Columns), Page Up and Page Down a window, Home and End the
// ends; Shift extends the selection to the item reached, a plain arrow moves
// the selection's anchor. An item that is not built (a virtual row far away)
// is scrolled into view with ScrollTo and the focus follows when it is built.
//
// A list keeps one as a field, sets Count, Columns (1 for a list) and
// ScrollTo each frame, calls Tick once per frame, and each item calls Claim
// while it builds and Keys on its focus target.
type GridNav struct {
	// Count is how many items there are; Columns how many per row (1 for
	// a list).
	Count, Columns int
	// Page is how far Page Up and Page Down move (0: the keys are not
	// handled); Ends enables Home and End.
	Page int
	Ends bool
	// ScrollTo brings item i's row into view so it gets built; nil when
	// every row is built.
	ScrollTo func(i int)

	focus int // the item that last had the focus
	want  int // the item asked to take the focus + 1; 0 none
	tries int
}

// navRetries is how many frames a requested focus waits for its item to be
// built (v0.2.0 retried up to 10 animation frames).
const navRetries = 10

// Focus is the item that had the focus last: a list's roving tab stop.
func (n *GridNav) Focus() int { return n.focus }

// SetFocus records the item that has the focus (an item took it by click or
// Tab).
func (n *GridNav) SetFocus(i int) { n.focus = i }

// Want asks for the focus to move to item i: scrolled into view if it is
// not built, and taken by the item when it builds.
func (n *GridNav) Want(i int) {
	if i < 0 || i >= n.Count {
		return
	}
	n.want, n.tries = i+1, 0
	n.focus = i
	if n.ScrollTo != nil {
		n.ScrollTo(i)
	}
}

// Wanted is the item asked to take the focus, if one waits.
func (n *GridNav) Wanted() (int, bool) { return n.want - 1, n.want > 0 }

// Tick is called once a frame, after the items built: a request that no item
// took in navRetries frames is dropped, and one still waiting keeps its item
// scrolled to.
func (n *GridNav) Tick(c *ui.Context) {
	if n.want == 0 {
		return
	}
	n.tries++
	if n.tries > navRetries {
		n.want = 0
		return
	}
	if n.ScrollTo != nil {
		n.ScrollTo(n.want - 1)
	}
	c.Invalidate()
}

// Claim is called as item `at` builds its focus target s: it takes the focus
// when a move asked for it, and notes the item when the focus lands on it.
func (n *GridNav) Claim(s ui.Element, at int) {
	if n.want == at+1 {
		// Focus scrolls the item into view only when the focus moves to
		// it. A move can target the item that already holds the focus (an
		// item reordered or moved to this place keeps it), which Focus
		// leaves where it is, and a pointer button held scrolls nothing:
		// so the scroll stays explicit.
		s.Focus().ScrollIntoView()
		n.want = 0
	}
	if s.Focused() {
		n.focus = at
	}
}

// step is how a key moves the focus, from item at.
func (n *GridNav) step(s ui.Element, shift bool) (to int, ok bool) {
	cols := max(1, n.Columns)
	mods := ui.Modifiers(0)
	if shift {
		mods = ui.Shift
	}
	switch {
	case cols > 1 && s.Shortcut(mods, ui.KeyLeft):
		return -1, true
	case cols > 1 && s.Shortcut(mods, ui.KeyRight):
		return 1, true
	case s.Shortcut(mods, ui.KeyUp):
		return -cols, true
	case s.Shortcut(mods, ui.KeyDown):
		return cols, true
	case n.Page > 0 && s.Shortcut(mods, ui.KeyPageUp):
		return -n.Page, true
	case n.Page > 0 && s.Shortcut(mods, ui.KeyPageDown):
		return n.Page, true
	case n.Ends && s.Shortcut(mods, ui.KeyHome):
		return -n.Count, true
	case n.Ends && s.Shortcut(mods, ui.KeyEnd):
		return n.Count, true
	}
	return 0, false
}

// Keys handles the arrow keys on the focused item `at` (call it with the
// item's focus target s, which does nothing unless s itself has the focus,
// as v0.2.0 ignored keys from buttons, links and inputs inside a row). It
// moves the focus, applies the selection rule (sel may be nil) and
// reports whether it took a key.
func (n *GridNav) Keys(s ui.Element, at int, sel *Selection) bool {
	if !s.Focused() || n.Count == 0 {
		return false
	}
	for _, shift := range [...]bool{false, true} {
		d, ok := n.step(s, shift)
		if !ok {
			continue
		}
		to := min(max(at+d, 0), n.Count-1)
		// A step off the end of a grid does nothing; a page or Home/End
		// goes as far as it can.
		cols := max(1, n.Columns)
		if (d == -1 || d == 1 || d == -cols || d == cols) && at+d != to {
			return true
		}
		if to == at {
			return true
		}
		n.Want(to)
		if sel != nil {
			sel.ArrowTo(to, shift)
		}
		return true
	}
	return false
}
