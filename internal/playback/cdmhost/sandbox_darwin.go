//go:build cgo

package cdmhost

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/playback"
)

// Sandbox describes how the CDM host child is confined on macOS: it runs
// under sandbox-exec with seatbeltProfile (seatbelt.go), which takes away
// the network, file writes outside the temporary folders, Sona's own
// folder, the credential stores and the keychain. Where sandbox-exec is
// missing or refuses the profile, or the developer turned it off
// (EnvSeatbelt), the child runs unconfined and Reason says why; unlike on
// Linux it still loads the CDM (development builds only: macOS has no
// release yet).
type Sandbox struct {
	Seatbelt   bool // runs under sandbox-exec with Sona's profile
	InetDenied bool // the child's own connect() to an inet address was refused
	// SecretsHidden: the child could not list ~/Library/Keychains, which
	// the profile hides along with Sona's folder, ~/.ssh and ~/.gnupg.
	SecretsHidden bool
	Reason        string // why a layer is missing, for the log; "" when Active
	// Unconfined is why the child runs without sandbox-exec ("" when it
	// runs under it); the parent sets it.
	Unconfined playback.SandboxGap
}

// Gaps are the missing layers (Diagnostics), or why the child runs
// unconfined.
func (s Sandbox) Gaps() []playback.SandboxGap {
	if !s.Seatbelt {
		if s.Unconfined == "" {
			return []playback.SandboxGap{playback.SandboxGapNoSeatbelt}
		}
		return []playback.SandboxGap{s.Unconfined}
	}
	var gaps []playback.SandboxGap
	if !s.InetDenied {
		gaps = append(gaps, playback.SandboxGapNetwork)
	}
	if !s.SecretsHidden {
		gaps = append(gaps, playback.SandboxGapKeychain)
	}
	return gaps
}

// Active reports whether the child runs under the profile and its
// self-checks found the network and the hidden folders out of reach.
func (s Sandbox) Active() bool { return s.Seatbelt && s.InetDenied && s.SecretsHidden }

// Kind is the sandbox the child runs in (Diagnostics):
// playback.SandboxSeatbelt, or playback.SandboxNone when it is unconfined.
func (s Sandbox) Kind() string {
	if s.Seatbelt {
		return playback.SandboxSeatbelt
	}
	return playback.SandboxNone
}

// Attrs are the layers as key-value pairs for a log line (playspike).
func (s Sandbox) Attrs() []any {
	return []any{
		"seatbelt", s.Seatbelt, "inet_denied", s.InetDenied, "secrets_hidden", s.SecretsHidden,
		"reason", strconv.Quote(s.Reason),
	}
}

// loadsCDM is always true on macOS: an unconfined child still loads the
// CDM, and Diagnostics says it is not sandboxed.
func (Sandbox) loadsCDM() bool { return true }

// envSeatbeltChild tells the child it was started under sandbox-exec.
const envSeatbeltChild = "SONA_CDMHOST_SEATBELT"

// sandboxEnv are the sandbox's own variables, never inherited by a child.
var sandboxEnv = []string{envSeatbeltChild}

// pluginPathEnv are the variables withheld from the child to keep plugin
// folders out of its scan: none here (Windows names the video plugins' folder
// in one, sandbox_windows.go).
var pluginPathEnv []string

// sandboxExec is macOS's Seatbelt launcher.
const sandboxExec = "/usr/bin/sandbox-exec"

// parentSandbox is the parent's part of the sandbox: why the child runs
// unconfined (gap; reason in the system's words, for the log), "" when it
// runs under sandbox-exec.
type parentSandbox struct {
	gap    playback.SandboxGap
	reason string
}

// seatbelt is this user's profile paths, or why the children of this
// process run unconfined: decided once, on the first spawn
// (seatbeltChoice), with a probe of sandbox-exec running /usr/bin/true
// under the profile.
var seatbelt = sync.OnceValues(func() (seatbeltPaths, parentSandbox) {
	paths := darwinSeatbeltPaths()
	_, lookErr := os.Stat(sandboxExec)
	gap, reason := seatbeltChoice(os.Getenv(EnvSeatbelt) == "0", lookErr, func() error {
		out, err := exec.Command(sandboxExec, "-p", seatbeltProfile(paths), "/usr/bin/true").CombinedOutput()
		if err != nil {
			if line, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n"); line != "" {
				return fmt.Errorf("%w: %s", err, line)
			}
		}
		return err
	})
	if gap != "" {
		return seatbeltPaths{}, parentSandbox{gap: gap, reason: reason}
	}
	return paths, parentSandbox{}
})

// startSandboxed starts the child under sandbox-exec with Sona's profile,
// which also lets it read the CDM library's folder (envCDMDir in env), or
// unconfined when the sandbox is unavailable (parentSandbox.reason).
func startSandboxed(exe string, env []string, log *slog.Logger) (proc, parentSandbox, error) {
	paths, ps := seatbelt()
	if ps.gap == "" {
		paths.Readable = nil
		for _, kv := range env {
			if d, ok := strings.CutPrefix(kv, envCDMDir+"="); ok && filepath.IsAbs(d) {
				if r, err := filepath.EvalSymlinks(d); err == nil {
					d = r
				}
				paths.Readable = []string{filepath.Clean(d)}
			}
		}
		p, err := startChild([]string{sandboxExec, "-p", seatbeltProfile(paths), exe}, append(slices.Clip(env), envSeatbeltChild+"=1"), nil)
		if err == nil {
			return p, parentSandbox{}, nil
		}
		ps = parentSandbox{gap: playback.SandboxGapNoSeatbelt, reason: "no macOS sandbox: sandbox-exec did not start: " + err.Error()}
	}
	log.Warn("cdmhost: starting the playback host without the macOS sandbox", "reason", ps.reason)
	p, err := startChild([]string{exe}, env, nil)
	return p, ps, err
}

// withParent adds the parent's reason for an unconfined child.
func (s Sandbox) withParent(ps parentSandbox) Sandbox {
	s.Unconfined = ps.gap
	if ps.reason != "" {
		if s.Reason != "" {
			s.Reason = ps.reason + "; " + s.Reason
		} else {
			s.Reason = ps.reason
		}
	}
	return s
}

// darwinSeatbeltPaths are the hidden and writable paths of the profile
// for this user, with symlinks resolved.
func darwinSeatbeltPaths() seatbeltPaths {
	var p seatbeltPaths
	home := os.Getenv("HOME")
	if !filepath.IsAbs(home) {
		return p
	}
	if dirs, err := config.ResolvePaths(os.Getenv); err == nil {
		p.Hidden = append(p.Hidden, dirs.Config, dirs.Data, dirs.State)
	}
	p.Hidden = append(p.Hidden,
		filepath.Join(home, "Library", "Keychains"),
		filepath.Join(home, ".ssh"),
		filepath.Join(home, ".gnupg"),
		filepath.Join(home, ".password-store"),
	)
	p.Writable = gstRegistryDirs(home)
	for _, l := range []*[]string{&p.Hidden, &p.Writable} {
		for i, d := range *l {
			if r, err := filepath.EvalSymlinks(d); err == nil {
				(*l)[i] = r
			}
		}
		slices.Sort(*l)
		*l = slices.Compact(*l)
	}
	return p
}

// gstRegistryDirs are where GStreamer keeps its plugin registry: the
// directory of $GST_REGISTRY_1_0 or $GST_REGISTRY when set, else
// gstreamer-1.0 in GLib's user cache directory ($XDG_CACHE_HOME, else
// ~/.cache).
func gstRegistryDirs(home string) []string {
	for _, env := range []string{"GST_REGISTRY_1_0", "GST_REGISTRY"} {
		if f := os.Getenv(env); filepath.IsAbs(f) {
			return []string{filepath.Dir(f)}
		}
	}
	cache := os.Getenv("XDG_CACHE_HOME")
	if !filepath.IsAbs(cache) {
		cache = filepath.Join(home, ".cache")
	}
	return []string{filepath.Join(cache, "gstreamer-1.0")}
}

// enterSandbox runs in the child before the CDM or GStreamer load. The
// parent already confined it (sandbox-exec applies the profile before the
// child's first instruction); it checks the profile holds.
func enterSandbox() Sandbox {
	sb := Sandbox{Seatbelt: os.Getenv(envSeatbeltChild) == "1"}
	if !sb.Seatbelt {
		return sb // the parent says why (withParent)
	}
	var reasons []string
	if err := inetRefused(); err != nil {
		reasons = append(reasons, "network allowed: "+err.Error())
	} else {
		sb.InetDenied = true
	}
	if err := secretsHidden(); err != nil {
		reasons = append(reasons, "keychain folder readable: "+err.Error())
	} else {
		sb.SecretsHidden = true
	}
	sb.Reason = strings.Join(reasons, "; ")
	return sb
}

// secretsHidden checks that the child cannot list ~/Library/Keychains,
// which every macOS user has: the profile denies it with EPERM.
func secretsHidden() error {
	dir := filepath.Join(os.Getenv("HOME"), "Library", "Keychains")
	_, err := os.ReadDir(dir)
	switch {
	case err == nil:
		return fmt.Errorf("listed %s", dir)
	case errors.Is(err, os.ErrPermission):
		return nil
	}
	return err
}
