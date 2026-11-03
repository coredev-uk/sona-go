package services

// EventScrobbleChanged carries the ScrobbleStatus whenever a connection,
// a pending count or a rejected credential changes.
const EventScrobbleChanged = "scrobble:changed"

// Scrobbler provider names (ScrobbleService.Disconnect).
const (
	ScrobblerLastFM       = "lastfm"
	ScrobblerListenBrainz = "listenbrainz"
)

// ScrobbleStatus is the runtime state of the scrobbler connections. The
// account names and on/off switches are in config.Settings.Scrobbling.
type ScrobbleStatus struct {
	LastFM       ScrobblerStatus `json:"lastfm"`
	ListenBrainz ScrobblerStatus `json:"listenbrainz"`
	// Unencrypted is set when the keyring is unavailable and the session
	// key and token are kept in 0600 files.
	Unencrypted bool `json:"unencrypted"`
}

// ScrobblerStatus is one provider's state.
type ScrobblerStatus struct {
	// Available is false when this build cannot use the provider (Last.fm
	// without an API account built in).
	Available bool `json:"available"`
	// Connected: a session key or token is stored.
	Connected bool `json:"connected"`
	// Connecting: waiting for the user to approve Sona in the browser.
	Connecting bool `json:"connecting"`
	// NeedsReconnect: the service refused the stored credentials; listens
	// are kept until the account is connected again.
	NeedsReconnect bool `json:"needsReconnect"`
	// Pending is the number of listens waiting to be sent.
	Pending int `json:"pending"`
}
