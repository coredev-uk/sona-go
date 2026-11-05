package artwork

import (
	"context"
	"errors"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

func within(t *testing.T, what string, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

// waiting reports whether some call has n callers waiting for it.
func (f *flights) waiting(n int) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, cl := range f.calls {
		if cl.callers >= n {
			return true
		}
	}
	return false
}

func TestFlightsShareOneCallAmongCallers(t *testing.T) {
	var f flights
	var runs atomic.Int32
	release := make(chan struct{})
	fn := func(context.Context) (any, error) {
		runs.Add(1)
		<-release
		return "v", nil
	}
	results := make(chan any, 2)
	for range 2 {
		go func() {
			v, err := f.do(t.Context(), "k", fn)
			if err != nil {
				v = err
			}
			results <- v
		}()
	}
	for !f.waiting(2) { // both callers wait for the one call
		runtime.Gosched()
	}
	close(release)
	for range 2 {
		select {
		case v := <-results:
			if v != "v" {
				t.Errorf("result %v", v)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("a caller was not answered")
		}
	}
	if n := runs.Load(); n != 1 {
		t.Errorf("the function ran %d times for two callers", n)
	}
}

func TestFlightsCancelTheCallWhenTheLastCallerGivesUp(t *testing.T) {
	var f flights
	started, ended := make(chan struct{}), make(chan error, 1)
	fn := func(ctx context.Context) (any, error) {
		close(started)
		<-ctx.Done()
		ended <- ctx.Err()
		return nil, ctx.Err()
	}
	first, cancelFirst := context.WithCancel(t.Context())
	second, cancelSecond := context.WithCancel(t.Context())
	errs := make(chan error, 2)
	go func() { _, err := f.do(first, "k", fn); errs <- err }()
	within(t, "the call to start", started)
	go func() { _, err := f.do(second, "k", fn); errs <- err }()
	for !f.waiting(2) {
		runtime.Gosched()
	}

	// One caller gives up: the other still waits, so the call goes on.
	cancelFirst()
	if err := <-errs; !errors.Is(err, context.Canceled) {
		t.Fatalf("the caller that gave up got %v", err)
	}
	select {
	case err := <-ended:
		t.Fatalf("the call ended (%v) while a caller still waited", err)
	default:
	}
	// The last one gives up: the call's context ends.
	cancelSecond()
	if err := <-errs; !errors.Is(err, context.Canceled) {
		t.Fatalf("the last caller got %v", err)
	}
	select {
	case err := <-ended:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("the call ended with %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the call kept going with nobody waiting")
	}
	f.wait() // the call has returned
}

func TestFlightsStartAFreshCallForACallerAfterOneWasCalledOff(t *testing.T) {
	var f flights
	hold, held := make(chan struct{}), make(chan struct{})
	first := func(ctx context.Context) (any, error) {
		close(held)
		<-ctx.Done()
		<-hold // still running when the next caller comes
		return nil, ctx.Err()
	}
	ctx, cancel := context.WithCancel(t.Context())
	gone := make(chan struct{})
	go func() { _, _ = f.do(ctx, "k", first); close(gone) }()
	within(t, "the first call to start", held)
	cancel()
	within(t, "the caller to give up", gone)

	// The first call is called off but has not returned: a new caller must not
	// share its failure.
	v, err := f.do(t.Context(), "k", func(context.Context) (any, error) { return "fresh", nil })
	if err != nil || v != "fresh" {
		t.Errorf("got %v, %v from a call that began after the first was called off", v, err)
	}
	close(hold)
	f.wait()
}
