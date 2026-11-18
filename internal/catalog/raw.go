package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// RawKinds are the cache kinds that hold each kind of item, most specific
// page first, for the item inspector (Settings → Diagnostics → Developer tools).
var RawKinds = map[string][]string{
	"song":        {"song-page", songKind},
	"album":       {albumPageKind},
	"playlist":    {"playlist-page"},
	"artist":      {"artist-page"},
	"music-video": {"music-video"},
}

// Raw returns the cached response of catalog item id of kind (a key of
// RawKinds) as the cache stores it, with the cache key it came from; ok is
// false when nothing is cached (it never asks Apple).
func (c *Catalog) Raw(ctx context.Context, kind, id string) (key string, body []byte, ok bool, err error) {
	sf := c.storefront()
	if sf == "" {
		return "", nil, false, ErrNoStorefront
	}
	scope := CacheScope(sf, c.api.ContentLanguage())
	for _, k := range RawKinds[kind] {
		key = scope + "|" + k + "|" + id
		row, err := c.db.RQ.GetCache(ctx, key)
		switch {
		case err == nil:
			return key, row.Body, true, nil
		case !errors.Is(err, sql.ErrNoRows):
			return "", nil, false, fmt.Errorf("catalog: read cache: %w", err)
		}
	}
	return "", nil, false, nil
}
