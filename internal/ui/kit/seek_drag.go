package kit

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// seekTick is how often the position of a playing song moves on screen.
const seekTick = 250 * time.Millisecond

// SeekDrag is a seek bar while the user drags it: the position shown is the
// dragged one, and the seek is made once, when the pointer lets go (or a key
// moved the slider). The bar, the mini player and the video view each hold
// one.
type SeekDrag struct {
	active bool
	secs   float64
}

// Active reports whether the slider is being dragged (or has a move that is
// not yet committed).
func (d *SeekDrag) Active() bool { return d.active }

// Pos is the position to show: the dragged one, or live.
func (d *SeekDrag) Pos(live time.Duration) time.Duration {
	if d.active {
		return time.Duration(d.secs * float64(time.Second))
	}
	return live
}

// Update follows the slider sl, whose value this frame is secs. It returns
// the position to seek to, and true, in the frame the move was committed.
func (d *SeekDrag) Update(sl ui.Element, secs float64) (to time.Duration, commit bool) {
	if sl.Changed() {
		d.active, d.secs = true, secs
	}
	if SliderCommitted(sl) && d.active {
		d.active = false
		return time.Duration(d.secs * float64(time.Second)), true
	}
	return 0, false
}

// Tick asks for a frame in 250 ms while a song plays and nobody drags, so the
// position moves (v0.2.0's useLivePosition).
func (d *SeekDrag) Tick(c *ui.Context, playing bool) {
	if playing && !d.active {
		c.After(seekTick)
	}
}
