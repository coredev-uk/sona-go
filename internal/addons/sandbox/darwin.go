//go:build darwin

package sandbox

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// seatbeltSystemHidden are the folders that hold the user's data besides the
// home directory: other volumes and the per-user and shared temporary
// folders, the keychains.
var seatbeltSystemHidden = []string{
	"/Users", "/Volumes", "/private/tmp", "/private/var/tmp", "/private/var/folders", "/Library/Keychains",
}

// sandboxExec is macOS's Seatbelt launcher.
const sandboxExec = "/usr/bin/sandbox-exec"

// Start runs the add-on under sandbox-exec with Sona's profile (seatbelt.go),
// in a session of its own (no controlling terminal), or says why it can't.
// sandbox-exec is deprecated but present on every macOS Sona runs on, and is
// the way to confine a program that is not Sona's own.
func Start(spec Spec) (*Child, error) {
	if err := spec.validate(); err != nil {
		return nil, err
	}
	if spec.ExeFile != nil {
		return nil, fail(ReasonUnsupported, "macOS can't run the file that was checked")
	}
	if spec.Unsandboxed || unsandboxedForTests() {
		return startUnix(spec, preparePlain(spec))
	}
	profile := seatbeltProfile(seatbeltPaths(spec.Policy))
	if err := probeProfile(profile); err != nil {
		return nil, fail(ReasonNoSeatbelt, "%v", err)
	}
	return startUnix(spec, func(cmd *exec.Cmd) (*launch, error) {
		cmd.Path = sandboxExec
		cmd.Args = append([]string{sandboxExec, "-p", profile, spec.Exe}, spec.Args...)
		cmd.Env = spec.Env
		cmd.SysProcAttr = plainAttr()
		return &launch{ready: func() error { return nil }, release: func() {}}, nil
	})
}

// seatbeltPaths is the profile's input for a policy, every path resolved.
func seatbeltPaths(p Policy) seatbeltInput {
	resolve := func(paths ...string) []string {
		var out []string
		for _, d := range paths {
			if r, err := filepath.EvalSymlinks(d); err == nil {
				d = r
			}
			out = append(out, filepath.Clean(d))
		}
		return out
	}
	in := seatbeltInput{Policy: p}
	in.Dir, in.Data = resolve(p.Dir)[0], resolve(p.Data)[0]
	in.Read, in.Sockets = resolve(p.Read...), resolve(p.Sockets...)
	hidden := slices.Clone(seatbeltSystemHidden)
	if home, err := os.UserHomeDir(); err == nil && filepath.IsAbs(home) {
		hidden = append(hidden, home)
	}
	in.Hidden = resolve(hidden...)
	slices.Sort(in.Hidden)
	in.Hidden = slices.Compact(in.Hidden)
	return in
}

// probeProfile checks the profile before it is trusted with an add-on:
// sandbox-exec must accept it and run a program under it, and the user's home
// must be out of that program's reach.
func probeProfile(profile string) error {
	if _, err := os.Stat(sandboxExec); err != nil {
		return err
	}
	if out, err := exec.Command(sandboxExec, "-p", profile, "/usr/bin/true").CombinedOutput(); err != nil {
		line, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
		return errors.New("sandbox-exec refused the profile: " + err.Error() + ": " + line)
	}
	home, err := os.UserHomeDir()
	if err != nil || !filepath.IsAbs(home) {
		return nil
	}
	// The home directory must not list: ls exits 0 when it can.
	if err := exec.Command(sandboxExec, "-p", profile, "/bin/ls", home).Run(); err == nil {
		return errors.New("the profile did not hide the user's home")
	}
	return nil
}

func startError(err error) error {
	if errors.Is(err, unix.ENOENT) || errors.Is(err, syscall.ENOENT) {
		return fail(ReasonNoSeatbelt, "start: %v", err)
	}
	return fail(ReasonSetup, "start: %v", err)
}
