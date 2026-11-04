//go:build e2e

// Package e2e drives the built Sona from outside, in a headless desktop
// session: `headless -- go test -tags e2e ./e2e/...` (CI: the e2e-headless
// job). It builds bin/sona-e2e with -tags probe,sqlite_fts5 (MyGo's
// automation plugin linked in, and the fake playback engine and local
// artwork server of cmd/sona/e2e_probe.go), starts it on a fixture profile
// (internal/ui/uitest/fixture: synthetic library, signed out, no network)
// with MYGO_AUTOMATION on a private socket, and drives it through that
// socket (e2e/automation) and reads its accessibility tree from the AT-SPI
// bus (e2e/atspi).
//
// Environment: E2E_BIN skips the build and runs that binary; E2E_OUT is a
// directory the test keeps its screenshots, the app's stderr and its sona.log in (default:
// the test's temporary directory, removed afterwards).
package e2e

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/e2e/automation"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/fixture"
)

const startTimeout = 30 * time.Second

var (
	buildOnce sync.Once
	appBin    string // the app under test
	profBin   string // e2e/profile, which writes the fixture profile
	buildErr  error
)

// build builds the binaries of the run once, with the tags the app needs:
// probe (the automation plugin and the e2e seam) and sqlite_fts5. The
// profile writer is a binary of its own so this package, built with just
// -tags e2e, needs no SQLite feature tag itself.
func build(t *testing.T) (app, profile string) {
	t.Helper()
	buildOnce.Do(func() {
		root, err := filepath.Abs("..")
		if err != nil {
			buildErr = err
			return
		}
		goBuild := func(out, pkg string, tags string) {
			if buildErr != nil {
				return
			}
			cmd := exec.Command("go", "build", "-tags", tags, "-o", out, pkg)
			cmd.Dir = root
			if b, err := cmd.CombinedOutput(); err != nil {
				buildErr = fmt.Errorf("go build %s: %v\n%s", pkg, err, b)
			}
		}
		profBin = filepath.Join(root, "bin", "e2e-profile")
		goBuild(profBin, "./e2e/profile", "sqlite_fts5")
		if p := os.Getenv("E2E_BIN"); p != "" {
			appBin, err = filepath.Abs(p)
			if err != nil {
				buildErr = err
			}
			return
		}
		appBin = filepath.Join(root, "bin", "sona-e2e")
		goBuild(appBin, "./cmd/sona", "probe,sqlite_fts5")
	})
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	return appBin, profBin
}

// app is a running Sona on a fixture profile.
type app struct {
	t      *testing.T
	cmd    *exec.Cmd
	done   chan struct{} // closed when the process has exited
	client *automation.Client
	paths  config.Paths
	out    string // where screenshots and logs go
	stderr string // the app's stderr file
	covers *coverServer
	start  time.Time
}

// coverServer answers the artwork requests of the app (the probe build sends
// all its HTTP here) with the fixture's generated covers.
type coverServer struct {
	srv  *httptest.Server
	mu   sync.Mutex
	hits int
}

func newCoverServer() *coverServer {
	cs := &coverServer{}
	h := fixture.Handler()
	cs.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cs.mu.Lock()
		cs.hits++
		cs.mu.Unlock()
		h.ServeHTTP(w, r)
	}))
	return cs
}

func (c *coverServer) addr() string { return c.srv.Listener.Addr().String() }

func (c *coverServer) served() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hits
}

// outDir is where artefacts go: E2E_OUT, else the test's temporary directory.
func outDir(t *testing.T) string {
	if d := os.Getenv("E2E_OUT"); d != "" {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		return d
	}
	return t.TempDir()
}

// startApp writes a fixture profile in private directories, starts the app
// on it and connects to its automation socket.
func startApp(t *testing.T) *app { return startAppWith(t) }

// startAppWith is startApp with arguments for the profile writer
// (e2e/profile: -first-run leaves the first-run setup undone).
func startAppWith(t *testing.T, profileArgs ...string) *app {
	t.Helper()
	if os.Getenv("WAYLAND_DISPLAY") == "" {
		t.Skip("no Wayland display: run through `headless -- go test -tags e2e ./e2e/...`")
	}
	bin, profile := build(t)

	// Short paths: a Unix socket's path is limited to 108 bytes.
	root, err := os.MkdirTemp("", "sona-e2e")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	env := map[string]string{
		"XDG_CONFIG_HOME": filepath.Join(root, "config"),
		"XDG_DATA_HOME":   filepath.Join(root, "data"),
		"XDG_STATE_HOME":  filepath.Join(root, "state"),
		"XDG_CACHE_HOME":  filepath.Join(root, "cache"),
	}
	paths, err := config.ResolvePaths(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	prof := exec.Command(profile, profileArgs...)
	prof.Env = append(os.Environ(),
		"XDG_CONFIG_HOME="+env["XDG_CONFIG_HOME"], "XDG_DATA_HOME="+env["XDG_DATA_HOME"],
		"XDG_STATE_HOME="+env["XDG_STATE_HOME"], "XDG_CACHE_HOME="+env["XDG_CACHE_HOME"])
	if out, err := prof.CombinedOutput(); err != nil {
		t.Fatalf("the fixture profile: %v\n%s", err, out)
	}

	a := &app{t: t, paths: paths, out: outDir(t), covers: newCoverServer(), done: make(chan struct{})}
	// Registered before quit, so it runs after the app has ended and before the
	// profile directory (with the log) is removed.
	t.Cleanup(a.keepLog)
	t.Cleanup(a.covers.srv.Close)
	sock := filepath.Join(root, "automation.sock")
	a.stderr = filepath.Join(a.out, strings.ReplaceAll(t.Name(), "/", "_")+"-stderr.log")
	logFile, err := os.Create(a.stderr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = logFile.Close() })

	a.cmd = exec.Command(bin)
	a.cmd.Env = append(os.Environ(),
		"XDG_CONFIG_HOME="+env["XDG_CONFIG_HOME"], "XDG_DATA_HOME="+env["XDG_DATA_HOME"],
		"XDG_STATE_HOME="+env["XDG_STATE_HOME"], "XDG_CACHE_HOME="+env["XDG_CACHE_HOME"],
		"MYGO_AUTOMATION="+sock,
		"SONA_E2E=1", "SONA_E2E_CDN="+a.covers.addr(), "SONA_PRIVATE=1",
	)
	a.cmd.Stdout, a.cmd.Stderr = logFile, logFile
	a.start = time.Now()
	if err := a.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { _ = a.cmd.Wait(); close(a.done) }()
	t.Cleanup(a.quit)
	t.Cleanup(func() {
		if t.Failed() {
			a.dumpLogs()
		}
	})

	a.client, err = automation.Dial(sock, startTimeout)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.client.Close() })
	return a
}

// quit stops the app (SIGTERM, then SIGKILL after 10 s); idempotent.
func (a *app) quit() {
	select {
	case <-a.done:
		return
	default:
	}
	_ = a.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-a.done:
	case <-time.After(10 * time.Second):
		_ = a.cmd.Process.Kill()
		<-a.done
	}
}

// log is the app's own log (the state directory's sona.log).
func (a *app) log() string {
	b, _ := os.ReadFile(a.paths.LogFile())
	return string(b)
}

// keepLog copies the app's own log into the output directory: it lives in the
// profile's state directory, which is removed with the test.
func (a *app) keepLog() {
	b, err := os.ReadFile(a.paths.LogFile())
	if err != nil {
		a.t.Logf("the app's log: %v", err)
		return
	}
	name := strings.ReplaceAll(a.t.Name(), "/", "_") + "-sona.log"
	if err := os.WriteFile(filepath.Join(a.out, name), b, 0o644); err != nil {
		a.t.Logf("keeping the app's log: %v", err)
	}
}

func (a *app) dumpLogs() {
	a.t.Helper()
	tail := func(s string) string {
		lines := strings.Split(strings.TrimSpace(s), "\n")
		return strings.Join(lines[max(0, len(lines)-40):], "\n")
	}
	b, _ := os.ReadFile(a.stderr)
	a.t.Logf("app stderr (tail):\n%s\napp log (tail):\n%s", tail(string(b)), tail(a.log()))
}

// until polls cond until it holds or the timeout passes.
func until(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(timeout); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}
