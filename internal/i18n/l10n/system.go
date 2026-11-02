package l10n

import (
	"os"
	"slices"
	"strings"
)

// EnvLanguages lists the languages the gettext environment names (LANGUAGE,
// LC_ALL, LC_MESSAGES, LANG) as BCP 47 tags, most preferred first. It is
// instant on every system; SystemLanguages adds the system's own list where
// the environment is not where it lives (macOS, Windows).
func EnvLanguages() []string { return envLanguages(os.Getenv) }

// parseAppleLanguages reads `defaults read -g AppleLanguages` (macOS's
// SystemLanguages): an old-style property list array, `(\n    "en-GB",\n    fr\n)`.
func parseAppleLanguages(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(strings.TrimPrefix(s, "("), ")")
	var tags []string
	for _, f := range strings.Split(s, ",") {
		t := strings.Trim(strings.TrimSpace(f), `"`)
		if t != "" && !slices.Contains(tags, t) {
			tags = append(tags, t)
		}
	}
	return tags
}
