//go:build linux || (cgo && (darwin || windows))

package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/cdm"
	"github.com/coredev-uk/sona-go/internal/playback/cdmhost"
	"github.com/coredev-uk/sona-go/internal/playback/native"
	"github.com/coredev-uk/sona-go/internal/playback/video"
)

// Linux, macOS and Windows (with cgo): the Widevine CDM host engine
// (internal/playback/native), loading the CDM Sona downloads from Google
// (internal/playback/cdm).

// childMain runs the CDM host child when this process is one (it never
// returns there).
func childMain() { cdmhost.ChildMain() }

// newEngine is the CDM host engine, loading the CDM src names; it starts
// no process until the first play, and reports no_cdm until the download
// is installed. Music video pictures go to vid. With SONA_PREVIEW on
// (playback.EnvPreview) it plays every song as its 30-second preview
// from storefront's catalog instead, with no CDM.
func newEngine(api *applemusic.Client, cur config.Settings, log *slog.Logger, src *cdm.Manager, storefront func() string, vid *video.Surface) playback.Engine {
	preview, err := playback.PreviewEnv(os.Getenv(playback.EnvPreview))
	if err != nil {
		log.Warn("playback", "err", err)
	}
	if preview {
		log.Info("playback: preview mode: every song plays its 30-second preview", "env", playback.EnvPreview)
	}
	return native.New(native.Options{
		Client:      api,
		CDM:         src,
		Logger:      log,
		IdleTimeout: time.Duration(cur.EngineIdleMinutes) * time.Minute,
		Quality:     applemusic.Quality(cur.AudioQuality),
		Preview:     preview,
		Storefront:  storefront,
		Video:       vid,
	})
}

// cdmLoadCheck loads a newly downloaded CDM in a sandboxed probe child,
// as a playback host would but without its audio stack, before it
// becomes current (cdm.Options).
func cdmLoadCheck(log *slog.Logger) func(ctx context.Context, lib string) error {
	return cdmhost.LoadCheck(log)
}

// prepareAudioTimeout bounds prepareAudio's child.
const prepareAudioTimeout = 5 * time.Minute

// prepareAudio readies GStreamer's plugin registry in a sandboxed child
// (cdmhost.PrepareAudio), so the first play does not wait for the plugin
// scan. It runs once, after the first paint.
func prepareAudio(ctx context.Context, log *slog.Logger) {
	ctx, cancel := context.WithTimeout(ctx, prepareAudioTimeout)
	defer cancel()
	t0 := time.Now()
	info, err := cdmhost.PrepareAudio(ctx, cdmhost.Options{Logger: log})
	took := time.Since(t0).Round(time.Millisecond)
	if err != nil {
		if ctx.Err() == nil || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			log.Warn("playback: audio stack not prepared; the first play prepares it", "err", err, "took", took)
		}
		return
	}
	log.Info("playback: audio stack ready", "gstreamer", info.GStreamer, "took", took)
}
