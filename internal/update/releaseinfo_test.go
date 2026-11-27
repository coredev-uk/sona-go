package update

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

// keys is a signing key pair as the build and the release job hold them.
type keys struct {
	pub  string // base64, what Checker.Key and Feed.Key take
	priv ed25519.PrivateKey
}

func newKeys(t *testing.T) keys {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	return keys{pub: base64.StdEncoding.EncodeToString(pub), priv: priv}
}

// testKey is a valid public key that signed nothing.
func testKey(t *testing.T) string { return newKeys(t).pub }

// info is release.json as the release job signs it.
func (k keys) info(t *testing.T, version, notes string) []byte {
	t.Helper()
	r, err := NewReleaseInfo(k.priv, version, time.Date(2026, 11, 27, 18, 0, 0, 0, time.UTC), notes)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

const (
	latestInfo = "/coredev-uk/sona-go/releases/latest/download/release.json"
	apiLatest  = "/repos/coredev-uk/sona-go/releases/latest"
	atomPath   = "/coredev-uk/sona-go/releases.atom"
)

func tagInfo(tag string) string {
	return "/coredev-uk/sona-go/releases/download/" + tag + "/release.json"
}
func apiTag(tag string) string { return "/repos/coredev-uk/sona-go/releases/tags/" + tag }

func ok(text string) http.HandlerFunc { return body(200, text) }

// TestReleaseStatementFormat pins the bytes cmd/releasesign signs for
// release.json, which installs out there must go on checking the same way.
func TestReleaseStatementFormat(t *testing.T) {
	digest := sha256.Sum256([]byte("Notes."))
	want := "sona-release-v1\x000.8.0\x00stable\x002026-11-27T18:00:00Z\x00" + hex.EncodeToString(digest[:])
	if got := string(ReleaseStatement("0.8.0", ChannelStable, "2026-11-27T18:00:00Z", "Notes.")); got != want {
		t.Fatalf("statement = %q, want %q", got, want)
	}
	// It is not any other statement: an archive's signature can't stand for it.
	if string(ReleaseStatement("0.8.0", ChannelStable, "x", "")) == string(Statement("darwin-arm64", "0.8.0", ChannelStable, nil)) {
		t.Fatal("release.json and manifest statements coincide")
	}
}

func TestNewReleaseInfo(t *testing.T) {
	k := newKeys(t)
	if _, err := NewReleaseInfo(k.priv, "latest", time.Now(), ""); err == nil {
		t.Error("signed a version that is not one")
	}
	r, err := NewReleaseInfo(k.priv, "v0.8.0-rc.1", time.Date(2026, 11, 27, 20, 0, 0, 0, time.FixedZone("x", 2*3600)), "n")
	if err != nil || r.Version != "0.8.0-rc.1" || r.Published != "2026-11-27T18:00:00Z" {
		t.Fatalf("release info = %+v, %v", r, err)
	}
}

// Latest reads the signed release.json of the latest release: its version,
// publication, notes and the release's page, without a call to the API.
func TestLatestReadsTheSignedReleaseInfo(t *testing.T) {
	k := newKeys(t)
	g := newGitHub(t, map[string]http.HandlerFunc{latestInfo: func(w http.ResponseWriter, r *http.Request) {
		for name := range r.Header {
			switch http.CanonicalHeaderKey(name) {
			case "User-Agent", "Accept-Encoding":
			default:
				t.Errorf("unexpected header %s", name)
			}
		}
		_, _ = w.Write(k.info(t, "0.8.0", "### Fixes\n\n- a (#1)"))
	}})
	rel, err := Checker{API: g.srv.URL, Web: g.srv.URL, Key: k.pub}.Latest(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if rel.Version != "0.8.0" || rel.Notes != "### Fixes\n\n- a (#1)" || !rel.Published.Equal(time.Date(2026, 11, 27, 18, 0, 0, 0, time.UTC)) ||
		rel.URL != "https://github.com/coredev-uk/sona-go/releases/tag/v0.8.0" {
		t.Fatalf("release = %+v", rel)
	}
	if n := g.count("/repos/"); n != 0 {
		t.Fatalf("the API was asked %d times", n)
	}
}

// Version reads the release.json of that tag, a prerelease too.
func TestVersionReadsTheReleaseInfoOfThatTag(t *testing.T) {
	k := newKeys(t)
	g := newGitHub(t, map[string]http.HandlerFunc{tagInfo("v0.8.0-rc.1"): func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(k.info(t, "0.8.0-rc.1", "rc notes"))
	}})
	c := Checker{API: g.srv.URL, Web: g.srv.URL, Key: k.pub}
	rel, err := c.Version(context.Background(), "v0.8.0-rc.1")
	if err != nil || rel.Version != "0.8.0-rc.1" || rel.Notes != "rc notes" {
		t.Fatalf("Version = %+v, %v", rel, err)
	}
	if _, err := c.Version(context.Background(), "../latest"); !errors.Is(err, ErrNoRelease) {
		t.Errorf("a version that is not one: %v", err)
	}
	if n := g.count("/repos/"); n != 0 {
		t.Fatalf("the API was asked %d times", n)
	}
}

// With Test versions, Latest is the release of the newest tag in releases.atom.
func TestLatestWithTestVersionsReadsTheNewestTag(t *testing.T) {
	k := newKeys(t)
	var g *github
	g = newGitHub(t, map[string]http.HandlerFunc{
		atomPath: func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(atomOf(
				g.srv.URL+"/coredev-uk/sona-go/releases/tag/v0.8.0-rc.2",
				g.srv.URL+"/coredev-uk/sona-go/releases/tag/v0.8.0-rc.1",
				g.srv.URL+"/coredev-uk/sona-go/releases/tag/v0.7.0",
			)))
		},
		tagInfo("v0.8.0-rc.2"): func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(k.info(t, "0.8.0-rc.2", "rc2")) },
	})
	rel, err := Checker{API: g.srv.URL, Web: g.srv.URL, Key: k.pub}.Latest(context.Background(), true)
	if err != nil || rel.Version != "0.8.0-rc.2" || rel.Notes != "rc2" {
		t.Fatalf("Latest = %+v, %v", rel, err)
	}
	if n := g.count("/repos/"); n != 0 {
		t.Fatalf("the API was asked %d times", n)
	}
}

// Nothing unsigned, mis-signed or for another release is read, and none of
// it falls back to the API, which only a release without release.json does.
func TestReleaseInfoIsRefusedUnlessSignedForThatRelease(t *testing.T) {
	k, other := newKeys(t), newKeys(t)
	tamper := func(edit func(*ReleaseInfo)) []byte {
		var r ReleaseInfo
		if err := json.Unmarshal(k.info(t, "0.8.0", "Notes."), &r); err != nil {
			t.Fatal(err)
		}
		edit(&r)
		raw, _ := json.Marshal(r)
		return raw
	}
	for _, tc := range []struct {
		name       string
		serve      []byte
		prerelease bool // read the newest tag's release rather than the latest
		wantSig    bool // ErrSignature rather than another refusal
	}{
		{"another key", other.info(t, "0.8.0", "Notes."), false, true},
		{"unsigned", tamper(func(r *ReleaseInfo) { r.Signature = "" }), false, true},
		{"notes changed", tamper(func(r *ReleaseInfo) { r.Notes = "Other." }), false, true},
		{"version changed", tamper(func(r *ReleaseInfo) { r.Version = "9.9.9" }), false, true},
		{"time changed", tamper(func(r *ReleaseInfo) { r.Published = "2030-01-01T00:00:00Z" }), false, true},
		{"signature is not base64", tamper(func(r *ReleaseInfo) { r.Signature = "!!" }), false, true},
		{"a manifest's statement", func() []byte {
			r := ReleaseInfo{Version: "0.8.0", Published: "2026-11-27T18:00:00Z", Notes: "Notes."}
			r.Signature = SignStatement(k.priv, "darwin-arm64", "0.8.0", make([]byte, sha256.Size))
			raw, _ := json.Marshal(r)
			return raw
		}(), false, true},
		{"not JSON", []byte("<html>"), false, false},
		{"no version", tamper(func(r *ReleaseInfo) { r.Version = "" }), false, false},
		{"a time that isn't one", tamper(func(r *ReleaseInfo) { r.Published = "yesterday" }), false, false},
		{"larger than the cap", []byte(strings.Repeat(" ", MaxReleaseInfo+1)), false, false},
		// A pre-release's genuine, signed release.json on the stable feed.
		{"a pre-release as the latest", k.info(t, "0.9.0-rc.1", "rc"), false, false},
		// An older release's genuine release.json under a newer tag.
		{"another release's under this tag", k.info(t, "0.7.0", "Old."), true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var g *github
			g = newGitHub(t, map[string]http.HandlerFunc{
				latestInfo: func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(tc.serve) },
				atomPath: func(w http.ResponseWriter, _ *http.Request) {
					_, _ = w.Write([]byte(atomOf(g.srv.URL + "/coredev-uk/sona-go/releases/tag/v0.8.0")))
				},
				tagInfo("v0.8.0"): func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(tc.serve) },
			})
			rel, err := Checker{API: g.srv.URL, Web: g.srv.URL, Key: k.pub}.Latest(context.Background(), tc.prerelease)
			if err == nil {
				t.Fatalf("read %+v", rel)
			}
			if errors.Is(err, ErrSignature) != tc.wantSig || errors.Is(err, ErrNoRelease) {
				t.Fatalf("err = %v (ErrSignature wanted: %v)", err, tc.wantSig)
			}
			if n := g.count("/repos/"); n != 0 {
				t.Fatalf("a refused release.json fell back to the API (%d calls)", n)
			}
		})
	}

	t.Run("a key that isn't one", func(t *testing.T) {
		g := newGitHub(t, nil)
		_, err := Checker{API: g.srv.URL, Web: g.srv.URL, Key: "not a key"}.Latest(context.Background(), false)
		if err == nil || errors.Is(err, ErrNoRelease) || g.count("") != 0 {
			t.Fatalf("err = %v, requests %v", err, g.hits)
		}
	})
}

// A release published without release.json (up to v0.8.0-rc.4) is read from
// the API as before.
func TestAReleaseWithoutReleaseInfoFallsBackToTheAPI(t *testing.T) {
	k := newKeys(t)
	const page = "https://github.com/coredev-uk/sona-go/releases/tag/"
	release := func(tag string, prerelease bool) string {
		b, _ := json.Marshal(map[string]any{"tag_name": tag, "html_url": page + tag, "prerelease": prerelease, "published_at": "2026-11-27T17:10:32Z", "body": "## Notes\n\n" + InstallNotesMarker + "\ninstall"})
		return string(b)
	}
	t.Run("Latest", func(t *testing.T) {
		g := newGitHub(t, map[string]http.HandlerFunc{apiLatest: ok(release("v0.7.0", false))})
		rel, err := Checker{API: g.srv.URL, Web: g.srv.URL, Key: k.pub}.Latest(context.Background(), false)
		if err != nil || rel.Version != "0.7.0" || rel.Notes != "## Notes" {
			t.Fatalf("Latest = %+v, %v", rel, err)
		}
		if g.count(latestInfo) != 1 || g.count("/repos/") != 1 {
			t.Fatalf("requests = %v, want release.json, then the API", g.hits)
		}
	})
	t.Run("Version", func(t *testing.T) {
		g := newGitHub(t, map[string]http.HandlerFunc{apiTag("v0.8.0-rc.4"): ok(release("v0.8.0-rc.4", true))})
		rel, err := Checker{API: g.srv.URL, Web: g.srv.URL, Key: k.pub}.Version(context.Background(), "0.8.0-rc.4")
		if err != nil || rel.Version != "0.8.0-rc.4" {
			t.Fatalf("Version = %+v, %v", rel, err)
		}
		if g.count(tagInfo("v0.8.0-rc.4")) != 1 || g.count("/repos/") != 1 {
			t.Fatalf("requests = %v, want release.json, then the API", g.hits)
		}
	})
	t.Run("Latest with test versions", func(t *testing.T) {
		var g *github
		g = newGitHub(t, map[string]http.HandlerFunc{
			atomPath: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(atomOf(g.srv.URL + "/coredev-uk/sona-go/releases/tag/v0.8.0-rc.4")))
			},
			apiTag("v0.8.0-rc.4"): ok(release("v0.8.0-rc.4", true)),
		})
		rel, err := Checker{API: g.srv.URL, Web: g.srv.URL, Key: k.pub}.Latest(context.Background(), true)
		if err != nil || rel.Version != "0.8.0-rc.4" {
			t.Fatalf("Latest = %+v, %v", rel, err)
		}
	})
	t.Run("no release at all", func(t *testing.T) {
		g := newGitHub(t, nil)
		if _, err := (Checker{API: g.srv.URL, Web: g.srv.URL, Key: k.pub}).Latest(context.Background(), false); !errors.Is(err, ErrNoRelease) {
			t.Fatalf("err = %v, want ErrNoRelease", err)
		}
		if _, err := (Checker{API: g.srv.URL, Web: g.srv.URL, Key: k.pub}).Version(context.Background(), "0.4.0"); !errors.Is(err, ErrNoRelease) {
			t.Fatalf("err = %v, want ErrNoRelease", err)
		}
	})
	t.Run("a limit on the API", func(t *testing.T) {
		g := newGitHub(t, map[string]http.HandlerFunc{apiLatest: body(403, `{"message":"rate limit"}`, "X-RateLimit-Remaining", "0", "X-RateLimit-Reset", "1795803306")})
		_, err := Checker{API: g.srv.URL, Web: g.srv.URL, Key: k.pub}.Latest(context.Background(), false)
		var rl *RateLimitError
		if !errors.As(err, &rl) || !rl.Reset.Equal(time.Unix(1795803306, 0)) {
			t.Fatalf("err = %v, want the limit", err)
		}
	})
}

// A build with no update key can't verify release.json: it reads the API as
// it always did.
func TestCheckerWithoutAKeyReadsTheAPI(t *testing.T) {
	g := newGitHub(t, map[string]http.HandlerFunc{apiLatest: ok(`{"tag_name":"v0.7.0"}`)})
	rel, err := Checker{API: g.srv.URL, Web: g.srv.URL}.Latest(context.Background(), false)
	if err != nil || rel.Version != "0.7.0" {
		t.Fatalf("Latest = %+v, %v", rel, err)
	}
	if n := g.count("/coredev-uk/"); n != 0 {
		t.Fatalf("asked for release.json without a key to verify it: %v", g.hits)
	}
}
