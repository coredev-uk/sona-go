package cli

import (
	"reflect"
	"sync"
)

// Hub broadcasts values by topic to add-on sessions. A subscriber that
// hasn't taken its last value gets the next one instead, so a slow
// session sees the latest and Publish never waits for it. A value equal
// to the last one published on its topic is dropped.
type Hub struct {
	mu     sync.Mutex
	last   map[string]any
	subs   map[string]map[chan any]struct{}
	closed bool
}

// NewHub returns an empty hub.
func NewHub() *Hub {
	return &Hub{last: map[string]any{}, subs: map[string]map[chan any]struct{}{}}
}

// Publish sends v to topic's subscribers, unless it is what was published
// last.
func (h *Hub) Publish(topic string, v any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if prev, ok := h.last[topic]; h.closed || (ok && reflect.DeepEqual(prev, v)) {
		return
	}
	h.last[topic] = v
	for ch := range h.subs[topic] {
		select {
		case <-ch: // replace the value not yet taken
		default:
		}
		ch <- v
	}
}

// Watch delivers topic's last value at once, if there is one, then each
// change, until cancel (which closes the channel) or Close.
func (h *Hub) Watch(topic string) (<-chan any, func()) {
	ch := make(chan any, 1)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		close(ch)
		return ch, func() {}
	}
	if v, ok := h.last[topic]; ok {
		ch <- v
	}
	if h.subs[topic] == nil {
		h.subs[topic] = map[chan any]struct{}{}
	}
	h.subs[topic][ch] = struct{}{}
	return ch, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if _, ok := h.subs[topic][ch]; ok {
			delete(h.subs[topic], ch)
			close(ch)
		}
	}
}

// Close ends every subscription.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	for _, set := range h.subs {
		for ch := range set {
			close(ch)
		}
	}
	clear(h.subs)
}
