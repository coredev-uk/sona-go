//go:build linux

package native

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/cdmhost"
	"github.com/coredev-uk/sona-go/internal/playback/fmp4"
	"github.com/coredev-uk/sona-go/internal/playback/hls"
	"github.com/coredev-uk/sona-go/internal/playback/soundcheck"
	"github.com/coredev-uk/sona-go/internal/playback/widevine"
)

// media is a prepared item: metadata, playlist, init segment and licence.
type media struct {
	id      string
	quality applemusic.Quality // flavor it was (or is being) prepared with
	ready   chan struct{}
	err     error

	info       applemusic.PlaybackInfo
	pl         *hls.Playlist
	key        *hls.Key
	init       []byte
	duration   time.Duration
	segMu      sync.Mutex
	segs       map[int]*segFetch // in-flight or prefetched segments
	lastUsed   time.Time
	licMu      sync.Mutex
	session    string // CDM session holding this item's key …
	sessionGen int    // … on this host generation
	slotToken  string
	// Radio streams (nil stream for songs): live keys rotate, so sessions
	// are kept per key init data (keySessions, on host generation keyGen).
	stream      *stream
	keySessions map[string]string
	keyGen      int
	sessionKeys map[string]*hls.Key // session → key, for renewals
	file        *upload             // a library upload (no playlist, key or licence)
	gain        float64             // Sound Check gain (linear; 0 = none known)
	// trim: duration is the audio between the encoder's priming and
	// padding, which the host trims (songs; streams play every sample).
	trim bool
}

func (m *media) done() bool {
	select {
	case <-m.ready:
		return true
	default:
		return false
	}
}

// outdated reports a prepared song of another bitrate than the current
// setting that the current item is not playing from; with mu held.
func (e *Engine) outdated(m *media) bool {
	return m.stream == nil && m.quality != e.quality && (e.cur == nil || e.cur.m != m)
}

// media returns the prepared item id, preparing it (or waiting for a
// concurrent preparation) as needed. withSeg0 also fetches segment 0.
func (e *Engine) media(ctx context.Context, id string, withSeg0 bool) (*media, error) {
	for attempt := 0; ; attempt++ {
		e.mu.Lock()
		m := e.cache[id]
		if m != nil && m.done() && (m.stale() || e.outdated(m)) {
			e.dropLocked(m)
			m = nil
		}
		owner := m == nil
		if owner {
			m = &media{id: id, quality: e.quality, ready: make(chan struct{})}
			e.cache[id] = m
			e.evict()
		}
		m.lastUsed = time.Now()
		e.mu.Unlock()
		if owner {
			m.err = e.prepare(ctx, m)
			if m.err != nil {
				e.mu.Lock()
				if e.cache[id] == m {
					delete(e.cache, id)
				}
				e.mu.Unlock()
			}
			close(m.ready)
		} else {
			select {
			case <-m.ready:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		if m.err != nil {
			// A preparation abandoned by another caller's context is retried.
			if !owner && attempt == 0 && ctx.Err() == nil && (errors.Is(m.err, context.Canceled) || errors.Is(m.err, context.DeadlineExceeded)) {
				continue
			}
			return nil, m.err
		}
		if withSeg0 && m.file == nil {
			if _, err := e.segment(ctx, m, 0); err != nil {
				return nil, err
			}
		}
		return m, nil
	}
}

// evict drops the least recently used prepared items beyond maxCached,
// never the current one; with mu held.
func (e *Engine) evict() {
	for len(e.cache) > maxCached {
		var victim *media
		for _, m := range e.cache {
			if (e.cur != nil && e.cur.id == m.id) || !m.done() {
				continue
			}
			if victim == nil || m.lastUsed.Before(victim.lastUsed) {
				victim = m
			}
		}
		if victim == nil {
			return
		}
		e.dropLocked(victim)
	}
}

// dropLocked forgets prepared item m and closes its CDM sessions; with mu
// held.
func (e *Engine) dropLocked(m *media) {
	if e.cache[m.id] == m {
		delete(e.cache, m.id)
	}
	if m.file != nil {
		m.file.cancel()
	}
	sids := map[string]int{}
	if m.session != "" {
		sids[m.session] = m.sessionGen
	}
	for _, sid := range m.keySessions {
		sids[sid] = m.keyGen
	}
	for sid, gen := range sids {
		delete(e.sessions, sid)
		if h := e.h; h != nil && gen == e.hostGen {
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = h.CloseSession(ctx, sid)
			}()
		}
	}
}

// prepare fetches playback info, the playlist and the init segment.
func (e *Engine) prepare(ctx context.Context, m *media) error {
	if e.api == nil {
		return errors.New("native: no Apple Music client")
	}
	if IsStation(m.id) {
		return e.prepareStream(ctx, m)
	}
	t0 := time.Now()
	info, err := e.api.WebPlayback(ctx, m.id, m.quality)
	if err != nil {
		return err
	}
	if !info.Protected() {
		return e.prepareUpload(ctx, m, info)
	}
	tInfo := time.Now()
	body, err := e.fetch(ctx, info.PlaylistURL, hls.ByteRange{})
	if err != nil {
		return fmt.Errorf("native: playlist: %w", err)
	}
	pl, err := hls.Parse(body)
	if err != nil {
		return err
	}
	first := pl.Segments[0]
	if first.Key == nil || first.Key.Method != "ISO-23001-7" || first.Key.KID == nil {
		return fmt.Errorf("native: %s: playlist has no ISO-23001-7 key with an inline key ID: %w", m.id, applemusic.ErrNotPlayable)
	}
	if first.Map == nil {
		return fmt.Errorf("native: %s: playlist has no EXT-X-MAP", m.id)
	}
	for i, s := range pl.Segments {
		if s.Map == nil || *s.Map != *first.Map || s.Key == nil || !bytes.Equal(s.Key.KID, first.Key.KID) {
			return fmt.Errorf("native: %s: segment %d changes init segment or key, which is not supported", m.id, i)
		}
	}
	initURL, err := hls.Resolve(info.PlaylistURL, first.Map.URI)
	if err != nil {
		return err
	}
	tList := time.Now()
	init, err := e.fetch(ctx, initURL, first.Map.Range)
	if err != nil {
		return fmt.Errorf("native: init segment: %w", err)
	}
	in, err := fmp4.ParseInit(init)
	if err != nil {
		return fmt.Errorf("native: init segment: %w", err)
	}
	// The playlist counts the encoder's priming, not its end padding: the
	// track's audio is what follows the priming.
	m.info, m.pl, m.key, m.init, m.duration = info, pl, first.Key, init, pl.Duration()-in.Priming()
	m.trim = true
	if l, ok := fmp4.LoudnessOf(init); ok {
		m.gain = soundcheck.FromLoudness(l)
	}
	e.log.Info("prepared", "id", m.id, "flavor", info.Flavor, "webPlayback", tInfo.Sub(t0), "playlist", tList.Sub(tInfo), "init", time.Since(tList))
	return nil
}

// segFetch is one segment download that several callers may wait for.
type segFetch struct {
	done chan struct{}
	data []byte
	err  error
}

// segment returns segment i of m, joining a prefetch of it if one is under
// way. Segment 0 stays cached (Preload, replays); others are dropped once
// delivered.
func (e *Engine) segment(ctx context.Context, m *media, i int) ([]byte, error) {
	f := e.prefetch(m, i)
	select {
	case <-f.done:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	m.segMu.Lock()
	if i != 0 || f.err != nil {
		if m.segs[i] == f {
			delete(m.segs, i)
		}
	}
	m.segMu.Unlock()
	return f.data, f.err
}

// prefetch starts downloading segment i of m unless that is under way.
func (e *Engine) prefetch(m *media, i int) *segFetch {
	m.segMu.Lock()
	defer m.segMu.Unlock()
	if f := m.segs[i]; f != nil {
		return f
	}
	if m.segs == nil {
		m.segs = map[int]*segFetch{}
	}
	f := &segFetch{done: make(chan struct{})}
	m.segs[i] = f
	go func() {
		defer close(f.done)
		ctx, cancel := context.WithTimeout(e.bg, time.Minute)
		defer cancel()
		s := m.pl.Segments[i]
		u, err := hls.Resolve(m.info.PlaylistURL, s.URI)
		if err == nil {
			f.data, err = e.fetch(ctx, u, s.Range)
		}
		if err != nil {
			f.err = fmt.Errorf("native: segment %d: %w", i, err)
		}
	}()
	return f
}

// retryDelays are the default waits between attempts on 429, 5xx and
// network errors.
var retryDelays = []time.Duration{250 * time.Millisecond, 500 * time.Millisecond, time.Second, 2 * time.Second, 4 * time.Second}

// httpError is a non-2xx CDN response.
type httpError struct {
	url  string
	code int
}

func (e *httpError) Error() string { return fmt.Sprintf("GET %s: HTTP %d", e.url, e.code) }

func retryable(err error) bool {
	var he *httpError
	if errors.As(err, &he) {
		return he.code == http.StatusTooManyRequests || he.code >= 500
	}
	return !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
}

// fetch GETs a CDN resource (or byte range of it), retrying transient
// failures. A server that ignores Range is tolerated by slicing.
func (e *Engine) fetch(ctx context.Context, url string, r hls.ByteRange) ([]byte, error) {
	var err error
	for attempt := 0; ; attempt++ {
		var b []byte
		if b, err = e.fetchOnce(ctx, url, r); err == nil {
			return b, nil
		}
		if attempt >= len(e.retry) || !retryable(err) || ctx.Err() != nil {
			return nil, err
		}
		delay := e.retry[attempt]
		var he *httpError
		if errors.As(err, &he) && he.code == http.StatusTooManyRequests {
			delay = max(delay, time.Second)
		}
		e.log.Debug("retrying fetch", "url", url, "err", err, "in", delay)
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (e *Engine) fetchOnce(ctx context.Context, url string, r hls.ByteRange) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if h := r.Header(); h != "" {
		req.Header.Set("Range", h)
	}
	// Apple's CDN refuses a share of requests with Go's default agent.
	req.Header.Set("User-Agent", applemusic.UserAgent)
	resp, err := e.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, &httpError{url: url, code: resp.StatusCode}
	}
	size := r.Length
	if resp.StatusCode == http.StatusOK && r.Length > 0 {
		// Range ignored: skip to the offset.
		if _, err := io.CopyN(io.Discard, resp.Body, r.Offset); err != nil {
			return nil, err
		}
	}
	var body []byte
	if size > 0 {
		body = make([]byte, size)
		if _, err := io.ReadFull(resp.Body, body); err != nil {
			return nil, fmt.Errorf("GET %s: %w", url, err)
		}
		return body, nil
	}
	if n, _ := strconv.Atoi(resp.Header.Get("Content-Length")); n > 0 {
		body = make([]byte, 0, n)
	}
	buf := bytes.NewBuffer(body)
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	return buf.Bytes(), nil
}

// prefetchCert fetches the default service certificate in the background so
// the first licence does not wait for it.
func (e *Engine) prefetchCert() {
	if e.api == nil {
		return
	}
	go func() {
		e.certMu.Lock()
		defer e.certMu.Unlock()
		if e.cert != nil {
			return
		}
		ctx, cancel := context.WithTimeout(e.bg, 30*time.Second)
		defer cancel()
		if cert, err := e.api.WidevineCert(ctx, applemusic.WidevineCertURL); err == nil {
			e.cert, e.certURL = cert, applemusic.WidevineCertURL
		}
	}()
}

// certificate installs the Widevine service certificate on host generation
// gen. The certificate is fetched once per process (per URL).
func (e *Engine) certificate(ctx context.Context, h host, gen int, url string) error {
	e.certMu.Lock()
	defer e.certMu.Unlock()
	if url == "" {
		url = applemusic.WidevineCertURL
	}
	if e.cert == nil || e.certURL != url {
		cert, err := e.api.WidevineCert(ctx, url)
		if err != nil {
			return err
		}
		e.cert, e.certURL, e.certGen = cert, url, 0
	}
	if e.certGen == gen {
		return nil
	}
	if err := h.SetServerCertificate(ctx, e.cert); err != nil {
		return err
	}
	e.certGen = gen
	return nil
}

// license makes sure host generation gen holds m's key.
func (e *Engine) license(ctx context.Context, m *media, h host, gen int) error {
	if m.file != nil {
		return nil // plain file: no licence
	}
	if m.stream != nil {
		return e.streamLicense(ctx, m, m.key, h, gen)
	}
	m.licMu.Lock()
	defer m.licMu.Unlock()
	if m.session != "" && m.sessionGen == gen {
		return nil
	}
	if err := e.certificate(ctx, h, gen, m.info.CertURL); err != nil {
		return err
	}
	t0 := time.Now()
	sid, challenge, err := h.CreateSession(ctx, m.key.KID)
	if err != nil {
		return err
	}
	tChallenge := time.Now()
	lic, err := e.api.AcquireLicense(ctx, applemusic.LicenseRequest{
		URL: m.info.LicenseURL, Challenge: challenge, KeyURI: m.key.URI,
		AdamID: m.info.SongID, IsLibrary: m.info.IsLibrary,
	})
	if err == nil {
		err = h.UpdateSession(ctx, sid, lic.Data)
	}
	if err != nil {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = h.CloseSession(closeCtx, sid)
		cancel()
		return err
	}
	m.session, m.sessionGen, m.slotToken = sid, gen, lic.SlotToken
	e.log.Debug("licensed", "id", m.id, "challenge", tChallenge.Sub(t0), "licence+update", time.Since(tChallenge))
	e.mu.Lock()
	e.sessions[sid] = m
	e.mu.Unlock()
	return nil
}

// renew answers a CDM message (licence renewal) for one of our sessions.
// Apple's licences last 15 minutes and Chrome's CDM asks for a renewal at
// 13, so a session older than that renews; every round trip is logged.
func (e *Engine) renew(h host, gen int, ev cdmhost.Event) {
	e.mu.Lock()
	m := e.sessions[ev.Session]
	e.mu.Unlock()
	if m == nil {
		e.log.Debug("CDM message for an unknown session", "session", ev.Session, "type", widevine.MessageType(ev.Type))
		return
	}
	t0 := time.Now()
	e.log.Info("licence renewal requested", "id", m.id, "session", ev.Session, "type", widevine.MessageType(ev.Type), "bytes", len(ev.Data))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	m.licMu.Lock()
	defer m.licMu.Unlock()
	var (
		lic applemusic.License
		err error
	)
	if m.stream != nil {
		if m.sessionKeys[ev.Session] == nil || m.keyGen != gen {
			return
		}
		lic, err = e.renewStream(ctx, m, ev.Session, ev.Data)
	} else {
		if m.session != ev.Session || m.sessionGen != gen {
			return
		}
		lic, err = e.api.AcquireLicense(ctx, applemusic.LicenseRequest{
			URL: m.info.LicenseURL, Challenge: ev.Data, KeyURI: m.key.URI,
			AdamID: m.info.SongID, IsLibrary: m.info.IsLibrary, Renewal: true, SlotToken: m.slotToken,
		})
	}
	if err == nil {
		if lic.SlotToken != "" {
			m.slotToken = lic.SlotToken
		}
		err = h.UpdateSession(ctx, ev.Session, lic.Data)
	}
	if err != nil {
		e.log.Warn("licence renewal failed", "id", m.id, "err", err)
		if errors.Is(err, applemusic.ErrUnauthorized) {
			e.mu.Lock()
			e.emit(playback.EventAuth)
			e.mu.Unlock()
		}
		return
	}
	e.log.Info("licence renewed", "id", m.id, "session", ev.Session, "took", time.Since(t0))
}

// feed streams m's segments from index from to the host for track tok,
// fetching up to readAhead segments ahead.
func (e *Engine) feed(ctx context.Context, it *item, tok uint64, h host, m *media, from int) {
	type result struct {
		data []byte
		err  error
	}
	ch := make(chan result, e.readAhead)
	go func() {
		defer close(ch)
		for i := from; i < len(m.pl.Segments); i++ {
			b, err := e.segment(ctx, m, i)
			select {
			case ch <- result{b, err}:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()
	for r := range ch {
		err := r.err
		if err == nil {
			err = h.Feed(ctx, tok, r.data)
		}
		if err != nil {
			if ctx.Err() == nil && !errors.Is(err, cdmhost.ErrStale) && !errors.Is(err, cdmhost.ErrExited) {
				e.fail(it, err)
			}
			return
		}
	}
	if ctx.Err() == nil {
		if err := h.EndOfTrack(ctx, tok); err != nil {
			if !errors.Is(err, cdmhost.ErrStale) && !errors.Is(err, cdmhost.ErrExited) && ctx.Err() == nil {
				_ = e.fail(it, err)
			}
			return
		}
		e.fed(it, tok)
	}
}

// hostMedia describes m for Host.Load and Host.Next.
func (m *media) hostMedia(init []byte) cdmhost.Media {
	hm := cdmhost.Media{Init: init, Gain: m.gain}
	if m.trim {
		hm.Duration = m.duration
	}
	return hm
}
