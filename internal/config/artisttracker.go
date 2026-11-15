package config

// ArtistTracker is the opt-in artist tracker (services/artisttracker.go):
// Sona watches the artists the user has favourited and lists their new
// releases. Every value is valid, so there is nothing to normalise; both
// are off until the user turns them on.
type ArtistTracker struct {
	// Enabled makes Sona check the favourite artists' releases about once a
	// day. Turning it on records what is out already as seen.
	Enabled bool `json:"enabled"`
	// Desktop also shows a desktop notification for a new release (one
	// summary when several arrive in one check), where the platform has
	// notifications. It does nothing while Enabled is off.
	Desktop bool `json:"desktop"`
}
