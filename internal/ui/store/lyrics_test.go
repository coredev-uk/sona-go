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
		if r = UseLyricsFrom(cl, "1", fetch); r.HasData {
			break
		}
	}
	if !r.HasData || len(r.Data.Lines) != 0 || r.Err != nil {
		t.Fatalf("no lyrics is %+v, want data without lines", r)
	}
	UseLyricsFrom(cl, "1", fetch)
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
	UseLyricsFrom(cl, "1", fetch)
	eventually(t, "the failure", func() bool { return UseLyricsFrom(cl, "1", fetch).Err != nil })
	for range 300 {
		clk.advance(200 * time.Millisecond)
		UseLyricsFrom(cl, "1", fetch)
	}
	if runs.Load() != 1 {
		t.Fatalf("the failed lyrics were asked %d times while on screen", runs.Load())
	}
	cl.Invalidate(KeyLyrics("1"))
	eventually(t, "the retry", func() bool { return runs.Load() == 2 })
}
