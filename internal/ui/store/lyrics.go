package store

import (
	"context"

	"github.com/coredev-uk/sona-go/internal/services"
)

// KeyLyrics is the one cache of a song's full lyrics, shared by the song page
// and the now-playing view (v0.2.0's keys.lyrics): it always holds the
// services.Lyrics DTO; a view that needs more derives it from there.
func KeyLyrics(id string) Key { return Key{"catalog", "lyrics", id} }

// UseLyrics is LyricsService.Lyrics for a catalogue song ID under KeyLyrics.
func UseLyrics(cl *Client, core *services.Core, id string) Result[services.Lyrics] {
	return UseLyricsFrom(cl, id, func(ctx context.Context) (services.Lyrics, error) {
		if core == nil {
			return services.Lyrics{}, errNoCore
		}
		return services.Service[services.LyricsService](core).Lyrics(ctx, id)
	})
}

// UseLyricsFrom is UseLyrics over another fetch (a fake in tests).
func UseLyricsFrom(cl *Client, id string, fetch func(ctx context.Context) (services.Lyrics, error)) Result[services.Lyrics] {
	// A song with no lyrics is an answer (empty lines), cached like any other,
	// not a failure asked again every few seconds.
	return Use(cl, KeyLyrics(id), func(ctx context.Context) (services.Lyrics, error) {
		l, err := fetch(ctx)
		if services.CodeOf(err) == services.CodeNotFound {
			return services.Lyrics{}, nil
		}
		return l, err
	}, Options{Stale: Forever, NoRetry: true}) // TanStack's retry: false: a failure is an answer, asked again on Try again
}
