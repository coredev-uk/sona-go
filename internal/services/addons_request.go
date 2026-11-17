package services

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/coredev-uk/sona-go/internal/addons"
	"github.com/coredev-uk/sona-go/internal/cli"
	"github.com/coredev-uk/sona-go/internal/config"
)

// The requests of Sona to its own add-ons (cli.HostMethods) and what keeps
// them up: the supervisor side of #302's proxies. A third-party add-on is
// never sent one: Request refuses any ID that is not one of Sona's own, and
// the session refuses it again unless the supervisor marked it bundled.

// inUse reports whether Sona's own add-on id has something to do: its
// integration is in use under st (bundledWanted), or a connect flow holds a
// lease on it.
func (s *AddonsService) inUse(id string, st config.Settings) bool {
	if bundledWanted(id, st) {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.leases[id] > 0
}

// Lease holds Sona's own add-on id up until release is called, so a flow
// that needs it before it has an account to serve (connecting one) can start
// it. It starts the add-on when it is on and the network is open; Request
// waits for it. release is safe to call twice. A lease on an ID that is not
// one of Sona's own add-ons holds nothing.
func (s *AddonsService) Lease(id string) (release func()) {
	if !addons.IsBundled(id) {
		return func() {}
	}
	s.mu.Lock()
	s.leases[id]++
	s.mu.Unlock()
	s.reconcile()
	var once sync.Once
	return func() {
		once.Do(func() {
			s.mu.Lock()
			if s.leases[id]--; s.leases[id] <= 0 {
				delete(s.leases, id)
			}
			s.mu.Unlock()
			s.reconcile()
		})
	}
}

// Request sends one of Sona's requests (cli.HostMethods) to the running
// bundled add-on id and decodes the result into out (nil: ignored). It waits
// for an add-on that is starting, until ctx ends, and fails with
// ErrAddonUnavailable for one that is off, refused or crashed. An error the
// add-on answered is the *cli.Error with its code.
func (s *AddonsService) Request(ctx context.Context, id, method string, params, out any) error {
	if !addons.IsBundled(id) {
		return fmt.Errorf("%w: %q is not one of Sona's own add-ons", ErrInvalidArgument, id)
	}
	sess, err := s.session(ctx, id)
	if err != nil {
		return err
	}
	raw, err := sess.Request(ctx, method, params)
	if err != nil {
		return err
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}

// session is the session of add-on id once it is up. It waits while the
// add-on is starting (or the network is not yet open, which starts it), and
// gives up at once when nothing is going to start it.
func (s *AddonsService) session(ctx context.Context, id string) (*cli.Session, error) {
	for {
		wake := s.ticker()
		s.mu.Lock()
		r, started, closed := s.runners[id], s.started, s.closed
		s.mu.Unlock()
		if closed {
			return nil, ErrAddonUnavailable
		}
		if sess, _ := r.current(); sess != nil {
			return sess, nil
		}
		if r != nil {
			if st, _ := r.snapshot(); st != AddonStarting && st != AddonRestarting {
				return nil, ErrAddonUnavailable
			}
		} else if started {
			return nil, ErrAddonUnavailable
		}
		select {
		case <-wake:
		case <-ctx.Done():
			return nil, fmt.Errorf("add-on %s: %w", id, ErrAddonUnavailable)
		}
	}
}

// ticker returns a channel that closes at the next change of an add-on's
// state.
func (s *AddonsService) ticker() <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ticked == nil {
		s.ticked = make(chan struct{})
	}
	return s.ticked
}

// tick wakes everything waiting on ticker.
func (s *AddonsService) tick() {
	s.mu.Lock()
	t := s.ticked
	s.ticked = nil
	s.mu.Unlock()
	if t != nil {
		close(t)
	}
}
