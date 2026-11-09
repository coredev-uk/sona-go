//go:build linux || (cgo && (darwin || windows))

package main

import (
	"errors"
	"net"
	"net/url"

	"github.com/coredev-uk/sona-go/internal/applemusic"
)

// exitNetwork is the status a -preview run ends with when it failed on the
// network (networkError) and not in the code under test: EX_TEMPFAIL of
// sysexits.h. A CI step that reaches Apple's catalogue (ci.yml's Windows
// job) runs the preview again on this status and fails on any other.
const exitNetwork = 75

// networkError reports whether err is the network's doing: a request that
// could not be made or finished (the http client's *url.Error: DNS,
// connection, TLS, its own timeout; a *net.OpError or *net.DNSError of a
// read or a lookup), or an answer that says to try again (HTTP 429, or a
// 5xx, Apple's or its CDN's). A refusal (401, 403, 404) and every error of
// Sona's own are not; neither is a bare context.DeadlineExceeded, which
// also is a net.Error (a player that hung is not the network's doing).
func networkError(err error) bool {
	var (
		ue *url.Error
		oe *net.OpError
		de *net.DNSError
		se *applemusic.StatusError
	)
	switch {
	case errors.As(err, &ue), errors.As(err, &oe), errors.As(err, &de):
		return true
	case errors.As(err, &se):
		return se.Code == 429 || se.Code >= 500
	}
	return false
}
