// Package auth owns Sona's Apple Music sign-in state: the stored Music User
// Token, the account metadata cached next to it, sign-in through Sona's
// own Apple Account form (package appleid), the loopback browser flow
// (package mut) or a pasted token, sign-out, and 401/403 triage. See
// docs/auth.md §4–§7.
package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/auth/appleid"
	"github.com/coredev-uk/sona-go/internal/auth/mut"
	"github.com/coredev-uk/sona-go/internal/auth/store"
)

// State is the sign-in state reported to the UI.
type State string

const (
	StateLoading     State = "loading"      // before Start has read the store
	StateSignedOut   State = "signed_out"   // no Music User Token
	StateSigningIn   State = "signing_in"   // sign-in in progress: loopback redirect or native two-factor code
	StateValidating  State = "validating"   // checking a token against /v1/me/storefront
	StateSignedIn    State = "signed_in"    // token in use (possibly not yet revalidated, see Status.Offline)
	StateNeedsReauth State = "needs_reauth" // Apple rejected the stored token; sign in again
	StateError       State = "error"        // unusable for another reason, e.g. developer token rejected
)

// Errors reported by validation and sign-in.
var (
	ErrMUTRejected      = errors.New("auth: Apple rejected the Music User Token")
	ErrDevTokenRejected = errors.New("auth: developer token rejected")
	ErrNoAccount        = errors.New("auth: no account metadata")
	ErrNotSignedIn      = errors.New("auth: not signed in")
)

// Account is the non-secret metadata cached for the signed-in account.
type Account struct {
	StorefrontID       string // e.g. "gb"
	StorefrontName     string // e.g. "United Kingdom"
	Language           string // storefront default language tag, e.g. "en-GB"
	Restricted         bool   // explicit content restricted (itre)
	MUTObtainedAt      time.Time
	MUTLastValidatedAt time.Time
}

// AccountStore persists Account (SQLite in the app). Load returns
// ErrNoAccount when nothing is stored; Delete of a missing account is not an
// error.
type AccountStore interface {
	Load(ctx context.Context) (Account, error)
	Save(ctx context.Context, a Account) error
	Delete(ctx context.Context) error
}

// Status is a snapshot of the sign-in state; it is the auth:changed payload.
type Status struct {
	State          State         `json:"state"`
	StorefrontID   string        `json:"storefrontId,omitempty"`
	StorefrontName string        `json:"storefrontName,omitempty"`
	Language       string        `json:"language,omitempty"`
	Restricted     bool          `json:"restricted"`
	Backend        store.Backend `json:"backend"`
	// Offline is set while the last validation of the token in use could not
	// reach Apple (network or server error); the token is kept and
	// validation retries with backoff.
	Offline   bool   `json:"offline"`
	LastError string `json:"lastError,omitempty"`
	// SignInURL is the loopback start page while State is signing_in, so the
	// UI can offer it when opening a browser failed.
	SignInURL string `json:"signInUrl,omitempty"`
	// SecondFactor is the pending two-factor challenge of a native
	// sign-in (SignInWithPassword) while State is signing_in.
	SecondFactor *appleid.SecondFactor `json:"secondFactor,omitempty"`
}

// Config configures a Manager. Dev, Store and Accounts are required.
type Config struct {
	HTTP     *http.Client           // default http.DefaultClient
	Dev      applemusic.TokenSource // the developer token provider, normally *devtoken.WebPlayer
	Store    store.Store
	Accounts AccountStore
	Logger   *slog.Logger
	APIBase  string // default applemusic.BaseURL

	AppName       string          // shown on Apple's consent screen; default "Sona"
	IconURL       string          // optional consent screen icon
	SignInTimeout time.Duration   // sign-in attempt deadline (loopback or native); default 10 minutes
	AppleID       appleid.Options // native sign-in endpoints and client; HTTP defaults to HTTP

	RetryMin       time.Duration // first validation retry after a network error; default 2s
	RetryMax       time.Duration // backoff cap; default 5m
	TriageCooldown time.Duration // minimum gap between ReportUnauthorized probes; default 1m
}

type hook struct {
	name string
	fn   func(context.Context) error
}

type subscriber struct {
	id int
	fn func(Status)
}

// Manager owns the sign-in state machine. All methods are safe for
// concurrent use.
type Manager struct {
	cfg  Config
	log  *slog.Logger
	ctx  context.Context // lifetime; canceled by Close
	stop context.CancelFunc

	// notify serializes state transitions with their callbacks so
	// subscribers see every transition in order. Lock order:
	// persist, then notify, then mu.
	notify sync.Mutex
	// persist serializes writes to Store and Accounts with the checks that
	// the writing flow is still current, so a racing SignOut always wins.
	persist sync.Mutex

	mu         sync.Mutex
	st         Status // State, Offline, LastError, SignInURL, SecondFactor
	started    bool
	record     store.Record // stored token, kept in needs_reauth for Revalidate
	token      string       // token handed to requests; "" unless usable
	account    Account
	hasAccount bool
	validated  bool // record validated since it was loaded or obtained

	attempt       uint64 // sign-in attempt generation
	cancelAttempt context.CancelFunc
	prior         State          // state to restore when the attempt fails
	native        *nativeAttempt // pending native two-factor step, if any

	bgCancel   context.CancelFunc // background validation loop
	triaging   bool
	lastTriage time.Time

	hooks  []hook
	subs   []subscriber
	nextID int
}

// NewManager returns a Manager in StateLoading; call Start to load the
// stored token.
//
// Wire the app-wide Apple Music client to it with
// applemusic.New(httpClient, dev, m.UserToken). The manager issues its own
// validation requests through applemusic clients built from cfg.
func NewManager(cfg Config) (*Manager, error) {
	if cfg.Dev == nil || cfg.Store == nil || cfg.Accounts == nil {
		return nil, errors.New("auth: Dev, Store and Accounts are required")
	}
	if cfg.HTTP == nil {
		cfg.HTTP = http.DefaultClient
	}
	if cfg.APIBase == "" {
		cfg.APIBase = applemusic.BaseURL
	}
	if cfg.RetryMin <= 0 {
		cfg.RetryMin = 2 * time.Second
	}
	if cfg.RetryMax <= 0 {
		cfg.RetryMax = 5 * time.Minute
	}
	if cfg.RetryMax < cfg.RetryMin {
		cfg.RetryMax = cfg.RetryMin
	}
	if cfg.TriageCooldown <= 0 {
		cfg.TriageCooldown = time.Minute
	}
	log := cfg.Logger
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	ctx, stop := context.WithCancel(context.Background())
	return &Manager{cfg: cfg, log: log, ctx: ctx, stop: stop, st: Status{State: StateLoading}}, nil
}

// Close cancels a pending sign-in and background validation.
func (m *Manager) Close() { m.stop() }

// Status returns the current state.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.statusLocked()
}

func (m *Manager) statusLocked() Status {
	s := m.st
	s.Backend = m.cfg.Store.Backend()
	if s.State == StateSignedOut || s.State == StateLoading {
		return s
	}
	s.Restricted = m.record.Restricted
	if m.hasAccount {
		s.StorefrontID = m.account.StorefrontID
		s.StorefrontName = m.account.StorefrontName
		s.Language = m.account.Language
	}
	return s
}

// UserToken returns the Music User Token for requests, or "" when signed
// out or when Apple rejected the token (needs_reauth), so catalog requests
// keep working with the developer token alone. Pass it to applemusic.New.
func (m *Manager) UserToken() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.token
}

// OnChange registers fn to receive every Status transition, in order. fn
// runs synchronously on the goroutine making the transition and must not
// call Manager methods other than Status and UserToken. The returned func
// unregisters fn.
func (m *Manager) OnChange(fn func(Status)) (remove func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	id := m.nextID
	m.subs = append(m.subs, subscriber{id, fn})
	return func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		for i, s := range m.subs {
			if s.id == id {
				m.subs = append(m.subs[:i:i], m.subs[i+1:]...)
				return
			}
		}
	}
}

// OnSignOut registers a wipe hook (engine sign-out, user data, artwork…).
// SignOut runs hooks in registration order; name labels its errors.
func (m *Manager) OnSignOut(name string, fn func(ctx context.Context) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hooks = append(m.hooks, hook{name, fn})
}

// update applies f under mu and, when f reports a change, delivers the new
// Status to subscribers. It reports whether f made a change.
func (m *Manager) update(f func() bool) bool {
	m.notify.Lock()
	defer m.notify.Unlock()
	m.mu.Lock()
	changed := f()
	s := m.statusLocked()
	subs := m.subs
	m.mu.Unlock()
	if changed {
		for _, sub := range subs {
			sub.fn(s)
		}
	}
	return changed
}

// Start loads the stored token. It reads only local storage: a stored token
// with cached account metadata reports signed_in at once and is validated in
// the background (offline-first); a stored token without metadata reports
// validating until Apple answers. Start never waits for the network. Later
// calls are no-ops. Background work runs until Close; ctx only bounds
// Start's own reads.
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return nil
	}
	m.started = true
	m.mu.Unlock()

	rec, err := m.cfg.Store.Get()
	switch {
	case errors.Is(err, store.ErrNotFound):
		m.update(func() bool { m.st = Status{State: StateSignedOut}; return true })
		return nil
	case errors.Is(err, store.ErrCorrupt):
		m.log.Warn("discarding unreadable stored credentials", "backend", m.cfg.Store.Backend(), "err", err)
		msg := "stored credentials were unreadable; sign in again"
		if derr := m.cfg.Store.Delete(); derr != nil {
			msg += " (" + derr.Error() + ")"
		}
		m.update(func() bool { m.st = Status{State: StateSignedOut, LastError: msg}; return true })
		return nil
	case err != nil:
		m.update(func() bool { m.st = Status{State: StateError, LastError: err.Error()}; return true })
		return fmt.Errorf("auth: load credentials: %w", err)
	}

	acct, aerr := m.cfg.Accounts.Load(ctx)
	if aerr != nil && !errors.Is(aerr, ErrNoAccount) {
		m.log.Warn("account metadata unavailable; revalidating", "err", aerr)
	}
	m.update(func() bool {
		m.record, m.token, m.validated = rec, rec.Token, false
		m.account, m.hasAccount = acct, aerr == nil
		m.st = Status{State: StateValidating}
		if m.hasAccount {
			m.st.State = StateSignedIn
		}
		return true
	})
	m.log.Info("loaded stored credentials", "mut", rec, "backend", m.cfg.Store.Backend(), "cached_account", aerr == nil)
	m.startValidation(rec)
	return nil
}

// startValidation (re)starts the background loop validating rec.
func (m *Manager) startValidation(rec store.Record) {
	ctx, cancel := context.WithCancel(m.ctx)
	m.mu.Lock()
	if m.bgCancel != nil {
		m.bgCancel()
	}
	m.bgCancel = cancel
	m.mu.Unlock()
	go m.validateLoop(ctx, rec)
}

func (m *Manager) stopValidationLocked() {
	if m.bgCancel != nil {
		m.bgCancel()
		m.bgCancel = nil
	}
}

// validateLoop validates rec, retrying network and server errors with
// exponential backoff until Apple gives a verdict or ctx ends.
func (m *Manager) validateLoop(ctx context.Context, rec store.Record) {
	delay := m.cfg.RetryMin
	for {
		sf, err := m.check(ctx, rec.Token)
		if ctx.Err() != nil {
			return
		}
		if err = m.settle(rec, sf, err); err == nil {
			return
		}
		m.log.Warn("token validation failed; will retry", "in", delay, "err", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		delay = min(delay*2, m.cfg.RetryMax)
	}
}

// client returns an Apple Music client sending tok as the Music User Token
// ("" sends none).
func (m *Manager) client(tok string) *applemusic.Client {
	c := applemusic.New(m.cfg.HTTP, m.cfg.Dev, func() string { return tok })
	c.Base = m.cfg.APIBase
	return c
}

// check validates tok with /v1/me/storefront. A 401/403 that survives the
// client's developer token refresh is split by a catalog request without the
// token: if the catalog works the token is bad (ErrMUTRejected), otherwise
// the developer token is (ErrDevTokenRejected). Other errors are transient.
func (m *Manager) check(ctx context.Context, tok string) (applemusic.Resource[applemusic.StorefrontAttributes], error) {
	sf, err := m.client(tok).Storefront(ctx)
	if err == nil || !errors.Is(err, applemusic.ErrUnauthorized) {
		return sf, err
	}
	m.mu.Lock()
	probe := m.account.StorefrontID
	m.mu.Unlock()
	if probe == "" {
		probe = "us"
	}
	perr := m.client("").Get(ctx, "/v1/storefronts/"+probe, nil, nil)
	switch {
	case perr == nil:
		return sf, fmt.Errorf("%w: %v", ErrMUTRejected, err)
	case errors.Is(perr, applemusic.ErrUnauthorized):
		return sf, fmt.Errorf("%w: %v", ErrDevTokenRejected, perr)
	default:
		return sf, fmt.Errorf("storefront %v; catalog probe: %w", err, perr)
	}
}

// settle applies a validation outcome for rec, the stored record in use.
// Outcomes for a record that is no longer current are dropped. It returns
// err when the outcome was transient.
func (m *Manager) settle(rec store.Record, sf applemusic.Resource[applemusic.StorefrontAttributes], err error) error {
	current := func() bool {
		return m.record.Token == rec.Token && m.cancelAttempt == nil &&
			m.st.State != StateSignedOut && m.st.State != StateLoading
	}
	switch {
	case err == nil:
		m.persist.Lock()
		defer m.persist.Unlock()
		m.mu.Lock()
		ok := current()
		m.mu.Unlock()
		if !ok {
			return nil
		}
		acct := accountFrom(sf, rec, time.Now())
		if serr := m.cfg.Accounts.Save(m.ctx, acct); serr != nil {
			m.log.Warn("saving account metadata failed", "err", serr)
		}
		m.update(func() bool {
			if !current() {
				return false
			}
			m.token, m.validated = rec.Token, true
			m.account, m.hasAccount = acct, true
			m.st = Status{State: StateSignedIn}
			return true
		})
		m.log.Info("token validated", "storefront", acct.StorefrontID)
		return nil
	case errors.Is(err, ErrMUTRejected):
		m.update(func() bool {
			if !current() || m.st.State == StateNeedsReauth {
				return false
			}
			m.token = ""
			m.st = Status{State: StateNeedsReauth, LastError: "Apple no longer accepts this sign-in; sign in again"}
			return true
		})
		m.log.Warn("music user token rejected", "err", err)
		return nil
	case errors.Is(err, ErrDevTokenRejected):
		m.update(func() bool {
			if !current() || m.st.State == StateError {
				return false
			}
			m.st = Status{State: StateError, LastError: "developer token rejected"}
			return true
		})
		m.log.Error("developer token rejected", "err", err)
		return nil
	default:
		m.update(func() bool {
			if !current() || m.st.Offline {
				return false
			}
			m.st.Offline = true
			return true
		})
		return err
	}
}

func accountFrom(sf applemusic.Resource[applemusic.StorefrontAttributes], rec store.Record, now time.Time) Account {
	return Account{
		StorefrontID:       sf.ID,
		StorefrontName:     sf.Attributes.Name,
		Language:           sf.Attributes.DefaultLanguageTag,
		Restricted:         rec.Restricted,
		MUTObtainedAt:      rec.ObtainedAt,
		MUTLastValidatedAt: now,
	}
}

// Revalidate checks the stored token now (the UI's "retry" in the error,
// needs_reauth or offline case) and applies the verdict. It returns nil when
// the token validated, an error wrapping ErrMUTRejected or
// ErrDevTokenRejected for Apple's verdict, or the transient error without
// changing state.
func (m *Manager) Revalidate(ctx context.Context) error {
	m.mu.Lock()
	rec, idle := m.record, m.cancelAttempt == nil
	m.mu.Unlock()
	if rec.Token == "" || !idle {
		return ErrNotSignedIn
	}
	sf, err := m.check(ctx, rec.Token)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if serr := m.settle(rec, sf, err); serr != nil {
		return serr
	}
	return err
}

// ReportUnauthorized lets request paths report a 401/403 that survived the
// client's developer-token refresh. Reports other than
// applemusic.ErrUnauthorized, reports outside signed_in, reports while a
// probe runs, and reports within TriageCooldown of the last probe are
// ignored. Otherwise a background probe decides: the token still validates
// (stay), Apple rejects the token (needs_reauth, once; later reports are
// ignored until a new sign-in), or Apple rejects the developer token
// (error "developer token rejected"). Network errors change nothing.
func (m *Manager) ReportUnauthorized(err error) {
	if !errors.Is(err, applemusic.ErrUnauthorized) {
		return
	}
	m.mu.Lock()
	if m.st.State != StateSignedIn || m.token == "" || m.triaging || time.Since(m.lastTriage) < m.cfg.TriageCooldown {
		m.mu.Unlock()
		return
	}
	m.triaging = true
	rec := m.record
	m.mu.Unlock()
	m.log.Info("triaging unauthorized response", "err", err)
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, time.Minute)
		defer cancel()
		defer func() {
			m.mu.Lock()
			m.triaging, m.lastTriage = false, time.Now()
			m.mu.Unlock()
		}()
		sf, cerr := m.check(ctx, rec.Token)
		if ctx.Err() != nil {
			return
		}
		if cerr = m.settle(rec, sf, cerr); cerr != nil {
			m.log.Warn("unauthorized triage inconclusive", "err", cerr)
		}
	}()
}

// beginAttempt supersedes any pending attempt, stops background validation,
// and enters state. The returned context ends on CancelSignIn, SignOut, a
// newer attempt, Close, or ctx.
func (m *Manager) beginAttempt(ctx context.Context, state State) (context.Context, uint64, context.CancelFunc) {
	actx, cancel := context.WithCancel(ctx)
	stopAfter := context.AfterFunc(m.ctx, cancel)
	var id uint64
	m.update(func() bool {
		if m.cancelAttempt != nil {
			m.cancelAttempt()
		} else {
			m.prior = m.st.State
		}
		m.attempt++
		id = m.attempt
		m.cancelAttempt = cancel
		m.stopValidationLocked()
		m.st = Status{State: state}
		return true
	})
	return actx, id, func() { stopAfter(); cancel() }
}

// restoreLocked ends the pending attempt and returns to the prior state; it
// returns the record to revalidate, if any.
func (m *Manager) restoreLocked(lastErr string) (store.Record, bool) {
	m.attempt++
	m.cancelAttempt = nil
	prior := m.prior
	if prior == StateLoading || prior == StateSigningIn || prior == "" {
		prior = StateSignedOut
	}
	m.st = Status{State: prior, LastError: lastErr}
	needs := m.record.Token != "" && !m.validated && (prior == StateSignedIn || prior == StateValidating)
	return m.record, needs
}

// failAttempt restores the prior state for attempt id and returns err.
func (m *Manager) failAttempt(id uint64, err error) error {
	if errors.Is(err, context.Canceled) {
		err = mut.ErrCanceled
	}
	msg := err.Error()
	switch {
	case errors.Is(err, mut.ErrCanceled):
		msg = ""
	case errors.Is(err, mut.ErrTimeout):
		msg = "sign-in timed out"
	case errors.Is(err, ErrMUTRejected):
		msg = "Apple rejected the sign-in"
	}
	var (
		rec  store.Record
		revl bool
	)
	if m.update(func() bool {
		if m.attempt != id {
			return false
		}
		rec, revl = m.restoreLocked(msg)
		return true
	}) && revl {
		m.startValidation(rec)
	}
	if msg != "" {
		m.log.Warn("sign-in failed", "err", err)
	}
	return err
}

// SignIn runs the loopback sign-in: it starts the single-use loopback
// server, enters signing_in with Status.SignInURL set, calls open with that
// URL (a failure is recorded in LastError and the attempt keeps waiting, so
// the UI can show the link), waits for Apple's redirect, validates the token
// and stores it. It blocks until the attempt finishes and returns
// mut.ErrCanceled after CancelSignIn, SignOut or a newer attempt, and
// mut.ErrTimeout at the deadline. On failure the previous state returns.
func (m *Manager) SignIn(ctx context.Context, open func(url string) error) error {
	ctx, id, done := m.beginAttempt(ctx, StateSigningIn)
	defer done()
	tok, err := m.cfg.Dev.Token(ctx)
	if err != nil {
		return m.failAttempt(id, fmt.Errorf("auth: developer token: %w", err))
	}
	sess, err := mut.Start(tok.JWT, mut.Options{AppName: m.cfg.AppName, IconURL: m.cfg.IconURL, Timeout: m.cfg.SignInTimeout})
	if err != nil {
		return m.failAttempt(id, err)
	}
	defer sess.Close()
	url := sess.StartURL()
	m.update(func() bool {
		if m.attempt != id {
			return false
		}
		m.st.SignInURL = url
		return true
	})
	if open != nil {
		if err := open(url); err != nil {
			m.log.Warn("opening sign-in page failed", "err", err)
			m.update(func() bool {
				if m.attempt != id {
					return false
				}
				m.st.LastError = "could not open a browser; open the sign-in link manually"
				return true
			})
		}
	}
	res, err := sess.Wait(ctx)
	if err != nil {
		return m.failAttempt(id, err)
	}
	m.log.Info("sign-in callback received", "mut", res)
	return m.finish(ctx, id, store.Record{Token: res.MusicUserToken, ObtainedAt: time.Now(), Restricted: res.Restricted})
}

// SignInWithToken validates a pasted Music User Token (the media-user-token
// cookie from music.apple.com) and stores it. It returns an error wrapping
// ErrMUTRejected if Apple refuses it; on failure the previous state returns.
func (m *Manager) SignInWithToken(ctx context.Context, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return errors.New("auth: empty token")
	}
	ctx, id, done := m.beginAttempt(ctx, StateValidating)
	defer done()
	return m.finish(ctx, id, store.Record{Token: token, ObtainedAt: time.Now()})
}

// finish validates rec for attempt id, persists it and enters signed_in.
func (m *Manager) finish(ctx context.Context, id uint64, rec store.Record) error {
	m.update(func() bool {
		if m.attempt != id {
			return false
		}
		m.st = Status{State: StateValidating}
		return true
	})
	sf, err := m.check(ctx, rec.Token)
	if ctx.Err() != nil {
		return m.failAttempt(id, mut.ErrCanceled)
	}
	if err != nil {
		return m.failAttempt(id, err)
	}

	m.persist.Lock()
	defer m.persist.Unlock()
	m.mu.Lock()
	current := m.attempt == id
	m.mu.Unlock()
	if !current {
		return mut.ErrCanceled
	}
	if err := m.cfg.Store.Set(rec); err != nil {
		return m.failAttempt(id, fmt.Errorf("auth: store token: %w", err))
	}
	acct := accountFrom(sf, rec, time.Now())
	if err := m.cfg.Accounts.Save(ctx, acct); err != nil {
		m.log.Warn("saving account metadata failed", "err", err)
	}
	if !m.update(func() bool {
		if m.attempt != id {
			return false
		}
		m.attempt++
		m.cancelAttempt = nil
		m.record, m.token, m.validated = rec, rec.Token, true
		m.account, m.hasAccount = acct, true
		m.st = Status{State: StateSignedIn}
		return true
	}) {
		return mut.ErrCanceled // SignOut raced in and will delete what was stored
	}
	m.log.Info("signed in", "mut", rec, "storefront", acct.StorefrontID, "backend", m.cfg.Store.Backend())
	return nil
}

// CancelSignIn abandons a pending sign-in (closing its loopback server) and
// returns to the state before it. Without one it does nothing.
func (m *Manager) CancelSignIn() {
	var (
		rec  store.Record
		revl bool
	)
	if m.update(func() bool {
		if m.cancelAttempt == nil {
			return false
		}
		m.cancelAttempt()
		rec, revl = m.restoreLocked("")
		return true
	}) && revl {
		m.startValidation(rec)
	}
}

// SignOut cancels any sign-in and background work, clears the in-memory
// token, deletes the stored token and the account metadata, then runs the
// OnSignOut hooks in order. Every step runs even if an earlier one fails;
// the failures are returned joined and shown in Status.LastError. It ends
// in signed_out.
func (m *Manager) SignOut(ctx context.Context) error {
	var hooks []hook
	m.update(func() bool {
		m.attempt++
		if m.cancelAttempt != nil {
			m.cancelAttempt()
			m.cancelAttempt = nil
		}
		m.stopValidationLocked()
		m.record, m.token, m.validated = store.Record{}, "", false
		m.account, m.hasAccount = Account{}, false
		m.st = Status{State: StateSignedOut}
		hooks = m.hooks
		return false // published once below, after the wipe
	})
	var errs []error
	m.persist.Lock()
	if err := m.cfg.Store.Delete(); err != nil {
		errs = append(errs, fmt.Errorf("delete stored token: %w", err))
	}
	if err := m.cfg.Accounts.Delete(ctx); err != nil {
		errs = append(errs, fmt.Errorf("delete account: %w", err))
	}
	m.persist.Unlock()
	for _, h := range hooks {
		if err := h.fn(ctx); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", h.name, err))
		}
	}
	err := errors.Join(errs...)
	m.update(func() bool {
		if m.st.State == StateSignedOut && err != nil {
			m.st.LastError = "sign-out incomplete: " + err.Error()
		}
		return true
	})
	if err != nil {
		m.log.Error("sign-out incomplete", "err", err)
		return fmt.Errorf("auth: sign out: %w", err)
	}
	m.log.Info("signed out")
	return nil
}
