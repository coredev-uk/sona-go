package artwork

import (
	"context"
	"sync"
)

// flights runs one function per key at a time and hands its result to every
// caller that asks for the key meanwhile, as singleflight does. The function
// runs detached from its callers, so one that gives up does not fail the
// others, but its context ends when the last caller has given up: a cover
// that scrolled off the screen stops downloading, a cover someone still waits
// for does not.
type flights struct {
	mu    sync.Mutex
	calls map[string]*call
	// abandoned counts the calls whose last caller gave up that are still
	// running: wait.
	abandoned sync.WaitGroup
}

type call struct {
	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
	val     any
	err     error
	callers int
}

// do returns fn's result for key, starting fn if no call for key is running.
// fn's context ends when every caller's ctx has ended. do returns ctx's error
// as soon as ctx ends.
func (f *flights) do(ctx context.Context, key string, fn func(context.Context) (any, error)) (any, error) {
	f.mu.Lock()
	cl := f.calls[key]
	if cl == nil || cl.ctx.Err() != nil { // none, or one that has been called off
		cl = &call{done: make(chan struct{})}
		cl.ctx, cl.cancel = context.WithCancel(context.Background())
		if f.calls == nil {
			f.calls = map[string]*call{}
		}
		f.calls[key] = cl
		go f.run(key, cl, fn)
	}
	cl.callers++
	f.mu.Unlock()

	select {
	case <-cl.done:
		return cl.val, cl.err
	case <-ctx.Done():
	}
	f.mu.Lock()
	cl.callers--
	if cl.callers == 0 {
		cl.cancel() // under the lock: nobody joins a call as it is called off
	}
	f.mu.Unlock()
	f.abandoned.Add(1)
	go func() {
		<-cl.done
		f.abandoned.Done()
	}()
	return nil, ctx.Err()
}

func (f *flights) run(key string, cl *call, fn func(context.Context) (any, error)) {
	cl.val, cl.err = fn(cl.ctx)
	f.mu.Lock()
	if f.calls[key] == cl {
		delete(f.calls, key)
	}
	f.mu.Unlock()
	cl.cancel()
	close(cl.done)
}

// wait blocks until the calls their callers gave up on have returned.
func (f *flights) wait() { f.abandoned.Wait() }
