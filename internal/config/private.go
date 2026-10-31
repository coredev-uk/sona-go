package config

// Private is the saved private listening setting (Settings → Playback).
// The running state can differ: a development build or SONA_PRIVATE
// overrides it for the session (internal/privatemode), and that override
// never writes this value. Every value is valid, so there is nothing to
// normalise.
type Private struct {
	// Enabled keeps plays out of Apple's listening history and
	// recommendations and stops scrobbling, the Discord status and the
	// local listening history. Off by default.
	Enabled bool `json:"enabled"`
}
