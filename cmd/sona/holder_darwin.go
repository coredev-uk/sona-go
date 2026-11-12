//go:build darwin

package main

import "sync"

// holder takes a power-management hold lazily and drops it again: the shape
// of the sleep inhibitor (inhibit_darwin.go) and the App Nap keeper
// (keeprunning_darwin.go).
type holder struct {
	// why is the reason the system lists, read when the hold is taken.
	why func() string
	// take takes the hold and returns what drops it.
	take func(reason string) (release func())

	mu      sync.Mutex
	release func() // nil when released
}

// hold takes the hold unless it is held.
func (h *holder) hold() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.release == nil {
		h.release = h.take(h.why())
	}
}

// drop releases the hold if it is held.
func (h *holder) drop() {
	h.mu.Lock()
	release := h.release
	h.release = nil
	h.mu.Unlock()
	if release != nil {
		release()
	}
}

// held reports whether the hold is taken.
func (h *holder) held() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.release != nil
}
