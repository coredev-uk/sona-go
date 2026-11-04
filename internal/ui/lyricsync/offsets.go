package lyricsync

import "sync"

// Offsets are the per-song lyrics offsets for this session (ms, by catalogue
// song ID; songs not listed are at 0). A misaligned lyrics file is a
// property of the song, so the offset stays with it: a new song starts in
// sync, and going back to a song keeps what was set for it. Nothing is
// stored across restarts. Every window shares one (Session), so the mini
// player's line keeps the timing set in the main window's Now Playing.
// Safe from any goroutine.
type Offsets struct {
	mu   sync.Mutex
	by   map[string]int64
	subs []func()
}

var session = &Offsets{}

// Session is the process's offsets.
func Session() *Offsets { return session }

// Get is the offset of songID (0 when none is set).
func (o *Offsets) Get(songID string) int64 {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.by[songID]
}

// Nudge moves songID's offset one step earlier (dir -1) or later (1).
func (o *Offsets) Nudge(songID string, dir int) {
	o.set(songID, func(v int64) int64 { return NudgeOffset(v, dir) })
}

// Reset puts songID back on its original timing.
func (o *Offsets) Reset(songID string) {
	o.set(songID, func(int64) int64 { return 0 })
}

// Clear forgets every offset (tests).
func (o *Offsets) Clear() {
	o.mu.Lock()
	o.by = nil
	o.mu.Unlock()
}

func (o *Offsets) set(songID string, f func(int64) int64) {
	o.mu.Lock()
	if o.by == nil {
		o.by = map[string]int64{}
	}
	o.by[songID] = f(o.by[songID])
	subs := append([]func(){}, o.subs...)
	o.mu.Unlock()
	for _, fn := range subs {
		fn()
	}
}

// OnChange calls fn after any change (from the goroutine that made it) and
// returns what stops that.
func (o *Offsets) OnChange(fn func()) (stop func()) {
	o.mu.Lock()
	o.subs = append(o.subs, fn)
	n := len(o.subs)
	o.mu.Unlock()
	return func() {
		o.mu.Lock()
		defer o.mu.Unlock()
		if n <= len(o.subs) {
			o.subs[n-1] = func() {}
		}
	}
}
