//go:build linux || (darwin && cgo)

package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/cdm"
	"github.com/coredev-uk/sona-go/internal/playback/cdmhost"
	"github.com/coredev-uk/sona-go/internal/playback/native"
)

// Linux and macOS: the Widevine CDM host engine (internal/playback/native),
// loading the CDM Sona downloads from Google (internal/playback/cdm).

// childMain runs the CDM host child when this process is one (it never
// returns there).
func childMain() { cdmhost.ChildMain() }

// newEngine is the CDM host engine, loading the CDM src names; it starts
// no process until the first play, and reports no_cdm until the download
// is installed.
func newEngine(api *applemusic.Client, cur config.Settings, log *slog.Logger, src *cdm.Manager) playback.Engine {
	return native.New(native.Options{
		Client:      api,
		CDM:         src,
		Logger:      log,
		IdleTimeout: time.Duration(cur.EngineIdleMinutes) * time.Minute,
		Quality:     applemusic.Quality(cur.AudioQuality),
	})
}

// cdmLoadCheck loads a newly downloaded CDM in a sandboxed probe child,
// as a playback host would, before it becomes current (cdm.Options).
func cdmLoadCheck(log *slog.Logger) func(ctx context.Context, lib string) error {
	return cdmhost.LoadCheck(log)
}
