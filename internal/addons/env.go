package addons

import (
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// allowedEnv are the variables an add-on inherits from Sona, by name. Nothing
// else does: a token, a key, a bus address or an automation switch in Sona's
// environment (MUSIC_USER_TOKEN, SONA_LASTFM_KEY, DBUS_SESSION_BUS_ADDRESS,
// MYGO_AUTOMATION, …) stays out of its reach. LC_* are allowed as a prefix.
// The user's home and temporary folders are not among them: the add-on's own
// are its data folder's (Environment).
var allowedEnv = []string{"PATH", "LANG", "LANGUAGE", "TZ"}

// allowedEnvWindows are what a Windows process needs to start at all.
var allowedEnvWindows = []string{"SYSTEMROOT", "SYSTEMDRIVE", "PATHEXT", "COMSPEC"}

// bundledEnv are the variables only Sona's own add-ons inherit besides the
// above:
//
//   - the proxy and certificate settings the Go HTTP client reads, which
//     Last.fm's and ListenBrainz's requests honoured when they ran inside
//     Sona;
//   - LD_LIBRARY_PATH, where the dynamic loader finds Sona's own libraries. An
//     add-on of Sona's is Sona's executable and has its loader needs: the
//     published Nix package's binary has no rpath (build/linux/nix/release.nix),
//     its wrapper sets the path, so without it the program (the sandbox's
//     init, then the add-on) ends at the loader with status 127 before it can
//     say anything. The folders it names are the ones sandbox.ProgramReads
//     shows the sandbox.
//
// Nothing in them is a secret of Sona's.
var bundledEnv = []string{
	"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "no_proxy",
	"SSL_CERT_FILE", "SSL_CERT_DIR",
	"LD_LIBRARY_PATH",
}

// bundledAllowed reports whether Sona's own add-ons inherit name: bundledEnv,
// and in a development build the fake service endpoints (devEnv).
func bundledAllowed(name string) bool { return slices.Contains(bundledEnv, name) || devEnv(name) }

// Environment builds the environment of the add-on id from parent (as
// os.Environ gives it): the allowlisted variables, then the add-on's own,
// which a parent variable of the same name never overrides. data is its
// private folder (SONA_ADDON_DATA): the home and temporary folders of a
// process that can write nowhere else are inside it (HOME, TMPDIR, and on
// Windows USERPROFILE, TEMP and TMP).
func Environment(parent []string, id, data string) []string {
	return environment(parent, id, data, runtime.GOOS == "windows")
}

func environment(parent []string, id, data string, windows bool) []string {
	out := make([]string, 0, 14)
	for _, kv := range parent {
		name, _, ok := strings.Cut(kv, "=")
		if !ok || name == "" {
			continue
		}
		if windows {
			name = strings.ToUpper(name)
		}
		if allowed(name, windows) || (IsBundled(id) && bundledAllowed(name)) {
			out = append(out, kv)
		}
	}
	tmp := filepath.Join(data, TempName)
	out = append(out, "SONA_ADDON_ID="+id, "SONA_ADDON_PROTOCOL=2", "SONA_ADDON_DATA="+data, "HOME="+data, "TMPDIR="+tmp)
	if windows {
		out = append(out, "USERPROFILE="+data, "TEMP="+tmp, "TMP="+tmp)
	}
	return out
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
