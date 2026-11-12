//go:build linux || (cgo && (darwin || windows))

package native

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/broadcast"
	"github.com/coredev-uk/sona-go/internal/tunein"
)

// Local broadcasters (Capital, KISS, …) are stations whose /v1/play/assets
// answer is one clear stream URL on the broadcaster's own servers, without
// a key server: no licence, and no CDM involved. The parent plays the
// stream (internal/playback/broadcast: HLS, PLS or Icecast) and feeds the
// elementary audio to the host as a live file (LoadFile with gst.Live),
// which decodes it like an upload; the host still makes no requests. What
// the stream says is on air becomes Status.OnAir once that audio is heard.
// A broadcast is live: no duration, no seeking, and a pause lets it go, so
// Resume joins it where it is now.

// broadcastMedia is a broadcaster's stream: the URL Apple named for it.
type broadcastMedia struct {
	url      string
	prepared time.Time // the URL may carry session parameters; see liveStale
	// tuneIn is the station's TuneIn ID when Apple serves it through
	// TuneIn ("" otherwise).
	tuneIn string
}

func (e *Engine) prepareBroadcast(m *media, url string) error {
	if url == "" {
		return fmt.Errorf("native: %s: broadcaster asset without a URL: %w", m.id, applemusic.ErrNotPlayable)
	}
	m.broadcast = &broadcastMedia{url: url, prepared: time.Now(), tuneIn: e.tuneInID(url)}
	e.log.Info("prepared broadcast", "id", m.id)
	return nil
}

// feedBroadcast plays broadcast m into the host as live track tok until
// ctx ends or the stream does (the track then ends once it has played).
// What is on air is announced until ctx ends or the track is replaced.
func (e *Engine) feedBroadcast(ctx context.Context, it *item, tok uint64, h host, m *media) {
	s := &broadcastSink{e: e, it: it, tok: tok, h: h, wake: make(chan struct{}, 1)}
	go s.announce(ctx)
	if id := m.broadcast.tuneIn; id != "" {
		go s.watchTuneIn(ctx, id)
	}
	err := broadcast.Play(ctx, broadcast.Options{HTTP: e.streamHTTP, UserAgent: applemusic.UserAgent, Logger: e.log}, m.broadcast.url, s)
	if err == nil {
		err = h.EndOfFile(ctx, tok)
	}
	if err != nil {
		e.feedFailed(ctx, it, err)
	}
}

// broadcastSink feeds a broadcast to the host and announces what is on
// air as playback reaches it.
type broadcastSink struct {
	e   *Engine
	it  *item
	tok uint64
	h   host
	off int64 // bytes fed

	mu      sync.Mutex
	pending []timedOnAir
	wake    chan struct{}

	// heard: the stream itself has named something on air, which then
	// stands in preference to TuneIn's.
	heard atomic.Bool
}

type timedOnAir struct {
	at time.Duration
	m  playback.OnAir
}

func (s *broadcastSink) Audio(ctx context.Context, b []byte) (err error) {
	s.off, err = feedChunks(ctx, s.h, s.tok, s.off, b)
	return err
}

func (s *broadcastSink) Meta(at time.Duration, m playback.OnAir) {
	if m != (playback.OnAir{}) {
		s.heard.Store(true)
	}
	s.mu.Lock()
	s.pending = append(s.pending, timedOnAir{at, m})
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// onAirPoll bounds how long the announcer waits before it looks at the
// position again (playback may pause, buffer or end meanwhile).
const onAirPoll = time.Second

// announce makes each pending OnAir the status once playback has reached
// the stream time it applies from, until ctx ends or the track is gone.
func (s *broadcastSink) announce(ctx context.Context) {
	for {
		s.mu.Lock()
		var next *timedOnAir
		if len(s.pending) > 0 {
			next = &s.pending[0]
		}
		s.mu.Unlock()
		wait := onAirPoll
		if next != nil {
			pos, ok := s.e.trackPosition(s.it, s.tok)
			if !ok {
				return
			}
			if pos >= next.at {
				s.e.setOnAir(s.it, s.tok, next.m)
				go s.addArt(ctx, next.m)
				s.mu.Lock()
				s.pending = s.pending[1:]
				s.mu.Unlock()
				continue
			}
			wait = min(next.at-pos, onAirPoll)
		}
		select {
		case <-ctx.Done():
			return
		case <-s.wake:
		case <-time.After(wait):
		}
	}
}

// addArt gives the song m, once it is on air, its catalog artwork.
func (s *broadcastSink) addArt(ctx context.Context, m playback.OnAir) {
	if art := s.e.onAirArt(ctx, m); art != "" {
		s.e.setOnAirArt(s.it, s.tok, m, art)
	}
}

// watchTuneIn reads station id's TuneIn profile: always once, for what
// Now Playing shows about the station, and then, for as long as the
// stream names nothing on air itself, for the song TuneIn says is playing
// (most stations feed TuneIn none). Failures back off (tunein.Watcher).
func (s *broadcastSink) watchTuneIn(ctx context.Context, id string) {
	var last playback.OnAir // what TuneIn last put on air
	w := &tunein.Watcher{
		Client: s.e.tuneIn,
		ID:     id,
		Want:   func() bool { return !s.heard.Load() },
		Emit: func(st tunein.Station) {
			s.e.setInfo(s.it, s.tok, playback.StationInfo{
				Description: st.Description, Genre: st.Genre, Location: st.Location, Website: st.Website,
			})
			if s.heard.Load() {
				return
			}
			m := playback.OnAir{Artist: st.Artist, Title: st.Title}
			if m == last {
				return
			}
			last = m
			s.e.setOnAir(s.it, s.tok, m)
			if m != (playback.OnAir{}) {
				go s.addArt(ctx, m)
			}
		},
	}
	if err := w.Watch(ctx); err != nil && ctx.Err() == nil {
		s.e.log.Debug("tunein", "err", err)
	}
}

// trackPosition returns the position of it while track tok of it is
// current; ok is false once it is not.
func (e *Engine) trackPosition(it *item, tok uint64) (time.Duration, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cur != it || it.tok != tok {
		return 0, false
	}
	return e.st.PositionAt(time.Now()), true
}

// setOnAir makes m what is on air while track tok of it is current.
func (e *Engine) setOnAir(it *item, tok uint64, m playback.OnAir) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cur != it || it.tok != tok || sameSong(e.st.OnAir, m) {
		return
	}
	e.st.OnAir = m
	e.log.Debug("on air", "id", it.id, "title", m.Title, "artist", m.Artist)
	e.emit(playback.EventState)
}

// sameSong reports whether a and b name the same song (the artwork found
// for it is not part of the name).
func sameSong(a, b playback.OnAir) bool { return a.Title == b.Title && a.Artist == b.Artist }

// setOnAirArt adds art to what is on air while that is still the song m.
func (e *Engine) setOnAirArt(it *item, tok uint64, m playback.OnAir, art string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cur != it || it.tok != tok || !sameSong(e.st.OnAir, m) || e.st.OnAir.Art == art {
		return
	}
	e.st.OnAir.Art = art
	e.emit(playback.EventState)
}

// setInfo makes info what is known about the broadcaster while track tok
// of it is current.
func (e *Engine) setInfo(it *item, tok uint64, info playback.StationInfo) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cur != it || it.tok != tok || e.st.Info == info {
		return
	}
	e.st.Info = info
	e.emit(playback.EventState)
}
