package services

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// noRedirect is a browser that shows what a link answers instead of
// following it.
var noRedirect = &http.Client{
	Timeout:       5 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// browse is the system's "open this address": it records the address Sona
// handed over (what the browser's command line shows), loads it and keeps
// the page it redirects to.
func (se *scrobbleEnv) browse(u string) error {
	se.argv <- u
	resp, err := noRedirect.Get(u)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	se.opened <- resp.Header.Get("Location")
	return nil
}

// The approval token must not be in the address Sona hands the system to
// open, which every user of the computer can read in the browser's command
// line; the browser still ends on Last.fm's page with it.
func TestLastFMApprovalTokenIsNotOnTheBrowsersCommandLine(t *testing.T) {
	se := newScrobbleEnv(t)
	if _, err := se.s.ConnectLastFM(t.Context()); err != nil {
		t.Fatal(err)
	}
	argv, page := <-se.argv, <-se.opened
	if strings.Contains(argv, "tok") || strings.Contains(argv, "api_key") || strings.Contains(argv, "last.fm") {
		t.Errorf("the address handed to the system carries the approval: %s", argv)
	}
	if !strings.HasPrefix(argv, "http://127.0.0.1:") {
		t.Errorf("the address handed to the system is not a loopback link: %s", argv)
	}
	if !strings.HasPrefix(page, "https://www.last.fm/api/auth/") || !strings.Contains(page, "token=tok") {
		t.Errorf("the browser did not end on the approval page: %s", page)
	}
}

// Whoever reads the link from the command line and loads it after the
// browser has gets nothing, and the link is no use with a changed name,
// method or host.
func TestApprovalLinkWorksOnce(t *testing.T) {
	const page = "https://www.last.fm/api/auth/?api_key=k&token=secret-token"
	l, err := newApprovalLink(page)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()

	get := func(method, url, host string) (int, string) {
		t.Helper()
		req, err := http.NewRequest(method, url, nil)
		if err != nil {
			t.Fatal(err)
		}
		if host != "" {
			req.Host = host
		}
		resp, err := noRedirect.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode, resp.Header.Get("Location")
	}

	base := "http://" + l.addr
	for name, tc := range map[string]struct{ method, url, host string }{
		"another path": {http.MethodGet, base + "/lastfm/guess", ""},
		"the root":     {http.MethodGet, base + "/", ""},
		"a POST":       {http.MethodPost, l.url, ""},
		"a HEAD":       {http.MethodHead, l.url, ""},
		"a rebound Host": {http.MethodGet, l.url, "evil.example:" +
			strings.TrimPrefix(l.addr, "127.0.0.1:")},
	} {
		if code, loc := get(tc.method, tc.url, tc.host); code != http.StatusNotFound || loc != "" {
			t.Errorf("%s: %d %q, want 404", name, code, loc)
		}
	}
	// None of those used the link.
	if code, loc := get(http.MethodGet, l.url, ""); code != http.StatusFound || loc != page {
		t.Fatalf("first load: %d %q, want 302 to the page", code, loc)
	}
	if code, loc := get(http.MethodGet, l.url, ""); code != http.StatusGone || loc != "" {
		t.Errorf("second load: %d %q, want 410 and no page", code, loc)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := noRedirect.Get(l.url); err == nil {
		t.Error("the link still answers after Close")
	}
}

// The link ends with the approval, whatever way it ends.
func TestApprovalLinkIsClosedWhenTheApprovalEnds(t *testing.T) {
	se := newScrobbleEnv(t)
	if _, err := se.s.ConnectLastFM(t.Context()); err != nil {
		t.Fatal(err)
	}
	argv := <-se.argv
	if _, err := noRedirect.Get(argv); err == nil {
		t.Errorf("the link %s still answers after the connection", argv)
	}
}
