// Package broadcast plays local broadcasters' live streams: the stations
// Apple Music lists as Local Broadcasters (Capital, KISS, Heart, …). For
// such a station Apple's /v1/play/assets answers with one clear URL on the
// broadcaster's (or TuneIn's) servers: an HLS playlist of MPEG-TS or packed
// AAC segments, a PLS playlist (TuneIn's: its station ident and pre-roll,
// then the live stream) or a Shoutcast/Icecast stream. Play follows that
// URL and hands on the elementary audio (ADTS AAC or MPEG audio frames)
// for the playback host to decode, and what the stream says is on air
// (ICY StreamTitle, ID3 tags, EXTINF titles) with the stream time it
// applies from. No DRM, licence or Apple credential is involved.
package broadcast

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/http/cookiejar"
	neturl "net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/hls"
	"github.com/coredev-uk/sona-go/internal/playback/id3"
)

// Sink receives a stream as it plays.
type Sink interface {
	// Audio takes the next elementary audio bytes; b is only valid during
	// the call. An error ends Play with it.
	Audio(ctx context.Context, b []byte) error
	// Meta reports what is on air from stream time at on: the duration of
	// the audio handed to Audio before it applies. A zero OnAir means the
	// stream names nothing now (speech, a jingle, adverts).
	Meta(at time.Duration, m playback.OnAir)
}

// Options configures Play.
type Options struct {
	// HTTP fetches the stream. It must have no overall timeout (a stream
	// lasts for hours): Play bounds every request and watches for stalls
	// itself. Play gives it a cookie jar of its own when it has none.
	HTTP      *http.Client
	UserAgent string
	Logger    *slog.Logger
}

const (
	// fetchTimeout bounds a playlist or segment download, and the wait
	// for a stream's response headers.
	fetchTimeout = 20 * time.Second
	// stallTimeout ends a stream that delivers nothing for this long.
	stallTimeout = 20 * time.Second
	// liveEdgeSegments is how many segments from the end of a live HLS
	// window playback starts (HLS recommends at least three).
	liveEdgeSegments = 3
	maxPlaylist      = 1 << 20  // bytes
	maxSegment       = 32 << 20 // bytes
	maxDepth         = 3        // playlists naming playlists
	maxRetries       = 3        // a request's attempts
	maxReconnects    = 3        // a stream's reconnections in a row without audio
)

var errStalled = errors.New("broadcast: the stream stopped delivering audio")

// Play plays the stream at url into sink until ctx ends or the stream
// does. It returns nil when the stream itself ended (an HLS playlist's
// ENDLIST, a playlist's last entry played out).
func Play(ctx context.Context, o Options, url string, sink Sink) error {
	hc := http.Client{}
	if o.HTTP != nil {
		hc = *o.HTTP
	}
	if hc.Jar == nil {
		hc.Jar, _ = cookiejar.New(nil)
	}
	log := o.Logger
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	s := &session{hc: &hc, ua: o.UserAgent, log: log, sink: sink}
	return s.play(ctx, url, 0)
}

// session is one Play.
type session struct {
	hc   *http.Client
	ua   string
	log  *slog.Logger
	sink Sink
	clk  clock
	on   playback.OnAir // last reported
	said bool           // on was reported
}

func (s *session) audio(ctx context.Context, b []byte) error {
	if len(b) == 0 {
		return nil
	}
	s.clk.write(b)
	return s.sink.Audio(ctx, b)
}

// meta reports m unless it is what was reported last.
func (s *session) meta(m playback.OnAir) {
	if s.said && m == s.on {
		return
	}
	s.on, s.said = m, true
	s.sink.Meta(s.clk.d, m)
}

// play plays url, whatever it is: an HLS, PLS or M3U playlist, or a
// stream.
func (s *session) play(ctx context.Context, url string, depth int) error {
	if depth > maxDepth {
		return fmt.Errorf("broadcast: %s: playlists nested too deep", url)
	}
	resp, cancel, err := s.open(ctx, url)
	if err != nil {
		return err
	}
	br := bufio.NewReaderSize(resp.Body, 64<<10)
	head, _ := br.Peek(512)
	ct, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	switch {
	case playlistType(ct) || isPLS(head) || bytes.HasPrefix(bytes.TrimPrefix(head, []byte("\ufeff")), []byte("#EXTM3U")):
		body, err := io.ReadAll(io.LimitReader(br, maxPlaylist))
		final := resp.Request.URL.String()
		_ = resp.Body.Close()
		cancel()
		if err != nil {
			return fmt.Errorf("broadcast: playlist %s: %w", url, err)
		}
		switch {
		case isPLS(body):
			return s.list(ctx, final, parsePLS(body), depth)
		case isHLS(body):
			return s.hls(ctx, final, body)
		default:
			return s.list(ctx, final, parseM3U(body), depth)
		}
	case audioType(ct):
		return s.stream(ctx, url, resp, br, cancel, depth == 0)
	default:
		_ = resp.Body.Close()
		cancel()
		return fmt.Errorf("broadcast: %s is %q, not an audio stream or playlist: %w", url, ct, ErrUnsupported)
	}
}

// ErrUnsupported reports a stream Sona cannot play.
var ErrUnsupported = errors.New("unsupported stream")

func playlistType(ct string) bool {
	switch ct {
	case "application/vnd.apple.mpegurl", "application/x-mpegurl", "audio/mpegurl", "audio/x-mpegurl", "audio/x-scpls", "application/pls+xml":
		return true
	}
	return false
}

// audioType reports a stream's content type: MPEG audio or AAC (some
// servers send no specific type).
func audioType(ct string) bool {
	switch ct {
	case "audio/mpeg", "audio/mp3", "audio/mpga", "audio/aac", "audio/aacp", "audio/x-aac", "audio/x-mpeg", "application/octet-stream", "":
		return true
	}
	return false
}

// list plays a playlist's entries in order. Broadcasters list one stream
// several times so that players reconnect: an entry that fails or ends
// moves on to the next. It returns the last entry's error.
func (s *session) list(ctx context.Context, base string, urls []string, depth int) error {
	if len(urls) == 0 {
		return fmt.Errorf("broadcast: playlist %s: no entries", base)
	}
	var err error
	for i, u := range urls {
		if u, err = hls.Resolve(base, u); err != nil {
			continue
		}
		if err = s.play(ctx, u, depth+1); ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			s.log.Warn("broadcast: playlist entry failed", "entry", i+1, "of", len(urls), "err", err)
		}
	}
	return err
}

// stream plays a Shoutcast/Icecast or plain audio stream from its open
// response to its end. reconnect: open it again when it drops (not for a
// playlist's entry: the playlist's next entry does that).
func (s *session) stream(ctx context.Context, url string, resp *http.Response, br *bufio.Reader, cancel context.CancelFunc, reconnect bool) error {
	for fails := 0; ; {
		before := s.clk.d
		err := s.readStream(ctx, resp, br, cancel)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !reconnect {
			return err
		}
		if s.clk.d > before {
			fails = 0
		}
		for {
			if fails++; fails > maxReconnects {
				return fmt.Errorf("broadcast: %s: %w", url, cmp.Or(err, io.ErrUnexpectedEOF))
			}
			s.log.Info("broadcast: reconnecting", "after", err, "attempt", fails)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(fails) * time.Second):
			}
			if resp, cancel, err = s.open(ctx, url); err == nil {
				break
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
		}
		br = bufio.NewReaderSize(resp.Body, 64<<10)
	}
}

// readStream reads one connection of a stream to its end: nil at the
// stream's end, else why it broke off.
func (s *session) readStream(ctx context.Context, resp *http.Response, br *bufio.Reader, cancel context.CancelFunc) error {
	defer cancel()
	defer func() { _ = resp.Body.Close() }()
	var stalled atomic.Bool
	watch := time.AfterFunc(stallTimeout, func() {
		stalled.Store(true)
		cancel()
	})
	defer watch.Stop()
	metaint, _ := strconv.Atoi(resp.Header.Get("icy-metaint"))
	r := newICYReader(br, metaint, func(title string) { s.meta(playback.OnAir{Title: title}) })
	var (
		buf  = make([]byte, 16<<10)
		head []byte // the stream's first bytes, until a leading ID3 tag is ruled out
		skip = -1   // bytes of a leading ID3 tag still to drop; -1 until known
	)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			watch.Reset(stallTimeout)
			b := buf[:n]
			if skip < 0 {
				// A file in a playlist (a station ident, a pre-roll) may
				// start with an ID3 tag, which is no audio.
				head = append(head, b...)
				b = nil
				if l, ok := id3.Len(head); ok {
					b, skip, head = head, l, nil
				}
			}
			if skip > 0 {
				k := min(skip, len(b))
				skip -= k
				b = b[k:]
			}
			if aerr := s.audio(ctx, b); aerr != nil {
				return aerr
			}
		}
		switch {
		case err == nil:
		case errors.Is(err, io.EOF):
			return nil
		case stalled.Load():
			return errStalled
		default:
			return err
		}
	}
}

// hls plays an HLS playlist (body, fetched from url): a live one from
// near its live edge, reloading it as the window slides; an ended one
// from its start.
func (s *session) hls(ctx context.Context, url string, body []byte) error {
	var err error
	if hls.IsMaster(body) {
		vs, err := hls.ParseMaster(body)
		if err != nil {
			return err
		}
		if url, err = hls.Resolve(url, hls.BestAAC(vs).URI); err != nil {
			return err
		}
		if body, url, err = s.fetch(ctx, url, hls.ByteRange{}); err != nil {
			return fmt.Errorf("broadcast: playlist: %w", err)
		}
	}
	pl, err := hls.Parse(body)
	if err != nil {
		return err
	}
	next := pl.Segments[0].Seq
	if !pl.Ended {
		next = pl.Segments[max(len(pl.Segments)-liveEdgeSegments, 0)].Seq
	}
	ts := newTSDemuxer()
	for {
		fed := false
		for _, seg := range pl.Segments {
			if seg.Seq < next {
				continue
			}
			u, err := hls.Resolve(url, seg.URI)
			if err != nil {
				return err
			}
			data, _, err := s.fetch(ctx, u, seg.Range)
			var se *statusError
			switch {
			case errors.As(err, &se) && se.code == http.StatusNotFound:
				// Gone from the server already: carry on with the next.
				s.log.Debug("broadcast: segment gone", "seq", seg.Seq)
			case err != nil:
				return fmt.Errorf("broadcast: segment %d: %w", seg.Seq, err)
			default:
				if err := s.segment(ctx, ts, seg, data); err != nil {
					return fmt.Errorf("broadcast: segment %d: %w", seg.Seq, err)
				}
			}
			next, fed = seg.Seq+1, true
		}
		if pl.Ended {
			return nil
		}
		if !fed {
			// Nothing new yet: wait for the window to slide.
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(max(pl.TargetDuration/2, time.Second)):
			}
		}
		if body, _, err = s.fetch(ctx, url, hls.ByteRange{}); err != nil {
			return fmt.Errorf("broadcast: playlist: %w", err)
		}
		if pl, err = hls.Parse(body); err != nil {
			return err
		}
		if next < pl.MediaSequence {
			// Fell out of the window: rejoin near the edge.
			next = pl.Segments[max(len(pl.Segments)-liveEdgeSegments, 0)].Seq
			s.log.Info("broadcast: rejoined the live edge")
		}
	}
}

// segment plays one HLS segment: an MPEG transport stream, or packed
// audio that may start with an ID3 tag. What is on air comes from the
// segment's ID3 tags, else from its EXTINF title attributes.
func (s *session) segment(ctx context.Context, ts *tsDemuxer, seg hls.Segment, data []byte) error {
	if m, ok := extinfOnAir(seg.Title); ok {
		s.meta(m)
	}
	if isTS(data) {
		return ts.segment(data, func(b []byte) error { return s.audio(ctx, b) }, func(tag []byte) {
			if m, ok := tagOnAir(tag); ok {
				s.meta(m)
			}
		})
	}
	if n, _ := id3.Len(data); n > 0 && n <= len(data) {
		if m, ok := tagOnAir(data[:n]); ok {
			s.meta(m)
		}
		data = data[n:]
	}
	return s.audio(ctx, data)
}

// tagOnAir reads the title and artist of an ID3 tag; ok is false when the
// tag names neither (packed audio's timestamp tags, for instance), so it
// says nothing about what is on air.
func tagOnAir(tag []byte) (m playback.OnAir, ok bool) {
	var event string
	id3.Frames(tag, func(id string, body []byte) bool {
		switch id {
		case "TIT2", "TT2":
			m.Title, ok = id3.Text(body), true
		case "TPE1", "TP1":
			m.Artist, ok = id3.Text(body), true
		case "WXXX", "WXX":
			event = id3.Text(body)
		}
		return true
	})
	return planetRadio(m, event), ok
}

// extinfOnAir reads title="…" and artist="…" from an EXTINF title, which
// some broadcasters fill with attributes.
func extinfOnAir(title string) (playback.OnAir, bool) {
	if !strings.Contains(title, "=") {
		return playback.OnAir{}, false
	}
	attrs, err := hls.ParseAttributes(title)
	if err != nil {
		return playback.OnAir{}, false
	}
	t, okT := attrs["title"]
	a, okA := attrs["artist"]
	return planetRadio(playback.OnAir{Title: strings.TrimSpace(t), Artist: strings.TrimSpace(a)}, attrs["url"]), okT || okA
}

// planetRadio puts the title and artist of Bauer's Planet Radio streams
// (KISS, Magic, Absolute Radio, …) the right way round: their ID3 tags and
// EXTINF titles name the artist as the title and the song as the artist.
// Such metadata links its event on Planet Radio's listen API (event: the
// EXTINF url attribute, or the ID3 WXXX frame, "URL, https://…").
func planetRadio(m playback.OnAir, event string) playback.OnAir {
	_, link, _ := strings.Cut(event, "https://")
	u, err := neturl.Parse("https://" + link)
	if link == "" || err != nil {
		return m
	}
	if h := u.Hostname(); h == "planetradio.co.uk" || strings.HasSuffix(h, ".planetradio.co.uk") {
		m.Title, m.Artist = m.Artist, m.Title
	}
	return m
}

// statusError is an HTTP response other than 200 or 206.
type statusError struct {
	url  string
	code int
}

func (e *statusError) Error() string { return fmt.Sprintf("GET %s: HTTP %d", e.url, e.code) }

func retryable(err error) bool {
	var se *statusError
	if errors.As(err, &se) {
		return se.code == http.StatusTooManyRequests || se.code >= 500
	}
	if errors.Is(err, errFetchTimeout) {
		return true
	}
	return !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
}

var errFetchTimeout = errors.New("broadcast: request timed out")

// open requests url and returns its response once the headers are in
// (within fetchTimeout). cancel ends the request; the caller closes the
// body and calls it.
func (s *session) open(ctx context.Context, url string) (*http.Response, context.CancelFunc, error) {
	rctx, cancel := context.WithCancelCause(ctx)
	req, err := http.NewRequestWithContext(rctx, http.MethodGet, url, nil)
	if err != nil {
		cancel(nil)
		return nil, nil, err
	}
	req.Header.Set("User-Agent", s.ua)
	req.Header.Set("Icy-MetaData", "1")
	t := time.AfterFunc(fetchTimeout, func() { cancel(errFetchTimeout) })
	resp, err := s.hc.Do(req)
	t.Stop()
	if err != nil {
		if c := context.Cause(rctx); errors.Is(c, errFetchTimeout) {
			err = fmt.Errorf("GET %s: %w", url, c)
		}
		cancel(nil)
		return nil, nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		cancel(nil)
		return nil, nil, &statusError{url: url, code: resp.StatusCode}
	}
	return resp, func() { cancel(nil) }, nil
}

// fetch downloads a playlist or segment (or a byte range of it), retrying
// transient failures, and returns it with the URL it came from after
// redirects.
func (s *session) fetch(ctx context.Context, url string, r hls.ByteRange) ([]byte, string, error) {
	var err error
	for attempt := 1; ; attempt++ {
		var (
			b     []byte
			final string
		)
		if b, final, err = s.fetchOnce(ctx, url, r); err == nil {
			return b, final, nil
		}
		if attempt >= maxRetries || !retryable(err) || ctx.Err() != nil {
			return nil, "", err
		}
		select {
		case <-ctx.Done():
			return nil, "", ctx.Err()
		case <-time.After(time.Duration(attempt) * 500 * time.Millisecond):
		}
	}
}

func (s *session) fetchOnce(ctx context.Context, url string, r hls.ByteRange) ([]byte, string, error) {
	rctx, cancel := context.WithTimeoutCause(ctx, fetchTimeout, errFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(rctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", s.ua)
	if h := r.Header(); h != "" {
		req.Header.Set("Range", h)
	}
	resp, err := s.hc.Do(req)
	if err != nil {
		if c := context.Cause(rctx); errors.Is(c, errFetchTimeout) {
			err = fmt.Errorf("GET %s: %w", url, c)
		}
		return nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, "", &statusError{url: url, code: resp.StatusCode}
	}
	body := io.Reader(resp.Body)
	if resp.StatusCode == http.StatusOK && r.Length > 0 {
		// Range ignored: skip to the offset and read the range.
		if _, err := io.CopyN(io.Discard, body, r.Offset); err != nil {
			return nil, "", noEOF(err)
		}
		body = io.LimitReader(body, r.Length)
	}
	b, err := io.ReadAll(io.LimitReader(body, maxSegment+1))
	if err != nil {
		if c := context.Cause(rctx); errors.Is(c, errFetchTimeout) {
			err = fmt.Errorf("GET %s: %w", url, c)
		}
		return nil, "", err
	}
	if len(b) > maxSegment {
		return nil, "", fmt.Errorf("GET %s: more than %d bytes", url, maxSegment)
	}
	return b, resp.Request.URL.String(), nil
}
