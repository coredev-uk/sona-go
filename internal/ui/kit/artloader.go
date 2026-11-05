package kit

import (
	"bytes"
	"cmp"
	"context"
	"image"
	_ "image/jpeg" // the backdrops
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
	// Backdrop is the pre-blurred backdrop JPEG of a template.
	Backdrop(ctx context.Context, template string) ([]byte, error)
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
// artwork cache (no HTTP of its own: the DTO's "/art/<key>/…" path names a
// key, and the cache downloads and sizes it) and decode it with
// ui.DecodeBitmap; the result reaches the main thread through the Poster,
// which redraws. Artwork asks for covers while it builds.
//
// Many covers are in flight at once: fetchWorkers fetches (a download is
// waiting, not working, and HTTP/2 carries them side by side) but at most one
// decode per CPU. The workers take the cover most wanted first (rank): the
// ones on screen, then the rest of the page, then what was only prefetched,
// each in the order the view asked; and a cover that the view stopped asking
// for, because it scrolled away or the page was left, is called off (see
// artloader_queue.go).
type ArtLoader struct {
	src  artSource
	post store.Poster
	now  func() time.Time

	// The main thread only.
	scale  float32
	known  bool // the scale is the window's, told or painted
	await  bool // something is waiting for known (a frame is due once it is)
	covers map[artKey]*cover
	clock  uint64              // advances with every ask, for dropping the oldest
	live   map[*cover]struct{} // covers queued or in flight

	jobs struct {
		sync.Mutex
		cond  *sync.Cond
		queue []*cover // unordered: pick takes the most wanted
		seq   uint64
		// onScreen counts the covers being worked on that were wanted for
		// the screen when they were taken.
		onScreen int
		closed   bool
		once     sync.Once
	}
	workers int
	decode  chan struct{}  // a slot per decode in flight
	running sync.WaitGroup // the workers
	ctx     context.Context
	cancel  context.CancelFunc
}

type artKey struct {
	src    string
	pw, ph int
}

type coverState uint8

const (
	coverIdle    coverState = iota // not asked for yet, or called off
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
	// tiny is a backdrop's pixels, kept (64 px square) for WashAt: the
	// bitmap lets go of its own once the GPU has them.
	tiny    image.Image
	fails   int
	retryAt time.Time
	shown   int // frames drawn, for ReleasePixels
	used    uint64
	view    *artWindow // the window whose build asked last

	// Under the loader's jobs lock: what the workers read while it is
	// queued.
	rank   artRank
	seq    uint64
	gen    uint64             // counts the requests made of the cover: an answer to an older one is dropped
	cancel context.CancelFunc // the request being worked on
}

// fetchWorkers is how many covers the app's loader has in flight at once.
// Most of that is waiting for the network, which HTTP/2 serves side by side;
// decoding is bounded apart (ArtLoader.decode).
const fetchWorkers = 16

// NewArtLoader returns a loader over the artwork cache that publishes
// through post (app's Poster).
func NewArtLoader(cache *artwork.Cache, post store.Poster) *ArtLoader {
	return newArtLoader(cache, post, fetchWorkers)
}

// newArtLoader is a loader with workers covers in flight at once and a
// decode per CPU.
func newArtLoader(src artSource, post store.Poster, workers int) *ArtLoader {
	if post == nil {
		post = func(fn func()) { fn() }
	}
	l := &ArtLoader{
		src: src, post: post, now: time.Now, scale: 1, covers: map[artKey]*cover{}, workers: workers,
		live:   map[*cover]struct{}{},
		decode: make(chan struct{}, runtime.GOMAXPROCS(0)),
	}
	l.ctx, l.cancel = context.WithCancel(context.Background())
	l.jobs.cond = sync.NewCond(&l.jobs.Mutex)
	return l
}

// SetScale tells the loader the window's device pixels per DIP before the
// first frame; Artwork learns it from painting otherwise (a first frame on
// a scaled screen asks for covers at scale 1, then again at the right
// size).
func (l *ArtLoader) SetScale(s float32) {
	if s > 0 {
		l.scale, l.known = s, true
	}
}

// noteScale is Artwork's paint callback: the window's scale is only known
// to a painter.
func (l *ArtLoader) noteScale(s float32) {
	if s <= 0 {
		return
	}
	changed := s != l.scale || (!l.known && l.await)
	l.scale, l.known, l.await = s, true, false
	if changed {
		l.post(func() {}) // a frame, which asks for the covers at the new size
	}
}

// scaleKnown reports whether the window's scale is known, and if not has the
// next paint that learns it ask for a frame: motion waits for it, as a request
// at the wrong size would be dropped and the video opened again.
func (l *ArtLoader) scaleKnown() bool {
	if !l.known {
		l.await = true
	}
	return l.known
}

// Close stops the workers; covers still queued are dropped.
func (l *ArtLoader) Close() {
	l.cancel()
	l.jobs.Lock()
	l.jobs.closed = true
	l.jobs.queue = nil
	l.jobs.cond.Broadcast()
	l.jobs.Unlock()
}

// Wait blocks until the workers have exited; call it after Close, from a
// goroutine that is not the main thread's if the Poster can block (tests'
// does not). The app does not need it: its process ends.
func (l *ArtLoader) Wait() { l.running.Wait() }

// cover is the cover of src at a box of w×h DIPs, asking for it the first
// time as view's build and as wanted as r says. nil when src names nothing.
func (l *ArtLoader) cover(v *artWindow, r artRank, src string, w, h float32) *cover {
	l.begin(v) // even for no cover: a pass of the view began
	if src == "" {
		return nil
	}
	return l.ask(v, artKey{src, devicePx(w, l.scale), devicePx(h, l.scale)}, r)
}

// Failed reports whether the cover of src at a box of w×h DIPs will never
// come: it could not be loaded after every retry, or src is not one of Sona's
// art URLs. It asks for the cover like Artwork does, so a picture that is
// still on its way is not failed. Main thread only.
func (l *ArtLoader) Failed(src string, w, h float32) bool {
	cv := l.cover(nil, rankBuilt, src, w, h)
	return cv != nil && cv.state == coverFailed
}

// backdrop is the pre-blurred backdrop of src (a DTO's
// "/art/<key>/backdrop.jpg" or a template: a few kilobytes of JPEG, decoded
// at its own size), asking for it the first time. nil when src names nothing.
func (l *ArtLoader) backdrop(c *ui.Context, src string) *cover {
	v := l.view(c)
	l.begin(v)
	if src == "" {
		return nil
	}
	return l.ask(v, artKey{src: src}, rankView) // no pixel size: a backdrop's own
}

// isBackdrop reports whether the key names a backdrop.
func (k artKey) isBackdrop() bool { return k.pw == 0 }

// ask is the cover of key, started when it is new (or waiting to be asked
// again). v is the window asking, nil for a caller outside a build, and r how
// much it is wanted; a cover already asked for this pass keeps the better of
// its rank and r.
func (l *ArtLoader) ask(v *artWindow, key artKey, r artRank) *cover {
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
	if v != nil {
		cv.view = v
	}
	switch cv.state {
	case coverIdle:
		cv.template = l.templateOf(key.src)
		if cv.template == "" {
			cv.state = coverFailed // not one of ours: the placeholder stays
			return cv
		}
		l.enqueue(cv, r)
	case coverQueued:
		l.want(cv, r)
	case coverWaiting:
		if !l.now().Before(cv.retryAt) {
			l.enqueue(cv, r)
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

func (l *ArtLoader) enqueue(cv *cover, r artRank) {
	cv.state = coverQueued
	l.live[cv] = struct{}{}
	l.jobs.once.Do(func() {
		for range l.workers {
			l.running.Go(l.work)
		}
	})
	l.jobs.Lock()
	l.jobs.seq++
	cv.rank, cv.seq = r, l.jobs.seq
	cv.gen++
	l.jobs.queue = append(l.jobs.queue, cv)
	l.jobs.cond.Signal()
	l.jobs.Unlock()
}

// load fetches a cover's JPEG and decodes it, a decode at a time per CPU. A
// backdrop's pixels are kept too (tiny).
func (l *ArtLoader) load(ctx context.Context, key artKey, template string) (*ui.Bitmap, image.Image, error) {
	var data []byte
	var err error
	if key.isBackdrop() {
		data, err = l.src.Backdrop(ctx, template)
	} else {
		data, err = l.src.Get(ctx, template, key.pw, key.ph)
	}
	if err != nil {
		return nil, nil, err
	}
	select {
	case l.decode <- struct{}{}:
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}
	defer func() { <-l.decode }()
	bmp, err := ui.DecodeBitmap(data)
	if err != nil || !key.isBackdrop() {
		return bmp, nil, err
	}
	tiny, _, _ := image.Decode(bytes.NewReader(data)) // nil on failure: the wash is then not sampled
	return bmp, tiny, nil
}

// finish publishes a decoded cover, on the main thread. gen is the request it
// answers: a cover called off, or asked for again, since is not its business.
func (l *ArtLoader) finish(cv *cover, gen uint64, bmp *ui.Bitmap, tiny image.Image, err error) {
	if gen != cv.gen {
		return
	}
	delete(l.live, cv)
	if err == nil {
		cv.bmp, cv.tiny, cv.state = bmp, tiny, coverReady
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
