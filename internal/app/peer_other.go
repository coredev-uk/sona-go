//go:build !linux && !darwin && !windows

package app

import (
	"errors"
	"fmt"
	"net"
)

// peerIsUser can't tell who is at the other end of c here, so the
// connection is refused.
func peerIsUser(net.Conn) error {
	return fmt.Errorf("app: peer credentials: %w", errors.ErrUnsupported)
}
