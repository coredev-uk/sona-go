package uitest

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/zalando/go-keyring"

	"github.com/coredev-uk/sona-go/internal/auth/store"
	"github.com/coredev-uk/sona-go/internal/discord"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/fixture"
)

// A real secret in the process keyring (go-keyring's mock stands for the
// machine's Secret Service) is unreachable from a uitest Core, and the Core's
// own keyring is the one it reads.
func TestCoreCannotReachTheProcessKeyring(t *testing.T) {
	keyring.MockInit()
	for _, provider := range []string{services.ScrobblerLastFM, services.ScrobblerListenBrainz} {
		if err := keyring.Set(store.Service, provider, "real-secret-of-"+provider); err != nil {
			t.Fatal(err)
		}
	}
	h := New(t)
	sc := services.Service[services.ScrobbleService](h.Core)

	st := sc.Status() // reads the stored credentials
	if !st.LastFM.Available {
		t.Error("Last.fm is not available: the harness gives the build a made-up account")
	}
	if st.LastFM.Connected || st.ListenBrainz.Connected {
		t.Fatalf("the Core found the process keyring's secrets: %+v", st)
	}
	if st.Unencrypted {
		t.Error("the Core fell back to a credentials file: its keyring is not the fake")
	}
	if got, err := keyring.Get(store.Service, services.ScrobblerLastFM); err != nil || got == "" {
		t.Errorf("the process keyring lost its secret: %q, %v", got, err)
	}

	// What the test puts in the Core's keyring is what the services read.
	h2 := New(t)
	if err := h2.Keyring.Set(store.Service, services.ScrobblerListenBrainz, "token"); err != nil {
		t.Fatal(err)
	}
	if st := services.Service[services.ScrobbleService](h2.Core).Status(); !st.ListenBrainz.Connected || st.LastFM.Connected {
		t.Errorf("a secret in the harness keyring: %+v", st)
	}
}

// A request to a host nobody serves fails at once, naming the host, whoever
// makes it: the Core's services, the auth manager's client, a test.
func TestNetworkOutsideTheFixturesFailsFast(t *testing.T) {
	h := New(t, Version("1.0.0"))

	t.Run("update check", func(t *testing.T) {
		start := time.Now()
		_, err := services.Service[services.UpdatesService](h.Core).Check(t.Context())
		assertOffline(t, err, "api.github.com", start)
	})
	t.Run("scrobbler", func(t *testing.T) {
		// The services' HTTP waits for the first paint.
		services.Service[services.SystemService](h.Core).FirstPaint()
		start := time.Now()
		_, err := services.Service[services.ScrobbleService](h.Core).ConnectListenBrainz(t.Context(), "token")
		assertOffline(t, err, "api.listenbrainz.org", start)
		if services.CodeOf(err) != services.CodeNetwork {
			t.Errorf("the services classify it as %q, not a network error", services.CodeOf(err))
		}
	})
	t.Run("client", func(t *testing.T) {
		start := time.Now()
		_, err := h.Network.Client().Get("https://example.com/path")
		assertOffline(t, err, "example.com/path", start)
	})
	if got := h.Network.Blocked(); len(got) < 3 {
		t.Errorf("Blocked lists %v", got)
	}
}

func assertOffline(t *testing.T, err error, want string, start time.Time) {
	t.Helper()
	if !errors.Is(err, ErrOffline) {
		t.Fatalf("error %v is not ErrOffline", err)
	}
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error %q does not name %s", err, want)
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("the request failed after %v", d)
	}
}

// The Network serves what the test registers, in process, and the fixture's
// covers.
func TestNetworkServesFixtureHosts(t *testing.T) {
	h := New(t, Version("1.0.0"))
	h.Network.ServeHandler("api.github.com", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/coredev-uk/sona-go/releases/latest" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"tag_name":"v2.0.0","published_at":"2026-01-01T00:00:00Z"}`))
	}))
	st, err := services.Service[services.UpdatesService](h.Core).Check(t.Context())
	if err != nil || !st.Available || st.Latest != "2.0.0" {
		t.Errorf("update check over the fixture: %+v, %v", st, err)
	}
	resp, err := h.Network.Client().Get("https://" + fixture.CDNHost + "/image/thumb/Fixture/cover1/16x16bb.png")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("a fixture cover: %v, %v", resp, err)
	}
	_ = resp.Body.Close()
	if len(h.Network.Blocked()) != 0 {
		t.Errorf("blocked %v", h.Network.Blocked())
	}
}

// The process environment a Core reads is the test's: nothing under the real
// home or XDG directories, no bus, no Discord, and Bare is isolated too.
func TestEnvironmentIsPrivate(t *testing.T) {
	realHome, _ := os.UserHomeDir()
	for _, opts := range [][]Option{nil, {Bare()}} {
		h := New(t, opts...)
		for _, dir := range []func() (string, error){os.UserHomeDir, os.UserConfigDir} {
			d, err := dir()
			if err != nil || !strings.HasPrefix(d, h.Dir+string(filepath.Separator)) {
				t.Errorf("%q (%v) is outside the test's directory %s (real home %s)", d, err, h.Dir, realHome)
			}
		}
		for _, k := range []string{"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_RUNTIME_DIR"} {
			if v := os.Getenv(k); !strings.HasPrefix(v, h.Dir+string(filepath.Separator)) {
				t.Errorf("%s = %q is outside the test's directory", k, v)
			}
		}
		// The cache is the per-user test cache (MyGo's font cache), beside
		// Sona's own: never the real cache directory itself.
		c, err := os.UserCacheDir()
		if err != nil || filepath.Base(c) != "uitest" || filepath.Base(filepath.Dir(c)) != "sona-tests" {
			t.Errorf("the cache directory %q (%v) is not the uitest test cache", c, err)
		}
		if id := discord.ClientID(); id != "" {
			t.Errorf("a Discord application %q is configured", id)
		}
		start := time.Now()
		if conn, err := dbus.SessionBus(); err == nil {
			_ = conn.Close()
			t.Error("the session bus is reachable")
		}
		if conn, err := dbus.SystemBus(); err == nil {
			_ = conn.Close()
			t.Error("the system bus is reachable")
		}
		if time.Since(start) > time.Second {
			t.Errorf("the bus connections took %v to fail", time.Since(start))
		}
		if _, err := net.Dial("unix", filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "discord-ipc-0")); err == nil {
			t.Error("a Discord socket answers")
		}
	}
}

// Code that reaches for http.DefaultClient (or a client with no transport) is
// held to the same Network for the length of the test, loopback servers of the
// test's own excepted; the default transport comes back afterwards.
func TestDefaultClientFailsFast(t *testing.T) {
	before := http.DefaultTransport
	t.Run("test", func(t *testing.T) {
		h := New(t, Bare())
		start := time.Now()
		_, err := http.DefaultClient.Get("https://example.com/")
		assertOffline(t, err, "example.com", start)
		_, err = (&http.Client{}).Get("https://api.github.com/")
		assertOffline(t, err, "api.github.com", start)
		if got := h.Network.Blocked(); len(got) != 2 {
			t.Errorf("Blocked lists %v", got)
		}

		srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		defer srv.Close()
		resp, err := http.Get(srv.URL)
		if err != nil {
			t.Fatalf("a loopback server of the test: %v", err)
		}
		_ = resp.Body.Close()
	})
	if http.DefaultTransport != before {
		t.Error("http.DefaultTransport was not restored")
	}
}
