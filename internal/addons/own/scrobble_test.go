package own

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/cli"
)

// bundledHost is newHost for one of Sona's own add-ons: the session is marked
// bundled, and the client has the answers register gives its ID.
func bundledHost(t *testing.T, id string, opts Options, credentials ...map[string]string) *host {
	t.Helper()
	toSona, fromAddon := io.Pipe()
	toAddon, fromSona := io.Pipe()
	sess := cli.NewSession(cli.AddonConfig{
		ID: id, Host: fakeHost{}, Bundled: true, Allowed: func(string) bool { return true },
		Credentials: append(credentials, nil)[0],
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

// fakeLastFM answers the methods of Last.fm's API the add-on calls. The
// signature is the client's own (scrobble's tests check it against the
// guide's examples), so it isn't checked again here.
type fakeLastFM struct {
	mu      sync.Mutex
	calls   []url.Values
	pending int // auth.getSession answers "not authorised" this many times
}

func (f *fakeLastFM) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, r.Form)
	switch r.Form.Get("method") {
	case "auth.getToken":
		_, _ = io.WriteString(w, `{"token":"tok1"}`)
	case "auth.getSession":
		if f.pending > 0 {
			f.pending--
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"error":14,"message":"Unauthorized Token"}`)
			return
		}
		_, _ = io.WriteString(w, `{"session":{"name":"fm-user","key":"sk-1"}}`)
	case "user.getInfo":
		if r.Form.Get("sk") != "sk-1" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"error":9,"message":"Invalid session key"}`)
			return
		}
		_, _ = io.WriteString(w, `{"user":{"name":"fm-user"}}`)
	case "track.scrobble", "track.updateNowPlaying":
		_, _ = io.WriteString(w, `{}`)
	default:
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":3,"message":"Invalid Method"}`)
	}
}

func lastFMHost(t *testing.T, creds map[string]string) (*host, *fakeLastFM) {
	t.Helper()
	f := &fakeLastFM{}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	opts := Options{Version: "1.2.3", HTTP: srv.Client(), Endpoints: Endpoints{LastFM: srv.URL + "/2.0/", LastFMAuth: "http://127.0.0.1:1/auth/"}}
	return bundledHost(t, "lastfm", opts, creds), f
}

var lastFMAccount = map[string]string{cli.CredAPIKey: "the-key", cli.CredAPISecret: "the-secret"}

func authorise(h *host, token string) (cli.ScrobbleResult, error) {
	raw, err := h.sess.Request(context.Background(), cli.AddonMethodScrobbleAuthorise, cli.ScrobbleAuthoriseParams{Token: token})
	var res cli.ScrobbleResult
	if err == nil {
		err = json.Unmarshal(raw, &res)
	}
	return res, err
}

// The browser approval runs in the add-on: it gets the token and builds the
// page from the build's API key (which hello brought), and then answers
// pending until the user approves, with the session key and the account.
func TestLastFMAddonAuthorises(t *testing.T) {
	h, f := lastFMHost(t, lastFMAccount)
	f.pending = 1
	begun, err := authorise(h, "")
	if err != nil || begun.Token != "tok1" {
		t.Fatalf("begin = %+v, %v", begun, err)
	}
	u, perr := url.Parse(begun.URL)
	if perr != nil || u.Host != "127.0.0.1:1" || u.Query().Get("api_key") != "the-key" || u.Query().Get("token") != "tok1" {
		t.Errorf("approval page = %s", begun.URL)
	}
	if _, err := authorise(h, begun.Token); code(err) != cli.AddonErrAuthorisationPending {
		t.Errorf("before the approval: %v, want authorisation_pending", err)
	}
	done, err := authorise(h, begun.Token)
	if err != nil || done.Account != "fm-user" || done.Credential != "sk-1" {
		t.Errorf("after the approval = %+v, %v", done, err)
	}
}

// Without the build's Last.fm account nothing is sent: every request is
// not_configured, and the token flow never reaches the service.
func TestLastFMAddonWithoutAnAccountIsNotConfigured(t *testing.T) {
	h, f := lastFMHost(t, nil)
	if _, err := authorise(h, ""); code(err) != cli.AddonErrNotConfigured {
		t.Errorf("begin: %v", err)
	}
	if _, err := request(h, cli.AddonMethodScrobbleNowPlaying, cli.ScrobbleNowPlayingParams{Credential: "sk-1", Track: cli.ScrobbleTrack{Artist: "A", Title: "T"}}); code(err) != cli.AddonErrNotConfigured {
		t.Errorf("now-playing: %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) != 0 {
		t.Errorf("%d requests reached Last.fm", len(f.calls))
	}
}

// Last.fm does what ListenBrainz does: a now-playing and a batch, as the
// session key, and a status that says whose key it is.
func TestLastFMAddonSendsWhatSonaAsks(t *testing.T) {
	h, f := lastFMHost(t, lastFMAccount)
	track := cli.ScrobbleTrack{Artist: "Björk", Title: "Jóga", Album: "Homogenic", DurationMs: 305000}
	if _, err := request(h, cli.AddonMethodScrobbleNowPlaying, cli.ScrobbleNowPlayingParams{Credential: "sk-1", Track: track}); err != nil {
		t.Fatal(err)
	}
	listens := cli.ScrobbleSubmitParams{Credential: "sk-1", Listens: []cli.ScrobbleListen{{ScrobbleTrack: track, ListenedAt: 1700000000}}}
	if _, err := request(h, cli.AddonMethodScrobbleSubmit, listens); err != nil {
		t.Fatal(err)
	}
	if res, err := request(h, cli.AddonMethodScrobbleStatus, cli.ScrobbleStatusParams{Credential: "sk-1"}); err != nil || res.Account != "fm-user" {
		t.Errorf("status = %+v, %v", res, err)
	}
	if _, err := request(h, cli.AddonMethodScrobbleStatus, cli.ScrobbleStatusParams{Credential: "sk-wrong"}); code(err) != cli.AddonErrCredentialsRejected {
		t.Errorf("a refused key: %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var methods []string
	for _, c := range f.calls {
		methods = append(methods, c.Get("method"))
		if c.Get("api_key") != "the-key" || c.Get("api_sig") == "" {
			t.Errorf("%s was sent without the build's signature: %v", c.Get("method"), c)
		}
	}
	if strings.Join(methods, " ") != "track.updateNowPlaying track.scrobble user.getInfo user.getInfo" {
		t.Errorf("methods = %v", methods)
	}
	if got := f.calls[1].Get("timestamp[0]"); got != "1700000000" || f.calls[1].Get("artist[0]") != "Björk" {
		t.Errorf("scrobble = %v", f.calls[1])
	}
}

// ListenBrainz has no browser approval: the request is unknown to it.
func TestListenBrainzAddonHasNoAuthorisation(t *testing.T) {
	h, _ := lbHost(t)
	if _, err := authorise(h, ""); code(err) != cli.AddonErrUnknownMethod {
		t.Errorf("authorise on ListenBrainz: %v", err)
	}
}
