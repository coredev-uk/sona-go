package cli

import "slices"

// Requests of Sona to its own scrobbling add-ons (`lastfm`, `listenbrainz`;
// #302). They are the second kind of request Sona sends an add-on, after
// invoke, and the only ones that exist for bundled add-ons alone: Session.Request
// refuses to send one to any other add-on. The names are fixed (there is no
// general "call this method" request, which would be a new surface), and the
// params carry listens that already passed the one private filter
// (privatemode.Mode.Hidden): the add-on only does the HTTP.
const (
	// AddonMethodScrobbleNowPlaying announces the song that plays.
	AddonMethodScrobbleNowPlaying = "scrobble.now-playing"
	// AddonMethodScrobbleSubmit delivers completed listens.
	AddonMethodScrobbleSubmit = "scrobble.submit"
	// AddonMethodScrobbleStatus asks whether the service is usable, and for a
	// credential not yet stored, whose account it is.
	AddonMethodScrobbleStatus = "scrobble.status"
)

// HostMethods are the requests Sona may send a bundled add-on, besides
// invoke. Nothing else is ever sent.
var HostMethods = []string{AddonMethodScrobbleNowPlaying, AddonMethodScrobbleSubmit, AddonMethodScrobbleStatus}

// IsHostMethod reports whether method is one of HostMethods.
func IsHostMethod(method string) bool { return slices.Contains(HostMethods, method) }

// Error codes of the scrobbling requests, which the host maps back to the
// scrobble package's errors, so the queue and its backoff act as they did when
// the HTTP ran inside Sona. Any other failure (the network, a server error) is
// "failed" and is retried.
const (
	// AddonErrCredentialsRejected: the service refused the credentials
	// (scrobble.ErrAuth): delivery stops until new ones are set.
	AddonErrCredentialsRejected = "credentials_rejected"
	// AddonErrSubmissionRejected: the service refused the submission itself
	// (scrobble.ErrRejected): it is dropped.
	AddonErrSubmissionRejected = "submission_rejected"
	// AddonErrAuthorisationPending: the user hasn't approved yet
	// (scrobble.ErrPending).
	AddonErrAuthorisationPending = "authorisation_pending"
	// AddonErrAuthorisationExpired: the approval timed out
	// (scrobble.ErrExpired).
	AddonErrAuthorisationExpired = "authorisation_expired"
	// AddonErrNotConfigured: the build has no account for the service
	// (scrobble.ErrNotConfigured).
	AddonErrNotConfigured = "not_configured"
	// AddonErrNetwork: the add-on couldn't reach the service. The host retries
	// and, in a connect flow, shows it as a network failure.
	AddonErrNetwork = "network"
)

// ScrobbleTrack is a song as a scrobbling service takes it.
type ScrobbleTrack struct {
	Artist     string `json:"artist"`
	Title      string `json:"title"`
	Album      string `json:"album,omitempty"`
	DurationMs int64  `json:"durationMs,omitempty"`
}

// ScrobbleListen is a completed listen: the track and when it started, in
// Unix seconds.
type ScrobbleListen struct {
	ScrobbleTrack
	ListenedAt int64 `json:"listenedAt"`
}

// ScrobbleNowPlayingParams is the params of scrobble.now-playing. Credential
// is the sign-in Sona holds for the account (the Last.fm session key, the
// ListenBrainz token).
type ScrobbleNowPlayingParams struct {
	Credential string        `json:"credential"`
	Track      ScrobbleTrack `json:"track"`
}

// ScrobbleSubmitParams is the params of scrobble.submit, at most as many
// listens as the service takes in one request.
type ScrobbleSubmitParams struct {
	Credential string           `json:"credential"`
	Listens    []ScrobbleListen `json:"listens"`
}

// ScrobbleStatusParams is the params of scrobble.status. With a credential
// the service is asked whose it is; without one only whether the add-on is
// usable at all.
type ScrobbleStatusParams struct {
	Credential string `json:"credential,omitempty"`
}

// ScrobbleResult is the result of every scrobbling request: OK is always
// true (a failure is an error frame), and Account is the user the credential
// belongs to, for scrobble.status of a credential.
type ScrobbleResult struct {
	OK      bool   `json:"ok"`
	Account string `json:"account,omitempty"`
}
