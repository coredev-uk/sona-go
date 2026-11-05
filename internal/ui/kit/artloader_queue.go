package kit

import (
	"context"
	"slices"

	"github.com/egoist/mygo/ui"
)

// artRank is what a cover is wanted for, which decides the order the workers
// take them in: the smallest first.
type artRank uint8

const (
	// rankView: on screen, as the last frame laid it out.
	rankView artRank = iota
	// rankBuilt: built into the view but not known to be on screen: the
	// page below the fold, the rows a long list builds beyond its window, a
	// first frame the layout has not placed yet.
	rankBuilt
	// rankAhead: asked for before the view needs it (PrefetchArtwork). It
	// never starts while a cover on screen is being worked on.
	rankAhead
	// rankStale: not asked again since the last build pass began. It is
	// called off when that pass has been painted, unless a later ask comes
	// first, and like rankAhead it waits for the covers on screen.
	rankStale
)

// artWindow is what the loader knows of one window's build passes, which is
// how a cover that left the screen is told from one still wanted. A pass is
// the builds from one paint to the next: every ask of the pass marks its
// cover as wanted, and the covers of the window that were not asked by the
// time the pass is painted are no longer wanted. Nothing ends a cover's life
// but a pass that was painted without asking for it, so a window that builds
// nothing (or stops painting) leaves its covers loading.
type artWindow struct {
	painted bool // the last pass has been painted: the next ask begins another
	// scale is the window's own device pixels per DIP, learned from its paints
	// (ArtLoader.scaleOf).
	scale pixelScale
	// since is the loader's clock when the current pass began: a cover asked
	// for after it is wanted by the pass, and trim never drops it.
	since uint64
}

// artWindowKey keys a loader's state of a window on the window's root element.
type artWindowKey struct{ l *ArtLoader }

// view is the state of the window c builds, nil for no window. It lives as long
// as the window's root element.
func (l *ArtLoader) view(c *ui.Context) *artWindow {
	if c == nil {
		return nil
	}
	return ui.Local(c.Root(), artWindowKey{l}, func() artWindow { return artWindow{} })
}

// Pass is the app's hook for a window's root view: call it first thing in
// the build, and call the func it returns from the root element's Draw. It
// tells the loader where the window's build passes begin and end, so that the
// covers a page stops asking for are called off even when the page that
// follows builds no cover at all. Without it a pass begins with the window's
// next Artwork and ends when one is painted.
func (l *ArtLoader) Pass(c *ui.Context) (end func(*ui.Painter)) {
	if l == nil {
		return func(*ui.Painter) {}
	}
	v := l.view(c)
	l.begin(v)
	return func(p *ui.Painter) { l.painted(v, p.Scale()) }
}

// begin starts a build pass of v if the last one was painted: its covers still
// in flight are stale until the pass asks for them again.
func (l *ArtLoader) begin(v *artWindow) {
	if v == nil || !v.painted {
		return
	}
	v.painted = false
	v.since = l.clock
	if len(l.live) == 0 {
		return
	}
	l.jobs.Lock()
	for cv := range l.live {
		if cv.view == v {
			cv.rank = rankStale
		}
	}
	l.jobs.Unlock()
}

// want raises a queued or running cover to r, if r is more than it has.
func (l *ArtLoader) want(cv *cover, r artRank) {
	if r >= cv.rank {
		return
	}
	l.jobs.Lock()
	cv.rank = r
	l.jobs.cond.Signal() // a worker that waited for it to be worth starting
	l.jobs.Unlock()
}

// painted is the paint callback of what a pass built: the window's scale,
// and the end of the pass, when what it no longer asks for is called off.
// All of it is called off under one hold of the jobs lock: cancelling the
// cover in flight frees its worker, which would otherwise take the next
// cover of the pass before its turn to be called off came, and start a
// request only to cancel it.
func (l *ArtLoader) painted(v *artWindow, scale float32) {
	l.noteScale(v, scale)
	if v == nil || v.painted {
		return
	}
	v.painted = true
	l.jobs.Lock()
	defer l.jobs.Unlock()
	for cv := range l.live {
		if cv.view == v && cv.rank == rankStale {
			l.callOff(cv)
		}
	}
}

// callOff stops working on a cover nobody asks for: out of the queue if it
// is waiting, its context cancelled if a worker has it (the artwork cache
// stops downloading it unless another cover shares the source). A later ask
// starts it again; an answer to the old request is dropped. The jobs lock
// is held.
func (l *ArtLoader) callOff(cv *cover) {
	if i := slices.Index(l.jobs.queue, cv); i >= 0 {
		l.jobs.queue = slices.Delete(l.jobs.queue, i, i+1)
	} else if cv.cancel != nil {
		cv.cancel()
	}
	cv.gen++
	cv.rank = rankBuilt
	cv.state = coverIdle
	delete(l.live, cv)
}

// job is a request a worker took.
type job struct {
	cv       *cover
	gen      uint64
	key      artKey
	template string
	ctx      context.Context
	cancel   context.CancelFunc
	onScreen bool // it was wanted for the screen when taken: see ArtLoader.jobs.onScreen
}

// pick is the index in the queue of the cover to start next: the most wanted,
// the earliest asked of its rank; -1 for none, which is also the answer while
// only covers nobody needs yet are queued and a cover on screen is being
// worked on, so that prefetching does not take the network or the CPUs from
// what the user is waiting for. The jobs lock is held.
func (l *ArtLoader) pick() int {
	best := -1
	for i, cv := range l.jobs.queue {
		if best < 0 {
			best = i
			continue
		}
		b := l.jobs.queue[best]
		if cv.rank < b.rank || cv.rank == b.rank && cv.seq < b.seq {
			best = i
		}
	}
	if best >= 0 && l.jobs.queue[best].rank >= rankAhead && l.jobs.onScreen > 0 {
		return -1
	}
	return best
}

// take blocks for the cover to start next (pick); false when the loader is
// closed.
func (l *ArtLoader) take() (job, bool) {
	l.jobs.Lock()
	defer l.jobs.Unlock()
	best := l.pick()
	for best < 0 && !l.jobs.closed {
		l.jobs.cond.Wait()
		best = l.pick()
	}
	if l.jobs.closed {
		return job{}, false
	}
	cv := l.jobs.queue[best]
	l.jobs.queue = slices.Delete(l.jobs.queue, best, best+1)
	j := job{cv: cv, gen: cv.gen, key: cv.key, template: cv.template, onScreen: cv.rank == rankView}
	if j.onScreen {
		l.jobs.onScreen++
	}
	j.ctx, j.cancel = context.WithCancel(l.ctx)
	cv.cancel = j.cancel
	return j, true
}

// done is a worker finishing a job: what it held back is worth starting if it
// was the last cover on screen.
func (l *ArtLoader) done(j job) {
	j.cancel()
	if !j.onScreen {
		return
	}
	l.jobs.Lock()
	l.jobs.onScreen--
	l.jobs.cond.Broadcast()
	l.jobs.Unlock()
}

// work is a worker: it takes the most wanted request, fetches and decodes it.
func (l *ArtLoader) work() {
	for {
		j, ok := l.take()
		if !ok {
			return
		}
		d, err := l.load(j.ctx, j.key, j.template)
		l.done(j)
		l.post(func() { l.finish(j.cv, j.gen, d, err) })
	}
}
