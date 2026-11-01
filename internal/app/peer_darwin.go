package app

import (
	"errors"
	"fmt"
	"net"
	"os"

	"golang.org/x/sys/unix"
)

// peerIsUser checks that the process at the other end of c runs as this
// process's user (LOCAL_PEERCRED).
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
		cred  *unix.Xucred
		opErr error
	)
	err = raw.Control(func(fd uintptr) {
		cred, opErr = unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
	})
	if err = errors.Join(err, opErr); err != nil {
		return fmt.Errorf("app: peer credentials: %w", err)
	}
	if int(cred.Uid) != os.Getuid() {
		return fmt.Errorf("app: peer process runs as uid %d", cred.Uid)
	}
	return nil
}
