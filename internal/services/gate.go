package services

import (
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// Gate holds back network work until the UI has painted its first frame
// (SystemService.FirstPaint), so cold start reads only local stores. It
// opens on its own after a fallback delay in case the UI never
// reports a paint.
type Gate struct {
	log      *slog.Logger
	once     sync.Once
	firstReq sync.Once
	opened   chan struct{}
	timer    *time.Timer

	mu     sync.Mutex
	onOpen []func()
}

// NewGate returns a closed gate that opens by itself after fallback.
func NewGate(fallback time.Duration, log *slog.Logger) *Gate {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	g := &Gate{log: log, opened: make(chan struct{})}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.timer = time.AfterFunc(fallback, func() {
		if !g.IsOpen() {
			log.Warn("startup: no first paint reported; opening the network", "after", fallback.String())
		}
		g.Open()
	})
	return g
}

// Open opens the gate (idempotent) and runs the OnOpen callbacks once, in
// registration order, on the calling goroutine.
func (g *Gate) Open() {
	g.once.Do(func() {
		close(g.opened)
		g.mu.Lock()
		g.timer.Stop()
		fns := g.onOpen
		g.onOpen = nil
		g.mu.Unlock()
		for _, fn := range fns {
			fn()
		}
	})
}

// IsOpen reports whether the gate has opened.
func (g *Gate) IsOpen() bool {
	select {
	case <-g.opened:
		return true
	default:
		return false
	}
}

// OnOpen registers fn to run when the gate opens; if it is already open,
// fn runs now.
func (g *Gate) OnOpen(fn func()) {
	g.mu.Lock()
	if !g.IsOpen() {
		g.onOpen = append(g.onOpen, fn)
		g.mu.Unlock()
		return
	}
	g.mu.Unlock()
	fn()
}

// Transport wraps rt so requests wait for the gate (or their context).
func (g *Gate) Transport(rt http.RoundTripper) http.RoundTripper {
	return gatedTransport{g: g, rt: rt}
}

type gatedTransport struct {
	g  *Gate
	rt http.RoundTripper
}

func (t gatedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	waited := !t.g.IsOpen()
	select {
	case <-t.g.opened:
	case <-r.Context().Done():
		if r.Body != nil {
			r.Body.Close()
		}
		return nil, r.Context().Err()
	}
	t.g.firstReq.Do(func() {
		t.g.log.Info("startup: first network request", "host", r.URL.Host, "held_until_paint", waited)
	})
	return t.rt.RoundTrip(r)
}
