package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/auth/devtoken"
	"github.com/coredev-uk/sona-go/internal/auth/mut"
	"github.com/coredev-uk/sona-go/internal/auth/store"
)

// Long enough for the loopback fragment format (≥200 base64 characters).
var (
	goodMUT    = "0.Agood" + strings.Repeat("g", 240)
	revokedMUT = "0.Arevoked" + strings.Repeat("r", 240)
)

// fakeApple serves /v1/me/storefront and /v1/storefronts/{id} like Apple:
// the developer token must be "dev", the MUT must be in good, and offline
// drops connections.
type fakeApple struct {
	srv     *httptest.Server
	mu      sync.Mutex
	good    map[string]bool
	devOK   bool
	offline bool
	gate    chan struct{} // when non-nil, requests wait for it to close
	me      atomic.Int32  // /v1/me/storefront hits
	catalog atomic.Int32  // /v1/storefronts hits
}

func newFakeApple(t *testing.T) *fakeApple {
	f := &fakeApple{good: map[string]bool{goodMUT: true}, devOK: true}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeApple) set(fn func(f *fakeApple)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

func (f *fakeApple) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	gate, offline, devOK, good := f.gate, f.offline, f.devOK, f.good[r.Header.Get("Media-User-Token")]
	f.mu.Unlock()
	if gate != nil {
		<-gate
	}
	if offline {
		conn, _, _ := w.(http.Hijacker).Hijack()
		conn.Close()
		return
	}
	if r.Header.Get("Origin") != "https://music.apple.com" || !devOK || r.Header.Get("Authorization") != "Bearer dev" {
		http.Error(w, "", http.StatusUnauthorized)
		return
	}
	switch {
	case r.URL.Path == "/v1/me/storefront":
		f.me.Add(1)
		if !good {
			http.Error(w, "", http.StatusForbidden)
			return
		}
		w.Write([]byte(`{"data":[{"id":"gb","type":"storefronts","attributes":{"name":"United Kingdom","defaultLanguageTag":"en-GB"}}]}`))
	case strings.HasPrefix(r.URL.Path, "/v1/storefronts/"):
		f.catalog.Add(1)
		w.Write([]byte(`{"data":[{"id":"gb","type":"storefronts"}]}`))
	default:
		http.NotFound(w, r)
	}
}

type fakeDev struct{ invalidated atomic.Int32 }

func (d *fakeDev) Token(context.Context) (devtoken.Token, error) {
	return devtoken.Token{JWT: "dev", Expires: time.Now().Add(24 * time.Hour)}, nil
}
func (d *fakeDev) Invalidate() { d.invalidated.Add(1) }

type memStore struct {
	mu  sync.Mutex
	rec *store.Record
	bad bool
}

func (s *memStore) Get() (store.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.bad {
		return store.Record{}, store.ErrCorrupt
	}
	if s.rec == nil {
		return store.Record{}, store.ErrNotFound
	}
	return *s.rec, nil
}
func (s *memStore) Set(r store.Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rec, s.bad = &r, false
	return nil
}
func (s *memStore) Delete() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rec, s.bad = nil, false
	return nil
}
func (s *memStore) Backend() store.Backend { return store.BackendSecretService }
func (s *memStore) token() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rec == nil {
		return ""
	}
	return s.rec.Token
}

type memAccounts struct {
	mu   sync.Mutex
	acct *Account
}

func (a *memAccounts) Load(context.Context) (Account, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.acct == nil {
		return Account{}, ErrNoAccount
	}
	return *a.acct, nil
}
func (a *memAccounts) Save(_ context.Context, acct Account) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.acct = &acct
	return nil
}
func (a *memAccounts) Delete(context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.acct = nil
	return nil
}
func (a *memAccounts) get() *Account {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.acct
}

type env struct {
	apple    *fakeApple
	dev      *fakeDev
	store    *memStore
	accounts *memAccounts
}

func newEnv(t *testing.T) *env {
	return &env{apple: newFakeApple(t), dev: &fakeDev{}, store: &memStore{}, accounts: &memAccounts{}}
}

// manager builds a Manager recording every published Status.
func (e *env) manager(t *testing.T) (*Manager, *recorder) {
	t.Helper()
	m, err := NewManager(Config{
		HTTP: e.apple.srv.Client(), Dev: e.dev, Store: e.store, Accounts: e.accounts,
		APIBase: e.apple.srv.URL, RetryMin: 5 * time.Millisecond, RetryMax: 20 * time.Millisecond,
		TriageCooldown: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	r := &recorder{ch: make(chan Status, 256)}
	m.OnChange(func(s Status) {
		r.mu.Lock()
		r.all = append(r.all, s)
		r.mu.Unlock()
		r.ch <- s
	})
	return m, r
}

type recorder struct {
	mu  sync.Mutex
	all []Status
	ch  chan Status
}

// wait returns the first published Status satisfying ok.
func (r *recorder) wait(t *testing.T, ok func(Status) bool) Status {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case s := <-r.ch:
			if ok(s) {
				return s
			}
		case <-timeout:
			t.Fatal("timed out waiting for status")
		}
	}
}

func (r *recorder) count(state State) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, s := range r.all {
		if s.State == state {
			n++
		}
	}
	return n
}

func is(state State) func(Status) bool { return func(s Status) bool { return s.State == state } }

func (e *env) stored(tok string, withAccount bool) {
	e.store.Set(store.Record{Token: tok, ObtainedAt: time.Now().Add(-time.Hour), Restricted: true})
	if withAccount {
		e.accounts.Save(context.Background(), Account{StorefrontID: "gb", StorefrontName: "United Kingdom", Language: "en-GB"})
	}
}

func TestStartWithoutTokenSignedOut(t *testing.T) {
	e := newEnv(t)
	m, _ := e.manager(t)
	if m.Status().State != StateLoading {
		t.Fatalf("before Start: %v", m.Status().State)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s := m.Status(); s.State != StateSignedOut || s.Backend != store.BackendSecretService {
		t.Fatalf("status = %+v", s)
	}
	if m.UserToken() != "" {
		t.Fatal("token while signed out")
	}
}

func TestRestartSignedInWithoutBlockingOnNetwork(t *testing.T) {
	e := newEnv(t)
	first, _ := e.manager(t)
	if err := first.SignInWithToken(context.Background(), goodMUT); err != nil {
		t.Fatal(err)
	}
	first.Close()

	gate := make(chan struct{})
	e.apple.set(func(f *fakeApple) { f.gate = gate })
	m, rec := e.manager(t)
	if err := m.Start(context.Background()); err != nil { // must not wait for the gated server
		t.Fatal(err)
	}
	s := m.Status()
	if s.State != StateSignedIn || s.StorefrontID != "gb" || s.StorefrontName != "United Kingdom" || s.Language != "en-GB" {
		t.Fatalf("status after restart = %+v", s)
	}
	if m.UserToken() != goodMUT {
		t.Fatal("restored token not in use")
	}
	close(gate)
	rec.wait(t, func(s Status) bool { return s.State == StateSignedIn && !s.Offline })
	if a := e.accounts.get(); a == nil || a.MUTLastValidatedAt.IsZero() {
		t.Fatalf("account not revalidated: %+v", a)
	}
}

func TestStartWithoutAccountValidates(t *testing.T) {
	e := newEnv(t)
	e.stored(goodMUT, false)
	// Hold Apple's answer so the validating state can't already be over.
	gate := make(chan struct{})
	e.apple.set(func(f *fakeApple) { f.gate = gate })
	m, rec := e.manager(t)
	m.Start(context.Background())
	if m.Status().State != StateValidating {
		t.Fatalf("state = %v", m.Status().State)
	}
	close(gate)
	s := rec.wait(t, is(StateSignedIn))
	if s.StorefrontID != "gb" || !s.Restricted {
		t.Fatalf("status = %+v", s)
	}
}

func TestOfflineStartStaysSignedIn(t *testing.T) {
	e := newEnv(t)
	e.stored(goodMUT, true)
	e.apple.set(func(f *fakeApple) { f.offline = true })
	m, rec := e.manager(t)
	m.Start(context.Background())
	s := rec.wait(t, func(s Status) bool { return s.Offline })
	if s.State != StateSignedIn || m.UserToken() != goodMUT {
		t.Fatalf("offline status = %+v", s)
	}
	if rec.count(StateNeedsReauth) != 0 || e.store.token() != goodMUT {
		t.Fatal("offline treated as a rejected token")
	}
	e.apple.set(func(f *fakeApple) { f.offline = false })
	rec.wait(t, func(s Status) bool { return s.State == StateSignedIn && !s.Offline })
}

func TestRevokedTokenNeedsReauthOnce(t *testing.T) {
	e := newEnv(t)
	e.stored(revokedMUT, true)
	m, rec := e.manager(t)
	m.Start(context.Background())
	s := rec.wait(t, is(StateNeedsReauth))
	if s.StorefrontID != "gb" || s.LastError == "" {
		t.Fatalf("needs_reauth status = %+v", s)
	}
	if m.UserToken() != "" {
		t.Fatal("rejected token still handed to requests")
	}
	me, cat := e.apple.me.Load(), e.apple.catalog.Load()
	for range 20 {
		m.ReportUnauthorized(&applemusic.StatusError{Code: http.StatusForbidden})
	}
	time.Sleep(50 * time.Millisecond)
	if e.apple.me.Load() != me || e.apple.catalog.Load() != cat {
		t.Fatal("reports in needs_reauth reached Apple")
	}
	if n := rec.count(StateNeedsReauth); n != 1 {
		t.Fatalf("needs_reauth published %d times", n)
	}

	// A new sign-in re-arms triage.
	if err := m.SignInWithToken(context.Background(), goodMUT); err != nil {
		t.Fatal(err)
	}
	e.apple.set(func(f *fakeApple) { f.good = map[string]bool{} }) // Apple revokes it
	for range 20 {
		go m.ReportUnauthorized(&applemusic.StatusError{Code: http.StatusForbidden})
	}
	rec.wait(t, is(StateNeedsReauth))
	time.Sleep(50 * time.Millisecond)
	if n := rec.count(StateNeedsReauth); n != 2 {
		t.Fatalf("needs_reauth published %d times in total", n)
	}
}

func TestReportUnauthorizedKeepsValidToken(t *testing.T) {
	e := newEnv(t)
	m, rec := e.manager(t)
	m.Start(context.Background())
	if err := m.SignInWithToken(context.Background(), goodMUT); err != nil {
		t.Fatal(err)
	}
	me := e.apple.me.Load()
	m.ReportUnauthorized(errors.New("unrelated")) // not a 401/403: ignored
	m.ReportUnauthorized(&applemusic.StatusError{Code: http.StatusForbidden})
	deadline := time.Now().Add(5 * time.Second)
	for e.apple.me.Load() == me && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond)
	if s := m.Status(); s.State != StateSignedIn || m.UserToken() != goodMUT {
		t.Fatalf("status = %+v", s)
	}
	if rec.count(StateNeedsReauth) != 0 {
		t.Fatal("valid token marked needs_reauth")
	}
}

func TestDeveloperTokenRejected(t *testing.T) {
	e := newEnv(t)
	e.stored(goodMUT, true)
	e.apple.set(func(f *fakeApple) { f.devOK = false })
	m, rec := e.manager(t)
	m.Start(context.Background())
	s := rec.wait(t, is(StateError))
	if s.LastError != "developer token rejected" {
		t.Fatalf("status = %+v", s)
	}
	if e.dev.invalidated.Load() == 0 {
		t.Fatal("developer token never refreshed")
	}
	if rec.count(StateNeedsReauth) != 0 || e.store.token() != goodMUT {
		t.Fatal("developer token failure blamed on the MUT")
	}
	e.apple.set(func(f *fakeApple) { f.devOK = true })
	if err := m.Revalidate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if m.Status().State != StateSignedIn {
		t.Fatalf("after Revalidate: %+v", m.Status())
	}
}

func TestSignInWithToken(t *testing.T) {
	e := newEnv(t)
	m, rec := e.manager(t)
	m.Start(context.Background())

	err := m.SignInWithToken(context.Background(), revokedMUT)
	if !errors.Is(err, ErrMUTRejected) {
		t.Fatalf("revoked token: %v", err)
	}
	if s := m.Status(); s.State != StateSignedOut || s.LastError == "" || e.store.token() != "" {
		t.Fatalf("after rejected paste: %+v", s)
	}

	if err := m.SignInWithToken(context.Background(), "  "+goodMUT+"\n"); err != nil {
		t.Fatal(err)
	}
	s := m.Status()
	if s.State != StateSignedIn || s.StorefrontID != "gb" || s.LastError != "" {
		t.Fatalf("status = %+v", s)
	}
	if e.store.token() != goodMUT || m.UserToken() != goodMUT {
		t.Fatal("token not stored / in use")
	}
	if a := e.accounts.get(); a == nil || a.StorefrontID != "gb" || a.MUTObtainedAt.IsZero() || a.MUTLastValidatedAt.IsZero() {
		t.Fatalf("account = %+v", a)
	}
	if rec.count(StateValidating) < 2 {
		t.Fatal("validating not published")
	}
}

// browser plays the user's browser plus Apple: it returns an open func that
// posts Apple's fragment for tok to the loopback /complete endpoint.
func browser(t *testing.T, tok string) func(string) error {
	return func(startURL string) error {
		go func() {
			frag := base64.StdEncoding.EncodeToString([]byte(`{"cid":"c","itre":"1","musicUserToken":"` + tok + `"}`))
			body, _ := json.Marshal(map[string]string{"fragment": "#" + frag})
			origin := startURL[:strings.Index(startURL, "/auth/")]
			req, _ := http.NewRequest(http.MethodPost, strings.TrimSuffix(startURL, "/start")+"/complete", strings.NewReader(string(body)))
			req.Header.Set("Origin", origin)
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Error(err)
				return
			}
			resp.Body.Close()
		}()
		return nil
	}
}

func TestLoopbackSignIn(t *testing.T) {
	e := newEnv(t)
	m, rec := e.manager(t)
	m.Start(context.Background())
	if err := m.SignIn(context.Background(), browser(t, goodMUT)); err != nil {
		t.Fatal(err)
	}
	s := m.Status()
	if s.State != StateSignedIn || !s.Restricted || s.SignInURL != "" {
		t.Fatalf("status = %+v", s)
	}
	got, _ := e.store.Get()
	if got.Token != goodMUT || !got.Restricted {
		t.Fatalf("stored %v", got)
	}
	signingIn := rec.wait(t, func(s Status) bool { return s.State == StateSigningIn && s.SignInURL != "" })
	if !strings.HasPrefix(signingIn.SignInURL, "http://127.0.0.1:") {
		t.Fatalf("sign-in URL = %q", signingIn.SignInURL)
	}
}

func TestLoopbackSignInRejectedTokenRestoresState(t *testing.T) {
	e := newEnv(t)
	e.stored(revokedMUT, true)
	m, rec := e.manager(t)
	m.Start(context.Background())
	rec.wait(t, is(StateNeedsReauth))
	err := m.SignIn(context.Background(), browser(t, revokedMUT))
	if !errors.Is(err, ErrMUTRejected) {
		t.Fatalf("err = %v", err)
	}
	if s := m.Status(); s.State != StateNeedsReauth || s.LastError == "" {
		t.Fatalf("status = %+v", s)
	}
}

func TestCancelSignIn(t *testing.T) {
	e := newEnv(t)
	m, _ := e.manager(t)
	m.Start(context.Background())
	urls := make(chan string, 1)
	done := make(chan error, 1)
	go func() {
		done <- m.SignIn(context.Background(), func(u string) error { urls <- u; return nil })
	}()
	u := <-urls
	if s := m.Status(); s.State != StateSigningIn || s.SignInURL != u {
		t.Fatalf("status = %+v", s)
	}
	m.CancelSignIn()
	if s := m.Status(); s.State != StateSignedOut || s.LastError != "" {
		t.Fatalf("after cancel: %+v", s)
	}
	if err := <-done; !errors.Is(err, mut.ErrCanceled) {
		t.Fatalf("SignIn = %v", err)
	}
	if resp, err := http.Get(u); err == nil {
		resp.Body.Close()
		t.Fatal("loopback server still serving after cancel")
	}
	m.CancelSignIn() // no attempt: no-op
}

func TestSecondSignInSupersedesFirst(t *testing.T) {
	e := newEnv(t)
	m, _ := e.manager(t)
	m.Start(context.Background())
	urls := make(chan string, 1)
	first := make(chan error, 1)
	go func() {
		first <- m.SignIn(context.Background(), func(u string) error { urls <- u; return nil })
	}()
	<-urls
	if err := m.SignIn(context.Background(), browser(t, goodMUT)); err != nil {
		t.Fatal(err)
	}
	if err := <-first; !errors.Is(err, mut.ErrCanceled) {
		t.Fatalf("first SignIn = %v", err)
	}
	if m.Status().State != StateSignedIn {
		t.Fatalf("status = %+v", m.Status())
	}
}

func TestOpenFailureKeepsWaiting(t *testing.T) {
	e := newEnv(t)
	m, rec := e.manager(t)
	m.Start(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- m.SignIn(context.Background(), func(string) error { return errors.New("no xdg-open") })
	}()
	s := rec.wait(t, func(s Status) bool { return s.LastError != "" })
	if s.State != StateSigningIn || s.SignInURL == "" {
		t.Fatalf("status = %+v", s)
	}
	m.CancelSignIn()
	<-done
}

func TestSignOutClearsStoreAccountAndRunsHooks(t *testing.T) {
	e := newEnv(t)
	e.stored(goodMUT, true)
	m, rec := e.manager(t)
	m.Start(context.Background())
	var order []string
	m.OnSignOut("engine", func(context.Context) error { order = append(order, "engine"); return nil })
	m.OnSignOut("userdata", func(context.Context) error { order = append(order, "userdata"); return errors.New("db locked") })
	m.OnSignOut("artwork", func(context.Context) error {
		if e.store.token() != "" || e.accounts.get() != nil {
			t.Error("hook ran before store and account were deleted")
		}
		order = append(order, "artwork")
		return nil
	})

	err := m.SignOut(context.Background())
	if err == nil || !strings.Contains(err.Error(), "userdata: db locked") {
		t.Fatalf("SignOut = %v", err)
	}
	if strings.Join(order, ",") != "engine,userdata,artwork" {
		t.Fatalf("hook order = %v", order)
	}
	s := rec.wait(t, is(StateSignedOut))
	if s.StorefrontID != "" || !strings.Contains(s.LastError, "db locked") {
		t.Fatalf("status = %+v", s)
	}
	if e.store.token() != "" || e.accounts.get() != nil || m.UserToken() != "" {
		t.Fatal("credentials survived sign-out")
	}
	m.ReportUnauthorized(&applemusic.StatusError{Code: http.StatusForbidden}) // ignored when signed out
	if m.Status().State != StateSignedOut {
		t.Fatal("report changed signed_out")
	}
}

func TestCorruptStoreSignsOut(t *testing.T) {
	e := newEnv(t)
	e.store.bad = true
	m, _ := e.manager(t)
	m.Start(context.Background())
	if s := m.Status(); s.State != StateSignedOut || s.LastError == "" {
		t.Fatalf("status = %+v", s)
	}
	if _, err := e.store.Get(); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("corrupt record kept: %v", err)
	}
}
