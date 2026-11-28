// Package sandbox starts an out-of-process add-on confined by the operating
// system (docs/addons.md, "The sandbox"; the threat model is "Add-ons" in
// docs/security-review.md). What an add-on can reach is a Policy, and the
// confinement is not optional: where a system cannot set it up, Start fails
// with an *Error saying why and runs nothing. The one way around is
// Spec.Unsandboxed, which only the supervisor sets, for Sona's own bundled
// add-ons (docs/addons.md, "When the sandbox can't start").
//
// Per OS:
//
//   - Linux: Sona's own executable re-executes itself as a small init in new
//     user, mount, PID, IPC and UTS namespaces (and a network namespace when
//     the add-on has no network), builds a filesystem view that holds only
//     what the policy names, adds Landlock where the kernel has it and a
//     seccomp filter, and runs the add-on from there (linux.go,
//     linux_helper.go, linux_view.go). The process that calls Main first is
//     that init.
//   - macOS: the add-on runs under sandbox-exec with a Seatbelt profile
//     (darwin.go, seatbelt.go).
//   - Windows: the add-on runs in an AppContainer inside a job object
//     (windows.go, over internal/appcontainer).
package sandbox

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Policy is what the confined add-on can reach. Nothing else of the user's
// system is visible to it.
type Policy struct {
	// Network lets the add-on use the network (the `network` permission).
	// Without it the add-on has no network at all, not even the loopback.
	Network bool
	// Dir is the add-on's own folder: readable, never writable. It is the
	// add-on's working directory.
	Dir string
	// Data is the add-on's private folder (SONA_ADDON_DATA): the one place it
	// can write. It exists before Start.
	Data string
	// Read are more folders the add-on may read, never write: for a bundled
	// add-on, where Sona's own program and libraries are. Only code sets it.
	Read []string
	// Sockets are Unix sockets of the user's session the add-on may connect
	// to (Discord's). Only code sets it; a manifest cannot.
	Sockets []string
}

// Spec is how to run one add-on.
type Spec struct {
	Policy
	// ID is the add-on's ID; on Windows it names its AppContainer profile.
	ID string
	// Exe is the absolute path of the program: inside Dir, or inside one of
	// Read.
	Exe string
	// ExeFile, when set, is the one open file that is run instead of Exe's
	// path (Linux; addons.OpenVerified): the file that was checked is the
	// file that runs.
	ExeFile *os.File
	// Args follow Exe in the command line.
	Args []string
	// Env is the add-on's whole environment (addons.Environment).
	Env []string
	// Stderr receives the add-on's standard error. It is never handed over
	// as a file: the add-on has no descriptor of Sona's, a terminal least of
	// all.
	Stderr io.Writer
	// Unsandboxed runs the add-on as a plain child process with Sona's own
	// rights: still in a group or job object of its own that ends with Sona,
	// with the environment given and the channel's pipes, but with no
	// confinement. Only the supervisor sets it, for one of Sona's own add-ons
	// (code-registered) after Start failed with an *Error; never for another.
	Unsandboxed bool
}

// Reason is why a system can't confine an add-on, a code the UI words
// (Settings → Add-ons).
type Reason string

// The reasons. Detail of an Error is for the log; only the code reaches the
// user.
const (
	// ReasonUserNamespaceRestricted: the system lets programs create user
	// namespaces but takes their capabilities away (Ubuntu's AppArmor
	// restriction, kernel.apparmor_restrict_unprivileged_userns).
	ReasonUserNamespaceRestricted Reason = "userns_restricted"
	// ReasonNoUserNamespace: the kernel, the container or the Flatpak sandbox
	// Sona runs in refuses user namespaces.
	ReasonNoUserNamespace Reason = "no_userns"
	// ReasonNoSeccomp: the syscall filter can't be built or installed.
	ReasonNoSeccomp Reason = "no_seccomp"
	// ReasonNoSeatbelt: macOS' sandbox-exec is missing or refuses the profile.
	ReasonNoSeatbelt Reason = "no_seatbelt"
	// ReasonNoAppContainer: Windows can't create the AppContainer, grant it
	// the folders or start the process in it.
	ReasonNoAppContainer Reason = "no_appcontainer"
	// ReasonUnsupported: this operating system has no sandbox for add-ons.
	ReasonUnsupported Reason = "unsupported"
	// ReasonSetup: any other failure while setting the sandbox up.
	ReasonSetup Reason = "setup_failed"
)

// Error is the sandbox failing to start: the add-on did not run.
type Error struct {
	Reason Reason
	Detail string
}

func (e *Error) Error() string {
	if e.Detail == "" {
		return "sandbox: " + string(e.Reason)
	}
	return "sandbox: " + string(e.Reason) + ": " + e.Detail
}

func fail(r Reason, format string, args ...any) *Error {
	return &Error{Reason: r, Detail: fmt.Sprintf(format, args...)}
}

// ReasonOf is the reason err gives for the sandbox failing, false when err is
// not the sandbox's.
func ReasonOf(err error) (Reason, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e.Reason, true
	}
	return "", false
}

// EnvUnsandboxed lets a test binary start its fake add-ons without the
// sandbox, on systems where its own sandbox tests are not the point (the
// supervisor's). Only a test binary honours it (testing.Testing): a Sona
// build never runs an add-on outside the sandbox.
const EnvUnsandboxed = "SONA_ADDON_UNSANDBOXED"

// validate checks what every OS needs of a spec.
func (s Spec) validate() error {
	for name, p := range map[string]string{"program": s.Exe, "folder": s.Dir, "data folder": s.Data} {
		if !filepath.IsAbs(p) {
			return fail(ReasonSetup, "the add-on's %s %q is not an absolute path", name, p)
		}
	}
	for _, p := range s.Read {
		if !filepath.IsAbs(p) {
			return fail(ReasonSetup, "the readable folder %q is not an absolute path", p)
		}
	}
	for _, p := range s.Sockets {
		if !filepath.IsAbs(p) {
			return fail(ReasonSetup, "the socket %q is not an absolute path", p)
		}
	}
	return nil
}

// ProgramReads are the folders Sona's own executable needs to read to run as
// a bundled add-on: its own folder, the folder of an AppImage it runs from
// (APPDIR) and the library folders its environment names. env is Sona's
// environment (KEY=VALUE). Only existing absolute folders are returned, links
// resolved, none inside another.
func ProgramReads(exe string, env []string) []string {
	var dirs []string
	add := func(p string) {
		if !filepath.IsAbs(p) {
			return
		}
		if r, err := filepath.EvalSymlinks(p); err == nil {
			p = r
		}
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			dirs = append(dirs, filepath.Clean(p))
		}
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	add(filepath.Dir(exe))
	value := func(key string) string {
		for _, kv := range env {
			if k, v, ok := strings.Cut(kv, "="); ok && k == key {
				return v
			}
		}
		return ""
	}
	add(value("APPDIR"))
	for _, d := range filepath.SplitList(value("LD_LIBRARY_PATH")) {
		add(d)
	}
	slices.SortFunc(dirs, func(a, b string) int { return len(a) - len(b) })
	var out []string
	for _, d := range dirs {
		if !slices.ContainsFunc(out, func(o string) bool {
			return d == o || strings.HasPrefix(d, strings.TrimSuffix(o, string(filepath.Separator))+string(filepath.Separator))
		}) {
			out = append(out, d)
		}
	}
	return out
}
