package mut

import (
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"io"
	"net/netip"
	"strconv"
	"strings"
)

// socketUID finds the TCP socket bound to local and connected to remote in a
// Linux /proc/net/tcp or /proc/net/tcp6 table and returns its owner's uid.
// The kernel prints each address as the in-memory (network order) bytes
// read as native-endian 32-bit words in hex, then ":" and the port in hex;
// the uid is the eighth column, mapped into the reader's user namespace.
// The parser is portable so its tests run everywhere; only the Linux
// listener (ownerOnly) reads the real tables.
func socketUID(table io.Reader, local, remote netip.AddrPort) (uint32, bool) {
	sc := bufio.NewScanner(table)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 8 {
			continue
		}
		l, ok := parseProcAddr(f[1])
		if !ok || l != local {
			continue
		}
		if r, ok := parseProcAddr(f[2]); !ok || r != remote {
			continue
		}
		uid, err := strconv.ParseUint(f[7], 10, 32)
		if err != nil {
			return 0, false
		}
		return uint32(uid), true
	}
	return 0, false
}

// parseProcAddr decodes "0100007F:1F90" (IPv4) or the 32-digit IPv6 form,
// unmapping IPv4-mapped IPv6 addresses.
func parseProcAddr(s string) (netip.AddrPort, bool) {
	hexAddr, hexPort, ok := strings.Cut(s, ":")
	if !ok || (len(hexAddr) != 8 && len(hexAddr) != 32) {
		return netip.AddrPort{}, false
	}
	raw, err := hex.DecodeString(hexAddr)
	if err != nil {
		return netip.AddrPort{}, false
	}
	var b [16]byte
	for i := 0; i < len(raw); i += 4 {
		binary.NativeEndian.PutUint32(b[i:], binary.BigEndian.Uint32(raw[i:]))
	}
	port, err := strconv.ParseUint(hexPort, 16, 16)
	if err != nil {
		return netip.AddrPort{}, false
	}
	var addr netip.Addr
	if len(raw) == 4 {
		addr = netip.AddrFrom4([4]byte(b[:4]))
	} else {
		addr = netip.AddrFrom16(b).Unmap()
	}
	return netip.AddrPortFrom(addr, uint16(port)), true
}
