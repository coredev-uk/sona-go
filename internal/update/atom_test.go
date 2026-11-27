package update

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

// The newest prerelease comes from releases.atom, with no call to the API,
// and the manifest it leads to is still the signed one.
func TestFeedPrereleaseNeverCallsTheAPI(t *testing.T) {
	r := newRelease(t, "0.3.0-rc.2", goodSig)
	m, _, err := r.feed().Manifest(context.Background(), "darwin-arm64", true)
	if err != nil || m.Version != "0.3.0-rc.2" {
		t.Fatalf("manifest %+v, err %v", m, err)
	}
	if r.hits["/coredev-uk/sona-go/releases.atom"] != 1 || r.hits["/coredev-uk/sona-go/releases/download/v0.3.0-rc.2/update-darwin-arm64.json"] != 1 {
		t.Fatalf("requests = %v", r.hits)
	}
	for p := range r.hits {
		if strings.HasPrefix(p, "/repos/") {
			t.Fatalf("asked the API: %s", p)
		}
	}
}

func TestNewestTagFromTheAtomFeed(t *testing.T) {
	const repo = "/coredev-uk/sona-go/releases/tag/"
	var g *github
	feed := func(entries ...string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if got := r.Header.Get("Accept"); got != "application/atom+xml" {
				t.Errorf("Accept = %q", got)
			}
			hrefs := make([]string, len(entries))
			for i, e := range entries {
				hrefs[i] = strings.Replace(e, "BASE", g.srv.URL, 1)
			}
			_, _ = w.Write([]byte(atomOf(hrefs...)))
		}
	}
	for _, tc := range []struct {
		name    string
		serve   http.HandlerFunc
		want    string
		wantErr error // errAny: some failure that is not ErrNoRelease
	}{
		{"newest first", feed("BASE"+repo+"v0.8.0-rc.4", "BASE"+repo+"v0.8.0-rc.3", "BASE"+repo+"v0.7.0"), "v0.8.0-rc.4", nil},
		{"the highest version, wherever it is listed", feed("BASE"+repo+"v0.7.0-rc.10", "BASE"+repo+"v0.7.0", "BASE"+repo+"v0.7.0-rc.9"), "v0.7.0", nil},
		{"a stable release above its rcs", feed("BASE"+repo+"v0.8.0-rc.4", "BASE"+repo+"v0.8.0"), "v0.8.0", nil},
		{"another repository's release", feed("BASE/evil/sona-go/releases/tag/v9.9.9", "BASE"+repo+"v0.7.0"), "v0.7.0", nil},
		{"a tag that is not a version", feed("BASE"+repo+"nightly", "BASE"+repo+"v0.7.0"), "v0.7.0", nil},
		{"a tag that leaves the folder", feed("BASE"+repo+"v9.9.9%2F..%2F..", "BASE"+repo+"v0.7.0"), "v0.7.0", nil},
		{"not a release page", feed("BASE/coredev-uk/sona-go/releases", "BASE"+repo+"v0.7.0"), "v0.7.0", nil},
		{"no releases", feed(), "", ErrNoRelease},
		{"only foreign releases", feed("BASE/evil/sona-go/releases/tag/v9.9.9"), "", ErrNoRelease},
		{"a 404", body(404, "Not Found"), "", ErrNoRelease},
		{"not XML", ok("<<<"), "", errAny},
		{"not a feed", ok(`<html xmlns="http://www.w3.org/1999/xhtml"><body>Sign in</body></html>`), "", errAny},
		{"cut off", func(w http.ResponseWriter, _ *http.Request) {
			full := atomOf("https://github.com/coredev-uk/sona-go/releases/tag/v0.7.0")
			_, _ = w.Write([]byte(full[:len(full)-30]))
		}, "", errAny},
		{"larger than the cap", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(strings.Repeat(" ", maxAtom+1)))
		}, "", errAny},
		{"an error page", body(500, "oops"), "", errAny},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g = newGitHub(t, map[string]http.HandlerFunc{atomPath: tc.serve})
			got, err := newestTag(context.Background(), nil, g.srv.URL, Repo)
			switch {
			case tc.wantErr == errAny:
				if err == nil || errors.Is(err, ErrNoRelease) {
					t.Fatalf("tag %q, err %v, want a failure", got, err)
				}
			case tc.wantErr != nil:
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("tag %q, err %v, want %v", got, err, tc.wantErr)
				}
			case err != nil || got != tc.want:
				t.Fatalf("tag %q, err %v, want %q", got, err, tc.want)
			}
		})
	}
}
