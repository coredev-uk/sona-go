//go:build !linux && !darwin && !windows

package main

import (
	"log/slog"

	"github.com/coredev-uk/sona-go/internal/mpris"
	"github.com/coredev-uk/sona-go/internal/services"
)

// mediaControls: no media controls on other systems.
func mediaControls(*slog.Logger) func(mpris.Player) (services.MediaControls, error) { return nil }
