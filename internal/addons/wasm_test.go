package addons

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/addons/wasmtest"
)

// safeBuffer is a stderr the test reads while the module writes it.
type safeBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (s *safeBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *safeBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// waitFor waits for text in what the module logged.
func (s *safeBuffer) waitFor(t *testing.T, text string) {
	t.Helper()
	for deadline := time.Now().Add(2 * time.Minute); !strings.Contains(s.String(), text); time.Sleep(5 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %q in the module's log:\n%s", text, s.String())
		}
	}
}

// testLimits are the defaults, with a short tick.
func testLimits() WasmLimits {
	l := DefaultWasmLimits()
	l.Tick = 20 * time.Millisecond
	return l
}

// runWasm starts the test add-on in a mode.
func runWasm(t *testing.T, h *WasmHost, env []string, data string) (*WasmProc, *safeBuffer) {
	t.Helper()
	log := &safeBuffer{}
	p, err := h.Start(WasmSpec{
		Module: wasmtest.Module(t), Name: "test.wasm",
		Env:     append(WasmEnvironment(os.Environ(), "demo", data != ""), env...),
		DataDir: data, Stderr: log,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Stop(time.Second) })
	return p, log
}

// sharedCache is a compilation cache folder every test of the package uses:
// compiling the module is most of what a test costs (seconds, and tens of
// seconds under the race detector), so it is done once.
var sharedCache string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "sona-wasm-cache")
	if err != nil {
		panic(err)
	}
	sharedCache = dir
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func newHost(t *testing.T, limits WasmLimits) *WasmHost {
	t.Helper()
	h := NewWasmHost(sharedCache, limits)
	if h.CacheNote != "" {
		t.Fatalf("no disk cache: %s", h.CacheNote)
	}
	t.Cleanup(h.Close)
	return h
}

func waitDone(t *testing.T, p *WasmProc) {
	t.Helper()
	select {
	case <-p.Done():
	case <-time.After(2 * time.Minute):
		t.Fatal("the module did not end")
	}
}

// A wasm add-on speaks protocol v2 over its standard input and output:
// hello, events, an invoke answered, the shutdown event, a clean exit.
func TestWasmSpeaksTheProtocol(t *testing.T) {
	p, log := runWasm(t, newHost(t, testLimits()), nil, "")
	r := bufio.NewReader(p.Conn())
	readFrame := func() map[string]any {
		t.Helper()
		line, err := r.ReadBytes('\n')
		if err != nil {
			t.Fatalf("read: %v\n%s", err, log.String())
		}
		var f map[string]any
		if err := json.Unmarshal(line, &f); err != nil {
			t.Fatalf("frame %q: %v", line, err)
		}
		return f
	}
	write := func(s string) {
		t.Helper()
		if _, err := p.Conn().Write([]byte(s + "\n")); err != nil {
			t.Fatal(err)
		}
	}
	hello := readFrame()
	if hello["method"] != "hello" || hello["params"].(map[string]any)["id"] != "demo" {
		t.Fatalf("first frame = %v, want hello of demo", hello)
	}
	write(`{"id":1,"result":{"id":"demo","protocol":"2.0.0","permissions":[]}}`)
	log.waitFor(t, "hello ok")
	write(`{"event":"status","data":{}}`)
	log.waitFor(t, "event status")
	write(`{"id":7,"method":"invoke","params":{"action":"addon.demo.send","target":null,"deadlineMs":1000}}`)
	ans := readFrame()
	if ans["id"] != float64(7) || ans["result"].(map[string]any)["toast"] != "wasm says addon.demo.send" {
		t.Errorf("answer = %v", ans)
	}
	write(`{"event":"shutdown"}`)
	waitDone(t, p)
	if err := p.Err(); err != nil {
		t.Errorf("exit = %v, want a clean exit", err)
	}
}

// The end of its input is its signal to leave.
func TestWasmLeavesWhenItsInputEnds(t *testing.T) {
	p, log := runWasm(t, newHost(t, testLimits()), nil, "")
	if _, err := bufio.NewReader(p.Conn()).ReadBytes('\n'); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	p.Stop(10 * time.Second)
	if time.Since(start) > 5*time.Second {
		t.Errorf("Stop took %v: the module did not leave on end of input", time.Since(start))
	}
	if err := p.Err(); err != nil {
		t.Errorf("exit = %v", err)
	}
	if !strings.Contains(log.String(), "input ended") {
		t.Errorf("the module did not see the end of its input:\n%s", log.String())
	}
}

func TestWasmEnvironmentIsTheAllowlist(t *testing.T) {
	parent := []string{"HOME=/home/u", "PATH=/bin", "TMPDIR=/tmp", "LANG=de_DE.UTF-8", "LC_ALL=C", "TZ=UTC", "MUSIC_USER_TOKEN=secret", "DBUS_SESSION_BUS_ADDRESS=unix:x", "SONA_ADDON_ID=evil"}
	got := strings.Join(WasmEnvironment(parent, "demo", true), "\n")
	for _, want := range []string{"LANG=de_DE.UTF-8", "LC_ALL=C", "TZ=UTC", "SONA_ADDON_ID=demo", "SONA_ADDON_PROTOCOL=2", "SONA_ADDON_DATA=/data"} {
		if !strings.Contains(got, want) {
			t.Errorf("environment lacks %q:\n%s", want, got)
		}
	}
	for _, bad := range []string{"HOME", "PATH", "TMPDIR", "MUSIC_USER_TOKEN", "DBUS_", "SONA_ADDON_ID=evil"} {
		if strings.Contains(got, bad) {
			t.Errorf("environment has %q:\n%s", bad, got)
		}
	}
	if strings.Contains(strings.Join(WasmEnvironment(parent, "demo", false), "\n"), "SONA_ADDON_DATA") {
		t.Error("a module with no data folder was told about one")
	}

	// And what the module sees is exactly that.
	p, log := runWasm(t, newHost(t, testLimits()), []string{"LC_WASMTEST_MODE=env"}, "")
	log.waitFor(t, "env SONA_ADDON_PROTOCOL=2")
	if s := log.String(); strings.Contains(s, "env HOME=") || strings.Contains(s, "env PATH=") {
		t.Errorf("the module saw the host's paths:\n%s", s)
	}
	_ = p
}

// A module can't read a file that is not preopened: no path of the host
// exists for it, `..` and links lead nowhere, and it can't make a link.
func TestWasmReachesOnlyItsDataFolder(t *testing.T) {
	root := t.TempDir()
	secret := filepath.Join(root, "secret.txt")
	if err := os.WriteFile(secret, []byte("do not read"), 0o600); err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(root, "data")
	if err := os.Mkdir(data, 0o700); err != nil {
		t.Fatal(err)
	}
	// Links the user (or a past add-on) left in the folder: out to a file and
	// out to a folder.
	if err := os.Symlink(secret, filepath.Join(data, "planted-link")); err != nil {
		t.Skipf("no symbolic links here: %v", err)
	}
	if err := os.Symlink(root, filepath.Join(data, "planted-dir")); err != nil {
		t.Fatal(err)
	}
	env := []string{"LC_WASMTEST_MODE=files", "LC_WASMTEST_SECRET=" + secret}
	p, log := runWasm(t, newHost(t, testLimits()), env, data)
	log.waitFor(t, "probe planted-dir")
	p.Stop(time.Second)
	got := log.String()
	for probe, want := range map[string]string{
		"etc-passwd":          "denied", // not preopened
		"dotdot":              "denied",
		"secret-by-host-path": "denied", // the host's path means nothing to it
		"data-write":          "ok",
		"data-read":           "ok",
		"data-mkdir":          "ok",
		"data-nested":         "ok",
		"symlink-out":         "denied", // it can't make a link
		"symlink-in":          "denied",
		"planted-link":        "denied", // and can't follow one
		"planted-dir":         "denied",
	} {
		if line := "probe " + probe + ": " + want; !strings.Contains(got, line) {
			t.Errorf("want %q in:\n%s", line, got)
		}
	}
	if b, err := os.ReadFile(filepath.Join(data, "hello.txt")); err != nil || string(b) != "hi" {
		t.Errorf("the data folder's file = %q, %v", b, err)
	}
	if _, err := os.Lstat(filepath.Join(data, "new-link")); err == nil {
		t.Error("the module made a link")
	}

	// Without a data folder it has no file at all.
	p, log = runWasm(t, newHost(t, testLimits()), env, "")
	log.waitFor(t, "probe planted-dir")
	p.Stop(time.Second)
	if got := log.String(); strings.Contains(got, ": ok") {
		t.Errorf("a module without a data folder reached a file:\n%s", got)
	}
}

// A module that spins is stopped, and says why; one that waits is not.
func TestWasmRunawayCPUIsStopped(t *testing.T) {
	l := testLimits()
	l.CPUBurst, l.CPUShare = 1500*time.Millisecond, 0.1
	h := newHost(t, l)

	p, log := runWasm(t, h, []string{"LC_WASMTEST_MODE=loop"}, "")
	log.waitFor(t, "looping")
	waitDone(t, p)
	if err := p.Err(); err != ErrWasmCPU {
		t.Errorf("exit = %v, want %v", err, ErrWasmCPU)
	}

	// The same budget leaves an add-on that waits for its input alone, however long.
	idle, _ := runWasm(t, h, nil, "")
	go func() { _, _ = bufio.NewReader(idle.Conn()).ReadBytes('\n') }()
	time.Sleep(3 * l.CPUBurst)
	select {
	case <-idle.Done():
		t.Fatalf("an idle add-on was stopped: %v", idle.Err())
	default:
	}
}

// A module that never listens is killed when it is stopped.
func TestWasmStopKillsADeafModule(t *testing.T) {
	p, log := runWasm(t, newHost(t, testLimits()), []string{"LC_WASMTEST_MODE=deaf"}, "")
	log.waitFor(t, "deaf")
	start := time.Now()
	p.Stop(100 * time.Millisecond)
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("Stop took %v", d)
	}
	select {
	case <-p.Done():
	default:
		t.Fatal("Stop returned with the module running")
	}
	if err := p.Err(); err == nil {
		t.Error("a killed module exited cleanly")
	}
}

// A module that allocates past its cap fails to grow and ends.
func TestWasmMemoryIsCapped(t *testing.T) {
	l := testLimits()
	l.MemoryPages = 400 // 25 MiB
	p, log := runWasm(t, newHost(t, l), []string{"LC_WASMTEST_MODE=alloc"}, "")
	waitDone(t, p)
	if p.Err() == nil {
		t.Error("a module that ran out of memory exited cleanly")
	}
	s := log.String()
	if !strings.Contains(s, "out of memory") {
		t.Errorf("the module's log doesn't say it ran out of memory:\n%s", s)
	}
	for _, line := range strings.Split(s, "\n") {
		var n int
		if _, err := fmt.Sscanf(line, "alloc %d MiB", &n); err == nil && n >= 25 {
			t.Errorf("%q: the module allocated %d MiB under a 25 MiB cap", line, n)
		}
	}
}

func TestWasmExitStatus(t *testing.T) {
	p, _ := runWasm(t, newHost(t, testLimits()), []string{"LC_WASMTEST_MODE=exit"}, "")
	waitDone(t, p)
	if err := p.Err(); err == nil || err.Error() != "exit status 3" {
		t.Errorf("exit = %v, want exit status 3", err)
	}
}

func TestWasmRefusesWhatIsNotAModule(t *testing.T) {
	h := newHost(t, testLimits())
	p, err := h.Start(WasmSpec{Module: []byte("\x00asm\x01\x00\x00\x00junk"), Name: "x.wasm"})
	if err != nil {
		t.Fatal(err)
	}
	waitDone(t, p)
	if p.Err() == nil {
		t.Error("garbage ran")
	}
	if _, err := h.Start(WasmSpec{Module: wasmtest.Module(t), DataDir: filepath.Join(t.TempDir(), "missing")}); err == nil {
		t.Error("a data folder that isn't there was mounted")
	}
}

// Compiled modules are kept in the cache folder, which is dropped when it
// isn't safe to load code from.
func TestWasmCache(t *testing.T) {
	dir := t.TempDir()
	h := NewWasmHost(dir, testLimits())
	if h.CacheNote != "" {
		t.Fatalf("no disk cache: %s", h.CacheNote)
	}
	p, _ := runWasm(t, h, nil, "")
	if _, err := bufio.NewReader(p.Conn()).ReadBytes('\n'); err != nil {
		t.Fatal(err)
	}
	p.Stop(time.Second)
	h.Close()
	var files int
	_ = filepath.WalkDir(dir, func(_ string, d os.DirEntry, _ error) error {
		if d.Type().IsRegular() {
			files++
		}
		return nil
	})
	if files == 0 {
		t.Error("nothing was cached")
	}
	if runtime.GOOS == "windows" {
		return
	}

	// An entry others can write is not loaded: the cache starts again.
	var entry string
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, _ error) error {
		if d.Type().IsRegular() && entry == "" {
			entry = p
		}
		return nil
	})
	if err := os.Chmod(entry, 0o666); err != nil {
		t.Fatal(err)
	}
	h2 := NewWasmHost(dir, testLimits())
	defer h2.Close()
	if h2.CacheNote != "" {
		t.Errorf("the cache was not rebuilt: %s", h2.CacheNote)
	}
	if _, err := os.Lstat(entry); err == nil {
		t.Error("an entry that others could write was kept")
	}
}
