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
func desktopIntegrations(out *services.Platform, _ *uiapp.Windows) {
	out.Notifications = openNotifier
	out.PlainNotifications = true
}
