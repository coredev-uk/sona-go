package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/coredev-uk/sona-go/internal/cli"
	"github.com/coredev-uk/sona-go/internal/scrobble"
)

// addonLink is what the scrobbling proxies need of the add-on supervisor:
// the AddonsService.
type addonLink interface {
	Request(ctx context.Context, id, method string, params, out any) error
	Lease(id string) (release func())
}

// addonScrobbler is a scrobble.Provider whose HTTP runs in one of Sona's own
// add-ons (#302). The tracker, the one private filter, the persistent queue
// and the backoff stay in Sona: only a listen that passed the filter reaches
// the queue, and so only such a listen is ever sent to the add-on, which
// does the request and answers in the codes (cli.AddonErrCredentialsRejected
// and the rest) that map back to the scrobble package's errors, so the queue
// acts as it did when the HTTP ran in Sona.
type addonScrobbler struct {
	id  string // the add-on's ID, and the provider's name
	max int
	ask func(ctx context.Context, method string, params, out any) error
}

func (p *addonScrobbler) Name() string  { return p.id }
func (p *addonScrobbler) MaxBatch() int { return p.max }

// NowPlaying announces t through the add-on.
func (p *addonScrobbler) NowPlaying(ctx context.Context, cred string, t scrobble.Track) error {
	return p.result(p.ask(ctx, cli.AddonMethodScrobbleNowPlaying,
		cli.ScrobbleNowPlayingParams{Credential: cred, Track: trackParams(t)}, nil))
}

// Submit delivers the listens through the add-on.
func (p *addonScrobbler) Submit(ctx context.Context, cred string, ls []scrobble.Listen) error {
	if len(ls) == 0 {
		return nil
	}
	params := cli.ScrobbleSubmitParams{Credential: cred, Listens: make([]cli.ScrobbleListen, len(ls))}
	for i, l := range ls {
		params.Listens[i] = cli.ScrobbleListen{ScrobbleTrack: trackParams(l.Track), ListenedAt: l.ListenedAt.Unix()}
	}
	return p.result(p.ask(ctx, cli.AddonMethodScrobbleSubmit, params, nil))
}

// Account asks the service, through the add-on, whose credential cred is. A
// credential it refuses is scrobble.ErrAuth.
func (p *addonScrobbler) Account(ctx context.Context, cred string) (string, error) {
	var res cli.ScrobbleResult
	if err := p.result(p.ask(ctx, cli.AddonMethodScrobbleStatus, cli.ScrobbleStatusParams{Credential: cred}, &res)); err != nil {
		return "", err
	}
	return res.Account, nil
}

func trackParams(t scrobble.Track) cli.ScrobbleTrack {
	return cli.ScrobbleTrack{Artist: t.Artist, Title: t.Title, Album: t.Album, DurationMs: t.Duration.Milliseconds()}
}

// result maps what the add-on answered back to the scrobble package's errors.
// An answer with no such meaning (the service is down, the add-on is not up,
// the request timed out) is an error to retry, as a network failure was.
func (p *addonScrobbler) result(err error) error {
	if err == nil {
		return nil
	}
	var ce *cli.Error
	if !errors.As(err, &ce) {
		return err // ErrAddonUnavailable, a cancelled context, a bad answer
	}
	switch ce.Code {
	case cli.AddonErrCredentialsRejected:
		return fmt.Errorf("%w: %s", scrobble.ErrAuth, p.id)
	case cli.AddonErrSubmissionRejected:
		return fmt.Errorf("%w: %s", scrobble.ErrRejected, p.id)
	case cli.AddonErrAuthorisationPending:
		return fmt.Errorf("%w: %s", scrobble.ErrPending, p.id)
	case cli.AddonErrAuthorisationExpired:
		return fmt.Errorf("%w: %s", scrobble.ErrExpired, p.id)
	case cli.AddonErrNotConfigured:
		return fmt.Errorf("%w: %s", scrobble.ErrNotConfigured, p.id)
	case cli.AddonErrNetwork:
		return fmt.Errorf("add-on %s: %w", p.id, addonNetworkError{})
	case cli.AddonErrNotRunning, cli.AddonErrTimeout:
		return fmt.Errorf("add-on %s: %w", p.id, ErrAddonUnavailable)
	}
	return fmt.Errorf("add-on %s: %s", p.id, ce.Code)
}

// scrobbleProxy returns the provider that does name's HTTP in its add-on,
// asking through the service's link (nil until attached: ErrAddonUnavailable).
func (s *ScrobbleService) scrobbleProxy(name string, batch int) *addonScrobbler {
	return &addonScrobbler{id: name, max: batch, ask: func(ctx context.Context, method string, params, out any) error {
		link := s.link()
		if link == nil {
			return ErrAddonUnavailable
		}
		return link.Request(ctx, name, method, params, out)
	}}
}

// attach gives the service the supervisor its add-ons run under, once every
// service exists.
func (s *ScrobbleService) attach(l addonLink) {
	s.mu.Lock()
	s.addons = l
	s.mu.Unlock()
}

func (s *ScrobbleService) link() addonLink {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.addons
}

// lease holds provider's add-on up while a connect flow runs, so it can ask
// the add-on before an account exists for the add-on to serve.
func (s *ScrobbleService) lease(provider string) (release func()) {
	if l := s.link(); l != nil {
		return l.Lease(provider)
	}
	return func() {}
}

// addonNetworkError is a service an add-on couldn't reach. It is a net.Error,
// which is what CodeOf takes for a network failure, so the connect flows
// word it as before ("Couldn't reach ListenBrainz").
type addonNetworkError struct{}

func (addonNetworkError) Error() string   { return "the service could not be reached" }
func (addonNetworkError) Timeout() bool   { return false }
func (addonNetworkError) Temporary() bool { return true }
