package playlist

import (
	"slices"
	"sync"

	"github.com/coredev-uk/sona-go/internal/services"
)

// withMove is a playlist page with the track at from moved to to (what the
// user sees at once); out-of-range or equal positions change nothing.
func withMove(d services.PlaylistDetail, from, to int) services.PlaylistDetail {
	n := len(d.Tracks)
	if from == to || from < 0 || to < 0 || from >= n || to >= n {
		return d
	}
	tracks := slices.Clone(d.Tracks)
	moved := tracks[from]
	tracks = slices.Delete(tracks, from, from+1)
	tracks = slices.Insert(tracks, to, moved)
	d.Tracks = tracks
	return d
}

// withRemoved is a playlist page without the tracks at positions, with its
// duration and track count recomputed.
func withRemoved(d services.PlaylistDetail, positions []int) services.PlaylistDetail {
	drop := make(map[int]bool, len(positions))
	for _, p := range positions {
		drop[p] = true
	}
	tracks := make([]services.Song, 0, len(d.Tracks))
	var ms int64
	for i, t := range d.Tracks {
		if !drop[i] {
			tracks = append(tracks, t)
			ms += t.DurationMs
		}
	}
	d.Tracks, d.DurationMs = tracks, ms
	d.Playlist.TrackCount = len(tracks)
	return d
}

// edits runs a playlist's edits one after another, in the order they were
// made: each names positions in the state the previous one left, so Go must
// apply them in that order (edit.ts's serial and editing).
type edits struct {
	mu      sync.Mutex
	pending map[string]int
	tail    map[string]chan struct{}
}

// serial runs call after the playlist's earlier edits, on a goroutine, and
// hands its outcome to done (also on that goroutine; the caller posts it to
// the main thread) with later true when more edits of the playlist are
// still queued. The count drops before done runs, so done sees whether
// later edits are on their way.
func (e *edits) serial(id string, call func() (services.PlaylistDetail, error), done func(d services.PlaylistDetail, err error, later bool)) {
	e.mu.Lock()
	if e.pending == nil {
		e.pending, e.tail = map[string]int{}, map[string]chan struct{}{}
	}
	e.pending[id]++
	prev := e.tail[id]
	mine := make(chan struct{})
	e.tail[id] = mine
	e.mu.Unlock()
	go func() {
		if prev != nil {
			<-prev
		}
		d, err := call()
		e.mu.Lock()
		e.pending[id]--
		later := e.pending[id] > 0
		if !later {
			delete(e.pending, id)
			if e.tail[id] == mine {
				delete(e.tail, id)
			}
		}
		e.mu.Unlock()
		close(mine)
		done(d, err, later)
	}()
}

// editing reports whether an edit of the playlist is queued or on its way
// to Apple.
func (e *edits) editing(id string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.pending[id] > 0
}
