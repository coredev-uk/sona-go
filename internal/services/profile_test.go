package services

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/auth"
	"github.com/coredev-uk/sona-go/internal/auth/store"
	"github.com/coredev-uk/sona-go/internal/db"
)

// TestGivenName is the greeting's rule for which part of a profile name to
// use (docs/design.md, "The greeting"): the first word where the script puts
// the given name first, the whole name otherwise. The names are made up.
func TestGivenName(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"Alex Morgan", "Alex"},
		{"  Alex   Morgan  ", "Alex"},
		{"Mary Ann Smith", "Mary"},
		{"Jean-Luc Picard", "Jean-Luc"},
		{"Zoë Åkesson", "Zoë"},
		{"Анна Иванова", "Анна"},
		{"Αλέξης Παπαδόπουλος", "Αλέξης"},
		{"Anna 田中", "Anna 田中"},   // mixed scripts: whole
		{"山田 太郎", "山田 太郎"},       // Han: family name first
		{"やまだ たろう", "やまだ たろう"},   // Kana
		{"김 민준", "김 민준"},         // Hangul
		{"محمد علي", "محمد علي"}, // not cut at a space
		{"Alex", "Alex"},         // one word stays as it is
		{"山田", "山田"},
		{"", ""},
		{"   ", ""},
		{"1234 5678", "1234 5678"}, // no letters: whole
	} {
		if got := givenName(c.in); got != c.want {
			t.Errorf("givenName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// fakeProfileApple answers the sign-in validation and the own-profile call,
// counting the latter. The person is made up.
type fakeProfileApple struct {
	mu        sync.Mutex
	asked     int
	none      bool // 404: no profile at all
	onboarded bool
	name      string
}

func (f *fakeProfileApple) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/v1/me/storefront":
		_, _ = w.Write([]byte(`{"data":[{"id":"gb","type":"storefronts","attributes":{"name":"United Kingdom","defaultLanguageTag":"en-GB","explicitContentPolicy":"allowed"}}]}`))
	case "/v1/me/social/profile":
		f.mu.Lock()
		f.asked++
		none, onboarded, name := f.none, f.onboarded, f.name
		f.mu.Unlock()
		if none {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{
			"id": "me", "type": "personal-social-profiles",
			"attributes":    map[string]any{"name": name, "handle": "alex.m", "isOnboarded": onboarded},
			"relationships": map[string]any{"social-profile": map[string]any{"data": []any{map[string]any{"id": "sp.alex", "type": "social-profiles"}}}},
		}}})
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeProfileApple) requests() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.asked
}

// profileRig is a ProfileService over a real account manager signed in with
// a stored token, a database that outlives restarts and a gate a test opens.
type profileRig struct {
	t     *testing.T
	d     *db.DB
	apple *fakeProfileApple
	srv   *httptest.Server
	clock *testClock

	e     *Env
	s     *ProfileService
	authm *auth.Manager
	emit  *recorder
	gate  *Gate
}

func newProfileRig(t *testing.T) *profileRig {
	t.Helper()
	d, err := db.Open(t.Context(), filepath.Join(t.TempDir(), "sona.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	r := &profileRig{
		t: t, d: d, apple: &fakeProfileApple{onboarded: true, name: "Alex Morgan"},
		clock: &testClock{t: time.Date(2026, 3, 10, 9, 0, 0, 0, time.UTC)},
	}
	r.srv = httptest.NewServer(r.apple)
	t.Cleanup(r.srv.Close)
	return r
}

// start runs Sona's launch: the service is built, the stored sign-in is
// restored (signedIn) and the first paint has or has not happened (painted).
func (r *profileRig) start(signedIn, painted bool) {
	r.t.Helper()
	if r.authm != nil {
		r.authm.Close()
		r.e.close()
	}
	var creds store.Store = &emptyStore{}
	if signedIn {
		creds = &recordStore{rec: store.Record{Token: "mut", ObtainedAt: time.Now()}}
	}
	authm, err := auth.NewManager(auth.Config{
		HTTP: r.srv.Client(), Dev: testDev{}, APIBase: r.srv.URL,
		Open:     func() (store.Store, error) { return creds, nil },
		Accounts: AccountStore{DB: r.d}, Logger: discard(),
	})
	if err != nil {
		r.t.Fatal(err)
	}
	r.t.Cleanup(authm.Close)
	api := applemusic.New(r.srv.Client(), testDev{}, authm.UserToken)
	api.Base, api.AMPBase = r.srv.URL, r.srv.URL
	r.authm, r.emit, r.gate = authm, &recorder{}, NewGate(time.Hour, nil)
	r.e = &Env{Deps: Deps{DB: r.d, API: api, Auth: authm, Gate: r.gate, Emit: r.emit, Logger: discard(), Now: r.clock.Now}}
	r.t.Cleanup(r.e.close)
	r.s = newProfileService(r.e)
	if painted {
		r.gate.Open()
	}
	authm.Start()
	want := auth.StateSignedOut
	if signedIn {
		want = auth.StateSignedIn
	}
	r.until("the sign-in state", func() bool { return authm.Status().State == want })
}

func (r *profileRig) until(what string, ok func() bool) {
	r.t.Helper()
	for deadline := time.Now().Add(5 * time.Second); !ok(); time.Sleep(5 * time.Millisecond) {
		if time.Now().After(deadline) {
			r.t.Fatalf("timed out waiting for %s", what)
		}
	}
}

func (r *profileRig) name() string {
	r.t.Helper()
	g, err := r.s.GreetingName(r.t.Context())
	if err != nil {
		r.t.Fatal(err)
	}
	return g.Name
}

// lastEvent is the payload of the latest "profile:changed", ok false when
// there was none.
func (r *profileRig) lastEvent() (GreetingName, bool) {
	r.emit.mu.Lock()
	defer r.emit.mu.Unlock()
	for i := len(r.emit.events) - 1; i >= 0; i-- {
		if r.emit.events[i] == EventProfileChanged {
			return r.emit.data[i].(GreetingName), true
		}
	}
	return GreetingName{}, false
}

func TestProfileNameIsFetchedAfterFirstPaintAndStored(t *testing.T) {
	r := newProfileRig(t)
	r.start(true, false)

	// Signed in, the first frame not drawn: nothing is asked, nothing known.
	time.Sleep(100 * time.Millisecond)
	if n := r.apple.requests(); n != 0 || r.name() != "" {
		t.Fatalf("before the first paint: %d requests, name %q", n, r.name())
	}

	r.gate.Open()
	r.until("the name", func() bool { return r.name() == "Alex" })
	if n := r.apple.requests(); n != 1 {
		t.Fatalf("%d requests, want 1", n)
	}
	if g, ok := r.lastEvent(); !ok || g.Name != "Alex" {
		t.Fatalf("event %+v (sent %v), want the name Alex", g, ok)
	}
	// What is kept is the name as Apple has it, never the handle.
	row, err := r.d.RQ.GetAccountProfile(t.Context())
	if err != nil || row.ProfileName != "Alex Morgan" || !row.ProfileCheckedAt.Valid {
		t.Fatalf("stored %+v, %v", row, err)
	}
}

func TestProfileNameIsReadFromStorageAtColdStart(t *testing.T) {
	r := newProfileRig(t)
	r.start(true, true)
	r.until("the name", func() bool { return r.name() == "Alex" })

	// Another launch, a few hours on: the name is there before anything
	// could be asked, and nothing is asked afterwards either.
	r.clock.set(r.clock.Now().Add(3 * time.Hour))
	r.start(true, false)
	if got := r.name(); got != "Alex" {
		t.Fatalf("cold start name %q, want Alex", got)
	}
	r.gate.Open()
	time.Sleep(150 * time.Millisecond)
	if n := r.apple.requests(); n != 1 {
		t.Fatalf("%d requests across both launches, want 1", n)
	}
}

func TestProfileNameIsAskedAgainOnlyAfterADay(t *testing.T) {
	r := newProfileRig(t)
	r.start(true, true)
	r.until("the name", func() bool { return r.name() == "Alex" })

	r.apple.mu.Lock()
	r.apple.name = "Sam Rivera"
	r.apple.mu.Unlock()
	r.clock.set(r.clock.Now().Add(23 * time.Hour))
	r.s.refresh(false)
	if n := r.apple.requests(); n != 1 || r.name() != "Alex" {
		t.Fatalf("after 23 h: %d requests, name %q", n, r.name())
	}
	r.clock.set(r.clock.Now().Add(2 * time.Hour))
	r.s.refresh(false)
	if n := r.apple.requests(); n != 2 || r.name() != "Sam" {
		t.Fatalf("after 25 h: %d requests, name %q", n, r.name())
	}
}

func TestProfileNameIsClearedAtSignOut(t *testing.T) {
	r := newProfileRig(t)
	r.start(true, true)
	r.until("the name", func() bool { return r.name() == "Alex" })

	if err := r.authm.SignOut(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := r.name(); got != "" {
		t.Fatalf("name %q after sign-out", got)
	}
	if g, ok := r.lastEvent(); !ok || g.Name != "" {
		t.Fatalf("last event %+v (sent %v), want an empty name", g, ok)
	}
	// A fetch that was still on its way finds nobody signed in.
	r.s.refresh(true)
	if got := r.name(); got != "" {
		t.Fatalf("name %q after a late fetch", got)
	}
}

func TestNoProfileOrOneNotSetUpMeansNoName(t *testing.T) {
	for _, c := range []struct {
		what string
		set  func(f *fakeProfileApple)
	}{
		{"no profile", func(f *fakeProfileApple) { f.none = true }},
		{"not set up", func(f *fakeProfileApple) { f.onboarded = false }},
	} {
		t.Run(c.what, func(t *testing.T) {
			r := newProfileRig(t)
			c.set(r.apple)
			r.start(true, true)
			r.until("the answer", func() bool {
				row, err := r.d.RQ.GetAccountProfile(t.Context())
				return err == nil && row.ProfileCheckedAt.Valid
			})
			if got := r.name(); got != "" {
				t.Fatalf("name %q, want none", got)
			}
			// The answer counts as asked: no request until a day has passed.
			r.s.refresh(false)
			if n := r.apple.requests(); n != 1 {
				t.Fatalf("%d requests, want 1", n)
			}
		})
	}
}
