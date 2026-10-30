package services

import (
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

	"github.com/coredev-uk/sona-go/internal/auth/store"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/scrobble"
)

// memSecret is an in-memory store.Secret.
type memSecret struct {
	mu sync.Mutex
	v  string
}

func (m *memSecret) Get() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.v == "" {
		return "", store.ErrNotFound
	}
	return m.v, nil
}
func (m *memSecret) Set(v string) error { m.mu.Lock(); m.v = v; m.mu.Unlock(); return nil }
func (m *memSecret) Delete() error      { m.mu.Lock(); m.v = ""; m.mu.Unlock(); return nil }
func (m *memSecret) Backend() store.Backend {
	return store.BackendSecretService
}

type scrobbleEnv struct {
	s       *ScrobbleService
	secrets map[string]*memSecret
	opened  chan string
}

// newScrobbleEnv serves Last.fm (approving the token on the second poll)
// and ListenBrainz (accepting the token "good").
func newScrobbleEnv(t *testing.T) *scrobbleEnv {
	t.Helper()
	var polls int
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch {
		case r.URL.Path == "/1/validate-token":
			if r.Header.Get("Authorization") == "Token good" {
				_, _ = io.WriteString(w, `{"valid":true,"user_name":"lb-user"}`)
			} else {
				_, _ = io.WriteString(w, `{"valid":false}`)
			}
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
	settings, err := config.OpenStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	se := &scrobbleEnv{secrets: map[string]*memSecret{}, opened: make(chan string, 1)}
	e := &Env{Deps: Deps{
		Paths:    config.Paths{State: t.TempDir()},
		Settings: settings,
		Logger:   discard(),
		Emit:     &recorder{},
		OpenURL:  func(u string) error { se.opened <- u; return nil },
	}}
	lastfm := &scrobble.LastFM{Key: "key", Secret: "secret", API: srv.URL + "/2.0/", HTTP: srv.Client()}
	lb := &scrobble.ListenBrainz{API: srv.URL, HTTP: srv.Client()}
	se.s = newScrobble(e, lastfm, lb, func(key string) (store.Secret, error) {
		if se.secrets[key] == nil {
			se.secrets[key] = &memSecret{}
		}
		return se.secrets[key], nil
	})
	se.s.poll = time.Millisecond
	t.Cleanup(se.s.close)
	return se
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
	if se.secrets["lastfm"].v != "sk-1" {
		t.Fatal("session key not in the secret store")
	}
	if a := se.s.e.Settings.Get().Scrobbling.LastFM; a != (config.ScrobbleAccount{Enabled: true, Username: "fm-user"}) {
		t.Fatalf("settings = %+v", a)
	}

	st, err = se.s.Disconnect("lastfm")
	if err != nil || st.LastFM.Connected {
		t.Fatalf("Disconnect = %+v, %v", st, err)
	}
	if se.secrets["lastfm"].v != "" {
		t.Fatal("session key survived disconnect")
	}
	if a := se.s.e.Settings.Get().Scrobbling.LastFM; a != (config.ScrobbleAccount{}) {
		t.Fatalf("settings after disconnect = %+v", a)
	}
}

func TestScrobbleConnectListenBrainz(t *testing.T) {
	se := newScrobbleEnv(t)
	if _, err := se.s.ConnectListenBrainz(t.Context(), "  "); ErrorCode(err) != CodeInvalidArgument {
		t.Fatalf("empty token: %v", err)
	}
	_, err := se.s.ConnectListenBrainz(t.Context(), "wrong")
	if ErrorCode(err) != CodeInvalidCredentials || se.secrets["listenbrainz"] != nil && se.secrets["listenbrainz"].v != "" {
		t.Fatalf("bad token: %v (code %s)", err, ErrorCode(err))
	}
	st, err := se.s.ConnectListenBrainz(t.Context(), " good\n")
	if err != nil || !st.ListenBrainz.Connected {
		t.Fatalf("ConnectListenBrainz = %+v, %v", st, err)
	}
	if se.secrets["listenbrainz"].v != "good" {
		t.Fatalf("stored token = %q", se.secrets["listenbrainz"].v)
	}
	if a := se.s.e.Settings.Get().Scrobbling.ListenBrainz; a.Username != "lb-user" || !a.Enabled {
		t.Fatalf("settings = %+v", a)
	}
	if _, err := se.s.Disconnect("spotify"); ErrorCode(err) != CodeInvalidArgument {
		t.Fatalf("unknown provider: %v", err)
	}
}

func TestScrobbleLastFMNotConfigured(t *testing.T) {
	se := newScrobbleEnv(t)
	se.s.lastfm = nil
	_, err := se.s.ConnectLastFM(t.Context())
	if !errors.Is(err, scrobble.ErrNotConfigured) || ErrorCode(err) != CodeNotConfigured || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("err = %v", err)
	}
	if se.s.Status().LastFM.Available {
		t.Fatal("Last.fm reported available without an API account")
	}
}
