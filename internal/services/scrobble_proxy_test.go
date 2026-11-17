package services

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/addons/own"
	"github.com/coredev-uk/sona-go/internal/cli"
	"github.com/coredev-uk/sona-go/internal/playback/coordinator"
	"github.com/coredev-uk/sona-go/internal/scrobble"
)

// pipeAddons runs Sona's own add-ons in the test's process, each the real
// client (internal/addons/own) behind a real cli.Session marked bundled, over
// a pipe pair: the proxy is tested against the add-on as Sona speaks to it,
// without a child process.
type pipeAddons struct {
	t    *testing.T
	opts own.Options

	mu       sync.Mutex
	sessions map[string]*cli.Session
	requests []sentRequest
	leased   map[string]int // leases taken, by add-on ID
	held     map[string]int // leases held now
	down     bool           // no add-on is running
}

type sentRequest struct {
	ID, Method string
	Params     json.RawMessage
}

type nopHost struct{}

func (nopHost) Call(context.Context, cli.Request) (any, error) {
	return nil, &cli.Error{Code: "failed"}
}

func (nopHost) Watch(string) (<-chan any, func(), error) {
	return nil, nil, &cli.Error{Code: cli.AddonErrUnknownTopic}
}

type pipeConn struct {
	r *io.PipeReader
	w *io.PipeWriter
}

func (c pipeConn) Read(b []byte) (int, error)  { return c.r.Read(b) }
func (c pipeConn) Write(b []byte) (int, error) { return c.w.Write(b) }
func (c pipeConn) Close() error                { _ = c.r.Close(); return c.w.Close() }

func newPipeAddons(t *testing.T, opts own.Options) *pipeAddons {
	t.Helper()
	return &pipeAddons{t: t, opts: opts, sessions: map[string]*cli.Session{}, leased: map[string]int{}, held: map[string]int{}}
}

// up starts add-on id once and returns its session.
func (p *pipeAddons) up(id string) (*cli.Session, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.down {
		return nil, ErrAddonUnavailable
	}
	if s := p.sessions[id]; s != nil {
		return s, nil
	}
	toSona, fromAddon := io.Pipe()
	toAddon, fromSona := io.Pipe()
	sess := cli.NewSession(cli.AddonConfig{ID: id, Host: nopHost{}, Bundled: true, Allowed: func(string) bool { return true }})
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan struct{})
	go func() { _ = sess.Serve(context.Background(), pipeConn{toSona, fromSona}); close(served) }()
	ran := make(chan struct{})
	go func() { _ = own.Run(ctx, id, toAddon, fromAddon, nil, p.opts); close(ran) }()
	p.t.Cleanup(func() {
		sess.Close()
		cancel()
		_ = fromAddon.Close()
		_ = fromSona.Close()
		<-served
		<-ran
	})
	select {
	case <-sess.Helloed():
	case <-time.After(10 * time.Second):
		p.t.Fatalf("add-on %s didn't say hello", id)
	}
	p.sessions[id] = sess
	return sess, nil
}

// Request implements addonLink.
func (p *pipeAddons) Request(ctx context.Context, id, method string, params, out any) error {
	raw, _ := json.Marshal(params)
	p.mu.Lock()
	p.requests = append(p.requests, sentRequest{id, method, raw})
	p.mu.Unlock()
	sess, err := p.up(id)
	if err != nil {
		return err
	}
	res, err := sess.Request(ctx, method, params)
	if err != nil {
		return err
	}
	if out != nil {
		return json.Unmarshal(res, out)
	}
	return nil
}

// Lease implements addonLink.
func (p *pipeAddons) Lease(id string) func() {
	p.mu.Lock()
	p.leased[id]++
	p.held[id]++
	p.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			p.mu.Lock()
			p.held[id]--
			p.mu.Unlock()
		})
	}
}

func (p *pipeAddons) sent(method string) []sentRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []sentRequest
	for _, r := range p.requests {
		if r.Method == method {
			out = append(out, r)
		}
	}
	return out
}

func (p *pipeAddons) setDown(on bool) {
	p.mu.Lock()
	p.down = on
	p.mu.Unlock()
}

// playOnce plays one whole 200 s song of the given ID through the scrobbler.
func playOnce(s *scrobble.Scrobbler, id string, at time.Time) {
	it := coordinator.Item{ID: id, Title: "Title " + id, Artist: "Artist", Album: "Album", Duration: 200 * time.Second}
	s.Observe(coordinator.Activity{Kind: coordinator.ActivityStart, Item: it, At: at, Duration: it.Duration})
	s.Observe(coordinator.Activity{
		Kind: coordinator.ActivityEnd, Item: it, At: at.Add(it.Duration), Position: it.Duration, Duration: it.Duration,
		Reason: coordinator.EndNatural,
	})
}

// connected is a service with a ListenBrainz account whose delivery runs.
func (se *scrobbleEnv) connected(t *testing.T) {
	t.Helper()
	if _, err := se.s.ConnectListenBrainz(t.Context(), "good"); err != nil {
		t.Fatal(err)
	}
	se.s.sc.Start()
}

func (se *scrobbleEnv) submissions() []string {
	se.mu.Lock()
	defer se.mu.Unlock()
	return slices.Clone(se.lbBodies)
}

// A listen that passed the filter reaches ListenBrainz through its add-on:
// the now-playing and the submit are requests of Sona to it, with the
// account's token, and the service queues and delivers as before.
func TestScrobbleListenBrainzIsDeliveredByTheAddon(t *testing.T) {
	se := newScrobbleEnv(t)
	se.connected(t)
	playOnce(se.s.sc, "1", time.Now().Add(-5*time.Minute))
	awaitScrobble(t, "the listen to reach ListenBrainz", func() bool { return se.s.sc.State(ScrobblerListenBrainz).Pending == 0 && len(se.submissions()) >= 2 })
	var now, submit cli.ScrobbleNowPlayingParams
	if rs := se.link.sent(cli.AddonMethodScrobbleNowPlaying); len(rs) != 1 || rs[0].ID != "listenbrainz" {
		t.Fatalf("now-playing requests = %+v", rs)
	} else {
		_ = json.Unmarshal(rs[0].Params, &now)
	}
	if rs := se.link.sent(cli.AddonMethodScrobbleSubmit); len(rs) != 1 {
		t.Fatalf("submit requests = %+v", rs)
	} else {
		_ = json.Unmarshal(rs[0].Params, &submit)
	}
	if now.Credential != "good" || now.Track.Artist != "Artist" || submit.Credential != "good" {
		t.Errorf("params = %+v, %+v", now, submit)
	}
	bodies := se.submissions()
	if len(bodies) != 2 {
		t.Fatalf("the service got %d submissions, want a now-playing and a listen: %v", len(bodies), bodies)
	}
}

// A listen hidden by private mode never becomes a request of Sona to the
// add-on, nor reaches the queue or the service; the next public one does.
func TestScrobbleListenBrainzNeverSeesAPrivateListen(t *testing.T) {
	se := newScrobbleEnv(t)
	se.connected(t)
	if _, err := se.private.Set(true); err != nil {
		t.Fatal(err)
	}
	playOnce(se.s.sc, "private", time.Now())
	if n := se.s.sc.State(ScrobblerListenBrainz).Pending; n != 0 {
		t.Fatalf("%d private listens queued", n)
	}
	time.Sleep(100 * time.Millisecond)
	for _, m := range []string{cli.AddonMethodScrobbleNowPlaying, cli.AddonMethodScrobbleSubmit} {
		if rs := se.link.sent(m); len(rs) != 0 {
			t.Errorf("a private listen reached the add-on as %s: %+v", m, rs)
		}
	}
	if b := se.submissions(); len(b) != 0 {
		t.Errorf("the service got %v", b)
	}
	if _, err := se.private.Set(false); err != nil {
		t.Fatal(err)
	}
	// It starts after private mode went off: a listen that began while it was
	// on stays private (privatemode.Mode.Hidden).
	playOnce(se.s.sc, "public", time.Now())
	awaitScrobble(t, "the public listen to be submitted", func() bool { return len(se.link.sent(cli.AddonMethodScrobbleSubmit)) == 1 })
}

// The add-on's codes act on the queue as the scrobble errors did: refused
// credentials stop delivery and keep the queue (the account needs
// reconnecting), a refused submission is dropped, and anything else is
// retried.
func TestScrobbleListenBrainzCodesDriveTheQueue(t *testing.T) {
	for name, tc := range map[string]struct {
		status       int
		wantRejected bool
		wantPending  int
		// refused credentials stop the announcement that goes first, so the
		// queue is never sent
		wantSubmit bool
	}{
		"credentials refused":  {http.StatusUnauthorized, true, 1, false},
		"submission refused":   {http.StatusBadRequest, false, 0, true},
		"service unavailable":  {http.StatusInternalServerError, false, 1, true},
		"throttled, try again": {http.StatusTooManyRequests, false, 1, true},
	} {
		t.Run(name, func(t *testing.T) {
			se := newScrobbleEnv(t)
			se.lbStatus(tc.status)
			se.connected(t)
			playOnce(se.s.sc, "1", time.Now().Add(-5*time.Minute))
			if tc.wantSubmit {
				awaitScrobble(t, "the submission to be attempted", func() bool { return len(se.link.sent(cli.AddonMethodScrobbleSubmit)) >= 1 })
			}
			awaitScrobble(t, "the queue to settle", func() bool {
				st := se.s.Status().ListenBrainz
				return st.NeedsReconnect == tc.wantRejected && st.Pending == tc.wantPending
			})
		})
	}
}

// An add-on that isn't running is a failure to retry: the listens stay
// queued and nothing is lost or marked as refused.
func TestScrobbleListenBrainzWaitsForTheAddon(t *testing.T) {
	se := newScrobbleEnv(t)
	se.connected(t)
	se.link.setDown(true)
	playOnce(se.s.sc, "1", time.Now().Add(-5*time.Minute))
	awaitScrobble(t, "the submission to be attempted", func() bool { return len(se.link.sent(cli.AddonMethodScrobbleSubmit)) >= 1 })
	if st := se.s.Status().ListenBrainz; st.NeedsReconnect || st.Pending != 1 {
		t.Fatalf("status = %+v", st)
	}
}

// Connecting holds the add-on up while it checks the token, and lets it go
// when the account is saved; a token it refuses leaves nothing behind.
func TestScrobbleConnectListenBrainzHoldsTheAddonUp(t *testing.T) {
	se := newScrobbleEnv(t)
	if _, err := se.s.ConnectListenBrainz(t.Context(), "wrong"); !errors.Is(err, scrobble.ErrAuth) {
		t.Fatalf("a refused token: %v", err)
	}
	if se.link.held["listenbrainz"] != 0 || se.link.leased["listenbrainz"] != 1 {
		t.Errorf("after a refusal: held %d, taken %d", se.link.held["listenbrainz"], se.link.leased["listenbrainz"])
	}
	if _, err := se.s.ConnectListenBrainz(t.Context(), "good"); err != nil {
		t.Fatal(err)
	}
	if se.link.held["listenbrainz"] != 0 || se.link.leased["listenbrainz"] != 2 {
		t.Errorf("after a connection: held %d, taken %d", se.link.held["listenbrainz"], se.link.leased["listenbrainz"])
	}
	// Checking the token is a scrobble.status request carrying it.
	rs := se.link.sent(cli.AddonMethodScrobbleStatus)
	if len(rs) != 2 {
		t.Fatalf("status requests = %+v", rs)
	}
}

// A ListenBrainz the add-on can't reach is a network failure to the user, as
// it was when the check ran in Sona; and the lease is let go.
func TestScrobbleConnectListenBrainzWithoutTheNetwork(t *testing.T) {
	se := newScrobbleEnv(t)
	se.srv.Close()
	_, err := se.s.ConnectListenBrainz(t.Context(), "good")
	if CodeOf(err) != CodeNetwork {
		t.Fatalf("err = %v (code %s), want a network failure", err, CodeOf(err))
	}
	if se.link.held["listenbrainz"] != 0 {
		t.Error("the lease was not released")
	}
}

// awaitScrobble waits until ok holds, failing after 15 s (waitUntil is the
// supervisor tests', which only build on Unix).
func awaitScrobble(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(15 * time.Second); !ok(); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}
