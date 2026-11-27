package services

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/update"
)

// githubRig is an UpdatesService (where Sona doesn't update itself) against a
// fake GitHub whose API answers with its limit used up while limited is set,
// and with a release otherwise. apiHits counts the API's requests.
type githubRig struct {
	s       *UpdatesService
	e       *Env
	rec     *recorder
	limited atomic.Bool
	reset   atomic.Int64 // the x-ratelimit-reset the limit names, Unix seconds
	apiHits atomic.Int32

	mu        sync.Mutex
	scheduled []scheduled
}

// scheduled is a retry the service asked to run after d.
type scheduled struct {
	d time.Duration
	f func()
}

func newGitHubRig(t *testing.T, version, latest string, check bool) *githubRig {
	t.Helper()
	g := &githubRig{}
	g.reset.Store(time.Now().Add(20 * time.Minute).Unix())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.apiHits.Add(1)
		if g.limited.Load() {
			w.Header().Set("X-RateLimit-Limit", "60")
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(g.reset.Load(), 10))
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"API rate limit exceeded"}`))
			return
		}
		_, _ = w.Write([]byte(`{"tag_name":"v` + latest + `","body":"### Fixes\n\n- a"}`))
	}))
	t.Cleanup(srv.Close)
	st, err := config.OpenStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Update(func(s *config.Settings) { s.Updates.Check = check }); err != nil {
		t.Fatal(err)
	}
	g.rec = &recorder{}
	g.e = &Env{Deps: Deps{Settings: st, Gate: NewGate(time.Hour, nil), Emit: g.rec, Logger: discard(), Now: time.Now, Version: version}}
	g.s = newUpdatesService(g.e)
	g.s.checker = update.Checker{API: srv.URL}
	g.s.schedule = func(d time.Duration, f func()) func() {
		g.mu.Lock()
		defer g.mu.Unlock()
		g.scheduled = append(g.scheduled, scheduled{d, f})
		return func() {}
	}
	t.Cleanup(g.e.close)
	return g
}

func (g *githubRig) retries() []scheduled {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]scheduled(nil), g.scheduled...)
}

// over ends the limit as the clock reaching its reset would.
func (g *githubRig) over() {
	g.limited.Store(false)
	g.s.mu.Lock()
	g.s.limitUntil = time.Now().Add(-time.Second)
	g.s.mu.Unlock()
}

// A limit reached is said as such, with when it is over, and nothing asks
// GitHub again until then: not Check now, the start-up check or What's New.
func TestUpdateRateLimitIsSaidAndHoldsEveryCall(t *testing.T) {
	g := newGitHubRig(t, "0.1.0", "0.2.0", true)
	g.limited.Store(true)
	reset := time.Unix(g.reset.Load(), 0)

	st, err := g.s.Check(context.Background())
	var rl *update.RateLimitError
	if !errors.As(err, &rl) || !rl.Reset.Equal(reset) {
		t.Fatalf("Check err = %v, want the limit until %v", err, reset)
	}
	if CodeOf(err) != CodeRateLimited || st.Error != CodeRateLimited || st.RetryAt != reset.UnixMilli() || st.Available || st.Checked != 0 {
		t.Fatalf("status = %+v (code %q)", st, CodeOf(err))
	}
	if g.apiHits.Load() != 1 {
		t.Fatalf("API hits = %d", g.apiHits.Load())
	}

	// Check now again, the start-up check and What's New: held.
	st, err = g.s.Check(context.Background())
	if !errors.As(err, &rl) || !rl.Reset.Equal(reset) || st.RetryAt != reset.UnixMilli() {
		t.Fatalf("a second Check: %+v, %v", st, err)
	}
	g.e.Gate.Open()
	time.Sleep(100 * time.Millisecond)
	if _, err := g.s.notesOf(context.Background(), "0.1.0"); !errors.As(err, &rl) {
		t.Fatalf("What's New err = %v, want the limit", err)
	}
	if _, err := g.s.CurrentNotes(context.Background()); !errors.As(err, &rl) {
		t.Fatalf("the current notes' err = %v, want the limit", err)
	}
	if n := g.apiHits.Load(); n != 1 {
		t.Fatalf("GitHub was asked %d times while its limit held, want the one that found it", n)
	}
	g.rec.mu.Lock()
	events := g.rec.events
	g.rec.mu.Unlock()
	if len(events) != 0 {
		t.Fatalf("events = %v", events)
	}

	// The diagnostics say it.
	if b, _ := g.s.bundle().(map[string]any); b["rateLimitedUntil"] == nil {
		t.Fatalf("diagnostics = %v", b)
	}

	// After the reset a check goes through and clears the limit.
	g.over()
	st, err = g.s.Check(context.Background())
	if err != nil || !st.Available || st.Latest != "0.2.0" || st.Error != "" || st.RetryAt != 0 || st.Checked == 0 {
		t.Fatalf("Check after the reset = %+v, %v", st, err)
	}
	if n := g.apiHits.Load(); n != 2 {
		t.Fatalf("API hits = %d", n)
	}
}

// The service checks again at the reset, once, and tells the window when
// that finds an update.
func TestUpdateChecksAgainWhenTheLimitIsOver(t *testing.T) {
	g := newGitHubRig(t, "0.1.0", "0.2.0", true)
	g.limited.Store(true)
	if _, err := g.s.Check(context.Background()); err == nil {
		t.Fatal("no limit")
	}
	// A What's New fetch the limit held back is tried again then too.
	g.s.fetchWhatsNew(context.Background(), "0.1.0")

	retries := g.retries()
	if len(retries) != 1 {
		t.Fatalf("scheduled %d retries, want the one the first answer made: What's New, held, asks nothing", len(retries))
	}
	if d := retries[0].d; d < 19*time.Minute || d > 21*time.Minute+5*time.Second {
		t.Fatalf("retry after %v, want the 20 minutes to the reset (and a second)", d)
	}

	g.over()
	retries[0].f()
	waitFor(t, func() bool {
		g.rec.mu.Lock()
		defer g.rec.mu.Unlock()
		return len(g.rec.events) >= 2
	})
	g.rec.mu.Lock()
	defer g.rec.mu.Unlock()
	if g.rec.events[0] != EventUpdateAvailable || g.rec.events[1] != EventWhatsNew {
		t.Fatalf("events = %v", g.rec.events)
	}
	if st := g.s.Status(); !st.Available || st.Latest != "0.2.0" {
		t.Fatalf("status = %+v", st)
	}
}

// With the start-up check off, the limit's end checks nothing by itself.
func TestUpdateDoesNotCheckAfterTheLimitWhenTheCheckIsOff(t *testing.T) {
	g := newGitHubRig(t, "0.1.0", "0.2.0", false)
	g.limited.Store(true)
	if _, err := g.s.Check(context.Background()); err == nil {
		t.Fatal("no limit")
	}
	retries := g.retries()
	if len(retries) != 1 {
		t.Fatalf("retries = %d", len(retries))
	}
	g.over()
	retries[0].f()
	time.Sleep(100 * time.Millisecond)
	if n := g.apiHits.Load(); n != 1 {
		t.Fatalf("API hits = %d, want only the first", n)
	}
}

// The hold is never shorter than a minute nor longer than the hour GitHub's
// window lasts: a reset already past, or one a day away, is not taken at its
// word.
func TestUpdateLimitHoldIsBounded(t *testing.T) {
	for _, tc := range []struct {
		name   string
		reset  time.Duration
		lo, hi time.Duration
	}{
		{"already past", -time.Minute, 59 * time.Second, 61 * time.Second},
		{"a day away", 24 * time.Hour, 59 * time.Minute, 61 * time.Minute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := newGitHubRig(t, "0.1.0", "0.2.0", true)
			g.limited.Store(true)
			g.reset.Store(time.Now().Add(tc.reset).Unix())
			st, err := g.s.Check(context.Background())
			var rl *update.RateLimitError
			if !errors.As(err, &rl) {
				t.Fatalf("err = %v", err)
			}
			held := time.Until(rl.Reset)
			if held < tc.lo || held > tc.hi || st.RetryAt != rl.Reset.UnixMilli() {
				t.Fatalf("held for %v, status RetryAt %d, want %v..%v", held, st.RetryAt, tc.lo, tc.hi)
			}
		})
	}
}

// Linux's check and What's New read the signed release.json, not the API.
func TestUpdateReadsReleaseInfoWithoutTheAPI(t *testing.T) {
	g := newGitHubRig(t, "0.1.0", "0.9.9", false)
	pub, priv := signingKey(t)
	var web atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		web.Add(1)
		var version, notes string
		switch r.URL.Path {
		case "/coredev-uk/sona-go/releases/latest/download/release.json":
			version, notes = "0.2.0", "### Fixes\n\n- b"
		case "/coredev-uk/sona-go/releases/download/v0.1.0/release.json":
			version, notes = "0.1.0", "### First"
		default:
			http.NotFound(w, r)
			return
		}
		info, err := update.NewReleaseInfo(priv, version, time.Date(2026, 11, 27, 18, 0, 0, 0, time.UTC), notes)
		if err != nil {
			t.Error(err)
		}
		_ = writeJSON(w, info)
	}))
	t.Cleanup(srv.Close)
	g.s.checker = update.Checker{API: "http://127.0.0.1:1", Web: srv.URL, Key: pub}

	st, err := g.s.Check(context.Background())
	if err != nil || !st.Available || st.Latest != "0.2.0" || st.Notes != "### Fixes\n\n- b" || st.URL != "https://github.com/coredev-uk/sona-go/releases/tag/v0.2.0" {
		t.Fatalf("Check = %+v, %v", st, err)
	}
	w, err := g.s.CurrentNotes(context.Background())
	if err != nil || w.Version != "0.1.0" || w.Notes != "### First" {
		t.Fatalf("CurrentNotes = %+v, %v", w, err)
	}
	if g.apiHits.Load() != 0 || web.Load() != 2 {
		t.Fatalf("API hits %d, release.json hits %d", g.apiHits.Load(), web.Load())
	}

	// A release.json signed by another key fails the check; it is not
	// bypassed through the API.
	other, _ := signingKey(t)
	g.s.checker.Key = other
	if _, err := g.s.Check(context.Background()); !errors.Is(err, update.ErrSignature) || CodeOf(err) != CodeUpdateSignature {
		t.Fatalf("err = %v, want ErrSignature", err)
	}
}

// signingKey is a key pair: the public key as the build embeds it, base64,
// and the private key that signs.
func signingKey(t *testing.T) (string, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(pub), priv
}

func writeJSON(w http.ResponseWriter, v any) error { return json.NewEncoder(w).Encode(v) }
