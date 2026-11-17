package own

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/cli"
)

// bundledHost is newHost for one of Sona's own add-ons: the session is marked
// bundled, and the client has the answers register gives its ID.
func bundledHost(t *testing.T, id string, opts Options) *host {
	t.Helper()
	toSona, fromAddon := io.Pipe()
	toAddon, fromSona := io.Pipe()
	sess := cli.NewSession(cli.AddonConfig{
		ID: id, Host: fakeHost{}, Bundled: true, Allowed: func(string) bool { return true },
	})
	h := &host{sess: sess, cli: NewClient(id, toAddon, fromAddon, nil), done: make(chan error, 1), sonaOut: fromSona}
	register(h.cli, id, opts)
	go func() { h.done <- sess.Serve(context.Background(), rwc{toSona, fromSona}) }()
	out := serve(h, nil, nil)
	// Cleanups run last in, first out: the session closes, then the client is
	// waited for.
	t.Cleanup(func() { _ = wait(t, out, "Serve") })
	t.Cleanup(func() {
		sess.Close()
		_ = fromAddon.Close()
		_ = fromSona.Close()
	})
	settled(t, sess)
	return h
}

// fakeListenBrainz records what reaches it and answers the token "good".
type fakeListenBrainz struct {
	mu     sync.Mutex
	bodies []string
	auth   []string
	status int // of submit-listens; 0 is 200
}

func (f *fakeListenBrainz) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.auth = append(f.auth, r.Header.Get("Authorization"))
	switch r.URL.Path {
	case "/1/validate-token":
		if r.Header.Get("Authorization") == "Token good" {
			_, _ = io.WriteString(w, `{"valid":true,"user_name":"lb-user"}`)
			return
		}
		_, _ = io.WriteString(w, `{"valid":false}`)
	case "/1/submit-listens":
		f.bodies = append(f.bodies, string(b))
		if f.status != 0 {
			w.WriteHeader(f.status)
			_, _ = io.WriteString(w, `{"error":"no"}`)
			return
		}
		_, _ = io.WriteString(w, `{"status":"ok"}`)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func lbHost(t *testing.T) (*host, *fakeListenBrainz) {
	t.Helper()
	f := &fakeListenBrainz{}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return bundledHost(t, "listenbrainz", Options{Version: "1.2.3", HTTP: srv.Client(), Endpoints: Endpoints{ListenBrainz: srv.URL}}), f
}

func request(h *host, method string, params any) (cli.ScrobbleResult, error) {
	raw, err := h.sess.Request(context.Background(), method, params)
	var res cli.ScrobbleResult
	if err == nil {
		err = json.Unmarshal(raw, &res)
	}
	return res, err
}

func code(err error) string {
	var ce *cli.Error
	if errors.As(err, &ce) {
		return ce.Code
	}
	return ""
}

// The add-on does the HTTP of a now-playing, a single listen and a batch, as
// the user's token, and says it is done.
func TestListenBrainzAddonSendsWhatSonaAsks(t *testing.T) {
	h, f := lbHost(t)
	track := cli.ScrobbleTrack{Artist: "Artist", Title: "Song", Album: "Album", DurationMs: 215000}
	if res, err := request(h, cli.AddonMethodScrobbleNowPlaying, cli.ScrobbleNowPlayingParams{Credential: "good", Track: track}); err != nil || !res.OK {
		t.Fatalf("now-playing = %+v, %v", res, err)
	}
	one := cli.ScrobbleSubmitParams{Credential: "good", Listens: []cli.ScrobbleListen{{ScrobbleTrack: track, ListenedAt: 1700000000}}}
	if _, err := request(h, cli.AddonMethodScrobbleSubmit, one); err != nil {
		t.Fatalf("submit one: %v", err)
	}
	two := cli.ScrobbleSubmitParams{Credential: "good", Listens: append(one.Listens, cli.ScrobbleListen{ScrobbleTrack: track, ListenedAt: 1700000300})}
	if _, err := request(h, cli.AddonMethodScrobbleSubmit, two); err != nil {
		t.Fatalf("submit two: %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.bodies) != 3 {
		t.Fatalf("%d submissions, want 3: %v", len(f.bodies), f.bodies)
	}
	for i, want := range []string{`"listen_type":"playing_now"`, `"listen_type":"single"`, `"listen_type":"import"`} {
		if !strings.Contains(f.bodies[i], want) {
			t.Errorf("body %d = %s, want %s", i, f.bodies[i], want)
		}
	}
	if !strings.Contains(f.bodies[1], `"listened_at":1700000000`) || !strings.Contains(f.bodies[1], `"duration_ms":215000`) ||
		!strings.Contains(f.bodies[1], `"submission_client_version":"1.2.3"`) || !strings.Contains(f.bodies[1], `"artist_name":"Artist"`) {
		t.Errorf("listen body = %s", f.bodies[1])
	}
	for _, a := range f.auth {
		if a != "Token good" {
			t.Errorf("Authorization = %q", a)
		}
	}
}

// What the service answered comes back as the code the host maps to the
// scrobble errors; the rest is "failed", whose text stays in the add-on's log.
func TestListenBrainzAddonAnswersInCodes(t *testing.T) {
	h, f := lbHost(t)
	listen := cli.ScrobbleSubmitParams{Credential: "good", Listens: []cli.ScrobbleListen{{ScrobbleTrack: cli.ScrobbleTrack{Artist: "A", Title: "T"}, ListenedAt: 1}}}
	for status, want := range map[int]string{
		http.StatusUnauthorized:        cli.AddonErrCredentialsRejected,
		http.StatusBadRequest:          cli.AddonErrSubmissionRejected,
		http.StatusTooManyRequests:     cli.AddonErrFailed,
		http.StatusInternalServerError: cli.AddonErrFailed,
	} {
		f.mu.Lock()
		f.status = status
		f.mu.Unlock()
		_, err := request(h, cli.AddonMethodScrobbleSubmit, listen)
		if got := code(err); got != want {
			t.Errorf("HTTP %d: code %q, want %q", status, got, want)
		}
	}
}

// A service the add-on can't reach is the code the host words as a network
// failure, not "failed".
func TestListenBrainzAddonSaysWhenTheServiceIsUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	h := bundledHost(t, "listenbrainz", Options{Version: "1", Endpoints: Endpoints{ListenBrainz: url}})
	if _, err := request(h, cli.AddonMethodScrobbleStatus, cli.ScrobbleStatusParams{Credential: "good"}); code(err) != cli.AddonErrNetwork {
		t.Errorf("an unreachable service: %v", err)
	}
}

// scrobble.status says whose a credential is, refuses one the service
// refuses, and without a credential only that the add-on is usable.
func TestListenBrainzAddonStatus(t *testing.T) {
	h, _ := lbHost(t)
	res, err := request(h, cli.AddonMethodScrobbleStatus, cli.ScrobbleStatusParams{Credential: "good"})
	if err != nil || res.Account != "lb-user" {
		t.Fatalf("status = %+v, %v", res, err)
	}
	if _, err := request(h, cli.AddonMethodScrobbleStatus, cli.ScrobbleStatusParams{Credential: "wrong"}); code(err) != cli.AddonErrCredentialsRejected {
		t.Errorf("a refused token: %v", err)
	}
	if res, err := request(h, cli.AddonMethodScrobbleStatus, cli.ScrobbleStatusParams{}); err != nil || !res.OK || res.Account != "" {
		t.Errorf("status without a credential = %+v, %v", res, err)
	}
}

// Arguments that make no request are refused before any HTTP.
func TestListenBrainzAddonRefusesBadArguments(t *testing.T) {
	h, f := lbHost(t)
	for name, tc := range map[string]struct {
		method string
		params any
	}{
		"now-playing without a credential": {cli.AddonMethodScrobbleNowPlaying, cli.ScrobbleNowPlayingParams{Track: cli.ScrobbleTrack{Artist: "A", Title: "T"}}},
		"now-playing without a title":      {cli.AddonMethodScrobbleNowPlaying, cli.ScrobbleNowPlayingParams{Credential: "good", Track: cli.ScrobbleTrack{Artist: "A"}}},
		"submit nothing":                   {cli.AddonMethodScrobbleSubmit, cli.ScrobbleSubmitParams{Credential: "good"}},
		"submit over the batch": {cli.AddonMethodScrobbleSubmit, cli.ScrobbleSubmitParams{
			Credential: "good", Listens: make([]cli.ScrobbleListen, 101),
		}},
		"params that aren't": {cli.AddonMethodScrobbleStatus, "x"},
	} {
		if _, err := request(h, tc.method, tc.params); code(err) != cli.AddonErrBadArguments {
			t.Errorf("%s: %v", name, err)
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.auth) != 0 {
		t.Errorf("%d requests reached the service", len(f.auth))
	}
}

// A request that is not one of Sona's fixed names is refused, even to a
// bundled add-on; and an add-on that is not bundled is never sent one: the
// session refuses it and writes nothing to the add-on.
func TestHostRequestsAreBundledOnly(t *testing.T) {
	h, _ := lbHost(t)
	if _, err := h.sess.Request(context.Background(), "host.call", map[string]string{"name": "x"}); code(err) != cli.AddonErrUnknownMethod {
		t.Errorf("an unknown method to a bundled add-on: %v", err)
	}

	third := newHost(t, "demo", nil, nil)
	called := make(chan string, 1)
	third.cli.Handle(cli.AddonMethodScrobbleSubmit, func(context.Context, json.RawMessage) (any, error) {
		called <- "submit"
		return cli.ScrobbleResult{OK: true}, nil
	})
	out := serve(third, nil, nil)
	settled(t, third.sess)
	for _, m := range cli.HostMethods {
		if _, err := third.sess.Request(context.Background(), m, cli.ScrobbleStatusParams{}); code(err) != cli.AddonErrPermission {
			t.Errorf("%s to a third-party add-on: %v, want permission_denied", m, err)
		}
	}
	select {
	case m := <-called:
		t.Errorf("a third-party add-on's handler ran for %s", m)
	case <-time.After(100 * time.Millisecond):
	}
	third.sess.Close()
	_ = wait(t, out, "Serve")
}
