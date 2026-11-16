//go:build unix

package addons

import (
	"os/exec"
	"syscall"
)

// group is the process group the add-on leads.
type group struct{}

func joinGroup(*exec.Cmd) (*group, error) { return &group{}, nil }

func (*group) release() {}

func (*group) terminate(cmd *exec.Cmd) { signalGroup(cmd, syscall.SIGTERM) }

func (*group) kill(cmd *exec.Cmd) { signalGroup(cmd, syscall.SIGKILL) }

// signalGroup signals the whole group (a negative PID): the add-on's
// children are in it, unless one makes a group of its own.
func signalGroup(cmd *exec.Cmd, sig syscall.Signal) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, sig)
	}
}
