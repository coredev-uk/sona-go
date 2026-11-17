//go:build linux || darwin

package sandbox

import (
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// waitDelay bounds the wait for the add-on's standard error to close after
// it exits: a grandchild that kept the pipe open must not hold Wait.
const waitDelay = 2 * time.Second

// A launch is what prepare sets cmd up with: ready runs once the process has
// started and waits for the confinement to report, failing when it could not
// be set up; release always runs last, and frees what prepare made.
type launch struct {
	ready   func() error
	release func()
	notes   []string // set by ready
}

// prepare sets cmd up to run the add-on confined (or, in a test, not).
type prepare func(cmd *exec.Cmd) (*launch, error)

// startUnix starts the process prepare describes, in a session of its own
// (so with no controlling terminal and a group Sona can signal) and with
// only the channel's two pipes and its standard error as descriptors.
func startUnix(spec Spec, prep prepare) (*Child, error) {
	inR, inW, err := os.Pipe()
	if err != nil {
		return nil, fail(ReasonSetup, "pipe: %v", err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		_ = inR.Close()
		_ = inW.Close()
		return nil, fail(ReasonSetup, "pipe: %v", err)
	}
	closeAll := func() {
		for _, f := range []*os.File{inR, inW, outR, outW} {
			_ = f.Close()
		}
	}
	cmd := &exec.Cmd{Dir: spec.Dir, Stdin: inR, Stdout: outW, WaitDelay: waitDelay}
	if spec.Stderr != nil {
		// A bare writer, so that os/exec copies from a pipe of its own even
		// when Stderr is an *os.File: a terminal is never passed on.
		cmd.Stderr = struct{ io.Writer }{spec.Stderr}
	}
	l, err := prep(cmd)
	if err != nil {
		closeAll()
		return nil, err
	}
	defer l.release()
	if err := cmd.Start(); err != nil {
		closeAll()
		return nil, startError(err)
	}
	_ = inR.Close()
	_ = outW.Close()
	pid := cmd.Process.Pid
	signalGroup := func(sig syscall.Signal) { _ = syscall.Kill(-pid, sig) }
	if err := l.ready(); err != nil {
		signalGroup(syscall.SIGKILL)
		_ = cmd.Wait()
		_ = inW.Close()
		_ = outR.Close()
		return nil, err
	}
	c := &Child{
		In: inW, Out: outR, Notes: l.notes, pid: pid, done: make(chan struct{}),
		terminate: func() { signalGroup(syscall.SIGTERM) },
		kill:      func() { signalGroup(syscall.SIGKILL) },
	}
	go func() {
		c.err = cmd.Wait()
		close(c.done)
	}()
	return c, nil
}

// preparePlain runs the program as it is: for a test binary that starts
// fake add-ons without the sandbox (EnvUnsandboxed).
func preparePlain(spec Spec) prepare {
	return func(cmd *exec.Cmd) (*launch, error) {
		cmd.Path, cmd.Args, cmd.Env = spec.Exe, append([]string{spec.Exe}, spec.Args...), spec.Env
		if spec.ExeFile != nil {
			if runFD == "" {
				return nil, fail(ReasonUnsupported, "this system can't run the file that was checked")
			}
			cmd.Path, cmd.ExtraFiles = runFD, []*os.File{spec.ExeFile}
		}
		cmd.SysProcAttr = plainAttr()
		return &launch{ready: func() error { return nil }, release: func() {}}, nil
	}
}
