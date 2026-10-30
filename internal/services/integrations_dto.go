package services

import "github.com/wailsapp/wails/v3/pkg/application"

// EventIntegrationsStatus carries IntegrationsStatus whenever it changes.
const EventIntegrationsStatus = "integrations:status"

func init() { application.RegisterEvent[IntegrationsStatus](EventIntegrationsStatus) }

// Notification and inhibitor states in IntegrationsStatus.
const (
	IntegrationReady       = "ready"       // notifications: the server answered
	IntegrationUnavailable = "unavailable" // the desktop service could not be reached
)

// IntegrationsStatus is what the desktop and Discord integrations are
// doing ("integrations:status" payload). Empty strings mean not tried yet
// (the feature is off, or nothing has needed it since start-up).
type IntegrationsStatus struct {
	// Notifications is "ready", "unavailable" (no notification server) or "".
	Notifications string `json:"notifications"`
	// Inhibit is the backend holding the suspend inhibitor ("logind" or
	// "portal"), "unavailable" when taking it failed, or "" when none is
	// held.
	Inhibit string        `json:"inhibit"`
	Discord DiscordStatus `json:"discord"`
}

// DiscordStatus is the Discord presence connection.
type DiscordStatus struct {
	// Configured: this build carries a Discord application ID. Without
	// one, presence cannot be turned on.
	Configured bool `json:"configured"`
	// Connected: Sona is talking to the Discord app (while presence is on
	// and Discord is running).
	Connected bool `json:"connected"`
}
