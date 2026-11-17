//go:build unix

package services

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/addons"
	"github.com/coredev-uk/sona-go/internal/addons/wasmtest"
)

// wasmCache is the compilation cache every test of the package shares:
// compiling the test module is most of a test's cost, so it is done once.
var wasmCache string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "sona-wasm-cache")
	if err != nil {
		panic(err)
	}
	wasmCache = dir
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// wasmManifest is a wasm add-on's manifest.
func wasmManifest(id, contributions string, perms ...string) string {
	return strings.Replace(manifestWith(id, contributions, perms...), `"command":"run"`, `"command":"run.wasm","runtime":"wasm"`, 1)
}

// installWasm writes a wasm add-on's folder: its manifest and the test
// module, which, being data, is not executable.
func (a *addonEnv) installWasm(t *testing.T, id, manifest string) string {
	t.Helper()
	dir := filepath.Join(a.root, id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
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

var warmOnce sync.Once

// warmWasmCache compiles the test module into the shared cache, once, however
// long that takes (under the race detector, tens of seconds): the tests then
// start a module in milliseconds, as an add-on starts once its code is cached.
func warmWasmCache(t *testing.T) {
	t.Helper()
	warmOnce.Do(func() {
		h := addons.NewWasmHost(wasmCache, addons.DefaultWasmLimits())
		defer h.Close()
		p, err := h.Start(addons.WasmSpec{Module: wasmtest.Module(t), Name: "warm.wasm", Env: []string{"LC_WASMTEST_MODE=exit"}})
		if err != nil {
			t.Fatal(err)
		}
		select {
		case <-p.Done():
		case <-time.After(5 * time.Minute):
			t.Fatal("the test module never compiled")
		}
	})
}

// wasmEnv is a supervisor whose wasm host shares the test cache. env is what
// the module is told (LC_WASMTEST_*, which the allowlist lets through).
func wasmEnv(t *testing.T, env ...string) *addonEnv {
	t.Helper()
	warmWasmCache(t)
	a := newAddonEnv(t)
	a.svc.opts.environ = func() []string { return env }
	a.svc.wasm = addons.NewWasmHost(wasmCache, a.svc.opts.wasm)
	return a
}

// The time an add-on has to say hello runs from when it is running, not from
// when Sona started it: compiling a module takes seconds the first time, and
// a module that is killed for it would never be cached and never start.
func TestWasmCompileIsNotHelloTime(t *testing.T) {
	a := newAddonEnv(t)
	a.svc.opts.environ = func() []string { return nil }
	a.svc.opts.helloTimeout = time.Second
	// A cache of its own, empty: this start compiles.
	a.svc.wasm = addons.NewWasmHost(t.TempDir(), a.svc.opts.wasm)
	a.installWasm(t, "wasmcold", wasmManifest("wasmcold", `{}`))
	if err := a.svc.Enable("wasmcold", nil); err != nil {
		t.Fatal(err)
	}
	a.svc.start()
	// Compiling from nothing takes seconds, tens of them under the race detector.
	for deadline := time.Now().Add(5 * time.Minute); a.info(t, "wasmcold").State != AddonRunning; time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("the cold module never ran: %+v", a.info(t, "wasmcold"))
		}
	}
	lines, _ := a.svc.Log("wasmcold")
	if text := strings.Join(lines, "\n"); strings.Contains(text, "no hello") {
		t.Errorf("the module was killed for the time its compile took:\n%s", text)
	}
}

// A wasm add-on is served like any other: it says hello, hears events, answers
// an invoke, and leaves when Sona stops it. Its program is data, so it is
// not executable, and no process is started for it.
func TestWasmAddonRunsHearsAndAnswers(t *testing.T) {
	a := wasmEnv(t, "LC_WASMTEST_PERMS=playback.read", "LC_WASMTEST_TOPICS=status")
	dir := a.installWasm(t, "wasmdemo", wasmManifest("wasmdemo", sendAction, "playback.read"))
	if fi, err := os.Stat(filepath.Join(dir, "run.wasm")); err != nil || fi.Mode().Perm()&0o111 != 0 {
		t.Fatalf("the module is %v, %v: this test wants it not executable", fi, err)
	}
	info := a.info(t, "wasmdemo")
	if info.State != AddonOff || info.Runtime != addons.RuntimeWasm || info.Problem != "" {
		t.Fatalf("listed as %+v", info)
	}
	if err := a.svc.Enable("wasmdemo", []string{"playback.read"}); err != nil {
		t.Fatal(err)
	}
	a.svc.start()
	a.waitState(t, "wasmdemo", AddonRunning)

	// The topic's latest value arrives at once, then the add-on hears it.
	log := addons.LogPath(a.logs, "wasmdemo")
	waitUntil(t, "the status event", fileHas(log, "event status"))
	res, err := a.svc.Invoke(t.Context(), "wasmdemo", "addon.wasmdemo.send", &AddonTarget{Kind: "album", ID: "1", Name: "Abbey Road"})
	if err != nil || res.Toast != "wasm says addon.wasmdemo.send" || res.URL != "https://example.com/wasm" {
		t.Errorf("invoke = %+v, %v", res, err)
	}
	if c := a.svc.Contributions("en"); len(c) != 1 || !c[0].Running || len(c[0].Actions) != 1 {
		t.Errorf("contributions = %+v", c)
	}

	// Stopping it sends the shutdown event; it leaves by itself.
	if err := a.svc.Disable("wasmdemo"); err != nil {
		t.Fatal(err)
	}
	a.svc.close()
	waitUntil(t, "the module to have left", fileHas(log, "sona: exited"))
	lines, _ := a.svc.Log("wasmdemo")
	text := strings.Join(lines, "\n")
	for _, want := range []string{"sona: started (WebAssembly in Sona, version 1.0.0)", "hello ok", "shutdown", "sona: exited: <nil>"} {
		if !strings.Contains(text, want) {
			t.Errorf("the log lacks %q:\n%s", want, text)
		}
	}
}

// A module that exits on its own is restarted with the supervisor's backoff,
// and switched off after five in a row (here three), like a process.
func TestWasmAddonCrashesAreBackedOffAndGivenUpOn(t *testing.T) {
	a := wasmEnv(t, "LC_WASMTEST_MODE=exit")
	a.svc.opts.maxCrashes = 3
	a.installWasm(t, "wasmcrash", wasmManifest("wasmcrash", `{}`))
	if err := a.svc.Enable("wasmcrash", nil); err != nil {
		t.Fatal(err)
	}
	a.svc.start()
	i := a.waitState(t, "wasmcrash", AddonCrashed)
	if i.Restarts != 3 || i.Enabled {
		t.Errorf("after the crashes: %+v", i)
	}
	lines, _ := a.svc.Log("wasmcrash")
	text := strings.Join(lines, "\n")
	for _, want := range []string{"exiting", "exited: exit status 3", "stopped on its own; starting again in 5ms", "switched off"} {
		if !strings.Contains(text, want) {
			t.Errorf("the log lacks %q:\n%s", want, text)
		}
	}
}

// A module that spins is stopped and logged, and counts as a crash.
func TestWasmAddonRunawayIsStoppedAndLogged(t *testing.T) {
	a := wasmEnv(t, "LC_WASMTEST_MODE=loop")
	a.svc.opts.maxCrashes = 1
	l := addons.DefaultWasmLimits()
	l.CPUBurst, l.CPUShare, l.Tick = 1500*time.Millisecond, 0.1, 20*time.Millisecond
	a.svc.wasm.Close()
	a.svc.wasm = addons.NewWasmHost(wasmCache, l)
	a.installWasm(t, "wasmloop", wasmManifest("wasmloop", `{}`))
	if err := a.svc.Enable("wasmloop", nil); err != nil {
		t.Fatal(err)
	}
	a.svc.start()
	a.waitState(t, "wasmloop", AddonCrashed)
	lines, _ := a.svc.Log("wasmloop")
	if text := strings.Join(lines, "\n"); !strings.Contains(text, "exited: "+addons.ErrWasmCPU.Error()) {
		t.Errorf("the log doesn't say it was stopped for its CPU:\n%s", text)
	}
}

// Quit stops a wasm add-on within the grace, like a process.
func TestWasmAddonStopsAtQuit(t *testing.T) {
	a := wasmEnv(t, "LC_WASMTEST_MODE=deaf")
	a.installWasm(t, "wasmdeaf", wasmManifest("wasmdeaf", `{}`))
	if err := a.svc.Enable("wasmdeaf", nil); err != nil {
		t.Fatal(err)
	}
	a.svc.start()
	waitUntil(t, "the module to run", func() bool {
		lines, _ := a.svc.Log("wasmdeaf")
		return strings.Contains(strings.Join(lines, "\n"), "deaf")
	})
	done := make(chan struct{})
	var once sync.Once
	go func() { a.svc.close(); once.Do(func() { close(done) }) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("quit did not stop a module that ignores its input")
	}
}

// What a wasm manifest may not say is refused, with the reason.
func TestWasmAddonRefusals(t *testing.T) {
	a := wasmEnv(t)
	good := wasmManifest("good", `{}`, "playback.read")
	a.installWasm(t, "good", good)
	a.installWasm(t, "nonet", strings.Replace(wasmManifest("nonet", `{}`, "playback.read"), `"playback.read"`, `"playback.read","network"`, 1))
	a.installWasm(t, "notwasm", strings.Replace(wasmManifest("notwasm", `{}`), `run.wasm`, `run`, 1))
	a.installWasm(t, "docker", strings.Replace(wasmManifest("docker", `{}`), `"wasm"`, `"docker"`, 1))
	// A command that isn't there, and one that is not a module.
	a.installWasm(t, "missing", strings.Replace(wasmManifest("missing", `{}`), `run.wasm`, `nope.wasm`, 1))
	got := map[string]AddonInfo{}
	for _, i := range a.svc.List() {
		got[i.ID] = i
	}
	for id, want := range map[string]string{
		"good": "", "nonet": addons.ProblemPermission, "notwasm": addons.ProblemCommand,
		"docker": addons.ProblemRuntime, "missing": addons.ProblemNoExecutable,
	} {
		if got[id].Problem != want {
			t.Errorf("%s: problem %q, want %q", id, got[id].Problem, want)
		}
	}

	// The file is read at start: what is not a module never runs.
	dir := a.installWasm(t, "junk", wasmManifest("junk", `{}`))
	if err := os.WriteFile(filepath.Join(dir, "run.wasm"), []byte("#!/bin/sh\necho hi\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := a.svc.Enable("junk", nil); err != nil {
		t.Fatal(err)
	}
	a.svc.opts.maxCrashes = 1
	a.svc.start()
	a.waitState(t, "junk", AddonCrashed)
	lines, _ := a.svc.Log("junk")
	if !strings.Contains(strings.Join(lines, "\n"), "could not start") {
		t.Errorf("log = %q", lines)
	}
}
