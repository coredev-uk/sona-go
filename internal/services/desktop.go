package services

import (
	"sync/atomic"

	"github.com/coredev-uk/sona-go/internal/config"
)

// Windows is the desktop shell's window control, provided by main
// (internal/shell).
type Windows interface {
	// ShowMain raises the main window and closes the mini player.
	ShowMain()
	// ToggleMini switches between the mini player and the main window.
	ToggleMini()
	// CloseMini closes the mini player as its window's close button
	// would: to the background, or quitting (Settings → Desktop).
	CloseMini()
	// ResetWindowSize returns the main window to its default size.
	ResetWindowSize()
}

// DesktopService is Sona as a desktop app: its windows, the autostart
// entry and the remembered route. The close action, start page and window
// size are settings (config.Desktop, config.Window) that the shell reads;
// this service applies the autostart toggle when settings change and keeps
// the frontend's copy of the settings Go writes itself current.
type DesktopService struct {
	o *Env
	// undoing is set while an autostart change that failed is reverted, so
	// a failure there is not reverted again.
	undoing atomic.Bool
}

func newDesktopService(e *Env) *DesktopService {
	d := &DesktopService{o: e}
	e.Settings.OnChange(d.settingsChanged)
	// The autostart file may have been removed or disabled from the
	// desktop's own settings since the last run: the file wins. Local
	// file work, off the start-up path.
	e.Go("desktop: autostart", d.reconcileAutostart)
	return d
}

// ShowMainWindow brings the main window back (the mini player's expand
// button).
func (d *DesktopService) ShowMainWindow() { d.o.Windows.ShowMain() }

// ToggleMiniPlayer switches between the mini player and the main window.
func (d *DesktopService) ToggleMiniPlayer() { d.o.Windows.ToggleMini() }

// CloseMiniPlayer is the mini player's close button: it keeps playing in
// the background or quits, like closing the main window.
func (d *DesktopService) CloseMiniPlayer() { d.o.Windows.CloseMini() }

// ResetWindowSize returns the main window to its default size.
func (d *DesktopService) ResetWindowSize() { d.o.Windows.ResetWindowSize() }

// SetLastRoute remembers the main window's route ("/album/1?song=2") for
// the next start. It is kept only while "resume where I left off" is on;
// an invalid route is dropped.
func (d *DesktopService) SetLastRoute(route string) error {
	cur := d.o.Settings.Get().Desktop
	if !cur.ResumeLastRoute || cur.LastRoute == route {
		return nil
	}
	_, err := d.o.Settings.Update(func(s *config.Settings) { s.Desktop.LastRoute = route })
	return err
}

// settingsChanged writes or removes the autostart entry when its settings
// change, and re-sends the settings when Go changed a field the user does
// not edit (window size, last route, autostart from reconcile): the
// frontend saves whole Settings objects, so a stale copy would put old
// values back.
func (d *DesktopService) settingsChanged(prev, next config.Settings) {
	p, n := prev.Desktop, next.Desktop
	if p.Autostart != n.Autostart || (n.Autostart && p.StartHidden != n.StartHidden) {
		if err := d.applyAutostart(n); err != nil {
			d.o.Logger.Warn("desktop: autostart", "on", n.Autostart, "err", err)
			if !d.undoing.Load() {
				// Observers must not call Update; undo it right after so
				// the toggle shows the entry's real state.
				d.o.Go("desktop: autostart undo", func() {
					d.undoing.Store(true)
					defer d.undoing.Store(false)
					if _, err := d.o.Settings.Update(func(s *config.Settings) { s.Desktop.Autostart = p.Autostart }); err != nil {
						d.o.Logger.Warn("desktop: autostart undo", "err", err)
					}
				})
			}
		}
	}
	if prev.Window != next.Window || p.LastRoute != n.LastRoute || p.Autostart != n.Autostart {
		d.o.Emit.Emit(EventSettingsChanged, next)
	}
}

// applyAutostart makes the autostart entry match d.
func (d *DesktopService) applyAutostart(set config.Desktop) error {
	a := d.o.Platform.Autostart
	switch {
	case a == nil && set.Autostart:
		return unsupported("starting at login")
	case a == nil:
		return nil
	case !set.Autostart:
		return a.Disable()
	}
	return a.Enable(set.StartHidden)
}

// reconcileAutostart adopts the entry's presence as the setting, and
// rewrites an existing entry so it runs this build (an AppImage or binary
// that moved). Without an autostart on this platform the setting is
// turned off.
func (d *DesktopService) reconcileAutostart() {
	exists := false
	if a := d.o.Platform.Autostart; a != nil {
		var err error
		if exists, err = a.Enabled(); err != nil {
			d.o.Logger.Warn("desktop: autostart", "err", err)
			return
		}
	}
	cur := d.o.Settings.Get().Desktop
	if exists != cur.Autostart {
		// settingsChanged writes or removes the entry to match.
		if _, err := d.o.Settings.Update(func(s *config.Settings) { s.Desktop.Autostart = exists }); err != nil {
			d.o.Logger.Warn("desktop: autostart", "err", err)
		}
		return
	}
	if exists {
		if err := d.applyAutostart(cur); err != nil {
			d.o.Logger.Warn("desktop: autostart refresh", "err", err)
		}
	}
}
