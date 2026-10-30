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
