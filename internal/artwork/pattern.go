package artwork

import (
	"regexp"
	"slices"
)

// Patterns are the covers Apple Music offers for a playlist in its iPhone
// and Mac apps: nine designs in the playlist's four colours with its name at
// the top left, PDCXS11 (the plain diagonal gradient Apple gives a new
// playlist) to PDCXS19. Apple's CDN draws each from the URL of the cover it
// generated for the playlist (gen/{w}x{h}AM.PDCXS1n.jpg?c1…c4=…&t=<name>&
// tc=…&signature=…): the signature covers the colours, the name and the text
// colour but not the design, so the playlist's own URL with another design
// is that design (checked 2026-11-06: PDCXS11 to PDCXS19 answer 200, PDCXS10
// and PDCXS20 404, a changed name or colour 400). The order is Apple's.
var Patterns = []string{"PDCXS11", "PDCXS12", "PDCXS13", "PDCXS14", "PDCXS15", "PDCXS16", "PDCXS17", "PDCXS18", "PDCXS19"}

// patternPart finds the design in the path of an Apple-generated playlist
// cover.
var patternPart = regexp.MustCompile(`^https://[^/?#]+/image/thumb/gen/[^/?#]*AM\.(PDCXS\d\d)\.`)

// Pattern returns the design (one of Patterns) of tmpl, an Apple-generated
// playlist cover, and false for any other template.
func Pattern(tmpl string) (string, bool) {
	m := patternPart.FindStringSubmatch(tmpl)
	if m == nil || !slices.Contains(Patterns, m[1]) {
		return "", false
	}
	return m[1], true
}

// WithPattern returns the Apple-generated playlist cover tmpl in design p
// (one of Patterns), and false when tmpl is not such a cover or p is not a
// design.
func WithPattern(tmpl, p string) (string, bool) {
	m := patternPart.FindStringSubmatchIndex(tmpl)
	if m == nil || !slices.Contains(Patterns, tmpl[m[2]:m[3]]) || !slices.Contains(Patterns, p) {
		return "", false
	}
	return tmpl[:m[2]] + p + tmpl[m[3]:], true
}

// isPattern reports an Apple-generated playlist cover in any design: the
// playlist's name is in its URL.
func isPattern(tmpl string) bool {
	_, ok := Pattern(tmpl)
	return ok
}
