//go:build linux || (darwin && cgo)

package main

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/auth/devtoken"
	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/native"
)

// maxPreview bounds what counts as a preview's length: Apple's clips are
// about 30 s, far shorter than any full track this checks.
const maxPreview = 45 * time.Second

// runPreview plays songs as their 30-second previews (native.Options.Preview)
// with the developer token alone: no Music User Token, no .env, no CDM
// (none is downloaded) and no licence. It checks that the host loaded no
// CDM, that the preview plays and advances, that it plays out at its end
// and the next one follows (as the coordinator starts it), and that no
// webPlayback, certificate or licence request went out.
func runPreview(log *slog.Logger, sf, songID, term, nextID, nextTerm string, listen time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	api := applemusic.New(&http.Client{Transport: licences}, devtoken.NewWebPlayer(nil), func() string { return "" })
	step("storefront (no MUT)", "id", sf)
	song, err := resolve(ctx, api, sf, songID, term)
	if err != nil {
		return err
	}
	step("song", "id", song.ID, "name", fmt.Sprintf("%q", song.Attributes.Name), "catalog_ms", song.Attributes.DurationInMillis)

	eng := native.New(native.Options{Client: api, Logger: log, IdleTimeout: -1, Preview: true, Storefront: func() string { return sf }})
	defer func() { _ = eng.Close() }()
	if err := eng.SetOutputDevice(ctx, outputDevice); err != nil {
		return err
	}
	rec := &recorder{events: make(chan stamped, 256)}
	go func() {
		for ev := range eng.Events() {
			rec.events <- stamped{time.Now(), ev}
		}
		close(rec.events)
	}()

	// 1. Cold play: a host with no CDM + catalog lookup + the clip.
	t0 := time.Now()
	if err := eng.Play(ctx, song.ID, 0); err != nil {
		return fmt.Errorf("play: %w", err)
	}
	coldLatency := time.Since(t0)
	info, _ := eng.Host()
	st := eng.Status()
	step("play → first audio (cold)", "latency", coldLatency.Round(time.Millisecond), "state", st.State, "preview", st.Preview)
	step("host", "pid", info.PID, "cdm", cmp.Or(info.CDMPath, "none"), "cdm_interface", info.CDMInterface)
	step("audio", "gstreamer", info.GStreamer, "decoder", info.Decoder, "sink", info.Sink)
	step("sandbox", append([]any{"kind", info.Sandbox.Kind()}, info.Sandbox.Attrs()...)...)
	if !st.Preview {
		return fmt.Errorf("status does not say preview: %+v", st)
	}
	if info.CDMPath != "" || info.CDMInterface != 0 {
		return fmt.Errorf("the preview host loaded a CDM: %s", info.CDMPath)
	}

	time.Sleep(2 * time.Second)
	audioStream(info.PID)
	realtime(eng)

	// 2. Watch it advance.
	start := eng.Status()
	from := start.Position + time.Since(start.UpdatedAt)
	deadline := time.Now().Add(listen)
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		s := eng.Status()
		pos := s.Position
		if s.State == playback.StatePlaying {
			pos += time.Since(s.UpdatedAt)
		}
		step("checkpoint", "state", s.State, "position", pos.Round(100*time.Millisecond), "duration", s.Duration.Round(10*time.Millisecond))
	}
	s := eng.Status()
	final := s.Position + time.Since(s.UpdatedAt)
	if s.State != playback.StatePlaying || final-from < listen-2*time.Second {
		return fmt.Errorf("preview did not advance: state %s, %v → %v over %v", s.State, from, final, listen)
	}
	dur := s.Duration
	if dur <= 0 || dur > maxPreview {
		return fmt.Errorf("preview length %v: not a preview", dur)
	}
	step("preview advancing", "advanced", (final - from).Round(100*time.Millisecond), "duration", dur.Round(10*time.Millisecond))
	memory("memory while playing", info.PID)

	// 3. The end: the preview plays out and the next one starts on the
	// Ended event, as the coordinator does.
	next, err := resolve(ctx, api, sf, nextID, nextTerm)
	if err != nil {
		return err
	}
	if err := eng.Preload(ctx, next.ID); err != nil {
		return fmt.Errorf("preload: %w", err)
	}
	if err := eng.Seek(ctx, dur-5*time.Second); err != nil {
		return fmt.Errorf("seek to end: %w", err)
	}
	rec.drain()
	ended, err := rec.waitFor(ctx, func(ev playback.Event) bool {
		return ev.Kind == playback.EventState && ev.Status.State == playback.StateEnded
	})
	if err != nil {
		return fmt.Errorf("waiting for the end: %w", err)
	}
	step("preview ended", "at", ended.ev.Status.Position.Round(10*time.Millisecond))
	tn := time.Now()
	if err := eng.Play(ctx, next.ID, 0); err != nil {
		return fmt.Errorf("play next: %w", err)
	}
	st = eng.Status()
	step("next preview", "id", next.ID, "name", fmt.Sprintf("%q", next.Attributes.Name), "ended_event→audio", st.UpdatedAt.Sub(ended.at).Round(time.Millisecond),
		"play_call→audio", time.Since(tn).Round(time.Millisecond), "preview", st.Preview)
	time.Sleep(3 * time.Second)
	s = eng.Status()
	if s.State != playback.StatePlaying || !s.Preview {
		return fmt.Errorf("next preview: %+v", s)
	}
	step("next preview playing", "position", (s.Position + time.Since(s.UpdatedAt)).Round(100*time.Millisecond))
	if err := eng.Stop(ctx); err != nil {
		return err
	}

	lic, drm := licences.requests()
	step("full-track requests", "licence", lic, "webPlayback+certificate", drm)
	if lic != 0 || drm != 0 {
		return fmt.Errorf("previews made %d licence and %d webPlayback or certificate requests", lic, drm)
	}
	return nil
}

// realtime prints the scheduling the host's audio threads got: the parent
// asks for it once the output plays, so it is known by the time the
// stream shows in audioStream.
func realtime(eng *native.Engine) {
	info, _ := eng.Host()
	rt := info.Realtime
	step("realtime", "state", cmp.Or(rt.State, "not asked"), "via", rt.Via, "priority", rt.Priority, "threads", rt.Threads,
		"reason", fmt.Sprintf("%q", rt.Reason))
}
