//go:build linux || (darwin && cgo)

package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/playback/cdm"
	"github.com/coredev-uk/sona-go/internal/playback/cdmhost"
)

// cdmLibrary returns the Widevine CDM to load, the way the app gets it:
// the CDM Sona downloads, installed in dir (default Sona's data directory under
// the current XDG directories), downloaded and verified there first when
// missing, with the app's load check.
func cdmLibrary(log *slog.Logger, dir string) (string, error) {
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
	step("cdm source", "source", "download", "version", inst.Version, "path", inst.Path, "took", time.Since(t0).Round(time.Millisecond))
	return inst.Path, nil
}
