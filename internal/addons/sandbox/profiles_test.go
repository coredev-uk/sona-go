package sandbox

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestProfileName(t *testing.T) {
	if got := ProfileName("weather"); got != "sona.addon.weather" {
		t.Errorf("ProfileName = %q", got)
	}
}

func TestProfileIDsAreTheBundledInstalledAndLeftOver(t *testing.T) {
	root := t.TempDir()
	installed := filepath.Join(root, "addons")
	state := filepath.Join(root, "state")
	for _, d := range []string{
		filepath.Join(installed, "weather"),
		filepath.Join(installed, ".hidden"), // not an ID
		filepath.Join(installed, "Bad_Name"),
		filepath.Join(installed, "x"), // too short
		filepath.Join(state, "weather", "data"),
		filepath.Join(state, "old-one", "data"), // uninstalled, its data left
		filepath.Join(state, "lastfm", "data"),  // bundled, listed once
	} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(installed, "stray-file"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	got := ProfileIDs([]string{"lastfm", "discord"}, installed, state, filepath.Join(root, "missing"))
	want := []string{"discord", "lastfm", "old-one", "weather"}
	if !slices.Equal(got, want) {
		t.Errorf("ProfileIDs = %q, want %q", got, want)
	}
	if got := ProfileIDs(nil); len(got) != 0 {
		t.Errorf("no sources gave %q", got)
	}
}
