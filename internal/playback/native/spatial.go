//go:build linux || (cgo && (darwin || windows))

package native

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/fmp4"
	"github.com/coredev-uk/sona-go/internal/playback/hls"
	"github.com/coredev-uk/sona-go/internal/playback/soundcheck"
)

// Spatial Audio. A song with the "spatial" or "atmos" audio trait has an
// enhancedHls master playlist (the catalogue's extendedAssetUrls) with
// Apple's binaural AAC rendition of its Dolby Atmos mix: AAC-LC at 256
// kbit/s, or HE-AAC at 64, 48 kHz, cbcs, in 15-second byte-range segments.
// For most songs Apple issues Sona's Widevine CDM its keys, so it plays
// like any other AAC stream through the CDM host. The first segment uses
// one key (s1/e1) and the rest another (c24); each has its own CDM session,
// licence and renewals, and each segment is fed with its key's KID because
// the init segment's default KID is zero. A song with no Widevine key
// (FairPlay only), or anything else that goes wrong before the first
// sample, plays the stereo stream instead (prepareSpatial's callers).

// The binaural renditions of an enhancedHls master.
const (
	spatialRendition256 = "audio-stereo-256-binaural"
	spatialRendition64  = "audio-HE-stereo-64-binaural"
)

// spatialRendition is the rendition quality q plays.
func spatialRendition(q applemusic.Quality) string {
	if q == applemusic.Quality64 {
		return spatialRendition64
	}
	return spatialRendition256
}

// hasSpatialTrait reports whether a song's audio traits say it has a
// Spatial Audio mix.
func hasSpatialTrait(traits []string) bool {
	return slices.Contains(traits, "spatial") || slices.Contains(traits, "atmos")
}

// spatialMedia is the part of a prepared media that plays a binaural
// rendition (media.spatial).
type spatialMedia struct {
	rendition string
	// keys are the playlist's distinct Widevine keys, in playlist order;
	// each has a CDM session (media.keySessions, by the key's URI).
	keys []*hls.Key
	// slots are the licence slot tokens by session, for renewals; with
	// media.licMu held.
	slots map[string]string
}

// spatialFallback is why a song plays its stereo stream although Spatial
// Audio is on.
type spatialFallback struct {
	reason string
	// code is the playback.Status.SpatialMiss code the UI shows; "" where
	// nothing is known of a mix (the lookup failed, or the song has no
	// Spatial Audio or Atmos trait): the UI says nothing then.
	code string
	err  error
	// quiet: nothing is wrong, the song just has no Spatial Audio mix
	// (logged at debug level).
	quiet bool
}

func (f *spatialFallback) Error() string {
	if f.err == nil {
		return f.reason
	}
	return f.reason + ": " + f.err.Error()
}

func (f *spatialFallback) Unwrap() error { return f.err }

// fallback is the error that sends a song to its stereo stream, unless err
// shows the caller gave up or Apple rejected the account, which the stereo
// stream would meet too.
func fallback(ctx context.Context, reason string, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, applemusic.ErrUnauthorized) {
		return err
	}
	return &spatialFallback{reason: reason, code: playback.SpatialUnavailable, err: err}
}

// lookupFailed is the fallback for a catalogue lookup that failed: the
// song's traits are unknown, so nothing is said of a mix it may not have.
func lookupFailed(ctx context.Context, err error) error {
	switch err := fallback(ctx, "the catalogue lookup failed", err).(type) {
	case *spatialFallback:
		err.code = ""
		return err
	default:
		return err
	}
}

// refused is the fallback for a licence the server refused (Apple keeps the
// mix for its own apps) or one that failed otherwise.
func refused(ctx context.Context, err error) error {
	var le *applemusic.LicenseError
	if errors.As(err, &le) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return &spatialFallback{reason: "a licence was refused", code: playback.SpatialAppleOnly, err: err}
	}
	return fallback(ctx, "a licence failed", err)
}

// errFairPlayOnly marks a rendition whose segments have no Widevine key:
// Apple serves it to its own apps only.
var errFairPlayOnly = errors.New("no Widevine key (FairPlay only)")

// Limits of the catalogue cache.
const (
	assetsTTL = 15 * time.Minute
	maxAssets = 128
)

// songAssets is a catalogue song's enhancedHls master, read once for a
// while (the traits of a song that has none are kept too).
type songAssets struct {
	done     chan struct{}
	at       time.Time
	traits   []string
	master   string        // the enhancedHls URL; "" when the song has none
	variants []hls.Variant // the master's renditions
	err      error
}

// assetsOf returns catalogue song id's assets, reading them in the
// background unless they are cached or already being read.
func (e *Engine) assetsOf(id string) *songAssets {
	e.assetMu.Lock()
	defer e.assetMu.Unlock()
	if a := e.assets[id]; a != nil {
		select {
		case <-a.done:
			if time.Since(a.at) < assetsTTL {
				return a
			}
		default:
			return a
		}
	}
	if e.assets == nil {
		e.assets = map[string]*songAssets{}
	}
	for len(e.assets) >= maxAssets {
		var oldest string
		for k, a := range e.assets {
			select {
			case <-a.done:
				if oldest == "" || a.at.Before(e.assets[oldest].at) {
					oldest = k
				}
			default:
			}
		}
		if oldest == "" {
			break
		}
		delete(e.assets, oldest)
	}
	a := &songAssets{done: make(chan struct{})}
	e.assets[id] = a
	go e.loadAssets(id, a)
	return a
}

// loadAssets reads catalogue song id's traits and, for a Spatial Audio
// song, its master playlist, into a.
func (e *Engine) loadAssets(id string, a *songAssets) {
	ctx, cancel := context.WithTimeout(e.bg, 30*time.Second)
	defer cancel()
	defer func() {
		a.at = time.Now()
		if a.err != nil { // not kept: the next song asks again
			e.assetMu.Lock()
			if e.assets[id] == a {
				delete(e.assets, id)
			}
			e.assetMu.Unlock()
		}
		close(a.done)
	}()
	var sf string
	if e.sf != nil {
		sf = e.sf()
	}
	if sf == "" {
		a.err = errors.New("no storefront to look the song up in")
		return
	}
	as, err := e.api.SongAssets(ctx, sf, id)
	if err != nil {
		a.err = err
		return
	}
	a.traits = as.AudioTraits
	if !hasSpatialTrait(a.traits) || as.EnhancedHLS == "" {
		return
	}
	body, err := e.fetch(ctx, as.EnhancedHLS, hls.ByteRange{})
	if err != nil {
		a.err = fmt.Errorf("enhancedHls master: %w", err)
		return
	}
	if a.variants, a.err = hls.ParseMaster(body); a.err == nil {
		a.master = as.EnhancedHLS
	}
}

// spatialKeys checks that every segment of pl is encrypted with a Widevine
// key and shares one init segment, and returns the distinct keys.
func spatialKeys(pl *hls.Playlist) ([]*hls.Key, error) {
	if len(pl.Segments) == 0 {
		return nil, errors.New("the playlist has no segments")
	}
	first := pl.Segments[0]
	if first.Map == nil {
		return nil, errors.New("the playlist has no EXT-X-MAP")
	}
	var keys []*hls.Key
	for i, s := range pl.Segments {
		k := s.Key
		if k == nil || k.KeyFormat != hls.WidevineKeyFormat || len(k.InitData) == 0 || len(k.KID) == 0 {
			return nil, fmt.Errorf("segment %d: %w", i, errFairPlayOnly)
		}
		if s.Map == nil || *s.Map != *first.Map {
			return nil, fmt.Errorf("segment %d changes the init segment", i)
		}
		if !slices.ContainsFunc(keys, func(o *hls.Key) bool { return o.URI == k.URI }) {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

// prepareSpatial prepares m (whose webPlayback answer is info) as its
// song's binaural rendition: the playlist, the init segment, and a licence
// for every key, which it takes on the host that is running or one it
// starts. It returns a *spatialFallback when the song can't play spatial,
// leaving m as it was, for the stereo stream. early is the catalogue lookup
// started before webPlayback answered, if the song's catalogue ID was known
// then.
func (e *Engine) prepareSpatial(ctx context.Context, m *media, info applemusic.PlaybackInfo, early *songAssets) error {
	a := early
	if a == nil {
		if info.SongID == "" {
			return &spatialFallback{reason: "webPlayback names no catalogue song"}
		}
		a = e.assetsOf(info.SongID)
	}
	select {
	case <-a.done:
	case <-ctx.Done():
		return ctx.Err()
	}
	switch {
	case a.err != nil:
		return lookupFailed(ctx, a.err)
	case !hasSpatialTrait(a.traits):
		return &spatialFallback{reason: "the song has no Spatial Audio mix", quiet: true}
	case a.master == "":
		return &spatialFallback{reason: "the song has no enhancedHls master", code: playback.SpatialNoMix}
	}
	rendition := spatialRendition(m.quality)
	vi := slices.IndexFunc(a.variants, func(v hls.Variant) bool { return v.Audio == rendition })
	if vi < 0 {
		return &spatialFallback{reason: "the master has no " + rendition + " rendition", code: playback.SpatialNoMix}
	}
	plURL, err := hls.Resolve(a.master, a.variants[vi].URI)
	if err != nil {
		return fallback(ctx, "the rendition's URL", err)
	}
	body, err := e.fetch(ctx, plURL, hls.ByteRange{})
	if err != nil {
		return fallback(ctx, "the rendition's playlist", err)
	}
	pl, err := hls.Parse(body)
	if err != nil {
		return fallback(ctx, "the rendition's playlist", err)
	}
	keys, err := spatialKeys(pl)
	if err != nil {
		code := playback.SpatialUnavailable
		if errors.Is(err, errFairPlayOnly) {
			code = playback.SpatialAppleOnly
		}
		return &spatialFallback{reason: "the rendition can't be played", code: code, err: err}
	}
	initURL, err := hls.Resolve(plURL, pl.Segments[0].Map.URI)
	if err != nil {
		return fallback(ctx, "the init segment's URL", err)
	}
	init, err := e.fetch(ctx, initURL, pl.Segments[0].Map.Range)
	if err != nil {
		return fallback(ctx, "the init segment", err)
	}
	in, err := fmp4.ParseInit(init)
	if err != nil {
		return fallback(ctx, "the init segment", err)
	}

	m.info = applemusic.PlaybackInfo{SongID: info.SongID, Flavor: rendition, PlaylistURL: plURL, LicenseURL: info.LicenseURL, CertURL: info.CertURL}
	m.pl, m.key, m.init, m.duration, m.trim = pl, keys[0], init, pl.Duration()-in.Priming(), true
	m.spatial = &spatialMedia{rendition: rendition, keys: keys, slots: map[string]string{}}
	if l, ok := fmp4.LoudnessOf(init); ok {
		m.gain = soundcheck.FromLoudness(l)
	}
	h, gen, err := e.ensureHost(ctx)
	if err == nil {
		if err = e.licenseSpatial(ctx, m, h, gen); err != nil {
			e.discardSessions(m, h)
			err = refused(ctx, err)
		}
	}
	if err != nil {
		m.info, m.pl, m.key, m.init, m.duration, m.trim, m.gain, m.spatial = applemusic.PlaybackInfo{}, nil, nil, nil, 0, false, 0, nil
	}
	return err
}

// licenseSpatial makes sure host generation gen holds a licensed session
// for every key of m's rendition (a new host loses them, so a restarted
// one is licensed again).
func (e *Engine) licenseSpatial(ctx context.Context, m *media, h host, gen int) error {
	m.licMu.Lock()
	defer m.licMu.Unlock()
	if m.keySessions == nil || m.keyGen != gen {
		e.mu.Lock()
		for _, sid := range m.keySessions {
			delete(e.sessions, sid)
		}
		e.mu.Unlock()
		m.keySessions, m.sessionKeys, m.keyGen = map[string]string{}, map[string]*hls.Key{}, gen
		m.spatial.slots = map[string]string{}
	}
	if len(m.keySessions) == len(m.spatial.keys) {
		return nil
	}
	if err := e.certificate(ctx, h, gen, m.info.CertURL); err != nil {
		return err
	}
	// A session per missing key is made first, then the licences are asked
	// for together: they are the slow part.
	type pending struct {
		n         int
		key       *hls.Key
		sid       string
		challenge []byte
		lic       applemusic.License
		err       error
	}
	var todo []*pending
	closeAll := func(ps []*pending) {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for _, p := range ps {
			_ = h.CloseSession(closeCtx, p.sid)
		}
	}
	t0 := time.Now()
	for i, k := range m.spatial.keys {
		if _, ok := m.keySessions[k.URI]; ok {
			continue
		}
		sid, challenge, err := h.CreateSession(ctx, k.InitData)
		if err != nil {
			closeAll(todo)
			return fmt.Errorf("key %d of %d: %w", i+1, len(m.spatial.keys), err)
		}
		todo = append(todo, &pending{n: i + 1, key: k, sid: sid, challenge: challenge})
	}
	var wg sync.WaitGroup
	for _, p := range todo {
		wg.Go(func() {
			p.lic, p.err = e.api.AcquireLicense(ctx, applemusic.LicenseRequest{
				URL: m.info.LicenseURL, Challenge: p.challenge, KeyURI: p.key.URI, AdamID: m.info.SongID,
			})
		})
	}
	wg.Wait()
	var failed []*pending
	var first error
	for _, p := range todo {
		if p.err == nil {
			p.err = h.UpdateSession(ctx, p.sid, p.lic.Data)
		}
		if p.err != nil {
			if first == nil {
				first = fmt.Errorf("key %d of %d: %w", p.n, len(m.spatial.keys), p.err)
			}
			failed = append(failed, p)
			continue
		}
		m.keySessions[p.key.URI], m.sessionKeys[p.sid], m.spatial.slots[p.sid] = p.sid, p.key, p.lic.SlotToken
		e.mu.Lock()
		e.sessions[p.sid] = m
		e.mu.Unlock()
	}
	closeAll(failed)
	if first != nil {
		return first
	}
	e.log.Debug("licensed", "id", m.id, "keys", len(todo), "took", time.Since(t0))
	return nil
}

// discardSessions closes m's key sessions on host h and forgets them.
func (e *Engine) discardSessions(m *media, h host) {
	m.licMu.Lock()
	defer m.licMu.Unlock()
	e.mu.Lock()
	for sid := range maps.Values(m.keySessions) {
		delete(e.sessions, sid)
	}
	e.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for sid := range maps.Values(m.keySessions) {
		_ = h.CloseSession(ctx, sid)
	}
	m.keySessions, m.sessionKeys, m.keyGen = nil, nil, 0
}

// renewSpatial answers a renewal message of one of m's key sessions; with
// m.licMu held. ok is false for a session m does not hold on gen.
func (e *Engine) renewSpatial(ctx context.Context, m *media, gen int, sid string, challenge []byte) (lic applemusic.License, ok bool, err error) {
	k := m.sessionKeys[sid]
	if k == nil || m.keyGen != gen {
		return lic, false, nil
	}
	lic, err = e.api.AcquireLicense(ctx, applemusic.LicenseRequest{
		URL: m.info.LicenseURL, Challenge: challenge, KeyURI: k.URI, AdamID: m.info.SongID,
		Renewal: true, SlotToken: m.spatial.slots[sid],
	})
	if err == nil && lic.SlotToken != "" {
		m.spatial.slots[sid] = lic.SlotToken
	}
	return lic, true, err
}

// missOf is the Status.SpatialMiss of m while it is the item heard; with mu
// held.
func (e *Engine) missOf(m *media) string {
	if m.spatial != nil || !e.spatial || !m.wantSpatial {
		return ""
	}
	return m.spatialMiss
}

// switchTimeout bounds preparing the other mix of the playing song.
const switchTimeout = 30 * time.Second

// SetSpatial turns Spatial Audio on or off. The armed next item follows
// the setting (it is prepared again), and so does the playing song: when
// it has a mix of the other kind to play, it is prepared and reloaded at
// its position, paused or playing as it was, in the background
// (switchMix). A song with no such mix, a station, a video or a preview
// plays on as it was.
func (e *Engine) SetSpatial(on bool) {
	e.mu.Lock()
	if e.spatial == on {
		e.mu.Unlock()
		return
	}
	e.spatial = on
	e.mixGen++
	it := e.cur
	if it != nil && it.pending != nil {
		e.dropPendingLocked(it, true)
		e.armLocked()
	}
	if !on && e.st.SpatialMiss != "" {
		e.st.SpatialMiss = ""
		e.emit(playback.EventState)
	}
	if it != nil && e.switchableLocked(it, it.m) {
		go e.switchMix(it, it.m)
	}
	e.mu.Unlock()
}

// switchableLocked reports whether the other Spatial Audio setting can
// change what it plays from m: a prepared, loaded song (not a station, a
// video, an upload or a preview) that is not playing the mix the setting
// asks for. With mu held.
func (e *Engine) switchableLocked(it *item, m *media) bool {
	if e.closed || e.preview || e.cur != it || it.m != m || m == nil || !it.loaded || e.h == nil {
		return false
	}
	if !m.trim || m.pl == nil || m.stream != nil || m.broadcast != nil || m.file != nil || m.video != nil || m.clip {
		return false
	}
	switch e.st.State {
	case playback.StatePlaying, playback.StatePaused, playback.StateLoading:
	default:
		return false
	}
	return (m.spatial != nil) != e.spatial
}

// switchMix plays the current song again from the mix the Spatial Audio
// setting now asks for, at its position. Whatever fails before the host is
// told to load leaves the audio as it is.
func (e *Engine) switchMix(it *item, old *media) {
	e.mixMu.Lock()
	defer e.mixMu.Unlock()
	ctx, cancel := context.WithTimeout(e.bg, switchTimeout)
	defer cancel()

	e.mu.Lock()
	if !e.switchableLocked(it, old) {
		e.mu.Unlock()
		return
	}
	gen, h, hostGen, on := e.mixGen, e.h, e.hostGen, e.spatial
	// The song is prepared afresh: the current media would be handed back.
	if e.cache[old.id] == old {
		delete(e.cache, old.id)
	}
	e.mu.Unlock()

	m2, err := e.media(ctx, old.id, false)
	if err == nil && (m2.spatial != nil) != (old.spatial != nil) {
		err = e.license(ctx, m2, h, hostGen) // else there is nothing to play, and nothing to license
	}
	e.ctl.Lock()
	defer e.ctl.Unlock()
	e.mu.Lock()
	// keep puts old back as the song's prepared media and forgets m2.
	keep := func() {
		if m2 != nil {
			e.dropLocked(m2)
		}
		if e.cache[old.id] == nil {
			e.cache[old.id] = old
		}
	}
	switch {
	case err != nil:
		keep()
		e.mu.Unlock()
		if e.bg.Err() == nil {
			e.log.Warn("spatial audio: switching the playing song failed; it keeps playing as it was", "id", old.id, "spatial", on, "err", err)
		}
		return
	case !e.switchableLocked(it, old) || e.mixGen != gen || e.h != h || e.hostGen != hostGen:
		keep() // a later command changed what plays, or the setting again
		e.mu.Unlock()
		return
	case (m2.spatial != nil) == (old.spatial != nil):
		// Nothing to hear: the song has no Spatial Audio mix to play.
		old.wantSpatial, old.spatialMiss = e.spatial, m2.spatialMiss
		keep()
		if st := e.missOf(old); st != e.st.SpatialMiss {
			e.st.SpatialMiss = st
			e.emit(playback.EventState)
		}
		e.mu.Unlock()
		e.log.Debug("spatial audio: the playing song has no other mix", "id", old.id, "why", m2.spatialMiss)
		return
	}
	now := time.Now()
	pos := e.st.PositionAt(now)
	if m2.duration > 0 {
		pos = min(pos, m2.duration)
	}
	e.track++
	tok := e.track
	it.tok, it.m, it.fed = tok, m2, false
	e.dropPendingLocked(it, false) // a reload drops what the host had queued
	if it.cancel != nil {
		it.cancel()
	}
	feedCtx, stopFeed := context.WithCancel(context.Background())
	it.cancel = stopFeed
	play := it.wantPlay
	state := playback.StatePaused
	if play {
		state = playback.StateLoading
	}
	e.st.Duration, e.st.Spatial, e.st.SpatialMiss = m2.duration, m2.spatial != nil, e.missOf(m2)
	e.setState(state, pos, now)
	e.mu.Unlock()

	e.prefetch(m2, m2.startSegment(pos))
	err = e.load(ctx, h, tok, m2, pos)
	if err == nil {
		e.startFeed(feedCtx, it, tok, h, hostGen, m2, pos)
		if play {
			err = h.Play(ctx)
		}
	}
	if err != nil {
		stopFeed()
		_ = e.fail(it, err)
		return
	}
	e.mu.Lock()
	e.dropLocked(old)
	e.mu.Unlock()
	e.log.Info("spatial audio: switched the playing song", "id", m2.id, "spatial", m2.spatial != nil, "at", pos)
}
