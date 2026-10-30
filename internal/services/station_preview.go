package services

import (
	"context"
	"sync"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
)

// A track station's page lists the songs it will play first. Apple has no
// read-only view of them: next-tracks hands out the next batch and
// advances the station. So the page takes one batch and parks it here,
// and the station feed (playback_stations.go) plays a parked batch before
// asking Apple for more: what the page lists is what plays.

// previewTTL is how long a parked batch stays the station's next one.
const previewTTL = 15 * time.Minute

type stationPreview struct {
	songs []applemusic.Song
	at    time.Time
}

// stationPreviews are the parked batches by station ID.
type stationPreviews struct {
	mu sync.Mutex
	m  map[string]stationPreview
}

// get returns the parked batch of id, if it is fresh.
func (p *stationPreviews) get(id string, now time.Time) ([]applemusic.Song, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	v, ok := p.m[id]
	if !ok || now.Sub(v.at) > previewTTL {
		return nil, false
	}
	return v.songs, true
}

// take returns and removes the parked batch of id, if it is fresh.
func (p *stationPreviews) take(id string, now time.Time) ([]applemusic.Song, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	v, ok := p.m[id]
	delete(p.m, id)
	return v.songs, ok && now.Sub(v.at) <= previewTTL
}

// park keeps songs as id's next batch, unless a fresh one is parked
// already (two page loads racing); it returns the batch that is parked.
func (p *stationPreviews) park(id string, songs []applemusic.Song, now time.Time) []applemusic.Song {
	p.mu.Lock()
	defer p.mu.Unlock()
	if v, ok := p.m[id]; ok && now.Sub(v.at) <= previewTTL {
		return v.songs
	}
	if p.m == nil {
		p.m = map[string]stationPreview{}
	}
	for k, v := range p.m {
		if now.Sub(v.at) > previewTTL {
			delete(p.m, k)
		}
	}
	p.m[id] = stationPreview{songs: songs, at: now}
	return songs
}

func (p *stationPreviews) clear() {
	p.mu.Lock()
	p.m = nil
	p.mu.Unlock()
}

// stationPreview returns the batch station id plays first: the parked one,
// or a new batch from Apple, parked.
func (e *Env) stationPreview(ctx context.Context, id string, limit int) ([]applemusic.Song, error) {
	if songs, ok := e.previews.get(id, e.Now()); ok {
		return songs, nil
	}
	songs, err := e.API.NextTracks(ctx, id, limit)
	if err != nil {
		return nil, err
	}
	return e.previews.park(id, songs, e.Now()), nil
}

// nextStationTracks returns station id's next batch: the one its page
// parked, else a new one from Apple.
func (e *Env) nextStationTracks(ctx context.Context, id string, limit int) ([]applemusic.Song, error) {
	if songs, ok := e.previews.take(id, e.Now()); ok {
		return songs, nil
	}
	return e.API.NextTracks(ctx, id, limit)
}
