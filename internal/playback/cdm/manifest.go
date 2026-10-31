package cdm

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Source is where the release comes from: the manifest URLs, tried in
// order, and the hosts the manifest and the package may be fetched from
// (redirects included). Every URL must be HTTPS.
type Source struct {
	Manifests     []string
	ManifestHosts []string
	PackageHosts  []string
}

// Mozilla is the pinned source: Firefox's GMP manifest for Widevine in
// Mozilla's repository (its GitHub mirror, then hg.mozilla.org, which
// redirects to hg-edge), the file Firefox's own GMP updater falls back to.
// It names the version, Google's download URLs and the package's SHA-512.
// The package itself comes only from Google's download hosts.
var Mozilla = Source{
	Manifests: []string{
		"https://raw.githubusercontent.com/mozilla-firefox/firefox/refs/heads/main/toolkit/content/gmp-sources/widevinecdm.json",
		"https://hg.mozilla.org/mozilla-central/raw-file/tip/toolkit/content/gmp-sources/widevinecdm.json",
	},
	ManifestHosts: []string{"raw.githubusercontent.com", "hg.mozilla.org", "hg-edge.mozilla.org"},
	PackageHosts:  []string{"edgedl.me.gvt1.com", "www.google.com", "dl.google.com"},
}

// Limits on what is read from the network.
const (
	maxManifestBytes = 1 << 20  // Mozilla's manifest is ~6 KiB
	maxPackageBytes  = 64 << 20 // the CRX is ~22 MiB
)

// Release is the platform's entry in the manifest.
type Release struct {
	Version string
	// URLs are the package's URL then its mirrors, each HTTPS on a
	// package host; entries on other hosts are dropped.
	URLs   []string
	Size   int64
	SHA512 []byte
}

// mozillaManifest is the subset of Mozilla's GMP manifest Sona reads.
type mozillaManifest struct {
	HashFunction string `json:"hashFunction"`
	Vendors      map[string]struct {
		Version   string                     `json:"version"`
		Platforms map[string]json.RawMessage `json:"platforms"`
	} `json:"vendors"`
}

type mozillaPlatform struct {
	Alias      string   `json:"alias"`
	FileURL    string   `json:"fileUrl"`
	MirrorURLs []string `json:"mirrorUrls"`
	Filesize   int64    `json:"filesize"`
	HashValue  string   `json:"hashValue"`
}

// widevineVendor is the manifest's vendor key for the Widevine CDM.
const widevineVendor = "gmp-widevinecdm"

// versionPattern is the shape of a Widevine version ("4.10.3112.0"). The
// version names an install directory, so nothing else is accepted.
var versionPattern = regexp.MustCompile(`^[0-9]{1,6}(\.[0-9]{1,6}){1,3}$`)

// parseManifest returns p's release from a manifest document.
func parseManifest(body []byte, p Platform, src Source) (Release, error) {
	var m mozillaManifest
	if err := json.Unmarshal(body, &m); err != nil {
		return Release{}, fail(CodeManifest, "manifest: %w", err)
	}
	if !strings.EqualFold(m.HashFunction, "sha512") {
		return Release{}, fail(CodeManifest, "manifest: hash function %q, want sha512", m.HashFunction)
	}
	vendor, ok := m.Vendors[widevineVendor]
	if !ok {
		return Release{}, fail(CodeManifest, "manifest: no %s vendor", widevineVendor)
	}
	if !versionPattern.MatchString(vendor.Version) {
		return Release{}, fail(CodeManifest, "manifest: version %q", vendor.Version)
	}
	key := p.Mozilla
	var e mozillaPlatform
	for hops := 0; ; hops++ {
		raw, ok := vendor.Platforms[key]
		if !ok {
			return Release{}, fail(CodeUnsupportedPlatform, "manifest: no Widevine CDM for %s (%s)", p, key)
		}
		e = mozillaPlatform{}
		if err := json.Unmarshal(raw, &e); err != nil {
			return Release{}, fail(CodeManifest, "manifest: %s: %w", key, err)
		}
		if e.Alias == "" {
			break
		}
		if hops == 3 {
			return Release{}, fail(CodeManifest, "manifest: alias chain from %s too long", p.Mozilla)
		}
		key = e.Alias
	}
	sum, err := hex.DecodeString(e.HashValue)
	if err != nil || len(sum) != 64 {
		return Release{}, fail(CodeManifest, "manifest: %s: hash %q is not a SHA-512", key, e.HashValue)
	}
	if e.Filesize <= 0 || e.Filesize > maxPackageBytes {
		return Release{}, fail(CodeTooLarge, "manifest: %s: size %d outside 1..%d", key, e.Filesize, maxPackageBytes)
	}
	r := Release{Version: vendor.Version, Size: e.Filesize, SHA512: sum}
	for _, u := range slices.Concat([]string{e.FileURL}, e.MirrorURLs) {
		if allowedURL(u, src.PackageHosts) && !slices.Contains(r.URLs, u) {
			r.URLs = append(r.URLs, u)
		}
	}
	if len(r.URLs) == 0 {
		return Release{}, fail(CodeUntrustedURL, "manifest: %s: no package URL on Google's hosts (%q)", key, e.FileURL)
	}
	return r, nil
}

// allowedURL reports whether raw is an HTTPS URL on one of hosts, with no
// user info and the default port.
func allowedURL(raw string, hosts []string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" && u.Port() != "443" {
		return false
	}
	return slices.Contains(hosts, strings.ToLower(u.Hostname()))
}

// restricted returns a copy of c that follows redirects only to hosts.
func restricted(c *http.Client, hosts []string) *http.Client {
	r := *c
	r.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		if !allowedURL(req.URL.String(), hosts) {
			return fail(CodeUntrustedURL, "redirect to %s", req.URL.Redacted())
		}
		return nil
	}
	return &r
}

// fetchRelease reads p's release from src's manifests, the first that
// answers. A manifest that answers but lacks p is final.
func fetchRelease(ctx context.Context, hc *http.Client, src Source, p Platform) (Release, error) {
	c := restricted(hc, src.ManifestHosts)
	var errs []error
	for _, u := range src.Manifests {
		if !allowedURL(u, src.ManifestHosts) {
			errs = append(errs, fail(CodeUntrustedURL, "manifest URL %s", u))
			continue
		}
		body, err := get(ctx, c, u, maxManifestBytes)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		r, err := parseManifest(body, p, src)
		if err == nil || CodeOf(err) == CodeUnsupportedPlatform {
			return r, err
		}
		errs = append(errs, err)
	}
	return Release{}, firstCoded(errs)
}

// get reads a small document.
func get(ctx context.Context, c *http.Client, u string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fail(CodeNetwork, "GET %s: HTTP %d", u, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fail(CodeTooLarge, "GET %s: more than %d bytes", u, limit)
	}
	return body, nil
}

// firstCoded joins errs, reporting the code of the most telling one: a
// refusal (anything but a network failure) wins over a network failure.
func firstCoded(errs []error) error {
	if len(errs) == 0 {
		return fail(CodeManifest, "no manifest source")
	}
	for _, err := range errs {
		if CodeOf(err) != CodeNetwork {
			return &Error{Code: CodeOf(err), Err: errors.Join(errs...)}
		}
	}
	return &Error{Code: CodeNetwork, Err: errors.Join(errs...)}
}

// compareVersions orders dotted versions numerically; both must match
// versionPattern.
func compareVersions(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := range max(len(as), len(bs)) {
		var x, y int
		if i < len(as) {
			x, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			y, _ = strconv.Atoi(bs[i])
		}
		if x != y {
			return x - y
		}
	}
	return 0
}

// String describes the release for logs.
func (r Release) String() string {
	return fmt.Sprintf("%s (%d bytes, %s)", r.Version, r.Size, r.URLs[0])
}
