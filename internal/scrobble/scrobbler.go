package scrobble

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/coredev-uk/sona-go/internal/diag"
	"github.com/coredev-uk/sona-go/internal/playback/coordinator"
)

// Provider is one scrobbling service. cred is its session key or token.
type Provider interface {
	Name() string
	MaxBatch() int
	NowPlaying(ctx context.Context, cred string, t Track) error
	Submit(ctx context.Context, cred string, ls []Listen) error
}

// Options configures a Scrobbler.
type Options struct {
	Providers []Provider
	// QueuePath persists undelivered listens (no credentials); "" keeps them
	// in memory only.
	QueuePath string
	MaxQueue  int // per provider; 0 means DefaultMaxQueue
	Logger    *slog.Logger
	// OnChange runs (outside locks) when a queue length or a provider's
	// rejected state changes.
	OnChange func()
	// Backoff is the wait before retry attempt n (from 0); nil doubles from
	// 5 s to 5 min.
	Backoff func(n int) time.Duration
	Now     func() time.Time // nil means time.Now
}

// State is one provider's delivery state.
type State struct {
	Pending  int  // listens waiting to be sent
	Rejected bool // the service refused the credentials; reconnect
}

// Scrobbler delivers now-playing announcements and listens to its
// providers. Observe never blocks on the network: delivery runs on one
// goroutine per provider once Start is called (after the first paint),
// retrying failures with backoff.
type Scrobbler struct {
	log      *slog.Logger
	onChange func()
	backoff  func(int) time.Duration

	ctx    context.Context // cancelled by Close
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu      sync.Mutex
	tracker Tracker
	q       *queue
	accts   []*account
	started bool
	closed  bool
}

type account struct {
	p        Provider
	cred     string // "" when not connected
	enabled  bool
	rejected bool
	now      *Track // announcement waiting to be sent
	wake     chan struct{}
}

// New returns a Scrobbler holding the listens a previous run saved.
func New(o Options) *Scrobbler {
	log := o.Logger
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	maxQ := o.MaxQueue
	if maxQ <= 0 {
		maxQ = DefaultMaxQueue
	}
	backoff := o.Backoff
	if backoff == nil {
		backoff = defaultBackoff
	}
	s := &Scrobbler{
		log:      log,
		onChange: o.OnChange,
		backoff:  backoff,
		q:        newQueue(o.QueuePath, maxQ),
		tracker:  Tracker{now: o.Now},
	}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	for _, p := range o.Providers {
		s.accts = append(s.accts, &account{p: p, wake: make(chan struct{}, 1)})
	}
	if err := s.q.load(); err != nil {
		log.Warn("scrobble: discarding saved queue", "err", err)
	}
	return s
}

func defaultBackoff(n int) time.Duration {
	return min(5*time.Second<<min(n, 10), 5*time.Minute)
}

func (s *Scrobbler) account(name string) *account {
	for _, a := range s.accts {
		if a.p.Name() == name {
			return a
		}
	}
	return nil
}

func (a *account) poke() {
	select {
	case a.wake <- struct{}{}:
	default:
	}
}

// SetCredential connects provider name with cred ("" disconnects it but
// keeps its queue, for a credential that could not be read). New
// credentials clear the rejected state and resume delivery.
func (s *Scrobbler) SetCredential(name, cred string) {
	s.mu.Lock()
	a := s.account(name)
	if a == nil {
		s.mu.Unlock()
		return
	}
	changed := a.rejected && cred != a.cred
	if cred != a.cred {
		a.rejected = false
	}
	a.cred = cred
	s.mu.Unlock()
	a.poke()
	if changed {
		s.changed()
	}
}

// SetEnabled pauses (false) or resumes taking new listens and
// announcements for name. Listens already queued are still delivered.
func (s *Scrobbler) SetEnabled(name string, on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if a := s.account(name); a != nil {
		a.enabled = on
		if !on {
			a.now = nil
		}
	}
}

// Forget disconnects name and drops its queued listens.
func (s *Scrobbler) Forget(name string) {
	s.mu.Lock()
	a := s.account(name)
	if a == nil {
		s.mu.Unlock()
		return
	}
	a.cred, a.enabled, a.rejected, a.now = "", false, false, nil
	s.q.clear(name)
	s.saveLocked()
	s.mu.Unlock()
	s.changed()
}

// State reports name's delivery state.
func (s *Scrobbler) State(name string) State {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.account(name)
	if a == nil {
		return State{}
	}
	return State{Pending: s.q.len(name), Rejected: a.rejected}
}

// Observe takes one coordinator transition (use it with
// Coordinator.ObserveActivity). After Close it still queues completed
// listens on disk, so the coordinator's final end event is kept.
func (s *Scrobbler) Observe(act coordinator.Activity) {
	s.mu.Lock()
	now, done := s.tracker.Observe(act)
	queued := false
	for _, a := range s.accts {
		if !a.enabled || a.cred == "" {
			continue
		}
		if now != nil && !a.rejected {
			a.now = now
			a.poke()
		}
		if done != nil {
			s.q.push(a.p.Name(), *done)
			queued = true
			a.poke()
		}
	}
	if queued {
		s.saveLocked()
	}
	s.mu.Unlock()
	if queued {
		s.changed()
	}
}

// Start begins delivery. Call it once the network may be used.
func (s *Scrobbler) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started || s.closed {
		return
	}
	s.started = true
	for _, a := range s.accts {
		s.wg.Add(1)
		diag.Go(s.log, "scrobble "+a.p.Name(), func() {
			defer s.wg.Done()
			s.run(a)
		})
		a.poke()
	}
}

// Close stops delivery (abandoning requests in flight) and saves the queue.
func (s *Scrobbler) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.mu.Unlock()
	s.cancel()
	s.wg.Wait()
	s.mu.Lock()
	s.saveLocked()
	s.mu.Unlock()
}

func (s *Scrobbler) saveLocked() {
	if err := s.q.save(); err != nil {
		s.log.Warn("scrobble: save queue", "err", err)
	}
}

func (s *Scrobbler) changed() {
	if s.onChange != nil {
		s.onChange()
	}
}

// run delivers for a until Close: the latest announcement, then the queue.
func (s *Scrobbler) run(a *account) {
	name := a.p.Name()
	attempt := 0
	for {
		var retry *time.Timer
		var fire <-chan time.Time
		if attempt > 0 {
			retry = time.NewTimer(s.backoff(attempt - 1))
			fire = retry.C
		}
		select {
		case <-s.ctx.Done():
		case <-a.wake:
		case <-fire:
		}
		if retry != nil {
			retry.Stop()
		}
		if s.ctx.Err() != nil {
			return
		}
		ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
		s.announce(ctx, a)
		err := s.flush(ctx, a)
		cancel()
		switch {
		case err == nil:
			attempt = 0
		case s.ctx.Err() != nil:
			return
		default:
			attempt++
			s.log.Warn("scrobble: delivery failed; will retry", "provider", name, "err", err, "attempt", attempt)
		}
	}
}

// announce sends the pending now-playing track once; failures are not
// retried (a stale announcement is useless).
func (s *Scrobbler) announce(ctx context.Context, a *account) {
	s.mu.Lock()
	t, cred := a.now, a.cred
	a.now = nil
	ok := t != nil && cred != "" && a.enabled && !a.rejected
	s.mu.Unlock()
	if !ok {
		return
	}
	err := a.p.NowPlaying(ctx, cred, *t)
	switch {
	case err == nil:
	case errors.Is(err, ErrAuth):
		s.reject(a, cred, err)
	default:
		s.log.Debug("scrobble: now playing not sent", "provider", a.p.Name(), "err", err)
	}
}

// flush sends the queue in batches. Batches the service refuses as invalid
// are dropped so they cannot block the queue; refused credentials stop it.
func (s *Scrobbler) flush(ctx context.Context, a *account) error {
	name := a.p.Name()
	for {
		s.mu.Lock()
		cred, rejected := a.cred, a.rejected
		batch, mark := s.q.head(name, a.p.MaxBatch())
		s.mu.Unlock()
		if cred == "" || rejected || len(batch) == 0 {
			return nil
		}
		err := a.p.Submit(ctx, cred, batch)
		if errors.Is(err, ErrRejected) {
			s.log.Error("scrobble: service refused listens; dropping them", "provider", name, "listens", len(batch), "err", err)
			err = nil
		}
		if errors.Is(err, ErrAuth) {
			s.reject(a, cred, err)
			return nil
		}
		if err != nil {
			return err
		}
		s.mu.Lock()
		s.q.pop(name, len(batch), mark)
		s.saveLocked()
		s.mu.Unlock()
		s.log.Debug("scrobble: delivered", "provider", name, "listens", len(batch))
		s.changed()
	}
}

// reject marks a's credentials refused, unless they changed since cred was
// sent.
func (s *Scrobbler) reject(a *account, cred string, err error) {
	s.mu.Lock()
	if a.cred != cred || a.rejected {
		s.mu.Unlock()
		return
	}
	a.rejected, a.now = true, nil
	s.mu.Unlock()
	s.log.Warn("scrobble: credentials rejected; reconnect the account", "provider", a.p.Name(), "err", err)
	s.changed()
}
