package kit

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

const (
	eqBarW    = 3  // DIPs
	eqGap     = 2  // DIPs
	eqHeight  = 12 // DIPs
	eqHalf    = 900 * time.Millisecond
	eqLevels  = 3
	eqLowest  = 0.35
	eqPeriod  = 2 * eqHalf // up and back (animation-direction: alternate)
	eqPerStep = eqHalf / eqLevels
)

// eqBars are the bars' height at rest and the offset of their animations
// into the cycle (index.css: 60%, 100%, 40%; delays 0, -300 ms, -600 ms).
var eqBars = [3]struct {
	height float32
	phase  time.Duration
}{{0.6, 0}, {1, 300 * time.Millisecond}, {0.4, 600 * time.Millisecond}}

// eqLevel is how tall a bar is at time t into its animation, as a scale of
// its height: scaleY from .35 to 1 in three steps over 900 ms and back
// (steps(3, jump-none), alternate), so it takes the values .35, .675 and 1.
// step is the index of the step, to tell when it changes.
func eqLevel(t time.Duration) (scale float32, step int) {
	t %= eqPeriod
	if t < 0 {
		t += eqPeriod
	}
	var i int
	if t < eqHalf {
		i = int(t / eqPerStep) // 0, 1, 2 going up
	} else {
		i = int((eqPeriod - 1 - t) / eqPerStep) // 2, 1, 0 coming back
	}
	i = min(max(i, 0), eqLevels-1)
	return eqLowest + (1-eqLowest)*float32(i)/(eqLevels-1), i
}

// untilEqStep is how long after t (into the cycle) the next step begins.
func untilEqStep(t time.Duration) time.Duration {
	t %= eqPeriod
	if t < 0 {
		t += eqPeriod
	}
	return eqPerStep - t%eqPerStep
}

// EqualiserBars is the now-playing mark: three amber bars (60, 100 and 40% of
// 12 DIPs, 3 wide), moving while the track plays and still when paused, when
// the page is covered by Now Playing or full-screen lyrics (an unseen loop
// still costs every frame) or when motion is reduced. The bars move in three
// steps, not smoothly (about ten frames a second: a smooth loop drew all
// sixty for as long as a song played). It replaces a row's index wherever
// the playing track appears. It is an image named "Now playing" or "Paused".
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
		wait := time.Duration(0)
		for i, b := range eqBars {
			scale := float32(1)
			if live {
				t := now + b.phase
				scale, _ = eqLevel(t)
				if w := untilEqStep(t); wait == 0 || w < wait {
					wait = w
				}
			}
			h := eqHeight * b.height * scale
			x := r.X + float32(i)*(eqBarW+eqGap)
			pt.Fill(ui.Rect{X: x, Y: r.Y + r.H - h, W: eqBarW, H: h}, amber, 1)
		}
		if live {
			pt.After(wait + time.Millisecond) // just past the step
		}
	})
	return box
}
