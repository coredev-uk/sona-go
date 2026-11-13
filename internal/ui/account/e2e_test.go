package account

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/auth"
	"github.com/coredev-uk/sona-go/internal/auth/appleid/appleidtest"
	authstore "github.com/coredev-uk/sona-go/internal/auth/store"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// These tests run the Section over the real services.AuthService and
// auth.Manager, with Apple's sign-in endpoints and Music API faked
// (appleidtest, an httptest server), the way the app wires them: the
// auth:changed event writes the status query.

// Long enough for the loopback fragment format (≥200 base64 characters).
var (
	goodMUT    = "0.Agood" + strings.Repeat("g", 240)
	revokedMUT = "0.Arevoked" + strings.Repeat("r", 240)
)

// newMusicAPI serves /v1/me/storefront and /v1/storefronts/{id} like Apple:
// the developer token must be "dev", the request must come from
// music.apple.com, and the Music User Token must be goodMUT.
func newMusicAPI(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") != "https://music.apple.com" || r.Header.Get("Authorization") != "Bearer dev" {
			http.Error(w, "", http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == "/v1/me/storefront":
			if r.Header.Get("Media-User-Token") != goodMUT {
				http.Error(w, "", http.StatusForbidden)
				return
			}
			_, _ = w.Write([]byte(`{"data":[{"id":"gb","type":"storefronts","attributes":{"name":"United Kingdom","defaultLanguageTag":"en-GB","supportedLanguageTags":["en-GB","cy-GB"]}}]}`))
		case strings.HasPrefix(r.URL.Path, "/v1/storefronts/"):
			_, _ = w.Write([]byte(`{"data":[{"id":"gb","type":"storefronts"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// memStore is a keyring in memory.
type memStore struct {
	mu  sync.Mutex
	rec *authstore.Record
	err error // returned by Get, like a keyring that failed
}

func (s *memStore) Get() (authstore.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return authstore.Record{}, s.err
	}
	if s.rec == nil {
		return authstore.Record{}, authstore.ErrNotFound
	}
	return *s.rec, nil
}

func (s *memStore) Set(r authstore.Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rec = &r
	return nil
}

func (s *memStore) Delete() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rec = nil
	return nil
}

func (s *memStore) Backend() authstore.Backend { return authstore.BackendSecretService }

func (s *memStore) token() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rec == nil {
		return ""
	}
	return s.rec.Token
}

func (s *memStore) setErr(err error) {
	s.mu.Lock()
	s.err = err
	s.mu.Unlock()
}

// recordingHandler is a slog handler that keeps every record as one line
// (message and attributes), for what must never be logged.
type recordingHandler struct {
	root  *recordingHandler
	mu    sync.Mutex
	lines []string
	pre   []slog.Attr
}

func newLogger(h *recordingHandler) *slog.Logger { return slog.New(h) }

func (h *recordingHandler) sink() *recordingHandler {
	if h.root != nil {
		return h.root
	}
	return h
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder
	b.WriteString(r.Message)
	add := func(a slog.Attr) { fmt.Fprintf(&b, " %s=%+v", a.Key, a.Value.Resolve().Any()) }
	for _, a := range h.pre {
		add(a)
	}
	r.Attrs(func(a slog.Attr) bool { add(a); return true })
	s := h.sink()
	s.mu.Lock()
	s.lines = append(s.lines, b.String())
	s.mu.Unlock()
	return nil
}

func (h *recordingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &recordingHandler{root: h.sink(), pre: append(slices.Clone(h.pre), attrs...)}
}

func (h *recordingHandler) WithGroup(string) slog.Handler { return h }

func (h *recordingHandler) all() []string {
	s := h.sink()
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.lines)
}

// e2e is a Section over the real AuthService.
type e2e struct {
	*rig
	mem  *memStore
	logs *recordingHandler
	seen []string // auth states the auth:changed handler saw, in order
}

func newE2E(t *testing.T, cfg appleidtest.Config, mem *memStore) *e2e {
	t.Helper()
	music := newMusicAPI(t)
	idms := appleidtest.New(t, cfg)
	logs := &recordingHandler{}
	h := uitest.New(t, uitest.AuthConfig(func(c *auth.Config) {
		c.APIBase = music.URL
		c.HTTP = music.Client() // the harness's Network serves no loopback server
		c.AppleID = idms.Options()
		c.Open = func() (authstore.Store, error) { return mem, nil }
		c.Logger = newLogger(logs)
		c.SignInTimeout = time.Minute
	}))
	e := &e2e{rig: &rig{t: t, h: h}, mem: mem, logs: logs}
	d := NewDeps(h.Env)
	d.Log = newLogger(logs)
	e.sec = New(d)
	// What features/auth does on auth:changed.
	off := store.On(h.Env.Bus, services.EventAuthChanged, func(st services.AuthStatus) {
		e.seen = append(e.seen, st.State)
		store.Set(h.Env.Queries, store.KeyAuth, st)
	})
	t.Cleanup(off)
	kit.SetToastPoster(h.Main.Post)
	t.Cleanup(func() { kit.CancelConfirm(); kit.CloseToast("") })
	e.tt = h.Tester(t, func(c *ui.Context) {
		ui.Column(c.Key("page")).FillWidth().Padding(24).Children(func() {
			e.sec.Build(c, false)
		})
		kit.ConfirmHost(c)
		kit.Toaster(c)
	}, 720, 1100)
	return e
}

// ready waits for the sign-in form.
func (e *e2e) ready() {
	e.t.Helper()
	e.tt.Until("the sign-in form", e.tt.Shows("Use the Apple Account with your Apple Music subscription."))
}

// signedIn waits for the signed-in account view of the fake account.
func (e *e2e) signedIn() {
	e.t.Helper()
	e.tt.Until("the signed-in account", e.tt.Shows("Signed in to Apple Music", "Storefront: United Kingdom · British English"))
}

// secretFree fails if any of secrets reached a log, a page opened or a text
// copied. A short secret (a code) matches as a whole word only.
func (e *e2e) secretFree(secrets ...string) {
	e.t.Helper()
	places := append(e.logs.all(), e.h.Opened()...)
	places = append(places, e.h.Copied()...)
	for _, s := range secrets {
		re := regexp.MustCompile(`\b` + regexp.QuoteMeta(s) + `\b`)
		for _, p := range places {
			if re.MatchString(p) {
				e.t.Errorf("a secret reached %q", p)
			}
		}
	}
}

func TestEndToEndPasswordSignInAndSignOut(t *testing.T) {
	mem := &memStore{}
	e := newE2E(t, appleidtest.Config{Account: "a@example.com", Password: "pw-secret-1", MUT: goodMUT, Restricted: true}, mem)
	e.ready()
	e.typeInto("Apple Account", "  a@example.com ")
	e.typeInto("Password", "pw-secret-1")
	e.click("Sign in")
	e.signedIn()
	e.shows("Explicit content is restricted on this account; Sona hides it.", "Where your token is kept", "Secret Service keyring")
	e.hides("Your token is stored unencrypted")
	if mem.token() != goodMUT {
		t.Error("the token was not stored in the keyring")
	}
	if !slices.Contains(e.seen, "signed_in") {
		t.Errorf("auth:changed states %q", e.seen)
	}
	if f := e.sec.flow; f != nil {
		t.Errorf("the sign-in form outlived the sign-in: %+v", f)
	}

	// Sign out: the question, then the form again, the keyring empty.
	e.click("Sign out")
	e.shows("Sign out of Apple Music?")
	e.tt.Key(0, ui.KeyEnter)
	e.tt.Until("the form", func() bool {
		return e.tt.HasText("Use the Apple Account with your Apple Music subscription.") && !e.tt.HasText("Sign out of Apple Music?")
	})
	if mem.token() != "" {
		t.Error("the token is still in the keyring")
	}
	e.secretFree("pw-secret-1", goodMUT)
}

func TestEndToEndWrongPasswordThenRight(t *testing.T) {
	mem := &memStore{}
	e := newE2E(t, appleidtest.Config{Account: "a@example.com", Password: "pw-secret-2", MUT: goodMUT}, mem)
	e.ready()
	e.typeInto("Apple Account", "a@example.com")
	e.typeInto("Password", "not-the-password")
	e.click("Sign in")
	e.tt.Until("the error", e.tt.Shows("That Apple Account or password isn't right"))
	if e.sec.flow.password != "" || mem.token() != "" {
		t.Errorf("password %q kept, token %q stored", e.sec.flow.password, mem.token())
	}
	e.typeInto("Password", "pw-secret-2")
	e.click("Sign in")
	e.signedIn()
	e.secretFree("pw-secret-2", "not-the-password", goodMUT)
}

func TestEndToEndTwoFactorWrongCodeThenRightCode(t *testing.T) {
	mem := &memStore{}
	e := newE2E(t, appleidtest.Config{Account: "a@example.com", Password: "pw-secret-3", MUT: goodMUT, TwoFactor: true}, mem)
	e.ready()
	e.typeInto("Apple Account", "a@example.com")
	e.typeInto("Password", "pw-secret-3")
	e.click("Sign in")
	e.tt.Until("the code step", e.tt.Shows("Enter the verification code"))
	e.shows("Apple sent a 6-digit code to the devices signed in to this Apple Account. Enter it here to finish signing in.")

	// A wrong code: the step stays, the field clears, Apple's attempt stays open.
	e.typeInto("Code", "999999")
	e.tt.Until("the wrong code", e.tt.Shows("That code isn't right"))
	e.shows("Enter the verification code")
	if e.sec.flow.code != "" || e.mem.token() != "" {
		t.Errorf("code %q kept, token %q stored", e.sec.flow.code, e.mem.token())
	}

	e.typeInto("Code", appleidtest.Code)
	e.signedIn()
	if mem.token() != goodMUT {
		t.Error("the token was not stored in the keyring")
	}
	e.secretFree("pw-secret-3", appleidtest.Code, "999999", goodMUT)
}

func TestEndToEndNoDeviceAccountGetsACallOnTheChosenNumber(t *testing.T) {
	mem := &memStore{}
	e := newE2E(t, appleidtest.Config{
		Account: "a@example.com", Password: "pw-secret-4", MUT: goodMUT, TwoFactor: true, NoDevices: true, NoAutoSend: true,
	}, mem)
	e.ready()
	e.typeInto("Apple Account", "a@example.com")
	e.typeInto("Password", "pw-secret-4")
	e.click("Sign in")
	e.tt.Until("the phone choice", e.tt.Shows("Which number should Apple text?"))
	e.shows("No Apple device is signed in to this account, so Apple sends codes by text or call.")
	e.has("+44 ••••• ••••12", "+1 (•••) •••-••34")

	e.click("Phone call")
	e.shows("Which number should Apple call?")
	e.click("+1 (•••) •••-••34")
	e.click("Call me")
	e.tt.Until("the code step", e.tt.Shows("Apple is calling +1 (•••) •••-••34 with a 6-digit code."))
	e.shows("Apple is calling with your code.")

	e.typeInto("Code", appleidtest.Code)
	e.signedIn()
	if mem.token() != goodMUT {
		t.Error("the token was not stored in the keyring")
	}
	e.secretFree("pw-secret-4", appleidtest.Code, goodMUT)
}

// browser plays the user's browser plus Apple: it loads the start link
// (taking the attempt's cookie) and posts Apple's fragment for tok to the
// loopback /complete endpoint with that cookie.
func browser(t *testing.T, startURL, tok string) {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}
	resp, err := c.Get(startURL)
	if err != nil {
		t.Error(err)
		return
	}
	_ = resp.Body.Close()
	frag := base64.StdEncoding.EncodeToString([]byte(`{"cid":"c","itre":"1","musicUserToken":"` + tok + `"}`))
	body, _ := json.Marshal(map[string]string{"fragment": "#" + frag})
	origin, rest, _ := strings.Cut(startURL, "/auth/")
	nonce, _, _ := strings.Cut(rest, "/start")
	req, _ := http.NewRequest(http.MethodPost, origin+"/auth/"+nonce+"/complete", strings.NewReader(string(body)))
	req.Header.Set("Origin", origin)
	req.Header.Set("Content-Type", "application/json")
	resp, err = c.Do(req)
	if err != nil {
		t.Error(err)
		return
	}
	_ = resp.Body.Close()
}

func TestEndToEndBrowserFallbackThroughTheLoopback(t *testing.T) {
	mem := &memStore{}
	e := newE2E(t, appleidtest.Config{Account: "a@example.com", Password: "pw", MUT: goodMUT}, mem)
	e.ready()
	e.click("Sign in with your browser")
	e.tt.Until("the page opened", func() bool { return len(e.h.Opened()) == 1 })
	e.tt.Until("the waiting view", e.tt.Shows("Finish signing in in your browser", "Open the page again", "Copy link"))
	e.lacks("Sign in with your browser")
	first := e.h.Opened()[0]
	if !strings.HasPrefix(first, "http://127.0.0.1:") || !strings.Contains(first, "/auth/") {
		t.Fatalf("opened %q", first)
	}

	// A new single-use link opens the page again, and copies for another browser.
	e.click("Open the page again")
	e.tt.Until("the page opened again", func() bool { return len(e.h.Opened()) == 2 })
	e.click("Copy link")
	e.tt.Until("the link copied", func() bool { return len(e.h.Copied()) == 1 })
	e.tt.Until("the toast", e.tt.Shows("Sign-in link copied"))
	if second, copied := e.h.Opened()[1], e.h.Copied()[0]; second == first || copied == first || copied == second {
		t.Errorf("the links repeat: %q %q %q", first, second, copied)
	}

	// The user finishes in the browser; Sona continues by itself.
	done := make(chan struct{})
	go func() { defer close(done); browser(t, first, goodMUT) }()
	e.signedIn()
	<-done
	if mem.token() != goodMUT {
		t.Error("the token was not stored in the keyring")
	}
	e.secretFree(goodMUT)
}

func TestEndToEndBrowserFallbackCancel(t *testing.T) {
	mem := &memStore{}
	e := newE2E(t, appleidtest.Config{Account: "a@example.com", Password: "pw", MUT: goodMUT}, mem)
	e.ready()
	e.click("Sign in with your browser")
	e.tt.Until("the waiting view", e.tt.Shows("Finish signing in in your browser"))
	e.click("Cancel")
	e.tt.Until("the form", e.tt.Shows("Use the Apple Account with your Apple Music subscription."))
	e.lacks("Finish signing in in your browser")
	if mem.token() != "" {
		t.Error("a cancelled sign-in stored a token")
	}
}

func TestEndToEndTokenPasteRejectedThenValid(t *testing.T) {
	mem := &memStore{}
	e := newE2E(t, appleidtest.Config{Account: "a@example.com", Password: "pw", MUT: goodMUT}, mem)
	e.ready()
	e.click("Paste a Music User Token")
	e.typeInto("Music User Token", revokedMUT)
	e.click("Use this token")
	e.tt.Until("the rejection", e.tt.Shows("Apple didn't accept that token. Copy a fresh one from a signed-in Apple Music web session."))
	if mem.token() != "" {
		t.Error("a rejected token was stored")
	}
	e.hides("Signed in to Apple Music")

	// Pasting the good one (the field is replaced as a paste would).
	e.sec.flow.token = "  " + goodMUT + "\n"
	e.tt.Drain()
	e.click("Use this token")
	e.signedIn()
	if mem.token() != goodMUT {
		t.Error("the token was not stored in the keyring")
	}
	e.secretFree(goodMUT, revokedMUT)
}

func TestEndToEndKeyringUnavailableThenRetry(t *testing.T) {
	mem := &memStore{rec: &authstore.Record{Token: goodMUT, ObtainedAt: time.Now().Add(-time.Hour), Restricted: true}}
	mem.setErr(errors.New("keyring get: no such secret collection"))
	e := newE2E(t, appleidtest.Config{Account: "a@example.com", Password: "pw", MUT: goodMUT}, mem)
	e.tt.Until("the keyring notice", e.tt.Shows("Sona couldn't read your sign-in from your keyring"))
	e.has("Try again", "Apple Account")
	if mem.token() != goodMUT {
		t.Fatal("the stored sign-in was dropped")
	}

	mem.setErr(nil)
	e.click("Try again")
	e.signedIn()
	e.hides("Sona couldn't read your sign-in from your keyring")
	if !slices.Contains(e.seen, "restoring") || e.seen[len(e.seen)-1] != "signed_in" {
		t.Errorf("auth:changed states %q", e.seen)
	}
	e.secretFree(goodMUT)
}
