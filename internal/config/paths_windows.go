package config

import (
	"fmt"
	"path/filepath"
)

// AppName is Sona's directory name under the Windows app data folders.
const AppName = "Sona"

// ResolvePaths computes Sona's directories from %APPDATA% and
// %LOCALAPPDATA% read through getenv: settings and the credentials
// fallback roam (%APPDATA%\Sona); the database, caches and state stay on
// this machine (%LOCALAPPDATA%\Sona). It creates nothing.
func ResolvePaths(getenv func(string) string) (Paths, error) {
	roaming, local := getenv("APPDATA"), getenv("LOCALAPPDATA")
	if !filepath.IsAbs(roaming) || !filepath.IsAbs(local) {
		return Paths{}, fmt.Errorf("config: APPDATA %q or LOCALAPPDATA %q is not absolute", roaming, local)
	}
	data := filepath.Join(local, AppName)
	cache := filepath.Join(data, "Cache")
	return Paths{
		Config: filepath.Join(roaming, AppName),
		Data:   data,
		Cache:  cache,
		Art:    filepath.Join(cache, "art"),
		State:  filepath.Join(data, "State"),
	}, nil
}
