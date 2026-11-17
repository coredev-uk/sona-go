package own

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/coredev-uk/sona-go/internal/cli"
	"github.com/coredev-uk/sona-go/internal/scrobble"
)

// Options are what the bundled add-ons are built from; Main fills it, tests
// replace it.
type Options struct {
	// Version is Sona's version: the user agent of its requests.
	Version string
	// HTTP is the client of every request; nil is a client with a 30 s
	// timeout over the default transport.
	HTTP *http.Client
	// Endpoints replaces the services' addresses. It is only ever set by a
	// development build (endpoints_dev.go) or a test.
	Endpoints Endpoints
}

// Endpoints are the services' base URLs; "" is the real one.
type Endpoints struct {
	ListenBrainz string
}

func (o Options) client() *http.Client {
	if o.HTTP != nil {
		return o.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (o Options) userAgent() string { return "Sona/" + o.Version }

// service is one scrobbling service as its add-on does the HTTP.
type service interface {
	scrobble.Provider
	// Account asks the service whose credential cred is; an account the
	// service refuses is scrobble.ErrAuth.
	Account(ctx context.Context, cred string) (string, error)
}

type listenBrainz struct{ *scrobble.ListenBrainz }

func newListenBrainz(o Options) service {
	return listenBrainz{&scrobble.ListenBrainz{
		API: o.Endpoints.ListenBrainz, HTTP: o.client(), UserAgent: o.userAgent(), Version: o.Version,
	}}
}

func (l listenBrainz) Account(ctx context.Context, cred string) (string, error) {
	return l.Validate(ctx, cred)
}

// serveScrobbler answers the requests of Sona that a scrobbling add-on gets
// (cli.HostMethods): the listens arrive already filtered and queued by Sona,
// and the add-on does the HTTP and tells how it went in codes the host maps
// back to the scrobble package's errors.
func serveScrobbler(c *Client, svc service) {
	c.Handle(cli.AddonMethodScrobbleNowPlaying, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p cli.ScrobbleNowPlayingParams
		if err := decode(raw, &p); err != nil || p.Credential == "" || p.Track.Artist == "" || p.Track.Title == "" {
			return nil, badArguments(err)
		}
		return ok(svc.NowPlaying(ctx, p.Credential, trackOf(p.Track)))
	})
	c.Handle(cli.AddonMethodScrobbleSubmit, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p cli.ScrobbleSubmitParams
		if err := decode(raw, &p); err != nil || p.Credential == "" || len(p.Listens) == 0 || len(p.Listens) > svc.MaxBatch() {
			return nil, badArguments(err)
		}
		ls := make([]scrobble.Listen, len(p.Listens))
		for i, l := range p.Listens {
			ls[i] = scrobble.Listen{Track: trackOf(l.ScrobbleTrack), ListenedAt: time.Unix(l.ListenedAt, 0)}
		}
		return ok(svc.Submit(ctx, p.Credential, ls))
	})
	c.Handle(cli.AddonMethodScrobbleStatus, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p cli.ScrobbleStatusParams
		if err := decode(raw, &p); err != nil {
			return nil, badArguments(err)
		}
		if p.Credential == "" {
			return cli.ScrobbleResult{OK: true}, nil
		}
		account, err := svc.Account(ctx, p.Credential)
		if err != nil {
			return nil, answerOf(err)
		}
		return cli.ScrobbleResult{OK: true, Account: account}, nil
	})
}

func decode(raw json.RawMessage, v any) error {
	if len(raw) == 0 {
		return errors.New("no params")
	}
	return json.Unmarshal(raw, v)
}

func badArguments(error) error { return &cli.Error{Code: cli.AddonErrBadArguments} }

func trackOf(t cli.ScrobbleTrack) scrobble.Track {
	return scrobble.Track{
		Artist: t.Artist, Title: t.Title, Album: t.Album,
		Duration: time.Duration(t.DurationMs) * time.Millisecond,
	}
}

// ok is the answer of a request that has nothing to return but how it went.
func ok(err error) (any, error) {
	if err != nil {
		return nil, answerOf(err)
	}
	return cli.ScrobbleResult{OK: true}, nil
}

// answerOf is err as the code the host maps back: what a scrobble error
// stands for, else the error itself (code "failed": the host retries it, and
// its text stays in the add-on's log).
func answerOf(err error) error {
	switch {
	case errors.Is(err, scrobble.ErrAuth):
		return &cli.Error{Code: cli.AddonErrCredentialsRejected}
	case errors.Is(err, scrobble.ErrRejected):
		return &cli.Error{Code: cli.AddonErrSubmissionRejected}
	case errors.Is(err, scrobble.ErrPending):
		return &cli.Error{Code: cli.AddonErrAuthorisationPending}
	case errors.Is(err, scrobble.ErrExpired):
		return &cli.Error{Code: cli.AddonErrAuthorisationExpired}
	case errors.Is(err, scrobble.ErrNotConfigured):
		return &cli.Error{Code: cli.AddonErrNotConfigured}
	case isNetwork(err):
		return &cli.Error{Code: cli.AddonErrNetwork}
	}
	return err
}

// isNetwork reports whether err is a failure to reach the service: a refused
// or dropped connection, a DNS failure, a timeout.
func isNetwork(err error) bool {
	var ue *url.Error
	var ne net.Error
	return errors.As(err, &ue) || errors.As(err, &ne)
}
