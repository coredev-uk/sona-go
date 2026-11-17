package services

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/addons/own"
	"github.com/coredev-uk/sona-go/internal/auth/store"
	"github.com/coredev-uk/sona-go/internal/cli"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/privatemode"
	"github.com/coredev-uk/sona-go/internal/scrobble"
)

type scrobbleEnv struct {
	s       *ScrobbleService
	srv     *httptest.Server
	kr      store.Keyring
	opened  chan string
	link    *pipeAddons
	private *privatemode.Mode

	mu       sync.Mutex
	lbBodies []string // what ListenBrainz's submit-listens got
	lbCode   int      // its answer's status, 0 for 200
}

// lbStatus makes ListenBrainz answer submit-listens with status.
func (se *scrobbleEnv) lbStatus(status int) {
	se.mu.Lock()
	se.lbCode = status
	se.mu.Unlock()
}

// newScrobbleEnv serves Last.fm (approving the token on the second poll)
// and ListenBrainz (accepting the token "good").
func newScrobbleEnv(t *testing.T) *scrobbleEnv {
	t.Helper()
	return newScrobbleEnvWith(t, &memKeyring{})
}

// newScrobbleEnvWith is newScrobbleEnv over the given keyring.
func newScrobbleEnvWith(t *testing.T, kr store.Keyring) *scrobbleEnv {
	t.Helper()
	var polls int
	var mu sync.Mutex
	se := &scrobbleEnv{private: privatemode.New(privatemode.Options{Save: func(bool) error { return nil }})}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch {
		case r.URL.Path == "/1/validate-token":
			if r.Header.Get("Authorization") == "Token good" {
				_, _ = io.WriteString(w, `{"valid":true,"user_name":"lb-user"}`)
			} else {
				_, _ = io.WriteString(w, `{"valid":false}`)
			}
		case r.URL.Path == "/1/submit-listens":
			b, _ := io.ReadAll(r.Body)
			se.mu.Lock()
			se.lbBodies = append(se.lbBodies, string(b))
			status := se.lbCode
			se.mu.Unlock()
			if status != 0 {
				w.WriteHeader(status)
				_, _ = io.WriteString(w, `{"error":"refused"}`)
				return
			}
			_, _ = io.WriteString(w, `{"status":"ok"}`)
		case r.Form.Get("api_sig") != scrobble.Sign(r.Form, "secret"):
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"error":13,"message":"Invalid method signature supplied"}`)
		case r.Form.Get("method") == "auth.getToken":
			_, _ = io.WriteString(w, `{"token":"tok"}`)
		case r.Form.Get("method") == "auth.getSession":
			mu.Lock()
			polls++
			n := polls
			mu.Unlock()
			if n < 2 {
				w.WriteHeader(http.StatusForbidden)
				_, _ = io.WriteString(w, `{"error":14,"message":"Unauthorized Token"}`)
				return
			}
			_, _ = io.WriteString(w, `{"session":{"name":"fm-user","key":"sk-1"}}`)
		default:
			_, _ = io.WriteString(w, `{}`)
		}
	}))
	t.Cleanup(srv.Close)
	se.srv = srv
	settings, err := config.OpenStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	se.kr, se.opened = kr, make(chan string, 1)
	e := &Env{Deps: Deps{
		Paths:    config.Paths{State: t.TempDir(), Config: t.TempDir()},
		Keyring:  se.kr,
		Settings: settings,
		Logger:   discard(),
		Emit:     &recorder{},
		OpenURL:  func(u string) error { se.opened <- u; return nil },
	}}
	se.link = newPipeAddons(t, own.Options{Version: "test", HTTP: srv.Client(), Endpoints: own.Endpoints{
		ListenBrainz: srv.URL, LastFM: srv.URL + "/2.0/", LastFMAuth: "https://www.last.fm/api/auth/",
	}})
	// The build's Last.fm account, as the supervisor sends it in hello.
	se.link.credentials["lastfm"] = map[string]string{cli.CredAPIKey: "key", cli.CredAPISecret: "secret"}
	e.Private = se.private
	se.s = newScrobble(e, true)
	se.s.attach(se.link)
	se.s.poll = time.Millisecond
	t.Cleanup(se.s.close)
	return se
}

// signIn is the sign-in stored for provider's add-on under key ("" when
// there is none): the keyring item `addon.<provider>` holds a JSON object.
func (se *scrobbleEnv) signIn(t *testing.T, provider, key string) string {
	t.Helper()
	raw, err := se.kr.Get("sona", "addon."+provider)
	if err != nil {
		return ""
	}
	m := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("addon.%s is not JSON: %v", provider, err)
	}
	return m[key]
}

func TestScrobbleConnectLastFMAndDisconnect(t *testing.T) {
	se := newScrobbleEnv(t)
	st, err := se.s.ConnectLastFM(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(<-se.opened)
	if u.Query().Get("token") != "tok" || u.Query().Get("api_key") != "key" {
		t.Fatalf("approval page = %s", u)
	}
	if !st.LastFM.Connected || st.LastFM.Connecting || st.ListenBrainz.Connected {
		t.Fatalf("status = %+v", st)
	}
	if se.signIn(t, "lastfm", "session") != "sk-1" {
		t.Fatal("session key not in the secret store")
	}
	if a := se.s.e.Settings.Get().Scrobbling.LastFM; a != (config.ScrobbleAccount{Enabled: true, Username: "fm-user"}) {
		t.Fatalf("settings = %+v", a)
	}

	st, err = se.s.Disconnect("lastfm")
	if err != nil || st.LastFM.Connected {
		t.Fatalf("Disconnect = %+v, %v", st, err)
	}
	if se.signIn(t, "lastfm", "session") != "" {
		t.Fatal("session key survived disconnect")
	}
	if a := se.s.e.Settings.Get().Scrobbling.LastFM; a != (config.ScrobbleAccount{}) {
		t.Fatalf("settings after disconnect = %+v", a)
	}
}

func TestScrobbleConnectListenBrainz(t *testing.T) {
	se := newScrobbleEnv(t)
	if _, err := se.s.ConnectListenBrainz(t.Context(), "  "); CodeOf(err) != CodeTokenRequired {
		t.Fatalf("empty token: %v", err)
	}
	_, err := se.s.ConnectListenBrainz(t.Context(), "wrong")
	if CodeOf(err) != CodeInvalidCredentials || se.signIn(t, "listenbrainz", "token") != "" {
		t.Fatalf("bad token: %v (code %s)", err, CodeOf(err))
	}
	st, err := se.s.ConnectListenBrainz(t.Context(), " good\n")
	if err != nil || !st.ListenBrainz.Connected {
		t.Fatalf("ConnectListenBrainz = %+v, %v", st, err)
	}
	if se.signIn(t, "listenbrainz", "token") != "good" {
		t.Fatalf("stored token = %q", se.signIn(t, "listenbrainz", "token"))
	}
	if a := se.s.e.Settings.Get().Scrobbling.ListenBrainz; a.Username != "lb-user" || !a.Enabled {
		t.Fatalf("settings = %+v", a)
	}
	if _, err := se.s.Disconnect("spotify"); CodeOf(err) != CodeInvalidArgument {
		t.Fatalf("unknown provider: %v", err)
	}
}

func TestScrobbleLastFMNotConfigured(t *testing.T) {
	se := newScrobbleEnv(t)
	se.s.lastfm = nil
	_, err := se.s.ConnectLastFM(t.Context())
	if !errors.Is(err, scrobble.ErrNotConfigured) || CodeOf(err) != CodeNotConfigured || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("err = %v", err)
	}
	if se.s.Status().LastFM.Available {
		t.Fatal("Last.fm reported available without an API account")
	}
}
