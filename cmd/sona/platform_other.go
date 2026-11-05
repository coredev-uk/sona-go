//go:build !linux

package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"runtime"
	"time"

	"github.com/coredev-uk/sona-go/build"
	"github.com/coredev-uk/sona-go/internal/app"
	"github.com/coredev-uk/sona-go/internal/cli"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	uiapp "github.com/coredev-uk/sona-go/internal/ui/app"
	"github.com/coredev-uk/sona-go/internal/ui/media"
	"github.com/coredev-uk/sona-go/internal/windowchrome"
)

// macOS and Windows (and any other OS): the UI with single instance and
// `sona <command>` over a local socket in the state folder
// (app.AcquireSocket). Windows has no other desktop integration yet; each is
// absent from services.Platform, so Capabilities reports it and the UI hides
// or explains it. macOS has MyGo's (integrations_darwin.go): sleep inhibit,
// the power source, the file chooser, and, from Sona.app, notifications and
// the login item. Media keys and Now Playing are MyGo's NowPlaying on macOS
// and Windows (nowplaying_desktop.go; internal/nowplaying). The tray is the
// UI's (internal/ui/app): on macOS a menu bar item with a template icon. The
// playback engine is engine_cdm.go's on macOS, engine_other.go's elsewhere.

// dialInstance reaches the running Sona for `sona <command>` through its
// socket in the state folder.
func dialInstance() (cli.Transport, error) {
	paths, err := config.ResolvePaths(os.Getenv)
	if err != nil {
		return nil, err
	}
	c, err := app.DialSocket(paths.State)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// platform is the single-instance claim on the socket.
type platform struct {
	log      *slog.Logger
	instance *app.SocketInstance // nil when the socket is unavailable
}

// openPlatform takes the single-instance lock and socket in the state
// folder; it fails with app.ErrForwarded when another Sona runs (act went
// to it). When the socket can't be set up Sona runs alone, without
// commands.
func openPlatform(paths config.Paths, act app.Activation, log *slog.Logger) (*platform, error) {
	p := &platform{log: log}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	instance, err := app.AcquireSocket(ctx, paths.State, act, log)
	switch {
	case errors.Is(err, app.ErrForwarded):
		return nil, err
	case err != nil:
		log.Warn("single-instance guard unavailable", "err", err)
	default:
		p.instance = instance
	}
	return p, nil
}

// Services offers the title bar, commands and, per OS, the desktop
// integrations (desktopIntegrations).
func (p *platform) Services(windows *uiapp.Windows) services.Platform {
	out := services.Platform{
		TitleBar: p.TitleBar(), Commands: p.instance != nil, MotionArtwork: media.Supported,
		MediaControls: mediaControls(p.log.With("component", "nowplaying")),
	}
	desktopIntegrations(&out, windows)
	return out
}

// TitleBar: on macOS the system's buttons inset into Sona's top bar; on
// Windows Sona's own title bar with Windows' caption buttons; elsewhere the
// system's frame.
func (*platform) TitleBar() windowchrome.Platform {
	switch runtime.GOOS {
	case "darwin":
		return windowchrome.Platform{Native: windowchrome.NativeInset, Automatic: config.TitleBarDesktop, Layout: windowchrome.MacLayout}
	case "windows":
		return windowchrome.Platform{Native: windowchrome.NativeFrame, Automatic: config.TitleBarSona, Layout: windowchrome.WindowsLayout}
	}
	return windowchrome.Platform{Native: windowchrome.NativeFrame, Automatic: config.TitleBarDesktop}
}

// trayTemplate is the tray's template icon: macOS tints it for the menu
// bar; Windows shows the colour icon.
func trayTemplate() []byte {
	if runtime.GOOS == "darwin" {
		return build.TrayTemplate
	}
	return nil
}

// Serve hands later launches' links to activate and runs commands sent by
// `sona <command>`, while the socket is held.
func (p *platform) Serve(activate func(app.Activation, bool), commands cli.Server) {
	if p.instance == nil {
		return
	}
	p.instance.HandleCommands(commands)
	go func() {
		for a := range p.instance.Activations() {
			activate(a, true)
		}
	}()
}

// Close releases the socket and the lock.
func (p *platform) Close() {
	if p.instance != nil {
		if err := p.instance.Close(); err != nil {
			p.log.Warn("single instance: close", "err", err)
		}
	}
}
