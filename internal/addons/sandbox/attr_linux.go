//go:build linux

package sandbox

import "syscall"

// runFD is the path of the child's descriptor 3 (Spec.ExeFile).
const runFD = "/proc/self/fd/3"

// plainAttr is a session of its own, which the kernel ends when the thread
// that started it does (for a locked thread, when Sona dies).
func plainAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true, Pdeathsig: syscall.SIGKILL}
}
