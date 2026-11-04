package media

import (
	"context"
	"image"
	"sync"

	"github.com/coredev-uk/sona-go/internal/playback/video"
)

// FrameSource is the picture on show of the playing music video:
// *video.Surface is one. Next waits for a picture newer than after (a Seq the
// caller has) and reports false when ctx ended or the surface was cleared
// (the Frame's Seq is then the one to wait from).
type FrameSource interface {
	Next(ctx context.Context, after uint64) (video.Frame, bool)
	// Seq is the sequence number of the latest change of the picture on
	// show: what a feed that wants only pictures shown from now on waits
	// after (Feed.Restart).
	Seq() uint64
}

var _ FrameSource = (*video.Surface)(nil)

// Picture is a music video picture as an RGBA image, valid until the next
// one is shown.
type Picture struct {
	Image *image.RGBA
	// Seq is the surface's sequence number of the picture.
	Seq uint64
}

// Feed turns the music video's pictures into RGBA images for a bitmap: one
// goroutine takes each picture the source shows (the latest only, never a
// backlog: a picture that was not drawn before the next arrived is dropped),
// converts it off the main thread (I420ToRGBA) and hands it over through the
// poster, which runs show on the main thread with the picture and clear when
// the video ends. Pictures live in memory only.
//
// While Active reports false (the view is not drawn, the window is hidden)
// pictures are taken but not converted, and the newest one is converted when
// Refresh says drawing resumed.
type Feed struct {
	src    FrameSource
	post   func(func())
	show   func(Picture)
	clear  func()
	active func() bool

	mu        sync.Mutex
	pending   *Picture    // converted, not yet shown
	shown     *image.RGBA // on screen
	spare     *image.RGBA // a buffer to convert into
	skipped   *video.Frame
	scheduled bool
	cancel    context.CancelFunc
	done      chan struct{}
}

// NewFeed returns a Feed of src. post runs a func on the main thread (the
// UI's Poster); show and clear run there; active (nil: always) says whether
// the video is being drawn.
func NewFeed(src FrameSource, post func(func()), show func(Picture), clear func(), active func() bool) *Feed {
	if active == nil {
		active = func() bool { return true }
	}
	return &Feed{src: src, post: post, show: show, clear: clear, active: active}
}

// Start begins taking pictures, the one on show first; it does nothing when
// it already has.
func (f *Feed) Start() { f.start(false) }

// Restart forgets the picture and begins again with the pictures shown from
// now on: the one on show now (of the item before, when the item changed) is
// never taken. A feed that is running stops first.
func (f *Feed) Restart() {
	f.Stop()
	f.start(true)
}

func (f *Feed) start(fresh bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.cancel != nil {
		return
	}
	var after uint64
	if fresh {
		after = f.src.Seq()
	}
	var ctx context.Context
	ctx, f.cancel = context.WithCancel(context.Background())
	f.done = make(chan struct{})
	go f.run(ctx, f.done, after)
}

// Stop ends the goroutine and waits for it. A picture posted before is shown
// or not as the main thread gets to it. The feed may be started again.
func (f *Feed) Stop() {
	f.mu.Lock()
	cancel, done := f.cancel, f.done
	f.cancel, f.done = nil, nil
	f.mu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	<-done
	// Nothing of the video stays: no picture waiting, none held for reuse.
	f.mu.Lock()
	f.pending, f.skipped, f.shown, f.spare = nil, nil, nil, nil
	f.mu.Unlock()
}

// Refresh converts and shows the newest picture the feed skipped while it was
// not drawn (a paused video has no next picture to wait for). Call it as the
// view is drawn again.
func (f *Feed) Refresh() {
	f.mu.Lock()
	fr := f.skipped
	f.skipped = nil
	running := f.cancel != nil
	f.mu.Unlock()
	if fr != nil && running {
		go f.publish(*fr) // off the main thread: the conversion is the cost
	}
}

func (f *Feed) run(ctx context.Context, done chan<- struct{}, after uint64) {
	defer close(done)
	for ctx.Err() == nil {
		fr, ok := f.src.Next(ctx, after)
		if !ok {
			if ctx.Err() != nil {
				return
			}
			// Cleared: nothing with a picture plays. Wait from the
			// sequence the surface named.
			after = fr.Seq
			f.mu.Lock()
			f.pending, f.skipped, f.shown, f.spare = nil, nil, nil, nil
			f.mu.Unlock()
			f.post(f.clear)
			continue
		}
		after = fr.Seq
		if !f.active() {
			f.mu.Lock()
			f.skipped = &fr
			f.mu.Unlock()
			continue
		}
		f.publish(fr)
	}
}

// publish converts fr and makes it the picture to show next.
func (f *Feed) publish(fr video.Frame) {
	f.mu.Lock()
	if f.cancel == nil {
		f.mu.Unlock()
		return // stopped
	}
	dst := f.spare
	f.spare = nil
	f.skipped = nil
	f.mu.Unlock()
	img := I420ToRGBA(dst, fr.Width, fr.Height, fr.Data)
	if img == nil {
		return // a malformed picture: the one before stays
	}
	f.mu.Lock()
	if f.cancel == nil { // stopped while it converted
		f.mu.Unlock()
		return
	}
	if f.pending != nil && f.pending.Seq > fr.Seq {
		// A newer picture was published while this one converted.
		f.spare = img
		f.mu.Unlock()
		return
	}
	if f.pending != nil {
		f.spare = f.pending.Image // dropped before it was shown
	}
	f.pending = &Picture{Image: img, Seq: fr.Seq}
	schedule := !f.scheduled
	f.scheduled = true
	f.mu.Unlock()
	if schedule {
		f.post(f.apply)
	}
}

// apply shows the pending picture, on the main thread.
func (f *Feed) apply() {
	f.mu.Lock()
	p := f.pending
	f.pending = nil
	f.scheduled = false
	f.mu.Unlock()
	if p == nil {
		return
	}
	f.show(*p)
	// The bitmap holds the new image now, so the previous one is free.
	f.mu.Lock()
	prev := f.shown
	f.shown = p.Image
	if prev != nil && f.spare == nil {
		f.spare = prev
	}
	f.mu.Unlock()
}
