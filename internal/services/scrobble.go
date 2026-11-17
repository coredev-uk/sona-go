package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
// and the ListenBrainz user token live in the keyring item of the add-on
// that sends them (`addon.lastfm`, `addon.listenbrainz`; addon_secrets.go),
// where a version before add-ons' own items (`lastfm`, `listenbrainz`) are
// moved once at start; settings hold only the account name and the on/off
// switch. Undelivered listens wait in <state>/scrobble-queue.json.
//
// Apple sign-out leaves the scrobbler connections alone: they are separate
// accounts the user connected on this desktop, like Sona's settings, and
// are removed only by disconnecting them (which deletes the secret and the
// provider's queued listens).
type ScrobbleService struct {
	e      *Env
	sc     *scrobble.Scrobbler
	lastfm *scrobble.LastFM // nil: no Last.fm API account in this build
	lb     *addonScrobbler  // ListenBrainz, in its add-on
	poll   time.Duration    // Last.fm approval polling interval

	loadOnce sync.Once

	secMu   sync.Mutex
	secrets map[string]store.Secret

	mu          sync.Mutex
	connected   map[string]bool
	unencrypted bool
	attempt     uint64
	cancel      context.CancelFunc // a Last.fm approval in progress
	addons      addonLink          // the supervisor of the add-ons (attach)
}

func newScrobbleService(e *Env) *ScrobbleService {
	httpc := &http.Client{Transport: e.gated(), Timeout: 30 * time.Second}
	var lastfm *scrobble.LastFM
	if key, secret, ok := scrobble.LastFMAccount(); ok {
		lastfm = &scrobble.LastFM{Key: key, Secret: secret, HTTP: httpc, UserAgent: "Sona/" + e.Version}
	}
	s := newScrobble(e, lastfm)
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

// newScrobble builds the service over lastfm (nil when the build has no
// Last.fm account) and the proxy of ListenBrainz's add-on, and follows the
// settings' switches.
func newScrobble(e *Env, lastfm *scrobble.LastFM) *ScrobbleService {
	s := &ScrobbleService{
		e: e, lastfm: lastfm, poll: lastfmPoll,
		secrets: map[string]store.Secret{}, connected: map[string]bool{},
	}
	s.lb = s.scrobbleProxy(ScrobblerListenBrainz, scrobble.ListenBrainzBatch)
	providers := []scrobble.Provider{s.lb}
	if lastfm != nil {
		providers = append(providers, lastfm)
	}
	s.sc = scrobble.New(scrobble.Options{
		Providers: providers,
		QueuePath: filepath.Join(e.Paths.State, "scrobble-queue.json"),
		Logger:    e.Logger.With("component", "scrobble"),
		OnChange:  s.emit,
		Hidden:    e.Private.Hidden,
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
	// The add-on that checks the token runs only while an account is
	// connected: hold it up for the check, and until the account is saved.
	defer s.lease(ScrobblerListenBrainz)()
	user, err := s.lb.Account(ctx, token)
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
	// A sign-in that could not be moved still sits in the old item: it goes
	// too, or it would come back at the next start.
	if old, err := s.e.openSecret(provider); err == nil {
		if err := old.Delete(); err != nil {
			s.e.Logger.Warn("scrobble: delete the old credentials", "provider", provider, "err", err)
		}
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

// saveAccount writes provider's settings entry and tells the UI,
// whose settings cache other sections save from.
func (s *ScrobbleService) saveAccount(provider string, a config.ScrobbleAccount) error {
	_, err := s.e.updateSettings(func(st *config.Settings) {
		if provider == ScrobblerLastFM {
			st.Scrobbling.LastFM = a
		} else {
			st.Scrobbling.ListenBrainz = a
		}
	})
	return err
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
			// The sign-in a version before add-ons kept in the item `name`
			// moves to the add-on's own, once. A move that failed leaves the
			// old item to be read below and tried again at the next start.
			moveErr := s.e.migrateSecret(s.log(), name, name, scrobbleKeys[name])
			if moveErr != nil {
				s.e.Logger.Warn("scrobble: move the sign-in to the add-on's secrets", "provider", name, "err", moveErr)
			}
			sec, err := s.secret(name)
			if err != nil {
				s.e.Logger.Warn("scrobble: open credentials", "provider", name, "err", err)
				continue
			}
			v, err := sec.Get()
			if errors.Is(err, store.ErrNotFound) && moveErr != nil {
				v, err = s.legacy(name)
			}
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

func (s *ScrobbleService) log() *slog.Logger { return s.e.Logger.With("component", "scrobble") }

// legacy reads the item a version before add-ons kept the sign-in in.
func (s *ScrobbleService) legacy(name string) (string, error) {
	old, err := s.e.openSecret(name)
	if err != nil {
		return "", err
	}
	return old.Get()
}

// secret is the sign-in of provider in its add-on's keyring item
// (`addon.<id>`, key scrobbleKeys[provider]; addon_secrets.go).
func (s *ScrobbleService) secret(name string) (store.Secret, error) {
	s.secMu.Lock()
	defer s.secMu.Unlock()
	if sec, ok := s.secrets[name]; ok {
		return sec, nil
	}
	sec := s.e.addonSecrets().slot(name, scrobbleKeys[name])
	s.secrets[name] = sec
	if sec.Backend() == store.BackendFile {
		s.mu.Lock()
		s.unencrypted = true
		s.mu.Unlock()
	}
	return sec, nil
}

// scrobbleKeys are the keys of the sign-ins in the add-ons' secrets.
var scrobbleKeys = map[string]string{ScrobblerLastFM: lastfmSecretKey, ScrobblerListenBrainz: listenbrainzSecretKey}

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
