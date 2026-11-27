// Package update checks GitHub for a newer Sona release. It is the one
// network call Sona makes to anyone but Apple, so it sends nothing about the
// user: anonymous GETs of the repository's release files (release.json, the
// update manifests, releases.atom), with a fixed User-Agent, no cookies and
// no credentials. Those are plain downloads, which GitHub's limit of 60
// anonymous API calls an hour per IP address doesn't cover; the API is read
// only for a release published without release.json.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
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

// Checker reads a release's version, notes and publication time. It reads
// the signed release.json asset, a plain download that GitHub's API limit
// doesn't cover, and asks the API only about a release published without it
// (up to v0.8.0-rc.4).
type Checker struct {
	// HTTP is the client to use; nil means a client with a 15 s timeout and
	// no cookie jar.
	HTTP *http.Client
	// API is the REST API root; empty means DefaultAPI.
	API string
	// Web is the site releases download from; empty means https://github.com.
	Web string
	// Repo is owner/name; empty means Repo.
	Repo string
	// Key is the base64 public key release.json must be signed with. Without
	// one (a build with no update key) nothing could be verified, and the
	// API is read as it always was.
	Key string
}

// Latest returns the repository's latest published release, from
// releases/latest/download/release.json. With prerelease it is the release of
// the newest tag in releases.atom, a prerelease too. A repository without a
// release yields ErrNoRelease; GitHub's limit on this network
// *RateLimitError.
func (c Checker) Latest(ctx context.Context, prerelease bool) (Release, error) {
	web, repo := releaseSite(c.Web, c.Repo)
	if !prerelease {
		return c.read(ctx, web, repo, "latest/download", "", "latest", false)
	}
	tag, err := newestTag(ctx, c.HTTP, web, repo)
	if err != nil {
		return Release{}, err
	}
	return c.tag(ctx, web, repo, tag)
}

// Version returns the published release of version (with or without a
// leading "v"), a prerelease too: a build on a release candidate has its
// notes. A version with no release, or one that is not a version, yields
// ErrNoRelease.
func (c Checker) Version(ctx context.Context, version string) (Release, error) {
	version = strings.TrimPrefix(version, "v")
	if !Valid(version) {
		return Release{}, ErrNoRelease
	}
	web, repo := releaseSite(c.Web, c.Repo)
	return c.tag(ctx, web, repo, "v"+version)
}

// tag reads the release of tag.
func (c Checker) tag(ctx context.Context, web, repo, tag string) (Release, error) {
	return c.read(ctx, web, repo, "download/"+url.PathEscape(tag), tag, "tags/"+url.PathEscape(tag), true)
}

// read reads the release.json under releases/{dir}: signed by Key, naming
// tag's version (tag "" is the latest release, which is never a
// prerelease unless prerelease says it may be). A release without the
// asset is read from the API at /repos/{repo}/releases/{apiPath} instead.
func (c Checker) read(ctx context.Context, web, repo, dir, tag, apiPath string, prerelease bool) (Release, error) {
	pub, err := ParseKey(c.Key)
	switch {
	case errors.Is(err, ErrNoKey):
		return c.api(ctx, repo, apiPath, prerelease)
	case err != nil:
		return Release{}, err
	}
	raw, err := get(ctx, c.HTTP, web+"/"+repo+"/releases/"+dir+"/"+ReleaseInfoName, MaxReleaseInfo)
	if errors.Is(err, ErrNoRelease) {
		return c.api(ctx, repo, apiPath, prerelease)
	}
	if err != nil {
		return Release{}, err
	}
	info, err := ParseReleaseInfo(raw, pub)
	if err != nil {
		return Release{}, err
	}
	switch {
	case tag == "" && !prerelease && ChannelOf(info.Version) != ChannelStable:
		return Release{}, fmt.Errorf("update: the latest release names the pre-release %s", info.Version)
	case tag != "" && info.Version != strings.TrimPrefix(tag, "v"):
		return Release{}, fmt.Errorf("update: release %s carries the notes of %s", tag, info.Version)
	}
	published, _ := time.Parse(time.RFC3339, info.Published) // ParseReleaseInfo checked it
	return Release{
		Version: info.Version, Name: "Sona " + info.Version, URL: releasePage(repo, "v"+info.Version),
		Notes: info.Notes, Published: published,
	}, nil
}

// releasePage is the page of the repository's release tagged tag.
func releasePage(repo, tag string) string {
	return "https://github.com/" + repo + "/releases/tag/" + tag
}

// api reads the release at /repos/{repo}/releases/{path} from GitHub's API:
// the way before release.json, and still for a release published without it.
func (c Checker) api(ctx context.Context, repo, path string, prerelease bool) (Release, error) {
	api := c.API
	if api == "" {
		api = DefaultAPI
	}
	raw, err := get(ctx, c.HTTP, strings.TrimRight(api, "/")+"/repos/"+repo+"/releases/"+path, 1<<20,
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
	if body.Draft || body.Prerelease && !prerelease || body.TagName == "" {
		return Release{}, ErrNoRelease
	}
	v := strings.TrimPrefix(body.TagName, "v")
	if !Valid(v) {
		return Release{}, fmt.Errorf("update: release tag %q is not a version", body.TagName)
	}
	// Only ever open the repository's own release pages.
	if page := releasePage(repo, ""); !strings.HasPrefix(body.HTMLURL, page) {
		body.HTMLURL = page + body.TagName
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

// ReleaseNotes is a release's body as release.json carries its notes: the
// changelog without the install instructions (InstallNotesMarker) and
// without the heading that names version (WithoutVersionHeading), trimmed.
func ReleaseNotes(body, version string) string {
	return strings.TrimSpace(WithoutVersionHeading(releaseNotes(body), version))
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
