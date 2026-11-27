package update

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// maxAtom bounds the releases feed: GitHub's lists the ten newest releases
// with their notes as HTML (120 kB on 2026-11-27).
const maxAtom = 4 << 20

// atomFeed is the part of GitHub's releases.atom that names the releases: each
// entry's alternate link is the release page, https://github.com/{repo}/releases/tag/{tag}.
// It lists no drafts, prereleases included, newest first.
type atomFeed struct {
	XMLName xml.Name `xml:"http://www.w3.org/2005/Atom feed"`
	Entries []struct {
		Links []struct {
			Rel  string `xml:"rel,attr"`
			Href string `xml:"href,attr"`
		} `xml:"http://www.w3.org/2005/Atom link"`
	} `xml:"http://www.w3.org/2005/Atom entry"`
}

// newestTag is the tag of the highest version in the repository's
// releases.atom, prereleases included. The feed is a plain page of the
// website, which GitHub's API limit doesn't cover, and it is not signed: the
// tag only says where to read the signed release (the manifest or
// release.json), whose own version must be that tag's.
func newestTag(ctx context.Context, hc *http.Client, web, repo string) (string, error) {
	raw, err := get(ctx, hc, web+"/"+repo+"/releases.atom", maxAtom, "Accept", "application/atom+xml")
	if err != nil {
		return "", err
	}
	var feed atomFeed
	if err := xml.Unmarshal(raw, &feed); err != nil {
		return "", fmt.Errorf("update: decode the releases feed: %w", err)
	}
	prefix := web + "/" + repo + "/releases/tag/"
	best := ""
	for _, e := range feed.Entries {
		for _, l := range e.Links {
			rest, ok := strings.CutPrefix(l.Href, prefix)
			if !ok || l.Rel != "alternate" {
				continue
			}
			tag, err := url.PathUnescape(rest)
			if err != nil || strings.ContainsAny(tag, "/?#") {
				continue
			}
			v := strings.TrimPrefix(tag, "v")
			if !Valid(v) {
				continue
			}
			if best == "" || Newer(v, strings.TrimPrefix(best, "v")) {
				best = tag
			}
		}
	}
	if best == "" {
		return "", ErrNoRelease
	}
	return best, nil
}
