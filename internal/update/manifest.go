package update

import (
	"context"
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
	// Web is the site releases download from (default https://github.com),
	// API the REST root (default DefaultAPI). Repo defaults to Repo.
	Web, API, Repo string
}

// Manifest reads the manifest of target from the latest release: one
// anonymous GET of releases/latest/download/update-<target>.json. With
// prerelease it lists the repository's releases first (the newest tag
// including prereleases) and reads that release's manifest. It returns the
// decoded manifest and its raw bytes.
func (f Feed) Manifest(ctx context.Context, target string, prerelease bool) (Manifest, []byte, error) {
	web, repo := strings.TrimRight(f.Web, "/"), f.Repo
	if web == "" {
		web = "https://github.com"
	}
	if repo == "" {
		repo = Repo
	}
	u := web + "/" + repo + "/releases/latest/download/" + ManifestName(target)
	if prerelease {
		tag, err := f.newestTag(ctx, repo)
		if err != nil {
			return Manifest{}, nil, err
		}
		u = web + "/" + repo + "/releases/download/" + tag + "/" + ManifestName(target)
	}
	raw, err := f.get(ctx, u, MaxManifest)
	if err != nil {
		return Manifest{}, nil, err
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return Manifest{}, nil, fmt.Errorf("update: decode manifest: %w", err)
	}
	if err := m.validate(); err != nil {
		return Manifest{}, nil, err
	}
	return m, raw, nil
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

func (f Feed) client() *http.Client {
	if f.HTTP != nil {
		return f.HTTP
	}
	return &http.Client{Timeout: 15 * time.Second}
}

func (f Feed) get(ctx context.Context, u string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := f.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("update: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, ErrNoRelease
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("update: %s answered %s", req.URL.Host, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}

// newestTag is the tag of the highest version among non-draft releases,
// prereleases included.
func (f Feed) newestTag(ctx context.Context, repo string) (string, error) {
	api := f.API
	if api == "" {
		api = DefaultAPI
	}
	raw, err := f.get(ctx, strings.TrimRight(api, "/")+"/repos/"+repo+"/releases?per_page=30", 4<<20)
	if err != nil {
		return "", err
	}
	var list []struct {
		Tag   string `json:"tag_name"`
		Draft bool   `json:"draft"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		return "", fmt.Errorf("update: decode releases: %w", err)
	}
	best := ""
	for _, r := range list {
		v := strings.TrimPrefix(r.Tag, "v")
		if r.Draft || !Valid(v) || strings.ContainsAny(r.Tag, "/?#") {
			continue
		}
		if best == "" || Newer(v, strings.TrimPrefix(best, "v")) {
			best = r.Tag
		}
	}
	if best == "" {
		return "", ErrNoRelease
	}
	return best, nil
}
