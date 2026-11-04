package player

import (
	"context"
	"time"
	"unicode"

	"github.com/coredev-uk/sona-go/internal/lyrics"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// lyricsSource is what the view asks of the lyrics service.
type lyricsSource interface {
	Lyrics(ctx context.Context, songID string) (services.Lyrics, error)
}

// lyricsResult is a song's lyrics as the query holds them: a song with none
// (not_found) is an answer, not a failure, so it is asked once, not again
// every few seconds.
type lyricsResult struct {
	lyrics *services.Lyrics // nil when the song has none
}

// lyricsSvc is the service the lyrics come from (a test's own, else the core's).
func (p *player) lyricsSvc() lyricsSource {
	if p.np.lyrics != nil {
		return p.np.lyrics
	}
	return services.Service[services.LyricsService](p.env.Core)
}

// lyricsKey is v0.2.0's keys.lyrics.
func lyricsKey(songID string) store.Key { return store.Key{"catalog", "lyrics", songID} }

// lyricsFor is the lyrics query of a catalogue song: never stale (a lyrics
// provider setting change invalidates ["catalog","lyrics"]), no retry.
func (p *player) lyricsFor(songID string) store.Result[lyricsResult] {
	svc := p.lyricsSvc()
	return store.Use(p.env.Queries, lyricsKey(songID), func(ctx context.Context) (lyricsResult, error) {
		l, err := svc.Lyrics(ctx, songID)
		if err != nil {
			if services.CodeOf(err) == services.CodeNotFound {
				return lyricsResult{}, nil
			}
			return lyricsResult{}, err
		}
		return lyricsResult{lyrics: &l}, nil
	}, store.Options{Stale: store.Forever})
}

// retryLyrics asks again after a failure.
func (p *player) retryLyrics(songID string) { p.env.Queries.Invalidate(lyricsKey(songID)) }

// motionSource is what the view asks of the motion service.
type motionSource interface {
	Album(ctx context.Context, id string) (string, error)
}

// albumMotion is the motion cover of the playing song's album ("" when it
// has none), fetched only while motion may play: cached an hour, no retry.
func (p *player) albumMotion(albumID string) string {
	if albumID == "" || !p.env.Motion.Allowed() {
		return ""
	}
	svc := p.np.motion
	if svc == nil {
		svc = services.Service[services.MotionService](p.env.Core)
	}
	r := store.Use(p.env.Queries, store.Key{"catalog", "album-motion", albumID}, func(ctx context.Context) (string, error) {
		return svc.Album(ctx, albumID)
	}, store.Options{Stale: time.Hour})
	return r.Data
}

// sweepWords are the words as the sweep text takes them. whole gives each a
// zero length, so it lights whole as it begins (reduced motion).
func sweepWords(ws []services.LyricWord, whole bool) []lyrics.Word {
	out := make([]lyrics.Word, len(ws))
	for i, w := range ws {
		out[i] = lyrics.Word{
			Begin: msDur(w.BeginMs), End: msDur(w.EndMs), Text: w.Text, Background: w.Background,
		}
		if whole {
			out[i].End = out[i].Begin
		}
	}
	return out
}

// rtl reports text that reads right to left: its first strong letter is.
func rtl(s string) bool {
	for _, r := range s {
		switch {
		case unicode.Is(unicode.Hebrew, r) || unicode.Is(unicode.Arabic, r) || unicode.Is(unicode.Syriac, r) || unicode.Is(unicode.Thaana, r):
			return true
		case unicode.IsLetter(r):
			return false
		}
	}
	return false
}

func msDur(ms int64) time.Duration { return time.Duration(ms) * time.Millisecond }
