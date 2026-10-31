package cdm

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestLiveDownload installs Google's real CDM from the pinned Mozilla
// source: the running platform's, and macOS arm64's and Windows amd64's
// (format-checked only). It downloads ~60 MiB, so it runs only with
// SONA_CDM_LIVE=1.
func TestLiveDownload(t *testing.T) {
	if os.Getenv("SONA_CDM_LIVE") != "1" {
		t.Skip("set SONA_CDM_LIVE=1 to download the real Widevine CDM from Google")
	}
	targets := []Platform{platform(t, "darwin", "arm64"), platform(t, "windows", "amd64")}
	if p, err := Current(); err == nil {
		targets = append([]Platform{p}, targets...)
	}
	for _, p := range targets {
		t.Run(p.String(), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
			defer cancel()
			dir := t.TempDir()
			inst, err := Ensure(ctx, Options{Dir: dir, Platform: p})
			if err != nil {
				t.Fatal(err)
			}
			if !versionPattern.MatchString(inst.Version) || inst.Path != filepath.Join(dir, inst.Version, p.Dir, p.Lib) {
				t.Fatalf("Ensure = %+v", inst)
			}
			if v := LibraryVersion(inst.Path); v != inst.Version {
				t.Errorf("LibraryVersion = %q, want %q", v, inst.Version)
			}
			if err := checkLibrary(inst.Path, p); err != nil {
				t.Error(err)
			}
			if ptr := readPointer(t, dir); ptr.Current != inst.Version || ptr.Platform != p.Dir {
				t.Errorf("pointer = %+v", ptr)
			}
			t.Logf("%s: Widevine %s at %s", p, inst.Version, inst.Path)
		})
	}
}
