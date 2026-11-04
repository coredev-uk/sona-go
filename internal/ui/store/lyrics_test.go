package store

import (
	"context"
	"fmt"
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
