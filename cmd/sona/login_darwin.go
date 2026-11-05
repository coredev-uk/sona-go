//go:build darwin

package main

import (
	"github.com/egoist/mygo"

	"github.com/coredev-uk/sona-go/internal/config"
)

// macLogin is the login item (Settings → General → Start Sona when you log
// in): MyGo's App.SetOpenAtLogin, which on macOS 13 and later makes Sona a
// login item of the user (SMAppService; System Settings → General → Login
// Items lists it and the user may turn it off there) and on macOS 12 adds a
// launch agent. Both need the app bundle: where there is none
// (platform_darwin.go) the platform has no Autostart at all. It is a
// services.Autostart.
type macLogin struct{}

// Enabled reports whether Sona is a login item the user has not turned off.
func (macLogin) Enabled() (bool, error) { return mygo.App.OpenAtLogin(), nil }

// Enable makes Sona start at login. hidden is not passed on: a login item
// starts the app with no arguments, so "Start in the tray" is read from the
// settings when Sona starts (loginStartsHidden).
func (macLogin) Enable(bool) error { return mygo.App.SetOpenAtLogin(true) }

// Disable removes the login item; a missing one is not an error.
func (macLogin) Disable() error { return mygo.App.SetOpenAtLogin(false) }

// loginStartsHidden reports whether this start is the system's launch at
// login and the user asked for it with no window ("Start in the tray"). A
// login item gets no --hidden argument, so macOS tells MyGo it opened the
// app at login (the launch event's login-item flag). Call it once MyGo is
// ready.
func loginStartsHidden(d config.Desktop) bool {
	return d.Autostart && d.StartHidden && mygo.App.WasOpenedAtLogin()
}
