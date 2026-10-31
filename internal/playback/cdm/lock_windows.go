package cdm

import (
	"os"

	"golang.org/x/sys/windows"
)

// lockPath takes an exclusive lock (LockFileEx) on the file at path
// (created when missing), waiting for its holder; unlock releases it.
func lockPath(path string) (unlock func(), err error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, &Error{Code: CodeDisk, Err: err}
	}
	h := windows.Handle(f.Fd())
	ol := new(windows.Overlapped)
	if err := windows.LockFileEx(h, windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, ol); err != nil {
		_ = f.Close()
		return nil, &Error{Code: CodeDisk, Err: err}
	}
	return func() {
		_ = windows.UnlockFileEx(h, 0, 1, 0, ol)
		_ = f.Close()
	}, nil
}
