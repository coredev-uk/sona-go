//go:build linux || (darwin && cgo)

package native

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/cdmhost"
	"github.com/coredev-uk/sona-go/internal/playback/fmp4"
	"github.com/coredev-uk/sona-go/internal/playback/hls"
)

// Music videos (playback.VideoID items) play as a track whose audio is
// the video's audio rendition, fetched, licensed, fed and timed exactly
// like a song's, plus its video rendition: the child decodes it with the
// CDM's own decoder and sends each picture when the audio reaches it,
// which the engine puts on its video.Surface for the web view. Apple
// licenses a software CDM for SD video only (hls.SelectVideo). Videos are
// never joined gaplessly to their neighbours (armLocked).

// videoTrack is a prepared music video's video rendition.
type videoTrack struct {
	url           string // media playlist
	pl            *hls.Playlist
	key           *hls.Key
	// offset is where the renditions' timestamps start (Apple's start at
	// the same offset, e.g. 10 s): subtracted from the audio's (Shift)
	// so it plays on the playlist's timeline. shift is what puts the
	// video's on the audio position's timeline (VideoMedia.Shift).
	offset, shift time.Duration
	init          []byte
	width, height int
	// The CDM session holding the video key on host generation
	// sessionGen, and its licence slot token for renewals; guarded by
	// media.licMu.
	session    string
	sessionGen int
	slotToken  string
}

// errMasterFetch hides the master playlist's URL, which names the account
// (dsid), from errors that get logged.
var errMasterFetch = errors.New("native: video master playlist could not be fetched")

// fetchMaster GETs a music video's master playlist, retrying transient
// failures like fetch but never naming its URL (it carries the account's
// dsid) in a log line or an error.
func (e *Engine) fetchMaster(ctx context.Context, url string) ([]byte, error) {
	for attempt := 0; ; attempt++ {
		b, err := e.fetchOnce(ctx, url, hls.ByteRange{})
		if err == nil {
			return b, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		code := 0
		var he *httpError
		if errors.As(err, &he) {
			code = he.code
		}
		if attempt >= len(e.retry) || !retryable(err) {
			if code != 0 {
				return nil, fmt.Errorf("%w: HTTP %d", errMasterFetch, code)
			}
			return nil, errMasterFetch
		}
		e.log.Debug("retrying the video master playlist", "status", code, "in", e.retry[attempt])
		select {
		case <-time.After(e.retry[attempt]):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// prepareVideo fetches music video m's playback info, its master playlist,
// and the media playlists and init segments of the renditions it plays:
// the best SD H.264 video a software CDM may play, and stereo AAC audio.
func (e *Engine) prepareVideo(ctx context.Context, m *media) error {
	if e.surface == nil || e.preview {
		// Previews play no videos: they have no MUT to ask webPlayback.
		return fmt.Errorf("native: %s: %w", m.id, playback.ErrUnsupported)
	}
	id := playback.VideoCatalogID(m.id)
	t0 := time.Now()
	info, err := e.api.VideoPlayback(ctx, id)
	if err != nil {
		return err
	}
	master, err := e.fetchMaster(ctx, info.MasterURL)
	if err != nil {
		return err
	}
	variants, err := hls.ParseMaster(master)
	if err != nil {
		return fmt.Errorf("native: %s: %w", m.id, err)
	}
	renditions, err := hls.ParseRenditions(master)
	if err != nil {
		return fmt.Errorf("native: %s: %w", m.id, err)
	}
	variant, ok := hls.SelectVideo(variants)
	if !ok {
		return fmt.Errorf("native: %s: %w", m.id, playback.ErrVideoHDOnly)
	}
	audio, ok := hls.SelectAudio(renditions, variant)
	if !ok {
		return fmt.Errorf("native: %s: no stereo AAC audio rendition: %w", m.id, applemusic.ErrNotPlayable)
	}
	var (
		urls [2]string
		pls  [2]*hls.Playlist
		keys [2]*hls.Key
		init [2][]byte
		errs [2]error
		wg   sync.WaitGroup
	)
	for i, ref := range []string{variant.URI, audio.URI} {
		if urls[i], err = hls.Resolve(info.MasterURL, ref); err != nil {
			return fmt.Errorf("native: %s: rendition URL: %w", m.id, err)
		}
		wg.Go(func() { pls[i], keys[i], init[i], errs[i] = e.videoRendition(ctx, urls[i]) })
	}
	wg.Wait()
	if err := errors.Join(errs[:]...); err != nil {
		return fmt.Errorf("native: %s: %w", m.id, err)
	}
	ain, err := fmp4.ParseInit(init[1])
	if err != nil {
		return fmt.Errorf("native: %s: audio init segment: %w", m.id, err)
	}
	if ain.IsVideo() {
		return fmt.Errorf("native: %s: the audio rendition is video", m.id)
	}
	m.info = applemusic.PlaybackInfo{SongID: info.SongID, LicenseURL: info.LicenseURL, CertURL: info.CertURL, PlaylistURL: urls[1]}
	m.pl, m.key, m.init = pls[1], keys[1], init[1]
	m.duration, m.trim = pls[1].Duration()-ain.Priming(), true
	// The renditions' timestamps start at an offset (10 s on Apple's
	// videos), the same for both; the first audio segment says where.
	// Fetched now, it stays cached for the start.
	seg0, err := e.segment(ctx, m, 0)
	if err != nil {
		return fmt.Errorf("native: %s: %w", m.id, err)
	}
	smp, err := fmp4.ParseFragment(seg0, ain)
	if err != nil || len(smp) == 0 {
		return fmt.Errorf("native: %s: first audio segment: %v", m.id, err)
	}
	offset := fmp4.Time(int64(smp[0].DecodeTime), ain.Timescale)
	m.video = &videoTrack{
		url: urls[0], pl: pls[0], key: keys[0], init: init[0], width: variant.Width, height: variant.Height,
		// Pictures meet the audio sample of the same timestamp, which
		// is ain.Priming() earlier than the audio position's zero.
		offset: offset, shift: offset + ain.Priming(),
	}
	e.log.Info("video prepared", "id", m.id, "resolution", fmt.Sprintf("%dx%d", variant.Width, variant.Height),
		"codecs", variant.Codecs, "kbps", variant.Bandwidth/1000, "audio", audio.GroupID, "offset", offset, "took", time.Since(t0))
	return nil
}

// videoRendition fetches and checks a music video's media playlist at u
// and its init segment.
func (e *Engine) videoRendition(ctx context.Context, u string) (*hls.Playlist, *hls.Key, []byte, error) {
	body, err := e.fetch(ctx, u, hls.ByteRange{})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("playlist: %w", err)
	}
	pl, err := hls.Parse(body)
	if err != nil {
		return nil, nil, nil, err
	}
	first := pl.Segments[0]
	if first.Key == nil || first.Key.KeyFormat != hls.WidevineKeyFormat || len(first.Key.KID) != 16 {
		return nil, nil, nil, fmt.Errorf("playlist has no Widevine key: %w", applemusic.ErrNotPlayable)
	}
	if first.Map == nil {
		return nil, nil, nil, errors.New("playlist has no EXT-X-MAP")
	}
	for i, s := range pl.Segments {
		if s.Map == nil || *s.Map != *first.Map || s.Key == nil || s.Key.URI != first.Key.URI {
			return nil, nil, nil, fmt.Errorf("segment %d changes init segment or key, which is not supported", i)
		}
	}
	initURL, err := hls.Resolve(u, first.Map.URI)
	if err != nil {
		return nil, nil, nil, err
	}
	init, err := e.fetch(ctx, initURL, first.Map.Range)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("init segment: %w", err)
	}
	return pl, first.Key, init, nil
}

// licenseVideo makes sure host generation gen holds m's video key: a CDM
// session of its own (Apple licenses the video and audio keys
// separately). A licence Apple refuses, or one whose key the CDM cannot
// use, is playback.ErrVideoLicence. With m.licMu held.
func (e *Engine) licenseVideo(ctx context.Context, m *media, h host, gen int) error {
	v := m.video
	if v.session != "" && v.sessionGen == gen {
		return nil
	}
	sid, challenge, err := h.CreateSession(ctx, v.key.InitData)
	if err != nil {
		return err
	}
	lic, err := e.api.AcquireLicense(ctx, applemusic.LicenseRequest{
		URL: m.info.LicenseURL, Challenge: challenge, KeyURI: v.key.URI, AdamID: m.info.SongID,
	})
	if err == nil {
		err = h.UpdateSession(ctx, sid, lic.Data)
	}
	if err != nil {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = h.CloseSession(closeCtx, sid)
		cancel()
		var le *applemusic.LicenseError
		if errors.As(err, &le) || errors.Is(err, cdmhost.ErrNoUsableKey) {
			return fmt.Errorf("%w: %w", playback.ErrVideoLicence, err)
		}
		return err
	}
	v.session, v.sessionGen, v.slotToken = sid, gen, lic.SlotToken
	e.mu.Lock()
	e.sessions[sid] = m
	e.mu.Unlock()
	return nil
}

// loadVideo adds m's video to track tok, which the host just loaded at
// pos, and makes tok the track whose pictures reach the surface.
func (e *Engine) loadVideo(ctx context.Context, h host, tok uint64, m *media, pos time.Duration) error {
	if m.video == nil {
		e.vtok.Store(0)
		return nil
	}
	if err := h.LoadVideo(ctx, tok, cdmhost.VideoMedia{Init: m.video.init, KID: m.video.key.KID, Shift: m.video.shift}, pos); err != nil {
		return err
	}
	e.vtok.Store(tok)
	return nil
}

// showFrame puts a picture the host sent on the surface if it belongs to
// the video loaded last (pictures of a superseded load are dropped). It
// runs on the host's reading goroutine.
func (e *Engine) showFrame(f cdmhost.VideoFrame) {
	if e.surface == nil || f.Track == 0 || f.Track != e.vtok.Load() {
		return
	}
	e.surface.Show(f.Width, f.Height, f.Position, f.Data)
}

// clearVideo takes the last picture off the surface: the current item is
// not a video any more. With mu held or not.
func (e *Engine) clearVideo() {
	e.vtok.Store(0)
	if e.surface != nil {
		e.surface.Clear()
	}
}

// feedVideo streams m's video segments from index from to the host for
// track tok, fetching up to readAhead segments ahead, then marks the end.
func (e *Engine) feedVideo(ctx context.Context, it *item, tok uint64, h host, m *media, from int) {
	v := m.video
	type result struct {
		data []byte
		err  error
	}
	ch := make(chan result, e.readAhead)
	go func() {
		defer close(ch)
		for i := from; i < len(v.pl.Segments); i++ {
			s := v.pl.Segments[i]
			u, err := hls.Resolve(v.url, s.URI)
			var b []byte
			if err == nil {
				b, err = e.fetch(ctx, u, s.Range)
			}
			if err != nil {
				err = fmt.Errorf("native: video segment %d: %w", i, err)
			}
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
	failed := func(err error) {
		if ctx.Err() == nil && !errors.Is(err, cdmhost.ErrStale) && !errors.Is(err, cdmhost.ErrExited) {
			_ = e.fail(it, err)
		}
	}
	for r := range ch {
		err := r.err
		if err == nil {
			err = h.FeedVideo(ctx, tok, r.data)
		}
		if err != nil {
			failed(err)
			return
		}
	}
	if ctx.Err() != nil {
		return
	}
	if err := h.EndOfVideo(ctx, tok); err != nil {
		failed(err)
	}
}
