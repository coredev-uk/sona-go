package services

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/coredev-uk/sona-go/internal/auth/store"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/scrobble"
)

// Last.fm desktop authorisation: how long Sona waits for the user to
// approve it in the browser, and how often it asks.
const (
	lastfmApproval = 10 * time.Minute
	lastfmPoll     = 3 * time.Second
)

// ScrobbleService connects Last.fm and ListenBrainz accounts and scrobbles
// what the coordinator plays (internal/scrobble). The Last.fm session key
// and the ListenBrainz user token live in the keyring (store.OpenSecret);
// settings hold only the account name and the on/off switch. Undelivered
// listens wait in <state>/scrobble-queue.json.
//
// Apple sign-out leaves the scrobbler connections alone: they are separate
// accounts the user connected on this desktop, like Sona's settings, and
// are removed only by disconnecting them (which deletes the secret and the
// provider's queued listens).
type ScrobbleService struct {
	e      *Env
	sc     *scrobble.Scrobbler
	lastfm *scrobble.LastFM // nil: no Last.fm API account in this build
	lb     *scrobble.ListenBrainz
	open   func(key string) (store.Secret, error)
	poll   time.Duration // Last.fm approval polling interval

	loadOnce sync.Once

	secMu   sync.Mutex
	secrets map[string]store.Secret

	mu          sync.Mutex
	connected   map[string]bool
	unencrypted bool
	attempt     uint64
	cancel      context.CancelFunc // a Last.fm approval in progress
}

func newScrobbleService(e *Env) *ScrobbleService {
	httpc := &http.Client{Transport: e.Gate.Transport(http.DefaultTransport), Timeout: 30 * time.Second}
	ua := "Sona/" + e.Version
	var lastfm *scrobble.LastFM
	if key, secret, ok := scrobble.LastFMAccount(); ok {
		lastfm = &scrobble.LastFM{Key: key, Secret: secret, HTTP: httpc, UserAgent: ua}
	}
	s := newScrobble(e, lastfm, &scrobble.ListenBrainz{HTTP: httpc, UserAgent: ua, Version: e.Version},
		func(key string) (store.Secret, error) { return store.OpenSecret(e.Paths.Config, key, e.Logger) })
	e.Coord.ObserveActivity(s.sc.Observe)
	e.OnNetwork(func() {
		e.Go("scrobble start", func() {
			s.load()
			s.sc.Start()
			s.emit()
		})
	})
	e.OnClose(s.close)
	return s
}

// newScrobble builds the service over the given clients (lastfm nil when
// the build has no Last.fm account) and follows the settings' switches.
func newScrobble(e *Env, lastfm *scrobble.LastFM, lb *scrobble.ListenBrainz, open func(string) (store.Secret, error)) *ScrobbleService {
	s := &ScrobbleService{
		e: e, lastfm: lastfm, lb: lb, open: open, poll: lastfmPoll,
		secrets: map[string]store.Secret{}, connected: map[string]bool{},
	}
	providers := []scrobble.Provider{lb}
	if lastfm != nil {
		providers = append(providers, lastfm)
	}
	s.sc = scrobble.New(scrobble.Options{
		Providers: providers,
		QueuePath: filepath.Join(e.Paths.State, "scrobble-queue.json"),
		Logger:    e.Logger.With("component", "scrobble"),
		OnChange:  s.emit,
		Private:   e.Private.Since,
	})
	s.apply(e.Settings.Get().Scrobbling)
	e.Settings.OnChange(func(prev, next config.Settings) {
		if prev.Scrobbling != next.Scrobbling {
			s.apply(next.Scrobbling)
		}
	})
	return s
}

// Status returns the connections' runtime state.
func (s *ScrobbleService) Status() ScrobbleStatus {
	s.load()
	return s.status()
}

// ConnectLastFM runs Last.fm's desktop authorisation: it opens the
// approval page in the browser and resolves once the user has allowed Sona
// (or the attempt fails, times out after ten minutes or is cancelled).
func (s *ScrobbleService) ConnectLastFM(ctx context.Context) (ScrobbleStatus, error) {
	if s.lastfm == nil {
		return ScrobbleStatus{}, scrobble.ErrNotConfigured
	}
	s.load()
	ctx, cancel := context.WithTimeout(ctx, lastfmApproval)
	defer cancel()
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.attempt++
	id := s.attempt
	s.cancel = cancel
	s.mu.Unlock()
	s.emit()

	sess, err := s.approveLastFM(ctx)
	s.mu.Lock()
	if s.attempt == id {
		s.cancel = nil
	}
	s.mu.Unlock()
	if err == nil {
		err = s.connect(ScrobblerLastFM, sess.Key, sess.Name)
	} else {
		s.emit()
	}
	if err != nil {
		return ScrobbleStatus{}, err
	}
	return s.status(), nil
}

// approveLastFM gets a token, opens its approval page and waits for the
// session.
func (s *ScrobbleService) approveLastFM(ctx context.Context) (scrobble.Session, error) {
	token, err := s.lastfm.Token(ctx)
	if err != nil {
		return scrobble.Session{}, err
	}
	if err := s.e.OpenURL(s.lastfm.AuthURL(token)); err != nil {
		return scrobble.Session{}, fmt.Errorf("%w: the Last.fm approval page: %w", ErrOpenBrowser, err)
	}
	return s.lastfm.AwaitSession(ctx, token, s.poll)
}

// CancelConnect abandons a pending Last.fm approval.
func (s *ScrobbleService) CancelConnect() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
	}
}

// ConnectListenBrainz checks a ListenBrainz user token and stores it.
func (s *ScrobbleService) ConnectListenBrainz(ctx context.Context, token string) (ScrobbleStatus, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return ScrobbleStatus{}, ErrTokenRequired
	}
	s.load()
	user, err := s.lb.Validate(ctx, token)
	if err != nil {
		return ScrobbleStatus{}, err
	}
	if err := s.connect(ScrobblerListenBrainz, token, user); err != nil {
		return ScrobbleStatus{}, err
	}
	return s.status(), nil
}

// Disconnect deletes provider's stored credentials and its queued listens
// and clears the account from settings.
func (s *ScrobbleService) Disconnect(provider string) (ScrobbleStatus, error) {
	if provider != ScrobblerLastFM && provider != ScrobblerListenBrainz {
		return ScrobbleStatus{}, fmt.Errorf("%w: unknown scrobbler %q", ErrInvalidArgument, provider)
	}
	s.load()
	sec, err := s.secret(provider)
	if err != nil {
		return ScrobbleStatus{}, err
	}
	if err := sec.Delete(); err != nil {
		return ScrobbleStatus{}, err
	}
	s.sc.Forget(provider)
	s.setConnected(provider, false)
	if err := s.saveAccount(provider, config.ScrobbleAccount{}); err != nil {
		return ScrobbleStatus{}, err
	}
	s.emit()
	return s.status(), nil
}

// connect stores cred for provider, starts using it and records the
// account (enabled) in settings.
func (s *ScrobbleService) connect(provider, cred, user string) error {
	sec, err := s.secret(provider)
	if err != nil {
		return err
	}
	if err := sec.Set(cred); err != nil {
		return err
	}
	s.sc.SetCredential(provider, cred)
	s.setConnected(provider, true)
	err = s.saveAccount(provider, config.ScrobbleAccount{Enabled: true, Username: user})
	s.emit()
	return err
}

// saveAccount writes provider's settings entry and tells the frontend,
// whose settings cache other sections save from.
func (s *ScrobbleService) saveAccount(provider string, a config.ScrobbleAccount) error {
	saved, err := s.e.Settings.Update(func(st *config.Settings) {
		if provider == ScrobblerLastFM {
			st.Scrobbling.LastFM = a
		} else {
			st.Scrobbling.ListenBrainz = a
		}
	})
	if err != nil {
		return err
	}
	if s.e.Emit != nil {
		s.e.Emit.Emit(EventSettingsChanged, saved)
	}
	return nil
}

// apply passes the settings' on/off switches to the scrobbler.
func (s *ScrobbleService) apply(c config.Scrobbling) {
	s.sc.SetEnabled(ScrobblerLastFM, c.LastFM.Enabled)
	s.sc.SetEnabled(ScrobblerListenBrainz, c.ListenBrainz.Enabled)
}

// load reads the stored credentials once (after the first paint, or when
// Settings asks first). Last.fm's is read even when this build cannot use
// it, so the account can still be disconnected.
func (s *ScrobbleService) load() {
	s.loadOnce.Do(func() {
		for _, name := range []string{ScrobblerLastFM, ScrobblerListenBrainz} {
			sec, err := s.secret(name)
			if err != nil {
				s.e.Logger.Warn("scrobble: open credentials", "provider", name, "err", err)
				continue
			}
			v, err := sec.Get()
			switch {
			case err == nil:
				s.sc.SetCredential(name, v)
				s.setConnected(name, true)
			case !errors.Is(err, store.ErrNotFound):
				s.e.Logger.Warn("scrobble: read credentials", "provider", name, "err", err)
			}
		}
	})
}

func (s *ScrobbleService) secret(name string) (store.Secret, error) {
	s.secMu.Lock()
	defer s.secMu.Unlock()
	if sec, ok := s.secrets[name]; ok {
		return sec, nil
	}
	sec, err := s.open(name)
	if err != nil {
		return nil, err
	}
	s.secrets[name] = sec
	if sec.Backend() == store.BackendFile {
		s.mu.Lock()
		s.unencrypted = true
		s.mu.Unlock()
	}
	return sec, nil
}

func (s *ScrobbleService) setConnected(name string, on bool) {
	s.mu.Lock()
	s.connected[name] = on
	s.mu.Unlock()
}

func (s *ScrobbleService) status() ScrobbleStatus {
	lf, lb := s.sc.State(ScrobblerLastFM), s.sc.State(ScrobblerListenBrainz)
	s.mu.Lock()
	defer s.mu.Unlock()
	return ScrobbleStatus{
		LastFM: ScrobblerStatus{
			Available: s.lastfm != nil, Connected: s.connected[ScrobblerLastFM], Connecting: s.cancel != nil,
			NeedsReconnect: lf.Rejected, Pending: lf.Pending,
		},
		ListenBrainz: ScrobblerStatus{
			Available: true, Connected: s.connected[ScrobblerListenBrainz],
			NeedsReconnect: lb.Rejected, Pending: lb.Pending,
		},
		Unencrypted: s.unencrypted,
	}
}

func (s *ScrobbleService) emit() {
	if s.e.Emit != nil {
		s.e.Emit.Emit(EventScrobbleChanged, s.status())
	}
}

func (s *ScrobbleService) close() {
	s.CancelConnect()
	s.sc.Close()
}
