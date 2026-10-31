//go:build unix

package cdm

import (
	"os"

	"golang.org/x/sys/unix"
)

// lockPath takes an exclusive flock on the file at path (created when
// missing), waiting for its holder; unlock releases it.
func lockPath(path string) (unlock func(), err error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, &Error{Code: CodeDisk, Err: err}
	}
	for {
		err = unix.Flock(int(f.Fd()), unix.LOCK_EX)
		if err != unix.EINTR {
			break
		}
	}
	if err != nil {
		_ = f.Close()
		return nil, &Error{Code: CodeDisk, Err: err}
	}
	return func() {
		_ = unix.Flock(int(f.Fd()), unix.LOCK_UN)
		_ = f.Close()
	}, nil
}
