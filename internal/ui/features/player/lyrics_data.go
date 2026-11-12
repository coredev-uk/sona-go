package player

import (
	"context"
	"time"
	"unicode"

	"github.com/coredev-uk/sona-go/internal/lyrics"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// lyricsFor is the lyrics query of a catalogue song: the one cache the song
// page shares (store.KeyLyrics, always a services.Lyrics), never stale. A song
// none of the sources has (not_found) is an answer the store caches as empty
// lyrics, not a failure asked again every few seconds.
func (p *player) lyricsFor(songID string) store.Result[services.Lyrics] {
	return store.UseLyrics(p.env.Queries, store.LyricsOf(p.np.lyrics, p.env.Core), songID)
}

// lyricsMissing reports a song with no lyrics: not found, or an answer with no
// lines.
func lyricsMissing(r store.Result[services.Lyrics]) bool {
	if r.HasData {
		return len(r.Data.Lines) == 0
	}
	return r.Err != nil && services.CodeOf(r.Err) == services.CodeNotFound
}

// retryLyrics asks again after a failure.
func (p *player) retryLyrics(songID string) { p.env.Queries.Invalidate(store.KeyLyrics(songID)) }

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
