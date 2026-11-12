package store

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/services"
)

// fakeLyrics is a LyricsSource answering every song with one fetch.
type fakeLyrics func(ctx context.Context) (services.Lyrics, error)

func (f fakeLyrics) Lyrics(ctx context.Context, _ string) (services.Lyrics, error) { return f(ctx) }

func useLyrics(cl *Client, id string, fetch func(ctx context.Context) (services.Lyrics, error)) Result[services.Lyrics] {
	return UseLyrics(cl, fakeLyrics(fetch), id)
}

// A song Apple has no lyrics for is an answer (no lines) cached as data, not a
// failure asked again.
func TestNoLyricsIsCachedAsAnAnswer(t *testing.T) {
	cl := NewClient(func(fn func()) { fn() })
	asks := 0
	fetch := func(context.Context) (services.Lyrics, error) {
		asks++
		return services.Lyrics{}, fmt.Errorf("lyrics: %w", applemusic.ErrNotFound)
	}
	var r Result[services.Lyrics]
	for end := time.Now().Add(5 * time.Second); time.Now().Before(end); time.Sleep(2 * time.Millisecond) {
		if r = useLyrics(cl, "1", fetch); r.HasData {
			break
		}
	}
	if !r.HasData || len(r.Data.Lines) != 0 || r.Err != nil {
		t.Fatalf("no lyrics is %+v, want data without lines", r)
	}
	useLyrics(cl, "1", fetch)
	if asks != 1 {
		t.Errorf("asked %d times", asks)
	}
}

// A lyrics failure is an answer (v0.2.0's retry: false): a view drawing it for
// a minute never asks again, and Invalidate (Try again) does.
func TestALyricsFailureIsNotAskedAgainWhileShown(t *testing.T) {
	cl, clk := newTestClient()
	var runs atomic.Int32
	fetch := func(context.Context) (services.Lyrics, error) {
		runs.Add(1)
		return services.Lyrics{}, errors.New("down")
	}
	useLyrics(cl, "1", fetch)
	eventually(t, "the failure", func() bool { return useLyrics(cl, "1", fetch).Err != nil })
	for range 300 {
		clk.advance(200 * time.Millisecond)
		useLyrics(cl, "1", fetch)
	}
	if runs.Load() != 1 {
		t.Fatalf("the failed lyrics were asked %d times while on screen", runs.Load())
	}
	cl.Invalidate(KeyLyrics("1"))
	eventually(t, "the retry", func() bool { return runs.Load() == 2 })
}
