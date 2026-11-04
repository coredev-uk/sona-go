package kit

import "github.com/egoist/mygo/ui"

// PENDING: stand-ins for kit-media's components, with the signatures it
// agreed (EqualiserBars, TileMotion, MotionPolicy, TileMotionOpts). They
// draw nothing real; delete this file when kit-media lands.

// MotionPolicy decides whether motion artwork plays (kit-media's).
type MotionPolicy struct{}

// EqualiserBars is the now-playing mark (kit-media's).
func EqualiserBars(c *ui.Context, playing bool, p *MotionPolicy) ui.Element {
	return ui.Box(c).Size(12, 12)
}

// TileMotionOpts are the options of TileMotion (kit-media's).
type TileMotionOpts struct{ Round, Hot, Focus bool }

// TileMotion plays a tile's motion cover (kit-media's).
func TileMotion(c *ui.Context, p *MotionPolicy, motion string, size float32, o TileMotionOpts) ui.Element {
	return ui.Box(c).Size(size, size)
}
