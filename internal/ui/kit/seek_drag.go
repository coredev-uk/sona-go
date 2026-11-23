package kit

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

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

// Tick asks for the view to be built again when the second shown as the
// position changes, while a song plays and nobody drags: the elapsed time
// and what is left are text, and text is built, not painted. pos is the
// position shown, of a song dur long. The bar itself does not wait for it:
// see Playhead.
func (d *SeekDrag) Tick(c *ui.Context, playing bool, pos, dur time.Duration) {
	if !playing || d.active || (dur > 0 && pos >= dur) {
		return
	}
	// A millisecond past the boundary: the position is whole milliseconds,
	// and a timer is never early but the clock it is read against may be.
	c.After(time.Second - pos%time.Second + time.Millisecond)
}

// Playhead is where a seek bar's song is, frame by frame. A bar that read
// the position when the view was built could move only as often as the view
// was built, and each build is the whole window's work; a Playhead is read
// in the slider's Draw callback instead, which asks for the next frame as
// the display can show it (AnimationFrame: the view is not built nor laid
// out for it) and paints the fill and the thumb at the clock's position
// through a transform, which the device's pixels do not round (Painter.Fill
// rounds a rectangle's edges to whole pixels: the edge of a fill sized each
// frame would move in steps of one). The zero Playhead is a bar that does
// not move by itself, as for a value that is not a song's position.
type Playhead struct {
	drag    *SeekDrag
	live    func() time.Duration
	span    float64 // the bar's length in seconds
	motion  *MotionPolicy
	playing bool
}

// moving reports whether the bar follows the clock now: its song plays and
// nobody drags. The drag is read as the frame is painted, after the
// slider's own update of it, so the first frame of a drag already shows
// the pointer.
func (h Playhead) moving() bool { return h.playing && !h.drag.active }

// Playhead is the bar's playhead for this frame: it follows live, the
// playback position now (store.Playback.Position, a function of the clock
// alone), only while the song plays, its length is known and nobody drags
// (which it finds out as it paints).
// dur is the song's length. motion says whether the page is covered (nil:
// never): a covered bar asks for no frames, and shows the position at each
// second as the view is built again.
func (d *SeekDrag) Playhead(playing bool, dur time.Duration, live func() time.Duration, motion *MotionPolicy) Playhead {
	return Playhead{drag: d, live: live, span: max(dur.Seconds(), 1), motion: motion, playing: playing && dur > 0 && live != nil}
}

// Frac is how far along the bar the song is, from 0 to 1, in the frame pt
// paints: at when the playhead does not follow the clock (the position the
// view was built with), else the clock's. Call it in the Draw callback; it
// asks for the next frame while the bar moves, unless motion is reduced or
// the page is covered.
func (h Playhead) Frac(pt *ui.Painter, at float32) float32 {
	if !h.moving() {
		return at
	}
	secs := h.live().Seconds()
	if secs < h.span && !theme.ReducedMotion() && !h.motion.Covered() {
		AnimationFrames(pt)
	}
	return float32(min(max(secs/h.span, 0), 1))
}

// Fill paints the filled part of a bar: full is the fill at its longest, k
// (0 to 1) how much of its length is filled, from its left edge, or from its
// right one when fromRight. While the bar moves, full is drawn whole and
// squeezed by k from the edge it grows from, so that its far edge moves by
// fractions of a pixel; still, the shortened rectangle is filled and its
// edges are whole pixels, crisp.
func (h Playhead) Fill(pt *ui.Painter, full ui.Rect, k float32, fromRight bool, color ui.Color, radius float32) {
	if k <= 0 || full.W <= 0 {
		return
	}
	if !h.moving() {
		r := full
		r.W *= k
		if fromRight {
			r.X = full.X + full.W - r.W
		}
		pt.Fill(r, color, radius)
		return
	}
	ox := full.X
	if fromRight {
		ox += full.W
	}
	pt.Transform(ui.Scaling(k, 1), ox, full.Y, func() { pt.Fill(full, color, radius) })
}

// Travel draws a mark (a thumb) that has moved dx along the bar from where
// it rests at the bar's start. draw paints it, given the distance it has to
// move itself: all of dx while the bar is still, where the mark's edges are
// whole pixels, none while it moves (the mark is moved by a transform instead,
// to a fraction of a pixel).
func (h Playhead) Travel(pt *ui.Painter, dx float32, draw func(off float32)) {
	if !h.moving() {
		draw(dx)
		return
	}
	pt.Transform(ui.Translation(dx, 0), 0, 0, func() { draw(0) })
}
