package uitest

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/zalando/go-keyring"
)

// A uitest Core is hermetic: whatever the machine running the tests has (a
// signed-in keyring, a network, a session bus, Sona's own settings), the
// services see none of it. Four seams carry that:
//
//   - Keyring, an in-memory store handed to the services (Deps.Keyring), so
//     the Secret Service is never asked;
//   - Network, the transport of every HTTP client the services and the
//     artwork cache use (Deps.Transport, artwork.Options.HTTP), which serves
//     the fixtures' hosts and fails any other request at once;
//   - http.DefaultTransport, which for the length of the test is the same
//     Network (loopback excepted, for httptest servers), so code that
//     reaches for http.DefaultClient fails as well;
//   - the process environment (isolate): home and XDG directories under the
//     test's temporary directory, the buses unreachable, and the build
//     settings of the integrations (Last.fm account, Discord application)
//     fixed.

// ErrOffline is what a request to a host the test does not serve fails with
// (errors.Is).
var ErrOffline = errors.New("uitest: network access is blocked in tests")

// offlineError is the error of a blocked request. It is a net.Error, so the
// services classify it as they do a real network failure.
type offlineError struct {
	method, host, path string
	served             []string
}

func (e *offlineError) Error() string {
	served := "no host"
	if len(e.served) > 0 {
		served = strings.Join(e.served, ", ")
	}
	return fmt.Sprintf("%v: %s %s%s (this test serves %s; Harness.Network.Serve adds a fixture host)",
		ErrOffline, e.method, e.host, e.path, served)
}
func (e *offlineError) Is(target error) bool { return target == ErrOffline }
func (e *offlineError) Timeout() bool        { return false }
func (e *offlineError) Temporary() bool      { return false }

// Network is the only network a test has: an http.RoundTripper that serves
// the hosts registered with Serve and fails every other request without
// dialling. Matching is on the host with its port first, then on the host
// alone.
type Network struct {
	mu      sync.Mutex
	routes  map[string]http.RoundTripper
	blocked []string
}

// NewNetwork returns a Network that serves nothing.
func NewNetwork() *Network { return &Network{routes: map[string]http.RoundTripper{}} }

// Serve answers requests to host (a name, or name:port) with rt.
func (n *Network) Serve(host string, rt http.RoundTripper) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.routes[strings.ToLower(host)] = rt
}

// ServeHandler answers requests to host with h, in process (no socket).
func (n *Network) ServeHandler(host string, h http.Handler) { n.Serve(host, handlerTransport{h}) }

// Blocked lists the requests that failed, as "METHOD host/path", in order.
func (n *Network) Blocked() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return slices.Clone(n.blocked)
}

// Client is an http.Client over the Network.
func (n *Network) Client() *http.Client { return &http.Client{Transport: n} }

// RoundTrip implements http.RoundTripper.
func (n *Network) RoundTrip(req *http.Request) (*http.Response, error) {
	n.mu.Lock()
	rt, ok := n.routes[strings.ToLower(req.URL.Host)]
	if !ok {
		rt, ok = n.routes[strings.ToLower(req.URL.Hostname())]
	}
	if ok {
		n.mu.Unlock()
		return rt.RoundTrip(req)
	}
	err := &offlineError{method: req.Method, host: req.URL.Host, path: req.URL.Path}
	for h := range n.routes {
		err.served = append(err.served, h)
	}
	slices.Sort(err.served)
	n.blocked = append(n.blocked, req.Method+" "+req.URL.Host+req.URL.Path)
	n.mu.Unlock()
	if req.Body != nil {
		_ = req.Body.Close()
	}
	return nil, err
}

// handlerTransport runs a handler for each request, in process.
type handlerTransport struct{ h http.Handler }

func (t handlerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	t.h.ServeHTTP(rec, req)
	resp := rec.Result()
	resp.Request = req
	return resp, nil
}

// Keyring is an in-memory secret store: what the services' Deps.Keyring is
// in a test, in place of the machine's Secret Service. It holds nothing
// until the test puts something in.
type Keyring struct {
	mu sync.Mutex
	m  map[[2]string]string
}

// NewKeyring returns an empty Keyring.
func NewKeyring() *Keyring { return &Keyring{m: map[[2]string]string{}} }

// Get implements authstore.Keyring: keyring.ErrNotFound when absent.
func (k *Keyring) Get(service, user string) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	v, ok := k.m[[2]string{service, user}]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return v, nil
}

// Set implements authstore.Keyring.
func (k *Keyring) Set(service, user, password string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.m[[2]string{service, user}] = password
	return nil
}

// Delete implements authstore.Keyring.
func (k *Keyring) Delete(service, user string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if _, ok := k.m[[2]string{service, user}]; !ok {
		return keyring.ErrNotFound
	}
	delete(k.m, [2]string{service, user})
	return nil
}

// processCache is the XDG cache directory of every uitest test: one per
// user, kept between runs, beside (never inside) Sona's own cache, as
// cdmtest keeps its CDM. It outlives the tests on purpose: MyGo registers
// the interface font once per process from a file it writes under the user
// cache directory, and fontconfig keeps its cache there; both write
// atomically, so test processes share it and nothing piles up.
var processCache = sync.OnceValues(func() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "sona-tests", "uitest")
	return dir, os.MkdirAll(dir, 0o700)
})

// denyDefaultTransport makes n the http.DefaultTransport until the test
// ends, loopback aside (httptest servers are the test's own), so a client
// built on http.DefaultClient or a nil transport fails like the Core's.
// The tests that call it cannot run in parallel (they set the environment).
func denyDefaultTransport(t testing.TB, n *Network) {
	t.Helper()
	orig := http.DefaultTransport
	http.DefaultTransport = defaultGuard{n: n, real: orig}
	t.Cleanup(func() { http.DefaultTransport = orig })
}

// defaultGuard is what denyDefaultTransport installs.
type defaultGuard struct {
	n    *Network
	real http.RoundTripper
}

func (g defaultGuard) RoundTrip(req *http.Request) (*http.Response, error) {
	if h := req.URL.Hostname(); h == "localhost" || net.ParseIP(h).IsLoopback() {
		return g.real.RoundTrip(req)
	}
	return g.n.RoundTrip(req)
}

// isolate points everything the process reads from its environment at dir,
// for the length of the test: HOME and the XDG directories (config, data,
// state, runtime), both D-Bus buses (an address nothing listens on: a
// connection fails at once instead of reaching the real bus), and the
// variables that would give the integrations a real account (the Discord
// application, the Last.fm API key; Last.fm gets a made-up one so its
// section is available, its calls fail on the Network). What os.UserHomeDir,
// os.UserConfigDir and os.UserCacheDir answer follows on each OS: the XDG
// variables on Linux; on macOS $HOME/Library, whose Caches is a link to the
// shared test cache (processCache); on Windows %USERPROFILE%, %AppData% and
// %LocalAppData%, which is the shared test cache itself.
func isolate(t testing.TB, dir string) {
	t.Helper()
	home := filepath.Join(dir, "home")
	run := filepath.Join(dir, "run")
	cache, err := processCache()
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{home, run} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	env := map[string]string{
		"HOME":                     home,
		"XDG_CONFIG_HOME":          filepath.Join(home, ".config"),
		"XDG_DATA_HOME":            filepath.Join(home, ".local", "share"),
		"XDG_STATE_HOME":           filepath.Join(home, ".local", "state"),
		"XDG_CACHE_HOME":           cache,
		"XDG_RUNTIME_DIR":          run,
		"DBUS_SESSION_BUS_ADDRESS": "unix:path=" + filepath.Join(run, "no-bus"),
		"DBUS_SYSTEM_BUS_ADDRESS":  "unix:path=" + filepath.Join(run, "no-bus"),
		"SONA_DISCORD_CLIENT_ID":   "",
		"SONA_LASTFM_KEY":          "uitest-key",
		"SONA_LASTFM_SECRET":       "uitest-secret",
		"SONA_DEV_INSTANCE":        "",
		"APPIMAGE":                 "",
	}
	switch runtime.GOOS {
	case "darwin":
		// os.UserCacheDir is $HOME/Library/Caches and ignores XDG_CACHE_HOME.
		lib := filepath.Join(home, "Library")
		if err := os.MkdirAll(lib, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(cache, filepath.Join(lib, "Caches")); err != nil && !errors.Is(err, os.ErrExist) {
			t.Fatal(err)
		}
	case "windows":
		env["USERPROFILE"] = home
		env["AppData"] = filepath.Join(home, "AppData", "Roaming")
		env["LocalAppData"] = cache // os.UserCacheDir is %LocalAppData% itself
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
}
