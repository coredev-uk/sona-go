package kit

import (
	"cmp"
	"context"
	"math"
	"runtime"
	"slices"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/artwork"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// artSource is what the loader needs of the artwork cache: *artwork.Cache
// is one, tests fake it.
type artSource interface {
	// Template is the Apple template behind a Go-served art path.
	Template(urlPath string) (template string, ok bool)
	// Get is the JPEG of a template at an exact box in pixels.
	Get(ctx context.Context, template string, w, h int) ([]byte, error)
}

var _ artSource = (*artwork.Cache)(nil)

const (
	// maxArt is how many decoded covers the loader keeps before it drops
	// the least recently asked for (a few hundred tiles of a long grid).
	maxArt = 800
	// releaseAfter is how many frames a bitmap is drawn before its CPU
	// pixels are dropped (Bitmap.ReleasePixels): it then draws from the GPU.
	releaseAfter = 2
)

// retryAfter are the waits before a cover that failed is asked for again (a
// new playlist's cover renders for a while), then it is left alone.
var retryAfter = []time.Duration{5 * time.Second, 30 * time.Second}

// ArtLoader decodes artwork at the exact size it is shown, off the main
// thread, and keeps the bitmaps. Workers fetch a cover's JPEG from the
// artwork cache (no HTTP: the DTO's "/art/<key>/…" path names a key) and
// decode it with ui.DecodeBitmap; the result reaches the main thread through
// the Poster, which redraws. Artwork asks for covers while it builds.
type ArtLoader struct {
	src  artSource
	post store.Poster
	now  func() time.Time

	// The main thread only.
	scale  float32
	covers map[artKey]*cover
	clock  uint64 // advances with every ask, for dropping the oldest

	jobs struct {
		sync.Mutex
		cond   *sync.Cond
		stack  []*cover // last in, first out: what scrolled into view last first
		closed bool
		once   sync.Once
	}
	workers int
	ctx     context.Context
	cancel  context.CancelFunc
}

type artKey struct {
	src    string
	pw, ph int
}

type coverState uint8

const (
	coverIdle    coverState = iota // not asked for yet
	coverQueued                    // a worker has it or will
	coverReady                     // bmp is set
	coverWaiting                   // failed: asked for again after retryAt
	coverFailed                    // failed for good: the placeholder stays
)

type cover struct {
	key      artKey
	template string
	state    coverState
	bmp      *ui.Bitmap
	fails    int
	retryAt  time.Time
	shown    int // frames drawn, for ReleasePixels
	used     uint64
}

// NewArtLoader returns a loader over the artwork cache that publishes
// through post (app's Poster).
func NewArtLoader(cache *artwork.Cache, post store.Poster) *ArtLoader {
	return newArtLoader(cache, post, max(4, runtime.NumCPU()/2))
}

func newArtLoader(src artSource, post store.Poster, workers int) *ArtLoader {
	if post == nil {
		post = func(fn func()) { fn() }
	}
	l := &ArtLoader{src: src, post: post, now: time.Now, scale: 1, covers: map[artKey]*cover{}, workers: workers}
	l.ctx, l.cancel = context.WithCancel(context.Background())
	l.jobs.cond = sync.NewCond(&l.jobs.Mutex)
	return l
}

// SetScale tells the loader the window's device pixels per DIP before the
// first frame; Artwork learns it from painting otherwise (a first frame on
// a scaled screen asks for covers at scale 1, then again at the right
// size).
func (l *ArtLoader) SetScale(s float32) {
	if s > 0 && s != l.scale {
		l.scale = s
	}
}

// noteScale is Artwork's paint callback: the window's scale is only known
// to a painter.
func (l *ArtLoader) noteScale(s float32) {
	if s > 0 && s != l.scale {
		l.scale = s
		l.post(func() {}) // a frame, which asks for the covers at the new size
	}
}

// Close stops the workers; covers still queued are dropped.
func (l *ArtLoader) Close() {
	l.cancel()
	l.jobs.Lock()
	l.jobs.closed = true
	l.jobs.stack = nil
	l.jobs.cond.Broadcast()
	l.jobs.Unlock()
}

// cover is the cover of src at a box of w×h DIPs, asking for it the first
// time. nil when src names nothing.
func (l *ArtLoader) cover(src string, w, h float32) *cover {
	if src == "" {
		return nil
	}
	key := artKey{src, devicePx(w, l.scale), devicePx(h, l.scale)}
	cv := l.covers[key]
	if cv == nil {
		if len(l.covers) >= maxArt {
			l.evict()
		}
		cv = &cover{key: key}
		l.covers[key] = cv
	}
	l.clock++
	cv.used = l.clock
	switch cv.state {
	case coverIdle:
		cv.template = l.templateOf(src)
		if cv.template == "" {
			cv.state = coverFailed // not one of ours: the placeholder stays
			return cv
		}
		l.enqueue(cv)
	case coverWaiting:
		if !l.now().Before(cv.retryAt) {
			l.enqueue(cv)
		}
	}
	return cv
}

// devicePx is a length in DIPs as device pixels, at least one.
func devicePx(dip, scale float32) int {
	return max(1, int(math.Round(float64(dip*scale))))
}

// templateOf is the Apple template of a Go-served art path, or src itself
// when it is a template.
func (l *ArtLoader) templateOf(src string) string {
	if t, ok := l.src.Template(src); ok {
		return t
	}
	if len(src) > 8 && (src[:8] == "https://" || src[:7] == "http://") {
		return src
	}
	return ""
}

func (l *ArtLoader) enqueue(cv *cover) {
	cv.state = coverQueued
	l.jobs.once.Do(func() {
		for range l.workers {
			go l.work()
		}
	})
	l.jobs.Lock()
	l.jobs.stack = append(l.jobs.stack, cv)
	l.jobs.cond.Signal()
	l.jobs.Unlock()
}

// work is a worker: it takes the newest request, fetches and decodes it.
func (l *ArtLoader) work() {
	for {
		l.jobs.Lock()
		for len(l.jobs.stack) == 0 && !l.jobs.closed {
			l.jobs.cond.Wait()
		}
		if l.jobs.closed {
			l.jobs.Unlock()
			return
		}
		cv := l.jobs.stack[len(l.jobs.stack)-1]
		l.jobs.stack = l.jobs.stack[:len(l.jobs.stack)-1]
		l.jobs.Unlock()

		// cv's key and template are read-only while queued.
		bmp, err := l.load(cv.key, cv.template)
		l.post(func() { l.finish(cv, bmp, err) })
	}
}

func (l *ArtLoader) load(key artKey, template string) (*ui.Bitmap, error) {
	data, err := l.src.Get(l.ctx, template, key.pw, key.ph)
	if err != nil {
		return nil, err
	}
	return ui.DecodeBitmap(data)
}

// finish publishes a decoded cover, on the main thread.
func (l *ArtLoader) finish(cv *cover, bmp *ui.Bitmap, err error) {
	if err == nil {
		cv.bmp, cv.state = bmp, coverReady
		return
	}
	if l.ctx.Err() != nil {
		cv.state = coverFailed
		return
	}
	cv.fails++
	if cv.fails > len(retryAfter) {
		cv.state = coverFailed
		return
	}
	cv.state, cv.retryAt = coverWaiting, l.now().Add(retryAfter[cv.fails-1])
}

// evict drops the least recently asked-for quarter of the covers that are
// not in flight.
func (l *ArtLoader) evict() {
	idle := make([]*cover, 0, len(l.covers))
	for _, cv := range l.covers {
		if cv.state != coverQueued {
			idle = append(idle, cv)
		}
	}
	if len(idle) == 0 {
		return
	}
	slices.SortFunc(idle, func(a, b *cover) int { return cmp.Compare(a.used, b.used) })
	for _, cv := range idle[:max(1, len(idle)/4)] {
		delete(l.covers, cv.key)
	}
}

// drawn counts a frame a cover's bitmap was painted in, and drops its CPU
// pixels after releaseAfter of them (the GPU has it by then).
func (cv *cover) drawn() {
	if cv.bmp == nil {
		return
	}
	cv.shown++
	if cv.shown == releaseAfter {
		cv.bmp.ReleasePixels()
	}
}

// bitmap is the cover's bitmap, nil until decoded (or for no cover).
func (cv *cover) bitmap() *ui.Bitmap {
	if cv == nil || cv.state != coverReady {
		return nil
	}
	return cv.bmp
}
