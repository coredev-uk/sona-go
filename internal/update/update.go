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
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

// Repo is Sona's GitHub repository (the module path's owner/name).
const Repo = "coredev-uk/sona-go"

// DefaultAPI is GitHub's REST API root.
const DefaultAPI = "https://api.github.com"

// userAgent is sent instead of Go's default; GitHub rejects requests
// without one. It deliberately carries no version or system details.
const userAgent = "Sona-update-check"

// InstallNotesMarker starts the install instructions the release workflow
// appends to a release's body (.github/workflows/release.yml). They are for
// the release page: Release.Notes stops before them, as the update manifests'
// notes do.
const InstallNotesMarker = "<!-- sona-install-notes -->"

// ErrNoRelease means the repository has no published (non-draft,
// non-prerelease) release yet, or is not visible anonymously.
var ErrNoRelease = errors.New("update: no published release")

// Release is a published GitHub release.
type Release struct {
	Version   string // the tag without a leading "v"
	Name      string
	URL       string // the release page
	Notes     string // the release notes (Markdown), without the install instructions; "" when there are none
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
	api, repo := c.API, c.Repo
	if api == "" {
		api = DefaultAPI
	}
	if repo == "" {
		repo = Repo
	}
	raw, err := get(ctx, c.HTTP, strings.TrimRight(api, "/")+"/repos/"+repo+"/releases/latest", 1<<20,
		"Accept", "application/vnd.github+json", "X-GitHub-Api-Version", "2022-11-28")
	if err != nil {
		return Release{}, err
	}
	var body struct {
		TagName     string    `json:"tag_name"`
		Name        string    `json:"name"`
		HTMLURL     string    `json:"html_url"`
		Body        string    `json:"body"`
		PublishedAt time.Time `json:"published_at"`
		Draft       bool      `json:"draft"`
		Prerelease  bool      `json:"prerelease"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return Release{}, fmt.Errorf("update: decode release: %w", err)
	}
	if body.Draft || body.Prerelease || body.TagName == "" {
		return Release{}, ErrNoRelease
	}
	v := strings.TrimPrefix(body.TagName, "v")
	if !Valid(v) {
		return Release{}, fmt.Errorf("update: release tag %q is not a version", body.TagName)
	}
	// Only ever open the repository's own release pages.
	page := "https://github.com/" + repo + "/releases/"
	if !strings.HasPrefix(body.HTMLURL, page) {
		body.HTMLURL = page + "tag/" + body.TagName
	}
	return Release{Version: v, Name: body.Name, URL: body.HTMLURL, Notes: releaseNotes(body.Body), Published: body.PublishedAt}, nil
}

// releaseNotes is a release's body as its notes: the changelog, trimmed,
// without the install instructions the workflow appends after
// InstallNotesMarker.
func releaseNotes(body string) string {
	notes, _, _ := strings.Cut(body, InstallNotesMarker)
	return strings.TrimSpace(notes)
}

// WithoutVersionHeading is notes without a leading heading line that names
// version: release-please opens an entry with "## [0.8.0](compare-url)
// (2026-11-27)", which the release notes dialog's own title and date repeat.
// The heading is any level and its text starts with version or [version]
// (with or without a leading "v"), not followed by more of a version
// ("0.8.0-rc.1" is another one). Blank lines after it go too. Any other
// heading, or one that is not the first line, stays.
func WithoutVersionHeading(notes, version string) string {
	notes = strings.TrimLeft(notes, "\r\n \t")
	first, rest, _ := strings.Cut(notes, "\n")
	text := strings.TrimLeft(first, "#")
	if n := len(first) - len(text); n < 1 || n > 6 || !strings.HasPrefix(text, " ") && !strings.HasPrefix(text, "\t") {
		return notes
	}
	text = strings.TrimLeft(text, " \t")
	if after, ok := strings.CutPrefix(text, "["); ok {
		text = after
	}
	text = strings.TrimPrefix(text, "v")
	after, ok := strings.CutPrefix(text, strings.TrimPrefix(version, "v"))
	if !ok || version == "" {
		return notes
	}
	if after != "" {
		if c := after[0]; c == '.' || c == '-' || c == '+' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' {
			return notes
		}
	}
	return strings.TrimLeft(rest, "\r\n \t")
}

// semverOf is v (with or without a leading "v") as x/mod's semver wants it:
// "v" first. ok is false unless v is MAJOR.MINOR.PATCH[-PRERELEASE][+BUILD]
// (semver 2.0.0): x/mod also takes the shorthands "v1" and "v1.2".
func semverOf(v string) (string, bool) {
	v = "v" + strings.TrimPrefix(v, "v")
	core, _, _ := strings.Cut(v, "+")
	core, _, _ = strings.Cut(core, "-")
	return v, strings.Count(core, ".") == 2 && semver.IsValid(v)
}

// Newer reports whether release version latest is newer than current under
// semantic versioning (a leading "v" is ignored on both). A current version
// that is not semver (a "dev" build) is never offered an update.
func Newer(latest, current string) bool {
	l, ok1 := semverOf(latest)
	c, ok2 := semverOf(current)
	return ok1 && ok2 && semver.Compare(l, c) > 0
}

// Valid reports whether v (with or without a leading "v") is a semantic
// version the check can compare.
func Valid(v string) bool {
	_, ok := semverOf(v)
	return ok
}

// AppImage reports whether this process runs from an AppImage (the runtime
// sets $APPIMAGE to the image's path). Such installs update in place with
// AppImageUpdate from the update information embedded at build time.
func AppImage() bool { return os.Getenv("APPIMAGE") != "" }
