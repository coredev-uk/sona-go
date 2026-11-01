package mut

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// appleFragment builds the payload authorize.music.apple.com appends:
// btoa(JSON.stringify({cid, itre, musicUserToken})).
func appleFragment(t *testing.T, json string) string {
	t.Helper()
	return "#" + base64.StdEncoding.EncodeToString([]byte(json))
}

var longToken = "0.A" + strings.Repeat("x", 240)

func TestDecodeFragment(t *testing.T) {
	tests := []struct {
		name       string
		fragment   string
		wantErr    bool
		restricted bool
		cid        string
	}{
		{"string itre", appleFragment(t, `{"cid":"abc","itre":"1","musicUserToken":"`+longToken+`"}`), false, true, "abc"},
		{"itre zero", appleFragment(t, `{"cid":"abc","itre":"0","musicUserToken":"`+longToken+`"}`), false, false, "abc"},
		{"bool itre numeric cid", appleFragment(t, `{"cid":42,"itre":true,"musicUserToken":"`+longToken+`"}`), false, true, "42"},
		{"without hash prefix", strings.TrimPrefix(appleFragment(t, `{"musicUserToken":"`+longToken+`"}`), "#"), false, false, ""},
		{"missing token", appleFragment(t, `{"cid":"abc","itre":"0","pad":"`+strings.Repeat("y", 200)+`"}`), true, false, ""},
		{"too short", appleFragment(t, `{"musicUserToken":"x"}`), true, false, ""},
		{"not base64 payload", "#" + strings.Repeat("!", 250), true, false, ""},
		{"empty", "", true, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DecodeFragment(tt.fragment)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("want error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.MusicUserToken != longToken || got.Restricted != tt.restricted || got.CID != tt.cid {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestAuthorizeURLCarriesTokenAndReferrer(t *testing.T) {
	u, err := url.Parse(AuthorizeURL("DEV", "Sona", "", "http://127.0.0.1:9/auth/n/callback"))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("referrer") != "http://127.0.0.1:9/auth/n/callback" {
		t.Fatalf("referrer = %q", q.Get("referrer"))
	}
	info, err := base64.StdEncoding.DecodeString(q.Get("a"))
	if err != nil || !strings.Contains(string(info), `"thirdPartyToken":"DEV"`) {
		t.Fatalf("a = %s (%v)", info, err)
	}
}

func newSession(t *testing.T) *Session {
	t.Helper()
	return newSessionTimeout(t, time.Minute)
}

func newSessionTimeout(t *testing.T, timeout time.Duration) *Session {
	t.Helper()
	s, err := Start("DEV", Options{Timeout: timeout})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func host(s *Session) string { return strings.TrimPrefix(s.origin, "http://") }

// start opens a new start link through the handler, as the browser Sona
// opened does.
func start(s *Session) *httptest.ResponseRecorder { return open(s, s.StartURL()) }

// open GETs link through the handler.
func open(s *Session, link string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, link, nil)
	req.Host = host(s)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

// startCookieOf returns the attempt cookie a /start response set.
func startCookieOf(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == startCookie {
			return c
		}
	}
	t.Fatalf("start set no %s cookie (status %d, Set-Cookie %q)", startCookie, rec.Code, rec.Header().Values("Set-Cookie"))
	return nil
}

func complete(s *Session, origin, host, fragment string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	body := `{"fragment":"` + fragment + `"}`
	req := httptest.NewRequest(http.MethodPost, s.origin+"/auth/"+s.nonce+"/complete", strings.NewReader(body))
	req.Host = host
	req.Header.Set("Origin", origin)
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

// pending fails the test if the attempt has ended.
func pending(t *testing.T, s *Session) {
	t.Helper()
	select {
	case r := <-s.result:
		t.Fatalf("attempt completed with %v", r)
	case err := <-s.failed:
		t.Fatalf("attempt failed: %v", err)
	default:
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.overLocked() {
		t.Fatal("attempt ended")
	}
}

func TestCompleteDeliversOnceAndRejectsReplays(t *testing.T) {
	s := newSession(t)
	frag := appleFragment(t, `{"cid":"c","itre":"0","musicUserToken":"`+longToken+`"}`)
	key := startCookieOf(t, start(s))

	rec := complete(s, s.origin, host(s), frag, key)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("first complete: %d %s", rec.Code, rec.Body)
	}
	if c := startCookieOf(t, rec); c.MaxAge >= 0 {
		t.Fatalf("complete should delete the cookie, got Max-Age %d", c.MaxAge)
	}
	if rec := complete(s, s.origin, host(s), frag, key); rec.Code != http.StatusGone {
		t.Fatalf("replay: want 410, got %d", rec.Code)
	}
	res, err := s.Wait(context.Background())
	if err != nil || res.MusicUserToken != longToken {
		t.Fatalf("Wait = %v, %v", res, err)
	}
}

func TestStartCookieBindsTheAttempt(t *testing.T) {
	s := newSession(t)
	rec := start(s)
	c := startCookieOf(t, rec)
	if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Domain != "" || c.Secure {
		t.Fatalf("cookie attributes: %q", rec.Header().Get("Set-Cookie"))
	}
	if c.Path != "/auth/"+s.nonce+"/" {
		t.Fatalf("cookie path %q", c.Path)
	}
	if c.MaxAge <= 0 || c.MaxAge > 60 {
		t.Fatalf("cookie Max-Age %d, want within the 60 s attempt", c.MaxAge)
	}
	if len(c.Value) < 43 || strings.Contains(s.StartURL(), c.Value) || c.Value == s.nonce {
		t.Fatalf("cookie value must be a fresh 256-bit secret outside the URL: %q", c.Value)
	}
}

func TestCompleteRequiresTheStartCookie(t *testing.T) {
	s := newSession(t)
	frag := appleFragment(t, `{"musicUserToken":"`+longToken+`"}`)
	key := startCookieOf(t, start(s))

	// Another client knows the URL (nonce) but never got the cookie.
	if rec := complete(s, s.origin, host(s), frag); rec.Code != http.StatusForbidden {
		t.Fatalf("no cookie: want 403, got %d", rec.Code)
	}
	wrong := *key
	wrong.Value = strings.Repeat("A", len(key.Value))
	if rec := complete(s, s.origin, host(s), frag, &wrong); rec.Code != http.StatusForbidden {
		t.Fatalf("wrong cookie: want 403, got %d", rec.Code)
	}
	short := *key
	short.Value = key.Value[:10]
	if rec := complete(s, s.origin, host(s), frag, &short); rec.Code != http.StatusForbidden {
		t.Fatalf("truncated cookie: want 403, got %d", rec.Code)
	}
	pending(t, s)

	// The browser that started still completes, even next to a stray cookie.
	if rec := complete(s, s.origin, host(s), frag, &wrong, key); rec.Code != http.StatusNoContent {
		t.Fatalf("right cookie: want 204, got %d %s", rec.Code, rec.Body)
	}
	if res, err := s.Wait(context.Background()); err != nil || res.MusicUserToken != longToken {
		t.Fatalf("Wait = %v, %v", res, err)
	}
}

func TestCompleteBeforeStartIsRefused(t *testing.T) {
	s := newSession(t)
	frag := appleFragment(t, `{"musicUserToken":"`+longToken+`"}`)
	if rec := complete(s, s.origin, host(s), frag); rec.Code != http.StatusForbidden {
		t.Fatalf("complete before start: want 403, got %d", rec.Code)
	}
	pending(t, s)
}

func TestStartLinkWorksOnceAndAReplayEndsTheAttempt(t *testing.T) {
	s := newSession(t)
	frag := appleFragment(t, `{"musicUserToken":"`+longToken+`"}`)
	link := s.StartURL()
	first := startCookieOf(t, open(s, link))

	rec := open(s, link)
	if rec.Code != http.StatusGone {
		t.Fatalf("replayed start: want 410, got %d", rec.Code)
	}
	if len(rec.Result().Cookies()) != 0 || strings.Contains(rec.Body.String(), "authorize.music.apple.com") {
		t.Fatalf("replayed start must get neither a cookie nor the authorize URL: %q %s", rec.Header().Values("Set-Cookie"), rec.Body)
	}
	if _, err := s.Wait(context.Background()); !errors.Is(err, ErrStartReused) {
		t.Fatalf("Wait after a replayed start: %v", err)
	}
	// Whoever holds the first cookie can no longer complete.
	if rec := complete(s, s.origin, host(s), frag, first); rec.Code != http.StatusGone {
		t.Fatalf("complete after the attempt ended: want 410, got %d", rec.Code)
	}
	if rec := start(s); rec.Code != http.StatusGone {
		t.Fatalf("fresh link after the attempt ended: want 410, got %d", rec.Code)
	}
}

// TestEachLinkWorksOnce covers "open the page again" and "copy link": every
// link StartURL issues works once, and a start without a ticket Sona issued
// is not found and leaves the attempt alone.
func TestEachLinkWorksOnce(t *testing.T) {
	s := newSession(t)
	frag := appleFragment(t, `{"musicUserToken":"`+longToken+`"}`)
	a, b := s.StartURL(), s.StartURL()
	if a == b {
		t.Fatal("StartURL returned the same link twice")
	}
	startCookieOf(t, open(s, a))
	second := startCookieOf(t, open(s, b))

	base := strings.Split(a, "?")[0]
	for _, link := range []string{base, base + "?t=", base + "?t=guess"} {
		if rec := open(s, link); rec.Code != http.StatusNotFound || len(rec.Result().Cookies()) != 0 {
			t.Fatalf("%s: want 404 without a cookie, got %d", link, rec.Code)
		}
	}
	pending(t, s)
	if rec := complete(s, s.origin, host(s), frag, second); rec.Code != http.StatusNoContent {
		t.Fatalf("complete from the second browser: %d %s", rec.Code, rec.Body)
	}
}

func TestTimeoutEndsTheAttempt(t *testing.T) {
	s := newSessionTimeout(t, 100*time.Millisecond)
	frag := appleFragment(t, `{"musicUserToken":"`+longToken+`"}`)
	key := startCookieOf(t, start(s))
	if key.MaxAge != 1 {
		t.Fatalf("Max-Age %d, want the remaining second", key.MaxAge)
	}
	if _, err := s.Wait(context.Background()); !errors.Is(err, ErrTimeout) {
		t.Fatalf("Wait: %v", err)
	}
	if rec := complete(s, s.origin, host(s), frag, key); rec.Code != http.StatusGone {
		t.Fatalf("complete after timeout: want 410, got %d", rec.Code)
	}
	if rec := start(s); rec.Code != http.StatusGone || len(rec.Result().Cookies()) != 0 {
		t.Fatalf("start after timeout: want 410 and no cookie, got %d", rec.Code)
	}
	if _, err := http.Get(s.StartURL()); err == nil {
		t.Fatal("server still listening after timeout")
	}
}

// TestHappyPathWithCookieJar runs the flow over real HTTP: a client with a
// cookie jar opens /start, follows "Apple's" redirect to /callback and
// posts the fragment the way the callback page does. A second client
// without the jar, racing with the same URL, cannot complete.
func TestHappyPathWithCookieJar(t *testing.T) {
	s := newSession(t)
	frag := appleFragment(t, `{"cid":"c","itre":"1","musicUserToken":"`+longToken+`"}`)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	browser := &http.Client{Jar: jar}
	resp, err := browser.Get(s.StartURL())
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("start: %d", resp.StatusCode)
	}
	resp, err = browser.Get(s.callbackURL())
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	post := func(c *http.Client) int {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, s.origin+"/auth/"+s.nonce+"/complete", strings.NewReader(`{"fragment":"`+frag+`"}`))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Origin", s.origin)
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}
	if code := post(http.DefaultClient); code != http.StatusForbidden {
		t.Fatalf("racing client without the cookie: want 403, got %d", code)
	}
	if code := post(browser); code != http.StatusNoContent {
		t.Fatalf("browser: want 204, got %d", code)
	}
	res, err := s.Wait(context.Background())
	if err != nil || res.MusicUserToken != longToken || !res.Restricted {
		t.Fatalf("Wait = %v, %v", res, err)
	}
	u, _ := url.Parse(s.callbackURL())
	if left := jar.Cookies(u); len(left) != 0 {
		t.Fatalf("cookie left in the jar after completion: %v", left)
	}
}

func TestHandlerRejectsForeignRequests(t *testing.T) {
	s := newSession(t)
	frag := appleFragment(t, `{"musicUserToken":"`+longToken+`"}`)
	key := startCookieOf(t, start(s))

	if rec := complete(s, "https://evil.example", host(s), frag, key); rec.Code != http.StatusForbidden {
		t.Fatalf("cross-origin POST: want 403, got %d", rec.Code)
	}
	if rec := complete(s, s.origin, "evil.example:80", frag, key); rec.Code != http.StatusForbidden {
		t.Fatalf("rebinding Host: want 403, got %d", rec.Code)
	}
	req := httptest.NewRequest(http.MethodGet, s.origin+"/auth/wrongnonce/start", nil)
	req.Host = host(s)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("wrong nonce: want 404, got %d", rec.Code)
	}
	if rec := complete(s, s.origin, host(s), "#garbage", key); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad payload: want 400, got %d", rec.Code)
	}
	// None of the rejected requests may complete or end the session.
	pending(t, s)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := s.Wait(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Wait after rejected requests: %v", err)
	}
}

func TestStartPageRedirectsToAppleWithCallbackReferrer(t *testing.T) {
	s := newSession(t)
	rec := start(s)
	body := rec.Body.String()
	if rec.Code != http.StatusOK || rec.Header().Get("Referrer-Policy") != "unsafe-url" {
		t.Fatalf("start: %d policy=%q", rec.Code, rec.Header().Get("Referrer-Policy"))
	}
	wantRef := url.QueryEscape(s.callbackURL())
	if !strings.Contains(body, "authorize.music.apple.com") || !strings.Contains(body, wantRef) {
		t.Fatalf("start page missing authorize URL with callback referrer:\n%s", body)
	}
}
