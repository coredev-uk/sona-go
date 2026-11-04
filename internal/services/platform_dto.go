package services

// Capabilities is what this platform and build support
// (PlatformService.Capabilities). A false field means the feature can't
// work here: the UI hides its control or says why, never shows it
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
	// MotionArtwork: motion covers can be decoded here (a video decoder
	// for Apple's H.264 motion artwork: GStreamer on Linux and macOS).
	// False on Windows until its decoder lands: covers show their stills.
	MotionArtwork bool `json:"motionArtwork"`
	// FileChooser: the native file dialog (settings import and export).
	FileChooser bool `json:"fileChooser"`
	// CLI: `sona <command>` reaches the running instance, and a second
	// launch hands its links to it.
	CLI bool `json:"cli"`
	// DiscordPresence: Sona can reach the Discord app on this OS (the
	// build also needs a client ID: integrations:status discord.configured).
	DiscordPresence bool `json:"discordPresence"`
	// DeveloperTools: MyGo's inspector is on for this run (a development
	// build started with SONA_INSPECTOR=1); DesktopService.OpenDeveloperTools
	// shows it.
	DeveloperTools bool `json:"developerTools"`
	// WindowControls is how the main window's title bar works here; the
	// UI lays out its top bar from it and the title bar setting.
	WindowControls WindowControls `json:"windowControls"`
}

// WindowControls is how title bars work on this platform
// (internal/windowchrome). With Settings → General → Title bar
// (config.TitleBar.Style, as the window was created: Style) it gives the
// main window's kind:
//   - Native "inset" (macOS): always "native-inset", the system's buttons
//     inside Sona's top bar, which keeps room for them at the start;
//   - Native "frame" (Windows, Linux): the style, with "auto" standing for
//     Automatic, gives "drawn" (Sona's own title bar in a frameless
//     window; "sona" with Start and End's buttons, "sona-no-buttons"
//     without) or "none" ("desktop": the desktop's own title bar, or none
//     on a tiling compositor).
type WindowControls struct {
	// Native is "inset" (macOS) or "frame" (Windows, Linux).
	Native string `json:"native"`
	// Automatic is what the "auto" style stands for here: "sona" on
	// Windows and the stacking Linux desktops Sona knows, "desktop" on
	// tiling compositors and unknown desktops (and macOS, which ignores
	// the setting).
	Automatic string `json:"automatic"`
	// Desktop names the desktop or compositor Sona recognised ("GNOME",
	// "KDE Plasma", "sway"); "" when none (always on macOS and Windows).
	Desktop string `json:"desktop"`
	// Start and End are the window buttons on each side, in reading order:
	// "minimize", "maximize", "close" (the desktop's own layout on Linux;
	// on macOS the traffic lights' order, for the room they take).
	Start []string `json:"start"`
	End   []string `json:"end"`
	// Style is the title bar style (config.TitleBar.Style) the main window
	// was created with: a change in Settings applies from the next start.
	Style string `json:"style"`
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
	// Video: music videos play (in SD, the only quality Apple licenses a
	// software CDM), their pictures shown by the UI's video view.
	Video bool `json:"video"`
}
