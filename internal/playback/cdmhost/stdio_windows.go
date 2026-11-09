//go:build cgo

package cdmhost

/*
#include <io.h>

// sona_crt_stdout_to_stderr points the C runtime's stdout at stderr, so
// what C code (GStreamer, the CDM) prints cannot corrupt the protocol.
// Best effort: a process without a C-level stdout has nothing to redirect.
static void sona_crt_stdout_to_stderr(void) {
  if (_get_osfhandle(1) != -1 && _get_osfhandle(2) != -1) _dup2(2, 1);
}
*/
import "C"

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// childDir is the working directory of a child: not Sona's folder, and
// nowhere a library could be planted for the child to find.
func childDir() string {
	if d := os.Getenv("SystemRoot"); d != "" {
		return d
	}
	return `C:\`
}

// redirectStdout hands the protocol its own copy of stdout (the pipe to the
// parent) and points stdout, Go's and the C runtime's, at stderr.
func redirectStdout() (*os.File, error) {
	out, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	if err != nil || out == 0 || out == windows.InvalidHandle {
		return nil, fmt.Errorf("no stdout to carry the protocol: %v", err)
	}
	errh, err := windows.GetStdHandle(windows.STD_ERROR_HANDLE)
	if err != nil || errh == 0 || errh == windows.InvalidHandle {
		return nil, fmt.Errorf("no stderr to redirect stdout to: %v", err)
	}
	self := windows.CurrentProcess()
	var dup windows.Handle
	if err := windows.DuplicateHandle(self, out, self, &dup, 0, false, windows.DUPLICATE_SAME_ACCESS); err != nil {
		return nil, fmt.Errorf("duplicate stdout: %w", err)
	}
	if err := windows.SetStdHandle(windows.STD_OUTPUT_HANDLE, errh); err != nil {
		return nil, fmt.Errorf("redirect stdout: %w", err)
	}
	C.sona_crt_stdout_to_stderr()
	return os.NewFile(uintptr(dup), "cdmhost-protocol"), nil
}
