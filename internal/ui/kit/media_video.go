package kit

import (
	"image"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/media"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// drawnFor is how long after the last time VideoSurface painted the video
// still counts as drawn: pictures are shown only while it is.
const drawnFor = 500 * time.Millisecond

// VideoFeed keeps the picture on show of the playing music video as a bitmap
// for VideoSurface: the latest picture only, in memory only. The CDM host
// child decodes and times the pictures (internal/playback/video.Surface holds
// the one on show); a goroutine takes each and hands it to the main thread,
// which updates one bitmap in place (Bitmap.UpdateYUV: the I420 planes as the
// child sent them, 1.5 bytes a pixel, converted by the GPU as it samples
// them), so the GPU keeps its texture and uploads the new planes. No CPU
// converts the picture: only the chroma planes are interleaved, on the main
// thread, as the GPU takes them. While the video is not being drawn (the view
// is closed, the window hidden or covered) pictures are not handed over at
// all.
type VideoFeed struct {
	feed *media.Feed

	// The main thread only.
	bmp     *ui.Bitmap
	picture bool // bmp shows a picture (false before the first and after a clear)
	seq     uint64
	w, h    int    // the picture's size
	shown   uint64 // how many pictures were shown since Start

	drawn atomic.Int64 // when VideoSurface last painted (unix ns)
}

// NewVideoFeed returns a feed of src (*video.Surface) that publishes through
// post (the app's Poster). It takes nothing until Start.
func NewVideoFeed(src media.FrameSource, post store.Poster) *VideoFeed {
	v := &VideoFeed{}
	if post == nil {
		post = func(fn func()) { fn() }
	}
	v.feed = media.NewFeed(src, post, v.show, v.clear, v.active)
	return v
}

// Start begins taking pictures; call it when a video starts playing or the
// view opens. It does nothing when it already has.
func (v *VideoFeed) Start() { v.feed.Start() }

// Stop ends the goroutine, when the view closes or the app quits, and
// forgets the picture.
func (v *VideoFeed) Stop() {
	v.feed.Stop()
	v.clear()
}

// Restart forgets the picture and takes only the pictures shown from now on:
// the one on show now belongs to the item before when the item just changed.
// Main thread.
func (v *VideoFeed) Restart() {
	v.clear()
	v.feed.Restart()
}

// HasPicture reports whether a picture is on show (false before the first
// and after the video ended). Main thread.
func (v *VideoFeed) HasPicture() bool { return v.picture }

// PictureSize is the size in pixels of the picture on show, 0×0 when there
// is none. Main thread.
func (v *VideoFeed) PictureSize() (w, h int) {
	if !v.picture {
		return 0, 0
	}
	return v.w, v.h
}

// Pictures counts the pictures shown since the feed was made: a playing video
// whose count stops moving has stalled. Main thread.
func (v *VideoFeed) Pictures() uint64 { return v.shown }

func (v *VideoFeed) active() bool {
	return time.Since(time.Unix(0, v.drawn.Load())) < drawnFor
}

// show is the main thread's: the next picture goes into the bitmap.
func (v *VideoFeed) show(p media.Picture) {
	img := p.YUV()
	if img == nil {
		return // the feed hands over whole pictures only
	}
	if v.bmp == nil {
		// UpdateYUV gives the bitmap its picture and its size.
		v.bmp = ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 1, 1)))
	}
	v.bmp.UpdateYUV(img)
	v.picture, v.seq = true, p.Seq
	v.w, v.h = p.Width, p.Height
	v.shown++
}

// clear is the main thread's: the video ended, the picture comes down.
func (v *VideoFeed) clear() { v.picture, v.bmp, v.w, v.h = false, nil, 0, 0 }

// VideoOpts are the options of VideoSurface.
type VideoOpts struct {
	// A11yLabel names the video for assistive technology ("Music video").
	A11yLabel string
}

// VideoSurface shows the playing music video's picture, as large as fits the
// box it is built in with its own shape (letterboxed, never cropped or
// stretched). Nothing shows until the first picture arrives. Frames reach it
// through the feed; building it marks the video as drawn, which is what lets
// the feed show pictures at all. The box fills its parent: size the parent.
func VideoSurface(c *ui.Context, v *VideoFeed, o VideoOpts) ui.Element {
	box := ui.Box(c).Fill()
	if o.A11yLabel != "" {
		box.Label(o.A11yLabel)
	}
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		// Painting: the video is on screen. Pictures skipped while it
		// was not are shown now.
		resumed := !v.active()
		v.drawn.Store(time.Now().UnixNano())
		if resumed {
			v.feed.Refresh()
		}
		if v.picture && v.bmp != nil {
			p.Image(v.bmp, r, ui.Contain)
		}
	})
	return box
}
