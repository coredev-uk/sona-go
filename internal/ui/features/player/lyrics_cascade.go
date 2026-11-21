package player

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// Lyrics move line by line, in a cascade. When the sung line changes every
// line on screen glides to its new place on its own: the lines the sung one
// leads (the ones behind it, after the way it goes) start together with it,
// and the ones that trail it start a little later each, so the column
// unfolds as a wave rather than moving as one block. The same rule serves
// Full-Screen Lyrics (stagePose) and the Now Playing sheet (follow).
const (
	// cascadeStep is how much later each place behind the leading line
	// starts to move.
	cascadeStep = 60 * time.Millisecond
	// cascadeSteps is how many places the wait grows for: lines further
	// behind than that start with the last of them, so that the far lines of
	// a tall sheet do not lag.
	cascadeSteps = 4
)

// cascadeDelay is how long a line waits before it glides, by its rank (the
// places from the sung line, negative above it) and the way the lyrics go:
// up when the sung line advances, down on a seek back. The line the sung one
// leads goes first and the ones trailing it follow, so the gap between a
// line and the next only ever opens: lines never run into each other. None
// under reduced motion, which jumps.
func cascadeDelay(rank int, up, reduced bool) time.Duration {
	behind := rank // below the sung line trails when the lyrics go up
	if !up {
		behind = -rank // above it trails when they go down
	}
	if reduced || behind <= 0 {
		return 0
	}
	return time.Duration(min(behind, cascadeSteps)) * cascadeStep
}

// cascadeAnim is how a line goes to its place: the climb token (600 ms,
// exponential out, which settles without a bounce) after its cascadeDelay.
func cascadeAnim(rank int, up, reduced bool) ui.Animation {
	m := theme.Current().Motion["climb"]
	return ui.Animation{Duration: m.Duration, Ease: m.Ease, Delay: cascadeDelay(rank, up, reduced)}
}

// cascadeMargin is the time past the last line's glide that cascadeSettle
// allows for the frame that finishes it.
const cascadeMargin = 50 * time.Millisecond

// cascadeSettle is how long after a change of line the last line, the
// farthest behind, has come to rest.
func cascadeSettle() time.Duration {
	return cascadeSteps*cascadeStep + theme.Current().Motion["climb"].Duration + cascadeMargin
}
