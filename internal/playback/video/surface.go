// Package video hands decoded music video pictures from the playback
// engine to the web view. The engine shows each picture on a Surface the
// moment it is due (the CDM host child times them against the audio);
// the web view asks for the next one at /video/frame (ServeHTTP) and draws
// it. Pictures live in memory only: the latest one, never a backlog, never
// on disk.
package video

import (
	"context"
	"encoding/binary"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Path is where the web view fetches pictures.
const Path = "/video/frame"

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

// headerSize is the size of the header before a picture's planes.
const headerSize = 16

// Header layout (little-endian): "SVF1", the sequence number (uint32,
// wrapping), width and height (uint16 each), the position in
// milliseconds (uint32).
var magic = [4]byte{'S', 'V', 'F', '1'}

// encodeHeader writes f's header into b (headerSize bytes).
func encodeHeader(b []byte, f Frame) {
	copy(b, magic[:])
	binary.LittleEndian.PutUint32(b[4:], uint32(f.Seq))
	binary.LittleEndian.PutUint16(b[8:], uint16(f.Width))
	binary.LittleEndian.PutUint16(b[10:], uint16(f.Height))
	binary.LittleEndian.PutUint32(b[12:], uint32(max(f.Position.Milliseconds(), 0)))
}

// poll is how long a request waits for a new picture before it answers
// 204 No Content (the client asks again).
const poll = time.Second

// ServeHTTP answers GET /video/frame?after=<seq>: the next picture newer
// than seq (the sequence number of the one the client drew; 0 or absent
// for any) as application/octet-stream, a 16-byte header then the I420
// planes; 204 when none came within a second, or the surface was cleared
// (X-Video-Seq then names the sequence to wait from).
func (s *Surface) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	after, _ := strconv.ParseUint(r.URL.Query().Get("after"), 10, 64)
	ctx, cancel := context.WithTimeout(r.Context(), poll)
	defer cancel()
	f, ok := s.Next(ctx, after)
	h := w.Header()
	h.Set("Cache-Control", "no-store")
	if !ok {
		if f.Seq != 0 {
			h.Set("X-Video-Seq", strconv.FormatUint(f.Seq, 10))
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	h.Set("Content-Type", "application/octet-stream")
	h.Set("X-Video-Seq", strconv.FormatUint(f.Seq, 10))
	h.Set("Content-Length", strconv.Itoa(headerSize+len(f.Data)))
	var hdr [headerSize]byte
	encodeHeader(hdr[:], f)
	_, _ = w.Write(hdr[:])
	_, _ = w.Write(f.Data)
}
