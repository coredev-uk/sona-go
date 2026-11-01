package mut

import (
	"net"
	"net/netip"
	"os"
)

// ownerOnly drops every connection whose client socket another user owns.
// On Linux any user can read the sign-in URL from the browser's (or
// xdg-open's) /proc/<pid>/cmdline, so the URL alone must not be enough to
// reach the server. A connection whose client socket cannot be found in
// /proc/net/tcp or tcp6 is dropped too.
func ownerOnly(ln net.Listener) net.Listener {
	return &ownerListener{Listener: ln, uid: uint32(os.Getuid()), tables: []string{"/proc/net/tcp", "/proc/net/tcp6"}}
}

type ownerListener struct {
	net.Listener
	uid    uint32
	tables []string
}

func (l *ownerListener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		if l.owned(c) {
			return c, nil
		}
		_ = c.Close()
	}
}

// owned reports whether the client end of c belongs to l.uid. The client's
// row in the table has the client address as local and the server address
// as remote.
func (l *ownerListener) owned(c net.Conn) bool {
	client, err1 := netip.ParseAddrPort(c.RemoteAddr().String())
	server, err2 := netip.ParseAddrPort(c.LocalAddr().String())
	if err1 != nil || err2 != nil {
		return false
	}
	for _, name := range l.tables {
		f, err := os.Open(name)
		if err != nil {
			continue
		}
		uid, ok := socketUID(f, client, server)
		_ = f.Close()
		if ok {
			return uid == l.uid
		}
	}
	return false
}
