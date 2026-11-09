//go:build linux || (cgo && darwin)

package cdmhost

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// childDir is the working directory of a child.
func childDir() string { return "/" }

// redirectStdout hands the protocol its own copy of stdout (the pipe to the
// parent) and points stdout at stderr.
func redirectStdout() (*os.File, error) {
	fd, err := unix.Dup(1)
	if err != nil {
		return nil, fmt.Errorf("dup stdout: %w", err)
	}
	if err := unix.Dup2(2, 1); err != nil {
		return nil, fmt.Errorf("redirect stdout: %w", err)
	}
	return os.NewFile(uintptr(fd), "cdmhost-protocol"), nil
}
