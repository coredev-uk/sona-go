//go:build !probe

package main

import (
	"log/slog"
	"net/http"

	"github.com/coredev-uk/sona-go/internal/playback"
)

// Outside a probe build the e2e seam is closed: the real engine and the
// real transport.

func e2eEngine(e playback.Engine, _ *slog.Logger) playback.Engine { return e }

func e2eTransport(t *http.Transport) *http.Transport { return t }
