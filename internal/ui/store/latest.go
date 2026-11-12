package store

import "sync"

// Latest sends values through send one at a time, off the caller's thread:
// values that arrive while one is being sent collapse into the newest,
// which always goes out last (v0.2.0's queries.ts `preview`). Errors are
// send's to ignore.
type Latest[T any] struct {
	// Send sends one value; called on its own goroutine, never twice at once.
	Send func(T)

	mu      sync.Mutex
	pending *T
	sending bool
}

// Put queues v and starts the sender if none is running. Any goroutine.
func (l *Latest[T]) Put(v T) {
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

func (l *Latest[T]) run() {
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
		l.Send(*next)
	}
}
