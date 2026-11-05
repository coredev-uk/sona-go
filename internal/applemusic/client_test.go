package applemusic

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/auth/devtoken"
)

// fakeDev hands out "dev-<n>" tokens; Invalidate bumps n.
type fakeDev struct {
	n           atomic.Int32
	invalidated atomic.Int32
}

func (f *fakeDev) Token(context.Context) (devtoken.Token, error) {
	return devtoken.Token{JWT: "dev-" + string(rune('0'+f.n.Load())), Expires: time.Now().Add(time.Hour)}, nil
}

func (f *fakeDev) Invalidate() { f.invalidated.Add(1); f.n.Add(1) }

// newTestClient returns a client for srv whose retry waits are recorded
// instead of slept.
func newTestClient(t *testing.T, h http.Handler) (*Client, *fakeDev, *[]time.Duration) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	dev := &fakeDev{}
	c := New(srv.Client(), dev, func() string { return "mut" })
	c.Base = srv.URL
	var mu sync.Mutex
	waits := &[]time.Duration{}
	c.Sleep = func(_ context.Context, d time.Duration) error {
		mu.Lock()
		*waits = append(*waits, d)
		mu.Unlock()
		return nil
	}
	return c, dev, waits
}

func TestDoSendsHeaders(t *testing.T) {
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer dev-0" || r.Header.Get("Media-User-Token") != "mut" ||
			r.Header.Get("Origin") != devtoken.WebPlayerOrigin {
			t.Errorf("headers = %v", r.Header)
		}
		w.Write([]byte(`{"data":[{"id":"gb","type":"storefronts","attributes":{"name":"United Kingdom"}}]}`))
	}))
	sf, err := c.Storefront(context.Background())
	if err != nil || sf.ID != "gb" || sf.Attributes.Name != "United Kingdom" {
		t.Fatalf("Storefront = %+v, %v", sf, err)
	}
}

func TestDo401RefreshesDevTokenOnce(t *testing.T) {
	var hits atomic.Int32
	c, dev, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Header.Get("Authorization") == "Bearer dev-0" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	var out struct{ OK bool }
	if err := c.Get(context.Background(), "/v1/x", nil, &out); err != nil || !out.OK {
		t.Fatalf("Get = %+v, %v", out, err)
	}
	if hits.Load() != 2 || dev.invalidated.Load() != 1 {
		t.Fatalf("hits=%d invalidated=%d, want 2 and 1", hits.Load(), dev.invalidated.Load())
	}
}

func TestDo401Twice(t *testing.T) {
	var hits atomic.Int32
	c, dev, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	err := c.Get(context.Background(), "/v1/x", nil, nil)
	var se *StatusError
	if !errors.Is(err, ErrUnauthorized) || !errors.As(err, &se) || se.Code != 401 {
		t.Fatalf("err = %v", err)
	}
	if hits.Load() != 2 || dev.invalidated.Load() != 1 {
		t.Fatalf("hits=%d invalidated=%d, want 2 and 1", hits.Load(), dev.invalidated.Load())
	}
}

func TestDo429HonoursRetryAfter(t *testing.T) {
	var hits atomic.Int32
	c, _, waits := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) <= 2 {
			w.Header().Set("Retry-After", "3")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write([]byte(`{}`))
	}))
	if err := c.Get(context.Background(), "/v1/x", nil, nil); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 3 || len(*waits) != 2 {
		t.Fatalf("hits=%d waits=%v", hits.Load(), *waits)
	}
	for _, w := range *waits {
		if w < 3*time.Second {
			t.Errorf("wait %v shorter than Retry-After", w)
		}
	}
}

func TestDo429GivesUp(t *testing.T) {
	var hits atomic.Int32
	c, _, waits := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	err := c.Get(context.Background(), "/v1/x", nil, nil)
	var se *StatusError
	if !errors.As(err, &se) || se.Code != http.StatusTooManyRequests {
		t.Fatalf("err = %v", err)
	}
	if hits.Load() != maxRetries+1 || len(*waits) != maxRetries {
		t.Fatalf("hits=%d waits=%d", hits.Load(), len(*waits))
	}
	// Exponential: each wait at least the previous one's nominal base.
	for i, w := range *waits {
		if lo := retryBase << i * 3 / 4; w < lo {
			t.Errorf("wait %d = %v, want >= %v", i, w, lo)
		}
	}
}

func TestDo5xxRetriesOnlyIdempotent(t *testing.T) {
	var hits atomic.Int32
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	c.Do(context.Background(), Request{Method: http.MethodPost, URL: "/v1/x"}, nil)
	if hits.Load() != 1 {
		t.Fatalf("POST hits = %d, want 1", hits.Load())
	}
	hits.Store(0)
	c.Get(context.Background(), "/v1/x", nil, nil)
	if hits.Load() != maxRetries+1 {
		t.Fatalf("GET hits = %d, want %d", hits.Load(), maxRetries+1)
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	for v, want := range map[string]time.Duration{
		"":                              0,
		"7":                             7 * time.Second,
		"junk":                          0,
		"Thu, 29 Oct 2026 12:00:10 GMT": 10 * time.Second,
		"Thu, 29 Oct 2026 11:00:00 GMT": 0,
	} {
		if got := parseRetryAfter(v, now); got != want {
			t.Errorf("parseRetryAfter(%q) = %v, want %v", v, got, want)
		}
	}
}

func TestSingleflightDedupesConcurrentGets(t *testing.T) {
	var hits atomic.Int32
	release := make(chan struct{})
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		<-release
		w.Write([]byte(`{"data":[{"id":"1"}]}`))
	}))
	const n = 10
	var wg sync.WaitGroup
	outs := make([]Page[Resource[struct{}]], n)
	errs := make([]error, n)
	for i := range n {
		wg.Go(func() { errs[i] = c.Get(context.Background(), "/v1/same", nil, &outs[i]) })
	}
	// Let every caller join the in-flight request before it completes.
	deadline := time.Now().Add(2 * time.Second)
	for hits.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	if hits.Load() != 1 {
		t.Fatalf("server hits = %d, want 1", hits.Load())
	}
	for i := range n {
		if errs[i] != nil || len(outs[i].Data) != 1 || outs[i].Data[0].ID != "1" {
			t.Fatalf("caller %d: %+v, %v", i, outs[i], errs[i])
		}
	}
	// Different URLs are not merged.
	c.Get(context.Background(), "/v1/other", nil, nil)
	if hits.Load() != 2 {
		t.Fatalf("hits = %d after distinct URL", hits.Load())
	}
}

func TestSingleflightCancelledLeader(t *testing.T) {
	var hits atomic.Int32
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 {
			<-r.Context().Done() // the leader is cancelled mid-request
			return
		}
		w.Write([]byte(`{"v":1}`))
	}))
	leaderCtx, cancel := context.WithCancel(context.Background())
	leaderDone := make(chan error, 1)
	go func() { leaderDone <- c.Get(leaderCtx, "/v1/x", nil, nil) }()
	for hits.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	followerDone := make(chan error, 1)
	var out struct{ V int }
	go func() { followerDone <- c.Get(context.Background(), "/v1/x", nil, &out) }()
	time.Sleep(20 * time.Millisecond)
	cancel()
	if err := <-leaderDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("leader err = %v", err)
	}
	if err := <-followerDone; err != nil || out.V != 1 {
		t.Fatalf("follower = %+v, %v", out, err)
	}
}

func TestStatusErrorIs(t *testing.T) {
	for code, want := range map[int][2]bool{401: {true, false}, 403: {true, false}, 404: {false, true}, 500: {false, false}} {
		err := error(&StatusError{Code: code})
		if errors.Is(err, ErrUnauthorized) != want[0] || errors.Is(err, ErrNotFound) != want[1] {
			t.Errorf("code %d: unauthorized=%v notfound=%v", code, errors.Is(err, ErrUnauthorized), errors.Is(err, ErrNotFound))
		}
	}
}

func TestDoDecodesEmptyBody(t *testing.T) {
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	var out map[string]any
	if err := c.Do(context.Background(), Request{Method: http.MethodPut, URL: "/v1/x", Body: json.RawMessage(`{}`)}, &out); err != nil {
		t.Fatal(err)
	}
}
