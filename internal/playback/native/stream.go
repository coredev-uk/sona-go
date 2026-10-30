package native

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/playback/cdmhost"
	"github.com/coredev-uk/sona-go/internal/playback/fmp4"
	"github.com/coredev-uk/sona-go/internal/playback/hls"
)

// Radio streams: live stations (Apple Music 1, …) and radio episodes play
// through GET /v1/play/assets → an HLS master playlist of CMAF AAC
// variants, encrypted 'cbcs' (SAMPLE-AES) with a Widevine pssh in the
// playlist and licences from the radio key server. Live playlists are a
// sliding window that is reloaded while playing; each programme brings a
// new init segment and key.

// liveStale is how long a prepared live stream's playlist stays usable
// for a (re)start; older ones are prepared again from the live edge.
const liveStale = 30 * time.Second

// liveEdgeSegments is how many segments from the end of a live window
// playback starts (HLS recommends at least three target durations).
const liveEdgeSegments = 3

// stream is the radio part of a prepared media.
type stream struct {
	live     bool
	prepared time.Time
	start    int // segment of pl to start a live stream from
	initURL  string
}

// IsStation reports whether id is a radio station or episode ("ra.…"),
// which Play streams instead of fetching webPlayback.
func IsStation(id string) bool { return strings.HasPrefix(id, "ra.") }

func (m *media) stale() bool {
	return m.stream != nil && m.stream.live && time.Since(m.stream.prepared) > liveStale
}

// startSegment is the segment playback from pos begins with.
func (m *media) startSegment(pos time.Duration) int {
	if m.stream != nil && m.stream.live {
		return m.stream.start
	}
	return m.pl.SegmentAt(min(pos, m.duration))
}

// prepareStream resolves station m.id to its stream and fetches the media
// playlist and the init segment to start with.
func (e *Engine) prepareStream(ctx context.Context, m *media) error {
	asset, err := e.api.RadioAssets(ctx, applemusic.StationPlayParams{ID: m.id, HasDRM: true})
	if err != nil {
		return err
	}
	if asset.KeyServerURL == "" || !strings.Contains(asset.URL, ".m3u8") {
		return fmt.Errorf("native: %s is a third-party stream, which this engine does not play: %w", m.id, applemusic.ErrNotPlayable)
	}
	plURL := asset.URL
	body, err := e.fetch(ctx, plURL, hls.ByteRange{})
	if err != nil {
		return fmt.Errorf("native: master playlist: %w", err)
	}
	if hls.IsMaster(body) {
		vs, err := hls.ParseMaster(body)
		if err != nil {
			return err
		}
		if plURL, err = hls.Resolve(plURL, hls.BestAAC(vs).URI); err != nil {
			return err
		}
		if body, err = e.fetch(ctx, plURL, hls.ByteRange{}); err != nil {
			return fmt.Errorf("native: playlist: %w", err)
		}
	}
	pl, err := hls.Parse(body)
	if err != nil {
		return err
	}
	st := &stream{live: !pl.Ended, prepared: time.Now()}
	if st.live {
		st.start = max(len(pl.Segments)-liveEdgeSegments, 0)
	}
	first := pl.Segments[st.start]
	if first.Key == nil || first.Key.KeyFormat != hls.WidevineKeyFormat || len(first.Key.InitData) == 0 {
		return fmt.Errorf("native: %s: stream has no Widevine key: %w", m.id, applemusic.ErrNotPlayable)
	}
	if first.Map == nil {
		return fmt.Errorf("native: %s: stream has no EXT-X-MAP", m.id)
	}
	if st.initURL, err = hls.Resolve(plURL, first.Map.URI); err != nil {
		return err
	}
	init, err := e.fetch(ctx, st.initURL, first.Map.Range)
	if err != nil {
		return fmt.Errorf("native: init segment: %w", err)
	}
	m.info = applemusic.PlaybackInfo{SongID: m.id, PlaylistURL: plURL, LicenseURL: asset.KeyServerURL, CertURL: asset.CertURL}
	m.pl, m.key, m.init, m.stream = pl, first.Key, init, st
	if !st.live {
		m.duration = pl.Duration()
	}
	e.log.Debug("prepared stream", "id", m.id, "live", st.live, "segments", len(pl.Segments))
	return nil
}

// streamLicense makes sure host generation gen holds key k of stream m
// (licensed sessions are kept per key, as live keys rotate).
func (e *Engine) streamLicense(ctx context.Context, m *media, k *hls.Key, h host, gen int) error {
	m.licMu.Lock()
	defer m.licMu.Unlock()
	id := string(k.InitData)
	if m.keyGen != gen {
		m.keySessions, m.keyGen = map[string]string{}, gen
	}
	if _, ok := m.keySessions[id]; ok {
		return nil
	}
	if err := e.certificate(ctx, h, gen, m.info.CertURL); err != nil {
		return err
	}
	sid, challenge, err := h.CreateSession(ctx, k.InitData)
	if err != nil {
		return err
	}
	lic, err := e.api.AcquireRadioLicense(ctx, applemusic.RadioLicenseRequest{
		URL: m.info.LicenseURL, Challenge: challenge, KeyURI: k.URI, SlotToken: m.slotToken,
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
	if lic.SlotToken != "" {
		m.slotToken = lic.SlotToken
	}
	m.keySessions[id] = sid
	if m.sessionKeys == nil {
		m.sessionKeys = map[string]*hls.Key{}
	}
	m.sessionKeys[sid] = k
	if m.session == "" || m.sessionGen != gen {
		m.session, m.sessionGen = sid, gen
	}
	e.log.Debug("stream licensed", "id", m.id, "sessions", len(m.keySessions))
	e.mu.Lock()
	e.sessions[sid] = m
	e.mu.Unlock()
	return nil
}

// renewStream answers a renewal message for a stream session.
func (e *Engine) renewStream(ctx context.Context, m *media, sid string, challenge []byte) (applemusic.License, error) {
	k := m.key
	if sk := m.sessionKeys[sid]; sk != nil {
		k = sk
	}
	return e.api.AcquireRadioLicense(ctx, applemusic.RadioLicenseRequest{
		URL: m.info.LicenseURL, Challenge: challenge, KeyURI: k.URI, SlotToken: m.slotToken,
	})
}

// feedStream streams m's segments to the host for track tok from the
// segment holding pos (live: from the live edge), reloading a live
// playlist as it slides. Each fragment is placed right after the previous
// one on the track timeline, so discontinuities and the jump to a live
// edge stay gapless; new init segments and keys are applied on the way.
func (e *Engine) feedStream(ctx context.Context, it *item, tok uint64, h host, gen int, m *media, pos time.Duration) {
	err := e.runStream(ctx, tok, h, gen, m, pos)
	if err == nil || ctx.Err() != nil || errors.Is(err, cdmhost.ErrStale) || errors.Is(err, cdmhost.ErrExited) {
		return
	}
	e.fail(it, err)
}

func (e *Engine) runStream(ctx context.Context, tok uint64, h host, gen int, m *media, pos time.Duration) error {
	in, err := fmp4.ParseInit(m.init)
	if err != nil {
		return err
	}
	var (
		pl      = m.pl
		from    = m.startSegment(pos)
		initURL = m.stream.initURL
		keyID   = string(m.key.InitData)
		next    time.Duration // track time the next fragment starts at
		nextSeq int
	)
	if !m.stream.live {
		next = pl.Segments[from].Start
	}
	nextSeq = pl.Segments[from].Seq
	for {
		fed := false
		for i, s := range pl.Segments {
			if s.Seq < nextSeq {
				continue
			}
			var newInit []byte
			if s.Map != nil {
				u, err := hls.Resolve(m.info.PlaylistURL, s.Map.URI)
				if err != nil {
					return err
				}
				if u != initURL {
					if newInit, err = e.fetch(ctx, u, s.Map.Range); err != nil {
						return fmt.Errorf("native: init segment: %w", err)
					}
					if in, err = fmp4.ParseInit(newInit); err != nil {
						return err
					}
					initURL = u
				}
			}
			if s.Key != nil && string(s.Key.InitData) != keyID {
				if err := e.streamLicense(ctx, m, s.Key, h, gen); err != nil {
					return err
				}
				keyID = string(s.Key.InitData)
			}
			var data []byte
			if pl == m.pl {
				data, err = e.segment(ctx, m, i)
			} else {
				var u string
				if u, err = hls.Resolve(m.info.PlaylistURL, s.URI); err == nil {
					data, err = e.fetch(ctx, u, s.Range)
				}
			}
			if err != nil {
				return fmt.Errorf("native: segment %d: %w", s.Seq, err)
			}
			first, dur, err := fragmentSpan(data, in)
			if err != nil {
				return fmt.Errorf("native: segment %d: %w", s.Seq, err)
			}
			frag := cdmhost.StreamFragment{Init: newInit, Data: data, Shift: first - next}
			if s.Key != nil && isZero(in.KID) {
				frag.KID = s.Key.KID
			}
			if err := h.FeedStream(ctx, tok, frag); err != nil {
				return err
			}
			next += dur
			nextSeq = s.Seq + 1
			fed = true
		}
		if !m.stream.live {
			return h.EndOfTrack(ctx, tok)
		}
		if !fed {
			// Nothing new yet: wait for the window to slide.
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(max(pl.TargetDuration/2, time.Second)):
			}
		}
		body, err := e.fetch(ctx, m.info.PlaylistURL, hls.ByteRange{})
		if err != nil {
			return fmt.Errorf("native: playlist: %w", err)
		}
		if pl, err = hls.Parse(body); err != nil {
			return err
		}
		if nextSeq < pl.MediaSequence {
			// Fell out of the window (a long pause): rejoin at the edge.
			nextSeq = pl.Segments[max(len(pl.Segments)-liveEdgeSegments, 0)].Seq
			e.log.Info("live stream rejoined at the edge", "id", m.id)
		}
		if pl.Ended && nextSeq > pl.Segments[len(pl.Segments)-1].Seq {
			return h.EndOfTrack(ctx, tok)
		}
	}
}

// fragmentSpan returns a fragment's first presentation time and total
// duration (encrypted payloads are not touched).
func fragmentSpan(data []byte, in *fmp4.Init) (first, dur time.Duration, err error) {
	samples, err := fmp4.ParseFragment(data, in)
	if err != nil {
		return 0, 0, err
	}
	if len(samples) == 0 {
		return 0, 0, errors.New("empty fragment")
	}
	var units int64
	for _, s := range samples {
		units += int64(s.Duration)
	}
	return fmp4.Time(samples[0].PTS(), in.Timescale), fmp4.Time(units, in.Timescale), nil
}

// isZero reports an absent or all-zero key ID (Apple's radio episodes
// leave tenc's default key ID zero; the playlist names the key).
func isZero(kid []byte) bool {
	for _, b := range kid {
		if b != 0 {
			return false
		}
	}
	return true
}
