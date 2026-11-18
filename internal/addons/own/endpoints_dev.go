//go:build dev

package own

import (
	"log/slog"
	"net"
	"net/url"
	"os"
)

// A development build (-tags dev) can point the add-ons at a fake service, so
// a live run never scrobbles to a real account. The variables are the
// services' addresses (SONA_DEV_ENDPOINT_LISTENBRAINZ, SONA_DEV_ENDPOINT_LASTFM
// for Last.fm's API URL, SONA_DEV_ENDPOINT_LASTFM_AUTH for the page the user
// approves Sona on), and only a loopback
// address is taken: a typo can't send a listen anywhere else. The
// supervisor passes them on to Sona's own add-ons only (addons.Environment).

func devEndpoints(log *slog.Logger) Endpoints {
	return Endpoints{
		ListenBrainz: loopback(log, "SONA_DEV_ENDPOINT_LISTENBRAINZ"),
		LastFM:       loopback(log, "SONA_DEV_ENDPOINT_LASTFM"), LastFMAuth: loopback(log, "SONA_DEV_ENDPOINT_LASTFM_AUTH"),
	}
}

// loopback is the URL in the variable name when it is an http(s) address on
// this machine, else "".
func loopback(log *slog.Logger, name string) string {
	v := os.Getenv(name)
	if v == "" {
		return ""
	}
	u, err := url.Parse(v)
	if err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		if h := u.Hostname(); h == "localhost" || net.ParseIP(h).IsLoopback() {
			return v
		}
	}
	log.Warn("ignoring a development endpoint that is not a loopback URL", "var", name)
	return ""
}
