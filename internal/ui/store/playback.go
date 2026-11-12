package store

import (
	"sync"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
)

// Engine states, mirroring Go's playback.State values.
const (
	StateIdle    = "idle"
	StateLoading = "loading"
	StatePlaying = "playing"
	StatePaused  = "paused"
	StateEnded   = "ended"
	StateError   = "error"
)

// EmptyState is the state before the coordinator says anything (v0.2.0's
// emptyState): idle, nothing queued, the volume at 0.8.
func EmptyState() services.PlaybackState {
	return services.PlaybackState{
		State:  StateIdle,
		Index:  -1,
		Volume: 0.8,
		Repeat: "off",
		Controls: services.PlaybackControls{
			Shuffle:  true,
			Repeat:   true,
			AutoPlay: true,
			Queue:    true,
		},
	}
}

// Playback is the UI's mirror of the playback coordinator. Go owns the
// queue and the engine and sends a snapshot only when something changes,
// so the UI derives the live position itself (Position) instead of
// polling. It follows services.EventPlaybackState and
// services.EventPlaybackQueue; Seed sets it from the services' snapshots
// before the first event.
type Playback struct {
	now func() time.Time

	mu    sync.Mutex
	state services.PlaybackState
	queue services.QueueState
	// unmuted is the volume to return to on unmute.
	unmuted float64
}

// NewPlayback mirrors b's playback events. now is the clock Position
// extrapolates by (nil: time.Now). Call it on the main thread, as On is.
func NewPlayback(b *Bus, now func() time.Time) *Playback {
	if now == nil {
		now = time.Now
	}
	p := &Playback{now: now, state: EmptyState(), queue: services.QueueState{Index: -1}, unmuted: DefaultUnmuted}
	On(b, services.EventPlaybackState, p.applyState)
	On(b, services.EventPlaybackQueue, p.applyQueue)
	return p
}

func (p *Playback) applyState(s services.PlaybackState) {
	p.mu.Lock()
	p.state = s
	p.mu.Unlock()
}

func (p *Playback) applyQueue(q services.QueueState) {
	p.mu.Lock()
	p.queue = q
	p.mu.Unlock()
}

// State is the latest snapshot of the now-playing state.
func (p *Playback) State() services.PlaybackState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state
}

// Queue is the latest queue; Go sends it when its version changes.
func (p *Playback) Queue() services.QueueState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.queue
}

// DefaultUnmuted is the volume an unmute returns to before the user has
// muted anything (v0.2.0's 0.8).
const DefaultUnmuted = 0.8

// Unmuted is the volume to return to on unmute: the last non-zero volume a
// mute left. The bar's button, the M key and the video view share it
// (v0.2.0's one `unmuted`).
func (p *Playback) Unmuted() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.unmuted
}

// SetUnmuted remembers v as the volume to return to on unmute; a zero or
// negative v (a mute remembered nothing) leaves it as it was.
func (p *Playback) SetUnmuted(v float64) {
	if v <= 0 {
		return
	}
	p.mu.Lock()
	p.unmuted = min(v, 1)
	p.mu.Unlock()
}

// Seed replaces the mirror with snapshots read from the services.
func (p *Playback) Seed(s services.PlaybackState, q services.QueueState) {
	p.mu.Lock()
	p.state, p.queue = s, q
	p.mu.Unlock()
}

// Patch changes the state locally while a command is in flight (a seek or
// a volume drag), until Go's next snapshot replaces it.
func (p *Playback) Patch(fn func(*services.PlaybackState)) {
	p.mu.Lock()
	fn(&p.state)
	p.mu.Unlock()
}

// Position is the playback position now: the snapshot's, plus the time
// since it was taken while playing, never before it (a clock behind Go's
// does not rewind) and never past the duration when that is known.
func (p *Playback) Position() time.Duration {
	p.mu.Lock()
	s := p.state
	p.mu.Unlock()
	return LivePosition(s, p.now())
}

// LivePosition is the position of a snapshot at now (v0.2.0's
// livePosition). PositionMs is exact at At, in Unix milliseconds.
func LivePosition(s services.PlaybackState, now time.Time) time.Duration {
	pos := s.PositionMs
	if s.State != StatePlaying {
		return time.Duration(pos) * time.Millisecond
	}
	pos += max(0, now.UnixMilli()-s.At)
	if s.DurationMs > 0 {
		pos = min(pos, s.DurationMs)
	}
	return time.Duration(pos) * time.Millisecond
}
