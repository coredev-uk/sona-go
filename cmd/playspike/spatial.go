//go:build linux || (cgo && (darwin || windows))

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/playback/cdmhost"
	"github.com/coredev-uk/sona-go/internal/playback/fmp4"
	"github.com/coredev-uk/sona-go/internal/playback/hls"
)

// runSpatial plays one AAC rendition of a song's enhancedHls master (the
// Spatial Audio binaural render by default) through the CDM host: it
// licenses every Widevine key the media playlist uses, loads the init
// segment and feeds each segment with its key ID, then plays for listen
// and reports the position, buffering and any error. Decrypted audio goes
// only to the sink, as in Sona.
func runSpatial(log *slog.Logger, envFile, songID, variant string, listen time.Duration, lib string) error {
	ctx, cancel := context.WithTimeout(context.Background(), listen+3*time.Minute)
	defer cancel()
	api, sf, err := signIn(ctx, envFile)
	if err != nil {
		return err
	}
	song, err := applemusic.AMPResource[enhancedSong](ctx, api, sf.ID, "songs", songID, applemusic.AMPQuery{Extend: []string{"extendedAssetUrls"}})
	if err != nil {
		return fmt.Errorf("catalog song %s: %w", songID, err)
	}
	a := song.Attributes
	step("song", "id", song.ID, "name", fmt.Sprintf("%q", a.Name), "artist", fmt.Sprintf("%q", a.ArtistName), "audioTraits", a.AudioTraits)
	master := a.ExtendedAssetURLs.EnhancedHLS
	if master == "" {
		return fmt.Errorf("song %s has no extendedAssetUrls.enhancedHls", songID)
	}
	body, err := get(ctx, master)
	if err != nil {
		return fmt.Errorf("enhancedHls master: %w", err)
	}
	variants, err := hls.ParseMaster(body)
	if err != nil {
		return err
	}
	var v *hls.Variant
	for i := range variants {
		if variants[i].Audio == variant {
			v = &variants[i]
			break
		}
	}
	if v == nil {
		return fmt.Errorf("song %s: no %s rendition in its enhancedHls master", songID, variant)
	}
	u, err := hls.Resolve(master, v.URI)
	if err != nil {
		return err
	}
	if body, err = get(ctx, u); err != nil {
		return err
	}
	pl, err := hls.Parse(body)
	if err != nil {
		return err
	}
	step("rendition", "name", variant, "codecs", v.Codecs, "kbps", v.Bandwidth/1000, "segments", len(pl.Segments), "duration", pl.Duration().Round(time.Millisecond))

	h, err := cdmhost.Start(ctx, cdmhost.Options{CDMPath: lib, Sink: outputSink, Device: outputDevice, Logger: log})
	if err != nil {
		return fmt.Errorf("cdm host: %w", err)
	}
	defer func() { _ = h.Close() }()
	cert, err := api.WidevineCert(ctx, "")
	if err != nil {
		return err
	}
	if err := h.SetServerCertificate(ctx, cert); err != nil {
		return err
	}
	licensed := map[string]bool{}
	for i, s := range pl.Segments {
		k := s.Key
		if k == nil || k.KeyFormat != hls.WidevineKeyFormat {
			return fmt.Errorf("segment %d has no Widevine key (FairPlay only): this song can't play spatial", i)
		}
		if licensed[k.URI] {
			continue
		}
		sid, challenge, err := h.CreateSession(ctx, k.InitData)
		if err != nil {
			return err
		}
		lic, err := api.AcquireLicense(ctx, applemusic.LicenseRequest{Challenge: challenge, KeyURI: k.URI, AdamID: song.ID})
		if err != nil {
			return fmt.Errorf("licence for %s: %w", k.URI, err)
		}
		if err := h.UpdateSession(ctx, sid, lic.Data); err != nil {
			return fmt.Errorf("licence for %s: %w", k.URI, err)
		}
		licensed[k.URI] = true
		step("licensed", "key", k.URI, "kid", fmt.Sprintf("%x", k.KID), "from_segment", i)
	}

	inits := map[hls.Map][]byte{}
	initOf := func(m *hls.Map) ([]byte, error) {
		if b, ok := inits[*m]; ok {
			return b, nil
		}
		mu, err := hls.Resolve(u, m.URI)
		if err != nil {
			return nil, err
		}
		b, err := getRange(ctx, mu, m.Range)
		if err == nil {
			inits[*m] = b
		}
		return b, err
	}
	first := pl.Segments[0]
	if first.Map == nil {
		return fmt.Errorf("playlist has no EXT-X-MAP")
	}
	init, err := initOf(first.Map)
	if err != nil {
		return fmt.Errorf("init segment: %w", err)
	}
	in, err := fmp4.ParseInit(init)
	if err != nil {
		return fmt.Errorf("init segment: %w", err)
	}
	step("init", "scheme", in.Scheme, "kid", fmt.Sprintf("%x", in.KID), "codec", in.Codec, "rate", in.SampleRate, "channels", in.Channels, "priming", in.Priming())

	const track = 1
	if err := h.Load(ctx, track, cdmhost.Media{Init: init, Duration: pl.Duration() - in.Priming()}, 0); err != nil {
		return err
	}
	fed := make(chan error, 1)
	go func() {
		for i, s := range pl.Segments {
			su, err := hls.Resolve(u, s.URI)
			if err != nil {
				fed <- err
				return
			}
			data, err := getRange(ctx, su, s.Range)
			if err != nil {
				fed <- fmt.Errorf("segment %d: %w", i, err)
				return
			}
			frag := cdmhost.StreamFragment{Data: data, KID: s.Key.KID}
			log.Debug("spatial segment", "i", i, "url", su, "offset", s.Range.Offset, "length", s.Range.Length)
			if s.Map != nil && *s.Map != *first.Map {
				if frag.Init, err = initOf(s.Map); err != nil {
					fed <- fmt.Errorf("segment %d init: %w", i, err)
					return
				}
			}
			fin := in
			if frag.Init != nil {
				if fin, err = fmp4.ParseInit(frag.Init); err != nil {
					fed <- fmt.Errorf("segment %d init: %w", i, err)
					return
				}
			}
			if smp, err := fmp4.ParseFragment(data, fin); err != nil || len(smp) == 0 {
				step("segment", "i", i, "parse", err)
			} else {
				step("segment", "i", i, "bytes", len(data), "new_init", frag.Init != nil, "kid", fmt.Sprintf("%x", s.Key.KID),
					"first_pts", fmp4.Time(smp[0].PTS(), fin.Timescale).Round(time.Millisecond), "samples", len(smp), "encrypted", smp[0].Encrypted(),
					"playlist_start", s.Start.Round(time.Millisecond))
			}
			if err := h.FeedStream(ctx, track, frag); err != nil {
				fed <- fmt.Errorf("segment %d: %w", i, err)
				return
			}
		}
		fed <- h.EndOfTrack(ctx, track)
	}()
	if err := h.Play(ctx); err != nil {
		return err
	}
	t0 := time.Now()
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	deadline := time.After(listen)
	var last time.Duration
	for {
		select {
		case err := <-fed:
			if err != nil {
				return err
			}
			step("fed", "segments", len(pl.Segments), "after", time.Since(t0).Round(time.Millisecond))
			fed = nil
		case ev := <-h.Events():
			switch ev.Kind {
			case cdmhost.EventStarted:
				step("started", "after", time.Since(t0).Round(time.Millisecond))
			case cdmhost.EventBuffering:
				step("buffering", "stalled", ev.Buffering, "position", ev.Position.Round(time.Millisecond))
			case cdmhost.EventError:
				return fmt.Errorf("playback: %s", ev.Err)
			case cdmhost.EventEnded:
				step("ended", "after", time.Since(t0).Round(time.Millisecond))
				return nil
			}
		case <-tick.C:
			pos, playing, err := h.Position(ctx)
			if err != nil {
				return err
			}
			step("position", "pos", pos.Round(time.Millisecond), "playing", playing, "advanced", (pos - last).Round(time.Millisecond))
			last = pos
		case <-deadline:
			pos, _, _ := h.Position(ctx)
			step("listened", "for", listen, "position", pos.Round(time.Millisecond))
			if pos < listen/2 {
				return fmt.Errorf("played %s in %s", pos, listen)
			}
			return nil
		case <-h.Done():
			return fmt.Errorf("cdm host ended: %v", h.Err())
		}
	}
}

// getRange fetches r of a CDN URL (all of it for a zero range).
func getRange(ctx context.Context, u string, r hls.ByteRange) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	if hdr := r.Header(); hdr != "" {
		req.Header.Set("Range", hdr)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return nil, fmt.Errorf("GET %s: HTTP %d", u, resp.StatusCode)
	}
	var b bytes.Buffer
	_, err = io.Copy(&b, resp.Body)
	return b.Bytes(), err
}
