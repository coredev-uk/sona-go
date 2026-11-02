package l10n

import (
	"context"
	"os/exec"
	"slices"
	"time"
)

// SystemLanguages lists the user's preferred languages (System Settings →
// General → Language & Region), most preferred first, then the
// environment's. A Finder-launched app has no LANG, so the list comes from
// the AppleLanguages default; it takes a few milliseconds.
func SystemLanguages() []string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/usr/bin/defaults", "read", "-g", "AppleLanguages").Output()
	var tags []string
	if err == nil {
		tags = parseAppleLanguages(string(out))
	}
	for _, t := range EnvLanguages() {
		if !slices.Contains(tags, t) {
			tags = append(tags, t)
		}
	}
	return tags
}
