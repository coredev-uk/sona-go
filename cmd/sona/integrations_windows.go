package main

import (
	"github.com/coredev-uk/sona-go/internal/services"
	uiapp "github.com/coredev-uk/sona-go/internal/ui/app"
)

// desktopIntegrations fills in what Windows offers beyond the window
// (services.Platform), all of it MyGo's:
//
//   - Track and reminder notifications are notification-area balloons, which
//     Windows 10 and later show as toasts (notify_desktop.go, shared with
//     macOS): a title, a text and a click that brings Sona forward. MyGo
//     shows them from the tray icon, or a temporary icon with the
//     application's when there is none.
//   - Start at login is a Run entry of the user (login_desktop.go, shared
//     with macOS), which Task Manager's Startup apps can turn off; a start
//     from it opens in the tray when "Start in the tray" says so.
//   - Stay awake while playing is a power request (MyGo's Power.KeepAwake,
//     inhibit_desktop.go), and the power source for motion artwork is
//     Power.IsOnBattery, polled and read again on resume and unlock
//     (power_desktop.go); both shared with macOS.
//   - KeepRunning stays nil: MyGo's Power.KeepRunning does nothing on
//     Windows yet (power throttling and EcoQoS stay on), so nothing reports
//     a hold that isn't there.
func desktopIntegrations(out *services.Platform, _ *uiapp.Windows) {
	out.Notifications = openNotifier
	out.PlainNotifications = true
	out.Autostart = desktopLogin{}
	out.Inhibitor = func(why func() string) services.SleepInhibitor { return newDesktopInhibitor(why) }
	out.Power = openDesktopPower
}
