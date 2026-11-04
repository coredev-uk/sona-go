// Package devinstance names a development run, so that two worktrees run
// side by side: with SONA_DEV_INSTANCE=<name> set, a development build
// (-tags dev) suffixes the single-instance D-Bus name, the MPRIS bus name
// and the dev-restore directory. A production build ignores the variable:
// Name is always empty there and nothing changes.
package devinstance

import "regexp"

// Env is the variable that names the instance.
const Env = "SONA_DEV_INSTANCE"

var unsafe = regexp.MustCompile(`[^A-Za-z0-9_]+`)

// sanitize makes a name that is a valid element of a D-Bus bus name and a
// directory name: letters, digits and underscores.
func sanitize(s string) string {
	s = unsafe.ReplaceAllString(s, "_")
	if len(s) > 64 {
		s = s[:64]
	}
	return s
}

// Suffix is what a bus name takes after its base: ".instance_<name>" (a
// valid MPRIS instance suffix, and a D-Bus element that does not start with
// a digit), "" without an instance.
func Suffix() string {
	if n := Name(); n != "" {
		return ".instance_" + n
	}
	return ""
}
