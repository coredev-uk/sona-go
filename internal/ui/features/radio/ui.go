package radio

import (
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// Page geometry, from the reference page (AppShell's main and the pages'
// own spacing), in DIPs.
const (
	bandGap    = 56 // mt-14: the line across the column above a band
	bandPad    = 24 // pt-6: from that line to the band's heading
	headingGap = 20 // mb-5: under a band's heading
	textMax    = 72 * 9
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

// progressTrack is a programme's progress bar: a 2 DIP track (--active) with
// a fill of value (0..1) in fill, a progress indicator for assistive
// technology named label and reading valueText (the percentage when empty).
func progressTrack(c *ui.Context, key, label, valueText string, value float64, fill ui.Color) ui.Element {
	e := ui.Box(c.Key(key)).Height(2).Shrink(0).Radius(theme.Current().Radius["full"]).Background(theme.C("active")).Clip().
		Role(ui.RoleProgress).Label(label)
	pct := float64(int(value*100 + 0.5))
	e.Range(0, 100, pct)
	if valueText == "" {
		valueText = strconv.Itoa(int(pct)) + "%"
	}
	e.Value(valueText)
	return e.Draw(func(p *ui.Painter, r ui.Rect) {
		if w := r.W * float32(value); w > 0 {
			p.Fill(ui.Rect{X: r.X, Y: r.Y, W: w, H: r.H}, fill, r.H/2)
		}
	})
}
