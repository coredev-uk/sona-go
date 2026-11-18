//go:build linux

package services

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coredev-uk/sona-go/internal/addons/wasmtest"
)

// wasmDevFolder writes a wasm add-on's folder outside the add-ons folder.
func wasmDevFolder(t *testing.T, id, manifest string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), id)
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "addon.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "run.wasm"), wasmtest.Module(t), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// `addons dev` runs a wasm add-on like any other: it asks first, streams the
// log, serves the add-on's actions and ends when its client stops it.
func TestDevRunsAWasmAddon(t *testing.T) {
	a := wasmEnv(t, "LC_WASMTEST_PERMS=playback.read")
	dir := wasmDevFolder(t, "wasmdev", wasmManifest("wasmdev", sendAction, "playback.read"))
	before := a.e.Settings.Get().Addons

	res := a.startDev(dir)
	q := a.waitAsk(t)
	if q.ID != "wasmdev" || q.Dir != dir {
		t.Errorf("question = %+v", q)
	}
	a.svc.AnswerDev(q.Token, true)
	r := recv(t, res)
	if r.err != nil {
		t.Fatal(r.err)
	}
	var lines []string
	next := int64(0)
	waitUntil(t, "the add-on's hello", func() bool {
		l, err := a.svc.DevLog(r.d.Token, next)
		if err != nil {
			t.Fatal(err)
		}
		lines, next = append(lines, l.Lines...), l.Next
		return strings.Contains(strings.Join(lines, "\n"), "hello ok")
	})
	if text := strings.Join(lines, "\n"); !strings.Contains(text, "started (WebAssembly in Sona") {
		t.Errorf("lines = %q", lines)
	}
	got, err := a.svc.Invoke(t.Context(), "wasmdev", "addon.wasmdev.send", &AddonTarget{Kind: "album", ID: "1", Name: "x"})
	if err != nil || got.Toast != "wasm says addon.wasmdev.send" {
		t.Errorf("invoke = %+v, %v", got, err)
	}
	// Nothing is saved, and the run is not an installed add-on.
	if now := a.e.Settings.Get().Addons; len(now) != len(before) {
		t.Errorf("the run changed the saved add-ons: %+v", now)
	}
	if err := a.svc.DevStop(r.d.Token); err != nil {
		t.Fatal(err)
	}
}

// Where a native add-on can't be run from the file that was checked (macOS,
// Windows), `addons dev` refuses it before it asks anything, and still runs a
// wasm add-on, whose bytes are read once.
func TestDevWasmRunsWhereNativeCannot(t *testing.T) {
	a := wasmEnv(t)
	a.svc.opts.verified = false

	native := devFolder(t, "nativedev", manifestJSON("nativedev"), "exit 0\n")
	if r := recv(t, a.startDev(native)); !errors.Is(r.err, errors.ErrUnsupported) {
		t.Errorf("a native add-on: err = %v, want unsupported", r.err)
	}
	if n := a.asked(); n != 0 {
		t.Errorf("the person was asked %d times about an add-on that can't run", n)
	}

	wasm := wasmDevFolder(t, "wasmdev", wasmManifest("wasmdev", `{}`))
	res := a.startDev(wasm)
	q := a.waitAsk(t)
	a.svc.AnswerDev(q.Token, true)
	r := recv(t, res)
	if r.err != nil {
		t.Fatal(r.err)
	}
	waitUntil(t, "the run", func() bool {
		a.svc.mu.Lock()
		defer a.svc.mu.Unlock()
		st, _ := a.svc.runners["wasmdev"].snapshot()
		return st == AddonRunning || st == AddonStarting
	})
	if err := a.svc.DevStop(r.d.Token); err != nil {
		t.Fatal(err)
	}
}
