package update

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// github is a fake GitHub that records the paths it was asked for; routes
// answers by path ("*" answers the rest) and anything else is a 404.
type github struct {
	srv  *httptest.Server
	mu   sync.Mutex
	hits []string
}

func newGitHub(t *testing.T, routes map[string]http.HandlerFunc) *github {
	t.Helper()
	g := &github{}
	g.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		g.hits = append(g.hits, r.URL.Path)
		g.mu.Unlock()
		h, ok := routes[r.URL.Path]
		if !ok {
			h, ok = routes["*"]
		}
		if ok {
			h(w, r)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(g.srv.Close)
	return g
}

// count is how many requests were for paths under prefix.
func (g *github) count(prefix string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	n := 0
	for _, p := range g.hits {
		if strings.HasPrefix(p, prefix) {
			n++
		}
	}
	return n
}

// body answers every request with status, headers (name, value pairs) and
// body.
func body(status int, text string, headers ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		for i := 0; i+1 < len(headers); i += 2 {
			w.Header().Set(headers[i], headers[i+1])
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(text))
	}
}

// atomOf is GitHub's releases.atom (its shape on 2026-11-27) with an entry
// for each release page in hrefs, newest first.
func atomOf(hrefs ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom" xmlns:media="http://search.yahoo.com/mrss/" xml:lang="en-US">
  <id>tag:github.com,2008:https://github.com/coredev-uk/sona-go/releases</id>
  <link type="text/html" rel="alternate" href="https://github.com/coredev-uk/sona-go/releases"/>
  <link type="application/atom+xml" rel="self" href="https://github.com/coredev-uk/sona-go/releases.atom"/>
  <title>Release notes from sona-go</title>
  <updated>2026-11-27T16:55:28Z</updated>
`)
	for _, h := range hrefs {
		fmt.Fprintf(&b, `  <entry>
    <id>tag:github.com,2008:Repository/1441430256/%s</id>
    <updated>2026-11-27T17:10:32Z</updated>
    <link rel="alternate" type="text/html" href="%s"/>
    <title>Sona</title>
    <content type="html">&lt;p&gt;Test build &lt;a href=&quot;x&quot;&gt;&lt;tt&gt;839a2a0&lt;/tt&gt;&lt;/a&gt;.&lt;/p&gt;</content>
    <author>
      <name>bot-cecil</name>
    </author>
  </entry>
`, h[strings.LastIndex(h, "/")+1:], h)
	}
	b.WriteString("</feed>\n")
	return b.String()
}

// GitHub's limit for this network is a typed error with the reset, wherever
// the update code meets it: the API, the Atom feed, a manifest and
// release.json. Anything else that is refused, or fails, is not.
func TestGitHubsLimitIsATypedError(t *testing.T) {
	// The reset names a whole second, as GitHub's x-ratelimit-reset does
	// (issue 728's was 1795803306, 18:15:06 UTC).
	resetSecs := time.Now().Add(2 * time.Hour).Unix()
	future := time.Date(2036, 1, 2, 3, 4, 5, 0, time.UTC)
	for _, tc := range []struct {
		name    string
		status  int
		headers []string
		exact   time.Time     // the reset when the answer fixes it
		after   time.Duration // else the reset is this long after the answer
	}{
		{name: "403 with none left", status: 403, headers: []string{"X-RateLimit-Limit", "60", "X-RateLimit-Remaining", "0", "X-RateLimit-Reset", strconv.FormatInt(resetSecs, 10)}, exact: time.Unix(resetSecs, 0)},
		{name: "429 with none left", status: 429, headers: []string{"X-RateLimit-Remaining", "0", "X-RateLimit-Reset", strconv.FormatInt(resetSecs, 10)}, exact: time.Unix(resetSecs, 0)},
		{name: "Retry-After seconds", status: 429, headers: []string{"Retry-After", "120"}, after: 120 * time.Second},
		{name: "Retry-After date", status: 403, headers: []string{"Retry-After", future.Format(http.TimeFormat)}, exact: future},
		{name: "the later of both", status: 403, headers: []string{"X-RateLimit-Remaining", "0", "X-RateLimit-Reset", strconv.FormatInt(resetSecs, 10), "Retry-After", "30"}, exact: time.Unix(resetSecs, 0)},
		{name: "none left and no reset", status: 403, headers: []string{"X-RateLimit-Remaining", "0"}, after: time.Hour},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := newGitHub(t, map[string]http.HandlerFunc{"*": body(tc.status, `{"message":"API rate limit exceeded"}`, tc.headers...)})
			before := time.Now()
			for what, call := range map[string]func() error{
				"the API": func() error {
					_, err := Checker{API: g.srv.URL, Web: g.srv.URL}.Latest(context.Background(), false)
					return err
				},
				"release.json": func() error {
					_, err := Checker{API: g.srv.URL, Web: g.srv.URL, Key: testKey(t)}.Version(context.Background(), "0.8.0")
					return err
				},
				"a manifest": func() error {
					_, _, err := Feed{Web: g.srv.URL, Key: testKey(t)}.Manifest(context.Background(), "darwin-arm64", false)
					return err
				},
				"the Atom feed": func() error {
					_, _, err := Feed{Web: g.srv.URL, Key: testKey(t)}.Manifest(context.Background(), "darwin-arm64", true)
					return err
				},
			} {
				var rl *RateLimitError
				err := call()
				if !errors.As(err, &rl) {
					t.Fatalf("%s: err = %v, want a *RateLimitError", what, err)
				}
				if tc.exact.IsZero() {
					if rl.Reset.Before(before.Add(tc.after)) || rl.Reset.After(time.Now().Add(tc.after)) {
						t.Errorf("%s: Reset = %v, want %v after the answer", what, rl.Reset, tc.after)
					}
				} else if !rl.Reset.Equal(tc.exact) {
					t.Errorf("%s: Reset = %v, want %v", what, rl.Reset, tc.exact)
				}
				if errors.Is(err, ErrNoRelease) {
					t.Errorf("%s: a limit is not a missing release", what)
				}
			}
			if n := g.count("/repos/"); n != 1 {
				t.Errorf("the API was asked %d times (once, by the Checker without a key), want 1: no fallback from a limit on release.json", n)
			}
		})
	}
}

func TestOtherRefusalsAreNotTheLimit(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		headers []string
	}{
		{"403 with calls left", 403, []string{"X-RateLimit-Remaining", "59", "X-RateLimit-Reset", "1795803306"}},
		{"403 with nothing said", 403, nil},
		{"429 with nothing said", 429, nil},
		{"500 with none left", 500, []string{"X-RateLimit-Remaining", "0"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := newGitHub(t, map[string]http.HandlerFunc{"*": body(tc.status, "no", tc.headers...)})
			_, err := Checker{API: g.srv.URL}.Latest(context.Background(), false)
			var rl *RateLimitError
			if err == nil || errors.As(err, &rl) || errors.Is(err, ErrNoRelease) {
				t.Fatalf("err = %v, want a plain failure", err)
			}
		})
	}
}

func TestRateLimitErrorSaysWhen(t *testing.T) {
	err := error(&RateLimitError{Reset: time.Date(2026, 11, 27, 18, 15, 6, 0, time.UTC)})
	if got := err.Error(); !strings.Contains(got, "2026-11-27T18:15:06Z") {
		t.Fatalf("Error() = %q", got)
	}
}
