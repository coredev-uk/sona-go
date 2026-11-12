package kit

import (
	"github.com/coredev-uk/sona-go/internal/ui/devstate"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/egoist/mygo/ui"
)

// The page geometry, from the reference page (AppShell's main) in DIPs.
const (
	PagePadTop    = 28  // pt-7
	PagePadBottom = 48  // pb-12
	PageNarrow    = 960 // the gutters shrink below this width
)

// PageGutter is the page's side padding at a width: 32, 24 under PageNarrow
// (0 reads as wide: nothing was laid out yet).
func PageGutter(width float32) float32 {
	m := theme.Current().Metric
	if width > 0 && width < PageNarrow {
		return m["gutter-narrow"]
	}
	return m["gutter"]
}

// PageScroll builds a page's frame: one scroll container tracking pos (dev
// builds keep the offset across restarts) holding a column inside the
// shell's gutters; the right one leaves room for the scroll bar, as
// WebKitGTK's took 12 DIPs of the page. body builds the page into it with the
// Pane virtual lists are built in. It returns the container's bounds as laid
// out in the last frame.
func PageScroll(c *ui.Context, pos *ui.ScrollState, gutter float32, body func(pane *Pane)) ui.Rect {
	devstate.Scroll(c, pos)
	sc := ui.Scroll(c).TrackScroll(pos).Grow(1).Basis(0).MinHeight(0).MinWidth(0)
	sc.Children(func() {
		scrollbar := theme.Current().Metric["scrollbar"]
		ui.Column(c).Padding(PagePadTop, gutter+scrollbar, PagePadBottom, gutter).Children(func() {
			body(&Pane{Pos: pos, Bounds: sc.Bounds})
		})
	})
	return sc.Bounds()
}

// PageFrame is the view state of a scroll page: the scroll place and the size
// of its scroll container as laid out in the last frame, which the next
// frame's gutter and columns follow. Embed it in a page's own view state.
type PageFrame struct {
	Pos ui.ScrollState
	// Width, Height are the scroll container's size and Top its place in the
	// window, zero until a frame laid it out.
	Width, Height, Top float32
	// Shown is the identity of what the page lists: another one starts from
	// its top.
	Shown string
}

// Gutter is the page's side padding at the width of the last frame.
func (f *PageFrame) Gutter() float32 { return PageGutter(f.Width) }

// Inner is the width of the page's content at the last frame's width.
func (f *PageFrame) Inner() float32 {
	return f.Width - 2*f.Gutter() - theme.Current().Metric["scrollbar"]
}

// Scroll builds the page: PageScroll over Pos, starting from the top when
// identity (what the page lists) is another than the last frame's, and asking
// for another frame when the container's size changes. body is given the Pane
// and the gutter.
func (f *PageFrame) Scroll(c *ui.Context, identity string, body func(pane *Pane, gutter float32)) {
	if f.Shown != identity {
		if f.Shown != "" {
			f.Pos = ui.ScrollState{}
		}
		f.Shown = identity
	}
	gutter := f.Gutter()
	b := PageScroll(c, &f.Pos, gutter, func(pane *Pane) { body(pane, gutter) })
	if b.W != f.Width || b.H != f.Height || b.Y != f.Top {
		f.Width, f.Height, f.Top = b.W, b.H, b.Y
		c.Invalidate() // the gutters and columns follow the width
	}
}
