//go:build !darwin && !windows

package config

import (
	"fmt"
	"path/filepath"
)

// AppName is the directory name used under every XDG base directory.
const AppName = "sona"

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
