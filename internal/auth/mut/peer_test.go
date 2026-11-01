package mut

import (
	"encoding/binary"
	"fmt"
	"net/netip"
	"strings"
	"testing"
)

// procAddr encodes an address the way the kernel prints it in /proc/net/tcp*.
func procAddr(ap netip.AddrPort, v6 bool) string {
	var raw []byte
	if v6 {
		b := ap.Addr().As16()
		raw = b[:]
	} else {
		b := ap.Addr().As4()
		raw = b[:]
	}
	var sb strings.Builder
	for i := 0; i < len(raw); i += 4 {
		fmt.Fprintf(&sb, "%08X", binary.NativeEndian.Uint32(raw[i:]))
	}
	return fmt.Sprintf("%s:%04X", sb.String(), ap.Port())
}

func procRow(local, remote netip.AddrPort, uid int, v6 bool) string {
	return fmt.Sprintf("   0: %s %s 01 00000000:00000000 00:00000000 00000000  %d        0 12345 1 0000000000000000 20 4 30 10 -1",
		procAddr(local, v6), procAddr(remote, v6), uid)
}

func TestSocketUIDFindsTheClientRow(t *testing.T) {
	server := netip.MustParseAddrPort("127.0.0.1:41000")
	client := netip.MustParseAddrPort("127.0.0.1:52000")
	other := netip.MustParseAddrPort("127.0.0.1:52001")
	head := "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n"
	table := head + strings.Join([]string{
		procRow(server, netip.MustParseAddrPort("0.0.0.0:0"), 1000, false), // listener
		procRow(server, client, 1000, false),                               // Sona's accepted end
		procRow(other, server, 1000, false),                                // a different client
		procRow(client, server, 1001, false),                               // the client: another user
	}, "\n")

	if uid, ok := socketUID(strings.NewReader(table), client, server); !ok || uid != 1001 {
		t.Fatalf("client row: uid %d ok %v, want 1001 (not the server end's 1000)", uid, ok)
	}
	if _, ok := socketUID(strings.NewReader(table), netip.MustParseAddrPort("127.0.0.1:9"), server); ok {
		t.Fatal("found a socket that is not in the table")
	}

	// A dual-stack client socket shows up in tcp6 as an IPv4-mapped address.
	mapped := netip.AddrPortFrom(netip.AddrFrom16(client.Addr().As16()), client.Port())
	mappedSrv := netip.AddrPortFrom(netip.AddrFrom16(server.Addr().As16()), server.Port())
	v6 := head + procRow(mapped, mappedSrv, 1002, true)
	if uid, ok := socketUID(strings.NewReader(v6), client, server); !ok || uid != 1002 {
		t.Fatalf("tcp6 row: uid %d ok %v", uid, ok)
	}
}

func TestParseProcAddrRejectsMalformed(t *testing.T) {
	for _, s := range []string{"", "0100007F", "0100007F:", "0100007F:XYZ", "zz00007F:0050", "00007F:0050", "0100007F:10000"} {
		if _, ok := parseProcAddr(s); ok {
			t.Errorf("parseProcAddr(%q) accepted", s)
		}
	}
}
