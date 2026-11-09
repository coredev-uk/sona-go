//go:build linux || (cgo && (darwin || windows))

package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/native"
)

// runStation plays stream station id (Apple's live radio, an episode, or
// a local broadcaster's clear stream) for listen, printing the state and
// what the stream says is on air whenever it changes, then checks that it
// played throughout. Nothing is reported to Apple: the engine plays
// without the coordinator, which is what reports plays.
func runStation(log *slog.Logger, envFile, id string, listen time.Duration, lib string) error {
	if !strings.HasPrefix(id, "ra.") {
		return fmt.Errorf("-station %q: not a station ID (ra.…)", id)
	}
	ctx, cancel := context.WithTimeout(context.Background(), listen+2*time.Minute)
	defer cancel()
	api, sf, err := signIn(ctx, envFile)
	if err != nil {
		return err
	}
	eng := native.New(native.Options{Client: api, CDM: native.CDMPath(lib), Logger: log, IdleTimeout: -1, Storefront: func() string { return sf.ID }, Sink: outputSink})
	defer func() { _ = eng.Close() }()
	if err := eng.SetOutputDevice(ctx, outputDevice); err != nil {
		return err
	}
	t0 := time.Now()
	var changes atomic.Int32
	go func() {
		var (
			on    playback.OnAir
			state playback.State
		)
		for ev := range eng.Events() {
			s := ev.Status
			if s.State != state {
				state = s.State
				step("state", "state", s.State, "at", time.Since(t0).Round(time.Millisecond), "err", fmt.Sprintf("%q", s.Err))
			}
			if s.OnAir != on {
				on = s.OnAir
				changes.Add(1)
				step("on air", "title", fmt.Sprintf("%q", on.Title), "artist", fmt.Sprintf("%q", on.Artist),
					"at", time.Since(t0).Round(time.Second), "position", s.Position.Round(time.Second))
			}
		}
	}()
	if err := eng.Play(ctx, id, 0); err != nil {
		return fmt.Errorf("play %s: %w", id, err)
	}
	step("play → first audio", "station", id, "latency", time.Since(t0).Round(time.Millisecond))
	info, _ := eng.Host()
	step("host", "pid", info.PID, "cdm", info.CDMVersion, "sink", info.Sink)
	time.Sleep(2 * time.Second)
	audioStream(info.PID)
	realtime(eng)

	start := eng.Status()
	deadline := time.Now().Add(listen)
	for time.Now().Before(deadline) {
		time.Sleep(min(15*time.Second, time.Until(deadline)))
		s := eng.Status()
		pos := s.Position
		if s.State == playback.StatePlaying {
			pos += time.Since(s.UpdatedAt)
		}
		step("checkpoint", "state", s.State, "position", pos.Round(100*time.Millisecond), "duration", s.Duration)
	}
	end := eng.Status()
	played := end.Position + time.Since(end.UpdatedAt) - start.Position
	step("listened", "played", played.Round(time.Second), "of", listen, "on_air_changes", changes.Load())
	if end.State != playback.StatePlaying {
		return fmt.Errorf("station %s is %s after %v: %s", id, end.State, listen, end.Err)
	}
	if played < listen*9/10 {
		return fmt.Errorf("station %s played %v of %v", id, played.Round(time.Second), listen)
	}
	return nil
}
