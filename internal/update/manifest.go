package update

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Manifest is update-<target>.json, MyGo's format (mygo/internal/update):
// the newest version for one platform. cmd/releasesign writes it; a test
// pins it against MyGo's own parser fields.
type Manifest struct {
	Version string `json:"version"`
	Notes   string `json:"notes,omitempty"`
	Date    string `json:"date,omitempty"`
	// URL, Size and Signature describe the archive (a gzip tar): its
	// address, its bytes and the Ed25519 signature of its SHA-256 digest.
	URL       string `json:"url"`
	Size      int64  `json:"size"`
	Signature string `json:"signature"`
	// SHA256 is the archive's digest in hex and StatementSignature the
	// Ed25519 signature of Statement(target, version, channel, digest): what
	// binds Version and the platform to the archive. MyGo ignores both. A
	// manifest without them is refused (VerifyStatement).
	SHA256             string `json:"sha256,omitempty"`
	StatementSignature string `json:"statement_signature,omitempty"`
}

// MaxManifest and MaxArchive bound what an install downloads (MyGo's
// limits).
const (
	MaxManifest = 1 << 20
	MaxArchive  = 1 << 30
)

// Target is the manifest target of an OS and architecture, "" where Sona
// doesn't update itself (Linux, which uses its package manager).
func Target(goos, goarch string) string {
	switch t := goos + "-" + goarch; t {
	case "darwin-arm64", "windows-amd64":
		return t
	}
	return ""
}

// ManifestName is the manifest's file name for target.
func ManifestName(target string) string { return "update-" + target + ".json" }

// Feed finds the manifest of the newest release.
type Feed struct {
	// HTTP is the client; nil means 15 s and no cookies.
	HTTP *http.Client
	// Web is the site releases download from (default https://github.com).
	// Repo defaults to Repo.
	Web, Repo string
	// Key is the base64 public key manifests must be signed with.
	Key string
}

// releaseSite is the website root and repository (owner/name) a feed reads
// from, with their defaults.
func releaseSite(web, repo string) (string, string) {
	if web = strings.TrimRight(web, "/"); web == "" {
		web = "https://github.com"
	}
	if repo == "" {
		repo = Repo
	}
	return web, repo
}

// Manifest reads the manifest of target from the latest release: one
// anonymous GET of releases/latest/download/update-<target>.json. With
// prerelease it reads the repository's releases.atom first (the newest tag,
// prereleases included) and then that release's manifest. Neither is an API
// call, so GitHub's anonymous limit doesn't reach them. It returns the
// decoded manifest and its raw bytes, once the signed statement says the
// version is the key holder's for target (VerifyStatement), the stable feed
// carries no pre-release and a release's manifest names that release.
func (f Feed) Manifest(ctx context.Context, target string, prerelease bool) (Manifest, []byte, error) {
	pub, err := ParseKey(f.Key)
	if err != nil {
		return Manifest{}, nil, err
	}
	web, repo := releaseSite(f.Web, f.Repo)
	u, tag := web+"/"+repo+"/releases/latest/download/"+ManifestName(target), ""
	if prerelease {
		var err error
		tag, err = newestTag(ctx, f.HTTP, web, repo)
		if err != nil {
			return Manifest{}, nil, err
		}
		u = web + "/" + repo + "/releases/download/" + url.PathEscape(tag) + "/" + ManifestName(target)
	}
	raw, err := get(ctx, f.HTTP, u, MaxManifest)
	if err != nil {
		return Manifest{}, nil, err
	}
	m, err := parseManifest(raw, pub, target, prerelease, tag)
	if err != nil {
		return Manifest{}, nil, err
	}
	return m, raw, nil
}

// parseManifest decodes the manifest of target in raw, and checks it (see
// Manifest): tag is the release it was read from, for a prerelease read.
func parseManifest(raw []byte, pub ed25519.PublicKey, target string, prerelease bool, tag string) (Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return Manifest{}, fmt.Errorf("update: decode manifest: %w", err)
	}
	if err := m.validate(); err != nil {
		return Manifest{}, err
	}
	if err := VerifyStatement(pub, m, target); err != nil {
		return Manifest{}, err
	}
	switch {
	case !prerelease && ChannelOf(m.Version) != ChannelStable:
		return Manifest{}, fmt.Errorf("update: the stable feed names the pre-release %s", m.Version)
	case prerelease && m.Version != strings.TrimPrefix(tag, "v"):
		return Manifest{}, fmt.Errorf("update: release %s carries the manifest of %s", tag, m.Version)
	}
	return m, nil
}

func (m Manifest) validate() error {
	switch {
	case !Valid(m.Version):
		return fmt.Errorf("update: manifest version %q is not a version", m.Version)
	case m.Signature == "" || m.URL == "":
		return errors.New("update: incomplete manifest")
	case m.Size <= 0 || m.Size > MaxArchive:
		return fmt.Errorf("update: invalid archive size %d", m.Size)
	}
	return checkURL(m.URL)
}

// checkURL allows HTTPS, and plain HTTP on the loopback interface (tests).
func checkURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme == "https" || (u.Scheme == "http" && isLoopback(u.Hostname())) {
		return nil
	}
	return fmt.Errorf("update: refusing %s: updates need HTTPS", raw)
}

func isLoopback(host string) bool {
	ip := net.ParseIP(host)
	return host == "localhost" || (ip != nil && ip.IsLoopback())
}

// get reads up to limit bytes of u with hc (nil: a client with a 15 s
// timeout and no cookie jar), sending headers (name, value pairs) beside the
// fixed User-Agent. A 404 is ErrNoRelease, an answer larger than limit an
// error, and GitHub's limit on this network a *RateLimitError.
func get(ctx context.Context, hc *http.Client, u string, limit int64, headers ...string) ([]byte, error) {
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("update: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNoRelease
	}
	if rl, ok := rateLimit(resp, time.Now()); ok {
		return nil, rl
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update: %s answered %s", req.URL.Host, resp.Status)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("update: reading %s: %w", req.URL.Host, err)
	}
	if int64(len(raw)) > limit {
		return nil, fmt.Errorf("update: %s answered more than %d bytes", req.URL.Host, limit)
	}
	return raw, nil
}
