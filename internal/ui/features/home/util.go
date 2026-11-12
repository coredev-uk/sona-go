package home

import (
	"github.com/egoist/mygo/ui"
)

// widthTracker follows the width an element laid out at, for what depends on
// it (v0.2.0's useElementWidth): the width of the last frame, 0 before the
// first.
type widthTracker struct{ w float32 }

// track reads root's width and asks for another frame when it changes. Call
// it on the element before its children are built.
func (m *widthTracker) track(c *ui.Context, root ui.Element) float32 {
	root.Draw(func(_ *ui.Painter, r ui.Rect) {
		if r.W != m.w {
			m.w = r.W
			c.Invalidate() // the columns follow the width
		}
	})
	if w := root.Bounds().W; w > 0 && m.w == 0 {
		m.w = w
	}
	return m.w
}

// hexColour parses an item's "#rrggbb" colour; false for anything else.
func hexColour(s string) (ui.Color, bool) {
	if len(s) != 7 || s[0] != '#' {
		return ui.Color{}, false
	}
	for i := 1; i < 7; i++ {
		if ch := s[i]; (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') && (ch < 'A' || ch > 'F') {
			return ui.Color{}, false
		}
	}
	return ui.Hex(s), true
}
