//go:build probe

package main

import (
	"context"
	"crypto/tls"
	"log/slog"
	"net"
	"net/http"
	"os"

	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/fakeengine"
)

// The e2e smoke (e2e/) runs a probe build against a fixture profile and no
// network. Two environment variables, read only here, close what the smoke
// must not reach:
//
//   - SONA_E2E=1 replaces the playback engine with fakeengine: items "play"
//     at once with no Apple, CDM or audio device.
//   - SONA_E2E_CDN=host:port sends every HTTP request of the app, whatever
//     its URL, to that address (TLS not verified): the smoke's local server
//     answers the fixture's generated covers and 404s the rest.
//
// A build without the probe tag has neither (e2e_off.go).

func e2eEngine(real playback.Engine, log *slog.Logger) playback.Engine {
	if os.Getenv("SONA_E2E") != "1" {
		return real
	}
	_ = real.Close()
	log.Warn("e2e: fake playback engine, nothing is played")
	return fakeengine.New()
}

func e2eTransport(t *http.Transport) *http.Transport {
	addr := os.Getenv("SONA_E2E_CDN")
	if addr == "" {
		return t
	}
	t.Proxy = nil
	t.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}
	t.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // a probe build's local fixture server
	return t
}
