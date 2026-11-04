package store

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Forever is a Stale that never expires (v0.2.0's TanStack default,
// staleTime: Infinity): the data is refetched only after Invalidate.
const Forever = time.Duration(1<<63 - 1)

const (
	defaultKeep = 5 * time.Minute
	// mountGap is how long a query may go unused before the next Use counts
	// as a new mount (a view built again, as TanStack's observer mounting):
	// only then is stale data refetched. Use is called every frame, so
	// frames inside the gap are the same mount.
	mountGap = time.Second
	// activeFor is how recently a query was used for Invalidate to refetch
	// it at once; older ones are only marked stale, and refetch when next
	// used.
	activeFor = time.Minute
	// errorRetry is how long a query with no data waits before Use asks
	// again after a failure, so a broken service is not asked every frame.
	errorRetry = 5 * time.Second
	// sweepEvery is how often dropped queries are looked for when nothing
	// calls Use.
	sweepEvery = 30 * time.Second
)

// Options tune a query.
type Options struct {
	// Stale is how long fetched data counts as fresh. A view that starts
	// using a query (a mount, see mountGap) refetches data older than that
	// and shows the old data meanwhile. 0 is stale at once, TanStack's
	// default; Forever is never.
	Stale time.Duration
	// Keep is how long an unused query is kept before it is dropped (and
	// its fetch cancelled). 0 is five minutes.
	Keep time.Duration
}

// Result is a query as a view reads it.
type Result[T any] struct {
	Data     T
	Err      error // the last fetch's error (services.Error inside); Data may still be an earlier result
	HasData  bool
	Loading  bool // no data yet, a fetch in flight
	Fetching bool // any fetch in flight
}

// Client caches query results by Key. A view calls Use every frame; the
// fetch runs on a goroutine and its result is published through the Poster
// the Client was made with, which redraws.
type Client struct {
	post Poster
	now  func() time.Time

	mu      sync.Mutex
	entries map[string]*entry
	timer   *time.Timer
	// lastUse is when Use last ran, and active the time spent using
	// queries: each Use advances it by what passed since the last one, at
	// most mountGap. Entries are dropped by it, not by the wall clock: a
	// window with no frames runs no Use, and its queries are still on
	// screen.
	lastUse   time.Time
	active    time.Duration
	lastSweep time.Duration // active, at the last sweep
}

type entry struct {
	key Key
	id  string

	ctx  context.Context // cancelled when the entry is dropped
	stop context.CancelFunc

	gen         uint64 // of the latest fetch; an older one's result is ignored
	cancelFetch context.CancelFunc
	fetching    bool
	fetch       func(ctx context.Context) (any, error)

	data    any
	has     bool
	err     error
	updated time.Time // of the data
	failed  time.Time // of the last failure
	invalid bool      // Invalidate ran since the data arrived

	used   time.Time
	usedAt time.Duration // Client.active when last used or made
	stale  time.Duration
	keep   time.Duration
}

// NewClient returns a Client publishing through post (nil: at once, on the
// calling goroutine).
func NewClient(post Poster) *Client {
	if post == nil {
		post = runNow
	}
	return &Client{post: post, now: time.Now, entries: map[string]*entry{}}
}

// Use returns the cached result for key and starts a fetch, on a goroutine
// whose context is cancelled when the entry is dropped, when there is no
// data or the data is stale as a view starts using it. Fetches of one key
// never run twice at once. Call it from a view every frame: it marks the
// query in use. Two calls with one key must mean one query: the key holds
// every input of fetch.
func Use[T any](cl *Client, key Key, fetch func(ctx context.Context) (T, error), o ...Options) Result[T] {
	var opt Options
	if len(o) > 0 {
		opt = o[0]
	}
	now := cl.now()
	cl.mu.Lock()
	defer cl.mu.Unlock()
	if !cl.lastUse.IsZero() {
		cl.active += min(now.Sub(cl.lastUse), mountGap)
	}
	cl.lastUse = now
	cl.sweepLocked(false)
	e := cl.entryLocked(key, now)
	mounting := e.used.IsZero() || now.Sub(e.used) > mountGap
	e.used, e.usedAt = now, cl.active
	e.stale, e.keep = opt.Stale, keepOf(opt)
	if e.fetch == nil || mounting {
		e.fetch = func(ctx context.Context) (any, error) { return fetch(ctx) }
	}
	data, typed := e.data.(T)
	has := e.has && typed
	var need bool
	switch {
	case e.fetching:
	case !has:
		need = e.failed.IsZero() || now.Sub(e.failed) >= errorRetry
	case e.invalid:
		need = true
	case mounting:
		need = now.Sub(e.updated) >= e.stale
	}
	if need {
		cl.fetchLocked(e, now)
	}
	r := Result[T]{Err: e.err, HasData: has, Fetching: e.fetching}
	if has {
		r.Data = data
	}
	r.Loading = !has && e.fetching
	return r
}

// Set writes a query's data: an optimistic update, or an event's payload.
// The data counts as fetched now; a view using the key redraws with it.
func Set[T any](cl *Client, key Key, v T) {
	now := cl.now()
	cl.mu.Lock()
	e := cl.entryLocked(key, now)
	e.data, e.has, e.err = v, true, nil
	e.updated, e.invalid = now, false
	if e.used.IsZero() {
		e.usedAt = cl.active // an entry made by Set and not yet used lives from now
	}
	cl.mu.Unlock()
	cl.post(func() {})
}

// Invalidate marks every query whose key starts with one of prefixes stale
// (all of them when none is given) and refetches the ones in use now; the
// rest refetch when they are next used. A fetch in flight is cancelled and
// started again.
func (cl *Client) Invalidate(prefixes ...Key) {
	now := cl.now()
	cl.mu.Lock()
	defer cl.mu.Unlock()
	for _, e := range cl.entries {
		if len(prefixes) > 0 && !matchesAny(e.key, prefixes) {
			continue
		}
		e.invalid = true
		if e.fetch != nil && now.Sub(e.used) < activeFor {
			cl.fetchLocked(e, now)
		}
	}
}

func matchesAny(k Key, prefixes []Key) bool {
	for _, p := range prefixes {
		if k.hasPrefix(p) {
			return true
		}
	}
	return false
}

// Reset drops every query, as a sign-out must: their fetches are cancelled.
func (cl *Client) Reset() {
	cl.mu.Lock()
	for id, e := range cl.entries {
		e.stop()
		delete(cl.entries, id)
	}
	cl.mu.Unlock()
	cl.post(func() {})
}

func keepOf(o Options) time.Duration {
	if o.Keep > 0 {
		return o.Keep
	}
	return defaultKeep
}

func (cl *Client) entryLocked(key Key, now time.Time) *entry {
	id := key.id()
	if e := cl.entries[id]; e != nil {
		return e
	}
	ctx, stop := context.WithCancel(context.Background())
	e := &entry{key: append(Key(nil), key...), id: id, ctx: ctx, stop: stop, keep: defaultKeep, usedAt: cl.active}
	cl.entries[id] = e
	if cl.timer == nil {
		cl.timer = time.AfterFunc(sweepEvery, cl.tick)
	}
	// An entry made by Set and not yet used lives from now.
	e.updated = now
	return e
}

// fetchLocked starts e's fetch, cancelling one in flight.
func (cl *Client) fetchLocked(e *entry, now time.Time) {
	if e.cancelFetch != nil {
		e.cancelFetch()
	}
	e.gen++
	gen := e.gen
	ctx, cancel := context.WithCancel(e.ctx)
	e.cancelFetch = cancel
	e.fetching, e.invalid = true, false
	fetch := e.fetch
	go func() {
		v, err := safeFetch(ctx, fetch)
		cl.post(func() { cl.finish(e, gen, v, err) })
	}()
}

// safeFetch runs fetch, turning a panic into an error: a query must not take
// the app down from its own goroutine.
func safeFetch(ctx context.Context, fetch func(context.Context) (any, error)) (v any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("query panicked: %v", r)
		}
	}()
	return fetch(ctx)
}

// finish publishes a fetch's result, on the main thread.
func (cl *Client) finish(e *entry, gen uint64, v any, err error) {
	now := cl.now()
	cl.mu.Lock()
	defer cl.mu.Unlock()
	if cl.entries[e.id] != e || e.gen != gen {
		return // dropped, or superseded by a newer fetch
	}
	e.fetching = false
	e.cancelFetch()
	e.cancelFetch = nil
	if err != nil {
		e.err, e.failed = err, now
		return
	}
	e.data, e.has, e.err = v, true, nil
	e.updated = now
}

// tick drops what has gone unused, then looks again later if anything is
// left.
func (cl *Client) tick() {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	cl.sweepLocked(true)
	cl.timer = nil
	if len(cl.entries) > 0 {
		cl.timer = time.AfterFunc(sweepEvery, cl.tick)
	}
}

// sweepLocked drops the entries unused for their keep time of active time
// (see Client.active), at most once a second of it unless force.
func (cl *Client) sweepLocked(force bool) {
	if !force && cl.active-cl.lastSweep < time.Second {
		return
	}
	cl.lastSweep = cl.active
	for id, e := range cl.entries {
		if cl.active-e.usedAt >= e.keep {
			e.stop()
			delete(cl.entries, id)
		}
	}
}
