package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/update"
)

// tarball is a gzip tar of the named files (a name ending in "/" is a
// directory), written in the order given.
func tarball(t *testing.T, names ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, n := range names {
		h := &tar.Header{Name: n, Typeflag: tar.TypeReg, Mode: 0o755, Size: 1}
		if n[len(n)-1] == '/' {
			h.Typeflag, h.Size = tar.TypeDir, 0
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Size > 0 {
			if _, err := tw.Write([]byte("x")); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func writeArchive(t *testing.T, b []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "a.tar.gz")
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReleasesignWritesVerifiableManifest(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	archiveBytes := tarball(t, "Sona.app/", "Sona.app/Contents/")
	archive := writeArchive(t, archiveBytes)
	out := filepath.Join(t.TempDir(), "m.json")
	env := map[string]string{"K": base64.StdEncoding.EncodeToString(priv)}
	now := func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) }
	err := run([]string{"-target", "darwin-arm64", "-archive", archive, "-version", "v0.3.0", "-url", "https://example.test/a.tar.gz", "-key-env", "K", "-out", out},
		func(k string) string { return env[k] }, now)
	if err != nil {
		t.Fatal(err)
	}
	var m update.Manifest
	b, _ := os.ReadFile(out)
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(archiveBytes)
	if m.Version != "0.3.0" || m.Size != int64(len(archiveBytes)) || m.Date != "2026-01-02T03:04:05Z" {
		t.Fatalf("manifest = %+v", m)
	}
	if err := update.Verify(pub, digest[:], m.Signature); err != nil {
		t.Fatal(err)
	}
}

func TestReleasesignRefusesWithoutKey(t *testing.T) {
	archive := writeArchive(t, tarball(t, "sona.exe"))
	err := run([]string{"-target", "windows-amd64", "-archive", archive, "-version", "0.3.0", "-url", "https://e.test/x"},
		func(string) string { return "" }, time.Now)
	if err == nil {
		t.Fatal("signed without a key")
	}
}

// The archives of every release up to 0.5.0-rc.3 began with "./", which the
// updater refuses: nothing signs such an archive again.
func TestReleasesignRefusesAnArchiveTheUpdaterCannotInstall(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	env := map[string]string{"K": base64.StdEncoding.EncodeToString(priv)}
	out := filepath.Join(t.TempDir(), "m.json")
	archive := writeArchive(t, tarball(t, "./", "./sona.exe", "./lib/"))
	err := run([]string{"-target", "windows-amd64", "-archive", archive, "-version", "0.3.0", "-url", "https://e.test/x", "-key-env", "K", "-out", out},
		func(k string) string { return env[k] }, time.Now)
	if !errors.Is(err, update.ErrArchive) {
		t.Fatalf("err = %v, want ErrArchive", err)
	}
	if _, serr := os.Stat(out); serr == nil {
		t.Error("a manifest was written for an archive the updater refuses")
	}
}

// -check needs no key and no URL, and signs nothing.
func TestReleasesignCheckOnlyChecks(t *testing.T) {
	good := writeArchive(t, tarball(t, "sona.exe", "lib/", "lib/a.dll"))
	if err := run([]string{"-check", "-target", "windows-amd64", "-archive", good}, func(string) string { return "" }, time.Now); err != nil {
		t.Fatalf("a good archive: %v", err)
	}
	bad := writeArchive(t, tarball(t, "./", "./sona.exe"))
	if err := run([]string{"-check", "-target", "windows-amd64", "-archive", bad}, func(string) string { return "" }, time.Now); !errors.Is(err, update.ErrArchive) {
		t.Fatalf("a ./ archive: %v", err)
	}
}
