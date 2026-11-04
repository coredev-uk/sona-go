package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/update"
)

func TestReleasesignWritesVerifiableManifest(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	dir := t.TempDir()
	archive := filepath.Join(dir, "a.tar.gz")
	if err := os.WriteFile(archive, []byte("archive bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "m.json")
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
	digest := sha256.Sum256([]byte("archive bytes"))
	if m.Version != "0.3.0" || m.Size != 13 || m.Date != "2026-01-02T03:04:05Z" {
		t.Fatalf("manifest = %+v", m)
	}
	if err := update.Verify(pub, digest[:], m.Signature); err != nil {
		t.Fatal(err)
	}
}

func TestReleasesignRefusesWithoutKey(t *testing.T) {
	err := run([]string{"-target", "windows-amd64", "-archive", "x", "-version", "0.3.0", "-url", "https://e.test/x"},
		func(string) string { return "" }, time.Now)
	if err == nil {
		t.Fatal("signed without a key")
	}
}
