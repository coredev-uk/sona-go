package lyrics

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/db"
)

// fakeProvider answers with a fixed result after an optional delay (or
// until released).
type fakeProvider struct {
	id        string
	localized bool
	l         Lyrics
	err       error
	delay     time.Duration
	release   chan struct{} // when set, Fetch waits for it
	calls     atomic.Int32
}

func (p *fakeProvider) ID() string      { return p.id }
func (p *fakeProvider) Localized() bool { return p.localized }

func (p *fakeProvider) Fetch(ctx context.Context, req Request) (Lyrics, error) {
	p.calls.Add(1)
	if p.release != nil {
		select {
		case <-p.release:
		case <-ctx.Done():
			return Lyrics{}, ctx.Err()
		}
	}
	if p.delay > 0 {
		select {
		case <-time.After(p.delay):
		case <-ctx.Done():
			return Lyrics{}, ctx.Err()
		}
	}
	return p.l, p.err
}

func timed(timing string, texts ...string) Lyrics {
	l := Lyrics{Timing: timing, Language: "en"}
	for i, s := range texts {
		ln := Line{Begin: time.Duration(i) * time.Second, End: time.Duration(i+1) * time.Second, Text: s}
		if timing == TimingWord {
			ln.Words = []Word{{Begin: ln.Begin, End: ln.End, Text: s}}
		}
		l.Lines = append(l.Lines, ln)
	}
	return l
}

type harness struct {
	r     *Resolver
	db    *db.DB
	cfg   config.Lyrics
	clock time.Time
	mu    sync.Mutex
}

func newHarness(t *testing.T, ps ...Provider) *harness {
	t.Helper()
	d, err := db.Open(context.Background(), filepath.Join(t.TempDir(), "sona.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	h := &harness{db: d, clock: time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)}
	h.cfg = config.Lyrics{PreferWordTiming: true}
	for _, p := range ps {
		h.cfg.Providers = append(h.cfg.Providers, config.LyricsProvider{ID: p.ID(), Enabled: true})
	}
	h.r = New(Options{DB: d, Providers: ps, Settings: func() config.Lyrics { return h.cfg }, Now: h.now, Timeout: 2 * time.Second})
	h.r.sleep = func(context.Context, time.Duration) error { return nil }
	return h
}

func (h *harness) now() time.Time {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.clock
}

func (h *harness) advance(d time.Duration) {
	h.mu.Lock()
	h.clock = h.clock.Add(d)
	h.mu.Unlock()
}

var req = Request{Track: song}

func TestBestTimingWins(t *testing.T) {
	apple := &fakeProvider{id: config.LyricsApple, l: timed(TimingLine, "line")}
	lrclib := &fakeProvider{id: config.LyricsLRCLIB, l: timed(TimingNone, "plain")}
	mxm := &fakeProvider{id: config.LyricsMusixmatch, l: timed(TimingWord, "word")}
	// The word-timed provider comes last, so the answer waits for (and
	// caches) all three: one that could not change the answer is not
	// awaited, and its answer may reach the cache after a later lookup.
	h := newHarness(t, apple, lrclib, mxm)
	l, err := h.r.Lyrics(context.Background(), req)
	if err != nil || l.Provider != config.LyricsMusixmatch || l.Timing != TimingWord {
		t.Fatalf("word preferred: %+v, %v", l, err)
	}
	// Without the word preference, synced lyrics count alike: the order decides.
	h.cfg.PreferWordTiming = false
	if l, _ = h.r.Lyrics(context.Background(), req); l.Provider != config.LyricsApple {
		t.Fatalf("order: %+v", l)
	}
	// Plain text loses to synced lyrics from a later provider.
	h.cfg.Providers[0], h.cfg.Providers[1] = h.cfg.Providers[1], h.cfg.Providers[0]
	h.cfg.Providers[2].Enabled = false
	if l, _ = h.r.Lyrics(context.Background(), req); l.Provider != config.LyricsApple || l.Timing != TimingLine {
		t.Fatalf("synced over plain: %+v", l)
	}
	// Changing the order and switches was answered from the cache.
	if apple.calls.Load() != 1 || mxm.calls.Load() != 1 || lrclib.calls.Load() != 1 {
		t.Fatalf("calls = %d %d %d", apple.calls.Load(), mxm.calls.Load(), lrclib.calls.Load())
	}
}

func TestFirstWordTimedResultWinsEarly(t *testing.T) {
	apple := &fakeProvider{id: config.LyricsApple, l: timed(TimingWord, "word")}
	slow := &fakeProvider{id: config.LyricsLRCLIB, l: timed(TimingLine, "line"), release: make(chan struct{})}
	h := newHarness(t, apple, slow)
	start := time.Now()
	l, err := h.r.Lyrics(context.Background(), req)
	if err != nil || l.Provider != config.LyricsApple {
		t.Fatalf("lyrics = %+v, %v", l, err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("waited for a provider that could not win")
	}
	close(slow.release)
}

func TestEarlierProviderIsAwaitedForATie(t *testing.T) {
	first := &fakeProvider{id: config.LyricsApple, l: timed(TimingWord, "apple"), delay: 50 * time.Millisecond}
	second := &fakeProvider{id: config.LyricsMusixmatch, l: timed(TimingWord, "mxm")}
	h := newHarness(t, first, second)
	if l, _ := h.r.Lyrics(context.Background(), req); l.Provider != config.LyricsApple {
		t.Fatalf("lyrics from %s, want the first in the order", l.Provider)
	}
}

func TestCachePositiveAndNegative(t *testing.T) {
	none := &fakeProvider{id: config.LyricsApple, err: ErrNotFound}
	line := &fakeProvider{id: config.LyricsLRCLIB, l: timed(TimingLine, "line")}
	h := newHarness(t, none, line)
	for range 2 {
		if l, err := h.r.Lyrics(context.Background(), req); err != nil || l.Provider != config.LyricsLRCLIB || l.Lines[0].Text != "line" {
			t.Fatalf("lyrics = %+v, %v", l, err)
		}
	}
	if none.calls.Load() != 1 || line.calls.Load() != 1 {
		t.Fatalf("calls = %d %d", none.calls.Load(), line.calls.Load())
	}
	// "None" expires before found lyrics do.
	h.advance(NotFoundTTL + time.Minute)
	_, _ = h.r.Lyrics(context.Background(), req)
	if none.calls.Load() != 2 || line.calls.Load() != 1 {
		t.Fatalf("after %s: calls = %d %d", NotFoundTTL, none.calls.Load(), line.calls.Load())
	}
	h.advance(FoundTTL)
	_, _ = h.r.Lyrics(context.Background(), req)
	if line.calls.Load() != 2 {
		t.Fatalf("after %s: calls = %d", FoundTTL, line.calls.Load())
	}
	if n, b, err := CacheSize(context.Background(), h.db); err != nil || n != 2 || b == 0 {
		t.Fatalf("cache size = %d entries, %d bytes, %v", n, b, err)
	}
	h.advance(FoundTTL + time.Minute)
	if n, err := Prune(context.Background(), h.db, h.now()); err != nil || n != 2 {
		t.Fatalf("pruned %d, %v", n, err)
	}
}

func TestStaleLyricsServedWhenProviderFails(t *testing.T) {
	p := &fakeProvider{id: config.LyricsLRCLIB, l: timed(TimingLine, "cached")}
	h := newHarness(t, p)
	_, _ = h.r.Lyrics(context.Background(), req)
	h.advance(FoundTTL + time.Hour)
	p.l, p.err = Lyrics{}, errors.New("offline")
	if l, err := h.r.Lyrics(context.Background(), req); err != nil || l.Lines[0].Text != "cached" {
		t.Fatalf("lyrics = %+v, %v", l, err)
	}
}

func TestNoLyricsAndFailures(t *testing.T) {
	none := &fakeProvider{id: config.LyricsApple, err: ErrNotFound}
	unavailable := &fakeProvider{id: config.LyricsMusixmatch, err: ErrUnavailable}
	h := newHarness(t, none, unavailable)
	if _, err := h.r.Lyrics(context.Background(), req); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	// A provider that failed (rather than having none) is reported, so the
	// view can offer a retry; its failure is not cached.
	broken := &fakeProvider{id: config.LyricsLRCLIB, err: errors.New("HTTP 500")}
	h = newHarness(t, none, broken)
	if _, err := h.r.Lyrics(context.Background(), req); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	broken.err, broken.l = nil, timed(TimingLine, "back")
	if l, err := h.r.Lyrics(context.Background(), req); err != nil || l.Lines[0].Text != "back" {
		t.Fatalf("retry = %+v, %v", l, err)
	}
	// Nothing enabled.
	h.cfg.Providers = nil
	if _, err := h.r.Lyrics(context.Background(), req); !errors.Is(err, ErrNotFound) {
		t.Fatalf("none enabled err = %v", err)
	}
}

func TestRateLimitBacksOff(t *testing.T) {
	p := &fakeProvider{id: config.LyricsMusixmatch, err: &RateLimitError{Provider: config.LyricsMusixmatch, RetryAfter: 10 * time.Minute}}
	h := newHarness(t, p)
	other := Request{Track: Track{CatalogID: "2", Title: "Other", Artist: "Test Artist"}}
	_, _ = h.r.Lyrics(context.Background(), req)
	_, _ = h.r.Lyrics(context.Background(), other)
	if p.calls.Load() != 1 {
		t.Fatalf("calls while backing off = %d", p.calls.Load())
	}
	h.advance(10*time.Minute + time.Second)
	p.err, p.l = nil, timed(TimingLine, "ok")
	if l, err := h.r.Lyrics(context.Background(), other); err != nil || p.calls.Load() != 2 || l.Lines[0].Text != "ok" {
		t.Fatalf("after backoff: %+v, %v, calls %d", l, err, p.calls.Load())
	}
}

func TestLimiter(t *testing.T) {
	now := time.Unix(0, 0)
	l := &limiter{spacing: time.Second}
	for i, want := range []time.Duration{0, time.Second, 2 * time.Second} {
		if d, err := l.reserve(now); err != nil || d != want {
			t.Fatalf("reserve %d = %s, %v", i, d, err)
		}
	}
	// One failure is tolerated, the second backs off, and it grows.
	l = &limiter{}
	l.done(errors.New("timeout"), now)
	if _, err := l.reserve(now); err != nil {
		t.Fatalf("after one failure: %v", err)
	}
	l.done(errors.New("timeout"), now)
	if _, err := l.reserve(now.Add(failureBase - time.Second)); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("after two failures: %v", err)
	}
	l.done(errors.New("timeout"), now)
	if _, err := l.reserve(now.Add(2*failureBase - time.Second)); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("after three failures: %v", err)
	}
	l.done(nil, now)
	if _, err := l.reserve(now.Add(2 * failureBase)); err != nil || l.fails != 0 {
		t.Fatalf("after a success: %v (fails %d)", err, l.fails)
	}
	// A rate limit without Retry-After waits the base, capped at an hour.
	l = &limiter{fails: 20}
	l.done(&RateLimitError{}, now)
	if !l.until.Equal(now.Add(maxBackoff)) {
		t.Fatalf("until = %s", l.until.Sub(now))
	}
}

func TestTranslationBorrowedAndLocalizedCacheKeys(t *testing.T) {
	apple := &fakeProvider{id: config.LyricsApple, localized: true, l: timed(TimingWord, "alpha", "bravo")}
	tr := timed(TimingLine, "alpha", "bravo")
	tr.TranslationLanguage = "es"
	tr.Lines[1].Translation = &Text{Text: "bravo es"}
	mxm := &fakeProvider{id: config.LyricsMusixmatch, localized: true, l: tr, delay: 30 * time.Millisecond}
	plain := &fakeProvider{id: config.LyricsLRCLIB, l: timed(TimingNone, "x")}
	h := newHarness(t, apple, mxm, plain)
	l, err := h.r.Lyrics(context.Background(), Request{Track: song, Language: "es-ES"})
	if err != nil || l.Provider != config.LyricsApple || l.TranslationLanguage != "es" || l.Lines[1].Translation == nil ||
		l.Lines[1].Translation.Text != "bravo es" || l.Lines[0].Translation != nil {
		t.Fatalf("lyrics = %+v, %v", l, err)
	}
	// What was cached for the winner is its own answer: the borrowed
	// translation went into the result only.
	if e, ok := h.r.cached(context.Background(), apple, song.Key(), "es-es"); !ok || e.lyrics.Lines[1].Translation != nil {
		t.Fatalf("cached = %+v, %v", e, ok)
	}
	// Answers that depend on the language are cached per language.
	if l, _ := h.r.Lyrics(context.Background(), Request{Track: song}); l.Provider != config.LyricsApple || apple.calls.Load() != 2 {
		t.Fatalf("no language: %+v, apple calls %d", l, apple.calls.Load())
	}
}

func TestCallerGivingUpLeavesLookupRunning(t *testing.T) {
	p := &fakeProvider{id: config.LyricsLRCLIB, l: timed(TimingLine, "late"), release: make(chan struct{})}
	h := newHarness(t, p)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := h.r.Lyrics(ctx, req); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	close(p.release)
	// The lookup finishes and caches: the next call needs no request.
	deadline := time.Now().Add(2 * time.Second)
	for {
		if e, ok := h.r.cached(context.Background(), p, req.Key(), ""); ok && e.found {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("not cached")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if l, err := h.r.Lyrics(context.Background(), req); err != nil || l.Lines[0].Text != "late" || p.calls.Load() != 1 {
		t.Fatalf("lyrics = %+v, %v, calls %d", l, err, p.calls.Load())
	}
}

// stuckProvider never answers and ignores its context.
type stuckProvider struct{ release chan struct{} }

func (stuckProvider) ID() string      { return config.LyricsApple }
func (stuckProvider) Localized() bool { return false }
func (p stuckProvider) Fetch(context.Context, Request) (Lyrics, error) {
	<-p.release
	return Lyrics{}, ErrNotFound
}

func TestStuckProviderDoesNotHoldTheAnswer(t *testing.T) {
	stuck := stuckProvider{release: make(chan struct{})}
	t.Cleanup(func() { close(stuck.release) })
	line := &fakeProvider{id: config.LyricsLRCLIB, l: timed(TimingLine, "line")}
	h := newHarness(t, stuck, line)
	h.r.timeout = 100 * time.Millisecond
	start := time.Now()
	l, err := h.r.Lyrics(context.Background(), req)
	if err != nil || l.Provider != config.LyricsLRCLIB {
		t.Fatalf("lyrics = %+v, %v", l, err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("took %s", d)
	}
}
