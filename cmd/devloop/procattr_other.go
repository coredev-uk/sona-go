//go:build !unix

package main

import "os/exec"

// ownGroup has nothing to set on this OS.
func ownGroup(*exec.Cmd) {}
