// Package update checks GitHub for a newer Sona release. It is the one
// network call Sona makes to anyone but Apple, so it sends nothing about the
// user: an anonymous GET of the repository's latest release, with a fixed
// User-Agent, no cookies and no credentials.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Repo is Sona's GitHub repository (the module path's owner/name).
const Repo = "coredev-uk/sona-go"

// DefaultAPI is GitHub's REST API root.
const DefaultAPI = "https://api.github.com"

// userAgent is sent instead of Go's default; GitHub rejects requests
// without one. It deliberately carries no version or system details.
const userAgent = "Sona-update-check"

// ErrNoRelease means the repository has no published (non-draft,
// non-prerelease) release yet, or is not visible anonymously.
var ErrNoRelease = errors.New("update: no published release")

// Release is a published GitHub release.
type Release struct {
	Version   string // the tag without a leading "v"
	Name      string
	URL       string // the release page
	Published time.Time
}

// Checker fetches the latest release.
type Checker struct {
	// HTTP is the client to use; nil means a client with a 15 s timeout and
	// no cookie jar.
	HTTP *http.Client
	// API is the REST API root; empty means DefaultAPI.
	API string
	// Repo is owner/name; empty means Repo.
	Repo string
}

// Latest returns the repository's latest published release. A repository
// without one yields ErrNoRelease.
func (c Checker) Latest(ctx context.Context) (Release, error) {
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	api, repo := c.API, c.Repo
	if api == "" {
		api = DefaultAPI
	}
	if repo == "" {
		repo = Repo
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(api, "/")+"/repos/"+repo+"/releases/latest", nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", userAgent)
	resp, err := hc.Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("update: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return Release{}, ErrNoRelease
	case resp.StatusCode != http.StatusOK:
		return Release{}, fmt.Errorf("update: GitHub answered %s", resp.Status)
	}
	var body struct {
		TagName     string    `json:"tag_name"`
		Name        string    `json:"name"`
		HTMLURL     string    `json:"html_url"`
		PublishedAt time.Time `json:"published_at"`
		Draft       bool      `json:"draft"`
		Prerelease  bool      `json:"prerelease"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return Release{}, fmt.Errorf("update: decode release: %w", err)
	}
	if body.Draft || body.Prerelease || body.TagName == "" {
		return Release{}, ErrNoRelease
	}
	v := strings.TrimPrefix(body.TagName, "v")
	if _, ok := parse(v); !ok {
		return Release{}, fmt.Errorf("update: release tag %q is not a version", body.TagName)
	}
	// Only ever open the repository's own release pages.
	page := "https://github.com/" + repo + "/releases/"
	if !strings.HasPrefix(body.HTMLURL, page) {
		body.HTMLURL = page + "tag/" + body.TagName
	}
	return Release{Version: v, Name: body.Name, URL: body.HTMLURL, Published: body.PublishedAt}, nil
}

// Newer reports whether release version latest is newer than current under
// semantic versioning (a leading "v" is ignored on both). A current version
// that is not semver (a "dev" build) is never offered an update.
func Newer(latest, current string) bool {
	l, ok1 := parse(strings.TrimPrefix(latest, "v"))
	c, ok2 := parse(strings.TrimPrefix(current, "v"))
	return ok1 && ok2 && compare(l, c) > 0
}

// Valid reports whether v (with or without a leading "v") is a semantic
// version the check can compare.
func Valid(v string) bool {
	_, ok := parse(strings.TrimPrefix(v, "v"))
	return ok
}

// AppImage reports whether this process runs from an AppImage (the runtime
// sets $APPIMAGE to the image's path). Such installs update in place with
// AppImageUpdate from the update information embedded at build time.
func AppImage() bool { return os.Getenv("APPIMAGE") != "" }

type version struct {
	core [3]uint64
	pre  []string // dot-separated pre-release identifiers; empty for a release
}

// parse reads MAJOR.MINOR.PATCH[-PRERELEASE][+BUILD] (semver 2.0.0).
func parse(s string) (version, bool) {
	var v version
	s, _, _ = strings.Cut(s, "+")
	s, pre, hasPre := strings.Cut(s, "-")
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return v, false
	}
	for i, p := range parts {
		if !numeric(p) || (len(p) > 1 && p[0] == '0') {
			return v, false
		}
		n, err := strconv.ParseUint(p, 10, 64)
		if err != nil {
			return v, false
		}
		v.core[i] = n
	}
	if hasPre {
		v.pre = strings.Split(pre, ".")
		for _, id := range v.pre {
			if id == "" {
				return v, false
			}
		}
	}
	return v, true
}

func numeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// compare orders versions by semver precedence: the numeric core, then a
// release above its pre-releases, then pre-release identifiers left to
// right (numeric ones numerically and below alphanumeric ones, a shorter
// list first when all shared identifiers are equal).
func compare(a, b version) int {
	for i := range a.core {
		if a.core[i] != b.core[i] {
			if a.core[i] > b.core[i] {
				return 1
			}
			return -1
		}
	}
	switch {
	case len(a.pre) == 0 && len(b.pre) == 0:
		return 0
	case len(a.pre) == 0:
		return 1
	case len(b.pre) == 0:
		return -1
	}
	for i := 0; i < len(a.pre) && i < len(b.pre); i++ {
		x, y := a.pre[i], b.pre[i]
		if x == y {
			continue
		}
		xn, yn := numeric(x), numeric(y)
		switch {
		case xn && yn:
			xi, _ := strconv.ParseUint(x, 10, 64)
			yi, _ := strconv.ParseUint(y, 10, 64)
			if xi > yi {
				return 1
			}
			return -1
		case xn:
			return -1
		case yn:
			return 1
		case x > y:
			return 1
		default:
			return -1
		}
	}
	switch {
	case len(a.pre) > len(b.pre):
		return 1
	case len(a.pre) < len(b.pre):
		return -1
	}
	return 0
}
