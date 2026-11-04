//go:build unix

package main

import (
	"os"
	"os/exec"
	"strings"
)

// rawKeys makes the terminal deliver single key presses without echo (so r
// and q need no Enter) and returns what puts it back. It does nothing when
// stdin is not a terminal.
func rawKeys() (restore func()) {
	if fi, err := os.Stdin.Stat(); err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return func() {}
	}
	saved := exec.Command("stty", "-g")
	saved.Stdin = os.Stdin
	out, err := saved.Output()
	if err != nil {
		return func() {}
	}
	set := exec.Command("stty", "-icanon", "-echo", "min", "1")
	set.Stdin = os.Stdin
	if set.Run() != nil {
		return func() {}
	}
	state := strings.TrimSpace(string(out))
	return func() {
		back := exec.Command("stty", state)
		back.Stdin = os.Stdin
		_ = back.Run()
	}
}
