//go:build linux || (cgo && (darwin || windows))

package native

import (
	"context"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/coredev-uk/sona-go/internal/playback"
)

// A broadcaster's stream names the song on air, never its cover. When the
// Apple Music catalog has exactly that song, its artwork stands for it in
// the player. The lookup is a catalog search (it reports nothing as a
// play), made only for a song the stream names with an artist and title,
// and it uses the artwork only for a hit whose title and artist both match.

const (
	onAirArtLimit   = 5               // search hits considered
	onAirArtTimeout = 8 * time.Second // one lookup
	onAirArtKeep    = 128             // songs remembered
)

// onAirArts remembers the artwork found (or not found: "") for on-air
// songs, so a song heard again (a reconnect, a repeat) costs no search.
type onAirArts struct {
	mu sync.Mutex
	m  map[string]string
}

func (a *onAirArts) get(key string) (string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	v, ok := a.m[key]
	return v, ok
}

func (a *onAirArts) put(key, art string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.m == nil || len(a.m) >= onAirArtKeep {
		a.m = map[string]string{}
	}
	a.m[key] = art
}

// onAirArt returns the catalog artwork template for the song m names, or
// "" when it names no artist and title, the catalog has no such song, or
// the search fails (the song then just shows no cover).
func (e *Engine) onAirArt(ctx context.Context, m playback.OnAir) string {
	if e.api == nil || e.sf == nil || m.Artist == "" || m.Title == "" {
		return ""
	}
	sf := e.sf()
	if sf == "" {
		return ""
	}
	key := sf + "\x00" + normalise(m.Artist) + "\x00" + normalise(m.Title)
	if art, ok := e.arts.get(key); ok {
		return art
	}
	ctx, cancel := context.WithTimeout(ctx, onAirArtTimeout)
	defer cancel()
	songs, err := e.api.SearchSongs(ctx, sf, m.Artist+" "+m.Title, onAirArtLimit)
	if err != nil {
		if ctx.Err() == nil {
			e.log.Debug("on air artwork", "err", err)
		}
		return ""
	}
	cands := make([]artCandidate, 0, len(songs))
	for _, s := range songs {
		c := artCandidate{title: s.Attributes.Name, artist: s.Attributes.ArtistName}
		if a := s.Attributes.Artwork; a != nil {
			c.art = a.Template()
		}
		cands = append(cands, c)
	}
	art := matchArt(m, cands)
	e.arts.put(key, art)
	return art
}

// artCandidate is one search hit.
type artCandidate struct{ title, artist, art string }

// matchArt returns the artwork of the first candidate that is the song m
// names: the same title (ignoring case, punctuation and a bracketed
// suffix such as "(Radio Edit)") and the same artist, or one of several
// ("A & B", "A feat. B") that the stream or the catalog lists.
func matchArt(m playback.OnAir, cands []artCandidate) string {
	title, artist := normalise(m.Title), normalise(m.Artist)
	if title == "" || artist == "" {
		return ""
	}
	for _, c := range cands {
		if c.art == "" || normalise(c.title) != title {
			continue
		}
		if ca := normalise(c.artist); ca == artist || sharesArtist(ca, artist) {
			return c.art
		}
	}
	return ""
}

// sharesArtist reports whether one artist line contains the other (the
// shorter being at least four letters, so "Eve" does not match "Steve").
func sharesArtist(a, b string) bool {
	if len(a) > len(b) {
		a, b = b, a
	}
	return len(a) >= 4 && strings.Contains(b, a)
}

// normalise reduces a title or artist to lower-case letters and digits,
// dropping bracketed parts: "Song (Radio Edit)" and "SONG" compare equal.
func normalise(s string) string {
	var b strings.Builder
	depth := 0
	for _, r := range s {
		switch {
		case r == '(' || r == '[':
			depth++
		case r == ')' || r == ']':
			depth = max(depth-1, 0)
		case depth == 0 && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}
