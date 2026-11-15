package media

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/playback/video"
)

// hangGuard bounds a wait that an event always ends: it only turns a hang
// into a failure, and is long enough for a shared, loaded runner.
const hangGuard = 30 * time.Second

// spySource reports the sequence every Next call waits after, so a test can
// tell what the feed has taken and that it is asking for the next picture.
type spySource struct {
	FrameSource
	nexts chan uint64
}

func (s *spySource) Next(ctx context.Context, after uint64) (video.Frame, bool) {
	select {
	case s.nexts <- after:
	default:
	}
	return s.FrameSource.Next(ctx, after)
}

// nextAfter is the sequence of the next Next call.
func (s *spySource) nextAfter(t *testing.T) uint64 {
	t.Helper()
	select {
	case after := <-s.nexts:
		return after
	case <-time.After(hangGuard):
		t.Fatal("the feed never asked for a picture")
		return 0
	}
}

// planes is a w×h I420 picture of one grey.
func planes(w, h int, luma byte) []byte {
	b := make([]byte, I420Len(w, h))
	for i := range b {
		b[i] = 128
	}
	for i := range w * h {
		b[i] = luma
	}
	return b
}

func TestFeedShowsPicturesFromTheSurface(t *testing.T) {
	s := video.NewSurface()
	var mu sync.Mutex
	var queue []func()
	post := func(fn func()) { mu.Lock(); queue = append(queue, fn); mu.Unlock() }
	runPosted := func() {
		mu.Lock()
		q := queue
		queue = nil
		mu.Unlock()
		for _, fn := range q {
			fn()
		}
	}
	shown := make(chan Picture, 8)
	cleared := make(chan struct{}, 8)
	f := NewFeed(s, post, func(p Picture) { shown <- p }, func() { cleared <- struct{}{} }, nil)
	f.Start()
	f.Start() // idempotent
	defer f.Stop()

	wait := func() Picture {
		t.Helper()
		deadline := time.After(hangGuard)
		for {
			runPosted()
			select {
			case p := <-shown:
				return p
			case <-deadline:
				t.Fatal("no picture shown")
			case <-time.After(time.Millisecond):
			}
		}
	}

	s.Show(64, 48, 0, planes(64, 48, 235))
	p := wait()
	if p.Width != 64 || p.Height != 48 || len(p.Data) != I420Len(64, 48) {
		t.Fatalf("picture is %d×%d with %d bytes", p.Width, p.Height, len(p.Data))
	}
	if p.Data[0] != 235 {
		t.Errorf("the first luma sample is %d, want 235 as shown", p.Data[0])
	}
	first := p.Seq

	// Only the latest picture is kept: several shown before the main thread
	// runs are one.
	s.Show(64, 48, time.Second, planes(64, 48, 16))
	s.Show(64, 48, 2*time.Second, planes(64, 48, 16))
	last := wait()
	if last.Seq <= first {
		t.Errorf("sequence %d did not move past %d", last.Seq, first)
	}
	if last.Data[0] != 16 {
		t.Errorf("the first luma sample is %d, want 16 as shown", last.Data[0])
	}

	// A new size is a new picture. A picture of the old size may still be
	// queued from the two shown above (CI saw one arrive here), so skip
	// those.
	s.Show(32, 24, 0, planes(32, 24, 128))
	p = wait()
	for p.Width == 64 {
		p = wait()
	}
	if p.Width != 32 || p.Height != 24 {
		t.Errorf("picture is %d×%d", p.Width, p.Height)
	}

	// Cleared: the video ended.
	s.Clear()
	deadline := time.After(hangGuard)
	for done := false; !done; {
		runPosted()
		select {
		case <-cleared:
			done = true
		case <-deadline:
			t.Fatal("clear not reported")
		case <-time.After(time.Millisecond):
		}
	}
}

func TestFeedHoldsPicturesWhileNotDrawn(t *testing.T) {
	s := video.NewSurface()
	var drawn atomic.Bool
	var posts atomic.Int32
	var mu sync.Mutex
	var queue []func()
	post := func(fn func()) { posts.Add(1); mu.Lock(); queue = append(queue, fn); mu.Unlock() }
	shown := make(chan Picture, 8)
	f := NewFeed(s, post, func(p Picture) { shown <- p }, func() {}, drawn.Load)
	f.Start()
	defer f.Stop()

	s.Show(16, 16, 0, planes(16, 16, 128))
	// The feed took it and left it: held as skipped, nothing posted.
	for deadline := time.Now().Add(hangGuard); ; time.Sleep(time.Millisecond) {
		f.mu.Lock()
		held := f.skipped != nil
		f.mu.Unlock()
		if held {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the feed never took the picture")
		}
	}
	if posts.Load() != 0 {
		t.Fatalf("%d pictures were posted while not drawn", posts.Load())
	}

	// Drawing again: the picture the feed skipped is shown.
	drawn.Store(true)
	f.Refresh()
	deadline := time.After(hangGuard)
	for {
		mu.Lock()
		q := queue
		queue = nil
		mu.Unlock()
		for _, fn := range q {
			fn()
		}
		select {
		case <-shown:
			return
		case <-deadline:
			t.Fatal("the skipped picture was not shown after Refresh")
		case <-time.After(time.Millisecond):
		}
	}
}

func TestFeedStopEndsTheGoroutine(t *testing.T) {
	f := NewFeed(video.NewSurface(), func(fn func()) { fn() }, func(Picture) {}, func() {}, nil)
	f.Start()
	stopped := make(chan struct{})
	go func() { f.Stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(hangGuard):
		t.Fatal("Stop did not return")
	}
	f.Stop() // twice is fine
	// And it can start again.
	f.Start()
	f.Stop()
}

func TestFeedKeepsNothingAfterStop(t *testing.T) {
	s := video.NewSurface()
	var mu sync.Mutex
	var queue []func()
	post := func(fn func()) { mu.Lock(); queue = append(queue, fn); mu.Unlock() }
	shown := 0
	f := NewFeed(s, post, func(Picture) { shown++ }, func() {}, nil)
	f.Start()
	s.Show(16, 16, 0, planes(16, 16, 128))
	deadline := time.After(hangGuard)
	for {
		mu.Lock()
		n := len(queue)
		mu.Unlock()
		if n > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("no picture posted")
		case <-time.After(time.Millisecond):
		}
	}
	// Stopped with a picture posted and not yet shown: it is never shown, and
	// nothing is held.
	f.Stop()
	mu.Lock()
	q := queue
	queue = nil
	mu.Unlock()
	for _, fn := range q {
		fn()
	}
	if shown != 0 {
		t.Errorf("%d pictures shown after Stop", shown)
	}
	f.mu.Lock()
	held := f.pending != nil || f.skipped != nil
	f.mu.Unlock()
	if held {
		t.Error("the feed still holds pictures after Stop")
	}
	// A restart does not flash the old picture, and a Refresh after Stop does nothing.
	f.Refresh() // synchronous: the goroutine is gone, so anything it posted is already queued
	mu.Lock()
	n := len(queue)
	mu.Unlock()
	if n != 0 || shown != 0 {
		t.Errorf("a stopped feed posted %d funcs, showed %d", n, shown)
	}
}

func TestFeedRestartTakesOnlyPicturesShownFromNow(t *testing.T) {
	s := video.NewSurface()
	s.Show(64, 48, 0, planes(64, 48, 235)) // the item before's last picture
	src := &spySource{FrameSource: s, nexts: make(chan uint64, 16)}
	shown := make(chan Picture, 8)
	f := NewFeed(src, func(fn func()) { fn() }, func(p Picture) { shown <- p }, func() {}, nil)
	defer f.Stop()

	f.Restart()
	// The feed waits for a picture after the one on show (1) rather than
	// taking it: had it asked after 0 it would be shown at once.
	if after := src.nextAfter(t); after != 1 {
		t.Fatalf("the feed took the picture on show before the restart: it asked after %d, want 1", after)
	}
	select {
	case p := <-shown:
		t.Fatalf("the picture on show before the restart was taken (seq %d)", p.Seq)
	default:
	}
	s.Show(64, 48, time.Second, planes(64, 48, 16))
	select {
	case p := <-shown:
		if p.Seq != 2 {
			t.Errorf("picture seq %d, want the new one (2)", p.Seq)
		}
	case <-time.After(hangGuard):
		t.Fatal("no picture after the restart")
	}
	// A Start after a Start-less Stop begins with the picture on show.
	f.Stop()
	f.Start()
	select {
	case p := <-shown:
		if p.Seq != 2 {
			t.Errorf("Start took seq %d, want the picture on show (2)", p.Seq)
		}
	case <-time.After(hangGuard):
		t.Fatal("Start did not take the picture on show")
	}
}

func TestFeedDropsAPictureThatWasPublishedAcrossARestart(t *testing.T) {
	s := video.NewSurface()
	s.Show(64, 48, 0, planes(64, 48, 235)) // the item before's last picture, Seq 1
	shown := make(chan Picture, 4)
	f := NewFeed(s, func(fn func()) { fn() }, func(p Picture) { shown <- p }, func() {}, nil)
	defer f.Stop()
	f.Restart()
	// The old picture, taken before the restart, is published now, with the
	// feed running again.
	f.publish(video.Frame{Seq: 1, Width: 64, Height: 48, Data: planes(64, 48, 235)})
	// publish is synchronous and the poster runs at once: a picture it
	// accepted is already shown.
	select {
	case p := <-shown:
		t.Fatalf("the picture of before the restart was shown (seq %d)", p.Seq)
	default:
	}
	// A picture shown after it is taken.
	f.publish(video.Frame{Seq: 2, Width: 64, Height: 48, Data: planes(64, 48, 16)})
	select {
	case p := <-shown:
		if p.Seq != 2 {
			t.Errorf("shown seq %d, want 2", p.Seq)
		}
	case <-time.After(hangGuard):
		t.Fatal("the new picture was not shown")
	}
}

// A malformed picture (planes shorter than its size says) never reaches the
// bitmap, which would panic on it, and the one before stays.
func TestFeedDropsAMalformedPicture(t *testing.T) {
	s := video.NewSurface()
	src := &spySource{FrameSource: s, nexts: make(chan uint64, 16)}
	var posts atomic.Int32
	f := NewFeed(src, func(fn func()) { posts.Add(1); fn() }, func(Picture) {}, func() {}, nil)
	f.Start()
	defer f.Stop()
	s.Show(64, 48, 0, planes(64, 48, 128)[:100])
	// The feed asks for the picture after this one (1) only once it has
	// dealt with it.
	for src.nextAfter(t) != 1 {
	}
	if n := posts.Load(); n != 0 {
		t.Fatalf("%d posts for a malformed picture", n)
	}
}
