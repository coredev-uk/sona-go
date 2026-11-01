//go:build unix

package app

import (
	"errors"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// tryLock takes an exclusive flock on the file at path (created when
// missing) without waiting: errLocked when another process holds it. The
// system releases it when the process ends, crashes included.
func tryLock(path string) (unlock func(), err error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	for {
		err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err != unix.EINTR {
			break
		}
	}
	if err != nil {
		_ = f.Close()
		if errors.Is(err, unix.EWOULDBLOCK) {
			return nil, errLocked
		}
		return nil, err
	}
	return func() {
		_ = unix.Flock(int(f.Fd()), unix.LOCK_UN)
		_ = f.Close()
	}, nil
}

// restrictSocket makes the socket file the user's alone.
func restrictSocket(path string) error { return os.Chmod(path, 0o600) }

// refused reports a connection refused: a socket file nobody listens on.
func refused(err error) bool { return errors.Is(err, syscall.ECONNREFUSED) }
