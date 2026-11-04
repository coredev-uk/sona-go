package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/coredev-uk/sona-go/internal/app"
	"github.com/coredev-uk/sona-go/internal/cli"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/desktop/inhibit"
	"github.com/coredev-uk/sona-go/internal/desktop/notify"
	"github.com/coredev-uk/sona-go/internal/mpris"
	"github.com/coredev-uk/sona-go/internal/power"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/windowchrome"
)

// Linux: the freedesktop integrations over D-Bus (MPRIS, notifications,
// logind/portal inhibit, UPower, the file chooser portal, single instance
// and commands) and the XDG autostart entry. The CDM host engine is
// engine_cdm.go's.

// dialInstance reaches the running Sona for `sona <command>` over the
// session bus.
func dialInstance() (cli.Transport, error) {
	c, err := app.DialBus()
	if err != nil {
		return nil, err
	}
	return c, nil
}

// platform is the session bus and the single-instance guard on it.
type platform struct {
	log      *slog.Logger
	conn     *dbus.Conn    // nil without a session bus
	instance *app.Instance // nil when the guard is unavailable
}

// openPlatform connects to the session bus and takes the single-instance
// name; it fails with app.ErrForwarded when another Sona runs (act went to
// it). Without a bus Sona runs alone, without the D-Bus integrations.
func openPlatform(_ config.Paths, act app.Activation, log *slog.Logger) (*platform, error) {
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
	return p, nil
}

// Services are the integrations for services.Deps.Platform. Without a
// session bus only the autostart entry remains.
func (p *platform) Services() services.Platform {
	out := services.Platform{Autostart: app.XDGAutostart{}, TitleBar: p.TitleBar()}
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
	out.Inhibitor = func(why func() string) services.SleepInhibitor {
		return inhibit.New(inhibit.Options{
			System:  func() (*dbus.Conn, error) { return dbus.ConnectSystemBus() },
			Session: conn,
			Who:     "Sona",
			Why:     why,
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

// TitleBar is how the main window's title bar works here, for
// PlatformService: the desktop's own frame (MyGo draws no title bar of its
// own yet; Sona's own is phase 3), with no window menu to open.
func (p *platform) TitleBar() windowchrome.Platform {
	d := windowchrome.Detect(os.Getenv)
	return windowchrome.Platform{
		Native:     windowchrome.NativeFrame,
		Automatic:  d.Automatic(),
		Desktop:    d.Name,
		WindowMenu: false,
		Layout:     func() windowchrome.Layout { return buttonLayout(d, p.log) },
	}
}

// buttonLayout reads the desktop's title bar buttons: KWin's on KDE
// Plasma, else the defaults. GTK's own setting (gtk-decoration-layout) was
// read through GTK4 by the Wails shell; MyGo has no call for it, so reading
// it again comes with the title bar in phase 3.
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
	}
	return windowchrome.DefaultLayout()
}

// Serve hands later launches' links to activate and runs commands sent by
// `sona <command>`, while the instance guard is held.
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
