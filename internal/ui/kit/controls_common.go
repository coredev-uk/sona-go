package kit

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// controlsRadius is a radius token of the current theme.
func controlsRadius(name string) float32 { return theme.Current().Radius[name] }

// controlsDisabled disables an element: pointer and keyboard pass it by, and
// MyGo paints it at 50% (the CSS disabled:opacity-50 + pointer-events-none).
func controlsDisabled(e ui.Element, off bool) {
	if off {
		e.Disabled(true)
	}
}

// controlsWatchWidth is e's width in the last frame, and asks for another
// frame when it is not the one the frame before saw (or there was none yet),
// so what is laid out from a measured width settles: the clamp of Notes, the
// slots of an empty page.
func controlsWatchWidth(c *ui.Context, e ui.Element) float32 {
	last := ui.Local(e, "watched", func() float32 { return -1 })
	w := e.Bounds().W
	if w != *last {
		*last = w
		c.After(time.Millisecond)
	}
	return w
}
