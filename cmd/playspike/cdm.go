//go:build linux || (darwin && cgo)

package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/playback/cdm"
	"github.com/coredev-uk/sona-go/internal/playback/cdmhost"
)

// cdmLibrary returns the Widevine CDM to load, the way the app chooses
// it: -cdm, else SONA_WIDEVINE_CDM (the developer override), else the CDM
// Sona downloads, installed in dir (default Sona's data directory under
// the current XDG directories), downloaded and verified there first when
// missing, with the app's load check.
func cdmLibrary(log *slog.Logger, override, dir string) (string, error) {
	if override == "" {
		override = os.Getenv(cdm.EnvOverride)
	}
	if override != "" {
		m, err := cdm.New(cdm.Options{Dir: os.TempDir(), Override: override, Logger: log})
		if err != nil {
			return "", err
		}
		lib, err := m.Library()
		if err != nil {
			return "", err
		}
		step("cdm source", "source", cdm.SourceOverride, "version", cdm.LibraryVersion(lib), "path", lib)
		return lib, nil
	}
	if dir == "" {
		paths, err := config.DefaultPaths()
		if err != nil {
			return "", err
		}
		dir = paths.WidevineDir()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	t0 := time.Now()
	inst, err := cdm.Ensure(ctx, cdm.Options{Dir: dir, Logger: log, LoadCheck: cdmhost.LoadCheck(log)})
	if err != nil {
		return "", err
	}
	step("cdm source", "source", cdm.SourceDownload, "version", inst.Version, "path", inst.Path, "took", time.Since(t0).Round(time.Millisecond))
	return inst.Path, nil
}
