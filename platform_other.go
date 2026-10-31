//go:build !linux

package main

import (
	"fmt"
	"log/slog"
	"os"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/coredev-uk/sona-go/internal/app"
	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/services"
)

// macOS and Windows (and any other OS): the UI without playback. Sona has
// no playback engine, media controls, notifications, sleep inhibitor,
// power monitor, file chooser, autostart entry, single instance or
// command transport here yet; each is absent from services.Platform, so
// Capabilities reports it and the frontend hides or explains it. The
// tray is Wails' own on every OS.

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

// childMain does nothing: there is no CDM host child here.
func childMain() {}

// runCommand fails: there is no transport to a running instance here.
func runCommand(args []string) int {
	_, _ = fmt.Fprintf(os.Stderr, "sona: %s: commands to a running Sona aren't available on %s yet\n", args[0], runtime.GOOS)
	return 1
}

// newEngine is the engine for a platform without one: every play fails
// with playback.ErrUnsupported.
func newEngine(*applemusic.Client, config.Settings, *slog.Logger) playback.Engine {
	return playback.NewUnsupported()
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
