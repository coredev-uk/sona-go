package radio

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// Page geometry, from the reference page (AppShell's main and the pages'
// own spacing), in DIPs.
const (
	padTop       = 28  // pt-7
	padBottom    = 48  // pb-12
	narrowWindow = 960 // the page's gutters shrink below this width
	bandGap      = 56  // mt-14: the line across the column above a band
	bandPad      = 24  // pt-6: from that line to the band's heading
	headingGap   = 20  // mb-5: under a band's heading
	textMax      = 72 * 9
	railMin      = 880 // content width from which a band hangs its heading in a rail
)

// text is a text in a token's type style (Type["sub"]…).
func text(c *ui.Context, s, style string) ui.Element {
	t := theme.Current().Type[style]
	return ui.Text(c, s).FontSize(t.Size).FixedLineHeight(t.LineHeight).FontWeight(int(t.Weight)).LetterSpacing(t.Tracking)
}

// frame is a page's scroll container: the offset, and the size of the last
// frame, which the next one starts from.
type frame struct {
	pos           ui.ScrollState
	width, height float32
}

// gutter is the page's side padding at this width.
func (f *frame) gutter() float32 {
	m := theme.Current().Metric
	if f.width > 0 && f.width < narrowWindow {
		return m["gutter-narrow"]
	}
	return m["gutter"]
}

// scroll builds the page: one scroll container with the page's padding (the
// right one leaves room for the scroll bar, as WebKitGTK's took 12 DIPs of
// the page, so the content keeps the size it had), and build inside it with
// the gutter. The container's size is read for the next frame.
func (f *frame) scroll(c *ui.Context, build func(gutter float32)) {
	gutter := f.gutter()
	sc := ui.Scroll(c).TrackScroll(&f.pos).Grow(1).Basis(0).MinHeight(0).MinWidth(0)
	sc.Children(func() {
		ui.Column(c).Padding(padTop, gutter+theme.Current().Metric["scrollbar"], padBottom, gutter).Children(func() { build(gutter) })
	})
	if b := sc.Bounds(); b.W != f.width || b.H != f.height {
		f.width, f.height = b.W, b.H
		c.Invalidate() // the gutters follow the width
	}
}

// widthOf follows the width an element laid out at, for the grids whose
// columns depend on it: the width of the last frame, 0 before the first.
type widthOf struct{ w float32 }

// track reads root's width and asks for another frame when it changes. Call
// it on the element before its children are built.
func (m *widthOf) track(c *ui.Context, root ui.Element) float32 {
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

// refresh is v0.2.0's useNow(30 000): the moment a page reads its schedules
// against, taken again every 30 seconds and never ticking in between. at is
// the frame time it was last taken at.
type refresh struct {
	at  time.Time
	now int64 // Unix ms
}

// refreshEvery is how often schedules are read again.
const refreshEvery = 30 * time.Second

// read is the moment to reason with this frame: wall (the clock the feature
// reads) when the last reading is 30 seconds old, else the last reading. It
// asks for the frame that takes the next one.
func (k *refresh) read(c *ui.Context, wall func() time.Time) int64 {
	frameTime := c.Now()
	if k.at.IsZero() || frameTime.Sub(k.at) >= refreshEvery {
		k.at, k.now = frameTime, wall().UnixMilli()
	}
	c.After(refreshEvery - frameTime.Sub(k.at))
	return k.now
}
