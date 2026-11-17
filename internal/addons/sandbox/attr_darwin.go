//go:build darwin

package sandbox

import "syscall"

// runFD: macOS can't run the file that was checked yet.
const runFD = ""

// plainAttr is a session of its own. This system has no parent-death signal:
// when Sona is killed, the add-on's end of input closes, and an add-on is
// expected to exit then (docs/addons.md).
func plainAttr() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setsid: true} }
