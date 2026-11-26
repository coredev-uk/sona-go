package kit

import (
	"context"
	"sync"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// SongArtistsService is what SongArtists asks of CatalogService
// (*services.CatalogService implements it).
type SongArtistsService interface {
	SongArtists(ctx context.Context, ids []string) (map[string][]services.Artist, error)
}

// songArtistsWait is how long lookups gather into one SongArtists call, and
// songArtistsRetry how long a failed lookup is left alone.
const (
	songArtistsWait  = 16 * time.Millisecond
	songArtistsRetry = time.Minute
)

// SongArtists is the artists of the songs a page shows that only know their
// first: a library song (its library artist is one), a queue slot played from
// a stored row. It asks the catalogue copy lazily, as a row or the player is
// built, never during sync: a song whose artist string may name several
// (applemusic.MayNameSeveral) and that has a catalogue ID is looked up, with every
// other one drawn in the same frames, in one batched CatalogService.SongArtists
// call; the answer is kept for the session (the catalogue cache keeps the
// songs a week), so a song is asked about once. Nothing is stored in the
// library. Of, and the builds around it, run on the main thread; the lookup
// runs on a goroutine and hands its answer back through the Poster.
type SongArtists struct {
	api  SongArtistsService
	post store.Poster
	wait time.Duration // the gathering window: tests set it

	mu    sync.Mutex
	known map[string]knownArtists // by catalogue ID
	queue []string
	// asked is the IDs queued or in flight, so a frame asks once.
	asked     map[string]bool
	scheduled bool
}

// knownArtists is what a lookup learnt of a song: its artists (none when
// Apple credits it to none, or the lookup failed), and when a failure may be
// tried again.
type knownArtists struct {
	artists []services.Artist
	retry   time.Time // zero: the answer stands
}

// NewSongArtists is the resolver over CatalogService. post is Env.Post: the
// answer is stored on the main thread and the windows redrawn.
func NewSongArtists(api SongArtistsService, post store.Poster) *SongArtists {
	return &SongArtists{api: api, post: post, wait: songArtistsWait, known: map[string]knownArtists{}, asked: map[string]bool{}}
}

// Of is the artists to offer for a song, a queue slot or a station seed: own
// when it has several; else, for a catalogue ID whose artist string may name
// several, the catalogue copy's once looked up (the lookup starts here, and the
// frame it ends in is a later one: until then the song reads as the one artist
// it knows). That holds for a song that knows one artist (an answer cut to the
// first) or none, even with no first ID. Else own, or its first artist, firstID
// under the string (nil when it has none). A lookup that finds a single artist
// leaves what was known as it was: a library song keeps its library artist's
// page. A nil SongArtists looks nothing up.
func (s *SongArtists) Of(catalogID, artist, firstID string, own []services.Artist) []services.Artist {
	if len(own) > 1 {
		return own
	}
	if s != nil && catalogID != "" && applemusic.MayNameSeveral(artist) {
		if got := s.lookup(catalogID); len(got) > 1 {
			return got
		}
	}
	if len(own) == 1 {
		return own
	}
	if firstID == "" {
		return nil
	}
	return []services.Artist{{ID: firstID, Name: artist}}
}

// OfSong is Of for a song.
func (s *SongArtists) OfSong(so services.Song) []services.Artist {
	return s.Of(so.CatalogID, so.Artist, so.ArtistID, so.Artists)
}

// OfPlaying is Of for the queue's current item.
func (s *SongArtists) OfPlaying(cur *services.QueueItem) []services.Artist {
	if cur == nil {
		return nil
	}
	id := cur.CatalogID
	if cur.Video || cur.Live {
		id = "" // not a catalogue song
	}
	return s.Of(id, cur.Artist, cur.ArtistID, cur.Artists)
}

// lookup is what is known of a catalogue song's artists, queueing the lookup
// when nothing is (or a failure is old enough to try again).
func (s *SongArtists) lookup(id string) []services.Artist {
	s.mu.Lock()
	defer s.mu.Unlock()
	k, ok := s.known[id]
	if ok && (k.retry.IsZero() || time.Now().Before(k.retry)) {
		return k.artists
	}
	if !s.asked[id] {
		s.asked[id] = true
		s.queue = append(s.queue, id)
		if !s.scheduled {
			s.scheduled = true
			if s.wait <= 0 {
				go s.flush()
			} else {
				time.AfterFunc(s.wait, s.flush)
			}
		}
	}
	return k.artists
}

// flush asks for everything queued in one call.
func (s *SongArtists) flush() {
	s.mu.Lock()
	ids := s.queue
	s.queue, s.scheduled = nil, false
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	got, err := s.api.SongArtists(ctx, ids)
	s.post(func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, id := range ids {
			delete(s.asked, id)
			if err != nil {
				s.known[id] = knownArtists{retry: time.Now().Add(songArtistsRetry)}
				continue
			}
			s.known[id] = knownArtists{artists: got[id]}
		}
	})
}
