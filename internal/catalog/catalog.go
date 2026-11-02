// Package catalog is a read-through SQLite cache over the Apple Music
// catalog endpoints.
//
// Each response is stored whole in catalog_cache (keyed by storefront and
// request) with a per-kind TTL, and songs, albums, artists and playlists are
// also upserted as catalog rows (library_id NULL) in the normalised tables so
// other components can resolve catalog IDs locally (e.g.
// dbq.GetTrackByCatalogID). Fresh cached responses are returned without a
// request; stale ones are refetched, and served when Apple cannot be reached.
package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/db/dbq"
)

// Cache lifetimes per response kind.
const (
	SearchTTL   = 30 * time.Minute
	LandingTTL  = 12 * time.Hour
	AlbumTTL    = 24 * time.Hour // the page's sections (similar albums, playlists) change
	VideoTTL    = 24 * time.Hour
	ArtistTTL   = 24 * time.Hour
	PlaylistTTL = time.Hour
	SongTTL     = 7 * 24 * time.Hour
)

// ErrNoStorefront is returned when the storefront func reports none (signed
// out and no default configured).
var ErrNoStorefront = errors.New("catalog: no storefront")

// Catalog serves catalog data through the cache.
type Catalog struct {
	db         *db.DB
	api        *applemusic.Client
	storefront func() string
	log        *slog.Logger
	now        func() time.Time
}

// New returns a Catalog. storefront returns the storefront to query (the
// signed-in account's, e.g. "gb"); it is called per request so a sign-in
// change applies immediately.
func New(d *db.DB, api *applemusic.Client, storefront func() string, log *slog.Logger) *Catalog {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Catalog{db: d, api: api, storefront: storefront, log: log, now: time.Now}
}

// cached implements the read-through policy for one response. fetch calls
// Apple; store writes normalised rows (may be nil).
func cached[T any](ctx context.Context, c *Catalog, kind, id string, ttl time.Duration,
	fetch func(ctx context.Context, sf string) (T, error), store func(q *dbq.Queries, v T, now int64) error,
) (T, error) {
	var zero T
	sf := c.storefront()
	if sf == "" {
		return zero, ErrNoStorefront
	}
	key := sf + "|" + kind + "|" + id
	now := c.now()
	var stale *T
	row, err := c.db.RQ.GetCache(ctx, key)
	switch {
	case err == nil:
		var v T
		if jerr := json.Unmarshal(row.Body, &v); jerr != nil {
			c.log.Warn("catalog cache: undecodable entry", "key", key, "err", jerr)
			break
		}
		if now.UnixMilli() < row.ExpiresAt {
			return v, nil
		}
		stale = &v
	case !errors.Is(err, sql.ErrNoRows):
		return zero, fmt.Errorf("catalog: read cache: %w", err)
	}

	v, err := fetch(ctx, sf)
	if err != nil {
		if errors.Is(err, applemusic.ErrNotFound) {
			if derr := c.db.Q.DeleteCache(ctx, key); derr != nil {
				c.log.Warn("catalog cache: delete", "key", key, "err", derr)
			}
			return zero, err
		}
		if stale != nil && ctx.Err() == nil && !errors.Is(err, applemusic.ErrUnauthorized) {
			c.log.Warn("catalog: serving stale entry", "key", key, "err", err)
			return *stale, nil
		}
		return zero, err
	}
	body, err := json.Marshal(v)
	if err != nil {
		return zero, fmt.Errorf("catalog: encode %s: %w", kind, err)
	}
	ms := db.Millis(now)
	err = c.db.Tx(ctx, func(q *dbq.Queries) error {
		if store != nil {
			if err := store(q, v, ms); err != nil {
				return err
			}
		}
		return q.PutCache(ctx, dbq.PutCacheParams{Key: key, Storefront: sf, Body: body, FetchedAt: ms, ExpiresAt: ms + ttl.Milliseconds()})
	})
	if err != nil {
		// The data is good; a cache write failure must not hide it.
		c.log.Warn("catalog cache: write", "key", key, "err", err)
	}
	return v, nil
}

// Search searches the catalog (every result group, or one group's page).
// Results are cached per term and page.
func (c *Catalog) Search(ctx context.Context, term string, opt applemusic.SearchOptions) (applemusic.SearchResults, error) {
	term = strings.Join(strings.Fields(strings.ToLower(term)), " ")
	if term == "" {
		return applemusic.SearchResults{}, nil
	}
	id := term + "|" + opt.Group + "|" + strconv.Itoa(opt.Limit) + "|" + strconv.Itoa(opt.Offset)
	return cached(ctx, c, "search", id, SearchTTL, func(ctx context.Context, sf string) (applemusic.SearchResults, error) {
		return applemusic.CatalogSearch(ctx, c.api, sf, term, opt)
	}, nil)
}

// SearchLanding returns the search landing page (browse categories).
func (c *Catalog) SearchLanding(ctx context.Context) (applemusic.MapResponse, error) {
	return cached(ctx, c, "search-landing", "", LandingTTL, func(ctx context.Context, sf string) (applemusic.MapResponse, error) {
		return applemusic.SearchLanding(ctx, c.api, sf)
	}, nil)
}

// Album returns catalog album id's page: every track (with popularity),
// its artists and the page sections (applemusic.AlbumPage).
func (c *Catalog) Album(ctx context.Context, id string) (applemusic.Album, error) {
	return cached(ctx, c, "album-page", id, AlbumTTL, func(ctx context.Context, sf string) (applemusic.Album, error) {
		return applemusic.AlbumPage(ctx, c.api, sf, id)
	}, func(q *dbq.Queries, a applemusic.Album, now int64) error {
		return storeAlbum(ctx, q, a, now)
	})
}

// Artist returns catalog artist id's page: biography and the page
// sections (applemusic.ArtistPage).
func (c *Catalog) Artist(ctx context.Context, id string) (applemusic.Artist, error) {
	return cached(ctx, c, "artist-page", id, ArtistTTL, func(ctx context.Context, sf string) (applemusic.Artist, error) {
		return applemusic.ArtistPage(ctx, c.api, sf, id)
	}, func(q *dbq.Queries, a applemusic.Artist, now int64) error {
		if _, err := storeArtist(ctx, q, a, now); err != nil {
			return err
		}
		for _, s := range a.Views.TopSongs.Data {
			if _, err := storeSong(ctx, q, s, now); err != nil {
				return err
			}
		}
		return nil
	})
}

// Playlist returns catalog playlist id's page: every track, the curator,
// featured artists and more playlists by the curator.
func (c *Catalog) Playlist(ctx context.Context, id string) (applemusic.CatalogPlaylist, error) {
	return cached(ctx, c, "playlist-page", id, PlaylistTTL, func(ctx context.Context, sf string) (applemusic.CatalogPlaylist, error) {
		return c.api.CatalogPlaylist(ctx, sf, id)
	}, func(q *dbq.Queries, p applemusic.CatalogPlaylist, now int64) error {
		return storePlaylist(ctx, q, p.Playlist, now)
	})
}

// Song returns catalog song id.
func (c *Catalog) Song(ctx context.Context, id string) (applemusic.Song, error) {
	return cached(ctx, c, "song", id, SongTTL, func(ctx context.Context, sf string) (applemusic.Song, error) {
		return c.api.Song(ctx, sf, id)
	}, func(q *dbq.Queries, s applemusic.Song, now int64) error {
		_, err := storeSong(ctx, q, s, now)
		return err
	})
}

// Prune deletes expired cache entries and returns how many it removed.
func (c *Catalog) Prune(ctx context.Context) (int64, error) {
	return c.db.Q.DeleteExpiredCache(ctx, db.Millis(c.now()))
}

// CacheSize reports the cached responses and their bytes.
func (c *Catalog) CacheSize(ctx context.Context) (entries, bytes int64, err error) {
	r, err := c.db.RQ.CatalogCacheSize(ctx)
	return r.Entries, r.Bytes, err
}

// ClearCache deletes every cached response and returns how many it
// removed. The normalised catalog rows stay (library views join them).
func (c *Catalog) ClearCache(ctx context.Context) (int64, error) {
	return c.db.Q.ClearCatalogCache(ctx)
}

func storeAlbum(ctx context.Context, q *dbq.Queries, a applemusic.Album, now int64) error {
	if _, err := storeAlbumRow(ctx, q, a, now); err != nil {
		return err
	}
	for _, r := range a.Relationships.Artists.Data {
		if _, err := storeArtist(ctx, q, r, now); err != nil {
			return err
		}
	}
	for _, s := range a.Relationships.Tracks.Data {
		if s.Type != applemusic.TypeSongs {
			continue // music videos
		}
		if len(s.Relationships.Albums.Data) == 0 {
			s.Relationships.Albums.Data = []applemusic.Album{a}
		}
		if _, err := storeSong(ctx, q, s, now); err != nil {
			return err
		}
	}
	return nil
}

func storeAlbumRow(ctx context.Context, q *dbq.Queries, a applemusic.Album, now int64) (int64, error) {
	at := a.Attributes
	var artist string
	if len(a.Relationships.Artists.Data) > 0 {
		artist = a.Relationships.Artists.Data[0].ID
	}
	return q.UpsertCatalogAlbum(ctx, dbq.UpsertCatalogAlbumParams{
		CatalogID: db.NullString(a.ID), Name: at.Name, ArtistName: at.ArtistName,
		ArtistCatalogID: db.NullString(artist), ArtworkTemplate: at.Artwork.Template(),
		ReleaseDate: at.ReleaseDate, Genre: first(at.GenreNames), TrackCount: int64(at.TrackCount),
		ContentRating: at.ContentRating, IsSingle: db.Bool(at.IsSingle), IsCompilation: db.Bool(at.IsCompilation),
		RecordLabel: at.RecordLabel, Copyright: at.Copyright, EditorialNotes: at.EditorialNotes.Text(),
		Url: at.URL, UpdatedAt: now,
	})
}

func storeArtist(ctx context.Context, q *dbq.Queries, a applemusic.Artist, now int64) (int64, error) {
	if a.Attributes.Name == "" {
		return 0, nil // relationship stub without attributes
	}
	return q.UpsertCatalogArtist(ctx, dbq.UpsertCatalogArtistParams{
		CatalogID: db.NullString(a.ID), Name: a.Attributes.Name,
		ArtworkTemplate: a.Attributes.Artwork.Template(), Url: a.Attributes.URL, UpdatedAt: now,
	})
}

func storeSong(ctx context.Context, q *dbq.Queries, s applemusic.Song, now int64) (int64, error) {
	a := s.Attributes
	var album, artist string
	if len(s.Relationships.Albums.Data) > 0 {
		album = s.Relationships.Albums.Data[0].ID
	}
	if len(s.Relationships.Artists.Data) > 0 {
		artist = s.Relationships.Artists.Data[0].ID
	}
	return q.UpsertCatalogTrack(ctx, dbq.UpsertCatalogTrackParams{
		CatalogID: db.NullString(s.ID), Name: a.Name, ArtistName: a.ArtistName, AlbumName: a.AlbumName,
		AlbumCatalogID: db.NullString(album), ArtistCatalogID: db.NullString(artist),
		ArtworkTemplate: a.Artwork.Template(), DurationMs: a.DurationInMillis,
		TrackNumber: int64(a.TrackNumber), DiscNumber: int64(a.DiscNumber), ReleaseDate: a.ReleaseDate,
		Genre: first(a.GenreNames), ContentRating: a.ContentRating, HasLyrics: db.Bool(a.HasLyrics),
		Isrc: a.ISRC, Composer: a.ComposerName, UpdatedAt: now,
	})
}

func storePlaylist(ctx context.Context, q *dbq.Queries, p applemusic.Playlist, now int64) error {
	at := p.Attributes
	tracks := p.Relationships.Tracks.Data
	id, err := q.UpsertCatalogPlaylist(ctx, dbq.UpsertCatalogPlaylistParams{
		CatalogID: db.NullString(p.ID), Name: at.Name, Description: at.Description.Text(),
		CuratorName: at.CuratorName, ArtworkTemplate: at.Artwork.Template(),
		LastModified: at.LastModifiedDate, TrackCount: int64(len(tracks)), Url: at.URL, UpdatedAt: now,
	})
	if err != nil {
		return err
	}
	if err := q.DeletePlaylistTracks(ctx, id); err != nil {
		return err
	}
	pos := int64(0)
	for _, s := range tracks {
		if s.Type != applemusic.TypeSongs {
			continue
		}
		tid, err := storeSong(ctx, q, s, now)
		if err != nil {
			return err
		}
		if err := q.InsertPlaylistTrack(ctx, dbq.InsertPlaylistTrackParams{PlaylistID: id, Position: pos, TrackID: tid}); err != nil {
			return err
		}
		pos++
	}
	return nil
}

func first(s []string) string {
	if len(s) == 0 {
		return ""
	}
	return s[0]
}
