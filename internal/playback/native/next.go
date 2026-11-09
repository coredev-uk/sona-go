//go:build linux || (cgo && (darwin || windows))

package native

import (
	"context"
	"errors"
	"time"

	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/cdmhost"
)

// Seamless track changes. When the coordinator arms the next item
// (SetNext) and the current one has been fed to the host to its end, the
// engine prepares the next item (licence, first segment) and queues it in
// the host with Host.Next, then feeds it like the current one. The host
// joins the two sample-exact, or crossfades when crossfade is set and the
// next item is not Gapless (the following track of the same album), and
// reports the switch with EventAdvance, which the engine passes on as
// playback.EventAdvance. Any reload of the current item (seek, crash,
// Play, Stop) drops what was queued; it is queued again once the item is
// fed to the end once more.

// maxCrossfade bounds the crossfade setting.
const maxCrossfade = 12 * time.Second

// SetNext arms next to follow the current item without a gap; a zero Next
// disarms.
func (e *Engine) SetNext(next playback.Next) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.next == next {
		return
	}
	e.next = next
	if it := e.cur; it != nil && it.pending != nil {
		e.dropPendingLocked(it, true)
	}
	e.armLocked()
}

// SetCrossfade sets the crossfade into the next item, 0 (gapless) to 12 s.
// It applies from the next item queued.
func (e *Engine) SetCrossfade(d time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.crossfade = min(max(d, 0), maxCrossfade)
}

// fed records that track tok of it (current, or queued to follow) has been
// handed to the host to its end.
func (e *Engine) fed(it *item, tok uint64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if it.tok == tok {
		it.fed = true
		e.armLocked()
	}
}

// armLocked starts queueing the armed next item behind the current one
// once that is fed to its end. With mu held.
func (e *Engine) armLocked() {
	it := e.cur
	if e.closed || e.next.ID == "" || it == nil || !it.fed || it.pending != nil || it.m == nil || !it.m.trim || e.h == nil ||
		it.m.video != nil || playback.IsVideo(e.next.ID) { // a video starts and ends on its own
		return
	}
	ctx, cancel := context.WithCancel(e.bg)
	p := &item{id: e.next.ID, cancel: cancel}
	it.pending = p
	go e.queueNext(ctx, it, it.tok, p, e.next, e.h, e.hostGen)
}

// dropPendingLocked forgets it's pending next item; tell also drops it
// from the host. With mu held.
func (e *Engine) dropPendingLocked(it *item, tell bool) {
	p := it.pending
	if p == nil {
		return
	}
	it.pending = nil
	p.cancel()
	if tell && p.loaded && e.h != nil {
		h := e.h
		go func() {
			e.ctl.Lock()
			defer e.ctl.Unlock()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := h.Next(ctx, 0, cdmhost.Media{}, 0); err != nil && !errors.Is(err, cdmhost.ErrExited) {
				e.log.Debug("drop next item", "err", err)
			}
		}()
	}
}

// queueNext prepares p (armed as next) and queues it in host h behind
// track tok of it.
func (e *Engine) queueNext(ctx context.Context, it *item, tok uint64, p *item, next playback.Next, h host, gen int) {
	m, err := e.media(ctx, p.id, true)
	if err == nil {
		err = e.license(ctx, m, h, gen)
	}
	if err == nil && !m.trim {
		err = errors.New("native: a stream cannot follow without a gap")
	}
	if err != nil {
		_ = e.fail(p, err)
		return
	}
	e.ctl.Lock()
	defer e.ctl.Unlock()
	e.mu.Lock()
	if e.cur != it || it.tok != tok || it.pending != p || e.h != h {
		e.mu.Unlock()
		return
	}
	e.track++
	p.tok, p.m, p.wantPlay = e.track, m, true
	fade := time.Duration(0)
	if !next.Gapless {
		// Never longer than half of either track.
		fade = min(e.crossfade, it.m.duration/2, m.duration/2)
	}
	e.mu.Unlock()

	err = h.Next(ctx, p.tok, m.hostMedia(m.init), fade)
	e.mu.Lock()
	defer e.mu.Unlock()
	if it.pending != p {
		return
	}
	if err != nil {
		if errors.Is(err, cdmhost.ErrEnded) {
			// Too late: the current item ends normally.
			e.log.Debug("next item queued too late", "id", p.id)
		} else if ctx.Err() == nil {
			e.log.Warn("queue next item", "id", p.id, "err", err)
		}
		it.pending = nil
		p.cancel()
		return
	}
	p.loaded = true
	e.log.Debug("next item queued", "id", p.id, "crossfade", fade)
	go e.feed(ctx, p, p.tok, h, m, 0)
}

// advanceLocked makes it's pending item current after the host switched to
// it. With mu held.
func (e *Engine) advanceLocked(it *item, ev cdmhost.Event) {
	p := it.pending
	it.pending = nil
	if it.cancel != nil {
		it.cancel()
	}
	p.waiters = it.waiters
	it.waiters = nil
	e.cur = p // p.cancel stops its feed, which may still be running
	e.next = playback.Next{}
	e.st = playback.Status{
		State:     playback.StatePlaying,
		ItemID:    p.id,
		Position:  ev.Position,
		Duration:  p.m.duration,
		Volume:    e.st.Volume,
		UpdatedAt: ev.At,
		Preview:   e.preview,
	}
	e.emit(playback.EventAdvance)
	e.armLocked() // when the coordinator arms the one after
}
