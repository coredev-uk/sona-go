package lyrics

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/coredev-uk/sona-go/internal/config"
)

// LRCLIBBase is LRCLIB's API (https://lrclib.net/docs).
const LRCLIBBase = "https://lrclib.net/api/"

// LRCLIB is the open lyrics database at lrclib.net: line-timed lyrics (word
// times where an entry has enhanced LRC), or plain text.
type LRCLIB struct {
	HTTP *http.Client
	Base string // default LRCLIBBase
	// UserAgent names Sona, as LRCLIB asks of its clients.
	UserAgent string
}

func (p *LRCLIB) ID() string      { return config.LyricsLRCLIB }
func (p *LRCLIB) Localized() bool { return false }

// lrclibRecord is one LRCLIB entry.
type lrclibRecord struct {
	ID           int64   `json:"id"`
	TrackName    string  `json:"trackName"`
	ArtistName   string  `json:"artistName"`
	AlbumName    string  `json:"albumName"`
	Duration     float64 `json:"duration"` // seconds
	Instrumental bool    `json:"instrumental"`
	PlainLyrics  string  `json:"plainLyrics"`
	SyncedLyrics string  `json:"syncedLyrics"`
}

func (r lrclibRecord) candidate() candidate {
	return candidate{Title: r.TrackName, Artist: r.ArtistName, Album: r.AlbumName, Duration: time.Duration(r.Duration * float64(time.Second))}
}

// Fetch asks for the exact signature first (LRCLIB matches the length
// within 2 s), then searches by title and artist.
func (p *LRCLIB) Fetch(ctx context.Context, req Request) (Lyrics, error) {
	if req.Title == "" || req.Artist == "" {
		return Lyrics{}, ErrNotFound
	}
	base := p.Base
	if base == "" {
		base = LRCLIBBase
	}
	h := http.Header{"User-Agent": {p.UserAgent}}
	var rec lrclibRecord
	q := url.Values{"track_name": {req.Title}, "artist_name": {req.Artist}}
	if req.Album != "" {
		q.Set("album_name", req.Album)
	}
	err := ErrNotFound
	if req.Duration > 0 {
		q.Set("duration", formatSeconds(req.Duration))
		err = getJSON(ctx, p.HTTP, p.ID(), base+"get?"+q.Encode(), h, &rec)
	}
	if err == nil && score(req.Track, rec.candidate()) < 0 {
		err = ErrNotFound
	}
	if errors.Is(err, ErrNotFound) {
		var found []lrclibRecord
		sq := url.Values{"track_name": {req.Title}, "artist_name": {req.Artist}}
		if err = getJSON(ctx, p.HTTP, p.ID(), base+"search?"+sq.Encode(), h, &found); err != nil {
			return Lyrics{}, err
		}
		cs := make([]candidate, len(found))
		for i, r := range found {
			cs[i] = r.candidate()
		}
		// Prefer synced entries among the matches.
		i := bestWith(req.Track, cs, func(i int) bool { return found[i].SyncedLyrics != "" })
		if i < 0 {
			return Lyrics{}, ErrNotFound
		}
		rec = found[i]
	} else if err != nil {
		return Lyrics{}, err
	}
	return lrclibLyrics(rec, req.Duration)
}

func lrclibLyrics(rec lrclibRecord, songEnd time.Duration) (Lyrics, error) {
	if rec.Instrumental {
		return Lyrics{}, ErrNotFound
	}
	out := Lyrics{Provider: config.LyricsLRCLIB}
	if rec.SyncedLyrics != "" {
		out.Lines, out.Timing = timedLines(parseLRC(rec.SyncedLyrics), songEnd)
	}
	if len(out.Lines) == 0 {
		out.Lines, out.Timing = plainLines(rec.PlainLyrics), TimingNone
	}
	if out.empty() {
		return Lyrics{}, ErrNotFound
	}
	return out, nil
}

// bestWith is best, preferring candidates for which preferred is true.
func bestWith(t Track, cs []candidate, preferred func(int) bool) int {
	idx, top := -1, -1
	for i, c := range cs {
		s := score(t, c)
		if s < 0 {
			continue
		}
		if preferred(i) {
			s += 1000
		}
		if s > top {
			idx, top = i, s
		}
	}
	return idx
}

// formatSeconds is d in whole seconds, rounded.
func formatSeconds(d time.Duration) string {
	return strconv.FormatInt(int64(math.Round(d.Seconds())), 10)
}
