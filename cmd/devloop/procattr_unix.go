//go:build unix

package main

import (
	"os/exec"
	"syscall"
)

// ownGroup runs the app in a process group of its own: a Ctrl-C at the
// terminal then reaches devloop only, which stops the app with one SIGTERM.
// In devloop's group the app would get the SIGINT too, and MyGo exits on a
// second signal without running its shutdown.
func ownGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
