//go:build linux || (darwin && cgo)

// Package native is Sona's CDM host playback engine, the Linux
// implementation of playback.Engine. The parent process (this package)
// does all networking: webPlayback metadata, the Widevine service
// certificate, licences, HLS playlists and encrypted segments. A cdmhost
// child process holds the Widevine CDM, decrypts, and plays the audio
// through GStreamer. The child is started on first use, shut down after an
// idle period, and restarted transparently after a crash. Music videos
// play the same way, their pictures decoded in the child and shown on a
// video.Surface (video.go). In preview mode (Options.Preview, preview.go)
// every song plays as its 30-second preview through a child that loads
// no CDM.
package native

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/cdmhost"
	"github.com/coredev-uk/sona-go/internal/playback/equalizer"
	"github.com/coredev-uk/sona-go/internal/playback/video"
	"github.com/coredev-uk/sona-go/internal/tunein"
)

// CDMSource names the Widevine CDM library each new playback host loads,
// and hears how loading it went (internal/playback/cdm.Manager).
type CDMSource interface {
	// Library returns the library to load now, or an error wrapping
	// playback.ErrNoCDM when none is ready. It may wait, within ctx, for
	// an install moments from finishing.
	Library(ctx context.Context) (string, error)
	// ReportInit reports a host's start with lib: err nil when the CDM
	// loaded, else the CDM's failure (cdmhost.ErrCDMLoad, or the host
	// dying while loading it). Other start failures are not reported.
	ReportInit(lib string, err error)
}

// CDMPath is a fixed CDM library (playspike's -cdm, tests).
type CDMPath string

// Library returns the path.
func (p CDMPath) Library(context.Context) (string, error) { return string(p), nil }

// ReportInit does nothing: there is nothing else to fall back to.
func (CDMPath) ReportInit(string, error) {}

// Options configures an Engine.
type Options struct {
	// Client makes every Apple request (webPlayback, certificate, licence).
	Client *applemusic.Client
	// HTTP fetches playlists and segments from Apple's CDN, which needs no
	// credentials. Nil uses a client with sane timeouts.
	HTTP *http.Client
	// CDM names the Widevine CDM each host loads (required unless
	// Preview).
	CDM    CDMSource
	Logger *slog.Logger
	// IdleTimeout ends the host process after this long idle, paused or
	// ended. Zero means 5 minutes; negative never.
	IdleTimeout time.Duration
	// ReadAhead is how many segments are fetched ahead of the one being
	// fed. Zero means 2.
	ReadAhead int
	// Quality is the AAC bitrate of the Widevine flavor to play; zero
	// means 256 kbps. SetQuality changes it for later items.
	Quality applemusic.Quality
	// Preview plays every song as Apple's 30-second preview (preview.go):
	// no CDM, no licence and no Music User Token, only the developer
	// token. Tests, CI and smoke runs use it (SONA_PREVIEW=1, playspike
	// -preview).
	Preview bool
	// Storefront returns the catalog storefront previews are looked up
	// in (required with Preview).
	Storefront func() string
	// Video is where music video pictures are shown (playback.VideoID
	// items); nil plays no videos (Capabilities.Video is false).
	Video *video.Surface
}

// Defaults for Options.
const (
	DefaultIdleTimeout = 5 * time.Minute
	defaultReadAhead   = 2
	maxCached          = 3                // prepared items kept (current + preloads)
	maxRestarts        = 3                // host crashes tolerated …
	restartWindow      = 2 * time.Minute  // … within this window
	startTimeout       = 20 * time.Second // Play/Resume wait for first audio
)

// host is the part of *cdmhost.Host the engine uses.
type host interface {
	Info() cdmhost.Info
	Events() <-chan cdmhost.Event
	Done() <-chan struct{}
	Err() error
	SetServerCertificate(ctx context.Context, cert []byte) error
	CreateSession(ctx context.Context, kid []byte) (string, []byte, error)
	UpdateSession(ctx context.Context, session string, license []byte) error
	CloseSession(ctx context.Context, session string) error
	Load(ctx context.Context, track uint64, m cdmhost.Media, start time.Duration) error
	Next(ctx context.Context, track uint64, m cdmhost.Media, fade time.Duration) error
	Feed(ctx context.Context, track uint64, fragment []byte) error
	FeedStream(ctx context.Context, track uint64, f cdmhost.StreamFragment) error
	EndOfTrack(ctx context.Context, track uint64) error
	Play(ctx context.Context) error
	Pause(ctx context.Context) (time.Duration, bool, error)
	Stop(ctx context.Context, track uint64) error
	SetVolume(ctx context.Context, v float64) error
	SetEqualizer(ctx context.Context, eq equalizer.Curve) error
	SetOutputDevice(ctx context.Context, device string) error
	LoadFile(ctx context.Context, track uint64, size int64, start time.Duration, gain float64) error
	SetTrackGain(ctx context.Context, on bool) error
	FeedFile(ctx context.Context, track uint64, offset int64, data []byte) error
	EndOfFile(ctx context.Context, track uint64) error
	Close() error
	LoadVideo(ctx context.Context, track uint64, m cdmhost.VideoMedia, start time.Duration) error
	FeedVideo(ctx context.Context, track uint64, fragment []byte) error
	EndOfVideo(ctx context.Context, track uint64) error
}

// Engine implements playback.Engine with the Widevine CDM host.
type Engine struct {
	api  *applemusic.Client
	http *http.Client
	// streamHTTP fetches broadcasters' streams: http's transport without
	// its overall timeout (a stream lasts for hours).
	streamHTTP *http.Client
	// tuneIn reads TuneIn's station profiles; tuneInID names the station
	// an asset URL plays there ("" for a station TuneIn does not serve).
	// Tests replace both.
	tuneIn   *tunein.Client
	tuneInID func(assetURL string) string
	// arts remembers the catalog artwork found for on-air songs.
	arts      onAirArts
	log       *slog.Logger
	idle      time.Duration
	readAhead int
	spawn     func(ctx context.Context, lib string) (host, error)
	cdm       CDMSource
	preview   bool          // Options.Preview
	sf        func() string // Options.Storefront
	// probe is cdmhost.Probe (Diagnose).
	probe func(ctx context.Context, o cdmhost.Options) (cdmhost.Info, error)
	// surface shows music video pictures; vtok is the track whose
	// pictures it takes (0: none), read by the host's reading goroutine.
	surface *video.Surface
	vtok    atomic.Uint64

	spawnMu sync.Mutex // serialises host starts
	ctl     sync.Mutex // orders control calls to the host (never held across network I/O)

	certMu  sync.Mutex
	cert    []byte // Widevine service certificate, cached for the process
	certURL string
	certGen int // host generation the certificate was installed on

	mu         sync.Mutex
	closed     bool
	st         playback.Status
	quality    applemusic.Quality // flavor for items prepared from now on
	eq         equalizer.Curve    // applied to every host
	device     string             // output device for every host (SetOutputDevice); "" = default
	soundCheck bool               // apply items' Sound Check gain
	h          host
	hostGen    int
	track      uint64        // last track generation handed to the host
	next       playback.Next // armed to follow cur seamlessly
	crossfade  time.Duration // into an armed next that is not Gapless
	cur        *item
	cache      map[string]*media
	sessions   map[string]*media // CDM session → media, for renewals
	idleT      *time.Timer
	restarts   []time.Time
	events     chan playback.Event

	retry  []time.Duration // waits between fetch attempts
	bg     context.Context // background work (prefetches); cancelled by Close
	stopBG context.CancelFunc
}

var _ playback.Engine = (*Engine)(nil)

// New returns an idle Engine. No process starts until Play or Preload.
func New(o Options) *Engine {
	log := o.Logger
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	hc := o.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	idle := o.IdleTimeout
	if idle == 0 {
		idle = DefaultIdleTimeout
	}
	ra := o.ReadAhead
	if ra <= 0 {
		ra = defaultReadAhead
	}
	q := o.Quality
	if q == 0 {
		q = applemusic.Quality256
	}
	e := &Engine{
		api:        o.Client,
		http:       hc,
		streamHTTP: &http.Client{Transport: hc.Transport, CheckRedirect: hc.CheckRedirect},
		tuneIn:     &tunein.Client{HTTP: hc, UserAgent: "Sona"},
		tuneInID:   tunein.StationID,
		log:        log.With("component", "playback"),
		idle:       idle,
		readAhead:  ra,
		quality:    q,
		st:         playback.Status{State: playback.StateIdle, Volume: 1, UpdatedAt: time.Now()},
		cache:      map[string]*media{},
		sessions:   map[string]*media{},
		events:     make(chan playback.Event, 32),
		retry:      retryDelays,
		probe:      cdmhost.Probe,
		cdm:        o.CDM,
		preview:    o.Preview,
		sf:         o.Storefront,
		surface:    o.Video,
	}
	e.bg, e.stopBG = context.WithCancel(context.Background())
	e.spawn = func(ctx context.Context, lib string) (host, error) {
		e.mu.Lock()
		eq, dev := e.eq, e.device
		e.mu.Unlock()
		return cdmhost.Start(ctx, cdmhost.Options{CDMPath: lib, Clear: e.preview, Logger: log, Device: dev, Equalizer: eq, OnVideoFrame: e.showFrame})
	}
	return e
}

// item is the current item.
type item struct {
	id       string
	tok      uint64 // track generation; changes on seek and host restarts
	m        *media
	wantPlay bool
	loaded   bool // the host has this item loaded
	fed      bool // every segment of tok was handed to the host
	// pending is the armed next item, being prepared for or queued in the
	// host to follow this one (next.go).
	pending *item
	cancel  context.CancelFunc
	waiters []chan error // Play/Resume calls waiting for tok to start, fail or pause
}

// waiter registers a wait for the item's next start; with mu held.
func (it *item) waiter() chan error {
	ch := make(chan error, 1)
	it.waiters = append(it.waiters, ch)
	return ch
}

// signal wakes every waiter; with mu held.
func (it *item) signal(err error) {
	for _, ch := range it.waiters {
		ch <- err
	}
	it.waiters = nil
}

// Host reports the running host process, if any.
func (e *Engine) Host() (cdmhost.Info, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.h == nil {
		return cdmhost.Info{}, false
	}
	return e.h.Info(), true
}

// SetQuality sets the AAC bitrate for the items played from now on. The
// playing item keeps its stream; prepared items of the other bitrate are
// prepared again when they are played.
func (e *Engine) SetQuality(q applemusic.Quality) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.quality = q
}

// Status returns the cached snapshot; it never blocks on the host.
func (e *Engine) Status() playback.Status {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.st
}

// Events delivers state changes until Close.
func (e *Engine) Events() <-chan playback.Event { return e.events }

// emit sends the current status; with mu held. A full channel drops its
// oldest event so the latest is never lost.
func (e *Engine) emit(kind playback.EventKind) {
	if e.closed {
		return
	}
	ev := playback.Event{Kind: kind, Status: e.st}
	for {
		select {
		case e.events <- ev:
			return
		default:
		}
		select {
		case <-e.events:
		default:
		}
	}
}

// setState updates the snapshot and emits; with mu held.
func (e *Engine) setState(s playback.State, pos time.Duration, at time.Time) {
	e.st.State, e.st.Position, e.st.UpdatedAt = s, pos, at
	if s != playback.StateError {
		e.st.Err = ""
	}
	e.emit(playback.EventState)
}

// position extrapolates the current position; with mu held.
func (e *Engine) position(now time.Time) time.Duration {
	p := e.st.Position
	if e.st.State == playback.StatePlaying {
		p += now.Sub(e.st.UpdatedAt)
	}
	if e.st.Duration > 0 && p > e.st.Duration {
		p = e.st.Duration
	}
	return max(p, 0)
}

var errClosed = errors.New("native: engine closed")

// Play loads id and plays it from start, replacing the current item. It
// returns once audio is playing (or the item was paused meanwhile). Errors
// from Apple wrap applemusic.ErrUnauthorized for 401/403.
func (e *Engine) Play(ctx context.Context, id string, start time.Duration) error {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return errClosed
	}
	e.clearVideo()
	e.replaceCurrent()
	it := &item{id: id, wantPlay: true}
	e.track++
	it.tok = e.track
	e.cur = it
	e.stopIdle()
	e.st = playback.Status{State: playback.StateLoading, ItemID: id, Position: max(start, 0), Volume: e.st.Volume, UpdatedAt: time.Now(), Preview: e.preview}
	if m := e.cache[id]; m != nil && m.done() && m.err == nil {
		e.st.Duration = m.duration
	}
	e.emit(playback.EventItem)
	e.mu.Unlock()
	return e.start(ctx, it, max(start, 0))
}

// replaceCurrent stops feeding the current item; with mu held.
func (e *Engine) replaceCurrent() {
	e.next = playback.Next{}
	if e.cur != nil {
		e.dropPendingLocked(e.cur, false)
		if e.cur.cancel != nil {
			e.cur.cancel()
		}
		e.cur.signal(context.Canceled)
		e.cur = nil
	}
}

// start prepares it (in parallel with starting the host), loads it at pos
// and, unless paused meanwhile, plays and waits for first audio.
func (e *Engine) start(ctx context.Context, it *item, pos time.Duration) error {
	t0 := time.Now()
	type hostResult struct {
		h   host
		gen int
		err error
	}
	hc := make(chan hostResult, 1)
	go func() {
		h, gen, err := e.ensureHost(ctx)
		hc <- hostResult{h, gen, err}
	}()
	e.prefetchCert()
	m, err := e.media(ctx, it.id, false)
	if err == nil && m.pl != nil {
		// The first segment downloads while the licence is negotiated.
		e.prefetch(m, m.startSegment(pos))
	}
	hr := <-hc
	if err == nil {
		err = hr.err
	}
	if err == nil {
		err = e.license(ctx, m, hr.h, hr.gen)
	}
	if err != nil {
		if ctx.Err() != nil {
			e.abandon(it, pos)
		}
		return e.fail(it, err)
	}
	prepared := time.Since(t0)

	e.mu.Lock()
	if e.cur != it {
		e.mu.Unlock()
		return fmt.Errorf("native: %s superseded: %w", it.id, context.Canceled)
	}
	it.m = m
	tok := it.tok
	e.dropPendingLocked(it, false) // a (re)load drops what the host had queued
	it.fed = false
	e.st.Duration = m.duration
	if m.duration > 0 && pos > m.duration {
		pos = m.duration
	}
	feedCtx, cancel := context.WithCancel(context.Background())
	if it.cancel != nil {
		it.cancel()
	}
	it.cancel = cancel
	var wait chan error
	if it.wantPlay {
		wait = it.waiter()
	}
	e.mu.Unlock()

	e.ctl.Lock()
	err = e.load(ctx, hr.h, tok, m, pos)
	e.mu.Lock()
	current := e.cur == it && it.tok == tok
	if err == nil && current {
		it.loaded = true
	}
	play := it.wantPlay
	e.mu.Unlock()
	if err == nil && current {
		e.startFeed(feedCtx, it, tok, hr.h, hr.gen, m, pos)
		if play {
			err = hr.h.Play(ctx)
		}
	}
	e.ctl.Unlock()
	if err != nil {
		if ctx.Err() != nil {
			e.abandon(it, pos)
		}
		return e.fail(it, err)
	}
	if !current {
		return fmt.Errorf("native: %s superseded: %w", it.id, context.Canceled)
	}
	if !play {
		return nil
	}
	select {
	case err := <-wait:
		if err != nil {
			return err
		}
		e.log.Debug("playing", "id", it.id, "at", pos, "prepare", prepared, "first_audio", time.Since(t0))
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(startTimeout):
		return e.fail(it, errors.New("native: no audio within "+startTimeout.String()))
	}
}

// abandon parks it as paused at pos after its caller gave up loading it,
// so Resume can pick it up again.
func (e *Engine) abandon(it *item, pos time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cur != it {
		return
	}
	it.wantPlay = false
	e.setState(playback.StatePaused, pos, time.Now())
	e.armIdle()
}

// fail records err for it (if still current) and returns it.
func (e *Engine) fail(it *item, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cur != it {
		if e.cur != nil && e.cur.pending == it {
			// The next item could not be prepared or fed: the current one
			// ends as usual and the coordinator starts it the slow way.
			e.log.Warn("next item not queued", "id", it.id, "err", err)
			e.dropPendingLocked(e.cur, true)
		}
		return err
	}
	if it.cancel != nil {
		it.cancel()
	}
	now := time.Now()
	e.st.State, e.st.Position, e.st.UpdatedAt, e.st.Err = playback.StateError, e.position(now), now, err.Error()
	e.emit(playback.EventError)
	if errors.Is(err, applemusic.ErrUnauthorized) {
		e.emit(playback.EventAuth)
	}
	it.signal(err)
	e.armIdle()
	e.log.Warn("playback failed", "id", it.id, "err", err)
	return err
}

// Preload prepares id: playback info, playlist, init segment, first
// segment and licence. It never touches the current item.
func (e *Engine) Preload(ctx context.Context, id string) error {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return errClosed
	}
	e.mu.Unlock()
	e.prefetchCert()
	m, err := e.media(ctx, id, true)
	if err == nil {
		var h host
		var gen int
		if h, gen, err = e.ensureHost(ctx); err == nil {
			err = e.license(ctx, m, h, gen)
		}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if err != nil && errors.Is(err, applemusic.ErrUnauthorized) {
		e.emit(playback.EventAuth)
	}
	e.armIdle()
	if err != nil {
		return fmt.Errorf("native: preload %s: %w", id, err)
	}
	return nil
}

// Pause pauses the current item; the position is exact at UpdatedAt.
func (e *Engine) Pause(ctx context.Context) error {
	e.ctl.Lock()
	defer e.ctl.Unlock()
	e.mu.Lock()
	it, h := e.cur, e.h
	if it == nil || !it.wantPlay {
		e.mu.Unlock()
		return nil
	}
	it.wantPlay = false
	loaded := it.loaded && h != nil
	e.mu.Unlock()

	pos, ok := time.Duration(0), false
	if loaded {
		var err error
		if pos, ok, err = h.Pause(ctx); err != nil && !errors.Is(err, cdmhost.ErrExited) {
			return err
		}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cur != it {
		return nil
	}
	now := time.Now()
	if !ok {
		pos = e.position(now)
	}
	if e.st.State != playback.StateEnded && e.st.State != playback.StateError {
		e.setState(playback.StatePaused, pos, now)
	}
	if it.m != nil && it.m.broadcast != nil {
		// A broadcast is not held while paused: Resume joins it live
		// again (restart), as a radio does.
		if it.cancel != nil {
			it.cancel()
		}
		it.loaded = false
	}
	it.signal(nil)
	e.armIdle()
	return nil
}

// Resume continues the current item, restarting the host first if it was
// shut down while idle.
func (e *Engine) Resume(ctx context.Context) error {
	e.mu.Lock()
	it, h := e.cur, e.h
	if it == nil {
		e.mu.Unlock()
		return errors.New("native: nothing to resume")
	}
	if it.wantPlay && e.st.State != playback.StateEnded {
		e.mu.Unlock()
		return nil
	}
	if e.st.State == playback.StateEnded || e.st.State == playback.StateError {
		// Replay from the start / retry, like a fresh Play of the item.
		pos := time.Duration(0)
		if e.st.State == playback.StateError {
			pos = e.st.Position
		}
		e.mu.Unlock()
		return e.Play(ctx, it.id, pos)
	}
	it.wantPlay = true
	e.stopIdle()
	if h == nil || !it.loaded {
		return e.restart(ctx, it)
	}
	wait := it.waiter()
	e.mu.Unlock()

	e.ctl.Lock()
	err := h.Play(ctx)
	e.ctl.Unlock()
	if err != nil {
		return e.fail(it, err)
	}
	select {
	case err := <-wait:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(startTimeout):
		return e.fail(it, errors.New("native: no audio within "+startTimeout.String()))
	}
}

// restart reloads it at its last position on a (new) host; with mu held,
// released before returning.
func (e *Engine) restart(ctx context.Context, it *item) error {
	pos := e.position(time.Now())
	e.track++
	it.tok, it.loaded = e.track, false
	if it.wantPlay {
		e.st.State, e.st.UpdatedAt = playback.StateLoading, time.Now()
		e.st.Position = pos
		e.emit(playback.EventState)
	}
	e.mu.Unlock()
	return e.start(ctx, it, pos)
}

// Seek moves the current item to pos, feeding from the segment that
// contains it; samples before pos are clipped by the pipeline.
func (e *Engine) Seek(ctx context.Context, pos time.Duration) error {
	e.ctl.Lock()
	defer e.ctl.Unlock()
	e.mu.Lock()
	it, h := e.cur, e.h
	if it == nil || it.m == nil {
		e.mu.Unlock()
		return errors.New("native: nothing to seek")
	}
	m := it.m
	if m.live() {
		e.mu.Unlock()
		return nil // live radio cannot seek
	}
	pos = max(pos, 0)
	if m.duration > 0 {
		pos = min(pos, m.duration)
	}
	gen := e.hostGen
	now := time.Now()
	if e.st.State == playback.StateEnded || e.st.State == playback.StateError {
		it.wantPlay = true
	}
	e.track++
	tok := e.track
	it.tok = tok
	// The host's Load drops the queued next item; it is queued again once
	// this item is fed to the end.
	e.dropPendingLocked(it, false)
	it.fed = false
	feedCtx, cancel := context.Background(), context.CancelFunc(func() {})
	if m.file == nil { // an upload's bytes keep flowing: the child seeks within them
		if it.cancel != nil {
			it.cancel()
		}
		feedCtx, cancel = context.WithCancel(context.Background())
		it.cancel = cancel
	}
	// Playing resumes with EventStarted once audio at pos is out.
	state := playback.StatePaused
	if it.wantPlay {
		state = playback.StateLoading
		e.stopIdle()
	}
	e.setState(state, pos, now)
	if h == nil || !it.loaded {
		// The host is gone: Resume (or crash recovery) reloads at pos.
		it.loaded = false
		play := it.wantPlay
		if play {
			e.ctl.Unlock()
			err := e.restart(ctx, it)
			e.ctl.Lock()
			return err
		}
		e.mu.Unlock()
		return nil
	}
	e.mu.Unlock()

	if err := h.Load(ctx, tok, m.hostMedia(nil), pos); err != nil {
		cancel()
		return e.fail(it, err)
	}
	if err := e.loadVideo(ctx, h, tok, m, pos); err != nil {
		cancel()
		return e.fail(it, err)
	}
	if m.file == nil {
		e.startFeed(feedCtx, it, tok, h, gen, m, pos)
	}
	if it.wantPlay {
		return h.Play(ctx)
	}
	return nil
}

// startFeed streams m from pos to the host for track tok.
func (e *Engine) startFeed(ctx context.Context, it *item, tok uint64, h host, gen int, m *media, pos time.Duration) {
	if m.file != nil {
		go e.feedFile(ctx, it, tok, h, m)
		return
	}
	if m.broadcast != nil {
		go e.feedBroadcast(ctx, it, tok, h, m)
		return
	}
	if m.stream != nil {
		go e.feedStream(ctx, it, tok, h, gen, m, pos)
		return
	}
	go e.feed(ctx, it, tok, h, m, m.pl.SegmentAt(pos))
	if m.video != nil {
		go e.feedVideo(ctx, it, tok, h, m, m.video.pl.SegmentAt(pos))
	}
}

// SetVolume sets the output volume (0..1, the slider position); the host
// applies playback.VolumeGain of it.
func (e *Engine) SetVolume(ctx context.Context, v float64) error {
	v = min(max(v, 0), 1)
	e.mu.Lock()
	e.st.Volume = v
	e.emit(playback.EventState)
	h := e.h
	e.mu.Unlock()
	if h != nil {
		return h.SetVolume(ctx, playback.VolumeGain(v))
	}
	return nil
}

// Stop unloads the current item.
func (e *Engine) Stop(ctx context.Context) error {
	e.ctl.Lock()
	defer e.ctl.Unlock()
	e.mu.Lock()
	e.replaceCurrent()
	e.clearVideo()
	e.track++
	tok := e.track
	h := e.h
	e.st = playback.Status{State: playback.StateIdle, Volume: e.st.Volume, UpdatedAt: time.Now()}
	e.emit(playback.EventState)
	e.armIdle()
	e.mu.Unlock()
	if h != nil {
		if err := h.Stop(ctx, tok); err != nil && !errors.Is(err, cdmhost.ErrExited) {
			return err
		}
	}
	return nil
}

// Close stops playback, ends the host process and closes Events.
func (e *Engine) Close() error {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return nil
	}
	e.replaceCurrent()
	e.closed = true
	e.stopBG()
	h := e.h
	e.h = nil
	if e.idleT != nil {
		e.idleT.Stop()
	}
	close(e.events)
	e.mu.Unlock()
	if h != nil {
		return h.Close()
	}
	return nil
}

// stopIdle cancels the idle shutdown; with mu held.
func (e *Engine) stopIdle() {
	if e.idleT != nil {
		e.idleT.Stop()
		e.idleT = nil
	}
}

// armIdle schedules the host's shutdown if nothing is playing; with mu held.
func (e *Engine) armIdle() {
	e.stopIdle()
	if e.idle < 0 || e.h == nil || e.closed {
		return
	}
	if e.cur != nil && e.cur.wantPlay && e.st.State != playback.StateEnded && e.st.State != playback.StateError {
		return
	}
	e.idleT = time.AfterFunc(e.idle, e.idleShutdown)
}

func (e *Engine) idleShutdown() {
	e.mu.Lock()
	h := e.h
	if h == nil || e.closed || (e.cur != nil && e.cur.wantPlay && e.st.State != playback.StateEnded && e.st.State != playback.StateError) {
		e.mu.Unlock()
		return
	}
	e.h = nil
	if e.cur != nil {
		if e.cur.cancel != nil {
			e.cur.cancel()
		}
		e.cur.loaded = false
	}
	e.mu.Unlock()
	e.log.Debug("ending idle playback host", "pid", h.Info().PID)
	h.Close()
}

// ensureHost returns the running host, starting one if needed.
func (e *Engine) ensureHost(ctx context.Context) (host, int, error) {
	e.spawnMu.Lock()
	defer e.spawnMu.Unlock()
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return nil, 0, errClosed
	}
	if e.h != nil {
		h, gen := e.h, e.hostGen
		e.mu.Unlock()
		return h, gen, nil
	}
	e.mu.Unlock()
	// The CDM is chosen per host: a version installed since the last one
	// started applies now, never to a running host. Previews need none.
	var lib string
	if !e.preview {
		var err error
		if lib, err = e.cdm.Library(ctx); err != nil {
			return nil, 0, fmt.Errorf("native: start playback host: %w", err)
		}
	}
	t0 := time.Now()
	h, err := e.spawn(ctx, lib)
	switch {
	case e.preview:
	case err == nil:
		e.cdm.ReportInit(lib, nil)
	case ctx.Err() == nil && (errors.Is(err, cdmhost.ErrCDMLoad) || errors.Is(err, cdmhost.ErrExited)):
		e.cdm.ReportInit(lib, err)
	}
	if err != nil {
		return nil, 0, fmt.Errorf("native: start playback host: %w", err)
	}
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		h.Close()
		return nil, 0, errClosed
	}
	e.hostGen++
	e.h = h
	gen, vol, eq, dev, sc := e.hostGen, e.st.Volume, e.eq, e.device, e.soundCheck
	e.mu.Unlock()
	info := h.Info()
	e.log.Info("playback host started", "pid", info.PID, "cdm", info.CDMVersion, "cdm_path", info.CDMPath,
		"preview", e.preview, "sink", info.Sink, "decoder", info.Decoder, "took", time.Since(t0))
	go e.watch(h, gen)
	if vol != 1 {
		_ = h.SetVolume(ctx, playback.VolumeGain(vol))
	}
	// The host started with the equaliser and device of when it was
	// spawned; re-send them in case they changed meanwhile (no-ops if not).
	if eq != (equalizer.Curve{}) {
		_ = h.SetEqualizer(ctx, eq)
	}
	if dev != "" {
		_ = h.SetOutputDevice(ctx, dev)
	}
	if sc {
		_ = h.SetTrackGain(ctx, true)
	}
	return h, gen, nil
}

// watch handles one host's events until it exits.
func (e *Engine) watch(h host, gen int) {
	for ev := range h.Events() {
		e.handle(h, gen, ev)
	}
	<-h.Done()
	e.hostExited(h, h.Err())
}

func (e *Engine) handle(h host, gen int, ev cdmhost.Event) {
	if ev.Kind == cdmhost.EventMessage {
		go e.renew(h, gen, ev)
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	it := e.cur
	if e.h != h || it == nil {
		return
	}
	if p := it.pending; p != nil && p.loaded && p.tok == ev.Track {
		switch ev.Kind {
		case cdmhost.EventAdvance:
			e.advanceLocked(it, ev)
		case cdmhost.EventError:
			e.log.Warn("next item failed in the host", "id", p.id, "err", ev.Err)
			e.dropPendingLocked(it, false) // the host dropped it already
		}
		return
	}
	if it.tok != ev.Track {
		return
	}
	if ev.Duration > 0 && it.m != nil && it.m.file != nil && !it.m.clip && ev.Duration != e.st.Duration {
		// An upload's length comes from the host's demuxer (a preview's
		// from its movie header: see media.clip).
		it.m.duration, e.st.Duration = ev.Duration, ev.Duration
		if ev.Kind == cdmhost.EventPosition {
			e.emit(playback.EventState)
		}
	}
	switch ev.Kind {
	case cdmhost.EventStarted:
		if it.wantPlay {
			e.setState(playback.StatePlaying, ev.Position, ev.At)
		}
		it.signal(nil)
	case cdmhost.EventPosition:
		if e.st.State != playback.StatePlaying {
			return
		}
		drift := ev.Position - (e.st.Position + ev.At.Sub(e.st.UpdatedAt))
		e.st.Position, e.st.UpdatedAt = ev.Position, ev.At
		if drift > 250*time.Millisecond || drift < -250*time.Millisecond {
			e.emit(playback.EventState)
		}
	case cdmhost.EventBuffering:
		if ev.Buffering {
			e.setState(playback.StateLoading, ev.Position, ev.At)
		} else if it.wantPlay {
			e.setState(playback.StatePlaying, ev.Position, ev.At)
		}
	case cdmhost.EventEnded:
		e.setState(playback.StateEnded, e.st.Duration, ev.At)
		// A start past the end ends at once: Play and Resume return then
		// rather than waiting for audio that never comes.
		it.signal(nil)
		e.armIdle()
	case cdmhost.EventError:
		// The child drops a failed pipeline and builds a new one on the next
		// Load (e.g. the PipeWire node went away), so reload like a crash.
		it.loaded = false
		if it.cancel != nil {
			it.cancel()
		}
		if !it.wantPlay {
			return // paused: Resume reloads
		}
		if e.recoverLocked(it) {
			e.log.Warn("playback pipeline failed; reloading", "item", it.id, "err", ev.Err)
			return
		}
		go e.fail(it, errors.New("native: "+ev.Err))
	}
}

// hostExited handles the end of a host process. An unexpected exit while
// an item is playing or loading restarts the host and resumes the item at
// its last position, at most maxRestarts times per restartWindow.
func (e *Engine) hostExited(h host, cause error) {
	e.mu.Lock()
	if e.h != h {
		e.mu.Unlock()
		return // closed on purpose
	}
	e.h = nil
	it := e.cur
	if it == nil {
		e.mu.Unlock()
		return
	}
	if it.cancel != nil {
		it.cancel()
	}
	it.loaded = false
	e.log.Warn("playback host exited", "err", cause, "item", it.id)
	if !it.wantPlay || e.st.State == playback.StateEnded || e.st.State == playback.StateError {
		e.mu.Unlock() // Resume or Play restarts it on demand
		return
	}
	if !e.recoverLocked(it) {
		now := time.Now()
		e.st.Err = fmt.Sprintf("playback host crashed %d times in %v: %v", maxRestarts+1, restartWindow, cause)
		e.setState(playback.StateError, e.position(now), now)
		e.emit(playback.EventError)
	}
	e.mu.Unlock()
}

// recoverLocked reloads it at its last position after a host crash or a
// pipeline failure, at most maxRestarts times per restartWindow; it reports
// false once that budget is spent. With mu held.
func (e *Engine) recoverLocked(it *item) bool {
	now := time.Now()
	recent := e.restarts[:0]
	for _, t := range e.restarts {
		if now.Sub(t) < restartWindow {
			recent = append(recent, t)
		}
	}
	e.restarts = recent
	if len(e.restarts) >= maxRestarts {
		return false
	}
	e.restarts = append(e.restarts, now)
	ctx, cancel := context.WithTimeout(context.Background(), startTimeout)
	go func() {
		defer cancel()
		e.mu.Lock()
		if e.cur != it || e.closed {
			e.mu.Unlock()
			return
		}
		_ = e.restart(ctx, it)
	}()
	return true
}
