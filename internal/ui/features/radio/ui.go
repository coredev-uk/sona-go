package radio

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// Page geometry, from the reference page (AppShell's main and the pages'
// own spacing), in DIPs.
const (
	bandGap    = 56 // mt-14: the line across the column above a band
	bandPad    = 24 // pt-6: from that line to the band's heading
	headingGap = 20 // mb-5: under a band's heading
	textMax    = 72 * 9
	railMin    = 880 // content width from which a band hangs its heading in a rail
)

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
