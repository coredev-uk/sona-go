package scrobble

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/playback/coordinator"
)

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func fastBackoff(int) time.Duration { return time.Millisecond }

// play feeds one full listen of a 200 s song starting at t0+i minutes.
func play(s *Scrobbler, i int) {
	it := song(fmt.Sprint(i), sec(200))
	at := t0.Add(time.Duration(i) * time.Minute)
	s.Observe(coordinator.Activity{Kind: coordinator.ActivityStart, Item: it, At: at, Duration: it.Duration})
	s.Observe(coordinator.Activity{Kind: coordinator.ActivityEnd, Item: it, At: at.Add(it.Duration), Position: it.Duration, Duration: it.Duration, Reason: coordinator.EndNatural})
}

func connect(s *Scrobbler, name, cred string) {
	s.SetCredential(name, cred)
	s.SetEnabled(name, true)
}

func TestScrobblerBatchesForLastFM(t *testing.T) {
	c, f := newLastFM(t)
	path := filepath.Join(t.TempDir(), "scrobble-queue.json")
	s := New(Options{Providers: []Provider{c}, QueuePath: path, Backoff: fastBackoff})
	defer s.Close()
	connect(s, "lastfm", "sk-abc")
	for i := range 120 {
		play(s, i)
	}
	if p := s.State("lastfm").Pending; p != 120 {
		t.Fatalf("pending before Start = %d (nothing is sent before the network opens)", p)
	}
	if n := len(f.methodCalls("track.scrobble")); n != 0 {
		t.Fatalf("%d scrobble calls before Start", n)
	}
	s.Start()
	waitFor(t, "delivery", func() bool { return s.State("lastfm").Pending == 0 })
	calls := f.methodCalls("track.scrobble")
	var sizes []int
	next := 0
	for _, c := range calls {
		n := countIndexed(c, "track")
		sizes = append(sizes, n)
		for i := range n {
			if want := fmt.Sprint(t0.Add(time.Duration(next) * time.Minute).Unix()); c.Get(fmt.Sprintf("timestamp[%d]", i)) != want {
				t.Fatalf("listen %d out of order: %v", next, c)
			}
			next++
		}
	}
	if fmt.Sprint(sizes) != "[50 50 20]" {
		t.Fatalf("batches = %v", sizes)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("queue file left after delivery: %v", err)
	}
}

func TestScrobblerRetriesAndPersistsOffline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "scrobble-queue.json")

	// Offline: Last.fm unreachable, ListenBrainz up. Listens wait on disk
	// for Last.fm only.
	down := httptest.NewServer(http.NotFoundHandler())
	down.Close()
	offline := &LastFM{Key: testKey, Secret: testSecret, API: down.URL + "/2.0/"}
	lb, lbf := newListenBrainz(t)
	s := New(Options{Providers: []Provider{offline, lb}, QueuePath: path, Backoff: fastBackoff})
	connect(s, "lastfm", "sk-abc")
	connect(s, "listenbrainz", "good")
	s.Start()
	play(s, 0)
	play(s, 1)
	waitFor(t, "listenbrainz delivery", func() bool { return s.State("listenbrainz").Pending == 0 })
	if got := len(lbf.submissions()); got == 0 {
		t.Fatal("listenbrainz blocked by the unreachable last.fm")
	}
	if p := s.State("lastfm").Pending; p != 2 {
		t.Fatalf("lastfm pending = %d", p)
	}
	s.Close()
	play(s, 2) // the coordinator's final end event arrives after Close
	fi, err := os.Stat(path)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("queue file: %v %v", fi, err)
	}
	b, _ := os.ReadFile(path)
	if strings.Contains(string(b), "sk-abc") || strings.Contains(string(b), "good") || !strings.Contains(string(b), "Title 2") {
		t.Fatalf("queue file = %s", b)
	}

	// Next start: the network is back but Last.fm answers 503 twice first.
	c, f := newLastFM(t)
	f.fail = 2
	s2 := New(Options{Providers: []Provider{c, lb}, QueuePath: path, Backoff: fastBackoff})
	defer s2.Close()
	if p := s2.State("lastfm").Pending; p != 3 {
		t.Fatalf("restored lastfm pending = %d", p)
	}
	connect(s2, "lastfm", "sk-abc")
	connect(s2, "listenbrainz", "good")
	s2.Start()
	waitFor(t, "lastfm delivery after retries", func() bool { return s2.State("lastfm").Pending == 0 })
	waitFor(t, "listenbrainz delivery", func() bool { return s2.State("listenbrainz").Pending == 0 })
	calls := f.methodCalls("track.scrobble")
	if len(calls) != 1 || countIndexed(calls[0], "track") != 3 || calls[0].Get("track[2]") != "Title 2" {
		t.Fatalf("scrobble calls = %v", calls)
	}
	f.mu.Lock()
	retried := f.fail == 0
	f.mu.Unlock()
	if !retried {
		t.Fatal("delivered without going through the 503s")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("queue file left after delivery: %v", err)
	}
}

// fakeProvider records calls and fails Submit with the queued errors.
type fakeProvider struct {
	name string
	mu   sync.Mutex
	errs []error
	now  []Track
	got  [][]Listen
}

func (p *fakeProvider) Name() string  { return p.name }
func (p *fakeProvider) MaxBatch() int { return 10 }
func (p *fakeProvider) NowPlaying(_ context.Context, _ string, t Track) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.now = append(p.now, t)
	return nil
}

func (p *fakeProvider) Submit(_ context.Context, cred string, ls []Listen) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.errs) > 0 {
		err := p.errs[0]
		p.errs = p.errs[1:]
		return err
	}
	if cred != "good" {
		return ErrAuth
	}
	p.got = append(p.got, ls)
	return nil
}

func (p *fakeProvider) delivered() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := 0
	for _, b := range p.got {
		n += len(b)
	}
	return n
}

func TestScrobblerRejectedCredentialsKeepTheQueue(t *testing.T) {
	p := &fakeProvider{name: "lastfm"}
	changes := make(chan struct{}, 100)
	s := New(Options{Providers: []Provider{p}, Backoff: fastBackoff, OnChange: func() { changes <- struct{}{} }})
	defer s.Close()
	connect(s, "lastfm", "revoked")
	s.Start()
	play(s, 0)
	waitFor(t, "rejection", func() bool { return s.State("lastfm").Rejected })
	if st := s.State("lastfm"); st.Pending != 1 {
		t.Fatalf("state = %+v, want the listen kept", st)
	}
	play(s, 1) // still queued while rejected
	s.SetCredential("lastfm", "good")
	waitFor(t, "delivery after reconnect", func() bool { return p.delivered() == 2 })
	if st := s.State("lastfm"); st.Rejected || st.Pending != 0 {
		t.Fatalf("state = %+v", st)
	}
	if len(changes) == 0 {
		t.Fatal("no change notifications")
	}
}

func TestScrobblerDropsRefusedListens(t *testing.T) {
	p := &fakeProvider{name: "lastfm", errs: []error{&LastFMError{Code: 6, Message: "Invalid parameters"}}}
	s := New(Options{Providers: []Provider{p}, Backoff: fastBackoff})
	defer s.Close()
	connect(s, "lastfm", "good")
	play(s, 0)
	s.Start()
	waitFor(t, "drop", func() bool { return s.State("lastfm").Pending == 0 })
	play(s, 1)
	waitFor(t, "next listen", func() bool { return p.delivered() == 1 })
}

func TestScrobblerAccountState(t *testing.T) {
	a, b := &fakeProvider{name: "lastfm"}, &fakeProvider{name: "listenbrainz"}
	path := filepath.Join(t.TempDir(), "q.json")
	s := New(Options{Providers: []Provider{a, b}, QueuePath: path, MaxQueue: 3})
	defer s.Close()
	play(s, 0) // nothing connected: not queued anywhere
	connect(s, "lastfm", "good")
	s.SetCredential("listenbrainz", "good") // connected but paused
	for i := 1; i <= 5; i++ {
		play(s, i)
	}
	if st := s.State("lastfm"); st.Pending != 3 {
		t.Fatalf("lastfm pending = %d, want the cap of 3", st.Pending)
	}
	if st := s.State("listenbrainz"); st.Pending != 0 {
		t.Fatalf("paused listenbrainz queued %d", st.Pending)
	}
	s.Start()
	waitFor(t, "delivery", func() bool { return a.delivered() == 3 })
	a.mu.Lock()
	first := a.got[0][0].Title
	a.mu.Unlock()
	if first != "Title 3" {
		t.Fatalf("oldest kept = %q, want the oldest listens dropped first", first)
	}
	a.mu.Lock()
	now := len(a.now)
	a.mu.Unlock()
	if now == 0 || len(b.now) != 0 {
		t.Fatalf("now playing: lastfm %d, paused listenbrainz %d", now, len(b.now))
	}

	s.SetEnabled("listenbrainz", true)
	s.Close()
	s2 := New(Options{Providers: []Provider{a, b}, QueuePath: path})
	defer s2.Close()
	connect(s2, "listenbrainz", "good")
	play(s2, 6)
	s2.Forget("listenbrainz")
	if st := s2.State("listenbrainz"); st.Pending != 0 {
		t.Fatalf("forgotten account kept %d listens", st.Pending)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("queue file kept after forgetting the only queue: %v", err)
	}
}
