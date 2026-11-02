//go:build !linux && !(darwin && cgo)

package main

import (
	"context"
	"log/slog"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/cdm"
	"github.com/coredev-uk/sona-go/internal/playback/video"
)

// Windows, and macOS built without cgo (which Wails needs there anyway):
// no playback engine.

// childMain does nothing: there is no CDM host child here.
func childMain() {}

// newEngine is the engine for a platform without one: every play fails
// with playback.ErrUnsupported, previews and videos too. It needs no CDM,
// so nothing is downloaded.
func newEngine(*applemusic.Client, config.Settings, *slog.Logger, *cdm.Manager, func() string, *video.Surface) playback.Engine {
	return playback.NewUnsupported()
}

// cdmLoadCheck is nil: there is no playback host here to load a CDM, so a
// download is only checked statically (its signatures and binary format).
func cdmLoadCheck(*slog.Logger) func(ctx context.Context, lib string) error { return nil }

// prepareAudio does nothing: there is no audio stack to prepare here.
func prepareAudio(context.Context, *slog.Logger) {}
