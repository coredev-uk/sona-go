//go:build linux || (cgo && (darwin || windows))

package main

import "os"

// killProcess ends the process with pid at once (SIGKILL, or
// TerminateProcess on Windows).
func killProcess(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}
