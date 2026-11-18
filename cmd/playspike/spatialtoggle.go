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

// runSpatialToggle plays each song in stereo, turns Spatial Audio on while
// it plays and then off again, as the setting does, and prints what the
// engine does with the playing song: the position and mix before and after
// each change, and why a song stays in stereo (Status.SpatialMiss). The
// position after a switch should be where the song was, plus the time that
// went by. Decrypted audio goes only to the sink.
func runSpatialToggle(log *slog.Logger, envFile string, ids []string, at time.Duration, lib string, quality applemusic.Quality) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	api, sf, err := signIn(ctx, envFile)
	if err != nil {
		return err
	}
	eng := native.New(native.Options{Client: api, CDM: native.CDMPath(lib), Logger: log, IdleTimeout: -1, Quality: quality, Storefront: func() string { return sf.ID }, Sink: outputSink})
	defer func() { _ = eng.Close() }()
	eng.SetSpatial(false)
	if err := eng.SetOutputDevice(ctx, outputDevice); err != nil {
		return err
	}
	go func() {
		for range eng.Events() { // the engine's channel must be drained
		}
	}()

	position := func() (time.Duration, playback.Status) {
		s := eng.Status()
		p := s.Position
		if s.State == playback.StatePlaying {
			p += time.Since(s.UpdatedAt)
		}
		return p, s
	}
	// settle waits until the playing song is Playing and done (true) says so, or for max.
	settle := func(max time.Duration, done func(playback.Status) bool) playback.Status {
		var s playback.Status
		for end := time.Now().Add(max); time.Now().Before(end); time.Sleep(100 * time.Millisecond) {
			if s = eng.Status(); s.State == playback.StatePlaying && done(s) {
				break
			}
		}
		return s
	}
	report := func(what, id string, before time.Duration, since time.Time) {
		p, s := position()
		expect := before + time.Since(since)
		step(what, "id", id, "state", s.State, "spatial", s.Spatial, "miss", s.SpatialMiss, "position", p.Round(100*time.Millisecond),
			"expected", expect.Round(100*time.Millisecond), "off_by", (p - expect).Round(10*time.Millisecond))
	}

	for _, id := range ids {
		if err := eng.Play(ctx, id, 0); err != nil {
			return fmt.Errorf("play %s: %w", id, err)
		}
		if err := eng.Seek(ctx, at); err != nil {
			return fmt.Errorf("seek %s: %w", id, err)
		}
		settle(20*time.Second, func(s playback.Status) bool { return s.Position >= at })
		time.Sleep(3 * time.Second)
		before, s := position()
		step("stereo, setting off", "id", id, "spatial", s.Spatial, "miss", s.SpatialMiss, "position", before.Round(100*time.Millisecond))

		t0 := time.Now()
		eng.SetSpatial(true)
		s = settle(15*time.Second, func(s playback.Status) bool { return s.Spatial || s.SpatialMiss != "" })
		step("setting on", "id", id, "spatial", s.Spatial, "miss", s.SpatialMiss, "took", time.Since(t0).Round(10*time.Millisecond))
		time.Sleep(3 * time.Second)
		report("3 s later", id, before, t0)

		before, _ = position()
		t1 := time.Now()
		eng.SetSpatial(false)
		s = settle(15*time.Second, func(s playback.Status) bool { return !s.Spatial })
		step("setting off", "id", id, "spatial", s.Spatial, "miss", s.SpatialMiss, "took", time.Since(t1).Round(10*time.Millisecond))
		time.Sleep(3 * time.Second)
		report("3 s later", id, before, t1)
		if err := eng.Stop(ctx); err != nil {
			return err
		}
	}
	step("done", "songs", strings.Join(ids, ","))
	return nil
}
