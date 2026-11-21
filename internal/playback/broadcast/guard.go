package broadcast

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"syscall"
	"time"
)

// A broadcaster's URL comes from Apple's play assets, often as plain http,
// and the playlists and redirects it leads to name more URLs. None of them
// may lead Sona's process to a service on the user's own machine or
// network: a blind GET to 127.0.0.1, a router's admin page or the cloud
// metadata address 169.254.169.254 is the attack. NewClient builds the
// client that refuses those at the one place that cannot be fooled, the
// dial, whatever the URL said, however DNS answers and wherever a redirect
// pointed.

const (
	maxRedirects = 5  // hops one request follows
	maxEntries   = 16 // entries of one playlist tried
)

// ErrPrivateAddress reports a dial refused because the address is not a
// public one.
var ErrPrivateAddress = errors.New("broadcast: refusing a non-public address")

// blocked are the ranges net/netip does not already classify.
var blocked = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),      // "this network"
	netip.MustParsePrefix("100.64.0.0/10"),  // carrier-grade NAT
	netip.MustParsePrefix("192.0.0.0/24"),   // IETF protocol assignments
	netip.MustParsePrefix("198.18.0.0/15"),  // benchmarking
	netip.MustParsePrefix("240.0.0.0/4"),    // reserved, broadcast included
	netip.MustParsePrefix("64:ff9b:1::/48"), // local-use NAT64
	netip.MustParsePrefix("100::/64"),       // discard-only
}

// nat64 is the well-known NAT64 prefix; its last four bytes are an IPv4
// address.
var nat64 = netip.MustParsePrefix("64:ff9b::/96")

// PublicAddr reports whether a is an address a stream may be fetched from:
// not loopback, private (RFC 1918, unique local), link-local (169.254.0.0/16
// holds cloud metadata), carrier-grade NAT, unspecified, multicast or
// reserved. An IPv4 address inside an IPv6 one (IPv4-mapped, NAT64) is
// judged as the IPv4 address.
func PublicAddr(a netip.Addr) bool {
	a = a.Unmap().WithZone("")
	if !a.IsValid() || a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast() || a.IsMulticast() || a.IsUnspecified() {
		return false
	}
	if nat64.Contains(a) {
		b := a.As16()
		return PublicAddr(netip.AddrFrom4([4]byte(b[12:])))
	}
	for _, p := range blocked {
		if p.Contains(a) {
			return false
		}
	}
	return true
}

// dialControl is net.Dialer.Control: it runs with the IP the dialer is
// about to connect to, after name resolution, so a name that resolves to a
// private address, one that changes its answer (DNS rebinding) and a
// redirect to an address literal are all refused here.
func dialControl(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	a, err := netip.ParseAddr(host)
	if err != nil {
		return fmt.Errorf("%w: %q is not an address", ErrPrivateAddress, host)
	}
	if !PublicAddr(a) {
		return fmt.Errorf("%w: %s", ErrPrivateAddress, a)
	}
	return nil
}

// NewClient returns the HTTP client for broadcasters' streams: no overall
// timeout (a stream lasts for hours; Play bounds each request itself), a
// transport that dials public addresses only, and redirects limited by
// checkRedirect. It does not use the proxy of the environment: a proxy
// would be the host dialled (usually a private address) and would fetch the
// target itself, past this check.
func NewClient() *http.Client { return newClient(dialControl) }

// newClient is NewClient with control as the dialer's check; tests that
// serve from loopback let their own server's address through.
func newClient(control func(network, address string, c syscall.RawConn) error) *http.Client {
	d := &net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second, Control: control}
	return &http.Client{
		Transport: &http.Transport{
			DialContext:           d.DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          10,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: time.Second,
		},
		CheckRedirect: checkRedirect,
	}
}

// checkRedirect follows at most maxRedirects redirects, only to http or
// https, and never from https to http.
func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > maxRedirects {
		return fmt.Errorf("broadcast: stopped after %d redirects", maxRedirects)
	}
	if s := req.URL.Scheme; s != "http" && s != "https" {
		return fmt.Errorf("broadcast: refusing a redirect to %q", s)
	}
	if req.URL.Scheme == "http" && via[len(via)-1].URL.Scheme == "https" {
		return errors.New("broadcast: refusing a redirect from https to http")
	}
	return nil
}
