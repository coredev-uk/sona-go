package library

import (
	"context"
	"math/rand/v2"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// run is shared/lib/run.ts: it runs a fire-and-forget command (playback,
// library edits) on a goroutine. A failure becomes a toast naming what
// failed and why; nothing is retried. done, when given, is told on the main
// thread whether it succeeded.
func (l *library) run(what string, fn func(ctx context.Context) error, done func(ok bool)) {
	go func() {
		err := fn(context.Background())
		if err != nil {
			kit.ToastFailure(what, err)
		}
		if done != nil {
			l.env.Post(func() { done(err == nil) })
		}
	}()
}

// loadedSongs are the rows of a paged list as songs for a play: a row that
// has not loaded is a zero Song, which is never playable (its place in the
// queue is kept for start).
func loadedSongs(rows []*services.Song) []services.Song {
	out := make([]services.Song, len(rows))
	for i, s := range rows {
		if s != nil {
			out[i] = *s
		}
	}
	return out
}

// randomPlayable is the index of a random playable song among those loaded
// (songs not loaded yet are nil), 0 when none is.
func randomPlayable(songs []*services.Song) int {
	var loaded []int
	for i, s := range songs {
		if s != nil && s.Playable {
			loaded = append(loaded, i)
		}
	}
	if len(loaded) == 0 {
		return 0
	}
	return loaded[rand.IntN(len(loaded))]
}
