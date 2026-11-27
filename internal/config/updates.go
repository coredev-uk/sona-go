package config

// Updates controls the update check.
type Updates struct {
	// Check asks GitHub for the latest Sona release once per start, after
	// the first paint. On by default.
	Check bool `json:"check"`
	// Prereleases offers test versions (-rc, -beta) too, so a build on one
	// updates to the next. Off by default; macOS and Windows installs only.
	Prereleases bool `json:"prereleases"`
	// WhatsNew shows the release notes of the version just installed, once,
	// the first time Sona starts after an update. On by default. It asks
	// GitHub for those notes after the first paint.
	WhatsNew bool `json:"whatsNew"`
	// WhatsNewSeen is the newest version whose notes were shown or turned out
	// to have none ("" on a first run, which the service fills in). Only the
	// service writes it; it stays on this computer (settings transfer leaves
	// it).
	WhatsNewSeen string `json:"whatsNewSeen"`
}
