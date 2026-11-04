package concerts

import (
	"slices"

	"github.com/egoist/mygo/ui"
)

// Roving focus for a long agenda (v0.2.0's roving.ts): every row holds one
// or two controls (the artist's link, then the tickets), and the whole list
// is one Tab stop. Up and Down move to the same control of the row above
// or below, Left and Right between a row's controls, Home and End to the
// first and last row (a ticket menu's button keeps Down for moving: its
// menu opens with Enter or Space). Tab leaves the list; coming back lands
// on the control focused last. MyGo's FocusGroup makes the controls one
// stop of Tab and remembers the one that had the focus; the arrows are the
// list's own (columns, clamped at the ends), claimed on the list's element
// so the group's round-the-list arrows never run.

// position is a control: its row of the agenda and its place in the row.
type position struct{ row, col int }

// rovingKey is a key that moves along the agenda.
type rovingKey int

const (
	keyOther rovingKey = iota
	keyDown
	keyUp
	keyRight
	keyLeft
	keyHome
	keyEnd
)

// moveFrom is where key moves from at among rows of counts[i] controls; false
// for other keys.
func moveFrom(key rovingKey, at position, counts []int) (position, bool) {
	last := len(counts) - 1
	clamp := func(row, col int) position {
		return position{row, min(col, max(counts[row]-1, 0))}
	}
	switch key {
	case keyDown:
		if at.row < last {
			return clamp(at.row+1, at.col), true
		}
		return at, true
	case keyUp:
		if at.row > 0 {
			return clamp(at.row-1, at.col), true
		}
		return at, true
	case keyRight:
		return position{at.row, min(at.col+1, max(counts[at.row]-1, 0))}, true
	case keyLeft:
		return position{at.row, max(at.col-1, 0)}, true
	case keyHome:
		return clamp(0, at.col), true
	case keyEnd:
		return clamp(last, at.col), true
	}
	return at, false
}

// roving is the agenda's focus state: the control that had the focus last,
// the one a key asked for, and how many controls each row of the last
// frame had.
type roving struct {
	cur     position
	want    position
	wanting bool
	counts  []int // rows of the last frame
	next    []int // rows of the frame being built
}

// begin starts a frame's list.
func (r *roving) begin() { r.next = r.next[:0] }

// row adds a row to the list and returns its index.
func (r *roving) row() int {
	r.next = append(r.next, 0)
	return len(r.next) - 1
}

// stop is called as a row builds a control s: it takes the focus when a
// key asked for it, and notes it when the focus lands on it.
func (r *roving) stop(s ui.Element, row int) {
	col := r.next[row]
	r.next[row]++
	at := position{row, col}
	if r.wanting && r.want == at {
		s.Focus().ScrollIntoView()
		r.wanting = false
	}
	if s.Focused() {
		r.cur = at
	}
}

// end finishes the frame's list; keys then reads the arrows on the list's
// element list, which holds the focus's chain.
func (r *roving) end(c *ui.Context, list ui.Element) {
	r.counts = slices.Clone(r.next)
	if len(r.counts) == 0 {
		return
	}
	r.cur.row = min(r.cur.row, len(r.counts)-1)
	r.cur.col = min(r.cur.col, max(r.counts[r.cur.row]-1, 0))
	for _, k := range []struct {
		key  ui.Key
		move rovingKey
	}{
		{ui.KeyDown, keyDown},
		{ui.KeyUp, keyUp},
		{ui.KeyRight, keyRight},
		{ui.KeyLeft, keyLeft},
		{ui.KeyHome, keyHome},
		{ui.KeyEnd, keyEnd},
	} {
		if !list.Shortcut(0, k.key) {
			continue
		}
		if to, ok := moveFrom(k.move, r.cur, r.counts); ok {
			r.want, r.wanting = to, true
			c.Invalidate()
		}
	}
}
