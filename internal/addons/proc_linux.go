//go:build linux

package addons

import "syscall"

// sysProcAttr puts the add-on in a group of its own and has the kernel kill
// it when the thread that started it ends, which for a locked thread is
// when Sona dies.
func sysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
}
