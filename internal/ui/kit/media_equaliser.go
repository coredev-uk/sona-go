package kit

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

const (
	eqBarW   = 3  // DIPs
	eqGap    = 2  // DIPs
	eqHeight = 12 // DIPs
	// eqLowest is the shortest a moving bar gets, as a scale of eqHeight;
	// the tallest is eqHeight itself.
	eqLowest = 0.25
	eqRadius = 1 // DIPs: the bars' corners
)

// eqBars are the bars' height at rest, as a scale of eqHeight (index.css:
// 60%, 100%, 40%), and how long each takes to go up and back down while the
// track plays. The periods are not multiples of one another, so the three
// bars drift in and out of step and the pattern takes minutes to come round
// again, though each bar's own movement is smooth and regular: no randomness,
// so nothing flickers.
var eqBars = [3]struct {
	rest   float32
	period time.Duration
}{{0.6, 1100 * time.Millisecond}, {1, 800 * time.Millisecond}, {0.4, 950 * time.Millisecond}}

// eqLevel is how tall a bar is at time t, as a scale of eqHeight: eqLowest to
// 1 and back over period. Each leg is a smootherstep: the bar lingers a
// moment at the top and the bottom and is quick through the middle, where
// the eye reads the movement.
func eqLevel(period, t time.Duration) float32 {
	t %= period
	if t < 0 {
		t += period
	}
	u := float64(t) / float64(period) // 0 to 1 over the cycle
	s := 1 - math.Abs(2*u-1)          // 0, up to 1 at the half, back to 0
	s = s * s * s * (s*(s*6-15) + 10) // smootherstep
	return eqLowest + (1-eqLowest)*float32(s)
}

// EqualiserBars is the now-playing mark: three amber bars (60, 100 and 40% of
// 12 DIPs at rest, 3 wide), moving while the track plays and still when
// paused, when the page is covered by Now Playing or full-screen lyrics (an
// unseen loop still costs every frame) or when motion is reduced. Each bar
// rises and falls between a quarter of the height and all of it on a period
// of its own, quick through the middle and slow at the turns. The bars are
// painted at the display's rate by painting this element alone again: the
// view is not built nor laid out for it, so a long list around it costs
// nothing per frame, and an element that is off screen is not painted, so it
// asks for no frame. It replaces a row's index wherever the playing track
// appears. It is an image named "Now playing" or "Paused".
// p says whether the page is covered (nil: never).
func EqualiserBars(c *ui.Context, playing bool, p *MotionPolicy) ui.Element {
	live := playing && !p.Covered() && !theme.ReducedMotion()
	label := i18n.T("Paused")
	if playing {
		label = i18n.T("Now playing")
	}
	amber := theme.C("amber")
	width := float32(len(eqBars))*eqBarW + float32(len(eqBars)-1)*eqGap
	box := ui.Box(c).Size(width, eqHeight).Role(ui.RoleImage).Label(label)
	box.Draw(func(pt *ui.Painter, r ui.Rect) {
		now := time.Duration(pt.Now().UnixNano()) // any steady origin will do: only the cycles matter
		bottom := r.Y + r.H
		for i, b := range eqBars {
			x := r.X + float32(i)*(eqBarW+eqGap)
			if !live {
				h := eqHeight * b.rest
				pt.Fill(ui.Rect{X: x, Y: bottom - h, W: eqBarW, H: h}, amber, eqRadius)
				continue
			}
			// A bar is drawn whole and squeezed from its foot: Fill rounds a
			// rectangle's edges to whole device pixels, so a bar sized each
			// frame would move its top a pixel at a time, a few times a
			// second at 1x; a transform is applied to the pixels, and the
			// top edge's partial coverage shows as it is.
			pt.Transform(ui.Scaling(1, eqLevel(b.period, now)), x, bottom, func() {
				pt.Fill(ui.Rect{X: x, Y: bottom - eqHeight, W: eqBarW, H: eqHeight}, amber, eqRadius)
			})
		}
		if live {
			pt.AnimationFrame()
		}
	})
	return box
}
