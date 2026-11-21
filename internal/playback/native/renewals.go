//go:build linux || (cgo && (darwin || windows))

package native

import (
	"sync"

	"github.com/coredev-uk/sona-go/internal/playback/cdmhost"
)

// renewals runs the licence renewals of one host's CDM messages one at a
// time, on one goroutine. A message for a session that already has one
// waiting replaces it (the CDM's newest request supersedes its older), so a
// child that floods messages costs at most one pending message per session
// (the caller admits only sessions the engine owns) and never more than one
// request to Apple at once.
type renewals struct {
	mu      sync.Mutex
	pending map[string]cdmhost.Event // session → its newest unanswered message
	wake    chan struct{}
	closed  chan struct{}
	once    sync.Once
}

func newRenewals() *renewals {
	return &renewals{pending: map[string]cdmhost.Event{}, wake: make(chan struct{}, 1), closed: make(chan struct{})}
}

// add queues ev, replacing the session's pending message.
func (r *renewals) add(ev cdmhost.Event) {
	r.mu.Lock()
	r.pending[ev.Session] = ev
	r.mu.Unlock()
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

// close stops run once the renewal in progress is done; the rest is dropped
// (the host has exited, and a renewal cannot reach it).
func (r *renewals) close() { r.once.Do(func() { close(r.closed) }) }

// run answers pending messages with do until close.
func (r *renewals) run(do func(cdmhost.Event)) {
	for {
		select {
		case <-r.closed:
			return
		case <-r.wake:
		}
		for {
			r.mu.Lock()
			var ev cdmhost.Event
			found := false
			for sid, p := range r.pending {
				ev, found = p, true
				delete(r.pending, sid)
				break
			}
			r.mu.Unlock()
			if !found {
				break
			}
			select {
			case <-r.closed:
				return
			default:
			}
			do(ev)
		}
	}
}
