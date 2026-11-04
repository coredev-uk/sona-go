package config

// Updates controls the update check.
type Updates struct {
	// Check asks GitHub for the latest Sona release once per start, after
	// the first paint. On by default.
	Check bool `json:"check"`
	// Prereleases offers test versions (-rc, -beta) too, so a build on one
	// updates to the next. Off by default; macOS and Windows installs only.
	Prereleases bool `json:"prereleases"`
}
