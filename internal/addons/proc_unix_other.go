//go:build unix && !linux

package addons

import "syscall"

// sysProcAttr puts the add-on in a group of its own. This system has no
// parent-death signal: when Sona is killed, the add-on's end of input closes,
// and an add-on is expected to exit then (docs/addons.md).
func sysProcAttr() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setpgid: true} }
