package kit

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

const (
	eqBarW   = 4  // DIPs
	eqGap    = 3  // DIPs: bars and gaps are whole DIPs and the group is 18 wide, even, so centred in an even cover every edge is on a device pixel at 1x and 2x
	eqHeight = 12 // DIPs
	// eqLowest is the shortest a moving bar gets, as a scale of eqHeight;
	// the tallest is eqHeight itself.
	eqLowest = 0.25
	// eqFloor is a paused bar, as a scale of eqHeight: dropped to a 2 DIP
	// stub, as short as the corners allow, so the row keeps its mark.
	eqFloor  = 2.0 / eqHeight
	eqRadius = 1 // DIPs: the bars' corners
	// eqSettle is how long the bars take to drop as the track pauses, or to
	// rise into their movement as it plays again.
	eqSettle = 300 * time.Millisecond
)

// eqMode is what the bars are doing: moving, standing at rest or dropped.
type eqMode uint8

const (
	eqLive eqMode = iota // playing, moving
	eqRest               // playing, still: covered or motion reduced
	eqDown               // paused
)

// eqState is what the bars keep from paint to paint: the mode they last
// showed, when it began and the heights it began from, so that a change of
// mode eases from where the bars stood rather than jumping.
type eqState struct {
	mode  eqMode
	since time.Time // zero: the mode the bars were built in, settled
	from  [len(eqBars)]float32
	last  [len(eqBars)]float32
}

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

// eqEase is an ease-out: quick to leave, slow to land, as a bar that drops
// and settles.
func eqEase(u float32) float32 {
	v := 1 - u
	return 1 - v*v*v
}

// EqualiserBars is the now-playing mark: three amber bars (60, 100 and 40% of
// 12 DIPs at rest, 4 wide, 3 apart), moving while the track plays. Each bar
// rises and falls between a quarter of the height and all of it on a period
// of its own, quick through the middle and slow at the turns. Paused, the
// bars drop to 2 DIP stubs over eqSettle from wherever they stood, and rise
// back into their movement as the track plays again. They hold still at rest
// height while playing under Now Playing or full-screen lyrics (an unseen
// loop still costs every frame) or with motion reduced, where a pause drops
// them at once. The bars are painted at up to 60 a second (AmbientFrames) by
// painting this element alone again: the view is not built nor laid out for
// it, so a long list around it costs nothing per frame, and an element that
// is off screen is not painted, so it asks for no frame. Settled and paused,
// they ask for none. It replaces a row's index wherever the playing track
// appears. It is an image named "Now playing" or "Paused".
// p says whether the page is covered (nil: never).
func EqualiserBars(c *ui.Context, playing bool, p *MotionPolicy) ui.Element {
	still := p.Covered() || theme.ReducedMotion()
	mode := eqDown
	label := i18n.T("Paused")
	if playing {
		label = i18n.T("Now playing")
		mode = eqLive
		if still {
			mode = eqRest
		}
	}
	amber := theme.C("amber")
	width := float32(len(eqBars))*eqBarW + float32(len(eqBars)-1)*eqGap
	box := ui.Box(c).Size(width, eqHeight).Role(ui.RoleImage).Label(label)
	st := ui.Local(box, "eq", func() eqState { return eqState{mode: mode} })
	box.Draw(func(pt *ui.Painter, r ui.Rect) {
		at := pt.Now()
		if st.mode != mode {
			st.mode, st.since, st.from = mode, at, st.last
		}
		k := float32(1) // how far the bars are from where the mode began to where it puts them
		if d := at.Sub(st.since); !still && !st.since.IsZero() && d < eqSettle {
			k = eqEase(float32(d) / float32(eqSettle))
		}
		now := time.Duration(at.UnixNano()) // any steady origin will do: only the cycles matter
		bottom := r.Y + r.H
		for i, b := range eqBars {
			to := float32(eqFloor)
			switch mode {
			case eqLive:
				to = eqLevel(b.period, now)
			case eqRest:
				to = b.rest
			}
			level := st.from[i] + (to-st.from[i])*k
			st.last[i] = level
			x := r.X + float32(i)*(eqBarW+eqGap)
			if k == 1 && mode != eqLive {
				h := eqHeight * level
				pt.Fill(ui.Rect{X: x, Y: bottom - h, W: eqBarW, H: h}, amber, eqRadius)
				continue
			}
			// A bar is drawn whole and squeezed from its foot: Fill rounds a
			// rectangle's edges to whole device pixels, so a bar sized each
			// frame would move its top a pixel at a time, a few times a
			// second at 1x; a transform is applied to the pixels, and the
			// top edge's partial coverage shows as it is.
			pt.Transform(ui.Scaling(1, level), x, bottom, func() {
				pt.Fill(ui.Rect{X: x, Y: bottom - eqHeight, W: eqBarW, H: eqHeight}, amber, eqRadius)
			})
		}
		if mode == eqLive || k < 1 {
			AmbientFrames(pt)
		}
	})
	return box
}
