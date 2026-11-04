// Package video hands decoded music video pictures from the playback
// engine to the UI. The engine shows each picture on a Surface the moment
// it is due (the CDM host child times them against the audio); the UI's
// video feed waits for the next one (Next) and draws it. Pictures live in
// memory only: the latest one, never a backlog, never on disk.
package video

import (
	"context"
	"sync"
	"time"
)

// Frame is one picture, its planes packed as I420 (Width×Height luma,
// then the two chroma planes of ⌈Width/2⌉×⌈Height/2⌉, U then V).
type Frame struct {
	Seq           uint64 // increases with every picture shown
	Width, Height int
	Position      time.Duration // on the item's timeline
	Data          []byte
}

// Surface holds the picture on show. The zero value is not usable; call
// NewSurface. Its methods are safe for concurrent use.
type Surface struct {
	mu   sync.Mutex
	seq  uint64
	cur  *Frame // nil when nothing is on show
	wake chan struct{}
}

// NewSurface returns an empty Surface.
func NewSurface() *Surface { return &Surface{wake: make(chan struct{})} }

// changedLocked wakes every waiter; with mu held.
func (s *Surface) changedLocked() {
	close(s.wake)
	s.wake = make(chan struct{})
}

// Show puts a picture on show. data is kept, not copied.
func (s *Surface) Show(w, h int, pos time.Duration, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	s.cur = &Frame{Seq: s.seq, Width: w, Height: h, Position: pos, Data: data}
	s.changedLocked()
}

// Clear takes the picture off show (the item playing has no video).
func (s *Surface) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cur == nil {
		return
	}
	s.seq++
	s.cur = nil
	s.changedLocked()
}

// Seq is the sequence number of the latest change of the picture on show (a
// picture shown or taken off): a reader that wants only pictures shown from
// now on waits after it (Next).
func (s *Surface) Seq() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.seq
}

// Next returns the picture on show once it is newer than after (a Seq
// the caller has), waiting until one is or ctx ends. It reports false when
// ctx ended first, or when the surface was cleared.
func (s *Surface) Next(ctx context.Context, after uint64) (Frame, bool) {
	for {
		s.mu.Lock()
		cur, seq, wake := s.cur, s.seq, s.wake
		s.mu.Unlock()
		if seq > after {
			if cur == nil {
				return Frame{Seq: seq}, false
			}
			return *cur, true
		}
		select {
		case <-wake:
		case <-ctx.Done():
			return Frame{}, false
		}
	}
}
