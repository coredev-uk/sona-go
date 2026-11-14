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
	eqLowest = 0.35
	// eqPeriod is a bar's cycle, up and back (index.css: 900 ms each way,
	// animation-direction: alternate).
	eqPeriod = 1800 * time.Millisecond
	// eqFPS is how often the bars are painted again. They move about eight
	// DIPs in 900 ms, far less than a pixel a frame at 30 a second: sixty
	// would look no smoother.
	eqFPS = 30
)

// eqBars are the bars' height at rest and the offset of their animations
// into the cycle (index.css: 60%, 100%, 40%; delays 0, -300 ms, -600 ms).
var eqBars = [3]struct {
	height float32
	phase  time.Duration
}{{0.6, 0}, {1, 300 * time.Millisecond}, {0.4, 600 * time.Millisecond}}

// eqLevel is how tall a bar is at time t into its animation, as a scale of
// its height: .35 to 1 and back over eqPeriod, eased at both ends so the bar
// slows where it turns, as a level meter's does.
func eqLevel(t time.Duration) float32 {
	t %= eqPeriod
	if t < 0 {
		t += eqPeriod
	}
	turn := 0.5 - 0.5*math.Cos(2*math.Pi*float64(t)/float64(eqPeriod)) // 0, up to 1 at the half, back to 0
	return eqLowest + (1-eqLowest)*float32(turn)
}

// EqualiserBars is the now-playing mark: three amber bars (60, 100 and 40% of
// 12 DIPs, 3 wide), moving while the track plays and still when paused, when
// the page is covered by Now Playing or full-screen lyrics (an unseen loop
// still costs every frame) or when motion is reduced. The bars move smoothly,
// at eqFPS frames a second, by painting this element alone again: the view
// is not built nor laid out for it, so a long list around it costs nothing
// per frame, and an element that is off screen is not painted, so it asks
// for no frame. It replaces a row's index wherever the playing track
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
		now := time.Duration(pt.Now().UnixNano()) // any steady origin will do: only the cycle matters
		for i, b := range eqBars {
			scale := float32(1)
			if live {
				scale = eqLevel(now + b.phase)
			}
			h := eqHeight * b.height * scale
			x := r.X + float32(i)*(eqBarW+eqGap)
			pt.Fill(ui.Rect{X: x, Y: r.Y + r.H - h, W: eqBarW, H: h}, amber, 1)
		}
		if live {
			pt.AnimationFrameRate(eqFPS)
		}
	})
	return box
}
