// Command cdmfetch installs Google's Widevine CDM the way Sona does
// (internal/playback/cdm): Mozilla's manifest, Google's package, the
// SHA-512 and CRX3 checks, the extraction and the binary-format check. It
// installs into Sona's data directory (the current XDG directories), or
// -dir, and prints the version and library. CI uses it to fill its CDM
// cache, and -platform checks the packages for other systems from any
// computer (no load check there).
//
// Usage: cdmfetch [-dir DIR] [-platform goos/goarch] [-version]
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/playback/cdm"
)

func main() {
	dir := flag.String("dir", "", "install directory (default: Sona's, <data>/widevine)")
	platform := flag.String("platform", "", "goos/goarch to install for (default: this computer's)")
	version := flag.Bool("version", false, "print the manifest's version for the platform and exit (no download)")
	flag.Parse()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(log, *dir, *platform, *version); err != nil {
		log.Error("cdmfetch failed", "code", cdm.CodeOf(err), "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger, dir, platform string, version bool) error {
	o := cdm.Options{Dir: dir, Logger: log}
	if platform != "" {
		goos, goarch, ok := strings.Cut(platform, "/")
		if !ok {
			return fmt.Errorf("-platform %q: want goos/goarch", platform)
		}
		p, err := cdm.PlatformFor(goos, goarch)
		if err != nil {
			return err
		}
		o.Platform = p
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	if version {
		rel, err := cdm.FetchRelease(ctx, o)
		if err != nil {
			return err
		}
		fmt.Println(rel.Version)
		return nil
	}
	if o.Dir == "" {
		paths, err := config.DefaultPaths()
		if err != nil {
			return err
		}
		o.Dir = paths.WidevineDir()
	}
	inst, err := cdm.Ensure(ctx, o)
	if err != nil {
		return err
	}
	fmt.Printf("version=%s path=%s\n", inst.Version, inst.Path)
	return nil
}
