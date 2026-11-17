//go:build linux || (cgo && (darwin || windows))

package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/native"
)

// runSpatialFlow plays songs through the engine with Spatial Audio on, as
// the app does: the first from the start across its key change, a seek
// back over it and on, then into each following song armed as the next
// item (gapless, or crossfaded over fade). It prints what each song plays
// (spatial or the stereo stream) and how the changes line up. Decrypted
// audio goes only to the sink.
func runSpatialFlow(log *slog.Logger, envFile string, ids []string, setting bool, fade, renewWait time.Duration, lib string, quality applemusic.Quality) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	api, sf, err := signIn(ctx, envFile)
	if err != nil {
		return err
	}
	eng := native.New(native.Options{Client: api, CDM: native.CDMPath(lib), Logger: log, IdleTimeout: -1, Quality: quality, Storefront: func() string { return sf.ID }, Sink: outputSink})
	defer eng.Close()
	eng.SetSpatial(setting)
	eng.SetCrossfade(fade)
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

	// position is the playing position now.
	position := func() (time.Duration, playback.Status) {
		s := eng.Status()
		p := s.Position
		if s.State == playback.StatePlaying {
			p += time.Since(s.UpdatedAt)
		}
		return p, s
	}
	// listen watches the playback for d, failing when it stops advancing.
	listen := func(what string, d time.Duration) error {
		prev, _ := position()
		for end := time.Now().Add(d); time.Now().Before(end); {
			time.Sleep(2 * time.Second)
			p, s := position()
			step(what, "item", s.ItemID, "state", s.State, "spatial", s.Spatial, "position", p.Round(100*time.Millisecond))
			if s.State != playback.StatePlaying || p < prev+time.Second {
				return fmt.Errorf("%s: state %s at %v after %v", what, s.State, p, prev)
			}
			prev = p
		}
		return nil
	}
	seek := func(to time.Duration) error {
		t0 := time.Now()
		rec.drain()
		if err := eng.Seek(ctx, to); err != nil {
			return err
		}
		if _, err := rec.waitFor(ctx, func(ev playback.Event) bool {
			return ev.Status.State == playback.StatePlaying && ev.Status.Position >= to && ev.Status.UpdatedAt.After(t0)
		}); err != nil {
			return fmt.Errorf("seek to %v: no audio: %w", to, err)
		}
		step("seek → audio", "to", to, "latency", time.Since(t0).Round(time.Millisecond))
		return nil
	}

	first := ids[0]
	t0 := time.Now()
	if err := eng.Play(ctx, first, 0); err != nil {
		return fmt.Errorf("play %s: %w", first, err)
	}
	st := eng.Status()
	step("play → first audio", "id", first, "spatial", st.Spatial, "duration", st.Duration.Round(time.Millisecond), "latency", time.Since(t0).Round(time.Millisecond))
	if err := listen("from the start (past the key change at 15 s)", 20*time.Second); err != nil {
		return err
	}
	if err := seek(60 * time.Second); err != nil {
		return err
	}
	if err := listen("after the key change", 6*time.Second); err != nil {
		return err
	}
	if err := seek(8 * time.Second); err != nil {
		return err
	}
	if err := listen("back before the key change", 12*time.Second); err != nil {
		return err
	}
	if renewWait > 0 {
		// Apple's licences last 15 minutes and the CDM asks for each
		// session's renewal at 13: keep playing (the song loops) until the
		// engine's log has shown both.
		step("playing until the renewals", "for", renewWait)
		for end := time.Now().Add(renewWait); time.Now().Before(end); {
			time.Sleep(10 * time.Second)
			p, s := position()
			if s.State != playback.StatePlaying {
				return fmt.Errorf("renewal wait: state %s at %v", s.State, p)
			}
			if s.Duration-p < 20*time.Second {
				if err := seek(0); err != nil {
					return err
				}
			}
		}
		p, s := position()
		step("renewal window over", "item", s.ItemID, "state", s.State, "spatial", s.Spatial, "position", p.Round(100*time.Millisecond))
	}

	for _, id := range ids[1:] {
		gapless := fade <= 0
		eng.SetNext(playback.Next{ID: id, Gapless: gapless})
		step("armed next", "id", id, "gapless", gapless, "crossfade", fade)
		dur := eng.Status().Duration
		// The queue is made once the item is fed to its end: wait for that,
		// then hurry to the last seconds.
		time.Sleep(4 * time.Second)
		rec.drain()
		if err := seek(max(dur-12*time.Second-fade, 0)); err != nil {
			return err
		}
		rec.drain()
		ev, err := rec.waitFor(ctx, func(ev playback.Event) bool { return ev.Kind == playback.EventAdvance })
		if err != nil {
			return fmt.Errorf("into %s: no advance: %w", id, err)
		}
		step("advance", "item", ev.ev.Status.ItemID, "spatial", ev.ev.Status.Spatial, "position", ev.ev.Status.Position.Round(time.Millisecond),
			"duration", ev.ev.Status.Duration.Round(time.Millisecond), "after_seek", time.Since(ev.at).Round(time.Millisecond))
		if ev.ev.Status.ItemID != id {
			return fmt.Errorf("advanced to %s, want %s", ev.ev.Status.ItemID, id)
		}
		if err := listen("after the change", 8*time.Second); err != nil {
			return err
		}
	}
	step("done", "songs", strings.Join(ids, ","))
	return eng.Stop(ctx)
}
