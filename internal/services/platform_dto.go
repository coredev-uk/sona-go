package services

// Capabilities is what this platform and build support
// (PlatformService.Capabilities). A false field means the feature can't
// work here: the frontend hides its control or says why, never shows it
// broken. On Linux every field is true when its desktop service exists
// (a session bus for the D-Bus ones).
type Capabilities struct {
	// OS is the Go GOOS: "linux", "darwin" or "windows".
	OS       string               `json:"os"`
	Playback PlaybackCapabilities `json:"playback"`
	// MediaControls: now-playing and media keys in the desktop (MPRIS).
	MediaControls bool `json:"mediaControls"`
	// Notifications: track-change notifications.
	Notifications bool `json:"notifications"`
	// Tray: a tray icon, so closing the window can keep Sona running.
	Tray bool `json:"tray"`
	// Autostart: start at login.
	Autostart bool `json:"autostart"`
	// SleepInhibit: keep the computer awake while music plays.
	SleepInhibit bool `json:"sleepInhibit"`
	// PowerState: the on-battery state ("power:changed") is followed.
	PowerState bool `json:"powerState"`
	// FileChooser: the native file dialog (settings import and export).
	FileChooser bool `json:"fileChooser"`
	// CLI: `sona private …` reaches the running instance, and a second
	// launch hands its links to it.
	CLI bool `json:"cli"`
	// DiscordPresence: Sona can reach the Discord app on this OS (the
	// build also needs a client ID: integrations:status discord.configured).
	DiscordPresence bool `json:"discordPresence"`
	// WebKitGTK: the web view is WebKitGTK (Linux), so its rendering
	// workarounds (Settings → Diagnostics) apply.
	WebKitGTK bool `json:"webkitGtk"`
}

// PlaybackCapabilities is what the playback engine supports.
type PlaybackCapabilities struct {
	// Available: this build has an engine that can play here. Without one
	// every play fails with CodeUnsupported.
	Available bool `json:"available"`
	// Engine is the engine's kind: "cdm" (the Widevine CDM host with
	// GStreamer), or "" when Available is false.
	Engine string `json:"engine"`
	// Volume: Sona sets the playback volume.
	Volume bool `json:"volume"`
	// Equalizer, SoundCheck, Crossfade, OutputDevice: Sona's own output
	// processing and device choice (AudioService).
	Equalizer    bool `json:"equalizer"`
	SoundCheck   bool `json:"soundCheck"`
	Crossfade    bool `json:"crossfade"`
	OutputDevice bool `json:"outputDevice"`
	// PrivateListening: Sona sends every play report itself, so private
	// listening withholds them all.
	PrivateListening bool `json:"privateListening"`
}
