package app

import (
	"errors"
	"fmt"
	"net"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// sioAFUnixGetPeerPID is SIO_AF_UNIX_GETPEERPID (_WSAIOR(IOC_VENDOR,
// 256)): the process ID at the other end of an AF_UNIX connection.
const sioAFUnixGetPeerPID = 0x58000100

// tryLock takes an exclusive lock (LockFileEx) on the file at path
// (created when missing) without waiting: errLocked when another process
// holds it. Windows releases it when the process ends.
func tryLock(path string) (unlock func(), err error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	h := windows.Handle(f.Fd())
	ol := new(windows.Overlapped)
	err = windows.LockFileEx(h, windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, ol)
	if err != nil {
		_ = f.Close()
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return nil, errLocked
		}
		return nil, err
	}
	return func() {
		_ = windows.UnlockFileEx(h, 0, 1, 0, ol)
		_ = f.Close()
	}, nil
}

// restrictSocket does nothing: the state folder's ACL, inherited from the
// user's profile, already keeps other users out, and Windows has no mode
// bits to set on the socket's reparse point.
func restrictSocket(string) error { return nil }

// refused reports a connection refused: a socket file nobody listens on.
func refused(err error) bool { return errors.Is(err, windows.WSAECONNREFUSED) }

// peerIsUser checks that the process at the other end of c runs as this
// process's user (the user SID of its token).
func peerIsUser(c net.Conn) error {
	uc, ok := c.(*net.UnixConn)
	if !ok {
		return fmt.Errorf("app: %T is not a Unix socket", c)
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return err
	}
	var (
		pid   uint32
		ioErr error
	)
	err = raw.Control(func(fd uintptr) {
		var n uint32
		ioErr = windows.WSAIoctl(windows.Handle(fd), sioAFUnixGetPeerPID, nil, 0,
			(*byte)(unsafe.Pointer(&pid)), uint32(unsafe.Sizeof(pid)), &n, nil, 0)
	})
	if err = errors.Join(err, ioErr); err != nil {
		return fmt.Errorf("app: peer process: %w", err)
	}
	p, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return fmt.Errorf("app: open peer process %d: %w", pid, err)
	}
	defer func() { _ = windows.CloseHandle(p) }()
	var tok windows.Token
	if err := windows.OpenProcessToken(p, windows.TOKEN_QUERY, &tok); err != nil {
		return fmt.Errorf("app: peer process token: %w", err)
	}
	defer func() { _ = tok.Close() }()
	peer, err := tok.GetTokenUser()
	if err != nil {
		return fmt.Errorf("app: peer user: %w", err)
	}
	self, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return fmt.Errorf("app: own user: %w", err)
	}
	if !peer.User.Sid.Equals(self.User.Sid) {
		return fmt.Errorf("app: peer process %d runs as another user (%s)", pid, peer.User.Sid)
	}
	return nil
}
