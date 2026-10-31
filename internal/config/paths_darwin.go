package config

import (
	"fmt"
	"path/filepath"
)

// AppName is Sona's directory name under the macOS user Library folders.
const AppName = "Sona"

// ResolvePaths computes Sona's directories from $HOME read through getenv,
// following Apple's file-system guidelines: settings, credentials fallback
// and the database in ~/Library/Application Support/Sona, logs and other
// state in its State folder, disposable caches in ~/Library/Caches/Sona.
// It creates nothing.
func ResolvePaths(getenv func(string) string) (Paths, error) {
	home := getenv("HOME")
	if !filepath.IsAbs(home) {
		return Paths{}, fmt.Errorf("config: HOME %q is not absolute", home)
	}
	support := filepath.Join(home, "Library", "Application Support", AppName)
	cache := filepath.Join(home, "Library", "Caches", AppName)
	return Paths{
		Config: support,
		Data:   support,
		Cache:  cache,
		Art:    filepath.Join(cache, "art"),
		State:  filepath.Join(support, "State"),
	}, nil
}
