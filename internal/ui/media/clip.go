package media

import (
	"image"
	"image/draw"
	"io"
	"time"

	"github.com/egoist/mygo/ui"
)

// Motion is a motion artwork player OpenMotion opened: a *Clip (decoded in
// this process, painted by Clip.Paint without building the view) or MyGo's
// *ui.VideoPlayer (the system's own framework, which a built ui.Video shows).
type Motion interface {
	// Play goes on from where Pause held it; a new player plays at once.
	Play()
	// Pause holds the picture on show.
	Pause()
	// Playing reports whether it moves: neither paused nor stopped.
	Playing() bool
	// Ready reports whether it has a picture to show.
	Ready() bool
	// Err is what stopped it before its end.
	Err() error
	// Close releases the decoder.
	Close()
}

const (
	// clipAhead is the frames a clip keeps decoded ahead of its playhead.
	clipAhead = 2
	// clipLag is how far behind the clock the playhead may fall before it
	// stops catching up and goes on from now: a window that was hidden, or
	// a frame that took long, does not race through the frames it missed.
	clipLag = 250 * time.Millisecond
	// clipPoll is how soon a clip whose next frame is not decoded yet looks
	// again.
	clipPoll = 4 * time.Millisecond
)

// A Clip is a looping motion artwork decoded in this process, shown by
// painting alone: a goroutine decodes the frames a couple ahead of the
// playhead, and Paint, called from a Draw callback, puts the frame that is
// due into one bitmap (Bitmap.Update, so the GPU keeps its texture) and asks
// to be painted again when the next one is, with Painter.After. A frame of the
// video is a frame that paints the cover's Draw callback again, never one
// that builds and lays out the view (ui.Video's frames do), and the clip asks
// at its own frame times, never faster.
//
// The clip belongs to the main thread, as the Draw callback does.
type Clip struct {
	d   *clipDecode
	bmp *ui.Bitmap

	// started is whether the goroutine decoding ahead runs; frames are what
	// it sends, and done is closed as it returns.
	started bool
	frames  chan clipFrame
	done    chan struct{}

	// shown is the bitmap's picture, unchanged until the next Update, and
	// due the time of the playhead's frame ending: the zero time while the
	// clip has not painted since it began or was paused, which left is how
	// much of the frame shown remained then.
	shown  *image.RGBA
	due    time.Time
	left   time.Duration
	seen   time.Time // the last frame Paint painted
	paused bool
	closed bool
	err    error
}

// clipFrame is a frame decoded ahead, or with no image what stopped decoding.
type clipFrame struct {
	img *image.RGBA
	dur time.Duration
	err error
}

// clipDecode is what a clip shares with the goroutine decoding for it, which
// must not keep the clip alive.
type clipDecode struct {
	d    ui.VideoDecoder
	free chan *image.RGBA // frames shown or passed over, to decode into again
	quit chan struct{}
}

// NewClip returns a clip of d's frames, playing. It decodes the first frame at
// once, which is on show until the playhead leaves it, and reports a video
// that does not decode, so it blocks: call it off the main thread. Closing the
// clip closes d if d is an io.Closer.
func NewClip(d ui.VideoDecoder) (*Clip, error) {
	f, err := d.Next()
	if err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	if err != nil {
		return nil, err
	}
	first := copyFrame(nil, f.Image)
	dec := &clipDecode{d: d, free: make(chan *image.RGBA, clipAhead+2), quit: make(chan struct{})}
	return &Clip{d: dec, bmp: ui.NewBitmap(first), shown: first, left: clipDuration(f.Duration)}, nil
}

// Size is the size of the clip's pictures in pixels.
func (k *Clip) Size() (w, h int) { return k.bmp.Size() }

// Play goes on from the frame it held.
func (k *Clip) Play() { k.paused = false }

// Pause holds the frame on show, and stops decoding once the frames ahead
// are ready. The next Paint paints it as it is and asks for no more.
func (k *Clip) Pause() {
	if k.paused {
		return
	}
	k.paused = true
	if !k.due.IsZero() {
		k.left = max(k.due.Sub(k.seen), 0)
		k.due = time.Time{}
	}
}

// Playing reports whether the clip plays: not paused, closed or failed.
func (k *Clip) Playing() bool { return !k.paused && !k.closed && k.err == nil }

// Ready is always true: the first frame is decoded.
func (k *Clip) Ready() bool { return true }

// Err is what stopped the decoder, as a frame that did not decode. The
// picture stays at the frame before.
func (k *Clip) Err() error { return k.err }

// Close stops decoding, and closes the decoder if it is an io.Closer, on a
// goroutine of its own once the frame it may be decoding is done.
func (k *Clip) Close() {
	if k.closed {
		return
	}
	k.closed = true
	close(k.d.quit)
	done := k.done
	k.frames, k.done = nil, nil
	if c, ok := k.d.d.(io.Closer); ok {
		go func() {
			if done != nil {
				<-done
			}
			_ = c.Close()
		}()
	}
}

// Paint paints the frame due, scaled to cover r, from a Draw callback of the
// element that shows the clip, and asks to be painted again when the next
// frame is due. A clip that is paused or failed paints the frame it holds
// and asks for nothing.
func (k *Clip) Paint(pt *ui.Painter, r ui.Rect) {
	if wait := k.advance(pt.Now()); wait > 0 {
		pt.After(wait)
	}
	pt.Image(k.bmp, r, ui.Cover)
}

// advance moves the playhead on to now, showing the frame due: the last of
// those that came due, the others passed over, so that a frame decoded late
// catches up rather than lags. It returns how long until it is to look again,
// 0 for never.
func (k *Clip) advance(now time.Time) time.Duration {
	k.seen = now
	if k.paused || k.closed || k.err != nil {
		return 0
	}
	if !k.started {
		k.started = true
		k.frames, k.done = make(chan clipFrame, clipAhead), make(chan struct{})
		go k.d.run(k.frames, k.done)
	}
	if k.due.IsZero() {
		k.due = now.Add(k.left)
	}
	if now.Sub(k.due) > clipLag {
		k.due = now
	}
	var next *image.RGBA
	for !now.Before(k.due) {
		var f clipFrame
		select {
		case f = <-k.frames:
		default:
			// Not decoded yet: look again soon.
			k.show(next)
			return clipPoll
		}
		if f.img == nil {
			k.err, k.due = f.err, time.Time{}
			k.show(next)
			return 0
		}
		k.d.release(next)
		next = f.img
		k.due = k.due.Add(f.dur)
	}
	k.show(next)
	return k.due.Sub(now)
}

// show puts img, if any, on the bitmap in place of the picture it showed,
// which the goroutine decodes into again.
func (k *Clip) show(img *image.RGBA) {
	if img == nil {
		return
	}
	k.bmp.Update(img)
	k.d.release(k.shown)
	k.shown = img
}

// release gives a frame no longer shown back to the goroutine, to decode
// into again.
func (d *clipDecode) release(img *image.RGBA) {
	if img == nil {
		return
	}
	select {
	case d.free <- img:
	default:
	}
}

// run decodes frames into frames until the clip is closed, going back to the
// first frame when it ends. It sends what stopped it as a frame of no image.
func (d *clipDecode) run(frames chan<- clipFrame, done chan<- struct{}) {
	defer close(done)
	send := func(f clipFrame) bool {
		select {
		case frames <- f:
			return true
		case <-d.quit:
			return false
		}
	}
	for {
		select {
		case <-d.quit:
			return
		default:
		}
		f, err := d.d.Next()
		if err == io.EOF {
			if err = d.d.Rewind(); err == nil {
				f, err = d.d.Next()
			}
		}
		if err != nil {
			send(clipFrame{err: err})
			return
		}
		var buf *image.RGBA
		select {
		case buf = <-d.free:
		default:
		}
		if !send(clipFrame{img: copyFrame(buf, f.Image), dur: clipDuration(f.Duration)}) {
			return
		}
	}
}

// copyFrame copies img into dst, or into a new image when dst is nil or of
// another size: the decoder draws its next frame into img, and the bitmap
// keeps the copy until its next Update.
func copyFrame(dst *image.RGBA, img image.Image) *image.RGBA {
	b := img.Bounds()
	if dst == nil || dst.Rect.Dx() != b.Dx() || dst.Rect.Dy() != b.Dy() {
		dst = image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	}
	draw.Draw(dst, dst.Rect, img, b.Min, draw.Src)
	return dst
}

// clipDuration is how long a frame shows: 100 ms for none, as browsers show
// GIFs'.
func clipDuration(d time.Duration) time.Duration {
	if d <= 0 {
		return 100 * time.Millisecond
	}
	return d
}

var _ Motion = (*Clip)(nil)
var _ Motion = (*ui.VideoPlayer)(nil)
