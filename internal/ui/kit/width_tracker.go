package kit

import "github.com/egoist/mygo/ui"

// WidthTracker follows the width an element laid out at, for the components
// whose columns depend on it (v0.2.0's useElementWidth): the width of the
// last frame, 0 before the first.
type WidthTracker struct{ w float32 }

// Track reads root's width and asks for another frame when it changes. Call
// it on the element before its children are built.
func (m *WidthTracker) Track(c *ui.Context, root ui.Element) float32 {
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
