package update

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewer(t *testing.T) {
	for _, tc := range []struct {
		latest, current string
		want            bool
	}{
		{"v0.2.0", "0.1.0", true},
		{"0.1.0", "0.1.0", false},
		{"v0.1.0", "v0.2.0", false},
		{"0.10.0", "0.9.9", true}, // numeric, not lexical
		{"1.0.0", "1.0.0-rc.1", true},
		{"1.0.0-rc.1", "1.0.0", false},
		{"1.0.0-rc.2", "1.0.0-rc.1", true},
		{"1.0.0-rc.10", "1.0.0-rc.9", true},
		{"1.0.0-beta", "1.0.0-alpha.1", true},
		{"1.0.0-alpha.1", "1.0.0-alpha", true},
		{"1.0.0-alpha", "1.0.0-1", true}, // alphanumeric above numeric
		{"0.2.0+build.5", "0.1.0", true},
		{"0.1.0+build.5", "0.1.0", false},
		{"0.2.0", "dev", false}, // dev builds are never offered updates
		{"garbage", "0.1.0", false},
		{"0.2", "0.1.0", false},
		{"01.2.0", "0.1.0", false},
	} {
		if got := Newer(tc.latest, tc.current); got != tc.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", tc.latest, tc.current, got, tc.want)
		}
	}
}

// Version reads the release of one tag, a prerelease too, and asks nothing
// for a version that is not one.
func TestVersionReadsThatTagsRelease(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		if r.URL.Path != "/repos/coredev-uk/sona-go/releases/tags/v0.3.0-rc.1" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"tag_name":"v0.3.0-rc.1","prerelease":true,"body":"### Fixes\n\n- a\n\n` + InstallNotesMarker + `\ninstall"}`))
	}))
	defer srv.Close()
	c := Checker{API: srv.URL}
	rel, err := c.Version(context.Background(), "v0.3.0-rc.1")
	if err != nil || rel.Version != "0.3.0-rc.1" || rel.Notes != "### Fixes\n\n- a" {
		t.Fatalf("Version = %+v, %v", rel, err)
	}
	if _, err := c.Version(context.Background(), "0.4.0"); !errors.Is(err, ErrNoRelease) {
		t.Errorf("a version with no release: %v", err)
	}
	path = ""
	if _, err := c.Version(context.Background(), "../latest"); !errors.Is(err, ErrNoRelease) || path != "" {
		t.Errorf("a version that is not one: %v (asked %q)", err, path)
	}
}

func TestLatestSendsNothingAboutTheUser(t *testing.T) {
	var got *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Clone(context.Background())
		_, _ = w.Write([]byte(`{"tag_name":"v0.3.1","name":"Sona 0.3.1","html_url":"https://github.com/coredev-uk/sona-go/releases/tag/v0.3.1","published_at":"2026-10-01T12:00:00Z"}`))
	}))
	defer srv.Close()

	rel, err := Checker{API: srv.URL}.Latest(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if rel.Version != "0.3.1" || rel.URL != "https://github.com/coredev-uk/sona-go/releases/tag/v0.3.1" || rel.Published.IsZero() {
		t.Fatalf("release = %+v", rel)
	}
	if got.URL.Path != "/repos/coredev-uk/sona-go/releases/latest" || got.URL.RawQuery != "" {
		t.Fatalf("request = %s", got.URL)
	}
	if ua := got.Header.Get("User-Agent"); ua != userAgent {
		t.Fatalf("User-Agent = %q", ua)
	}
	for k := range got.Header {
		switch http.CanonicalHeaderKey(k) {
		case "Accept", "X-Github-Api-Version", "User-Agent", "Accept-Encoding":
		default:
			t.Errorf("unexpected header %s: %q", k, got.Header.Get(k))
		}
	}
}

// The notes are the release body's changelog: the install instructions the
// workflow appends after the marker are the release page's, not the notes'.
func TestLatestNotesStopAtTheInstallInstructions(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
	}{
		{"changelog and install notes", "## Fixes\n\n- a (#1)\n\n\n" + InstallNotesMarker + "\n## Installing\n\nRun it.\n", "## Fixes\n\n- a (#1)"},
		{"changelog only", "  ## Fixes\n\n- a (#1)\n", "## Fixes\n\n- a (#1)"},
		{"install notes only", InstallNotesMarker + "\n## Installing", ""},
		{"no body", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(map[string]any{"tag_name": "v0.3.1", "body": tc.body})
			if err != nil {
				t.Fatal(err)
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(body) }))
			defer srv.Close()
			rel, err := Checker{API: srv.URL}.Latest(context.Background(), false)
			if err != nil {
				t.Fatal(err)
			}
			if rel.Notes != tc.want {
				t.Fatalf("Notes = %q, want %q", rel.Notes, tc.want)
			}
		})
	}
}

func TestWithoutVersionHeading(t *testing.T) {
	for _, tc := range []struct {
		name, notes, version, want string
	}{
		{"release-please", "## [0.8.0](https://github.com/coredev-uk/sona-go/compare/v0.7.0...v0.8.0) (2026-11-27)\n\n\n### Features\n\n* a\n", "0.8.0", "### Features\n\n* a\n"},
		{"bare version", "# 0.8.0 (2026-11-27)\n\nBody", "0.8.0", "Body"},
		{"bracket only", "### [0.8.0]\nBody", "0.8.0", "Body"},
		{"leading v", "## [v0.8.0](x)\nBody", "0.8.0", "Body"},
		{"tag as version", "## [0.8.0](x)\nBody", "v0.8.0", "Body"},
		{"leading blank lines", "\n\n  \n## [0.8.0](x)\r\n\r\nBody", "0.8.0", "Body"},
		{"crlf", "## [0.8.0](x)\r\n\r\nBody\r\n", "0.8.0", "Body\r\n"},
		{"only the heading", "## [0.8.0](x)", "0.8.0", ""},
		{"another version", "## [0.7.0](x)\n\nBody", "0.8.0", "## [0.7.0](x)\n\nBody"},
		{"a longer version", "## [0.8.0-rc.1](x)\n\nBody", "0.8.0", "## [0.8.0-rc.1](x)\n\nBody"},
		{"a longer number", "## [0.8.01](x)\n\nBody", "0.8.0", "## [0.8.01](x)\n\nBody"},
		{"another heading", "### Fixes\n\n* a", "0.8.0", "### Fixes\n\n* a"},
		{"not the first line", "Intro\n\n## [0.8.0](x)\n\nBody", "0.8.0", "Intro\n\n## [0.8.0](x)\n\nBody"},
		{"not a heading", "[0.8.0](x) is out\n\nBody", "0.8.0", "[0.8.0](x) is out\n\nBody"},
		{"seven hashes", "####### 0.8.0\nBody", "0.8.0", "####### 0.8.0\nBody"},
		{"no space", "##0.8.0\nBody", "0.8.0", "##0.8.0\nBody"},
		{"no version", "## [0.8.0](x)\nBody", "", "## [0.8.0](x)\nBody"},
		{"empty", "", "0.8.0", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := WithoutVersionHeading(tc.notes, tc.version); got != tc.want {
				t.Errorf("WithoutVersionHeading(%q, %q) = %q, want %q", tc.notes, tc.version, got, tc.want)
			}
		})
	}
}

func TestLatestResponses(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		body    string
		wantErr error
		wantURL string
	}{
		{name: "no release", status: 404, body: `{"message":"Not Found"}`, wantErr: ErrNoRelease},
		{name: "prerelease", status: 200, body: `{"tag_name":"v1.0.0-rc.1","prerelease":true}`, wantErr: ErrNoRelease},
		{
			name: "foreign page", status: 200, body: `{"tag_name":"v1.2.3","html_url":"https://evil.example/x"}`,
			wantURL: "https://github.com/coredev-uk/sona-go/releases/tag/v1.2.3",
		},
		{name: "rate limited", status: 403, body: `{"message":"rate limit"}`, wantErr: errAny},
		{name: "bad tag", status: 200, body: `{"tag_name":"nightly"}`, wantErr: errAny},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			rel, err := Checker{API: srv.URL}.Latest(context.Background(), false)
			switch {
			case tc.wantErr == errAny:
				if err == nil || errors.Is(err, ErrNoRelease) {
					t.Fatalf("err = %v, want a failure", err)
				}
			case tc.wantErr != nil:
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
			default:
				if err != nil || rel.URL != tc.wantURL {
					t.Fatalf("got %+v, %v", rel, err)
				}
			}
		})
	}
}

var errAny = errors.New("any error")
