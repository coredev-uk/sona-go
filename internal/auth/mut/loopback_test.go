package mut

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
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
	s, err := Start("DEV", Options{Timeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func complete(s *Session, origin, host, fragment string) *httptest.ResponseRecorder {
	body := `{"fragment":"` + fragment + `"}`
	req := httptest.NewRequest(http.MethodPost, s.origin+"/auth/"+s.nonce+"/complete", strings.NewReader(body))
	req.Host = host
	req.Header.Set("Origin", origin)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func TestCompleteDeliversOnceAndRejectsReplays(t *testing.T) {
	s := newSession(t)
	host := strings.TrimPrefix(s.origin, "http://")
	frag := appleFragment(t, `{"cid":"c","itre":"0","musicUserToken":"`+longToken+`"}`)

	if rec := complete(s, s.origin, host, frag); rec.Code != http.StatusNoContent {
		t.Fatalf("first complete: %d %s", rec.Code, rec.Body)
	}
	if rec := complete(s, s.origin, host, frag); rec.Code != http.StatusGone {
		t.Fatalf("replay: want 410, got %d", rec.Code)
	}
	res, err := s.Wait(context.Background())
	if err != nil || res.MusicUserToken != longToken {
		t.Fatalf("Wait = %v, %v", res, err)
	}
}

func TestHandlerRejectsForeignRequests(t *testing.T) {
	s := newSession(t)
	host := strings.TrimPrefix(s.origin, "http://")
	frag := appleFragment(t, `{"musicUserToken":"`+longToken+`"}`)

	if rec := complete(s, "https://evil.example", host, frag); rec.Code != http.StatusForbidden {
		t.Fatalf("cross-origin POST: want 403, got %d", rec.Code)
	}
	if rec := complete(s, s.origin, "evil.example:80", frag); rec.Code != http.StatusForbidden {
		t.Fatalf("rebinding Host: want 403, got %d", rec.Code)
	}
	req := httptest.NewRequest(http.MethodGet, s.origin+"/auth/wrongnonce/start", nil)
	req.Host = host
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("wrong nonce: want 404, got %d", rec.Code)
	}
	if rec := complete(s, s.origin, host, "#garbage"); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad payload: want 400, got %d", rec.Code)
	}
	// None of the rejected requests may complete the session.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := s.Wait(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Wait after rejected requests: %v", err)
	}
}

func TestStartPageRedirectsToAppleWithCallbackReferrer(t *testing.T) {
	s := newSession(t)
	req := httptest.NewRequest(http.MethodGet, s.StartURL(), nil)
	req.Host = strings.TrimPrefix(s.origin, "http://")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	body := rec.Body.String()
	if rec.Code != http.StatusOK || rec.Header().Get("Referrer-Policy") != "unsafe-url" {
		t.Fatalf("start: %d policy=%q", rec.Code, rec.Header().Get("Referrer-Policy"))
	}
	wantRef := url.QueryEscape(s.callbackURL())
	if !strings.Contains(body, "authorize.music.apple.com") || !strings.Contains(body, wantRef) {
		t.Fatalf("start page missing authorize URL with callback referrer:\n%s", body)
	}
}
