//go:build !darwin && !linux && !windows

package main

import (
	"github.com/coredev-uk/sona-go/internal/services"
	uiapp "github.com/coredev-uk/sona-go/internal/ui/app"
)

// desktopIntegrations: an OS other than Linux, macOS and Windows has no
// notifications, sleep inhibitor, power monitor, file chooser or autostart
// entry; each is absent from services.Platform, so Capabilities reports it
// and the UI says so.
func desktopIntegrations(*services.Platform, *uiapp.Windows) {}
