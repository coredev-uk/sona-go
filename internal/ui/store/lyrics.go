package store

import (
	"context"

	"github.com/coredev-uk/sona-go/internal/services"
)

// KeyLyrics is the one cache of a song's full lyrics, shared by the song page
// and the now-playing view (v0.2.0's keys.lyrics): it always holds the
// services.Lyrics DTO; a view that needs more derives it from there.
func KeyLyrics(id string) Key { return Key{"catalog", "lyrics", id} }

// LyricsSource is what a view asks of the lyrics service
// (services.LyricsService, or a test's fake).
type LyricsSource interface {
	Lyrics(ctx context.Context, songID string) (services.Lyrics, error)
}

// LyricsOf is the source a view reads lyrics from: override (a test's own)
// when there is one, else the core's lyrics service.
func LyricsOf(override LyricsSource, core *services.Core) LyricsSource {
	if override != nil {
		return override
	}
	return services.Service[services.LyricsService](core)
}

// UseLyrics is src's Lyrics for a catalogue song ID under KeyLyrics.
func UseLyrics(cl *Client, src LyricsSource, id string) Result[services.Lyrics] {
	// A song with no lyrics is an answer (empty lines), cached like any other,
	// not a failure asked again every few seconds.
	return Use(cl, KeyLyrics(id), func(ctx context.Context) (services.Lyrics, error) {
		l, err := src.Lyrics(ctx, id)
		if services.CodeOf(err) == services.CodeNotFound {
			return services.Lyrics{}, nil
		}
		return l, err
	}, Options{Stale: Forever, NoRetry: true}) // TanStack's retry: false: a failure is an answer, asked again on Try again
}
