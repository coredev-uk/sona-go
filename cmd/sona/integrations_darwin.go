//go:build darwin

package main

import (
	"github.com/coredev-uk/sona-go/internal/services"
	uiapp "github.com/coredev-uk/sona-go/internal/ui/app"
)

// desktopIntegrations fills in what macOS offers beyond the window
// (services.Platform): all of it MyGo's.
//
//   - Sleep inhibit (MyGo's Power.KeepAwake), the power source for
//     "on battery" (Power.IsOnBattery, polled) and the file chooser
//     (Dialog) work from a bare binary as well as from Sona.app.
//   - Notifications (UNUserNotificationCenter) and the login item
//     (SMAppService) are tied to the app bundle by macOS: outside Sona.app
//     (a source build, bin/sona) they are absent and NeedsAppBundle says why.
func desktopIntegrations(out *services.Platform, windows *uiapp.Windows) {
	out.Inhibitor = func(why func() string) services.SleepInhibitor { return newMacInhibitor(why) }
	out.Power = openMacPower
	out.Files = newMacFiles(windows.Main)
	if !runsFromAppBundle() {
		out.NeedsAppBundle = true
		return
	}
	out.Notifications = openNotifier
	out.PlainNotifications = true
	out.Autostart = macLogin{}
}
