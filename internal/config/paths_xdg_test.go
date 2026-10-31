//go:build !darwin && !windows

package config

import (
	"os"
	"testing"
)

func TestResolvePathsXDG(t *testing.T) {
	p, err := ResolvePaths(envMap(map[string]string{
		"HOME":            "/home/u",
		"XDG_CONFIG_HOME": "/x/config",
		"XDG_DATA_HOME":   "/x/data",
		"XDG_CACHE_HOME":  "/x/cache",
		"XDG_STATE_HOME":  "/x/state",
	}))
	if err != nil {
		t.Fatal(err)
	}
	want := Paths{
		Config: "/x/config/sona",
		Data:   "/x/data/sona",
		Cache:  "/x/cache/sona",
		Art:    "/x/cache/sona/art",
		State:  "/x/state/sona",
	}
	if p != want {
		t.Fatalf("got %+v, want %+v", p, want)
	}
}

func TestResolvePathsDefaultsAndRelativeIgnored(t *testing.T) {
	p, err := ResolvePaths(envMap(map[string]string{
		"HOME":            "/home/u",
		"XDG_CONFIG_HOME": "relative/config", // ignored per the XDG spec
	}))
	if err != nil {
		t.Fatal(err)
	}
	want := Paths{
		Config: "/home/u/.config/sona",
		Data:   "/home/u/.local/share/sona",
		Cache:  "/home/u/.cache/sona",
		Art:    "/home/u/.cache/sona/art",
		State:  "/home/u/.local/state/sona",
	}
	if p != want {
		t.Fatalf("got %+v, want %+v", p, want)
	}
}

func TestResolvePathsNoHome(t *testing.T) {
	if _, err := ResolvePaths(envMap(nil)); err == nil {
		t.Fatal("want error without HOME or XDG variables")
	}
}

func TestEnsureCreatesAndTightens(t *testing.T) {
	root := t.TempDir()
	p, err := ResolvePaths(envMap(map[string]string{"HOME": root}))
	if err != nil {
		t.Fatal(err)
	}
	// A pre-existing, world-readable config dir must be tightened.
	if err := os.MkdirAll(p.Config, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := p.Ensure(); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{p.Config, p.Data, p.Cache, p.Art, p.State} {
		fi, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if !fi.IsDir() || fi.Mode().Perm() != 0o700 {
			t.Errorf("%s: mode %v, want dir 0700", dir, fi.Mode())
		}
	}
}
