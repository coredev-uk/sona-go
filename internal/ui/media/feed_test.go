package media

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/playback/video"
)

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
		deadline := time.After(5 * time.Second)
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
	if b := p.Image.Bounds(); b.Dx() != 64 || b.Dy() != 48 {
		t.Fatalf("picture is %v", b)
	}
	if px := p.Image.Pix[:4]; px[0] != 255 || px[1] != 255 || px[2] != 255 || px[3] != 255 {
		t.Errorf("white luma converted to %v", px)
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
	if px := last.Image.Pix[:4]; px[0] != 0 || px[1] != 0 || px[2] != 0 {
		t.Errorf("black luma converted to %v", px)
	}

	// A new size gets a new image.
	s.Show(32, 24, 0, planes(32, 24, 128))
	if p := wait(); p.Image.Bounds().Dx() != 32 {
		t.Errorf("picture is %v", p.Image.Bounds())
	}

	// Cleared: the video ended.
	s.Clear()
	deadline := time.After(5 * time.Second)
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

func TestFeedSkipsConvertingWhileNotDrawn(t *testing.T) {
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
	time.Sleep(100 * time.Millisecond) // the feed took it and left it
	if posts.Load() != 0 {
		t.Fatalf("%d pictures were converted and posted while not drawn", posts.Load())
	}

	// Drawing again: the picture the feed skipped is converted and shown.
	drawn.Store(true)
	f.Refresh()
	deadline := time.After(5 * time.Second)
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
	case <-time.After(5 * time.Second):
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
	deadline := time.After(5 * time.Second)
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
	held := f.pending != nil || f.shown != nil || f.spare != nil || f.skipped != nil
	f.mu.Unlock()
	if held {
		t.Error("the feed still holds decoded pictures after Stop")
	}
	// A restart does not flash the old picture, and a Refresh after Stop does nothing.
	f.Refresh()
	time.Sleep(20 * time.Millisecond)
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
	shown := make(chan Picture, 8)
	f := NewFeed(s, func(fn func()) { fn() }, func(p Picture) { shown <- p }, func() {}, nil)
	defer f.Stop()

	f.Restart()
	select {
	case p := <-shown:
		t.Fatalf("the picture on show before the restart was taken (seq %d)", p.Seq)
	case <-time.After(50 * time.Millisecond):
	}
	s.Show(64, 48, time.Second, planes(64, 48, 16))
	select {
	case p := <-shown:
		if p.Seq != 2 {
			t.Errorf("picture seq %d, want the new one (2)", p.Seq)
		}
	case <-time.After(5 * time.Second):
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
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not take the picture on show")
	}
}
