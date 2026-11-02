package l10n

import (
	"slices"

	"golang.org/x/sys/windows"
)

// SystemLanguages lists the user's display languages (Settings → Time &
// language), most preferred first, then the environment's.
func SystemLanguages() []string {
	tags, err := windows.GetUserPreferredUILanguages(windows.MUI_LANGUAGE_NAME)
	if err != nil {
		tags = nil
	}
	for _, t := range EnvLanguages() {
		if !slices.Contains(tags, t) {
			tags = append(tags, t)
		}
	}
	return tags
}
