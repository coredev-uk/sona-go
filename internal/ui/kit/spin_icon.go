package kit

import (
	"time"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/egoist/mygo/ui"
)

// spinPeriod is the time an icon takes for one turn.
const spinPeriod = time.Second

// SpinIcon is the Lucide icon svg, size DIPs square in colour, which turns
// once a second while on, unless motion is reduced: then, and when not on, it
// stands still as a plain icon does. Like an icon it is decoration that
// assistive technology does not see: the control that shows it says what it
// waits for.
//
// It turns by painting: the view is built once and the Draw callback paints
// the icon rotated by the frame's clock, asking for the next paint with
// AnimationFrames, so a spinner costs no build or layout per frame (an
// element's Loop does, as it asks for a built frame).
func SpinIcon(c *ui.Context, svg *ui.SVG, size float32, color ui.Color, on bool) ui.Element {
	if !on || theme.ReducedMotion() {
		return ui.Icon(c, svg).FontSize(size).TextColor(color).Shrink(0)
	}
	return ui.Box(c).Size(size, size).Shrink(0).Draw(func(pt *ui.Painter, r ui.Rect) {
		turn := float32(pt.Now().UnixNano()%int64(spinPeriod)) / float32(spinPeriod)
		cx, cy := r.X+r.W/2, r.Y+r.H/2
		pt.Transform(ui.Rotation(turn*360), cx, cy, func() { pt.Icon(svg, r, color) })
		AnimationFrames(pt)
	})
}
