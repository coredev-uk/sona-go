package broadcast

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestDialControl(t *testing.T) {
	refused := []string{
		"127.0.0.1", "127.8.9.10", "::1",
		"10.0.0.1", "172.16.0.1", "172.31.255.255", "192.168.1.1", "fd00::1", "fc00::1",
		"169.254.169.254", "169.254.0.1", "fe80::1",
		"100.64.0.1", "100.127.255.255",
		"0.0.0.0", "0.1.2.3", "::",
		"224.0.0.1", "239.255.255.250", "ff02::1", "ff05::2",
		"240.0.0.1", "255.255.255.255", "198.18.0.1", "192.0.0.8",
		"::ffff:127.0.0.1", "::ffff:10.0.0.1", "::ffff:169.254.169.254",
		"64:ff9b::7f00:1", "64:ff9b::a00:1", "64:ff9b::a9fe:a9fe",
	}
	for _, ip := range refused {
		err := dialControl("tcp", net.JoinHostPort(ip, "80"), nil)
		if !errors.Is(err, ErrPrivateAddress) {
			t.Errorf("dialControl(%s) = %v, want ErrPrivateAddress", ip, err)
		}
	}
	allowed := []string{"8.8.8.8", "93.184.216.34", "2606:4700:4700::1111", "::ffff:8.8.8.8", "64:ff9b::808:808", "100.128.0.1", "172.32.0.1"}
	for _, ip := range allowed {
		if err := dialControl("tcp", net.JoinHostPort(ip, "443"), nil); err != nil {
			t.Errorf("dialControl(%s) = %v, want nil", ip, err)
		}
	}
	// A name is never dialled: the dialer has resolved it by now.
	if err := dialControl("tcp", "localhost:80", nil); !errors.Is(err, ErrPrivateAddress) {
		t.Errorf("dialControl(localhost) = %v, want ErrPrivateAddress", err)
	}
	if !PublicAddr(netip.MustParseAddr("1.1.1.1")) || PublicAddr(netip.Addr{}) {
		t.Error("PublicAddr(1.1.1.1) false or PublicAddr(invalid) true")
	}
}

// The production client cannot reach a loopback server at all.
func TestClientRefusesLoopback(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	defer srv.Close()
	err := Play(t.Context(), Options{HTTP: NewClient(), UserAgent: "sona-test"}, srv.URL+"/stream", &recorder{})
	if !errors.Is(err, ErrPrivateAddress) {
		t.Errorf("Play = %v, want ErrPrivateAddress", err)
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("server got %d requests", n)
	}
}

// guardedClient is the production client but for letting through the
// address of the server standing for the broadcaster; every other address,
// loopback included, is refused as in production.
func guardedClient(allow string) *http.Client {
	return newClient(func(network, address string, c syscall.RawConn) error {
		if address == allow {
			return nil
		}
		return dialControl(network, address, c)
	})
}

// target is a server standing for a victim on the user's network: it
// counts every request.
func target(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "audio/aac")
		_, _ = w.Write(make([]byte, 1024))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func hostOf(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	return u.Host
}

// A broadcaster (A) that names another address (B) in a playlist, a
// redirect or an HLS playlist does not make Sona send B a request.
func TestBroadcasterCannotAimRequestsAtPrivateAddresses(t *testing.T) {
	b, bHits := target(t)
	for _, tc := range []struct {
		name, ct, body string
		redirect       bool
	}{
		{name: "PLS", ct: "audio/x-scpls", body: "[playlist]\nNumberOfEntries=1\nFile1=%s/admin\n"},
		{name: "M3U", ct: "audio/x-mpegurl", body: "#EXTM3U\n%s/admin\n"},
		{name: "HLS", ct: "application/vnd.apple.mpegurl", body: "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:2\n#EXT-X-MEDIA-SEQUENCE:0\n#EXTINF:2.0,\n%s/admin.ts\n#EXT-X-ENDLIST\n"},
		{name: "redirect", redirect: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/seg0.ts" {
					_, _ = w.Write(make([]byte, 188))
					return
				}
				if tc.redirect {
					http.Redirect(w, r, b.URL+"/admin", http.StatusFound)
					return
				}
				w.Header().Set("Content-Type", tc.ct)
				_, _ = fmt.Fprintf(w, tc.body, b.URL)
			}))
			defer a.Close()
			before := bHits.Load()
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			err := Play(ctx, Options{HTTP: guardedClient(hostOf(t, a)), UserAgent: "sona-test"}, a.URL+"/live", &recorder{})
			if !errors.Is(err, ErrPrivateAddress) {
				t.Errorf("Play = %v, want ErrPrivateAddress", err)
			}
			if n := bHits.Load() - before; n != 0 {
				t.Errorf("the target got %d requests", n)
			}
		})
	}
}

// A chain of redirects is followed 5 hops and no further.
func TestRedirectCap(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	a := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		var n int
		_, _ = fmt.Sscanf(r.URL.Path, "/r/%d", &n)
		http.Redirect(w, r, fmt.Sprintf("/r/%d", n+1), http.StatusFound)
	}))
	defer a.Close()
	err := Play(t.Context(), Options{HTTP: guardedClient(hostOf(t, a)), UserAgent: "sona-test"}, a.URL+"/r/0", &recorder{})
	if err == nil || !strings.Contains(err.Error(), "redirects") {
		t.Fatalf("Play = %v, want a redirect limit error", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(paths) != maxRedirects+1 {
		t.Errorf("%d requests %v, want the first and %d redirects", len(paths), paths, maxRedirects)
	}
}

func TestCheckRedirect(t *testing.T) {
	mk := func(u string) *http.Request {
		r, err := http.NewRequest(http.MethodGet, u, nil)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	for _, tc := range []struct {
		name  string
		to    string
		via   []string
		allow bool
	}{
		{"http to http", "http://b/x", []string{"http://a/"}, true},
		{"http to https", "https://b/x", []string{"http://a/"}, true},
		{"https to https", "https://b/x", []string{"https://a/"}, true},
		{"https to http", "http://b/x", []string{"https://a/"}, false},
		{"https then http then http", "http://c/x", []string{"https://a/", "http://b/"}, true},
		{"ftp", "ftp://b/x", []string{"http://a/"}, false},
		{"file", "file:///etc/passwd", []string{"https://a/"}, false},
		{"scheme-relative gopher", "gopher://b/", []string{"http://a/"}, false},
	} {
		var via []*http.Request
		for _, v := range tc.via {
			via = append(via, mk(v))
		}
		if err := checkRedirect(mk(tc.to), via); (err == nil) != tc.allow {
			t.Errorf("%s: checkRedirect = %v, allowed %v", tc.name, err, tc.allow)
		}
	}
	via := make([]*http.Request, maxRedirects+1)
	for i := range via {
		via[i] = mk("http://a/")
	}
	if err := checkRedirect(mk("http://b/"), via[:maxRedirects]); err != nil {
		t.Errorf("redirect %d refused: %v", maxRedirects, err)
	}
	if err := checkRedirect(mk("http://b/"), via); err == nil {
		t.Errorf("redirect %d allowed", maxRedirects+1)
	}
}

// A playlist of tens of thousands of entries gets maxEntries requests, each
// after a pause.
func TestPlaylistEntryCap(t *testing.T) {
	oldB, oldM := entryBackoff, entryBackoffMax
	entryBackoff, entryBackoffMax = time.Millisecond, 2*time.Millisecond
	defer func() { entryBackoff, entryBackoffMax = oldB, oldM }()

	var entries atomic.Int32
	a := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/list.pls" {
			w.Header().Set("Content-Type", "audio/x-scpls")
			var b strings.Builder
			b.WriteString("[playlist]\n")
			for i := 1; i <= 5000; i++ {
				fmt.Fprintf(&b, "File%d=/e/%d\n", i, i)
			}
			_, _ = w.Write([]byte(b.String()))
			return
		}
		entries.Add(1)
		http.NotFound(w, r)
	}))
	defer a.Close()
	err := Play(t.Context(), Options{HTTP: a.Client(), UserAgent: "sona-test"}, a.URL+"/list.pls", &recorder{})
	if err == nil {
		t.Error("Play succeeded though no entry plays")
	}
	if n := entries.Load(); n != maxEntries {
		t.Errorf("%d entries requested, want %d", n, maxEntries)
	}
}

// The pause between entries grows and ends with the context.
func TestPlaylistEntryBackoff(t *testing.T) {
	oldB, oldM := entryBackoff, entryBackoffMax
	entryBackoff, entryBackoffMax = 200*time.Millisecond, 200*time.Millisecond
	defer func() { entryBackoff, entryBackoffMax = oldB, oldM }()

	var entries atomic.Int32
	a := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/list.m3u" {
			w.Header().Set("Content-Type", "audio/x-mpegurl")
			_, _ = w.Write([]byte("#EXTM3U\n/e/1\n/e/2\n/e/3\n/e/4\n/e/5\n"))
			return
		}
		entries.Add(1)
		http.NotFound(w, r)
	}))
	defer a.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 350*time.Millisecond)
	defer cancel()
	err := Play(ctx, Options{HTTP: a.Client(), UserAgent: "sona-test"}, a.URL+"/list.m3u", &recorder{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Play = %v, want the context's deadline", err)
	}
	// First at once, the second after 200 ms; the third would be at 400 ms.
	if n := entries.Load(); n != 2 {
		t.Errorf("%d entries requested in 350 ms, want 2", n)
	}
}
