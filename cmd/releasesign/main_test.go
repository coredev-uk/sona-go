package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
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
	// The second signature binds the version, the platform and the digest, as
	// the updater checks them.
	if m.SHA256 != hex.EncodeToString(digest[:]) {
		t.Fatalf("sha256 = %s", m.SHA256)
	}
	if err := update.VerifyStatement(pub, m, "darwin-arm64"); err != nil {
		t.Fatal(err)
	}
	if err := update.VerifyStatement(pub, m, "windows-amd64"); !errors.Is(err, update.ErrSignature) {
		t.Fatalf("the statement verifies for another platform: %v", err)
	}
	m.Version = "9.9.9"
	if err := update.VerifyStatement(pub, m, "darwin-arm64"); !errors.Is(err, update.ErrSignature) {
		t.Fatalf("the statement verifies for another version: %v", err)
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

// release.json carries the notes cut at the install instructions and without
// the version's heading, and the updater verifies it against the key.
func TestReleasesignWritesVerifiableReleaseInfo(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	dir := t.TempDir()
	body := "## [0.3.0](https://example.test/compare) (2026-01-02)\n\n### Fixes\n\n- a (#1)\n\n" + update.InstallNotesMarker + "\n## Installing\n\nRun it.\n"
	notes := filepath.Join(dir, "changelog.md")
	if err := os.WriteFile(notes, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "release.json")
	env := map[string]string{"K": base64.StdEncoding.EncodeToString(priv)}
	now := func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) }
	if err := run([]string{"-release-info", "-version", "v0.3.0", "-notes", notes, "-key-env", "K", "-out", out}, func(k string) string { return env[k] }, now); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	info, err := update.ParseReleaseInfo(raw, pub)
	if err != nil {
		t.Fatal(err)
	}
	if info.Version != "0.3.0" || info.Published != "2026-01-02T03:04:05Z" || info.Notes != "### Fixes\n\n- a (#1)" {
		t.Fatalf("release.json = %+v", info)
	}
	other, _, _ := ed25519.GenerateKey(nil)
	if _, err := update.ParseReleaseInfo(raw, other); !errors.Is(err, update.ErrSignature) {
		t.Fatalf("verified with another key: %v", err)
	}

	// -verify checks a file against the key in -pub-env and refuses one
	// that another key signed.
	env["P"] = base64.StdEncoding.EncodeToString(pub)
	env["OTHER"] = base64.StdEncoding.EncodeToString(other)
	if err := run([]string{"-verify", out, "-pub-env", "P"}, func(k string) string { return env[k] }, now); err != nil {
		t.Fatalf("-verify with the signing key: %v", err)
	}
	if err := run([]string{"-verify", out, "-pub-env", "OTHER"}, func(k string) string { return env[k] }, now); !errors.Is(err, update.ErrSignature) {
		t.Fatalf("-verify with another key: %v", err)
	}
}

// A release.json needs a version and a key, and a date that is a date.
func TestReleasesignReleaseInfoRefusals(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	env := map[string]string{"K": base64.StdEncoding.EncodeToString(priv)}
	getenv := func(k string) string { return env[k] }
	out := filepath.Join(t.TempDir(), "release.json")
	for name, args := range map[string][]string{
		"no version": {"-release-info", "-key-env", "K", "-out", out},
		"no key":     {"-release-info", "-version", "0.3.0", "-key-env", "NONE", "-out", out},
		"bad date":   {"-release-info", "-version", "0.3.0", "-date", "yesterday", "-key-env", "K", "-out", out},
	} {
		if err := run(args, getenv, time.Now); err == nil {
			t.Errorf("%s: wrote a release.json", name)
		}
	}
	if _, err := os.Stat(out); err == nil {
		t.Error("a refused run left a file")
	}
}
