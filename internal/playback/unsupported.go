package playback

import (
	"context"
	"sync"
	"time"
)

// Unsupported is the Engine of a platform or build without a playback
// engine: every command fails with ErrUnsupported, it stays idle, sends no
// events and supports nothing.
type Unsupported struct {
	st        Status
	events    chan Event
	closeOnce sync.Once
}

var _ Engine = (*Unsupported)(nil)

// NewUnsupported returns an idle Unsupported engine.
func NewUnsupported() *Unsupported {
	return &Unsupported{
		st:     Status{State: StateIdle, Volume: 1, UpdatedAt: time.Now()},
		events: make(chan Event),
	}
}

// Play fails with ErrUnsupported.
func (*Unsupported) Play(context.Context, string, time.Duration) error { return ErrUnsupported }

// Preload fails with ErrUnsupported.
func (*Unsupported) Preload(context.Context, string) error { return ErrUnsupported }

// SetNext does nothing: there is never a current item to follow.
func (*Unsupported) SetNext(Next) {}

// Pause fails with ErrUnsupported.
func (*Unsupported) Pause(context.Context) error { return ErrUnsupported }

// Resume fails with ErrUnsupported.
func (*Unsupported) Resume(context.Context) error { return ErrUnsupported }

// Seek fails with ErrUnsupported.
func (*Unsupported) Seek(context.Context, time.Duration) error { return ErrUnsupported }

// SetVolume fails with ErrUnsupported.
func (*Unsupported) SetVolume(context.Context, float64) error { return ErrUnsupported }

// Stop fails with ErrUnsupported.
func (*Unsupported) Stop(context.Context) error { return ErrUnsupported }

// Status reports StateIdle at full volume, as of NewUnsupported.
func (u *Unsupported) Status() Status { return u.st }

// Events never delivers; Close closes it.
func (u *Unsupported) Events() <-chan Event { return u.events }

// Capabilities is the zero value: no engine, nothing supported.
func (*Unsupported) Capabilities() Capabilities { return Capabilities{} }

// Close closes Events. It is idempotent and returns nil.
func (u *Unsupported) Close() error {
	u.closeOnce.Do(func() { close(u.events) })
	return nil
}
