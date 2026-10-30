package services

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"

	"github.com/coredev-uk/sona-go/internal/app"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/playback/widevine"
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
// entry, the remembered route and the native file chooser. The close
// action, start page and window size are settings (config.Desktop,
// config.Window) that the shell reads; this service applies the autostart
// toggle when settings change and keeps the frontend's copy of the
// settings Go writes itself current.
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

// ChooseCDMPath opens the desktop's file chooser (xdg-desktop-portal) for
// libwidevinecdm.so and returns the chosen path, or "" when the user
// cancels. It does not save the choice.
func (d *DesktopService) ChooseCDMPath(ctx context.Context) (string, error) {
	if d.o.Bus == nil {
		return "", errors.New("the file chooser needs a D-Bus session bus")
	}
	return app.OpenFile(ctx, d.o.Bus, app.OpenFileOptions{
		Title:       "Choose the Widevine CDM",
		AcceptLabel: "Use this CDM",
		Filters: []app.FileFilter{
			{Name: "Widevine CDM (libwidevinecdm.so)", Patterns: []string{"libwidevinecdm.so"}},
			{Name: "Shared libraries", Patterns: []string{"*.so"}},
		},
		Folder: cdmFolder(d.o.Settings.Get().CDMPath),
	})
}

// cdmFolder is where the chooser opens: the CDM in use (the override, else
// the one found), or the portal's default.
func cdmFolder(override string) string {
	if p, _, err := widevine.FindCDM(override); err == nil {
		return filepath.Dir(p)
	}
	if override != "" {
		return filepath.Dir(override)
	}
	return ""
}

// settingsChanged writes or removes the autostart entry when its settings
// change, and re-sends the settings when Go changed a field the user does
// not edit (window size, last route, autostart from reconcile): the
// frontend saves whole Settings objects, so a stale copy would put old
// values back.
func (d *DesktopService) settingsChanged(prev, next config.Settings) {
	p, n := prev.Desktop, next.Desktop
	if p.Autostart != n.Autostart || (n.Autostart && p.StartHidden != n.StartHidden) {
		if err := applyAutostart(n); err != nil {
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
func applyAutostart(d config.Desktop) error {
	path, err := app.AutostartPath()
	if err != nil {
		return err
	}
	if !d.Autostart {
		return app.RemoveAutostart(path)
	}
	exe, err := app.LaunchCommand()
	if err != nil {
		return err
	}
	argv := []string{exe}
	if d.StartHidden {
		argv = append(argv, app.HiddenFlag)
	}
	return app.WriteAutostart(path, argv)
}

// reconcileAutostart adopts the entry's presence as the setting, and
// rewrites an existing entry so it runs this build (an AppImage or binary
// that moved).
func (d *DesktopService) reconcileAutostart() {
	path, err := app.AutostartPath()
	if err != nil {
		d.o.Logger.Warn("desktop: autostart", "err", err)
		return
	}
	exists := app.AutostartExists(path)
	cur := d.o.Settings.Get().Desktop
	if exists != cur.Autostart {
		// settingsChanged writes or removes the entry to match.
		if _, err := d.o.Settings.Update(func(s *config.Settings) { s.Desktop.Autostart = exists }); err != nil {
			d.o.Logger.Warn("desktop: autostart", "err", err)
		}
		return
	}
	if exists {
		if err := applyAutostart(cur); err != nil {
			d.o.Logger.Warn("desktop: autostart refresh", "err", err)
		}
	}
}
