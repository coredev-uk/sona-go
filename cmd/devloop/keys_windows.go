//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"
)

// rawKeys makes the console deliver single key presses without echo, as on
// Unix (keys_unix.go), and returns what puts it back. Without it the console
// is in line mode, so r and q did nothing until Enter. Ctrl+C still
// interrupts (ENABLE_PROCESSED_INPUT stays). It does nothing when stdin is not
// a console.
func rawKeys() (restore func()) {
	h := windows.Handle(os.Stdin.Fd())
	var mode uint32
	if windows.GetConsoleMode(h, &mode) != nil {
		return func() {}
	}
	if windows.SetConsoleMode(h, mode&^(windows.ENABLE_LINE_INPUT|windows.ENABLE_ECHO_INPUT)) != nil {
		return func() {}
	}
	return func() { _ = windows.SetConsoleMode(h, mode) }
}
