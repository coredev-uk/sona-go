//go:build linux

package sandbox

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// The two environments of the Linux sandbox's two processes. Sona starts a
// copy of its own executable as the add-on's init with the request in
// envRequest (linux_helper.go); that init builds the confinement and runs
// the add-on, whose environment has no trace of it.
const envRequest = "SONA_ADDON_SANDBOX"

// The descriptors the init gets, after standard input, output and error: the
// file to run (Spec.ExeFile, when there is one) and the pipe it reports on.
const (
	fdExe    = 3
	fdStatus = 4
)

// readyTimeout is how long the init has to set the sandbox up and start the
// add-on.
const readyTimeout = 15 * time.Second

// Start runs the add-on confined, or says why it can't.
func Start(spec Spec) (*Child, error) {
	if err := spec.validate(); err != nil {
		return nil, err
	}
	if spec.Unsandboxed || unsandboxedForTests() {
		return startUnix(spec, preparePlain(spec))
	}
	return startUnix(spec, prepareSandboxed(spec))
}

// request is what Sona tells the init: what to run and what it may reach.
type request struct {
	Policy
	// Exe is the program, as the add-on's argv[0]; ExeFD says to run
	// descriptor 3 (the open, checked file) instead of the path.
	Exe   string   `json:"exe"`
	ExeFD bool     `json:"exeFd,omitempty"`
	Args  []string `json:"args,omitempty"`
}

// report is the init's one line on the status pipe: the sandbox is up and
// the add-on runs, or why not.
type report struct {
	OK bool `json:"ok"`
	// Stage says what failed: "userns" (no capabilities in the new user
	// namespace), "seccomp", "exec" (the add-on's program didn't start) or
	// "setup" (anything else).
	Stage  string `json:"stage,omitempty"`
	Detail string `json:"detail,omitempty"`
	// Landlock is the ABI version enforced, 0 when the kernel has none.
	Landlock int `json:"landlock"`
}

// notExecError is a failure of the add-on's own program to start, which says
// nothing about the sandbox.
type notExecError struct{ detail string }

func (e *notExecError) Error() string { return "addons: start: " + e.detail }

func prepareSandboxed(spec Spec) prepare {
	return func(cmd *exec.Cmd) (*launch, error) {
		req, err := json.Marshal(request{Policy: spec.Policy, Exe: spec.Exe, ExeFD: spec.ExeFile != nil, Args: spec.Args})
		if err != nil {
			return nil, fail(ReasonSetup, "request: %v", err)
		}
		statusR, statusW, err := os.Pipe()
		if err != nil {
			return nil, fail(ReasonSetup, "pipe: %v", err)
		}
		cmd.Path, cmd.Args = "/proc/self/exe", []string{"sona-addon-sandbox"}
		cmd.Env = append(append([]string(nil), spec.Env...), envRequest+"="+string(req))
		cmd.ExtraFiles = []*os.File{spec.ExeFile, statusW} // descriptors 3 and 4; a nil file leaves 3 closed
		cmd.SysProcAttr = namespaceAttr(spec.Network)
		l := &launch{release: func() { _ = statusR.Close(); _ = statusW.Close() }}
		l.ready = func() error {
			_ = statusW.Close() // ours: the init's copy is the only writer left
			rep, err := readReport(statusR)
			if err == nil && rep.Landlock == 0 {
				l.notes = append(l.notes, "this kernel has no Landlock: the add-on's filesystem view, namespaces and syscall filter confine it, without the kernel's second check of the files")
			}
			return err
		}
		return l, nil
	}
}

// namespaceAttr starts the init in new user, mount, PID, IPC and UTS
// namespaces, and a new network namespace (no interfaces: no network, and
// abstract Unix sockets of its own) unless the add-on has the network. The
// user namespace maps the caller's uid and gid onto themselves; it gives the
// init CAP_SYS_ADMIN in it alone, ambient so that it survives the exec, to
// build the filesystem view. The new session has no controlling terminal.
func namespaceAttr(network bool) *syscall.SysProcAttr {
	flags := uintptr(syscall.CLONE_NEWUSER | syscall.CLONE_NEWNS | syscall.CLONE_NEWPID | syscall.CLONE_NEWIPC | syscall.CLONE_NEWUTS)
	if !network {
		flags |= syscall.CLONE_NEWNET
	}
	return &syscall.SysProcAttr{
		Cloneflags:  flags,
		UidMappings: []syscall.SysProcIDMap{{ContainerID: os.Getuid(), HostID: os.Getuid(), Size: 1}},
		GidMappings: []syscall.SysProcIDMap{{ContainerID: os.Getgid(), HostID: os.Getgid(), Size: 1}},
		AmbientCaps: []uintptr{unix.CAP_SYS_ADMIN},
		Setsid:      true,
		// The thread that starts the add-on stays locked to it: when Sona
		// dies the kernel kills the init, and with it everything in the
		// PID namespace.
		Pdeathsig: syscall.SIGKILL,
	}
}

// readReport reads the init's report: an error when the sandbox is not up.
func readReport(r *os.File) (report, error) {
	_ = r.SetReadDeadline(time.Now().Add(readyTimeout))
	line, err := bufio.NewReader(r).ReadBytes('\n')
	if err != nil {
		return report{}, fail(ReasonSetup, "the sandbox's init ended without a report: %v", err)
	}
	var rep report
	if err := json.Unmarshal(line, &rep); err != nil {
		return report{}, fail(ReasonSetup, "the sandbox's report is not valid: %v", err)
	}
	if rep.OK {
		return rep, nil
	}
	switch rep.Stage {
	case "userns":
		return rep, fail(usernsReason(), "%s", rep.Detail)
	case "seccomp":
		return rep, fail(ReasonNoSeccomp, "%s", rep.Detail)
	case "exec":
		return rep, &notExecError{rep.Detail}
	}
	return rep, fail(ReasonSetup, "%s", rep.Detail)
}

// startError says why the init could not even be created: the kernel refused
// a user namespace, or something else.
func startError(err error) error {
	for _, e := range []error{unix.EPERM, unix.EACCES, unix.EINVAL, unix.ENOSPC, unix.ENOSYS, unix.EUSERS} {
		if errors.Is(err, e) {
			return fail(usernsReason(), "start: %v", err)
		}
	}
	return fail(ReasonSetup, "start: %v", err)
}

// usernsReason tells the restriction Ubuntu puts on user namespaces from a
// system that has none.
func usernsReason() Reason {
	if sysctl("/proc/sys/kernel/apparmor_restrict_unprivileged_userns") == 1 {
		return ReasonUserNamespaceRestricted
	}
	return ReasonNoUserNamespace
}

// sysctl reads a number from /proc/sys; -1 when it is not there.
func sysctl(path string) int {
	b, err := os.ReadFile(path)
	if err != nil {
		return -1
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return -1
	}
	return n
}
