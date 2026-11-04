package store

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/coredev-uk/sona-go/internal/services"
)

var _ services.Emitter = (*Bus)(nil)

// Bus carries the events the Go services emit (services.Emitter) to the
// UI's typed subscribers. Events emitted before SetPoster are queued and
// delivered, in order, once it is set; after that each is delivered by the
// Poster, one at a time and in emission order, on the main thread.
type Bus struct {
	log *slog.Logger

	mu       sync.Mutex
	post     Poster
	flushing bool // SetPoster is still delivering the queue
	queue    []event
	subs     map[string][]*sub
	next     uint64
	warned   map[string]bool
}

type event struct {
	name string
	data []any
}

type sub struct {
	id   uint64
	live bool // false once off ran, even while an event is being delivered
	fn   func(data []any)
}

// NewBus returns a Bus that logs through log (nil: nowhere).
func NewBus(log *slog.Logger) *Bus {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Bus{log: log, subs: map[string][]*sub{}, warned: map[string]bool{}}
}

// SetPoster says how events reach the main thread. Events emitted before
// it are delivered now, in order, ahead of any emitted meanwhile.
func (b *Bus) SetPoster(p Poster) {
	if p == nil {
		p = runNow
	}
	b.mu.Lock()
	b.post = p
	b.flushing = true
	for {
		q := b.queue
		if len(q) == 0 {
			b.flushing = false
			b.mu.Unlock()
			return
		}
		b.queue = nil
		b.mu.Unlock()
		for _, e := range q {
			p(func() { b.deliver(e) })
		}
		b.mu.Lock()
	}
}

// Emit sends an event (services.Emitter). It is safe from any goroutine
// and reports whether a subscriber exists for name.
func (b *Bus) Emit(name string, data ...any) bool {
	e := event{name: name, data: data}
	b.mu.Lock()
	has := len(b.subs[name]) > 0
	p := b.post
	if p == nil || b.flushing {
		b.queue = append(b.queue, e)
		b.mu.Unlock()
		return has
	}
	b.mu.Unlock()
	p(func() { b.deliver(e) })
	return has
}

// deliver runs on the main thread.
func (b *Bus) deliver(e event) {
	b.mu.Lock()
	subs := append([]*sub(nil), b.subs[e.name]...)
	b.mu.Unlock()
	for _, s := range subs {
		b.mu.Lock()
		live := s.live
		b.mu.Unlock()
		if live {
			s.fn(e.data)
		}
	}
}

// On subscribes fn to the events called name, whose payload is a T. fn
// runs on the main thread. A payload of another type is logged once and
// dropped, so one mistyped emitter cannot panic the UI. The returned func
// unsubscribes; an event already being delivered skips a subscriber that
// went away.
func On[T any](b *Bus, name string, fn func(T)) (off func()) {
	s := &sub{live: true}
	s.fn = func(data []any) {
		var v T
		if len(data) > 0 {
			t, ok := data[0].(T)
			if !ok {
				b.warnMismatch(name, data[0], v)
				return
			}
			v = t
		}
		fn(v)
	}
	b.mu.Lock()
	b.next++
	s.id = b.next
	b.subs[name] = append(b.subs[name], s)
	b.mu.Unlock()
	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		s.live = false
		list := b.subs[name]
		for i, x := range list {
			if x == s {
				b.subs[name] = append(list[:i:i], list[i+1:]...)
				break
			}
		}
	}
}

func (b *Bus) warnMismatch(name string, got, want any) {
	key := fmt.Sprintf("%s|%T|%T", name, got, want)
	b.mu.Lock()
	seen := b.warned[key]
	b.warned[key] = true
	b.mu.Unlock()
	if !seen {
		b.log.Warn("event payload of another type dropped", "event", name, "got", fmt.Sprintf("%T", got), "want", fmt.Sprintf("%T", want))
	}
}
