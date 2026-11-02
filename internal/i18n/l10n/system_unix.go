//go:build !darwin && !windows

package l10n

// SystemLanguages lists the system's languages, most preferred first. On
// Linux and the BSDs the desktop exports them as the gettext environment.
func SystemLanguages() []string { return EnvLanguages() }
