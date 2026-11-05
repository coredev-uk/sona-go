//go:build !darwin && !linux

package main

import (
	"github.com/coredev-uk/sona-go/internal/services"
	uiapp "github.com/coredev-uk/sona-go/internal/ui/app"
)

// desktopIntegrations: Windows has no notifications, sleep inhibitor, power
// monitor, file chooser or autostart entry here yet; each is absent from
// services.Platform, so Capabilities reports it and the UI says so.
func desktopIntegrations(*services.Platform, *uiapp.Windows) {}
