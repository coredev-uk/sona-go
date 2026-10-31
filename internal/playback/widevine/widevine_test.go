package widevine

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func writeFile(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func testFinder(t *testing.T) (finder, string) {
	root := t.TempDir()
	return finder{
		opt:      filepath.Join(root, "opt"),
		home:     filepath.Join(root, "home"),
		getenv:   func(string) string { return "" },
		lookPath: func(string) (string, error) { return "", exec.ErrNotFound },
		arch:     "linux_x64",
	}, root
}

const sub = "_platform_specific/linux_x64/libwidevinecdm.so"

func TestFindCDMOrder(t *testing.T) {
	f, root := testFinder(t)
	if _, _, err := f.find(""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("empty system: %v", err)
	}

	// Firefox GMP: newest version wins, version from the adjacent manifest.
	ff := filepath.Join(f.home, ".mozilla/firefox/abc.default/gmp-widevinecdm")
	writeFile(t, filepath.Join(ff, "4.10.2830.0", libName), "x")
	writeFile(t, filepath.Join(ff, "4.10.2891.0", libName), "x")
	writeFile(t, filepath.Join(ff, "4.10.2891.0", "manifest.json"), `{"version":"4.10.2891.0"}`)
	assertFound(t, f, filepath.Join(ff, "4.10.2891.0", libName), "4.10.2891.0")

	// A Chromium-family component CDM beats Firefox; the latest-component
	// pointer beats a newer-looking directory.
	wv := filepath.Join(f.home, ".config/BraveSoftware/Brave-Browser/WidevineCdm")
	writeFile(t, filepath.Join(wv, "4.10.3050.0", sub), "x")
	writeFile(t, filepath.Join(wv, "4.10.3050.0", "manifest.json"), `{"version":"4.10.3050.0"}`)
	writeFile(t, filepath.Join(wv, "4.10.9999.0", sub), "x")
	writeFile(t, filepath.Join(wv, "latest-component-updated-widevine-cdm"), `{"Path":"`+filepath.Join(wv, "4.10.3050.0")+`"}`)
	assertFound(t, f, filepath.Join(wv, "4.10.3050.0", sub), "4.10.3050.0")

	// google-chrome-stable on PATH with the Nix layout (bin/ + share/).
	pkg := filepath.Join(root, "store/google-chrome-156")
	writeFile(t, filepath.Join(pkg, "bin/google-chrome-stable"), "#!/bin/sh")
	writeFile(t, filepath.Join(pkg, "share/google/chrome/WidevineCdm", sub), "x")
	writeFile(t, filepath.Join(pkg, "share/google/chrome/WidevineCdm/manifest.json"), `{"version":"4.10.3112.0"}`)
	link := filepath.Join(root, "profile/bin/google-chrome-stable")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(pkg, "bin/google-chrome-stable"), link); err != nil {
		t.Fatal(err)
	}
	f.lookPath = func(name string) (string, error) {
		if name == "google-chrome-stable" {
			return link, nil
		}
		return "", exec.ErrNotFound
	}
	p, v, err := f.find("")
	if err != nil || v != "4.10.3112.0" || !isFile(p) || !bytes.Contains([]byte(p), []byte("store/google-chrome-156")) {
		t.Fatalf("nix chrome: %q %q %v", p, v, err)
	}

	// /opt/google/chrome beats everything discovered.
	opt := filepath.Join(f.opt, "chrome/WidevineCdm")
	writeFile(t, filepath.Join(opt, sub), "x")
	writeFile(t, filepath.Join(opt, "manifest.json"), `{"version":"4.10.1.0"}`)
	assertFound(t, f, filepath.Join(opt, sub), "4.10.1.0")

	// The environment override beats discovery; an explicit override beats both.
	envLib := filepath.Join(root, "env/libwidevinecdm.so")
	writeFile(t, envLib, "x")
	f.getenv = func(k string) string {
		if k == EnvCDM {
			return envLib
		}
		return ""
	}
	assertFound(t, f, envLib, "")
	over := filepath.Join(root, "over/4.10.5.0")
	writeFile(t, filepath.Join(over, libName), "x")
	if p, v, err := f.find(over); err != nil || p != filepath.Join(over, libName) || v != "4.10.5.0" {
		t.Fatalf("override dir: %q %q %v", p, v, err)
	}
	if _, _, err := f.find(filepath.Join(root, "missing.so")); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("missing override must fail explicitly: %v", err)
	}
}

func assertFound(t *testing.T, f finder, wantPath, wantVersion string) {
	t.Helper()
	p, v, err := f.find("")
	if err != nil || p != wantPath || v != wantVersion {
		t.Fatalf("find = %q %q %v, want %q %q", p, v, err, wantPath, wantVersion)
	}
}

func TestPSSH(t *testing.T) {
	kid, _ := hex.DecodeString("000000004224e8e0001db05d823e28e5")
	got, err := PSSH(kid)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := hex.DecodeString("00000034" + "70737368" + "00000000" + "edef8ba979d64acea3c827dcd51d21ed" + "00000014" + "08011210" + "000000004224e8e0001db05d823e28e5")
	if !bytes.Equal(got, want) {
		t.Fatalf("pssh %x", got)
	}
	if _, err := PSSH(kid[:8]); err == nil {
		t.Fatal("short KID accepted")
	}
}

// TestCDM exercises the real CDM named by SONA_WIDEVINE_CDM (a developer
// opts in, CONTRIBUTING.md; CI runners have Chrome installed but do not):
// initialisation, licence request generation, timer delivery, a decrypt
// without a key, and session close. No network.
func TestCDM(t *testing.T) {
	if os.Getenv("SONA_WIDEVINE_CDM") == "" {
		t.Skip("SONA_WIDEVINE_CDM not set")
	}
	path, version, err := FindCDM("")
	if err != nil {
		t.Skip("no Widevine CDM installed:", err)
	}
	c, err := Open(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	t.Logf("CDM %s (%s) interface %d host %d", c.Version(), version, c.Interface(), c.HostInterface())
	if c.Version() == "" || (c.Interface() != 10 && c.Interface() != 11) {
		t.Fatalf("version %q interface %d", c.Version(), c.Interface())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	kid, _ := hex.DecodeString("000000004224e8e0001db05d823e28e5")
	pssh, _ := PSSH(kid)
	id, req, err := c.CreateSession(ctx, pssh)
	if err != nil {
		t.Fatal(err)
	}
	if id == "" || len(req) < 100 {
		t.Fatalf("session %q request %d bytes", id, len(req))
	}
	if err := c.UpdateSession(ctx, id, []byte("not a licence")); err == nil {
		t.Fatal("garbage licence accepted")
	}

	iv := make([]byte, 16)
	data := []byte("0123456789abcdef")
	bufs := []Buffer{{KeyID: kid, IV: iv, Data: data, Out: make([]byte, len(data))}}
	if err := c.Decrypt(bufs); !errors.Is(err, ErrNoKey) && !errors.Is(err, ErrDecrypt) {
		t.Fatalf("decrypt without licence: %v", err)
	}
	if err := c.CloseSession(ctx, id); err != nil {
		t.Fatal(err)
	}
	// The CDM schedules periodic work through Host::SetTimer; the host must
	// call TimerExpired back.
	deadline := time.Now().Add(3 * time.Second)
	for c.TimersFired() == 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if c.TimersFired() == 0 {
		t.Fatal("no CDM timer fired within 3s")
	}
	t.Logf("timers fired: %d", c.TimersFired())
}
