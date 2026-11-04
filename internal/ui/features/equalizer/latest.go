package equalizer

import "sync"

// latest sends values through send one at a time, off the caller's thread:
// values that arrive while one is being sent collapse into the newest,
// which always goes out last (v0.2.0's queries.ts `preview`). Errors are
// send's to ignore.
type latest[T any] struct {
	send func(T)

	mu      sync.Mutex
	pending *T
	sending bool
}

// Put queues v and starts the sender if none is running. Any goroutine.
func (l *latest[T]) Put(v T) {
	l.mu.Lock()
	l.pending = &v
	if l.sending {
		l.mu.Unlock()
		return
	}
	l.sending = true
	l.mu.Unlock()
	go l.run()
}

func (l *latest[T]) run() {
	for {
		l.mu.Lock()
		next := l.pending
		l.pending = nil
		if next == nil {
			l.sending = false
			l.mu.Unlock()
			return
		}
		l.mu.Unlock()
		l.send(*next)
	}
}
