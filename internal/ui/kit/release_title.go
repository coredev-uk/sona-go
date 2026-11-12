package kit

import "strings"

// ReleaseTitle is a single's or EP's title without Apple's " - Single" /
// " - EP" suffix, and which it was ("single", "ep", or "" for neither): the
// catalogue lists say the kind in their own line.
func ReleaseTitle(name string) (title, kind string) {
	if t, ok := strings.CutSuffix(name, " - Single"); ok && t != "" {
		return t, "single"
	}
	if t, ok := strings.CutSuffix(name, " - EP"); ok && t != "" {
		return t, "ep"
	}
	return name, ""
}
