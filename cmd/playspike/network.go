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
// could not be made or finished (DNS, connection, TLS, timeout, a reset
// connection: the http client's *url.Error, any net.Error), or an answer
// that says to try again (HTTP 429, or a 5xx, Apple's or its CDN's). A
// refusal (401, 403, 404) and every error of Sona's own are not.
func networkError(err error) bool {
	var (
		ue *url.Error
		ne net.Error
		se *applemusic.StatusError
	)
	switch {
	case errors.As(err, &ue), errors.As(err, &ne):
		return true
	case errors.As(err, &se):
		return se.Code == 429 || se.Code >= 500
	}
	return false
}
