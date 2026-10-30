package config

// Updates controls the update check.
type Updates struct {
	// Check asks GitHub for the latest Sona release once per start, after
	// the first paint. On by default.
	Check bool `json:"check"`
}
