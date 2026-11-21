package config

// ArtistTracker is the opt-in artist tracker (services/artisttracker.go):
// Sona watches the artists the user has favourited and lists their new
// releases. Every value is valid, so there is nothing to normalise; both
// are off until the user turns them on.
type ArtistTracker struct {
	// Enabled makes Sona check the favourite artists' releases about once a
	// day. Turning it on records what is out already as seen.
	Enabled bool `json:"enabled"`
	// Desktop also shows releases as desktop notifications, where the
	// platform has them: a release reminder that came due
	// (services/reminders.go) and what the tracker found (one summary when
	// several arrive in one check). Both are in the notification centre
	// whatever it says. It is independent of Enabled, as reminders are.
	Desktop bool `json:"desktop"`
}
