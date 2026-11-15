//go:build darwin || windows

package main

import (
	"github.com/egoist/mygo"

	"github.com/coredev-uk/sona-go/internal/config"
)

// desktopLogin is the login item (Settings → General → Start Sona when you
// log in): MyGo's App.SetOpenAtLogin. It is a services.Autostart.
//
//   - macOS 13 and later: Sona becomes a login item of the user
//     (SMAppService; System Settings → General → Login Items lists it and the
//     user may turn it off there); macOS 12 gets a launch agent. Both need
//     the app bundle: where there is none (integrations_darwin.go) the
//     platform has no Autostart at all.
//   - Windows: a value named "Sona" (the app's name, MyGo's appID) in the
//     user's Run key, holding this executable's path; Task Manager's Startup
//     apps turns it off in StartupApproved. The installer's uninstaller
//     deletes both values (build/windows/installer.nsi). A portable copy that
//     moved reads as off, so DesktopService.reconcileAutostart turns the
//     setting off and removes the entry that names the old place.
type desktopLogin struct{}

// Enabled reports whether Sona starts at login and the user has not turned
// it off in the system's settings (on Windows: the entry runs this
// executable).
func (desktopLogin) Enabled() (bool, error) { return mygo.App.OpenAtLogin(), nil }

// Enable makes Sona start at login. hidden is not passed on: the system
// starts the app with no arguments of Sona's, so "Start in the tray" is read
// from the settings when Sona starts (loginStartsHidden).
func (desktopLogin) Enable(bool) error { return mygo.App.SetOpenAtLogin(true) }

// Disable removes the login item; a missing one is not an error.
func (desktopLogin) Disable() error { return mygo.App.SetOpenAtLogin(false) }

// loginStartsHidden reports whether this start is the system's launch at
// login and the user asked for it with no window ("Start in the tray"). The
// login item gets no --hidden argument, so MyGo says whether it opened the
// app at login: macOS's launch event carries a login-item flag, and Windows'
// Run entry passes MyGo's own argument, which MyGo takes off os.Args before
// main. Call it once MyGo is ready.
func loginStartsHidden(d config.Desktop) bool {
	return d.Autostart && d.StartHidden && mygo.App.WasOpenedAtLogin()
}
