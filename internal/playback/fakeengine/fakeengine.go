// Package fakeengine is a playback.Engine that plays nothing and answers
// at once: every item "plays" for a fixed length, the position follows a
// clock, and the commands it was given are recorded. The UI tests
// (internal/ui/uitest) and the e2e smoke's probe build (cmd/sona, SONA_E2E)
// use it where the real engine would need Apple, a CDM and audio hardware.
// It never touches the network or an audio device.
package fakeengine

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/coredev-uk/sona-go/internal/playback"
)

// DefaultLength is how long an item plays.
const DefaultLength = 3 * time.Minute

// Engine is the fake. The zero value is not usable: use New.
type Engine struct {
	// Now is the clock the position follows; nil is time.Now. Set it
	// before the first command.
	Now func() time.Time
	// Length is how long each item plays; zero is DefaultLength.
	Length time.Duration

	mu     sync.Mutex
	st     playback.Status
	events chan playback.Event
	calls  []string
	plays  []string
}

var _ playback.Engine = (*Engine)(nil)

// New returns an idle engine at full volume.
func New() *Engine {
	return &Engine{
		st:     playback.Status{State: playback.StateIdle, Volume: 1},
		events: make(chan playback.Event, 1),
	}
}

func (e *Engine) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

func (e *Engine) length() time.Duration {
	if e.Length > 0 {
		return e.Length
	}
	return DefaultLength
}

// emit sends the status, replacing one nobody has read yet (latest wins, as
// the real engines' events do).
func (e *Engine) emit(kind playback.EventKind) {
	ev := playback.Event{Kind: kind, Status: e.st}
	select {
	case e.events <- ev:
	default:
		select {
		case <-e.events:
		default:
		}
		e.events <- ev
	}
}

// position is where the item is now.
func (e *Engine) position() time.Duration {
	p := e.st.Position
	if e.st.State == playback.StatePlaying {
		p += e.now().Sub(e.st.UpdatedAt)
	}
	return min(max(p, 0), e.st.Duration)
}

func (e *Engine) record(format string, args ...any) {
	e.calls = append(e.calls, fmt.Sprintf(format, args...))
}

// Play starts id at start.
func (e *Engine) Play(_ context.Context, id string, start time.Duration) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.record("play %s", id)
	e.plays = append(e.plays, id)
	e.st = playback.Status{
		State: playback.StatePlaying, ItemID: id, Position: start, Duration: e.length(),
		Volume: e.st.Volume, UpdatedAt: e.now(),
	}
	e.emit(playback.EventItem)
	return nil
}

// Preload records the request.
func (e *Engine) Preload(_ context.Context, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.record("preload %s", id)
	return nil
}

// SetNext does nothing: the item never ends by itself.
func (e *Engine) SetNext(playback.Next) {}

func (e *Engine) set(state playback.State, call string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.record("%s", call)
	e.st.Position, e.st.UpdatedAt, e.st.State = e.position(), e.now(), state
	e.emit(playback.EventState)
}

// Pause pauses the item.
func (e *Engine) Pause(context.Context) error { e.set(playback.StatePaused, "pause"); return nil }

// Resume resumes the item.
func (e *Engine) Resume(context.Context) error { e.set(playback.StatePlaying, "resume"); return nil }

// Seek moves the item to pos.
func (e *Engine) Seek(_ context.Context, pos time.Duration) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.record("seek %s", pos)
	e.st.Position, e.st.UpdatedAt = min(max(pos, 0), e.st.Duration), e.now()
	e.emit(playback.EventState)
	return nil
}

// SetVolume sets the level.
func (e *Engine) SetVolume(_ context.Context, v float64) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.record("volume %g", v)
	e.st.Volume = playback.ClampVolume(v)
	e.emit(playback.EventState)
	return nil
}

// Stop unloads the item.
func (e *Engine) Stop(context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.record("stop")
	e.st = playback.Status{State: playback.StateIdle, Volume: e.st.Volume, UpdatedAt: e.now()}
	e.emit(playback.EventState)
	return nil
}

// Status is the engine's state, the position as of now.
func (e *Engine) Status() playback.Status {
	e.mu.Lock()
	defer e.mu.Unlock()
	s := e.st
	s.Position = e.position()
	return s
}

// Events delivers a state change; it is never closed.
func (e *Engine) Events() <-chan playback.Event { return e.events }

// Capabilities are those of an engine with a volume.
func (*Engine) Capabilities() playback.Capabilities {
	return playback.Capabilities{Kind: playback.KindCDM, Volume: true}
}

// Close does nothing.
func (*Engine) Close() error { return nil }

// Played are the IDs Play was called with, in order.
func (e *Engine) Played() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return slices.Clone(e.plays)
}

// Calls are the commands received, in order, as "play <id>", "pause",
// "resume", "seek <pos>", "volume <v>", "stop" and "preload <id>".
func (e *Engine) Calls() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return slices.Clone(e.calls)
}
