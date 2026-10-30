// Package config resolves Sona's XDG directories and loads and saves the
// user settings file. It is the only package that reads XDG environment
// variables; everything else receives paths from it.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// AppName is the directory name used under every XDG base directory.
const AppName = "sona"

// Paths holds Sona's per-user directories. All of them are created with mode
// 0700 by [Paths.Ensure].
type Paths struct {
	Config string // $XDG_CONFIG_HOME/sona: settings.json, credentials fallback
	Data   string // $XDG_DATA_HOME/sona: the SQLite cache
	Cache  string // $XDG_CACHE_HOME/sona: disposable caches
	Art    string // $XDG_CACHE_HOME/sona/art: artwork variants and backdrops
	State  string // $XDG_STATE_HOME/sona: logs
}

// ResolvePaths computes Sona's directories from the XDG Base Directory
// variables read through getenv, falling back to the spec's defaults under
// $HOME. Per the spec, a relative XDG value is ignored. It creates nothing.
func ResolvePaths(getenv func(string) string) (Paths, error) {
	home := getenv("HOME")
	base := func(env, def string) (string, error) {
		if v := getenv(env); filepath.IsAbs(v) {
			return filepath.Join(v, AppName), nil
		}
		if !filepath.IsAbs(home) {
			return "", fmt.Errorf("config: %s unset and HOME %q is not absolute", env, home)
		}
		return filepath.Join(home, def, AppName), nil
	}
	var p Paths
	var err error
	if p.Config, err = base("XDG_CONFIG_HOME", ".config"); err != nil {
		return Paths{}, err
	}
	if p.Data, err = base("XDG_DATA_HOME", filepath.Join(".local", "share")); err != nil {
		return Paths{}, err
	}
	if p.Cache, err = base("XDG_CACHE_HOME", ".cache"); err != nil {
		return Paths{}, err
	}
	if p.State, err = base("XDG_STATE_HOME", filepath.Join(".local", "state")); err != nil {
		return Paths{}, err
	}
	p.Art = filepath.Join(p.Cache, "art")
	return p, nil
}

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
	for _, dir := range []string{p.Config, p.Data, p.Cache, p.Art, p.State} {
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

// CredentialsFile is the 0600 credentials fallback used when Secret Service
// is unavailable.
func (p Paths) CredentialsFile() string { return filepath.Join(p.Config, "credentials.json") }

// DatabaseFile is the SQLite media cache and app state database.
func (p Paths) DatabaseFile() string { return filepath.Join(p.Data, "sona.db") }

// LogFile is the JSON log file; rotated copies sit beside it.
func (p Paths) LogFile() string { return filepath.Join(p.State, "sona.log") }
