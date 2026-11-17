// Package config resolves Sona's per-user directories (XDG on Linux and
// other Unix systems, the OS conventions on macOS and Windows) and loads
// and saves the user settings file. It is the only package that reads the
// directory environment variables; everything else receives paths from
// it.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Paths holds Sona's per-user directories, resolved for the OS by
// ResolvePaths (paths_xdg.go, paths_darwin.go, paths_windows.go). All of
// them are created with mode 0700 by [Paths.Ensure]; two may be the same
// directory (macOS keeps config and data together).
type Paths struct {
	Config string // settings.json, credentials fallback
	Data   string // the SQLite cache
	Cache  string // disposable caches
	Art    string // Cache/art: artwork variants and backdrops
	State  string // logs, the saved queue, crash reports
}

// AddonsDir is the folder of the user's add-ons, one folder each
// (docs/addons.md). Ensure creates it with mode 0700: an add-on is code Sona
// runs, so only its user may add one.
func (p Paths) AddonsDir() string { return filepath.Join(p.Config, "addons") }

// AddonLogDir holds the add-ons' logs, one file each, beside sona.log.
func (p Paths) AddonLogDir() string { return filepath.Join(p.State, "addons") }

// WasmCacheDir holds the compiled code of WebAssembly add-ons, keyed by the
// hash of each module. It is machine code Sona loads and runs, so only its
// user may write it (internal/addons.NewWasmHost creates it with mode 0700
// and drops it if it isn't safe) and no add-on can reach it. It is a cache:
// deleting it costs one compile.
func (p Paths) WasmCacheDir() string { return filepath.Join(p.Cache, "addons-wasm") }

// DefaultPaths resolves the directories from the process environment and
// ensures they exist.
func DefaultPaths() (Paths, error) {
	p, err := ResolvePaths(os.Getenv)
	if err != nil {
		return Paths{}, err
	}
	return p, p.Ensure()
}

// Ensure creates every directory with mode 0700 and tightens the mode of
// directories that already exist with looser permissions. Parent XDG
// directories that do not exist are created 0700 as well.
func (p Paths) Ensure() error {
	var errs []error
	for _, dir := range []string{p.Config, p.Data, p.Cache, p.Art, p.State, p.AddonsDir(), p.AddonLogDir()} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			errs = append(errs, fmt.Errorf("config: create %s: %w", dir, err))
			continue
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			errs = append(errs, fmt.Errorf("config: chmod %s: %w", dir, err))
		}
	}
	return errors.Join(errs...)
}

// SettingsFile is the JSON settings file.
func (p Paths) SettingsFile() string { return filepath.Join(p.Config, "settings.json") }

// DatabaseFile is the SQLite media cache and app state database.
func (p Paths) DatabaseFile() string { return filepath.Join(p.Data, "sona.db") }

// LogFile is the JSON log file; rotated copies sit beside it.
func (p Paths) LogFile() string { return filepath.Join(p.State, "sona.log") }

// ThemesDir is the folder of the user's theme files (site/src/content/docs/reference/theme-format.md). It is
// created when the themes service first lists it, not at start-up.
func (p Paths) ThemesDir() string { return filepath.Join(p.Config, "themes") }

// WidevineDir is where Sona installs the Widevine CDM it downloads
// (internal/playback/cdm): a subtree of its own, so a sandbox that hides
// the rest of Sona's data from the playback host (the macOS Seatbelt
// profile) can allow reading just this one.
func (p Paths) WidevineDir() string { return filepath.Join(p.Data, "widevine") }

// CoversDir holds the images the user chose as playlist covers
// (artwork.SaveCover). It is user data, kept apart from the artwork cache,
// which Sona may empty at any time.
func (p Paths) CoversDir() string { return filepath.Join(p.Data, "covers") }
