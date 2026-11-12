package kit

import "github.com/egoist/mygo/ui"

// virtualRows is what a list that builds only the rows showing (TrackList,
// CoverGrid, in a pane) keeps of where it sits in the pane: top is the list's
// offset in the pane's content; drawTop is that offset as the last paint saw
// it (one frame's position and scroll offset together, never of two), drawn
// says there was a paint.
type virtualRows struct {
	top, drawTop float32
	drawn        bool
}

// paint records the offset a paint saw, from the list's Draw callback, and
// reports whether it changed (the caller then invalidates: the rows built
// followed the old one).
func (v *virtualRows) paint(top float32) bool {
	if v.drawn && top == v.drawTop {
		return false
	}
	v.drawTop, v.drawn = top, true
	return true
}

// origin sets top: the offset the last paint saw, and before the first paint
// the one worked out from the list's box (extra: what lies above the rows
// inside it, a header).
func (v *virtualRows) origin(root ui.Rect, p *Pane, extra float32) {
	if v.drawn {
		v.top = v.drawTop
		return
	}
	v.top = root.Y - p.Bounds().Y + p.Pos.Y + extra
}

// window is the rows to build, from through to: those in view and overscan
// beyond them. rowH is a row's height, and noLayout the height the pane is
// taken to have before its first layout. from is past to when the pane is
// scrolled beyond the rows.
func (v *virtualRows) window(p *Pane, count int, rowH float32, overscan int, noLayout float32) (from, to int) {
	y := p.Pos.Y - v.top
	h := p.Bounds().H
	if h <= 0 {
		h = noLayout
	}
	first := max(0, int(y/rowH))
	last := min(count-1, int((y+h)/rowH))
	return max(0, first-overscan), min(count-1, last+overscan)
}

// scrollIntoView scrolls the pane as little as shows the span top…bottom of
// its content, margin clear of its edges, so the rows there get built.
func (v *virtualRows) scrollIntoView(p *Pane, top, bottom, margin float32) {
	pane := p.Bounds()
	switch {
	case top-margin < p.Pos.Y:
		p.Pos.Y = max(0, top-margin)
	case pane.H > 0 && bottom+margin > p.Pos.Y+pane.H:
		p.Pos.Y = bottom + margin - pane.H
	}
}

// rowSpan is the rows a virtual list builds, in order: from through to, and
// pin, a row outside them that a drag in progress keeps built (-1: none).
type rowSpan struct{ from, to, pin int }

// spanOf is the span of rows from…to of count, with the dragged row pinned
// (pinned, ok: DragPinned) when it falls outside them.
func spanOf(from, to, count, pinned int, ok bool) rowSpan {
	s := rowSpan{from, to, -1}
	if ok && pinned < count && (pinned < from || pinned > to) {
		s.pin = pinned
	}
	return s
}

// empty reports that no row is built.
func (s rowSpan) empty() bool { return s.from > s.to && s.pin < 0 }

// each calls fn with the span's rows in ascending order.
func (s rowSpan) each(fn func(i int)) {
	before := s.pin >= 0 && s.pin < s.from
	if before {
		fn(s.pin)
	}
	for i := s.from; i <= s.to; i++ {
		fn(i)
	}
	if s.pin >= 0 && !before {
		fn(s.pin)
	}
}
