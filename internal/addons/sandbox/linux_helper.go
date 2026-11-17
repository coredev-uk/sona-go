//go:build linux

package sandbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// Main turns the process into an add-on's init when Sona started it as one
// (envRequest is set): it builds the confinement around itself, runs the
// add-on in it and exits when the add-on does, ending everything left in the
// PID namespace with it. Otherwise it returns at once. Call it first thing in
// main, and in the TestMain of a test binary that starts sandboxed add-ons.
func Main() {
	raw, ok := os.LookupEnv(envRequest)
	if !ok {
		return
	}
	os.Exit(runInit(raw))
}

// runInit is the init's life. It is PID 1 of the add-on's PID namespace and
// stays on one OS thread, which Landlock and seccomp are applied to and the
// add-on is started from, so that the add-on has them and the init's other
// threads (the Go runtime's) need not.
func runInit(raw string) int {
	runtime.LockOSThread()
	status := os.NewFile(fdStatus, "status")
	say := func(r report) {
		line, _ := json.Marshal(r)
		_, _ = status.Write(append(line, '\n'))
	}
	die := func(stage, format string, args ...any) int {
		fmt.Fprintf(os.Stderr, "sona: sandbox: "+format+"\n", args...)
		say(report{Stage: stage, Detail: fmt.Sprintf(format, args...)})
		return 1
	}
	var req request
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		return die("setup", "the request is not valid: %v", err)
	}
	// The report's pipe must not reach the add-on, nor anything else Sona
	// had open that was not made close-on-exec.
	unix.CloseOnExec(fdStatus)
	closeDescriptors(req.ExeFD)
	if err := noTerminals(); err != nil {
		return die("setup", "%v", err)
	}
	if !hasSysAdmin() {
		return die("userns", "no capabilities in the new user namespace")
	}
	view, err := buildView(req)
	if err != nil {
		return die("setup", "filesystem view: %v", err)
	}
	// The sandbox's own hostname, in the UTS namespace of the add-on alone.
	_ = unix.Sethostname([]byte("sona-addon"))
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return die("setup", "no_new_privs: %v", err)
	}
	abi, err := restrictLandlock(req, view)
	if err != nil {
		return die("setup", "landlock: %v", err)
	}
	// Ambient capabilities would survive the exec; nothing else does.
	_ = unix.Prctl(unix.PR_CAP_AMBIENT, unix.PR_CAP_AMBIENT_CLEAR_ALL, 0, 0, 0)
	if err := installSeccomp(req); err != nil {
		return die("seccomp", "%v", err)
	}

	env := make([]string, 0, len(os.Environ()))
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, envRequest+"=") {
			env = append(env, kv)
		}
	}
	path, files := req.Exe, []uintptr{0, 1, 2}
	if req.ExeFD {
		path, files = "/proc/self/fd/3", append(files, fdExe)
	}
	pid, err := syscall.ForkExec(path, append([]string{req.Exe}, req.Args...), &syscall.ProcAttr{
		Dir: req.Dir, Env: env, Files: files,
		Sys: &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL},
	})
	if err != nil {
		return die("exec", "run %s: %v", req.Exe, err)
	}
	say(report{OK: true, Landlock: abi})
	_ = status.Close()
	return reap(pid)
}

// reap is what an init does: it passes the signals Sona sends on to the
// add-on (the kernel gives PID 1 no default action for them), reaps every
// process that is left without a parent, and exits with the add-on's status
// when it ends. Whatever else still runs is killed with the namespace.
func reap(child int) int {
	sigs := make(chan os.Signal, 8)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP, syscall.SIGQUIT, syscall.SIGUSR1, syscall.SIGUSR2)
	go func() {
		for s := range sigs {
			if sig, ok := s.(syscall.Signal); ok {
				_ = unix.Kill(child, sig)
			}
		}
	}()
	for {
		var ws unix.WaitStatus
		pid, err := unix.Wait4(-1, &ws, 0, nil)
		switch {
		case errors.Is(err, unix.EINTR):
			continue
		case err != nil:
			return 0 // no children left
		case pid != child:
			continue
		case ws.Signaled():
			return 128 + int(ws.Signal())
		}
		return ws.ExitStatus()
	}
}

// closeDescriptors closes every descriptor of the init but the standard
// three, the report's and, when there is one, the file to run.
func closeDescriptors(keepExe bool) {
	first := fdStatus + 1
	if !keepExe {
		_ = unix.Close(fdExe)
	}
	if err := unix.CloseRange(uint(first), math.MaxUint32, 0); err == nil {
		return
	}
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return
	}
	for _, e := range entries {
		var fd int
		if _, err := fmt.Sscan(e.Name(), &fd); err == nil && fd >= first {
			_ = unix.Close(fd)
		}
	}
}

// noTerminals refuses to run when a standard descriptor is a terminal:
// Start gives the add-on pipes, so one that isn't is a bug that would put a
// terminal of the user's in an add-on's hands.
func noTerminals() error {
	for fd := range 3 {
		if _, err := unix.IoctlGetTermios(fd, unix.TCGETS); err == nil {
			return fmt.Errorf("standard descriptor %d is a terminal", fd)
		}
	}
	return nil
}

// hasSysAdmin reports whether the init holds CAP_SYS_ADMIN, in its user
// namespace: it does not under Ubuntu's AppArmor restriction.
func hasSysAdmin() bool {
	hdr := unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}
	var data [2]unix.CapUserData
	if err := unix.Capget(&hdr, &data[0]); err != nil {
		return false
	}
	return data[0].Effective&(1<<unix.CAP_SYS_ADMIN) != 0
}
