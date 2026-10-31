//go:build !linux

package main

import (
	"fmt"
	"log/slog"
	"os"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/coredev-uk/sona-go/internal/app"
	"github.com/coredev-uk/sona-go/internal/services"
)

// macOS and Windows (and any other OS): the UI without the desktop
// integrations. Sona has no media controls, notifications, sleep
// inhibitor, power monitor, file chooser, autostart entry, single
// instance or command transport here yet; each is absent from
// services.Platform, so Capabilities reports it and the frontend hides or
// explains it. The tray is Wails' own on every OS. The playback engine is
// engine_cdm.go's on macOS, engine_other.go's elsewhere.

var (
	linuxOptions application.LinuxOptions
	macOptions   = application.MacOptions{
		// The shell hides windows rather than closing them and quits
		// itself (Settings → Desktop); keep running with no window.
		ApplicationShouldTerminateAfterLastWindowClosed: false,
	}
	windowsOptions = application.WindowsOptions{
		DisableQuitOnLastWindowClosed: true,
	}
)

// runCommand fails: there is no transport to a running instance here.
func runCommand(args []string) int {
	_, _ = fmt.Fprintf(os.Stderr, "sona: %s: commands to a running Sona aren't available on %s yet\n", args[0], runtime.GOOS)
	return 1
}

// platform has nothing to hold here.
type platform struct{}

// openPlatform never forwards: there is no single-instance guard here.
func openPlatform(app.Activation, *slog.Logger) (*platform, error) { return &platform{}, nil }

// Services offers the tray only.
func (*platform) Services() services.Platform { return services.Platform{Tray: true} }

// Serve does nothing: later launches start their own instance.
func (*platform) Serve(func(app.Activation, bool), app.Commands) {}

// Close does nothing.
func (*platform) Close() {}
