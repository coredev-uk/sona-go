// Package playback defines the contract between Sona's playback coordinator
// and an engine that renders one Apple Music track at a time.
//
// The coordinator owns the queue; an engine only loads, plays, pauses, seeks
// and reports on the current item. The production engine is
// internal/playback/native (Widevine CDM host + GStreamer).
package playback

import (
	"context"
	"time"
)

// State is an engine's playback state.
type State string

const (
	StateIdle    State = "idle"    // nothing loaded
	StateLoading State = "loading" // fetching playback info, licence or first segment
	StatePlaying State = "playing"
	StatePaused  State = "paused"
	StateEnded   State = "ended" // current item played to the end
	StateError   State = "error" // current item failed; see Status.Err
)

// Status is a snapshot of the engine. Position is exact at UpdatedAt; while
// State is StatePlaying, callers extrapolate (Position + time.Since(UpdatedAt))
// instead of polling.
type Status struct {
	State     State
	ItemID    string // Apple catalog song ID ("1440..."), or library ID ("i....")
	Position  time.Duration
	Duration  time.Duration
	Volume    float64 // 0..1
	UpdatedAt time.Time
	Err       string // set with StateError
}

// EventKind classifies an Event.
type EventKind string

const (
	EventState EventKind = "state" // State, Position or Volume changed
	EventItem  EventKind = "item"  // a new item became current
	EventError EventKind = "error" // playback failed; Status.Err has details
	EventAuth  EventKind = "auth"  // Apple rejected the credentials (401/403)
)

// Event is emitted on Engine.Events. Status is the snapshot after the change.
type Event struct {
	Kind   EventKind
	Status Status
}

// Engine renders one track at a time.
type Engine interface {
	// Play loads id and starts it at start, replacing the current item.
	Play(ctx context.Context, id string, start time.Duration) error
	// Preload prepares id (playback info, licence, first segment) so a later
	// Play starts without network round-trips. It never interrupts playback.
	Preload(ctx context.Context, id string) error
	Pause(ctx context.Context) error
	Resume(ctx context.Context) error
	Seek(ctx context.Context, pos time.Duration) error
	SetVolume(ctx context.Context, v float64) error
	// Stop unloads the current item and returns to StateIdle.
	Stop(ctx context.Context) error
	Status() Status
	// Events delivers changes until Close. Slow receivers miss
	// intermediate state events, never the latest one.
	Events() <-chan Event
	Close() error
}
