package widevine

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
)

// EnvCDM overrides CDM discovery with a path to libwidevinecdm.so.
const EnvCDM = "SONA_WIDEVINE_CDM"

const libName = "libwidevinecdm.so"

// ErrNotFound reports that no installed browser provides a Widevine CDM.
var ErrNotFound = errors.New("widevine: no Widevine CDM found (install Google Chrome, or set " + EnvCDM + ")")

// FindCDM locates an installed libwidevinecdm.so and its version. It tries,
// in order: override, $SONA_WIDEVINE_CDM, Google Chrome installs under
// /opt/google, the directory of google-chrome(-stable) on PATH (Nix keeps
// the CDM in ../share/google/chrome), the component-updated CDMs of
// Chrome, Chromium, Brave, Vivaldi and Edge profiles, and Firefox/Zen GMP
// plugin directories. An explicit override that does not exist is an error.
// The version comes from the adjacent manifest.json, else the directory name.
func FindCDM(override string) (path, version string, err error) {
	home, _ := os.UserHomeDir()
	return finder{opt: "/opt/google", home: home, getenv: os.Getenv, lookPath: exec.LookPath, arch: platformDir()}.find(override)
}

type finder struct {
	home     string
	getenv   func(string) string
	lookPath func(string) (string, error)
	arch     string // "linux_x64" or "linux_arm64"
	opt      string // Google's install root, /opt/google
}

func platformDir() string {
	if runtime.GOARCH == "arm64" {
		return "linux_arm64"
	}
	return "linux_x64"
}

func (f finder) find(override string) (string, string, error) {
	for _, explicit := range []string{override, f.getenv(EnvCDM)} {
		if explicit == "" {
			continue
		}
		p := explicit
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			p = filepath.Join(p, libName)
		}
		if !isFile(p) {
			return "", "", fmt.Errorf("widevine: CDM %s not found", explicit)
		}
		return p, cdmVersion(p), nil
	}
	for _, p := range f.candidates() {
		if isFile(p) {
			return p, cdmVersion(p), nil
		}
	}
	return "", "", ErrNotFound
}

// candidates lists possible CDM paths, best first.
func (f finder) candidates() []string {
	sub := filepath.Join("_platform_specific", f.arch, libName)
	var out []string
	for _, c := range []string{"chrome", "chrome-beta", "chrome-unstable"} {
		out = append(out, filepath.Join(f.opt, c, "WidevineCdm", sub))
	}
	for _, bin := range []string{"google-chrome-stable", "google-chrome", "google-chrome-beta", "google-chrome-unstable"} {
		p, err := f.lookPath(bin)
		if err != nil {
			continue
		}
		if r, err := filepath.EvalSymlinks(p); err == nil {
			p = r
		}
		dir := filepath.Dir(p)
		out = append(out,
			filepath.Join(dir, "WidevineCdm", sub),
			filepath.Join(dir, "..", "share", "google", "chrome", "WidevineCdm", sub))
	}
	if f.home == "" {
		return out
	}
	config := filepath.Join(f.home, ".config")
	for _, profile := range []string{
		filepath.Join(config, "google-chrome"),
		filepath.Join(config, "google-chrome-beta"),
		filepath.Join(config, "google-chrome-unstable"),
		filepath.Join(config, "chromium"),
		filepath.Join(config, "BraveSoftware", "Brave-Browser"),
		filepath.Join(config, "vivaldi"),
		filepath.Join(config, "vivaldi-snapshot"),
		filepath.Join(config, "microsoft-edge"),
		filepath.Join(config, "microsoft-edge-beta"),
		filepath.Join(config, "microsoft-edge-dev"),
		filepath.Join(f.home, ".var", "app", "com.google.Chrome", "config", "google-chrome"),
		filepath.Join(f.home, ".var", "app", "org.chromium.Chromium", "config", "chromium"),
	} {
		out = append(out, componentCDMs(filepath.Join(profile, "WidevineCdm"), sub)...)
	}
	for _, profiles := range []string{
		filepath.Join(f.home, ".mozilla", "firefox"),
		filepath.Join(f.home, ".zen"),
		filepath.Join(config, "zen"),
		filepath.Join(f.home, ".var", "app", "org.mozilla.firefox", ".mozilla", "firefox"),
	} {
		dirs, _ := filepath.Glob(filepath.Join(profiles, "*", "gmp-widevinecdm", "*"))
		out = append(out, newestFirst(dirs, libName)...)
	}
	return out
}

// componentCDMs lists a Chromium-family component directory: the version
// named in latest-component-updated-widevine-cdm first, then the others
// newest first.
func componentCDMs(root, sub string) []string {
	var out []string
	if b, err := os.ReadFile(filepath.Join(root, "latest-component-updated-widevine-cdm")); err == nil {
		var latest struct{ Path string }
		if json.Unmarshal(b, &latest) == nil && latest.Path != "" {
			out = append(out, filepath.Join(latest.Path, sub))
		}
	}
	dirs, _ := filepath.Glob(filepath.Join(root, "*"))
	return append(out, newestFirst(dirs, sub)...)
}

// newestFirst sorts version-named directories descending and appends rel.
func newestFirst(dirs []string, rel string) []string {
	dirs = slices.DeleteFunc(dirs, func(d string) bool { return parseVersion(filepath.Base(d)) == nil })
	slices.SortFunc(dirs, func(a, b string) int {
		return slices.Compare(parseVersion(filepath.Base(b)), parseVersion(filepath.Base(a)))
	})
	out := make([]string, len(dirs))
	for i, d := range dirs {
		out[i] = filepath.Join(d, rel)
	}
	return out
}

func parseVersion(s string) []int {
	parts := strings.Split(s, ".")
	if len(parts) < 2 {
		return nil
	}
	v := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		v[i] = n
	}
	return v
}

// cdmVersion reads the version from manifest.json next to the library
// (Firefox layout) or at the component root (Chrome layout).
func cdmVersion(lib string) string {
	dir := filepath.Dir(lib)
	for _, m := range []string{filepath.Join(dir, "manifest.json"), filepath.Join(dir, "..", "..", "manifest.json")} {
		b, err := os.ReadFile(m)
		if err != nil {
			continue
		}
		var man struct {
			Version string `json:"version"`
		}
		if json.Unmarshal(b, &man) == nil && man.Version != "" {
			return man.Version
		}
	}
	for d := dir; d != "/" && d != "."; d = filepath.Dir(d) {
		if parseVersion(filepath.Base(d)) != nil {
			return filepath.Base(d)
		}
	}
	return ""
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}
