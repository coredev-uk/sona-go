package uitest

import (
	"sync"
	"time"
)

// Clock is a clock that stands still until Advance: the playback mirror's
// extrapolated position and the fake engine follow it, so a test reads the
// same on a busy machine as on a fast one.
type Clock struct {
	mu sync.Mutex
	t  time.Time
}

// NewClock starts a clock at Epoch.
func NewClock() *Clock { return &Clock{t: Epoch} }

// Now is the clock's time.
func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

// Advance moves the clock on by d.
func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// MainThread is a store.Poster that runs what is posted where the test
// says: posts queue (from any goroutine, as service calls and cover decodes
// post) and Drain runs them on the caller's goroutine, as the app's main
// thread runs them. Nothing is lost and nothing runs concurrently with the
// view.
type MainThread struct {
	mu sync.Mutex
	q  []func()
}

// Post queues fn (the store.Poster).
func (m *MainThread) Post(fn func()) {
	m.mu.Lock()
	m.q = append(m.q, fn)
	m.mu.Unlock()
}

// Drain runs the queued functions, and those they queue, in order, and
// reports how many ran.
func (m *MainThread) Drain() int {
	n := 0
	for {
		m.mu.Lock()
		if len(m.q) == 0 {
			m.mu.Unlock()
			return n
		}
		fn := m.q[0]
		m.q = m.q[1:]
		m.mu.Unlock()
		fn()
		n++
	}
}

// Pending reports whether something is queued.
func (m *MainThread) Pending() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.q) > 0
}
