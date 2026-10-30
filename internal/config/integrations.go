package config

// Integrations switches Sona's desktop and Discord integrations. Every
// value is valid, so there is nothing to normalise.
type Integrations struct {
	// Notifications shows a notification when the track changes (not while
	// the main window is focused).
	Notifications bool `json:"notifications"`
	// InhibitSleep keeps the system from suspending while music plays.
	InhibitSleep bool `json:"inhibitSleep"`
	// DiscordPresence shows the playing track as the Discord status
	// (opt-in).
	DiscordPresence bool `json:"discordPresence"`
}

func defaultIntegrations() Integrations {
	return Integrations{Notifications: true, InhibitSleep: true}
}
