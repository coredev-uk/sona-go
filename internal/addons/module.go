package addons

import (
	"io"
	"os"
)

// MaxModuleBytes bounds a WebAssembly module. A Go module for wasip1 with the
// standard library's `net/http` is about 10 MiB; this is a generous ceiling
// that still keeps a hostile file from filling memory before it is compiled.
const MaxModuleBytes = 32 << 20

// ReadModule reads the WebAssembly module at exe, the path Entry.Exe gives,
// and returns its bytes: the bytes that are compiled and run, so a file
// swapped after this returns changes nothing (the counterpart of
// OpenVerified for a native program, on every OS).
//
// The checks are made on the open file: it is a regular file the user owns
// that group and others can't write (on Unix), no larger than
// MaxModuleBytes, and it is the file at exe now (a path swapped for another
// file or a link between the scan and this call is refused).
func ReadModule(exe string) ([]byte, error) {
	f, err := os.Open(exe)
	if err != nil {
		return nil, problem(ProblemNoExecutable, "open %s: %v", exe, err)
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	if err != nil {
		return nil, problem(ProblemUnreadable, "%v", err)
	}
	if !fi.Mode().IsRegular() {
		return nil, problem(ProblemNoExecutable, "the module is not a regular file")
	}
	if err := checkFileSafe(fi); err != nil {
		return nil, err
	}
	// exe was resolved link-free: if it is a link now, or another file, it
	// changed since the scan.
	if at, err := os.Lstat(exe); err != nil || !os.SameFile(fi, at) {
		return nil, problem(ProblemEscapes, "the module changed since it was checked")
	}
	if fi.Size() > MaxModuleBytes {
		return nil, problem(ProblemNoExecutable, "the module is %d bytes, more than %d", fi.Size(), MaxModuleBytes)
	}
	b, err := io.ReadAll(io.LimitReader(f, MaxModuleBytes+1))
	if err != nil {
		return nil, problem(ProblemUnreadable, "%v", err)
	}
	if len(b) > MaxModuleBytes {
		return nil, problem(ProblemNoExecutable, "the module is more than %d bytes", MaxModuleBytes)
	}
	if len(b) < 8 || string(b[:4]) != "\x00asm" {
		return nil, problem(ProblemNoExecutable, "not a WebAssembly module (no \\0asm header)")
	}
	return b, nil
}
