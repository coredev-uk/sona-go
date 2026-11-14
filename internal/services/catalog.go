package services

import (
	"context"

	"github.com/coredev-uk/sona-go/internal/catalog"
)

// CatalogService serves the Apple Music catalog through the SQLite
// read-through cache.
type CatalogService struct {
	e *Env
	c *catalog.Catalog
	m mapper
}

func newCatalogService(e *Env) *CatalogService {
	return &CatalogService{e: e, c: e.Catalog, m: e.m}
}

func (s *CatalogService) fail(err error) error { return s.e.Fail(err) }

// Song returns a catalog song.
func (s *CatalogService) Song(ctx context.Context, id string) (Song, error) {
	so, err := s.c.Song(ctx, id)
	if err != nil {
		return Song{}, s.fail(err)
	}
	return s.m.amSong(so), nil
}

// SongArtists returns the artists of catalogue songs by catalogue ID, each
// as Song.Artists: what a library song, a queue slot or a row that kept only
// its first artist needs to offer all of them. The songs come from
// catalog.Songs, so the lookup is one request per hundred IDs not already in
// the cache (which keeps them a week), and never one per song. A song Apple
// doesn't serve is absent, one it credits to no artist maps to none; a failed
// lookup fails them all. Without a catalogue (no Apple account) nothing is
// known. Nothing is stored in the library: the catalogue cache is the copy.
func (s *CatalogService) SongArtists(ctx context.Context, ids []string) (map[string][]Artist, error) {
	if s.c == nil {
		return nil, nil
	}
	songs, err := s.c.Songs(ctx, ids)
	if err != nil {
		return nil, s.fail(err)
	}
	out := make(map[string][]Artist, len(songs))
	for id, so := range songs {
		out[id] = s.m.amArtists(so.Relationships.Artists.Data)
	}
	return out, nil
}
