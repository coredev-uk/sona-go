package addons

import (
	"runtime"
	"strings"
)

// allowedEnv are the variables an add-on inherits from Sona, by name. Nothing
// else does: a token, a key, a bus address or an automation switch in Sona's
// environment (MUSIC_USER_TOKEN, SONA_LASTFM_KEY, DBUS_SESSION_BUS_ADDRESS,
// MYGO_AUTOMATION, …) stays out of its reach. LC_* are allowed as a prefix.
var allowedEnv = []string{"HOME", "PATH", "LANG", "LANGUAGE", "TZ", "TMPDIR"}

// allowedEnvWindows are what a Windows process needs to start at all.
var allowedEnvWindows = []string{"SYSTEMROOT", "SYSTEMDRIVE", "TEMP", "TMP", "USERPROFILE", "PATHEXT", "COMSPEC"}

// Environment builds the environment of the add-on id from parent (as
// os.Environ gives it): the allowlisted variables, then the add-on's own,
// which a parent variable of the same name never overrides.
func Environment(parent []string, id string) []string {
	return environment(parent, id, runtime.GOOS == "windows")
}

func environment(parent []string, id string, windows bool) []string {
	out := make([]string, 0, 12)
	for _, kv := range parent {
		name, _, ok := strings.Cut(kv, "=")
		if !ok || name == "" {
			continue
		}
		if windows {
			name = strings.ToUpper(name)
		}
		if allowed(name, windows) {
			out = append(out, kv)
		}
	}
	return append(out, "SONA_ADDON_ID="+id, "SONA_ADDON_PROTOCOL=2")
}

func allowed(name string, windows bool) bool {
	for _, a := range allowedEnv {
		if name == a {
			return true
		}
	}
	if strings.HasPrefix(name, "LC_") {
		return true
	}
	if windows {
		for _, a := range allowedEnvWindows {
			if name == a {
				return true
			}
		}
	}
	return false
}
