package auth

import (
	"context"
	"sync"
	"time"

	"github.com/coredev-uk/sona-go/internal/auth/store"
)

// vault runs the credential store's operations one at a time, in the order
// they were asked, each on a goroutine of its own, so a keyring that hangs
// (a Secret Service activation that never completes, an unlock prompt
// nobody answers) holds up no caller beyond its own deadline. An operation
// whose caller stopped waiting still runs, and later ones wait behind it:
// writes and deletes reach the keyring in order, however late it answers.
//
// The store is opened (store.Open probes the keyring and may fall back to
// the 0600 file) by the first operation, and again by one that asks for a
// fresh open.
type vault struct {
	open func() (store.Store, error)

	mu      sync.Mutex
	tail    chan struct{} // closed once the last queued operation has run
	backend store.Backend // the open store's backend; "" until one is open

	st store.Store // used only by the running operation
}

func newVault(open func() (store.Store, error)) *vault {
	done := make(chan struct{})
	close(done)
	return &vault{open: open, tail: done}
}

// Backend reports where the open store keeps the token, "" before the
// first open has answered.
func (v *vault) Backend() store.Backend {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.backend
}

// do queues fn behind every earlier operation and returns at once; the
// channel receives fn's error, or the open error without running fn. With
// fresh the store is opened again first (a retry after the keyring was
// unavailable).
func (v *vault) do(fresh bool, fn func(store.Store) error) <-chan error {
	return v.enqueue(func() error { return v.run(fresh, fn) })
}

// migrate queues a move from the open store to the one open returns:
// fn(from, to) runs with both, and when it succeeds to becomes the open
// store. The open store is opened first if none is.
func (v *vault) migrate(open func() (store.Store, error), fn func(from, to store.Store) error) <-chan error {
	return v.enqueue(func() error {
		if err := v.run(false, func(store.Store) error { return nil }); err != nil {
			return err
		}
		to, err := open()
		if err != nil {
			return err
		}
		if err := fn(v.st, to); err != nil {
			return err
		}
		v.st = to
		v.mu.Lock()
		v.backend = to.Backend()
		v.mu.Unlock()
		return nil
	})
}

func (v *vault) enqueue(op func() error) <-chan error {
	res := make(chan error, 1)
	next := make(chan struct{})
	v.mu.Lock()
	prev := v.tail
	v.tail = next
	v.mu.Unlock()
	go func() {
		defer close(next)
		<-prev
		res <- op()
	}()
	return res
}

func (v *vault) run(fresh bool, fn func(store.Store) error) error {
	if fresh || v.st == nil {
		v.st = nil
		s, err := v.open()
		v.mu.Lock()
		v.backend = ""
		if err == nil {
			v.backend = s.Backend()
		}
		v.mu.Unlock()
		if err != nil {
			return err
		}
		v.st = s
	}
	return fn(v.st)
}

// await waits for a vault operation's answer until ctx ends or limit
// passes (ErrKeyringTimeout); after slowAfter without one it calls slow
// once (nil: nothing).
func await(ctx context.Context, res <-chan error, slowAfter, limit time.Duration, slow func()) error {
	timeout := time.NewTimer(limit)
	defer timeout.Stop()
	slowly := time.NewTimer(slowAfter)
	defer slowly.Stop()
	for {
		select {
		case err := <-res:
			return err
		case <-slowly.C:
			if slow != nil {
				slow()
			}
		case <-timeout.C:
			return ErrKeyringTimeout
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
