package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/coredev-uk/sona-go/internal/app"
	"github.com/coredev-uk/sona-go/internal/desktop/inhibit"
	"github.com/coredev-uk/sona-go/internal/desktop/notify"
	"github.com/coredev-uk/sona-go/internal/mpris"
	"github.com/coredev-uk/sona-go/internal/power"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/shell"
	"github.com/coredev-uk/sona-go/internal/windowchrome"
)

// Linux: the freedesktop integrations over D-Bus (MPRIS, notifications,
// logind/portal inhibit, UPower, the file chooser portal, single instance
// and commands) and the XDG autostart entry. The CDM host engine is
// engine_cdm.go's.

var (
	linuxOptions = application.LinuxOptions{
		// Also becomes the program name (g_set_prgname) and Wayland app_id.
		ApplicationID: AppID,
	}
	macOptions     application.MacOptions
	windowsOptions application.WindowsOptions
)

// runCommand sends a subcommand (`sona private on`) to the running
// instance over the session bus and returns the exit status.
func runCommand(args []string) int { return app.RunCommand(args, os.Stdout, os.Stderr) }

// platform is the session bus and the single-instance guard on it.
type platform struct {
	log      *slog.Logger
	conn     *dbus.Conn    // nil without a session bus
	instance *app.Instance // nil when the guard is unavailable
}

// openPlatform connects to the session bus and takes the single-instance
// name; it fails with app.ErrForwarded when another Sona runs (act went to
// it). Without a bus Sona runs alone, without the D-Bus integrations.
func openPlatform(act app.Activation, log *slog.Logger) (*platform, error) {
	p := &platform{log: log}
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		log.Warn("no D-Bus session bus: single-instance and MPRIS disabled", "err", err)
	} else {
		p.conn = conn
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		instance, err := app.Acquire(ctx, conn, act, log)
		cancel()
		switch {
		case errors.Is(err, app.ErrForwarded):
			_ = conn.Close()
			return nil, err
		case err != nil:
			log.Warn("single-instance guard unavailable", "err", err)
		default:
			p.instance = instance
		}
	}
	// Without an accessibility bus, WebKit's sandboxed web process warns
	// on every start; tell it there is none (app.HideMissingA11yBus).
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	noA11yBus, err := app.HideMissingA11yBus(ctx, p.conn)
	cancel()
	if err != nil {
		log.Warn("accessibility bus lookup", "err", err)
	}
	if noA11yBus {
		log.Info("startup: no accessibility bus; web content is not exposed to AT-SPI")
	}
	return p, nil
}

// Services are the integrations for services.Deps.Platform. Without a
// session bus only the tray and the autostart entry remain.
func (p *platform) Services() services.Platform {
	out := services.Platform{Autostart: app.XDGAutostart{}, Tray: true, TitleBar: p.TitleBar()}
	conn := p.conn
	if conn == nil {
		return out
	}
	mlog := p.log.With("component", "mpris")
	out.MediaControls = func(player mpris.Player) (services.MediaControls, error) {
		srv, err := mpris.New(conn, player, mlog)
		if err != nil {
			return nil, err
		}
		return srv, nil
	}
	out.Notifications = func(ctx context.Context, onAction func(string)) (services.Notifier, error) {
		n, err := notify.New(ctx, conn, notify.Options{
			AppName:      "Sona",
			AppIcon:      "sona",
			DesktopEntry: mpris.DesktopEntry,
			OnAction:     onAction,
		})
		if err != nil {
			return nil, err
		}
		return n, nil
	}
	out.Inhibitor = func() services.SleepInhibitor {
		return inhibit.New(inhibit.Options{
			System:  func() (*dbus.Conn, error) { return dbus.ConnectSystemBus() },
			Session: conn,
			Who:     "Sona",
			Why:     "Playing music",
		})
	}
	out.Power = func() (services.PowerMonitor, error) {
		mon, err := power.Open()
		if err != nil {
			return nil, err
		}
		return mon, nil
	}
	out.Files = app.Portal{Conn: conn}
	out.Commands = p.instance != nil
	return out
}

// TitleBar is Sona's own title bar on the stacking desktops it recognises
// and the desktop's on tiling compositors and unknown desktops, with the
// buttons laid out as the desktop lays out its own: KWin's (kwinrc) on KDE
// Plasma, GTK's elsewhere.
func (p *platform) TitleBar() windowchrome.Platform {
	d := windowchrome.Detect(os.Getenv)
	return windowchrome.Platform{
		Native:     windowchrome.NativeFrame,
		Automatic:  d.Automatic(),
		Desktop:    d.Name,
		WindowMenu: true,
		Layout:     func() windowchrome.Layout { return buttonLayout(d, p.log) },
	}
}

// buttonLayout reads the desktop's title bar buttons; KWin's defaults
// (and GTK's) when it can't.
func buttonLayout(d windowchrome.Desktop, log *slog.Logger) windowchrome.Layout {
	if d.KDE {
		if path := windowchrome.KWinRC(os.Getenv); path != "" {
			l, err := windowchrome.ReadKWin(path)
			if err == nil {
				return l
			}
			if !errors.Is(err, os.ErrNotExist) {
				log.Warn("title bar: read KWin's buttons", "path", path, "err", err)
			}
		}
		return windowchrome.DefaultLayout()
	}
	if layout := shell.DecorationLayout(); layout != "" {
		return windowchrome.ParseGTK(layout)
	}
	return windowchrome.DefaultLayout()
}

// Serve hands later launches' links to activate and runs commands sent by
// `sona <command>`, while the instance guard is held.
func (p *platform) Serve(activate func(app.Activation, bool), commands app.Commands) {
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

// Close releases the instance name and the bus.
func (p *platform) Close() {
	if p.instance != nil {
		if err := p.instance.Close(); err != nil {
			p.log.Warn("single instance: close", "err", err)
		}
	}
	if p.conn != nil {
		_ = p.conn.Close()
	}
}
