package lyrics

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/db"
)

// Defaults for Options.
const (
	// DefaultTimeout bounds one provider's answer (all its requests).
	DefaultTimeout = 10 * time.Second
)

// spacing is the least time between two requests to a provider: the
// community services get a gentle pace, Apple its client's own policy.
var spacing = map[string]time.Duration{
	config.LyricsMusixmatch: time.Second,
	config.LyricsLRCLIB:     250 * time.Millisecond,
	config.LyricsNetEase:    500 * time.Millisecond,
}

// Backoff after failures: a provider that rate limits Sona is left alone
// for what it asked or rateLimitBase doubling per repeat; one failing in
// other ways (offline, errors, timeouts) from its second failure in a row,
// for failureBase doubling. Both stop growing at maxBackoff.
const (
	rateLimitBase = time.Minute
	failureBase   = 15 * time.Second
	maxBackoff    = time.Hour
)

// Options configure a Resolver.
type Options struct {
	DB        *db.DB
	Providers []Provider
	// Settings returns the current lyrics settings (order, switches,
	// word-timing preference).
	Settings func() config.Lyrics
	Logger   *slog.Logger
	Now      func() time.Time // default time.Now
	Timeout  time.Duration    // per provider; default DefaultTimeout
}

// Resolver finds a song's lyrics across the enabled providers.
type Resolver struct {
	db        *db.DB
	providers map[string]Provider
	settings  func() config.Lyrics
	log       *slog.Logger
	now       func() time.Time
	timeout   time.Duration
	limits    map[string]*limiter
	flight    singleflight.Group
	// sleep waits for a provider's next request slot; tests replace it.
	sleep func(ctx context.Context, d time.Duration) error
}

// New returns a Resolver over o.Providers.
func New(o Options) *Resolver {
	r := &Resolver{
		db: o.DB, settings: o.Settings, log: o.Logger, now: o.Now, timeout: o.Timeout,
		providers: map[string]Provider{}, limits: map[string]*limiter{}, sleep: sleepCtx,
	}
	if r.log == nil {
		r.log = slog.New(slog.DiscardHandler)
	}
	if r.now == nil {
		r.now = time.Now
	}
	if r.timeout <= 0 {
		r.timeout = DefaultTimeout
	}
	for _, p := range o.Providers {
		r.providers[p.ID()] = p
		r.limits[p.ID()] = &limiter{spacing: spacing[p.ID()]}
	}
	return r
}

// Lyrics returns the best lyrics for req from the enabled providers, or an
// error matching ErrNotFound when none has any. When no provider has
// lyrics and one failed (rather than answering "none"), its error is
// returned so the caller can offer a retry. Concurrent calls for the same
// request share one lookup, which keeps going (to fill the cache) when
// the caller gives up.
func (r *Resolver) Lyrics(ctx context.Context, req Request) (Lyrics, error) {
	cfg := r.settings()
	var ps []Provider
	for _, id := range cfg.Enabled() {
		if p, ok := r.providers[id]; ok {
			ps = append(ps, p)
		}
	}
	if len(ps) == 0 {
		return Lyrics{}, fmt.Errorf("%w: no lyrics provider is on", ErrNotFound)
	}
	ids := make([]string, len(ps))
	for i, p := range ps {
		ids[i] = p.ID()
	}
	key := fmt.Sprintf("%s|%s|%s|%t", req.Key(), strings.ToLower(req.Language), strings.Join(ids, ","), cfg.PreferWordTiming)
	ch := r.flight.DoChan(key, func() (any, error) {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*r.timeout)
		defer cancel()
		return r.resolve(ctx, req, ps, cfg.PreferWordTiming)
	})
	select {
	case res := <-ch:
		if res.Err != nil {
			return Lyrics{}, res.Err
		}
		return res.Val.(Lyrics), nil
	case <-ctx.Done():
		return Lyrics{}, ctx.Err()
	}
}

// outcome is one provider's answer: lyrics, or an error (ErrNotFound for
// none).
type outcome struct {
	i   int
	l   Lyrics
	err error
}

// resolve asks the providers (cache first, then the network in parallel)
// and returns as soon as no provider still running could change the
// answer: a later provider can only win with better timing, an earlier
// one with equal timing too. While the best result lacks a wanted
// translation, a provider still running that may carry one is awaited too.
func (r *Resolver) resolve(ctx context.Context, req Request, ps []Provider, preferWord bool) (Lyrics, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	track := req.Key()
	results := make([]*outcome, len(ps))
	ch := make(chan outcome, len(ps))
	for i, p := range ps {
		lang := langKey(p, req)
		e, ok := r.cached(ctx, p, track, lang)
		if ok && e.fresh {
			o := outcome{i: i, l: e.lyrics}
			if !e.found {
				o.err = ErrNotFound
			}
			results[i] = &o
			continue
		}
		var stale *Lyrics
		if ok && e.found {
			stale = &e.lyrics
		}
		go func() {
			l, err := r.fetch(ctx, p, req, track, lang, stale)
			ch <- outcome{i: i, l: l, err: err}
		}()
	}
	for {
		if l, done, err := decide(results, ps, req, preferWord, false); done {
			return l, err
		}
		select {
		case o := <-ch:
			results[o.i] = &o
		case <-ctx.Done():
			// Out of time: the best answer so far, without those still running.
			if l, _, err := decide(results, ps, req, preferWord, true); err == nil {
				return l, nil
			}
			return Lyrics{}, ctx.Err()
		}
	}
}

// decide picks the answer once it can no longer change (done). final
// decides now, as if every provider still running had no lyrics.
func decide(results []*outcome, ps []Provider, req Request, preferWord, final bool) (Lyrics, bool, error) {
	bi, rank := -1, 0
	for i, o := range results {
		if o != nil && o.err == nil {
			if r := o.l.rank(preferWord); r > rank {
				bi, rank = i, r
			}
		}
	}
	pending := func(from, to int, want func(Provider) bool) bool {
		for j := from; j < to && !final; j++ {
			if results[j] == nil && want(ps[j]) {
				return true
			}
		}
		return false
	}
	all := func(Provider) bool { return true }
	if bi < 0 {
		if pending(0, len(ps), all) {
			return Lyrics{}, false, nil
		}
		return Lyrics{}, true, failure(results)
	}
	// An earlier provider could still tie and win; a lower rank can still
	// be beaten by anyone.
	if pending(0, bi, all) || rank < maxRank(preferWord) && pending(bi+1, len(ps), all) {
		return Lyrics{}, false, nil
	}
	l := results[bi].l
	l.Lines = slices.Clone(l.Lines)
	if wantsTranslation(l, req) {
		if pending(0, len(ps), Provider.Localized) {
			return Lyrics{}, false, nil
		}
		for _, o := range results {
			if o != nil && o.err == nil && o.i != bi && borrowTranslation(&l, o.l, req.Language) {
				break
			}
		}
	}
	return l, true, nil
}

// failure is the error when no provider has lyrics: the first real
// failure, else ErrNotFound (every provider said none, or couldn't be
// asked).
func failure(results []*outcome) error {
	for _, o := range results {
		var rl *RateLimitError
		if o.err != nil && !errors.Is(o.err, ErrNotFound) && !errors.Is(o.err, ErrUnavailable) && !errors.As(o.err, &rl) {
			return o.err
		}
	}
	return ErrNotFound
}

// wantsTranslation reports whether l lacks the translation req asks for
// (and isn't already in that language).
func wantsTranslation(l Lyrics, req Request) bool {
	lang := primary(req.Language)
	return lang != "" && l.TranslationLanguage == "" && primary(l.Language) != lang
}

// borrowTranslation copies into l the translations other carries in lang,
// matched by line text; it reports whether any line got one.
func borrowTranslation(l *Lyrics, other Lyrics, lang string) bool {
	if other.TranslationLanguage == "" || primary(other.TranslationLanguage) != primary(lang) {
		return false
	}
	byText := map[string]*Text{}
	for _, ln := range other.Lines {
		if ln.Translation != nil {
			byText[normalize(ln.Text)] = ln.Translation
		}
	}
	found := false
	for i := range l.Lines {
		if t, ok := byText[normalize(l.Lines[i].Text)]; ok && l.Lines[i].Translation == nil {
			l.Lines[i].Translation = t
			found = true
		}
	}
	if found {
		l.TranslationLanguage = other.TranslationLanguage
	}
	return found
}

// fetch asks one provider (within its pace, backoff and timeout) and
// caches the answer. When it fails, stale cached lyrics stand in.
func (r *Resolver) fetch(ctx context.Context, p Provider, req Request, track, lang string, stale *Lyrics) (Lyrics, error) {
	lim := r.limits[p.ID()]
	wait, err := lim.reserve(r.now())
	if err == nil && wait > 0 {
		err = r.sleep(ctx, wait)
	}
	var l Lyrics
	if err == nil {
		pctx, cancel := context.WithTimeout(ctx, r.timeout)
		l, err = p.Fetch(pctx, req)
		cancel()
		if err == nil && (len(l.Lines) == 0 || l.empty()) {
			err = ErrNotFound
		}
		if ctx.Err() == nil {
			lim.done(err, r.now())
		}
	}
	switch {
	case err == nil:
		l.Provider = p.ID()
		r.store(context.WithoutCancel(ctx), p, track, lang, &l)
		return l, nil
	case errors.Is(err, ErrNotFound):
		r.store(context.WithoutCancel(ctx), p, track, lang, nil)
		return Lyrics{}, err
	case ctx.Err() != nil:
		// The answer was decided without this provider.
		return Lyrics{}, ctx.Err()
	}
	if !errors.Is(err, ErrUnavailable) {
		r.log.Warn("lyrics: provider failed", "provider", p.ID(), "err", err)
	}
	if stale != nil {
		return *stale, nil
	}
	return Lyrics{}, err
}

// langKey is the cache's language for p's answer to req.
func langKey(p Provider, req Request) string {
	if !p.Localized() {
		return ""
	}
	return strings.ToLower(req.Language)
}

// limiter paces one provider's requests and backs off after failures.
type limiter struct {
	spacing time.Duration

	mu    sync.Mutex
	next  time.Time // the next request's earliest start
	until time.Time // backing off until then
	fails int       // failures in a row
}

// reserve books the next request slot and returns how long to wait for
// it, or ErrUnavailable while backing off.
func (l *limiter) reserve(now time.Time) (time.Duration, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Before(l.until) {
		return 0, fmt.Errorf("%w: backing off for %s", ErrUnavailable, l.until.Sub(now).Round(time.Second))
	}
	start := now
	if l.next.After(now) {
		start = l.next
	}
	l.next = start.Add(l.spacing)
	return start.Sub(now), nil
}

// done records a request's result.
func (l *limiter) done(err error, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	var rl *RateLimitError
	switch {
	case err == nil || errors.Is(err, ErrNotFound):
		l.fails = 0
	case errors.Is(err, ErrUnavailable):
	case errors.As(err, &rl):
		l.fails++
		l.until = now.Add(min(max(rl.RetryAfter, rateLimitBase<<min(l.fails-1, 6)), maxBackoff))
	default:
		l.fails++
		if l.fails >= 2 {
			l.until = now.Add(min(failureBase<<min(l.fails-2, 8), maxBackoff))
		}
	}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
