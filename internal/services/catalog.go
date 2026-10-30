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

// newCatalogService also prunes expired cache entries once the network
// opens.
func newCatalogService(e *Env) *CatalogService {
	e.OnNetwork(func() {
		go func() {
			if n, err := e.Catalog.Prune(context.Background()); err != nil {
				e.Logger.Warn("catalog prune", "err", err)
			} else if n > 0 {
				e.Logger.Info("catalog cache pruned", "entries", n)
			}
		}()
	})
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
