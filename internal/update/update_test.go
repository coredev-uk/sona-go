package update

import (
	"context"
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

func TestLatestSendsNothingAboutTheUser(t *testing.T) {
	var got *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Clone(context.Background())
		_, _ = w.Write([]byte(`{"tag_name":"v0.3.1","name":"Sona 0.3.1","html_url":"https://github.com/coredev-uk/sona-go/releases/tag/v0.3.1","published_at":"2026-10-01T12:00:00Z"}`))
	}))
	defer srv.Close()

	rel, err := Checker{API: srv.URL}.Latest(context.Background())
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
			rel, err := Checker{API: srv.URL}.Latest(context.Background())
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
