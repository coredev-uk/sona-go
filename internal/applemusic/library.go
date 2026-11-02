package applemusic

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/sync/errgroup"
)

// Library collection paths.
const (
	LibrarySongsPath     = "/v1/me/library/songs"
	LibraryAlbumsPath    = "/v1/me/library/albums"
	LibraryArtistsPath   = "/v1/me/library/artists"
	LibraryPlaylistsPath = "/v1/me/library/playlists"
)

// Queries used for the library collections: they carry exactly the
// relationships the sync stores. relate= adds related IDs only, which also
// drops the large default tracks relationship from albums.
var (
	// Songs newest-added first (library songs have no dateAdded attribute,
	// so the position is the only "date added" order Apple exposes).
	librarySongsQuery = url.Values{"relate": {"albums,artists"}, "sort": {"-dateAdded"}}
	// Albums newest-added first, like songs, so an incremental sync can
	// stop at the first album it already has.
	libraryAlbumsQuery    = url.Values{"relate": {"catalog,artists"}, "sort": {"-dateAdded"}}
	libraryArtistsQuery   = url.Values{"include": {"catalog"}, "fields[artists]": {"name,artwork,url"}}
	libraryPlaylistsQuery = url.Values{"relate": {"catalog"}}
)

// LibrarySongs walks the user's library songs, newest added first, with
// their library album and artist IDs (see FetchPages).
func (c *Client) LibrarySongs(ctx context.Context, opt PageOptions, fn func(offset int, songs []Song) error) (int, error) {
	return FetchPages(ctx, c, LibrarySongsPath, librarySongsQuery, opt, fn)
}

// LibrarySongsPage fetches one page (limit capped at 100) of the
// LibrarySongs walk; Meta.Total is the number of library songs.
func (c *Client) LibrarySongsPage(ctx context.Context, limit, offset int) (Page[Song], error) {
	return libraryPage[Song](ctx, c, LibrarySongsPath, librarySongsQuery, limit, offset)
}

// LibraryAlbums walks the user's library albums, newest added first, with
// their catalog and library artist IDs.
func (c *Client) LibraryAlbums(ctx context.Context, opt PageOptions, fn func(offset int, albums []Album) error) (int, error) {
	return FetchPages(ctx, c, LibraryAlbumsPath, libraryAlbumsQuery, opt, fn)
}

// LibraryAlbumsPage fetches one page (limit capped at 100) of the
// LibraryAlbums walk; Meta.Total is the number of library albums.
func (c *Client) LibraryAlbumsPage(ctx context.Context, limit, offset int) (Page[Album], error) {
	return libraryPage[Album](ctx, c, LibraryAlbumsPath, libraryAlbumsQuery, limit, offset)
}

// LibraryArtists walks the user's library artists with their catalog artist
// (name, artwork, url) included.
func (c *Client) LibraryArtists(ctx context.Context, opt PageOptions, fn func(offset int, artists []Artist) error) (int, error) {
	return FetchPages(ctx, c, LibraryArtistsPath, libraryArtistsQuery, opt, fn)
}

// LibraryArtistsPage fetches one page (limit capped at 100) of the
// LibraryArtists walk; Meta.Total is the number of library artists.
func (c *Client) LibraryArtistsPage(ctx context.Context, limit, offset int) (Page[Artist], error) {
	return libraryPage[Artist](ctx, c, LibraryArtistsPath, libraryArtistsQuery, limit, offset)
}

func libraryPage[T any](ctx context.Context, c *Client, path string, q url.Values, limit, offset int) (Page[T], error) {
	var p Page[T]
	err := c.Get(ctx, path, withPage(q, max(1, min(limit, defaultPageSize)), offset), &p)
	return p, err
}

// LibraryPlaylists walks the user's library playlists.
func (c *Client) LibraryPlaylists(ctx context.Context, opt PageOptions, fn func(offset int, playlists []Playlist) error) (int, error) {
	return FetchPages(ctx, c, LibraryPlaylistsPath, libraryPlaylistsQuery, opt, fn)
}

// LibraryPlaylistsPage fetches one page (limit capped at 100) of the
// LibraryPlaylists walk; Meta.Total is the number of library playlists.
func (c *Client) LibraryPlaylistsPage(ctx context.Context, limit, offset int) (Page[Playlist], error) {
	return libraryPage[Playlist](ctx, c, LibraryPlaylistsPath, libraryPlaylistsQuery, limit, offset)
}

// LibraryPlaylistTracks returns every track of library playlist id in
// order. Apple answers 404 for an empty playlist; that returns no tracks.
func (c *Client) LibraryPlaylistTracks(ctx context.Context, id string, opt PageOptions) ([]Song, error) {
	var out []Song
	_, err := FetchPages(ctx, c, LibraryPlaylistsPath+"/"+url.PathEscape(id)+"/tracks", nil, opt, func(_ int, items []Song) error {
		out = append(out, items...)
		return nil
	})
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	return out, err
}

// MadeForYou returns the library copies of the user's personal mixes
// (Apple's Made for You: Favourites Mix, New Music, Heavy Rotation, …),
// in Apple's order.
func (c *Client) MadeForYou(ctx context.Context) ([]Playlist, error) {
	body, err := AMP[Page[Playlist]](ctx, c, LibraryPlaylistsPath, AMPQuery{
		Limit: 100,
		Extra: url.Values{"filter[featured]": {"made-for-you"}, "sort": {"type"}},
	})
	return body.Data, err
}

// LibraryMusicVideos returns a page of the user's library music videos
// (Apple caps limit at 100) and, in Meta.Total, how many there are.
func (c *Client) LibraryMusicVideos(ctx context.Context, limit, offset int) (Page[MusicVideo], error) {
	return AMP[Page[MusicVideo]](ctx, c, "/v1/me/library/music-videos", AMPQuery{
		Limit:  max(1, min(limit, 100)),
		Offset: offset,
		Extra:  url.Values{"include[library-music-videos]": {"catalog"}},
	})
}

// LibraryMusicVideo returns the user's library music video id with its
// catalog copy (MusicVideo.CatalogID).
func (c *Client) LibraryMusicVideo(ctx context.Context, id string) (MusicVideo, error) {
	body, err := AMP[Page[MusicVideo]](ctx, c, "/v1/me/library/music-videos/"+url.PathEscape(id), AMPQuery{
		Extra: url.Values{"include": {"catalog"}},
	})
	if err != nil {
		return MusicVideo{}, err
	}
	return first(body.Data, TypeLibraryMusicVideos+" "+id)
}

// RecentlyPlayed returns a page of the user's recently played tracks
// (catalog or library songs; Apple caps limit at 30).
func (c *Client) RecentlyPlayed(ctx context.Context, limit, offset int) (Relationship[Song], error) {
	var body Relationship[Song]
	q := url.Values{"limit": {strconv.Itoa(max(1, min(limit, 30)))}}
	if offset > 0 {
		q.Set("offset", strconv.Itoa(offset))
	}
	err := c.Get(ctx, "/v1/me/recent/played/tracks", q, &body)
	return body, err
}

// Rating values: the web player's Favourite and Suggest Less.
const (
	RatingFavorite    = 1
	RatingSuggestLess = -1
)

const ratingBatch = 100

// Ratings returns the ratings (RatingFavorite or RatingSuggestLess) of the
// resources of type typ (e.g. TypeSongs, TypeArtists, TypeLibrarySongs)
// with the given IDs, keyed by ID. Catalog and library copies share one
// rating. Unrated IDs are absent. IDs are queried in batches of 100,
// four batches at a time.
func (c *Client) Ratings(ctx context.Context, typ string, ids []string) (map[string]int, error) {
	want := make(map[string]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	out := make(map[string]int)
	results := make([][]Resource[struct {
		Value int `json:"value"`
	}], (len(ids)+ratingBatch-1)/ratingBatch)
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(defaultParallel)
	for i := range results {
		batch := ids[i*ratingBatch : min(len(ids), (i+1)*ratingBatch)]
		g.Go(func() error {
			var body Page[Resource[struct {
				Value int `json:"value"`
			}]]
			err := c.Get(gctx, "/v1/me/ratings/"+url.PathEscape(typ), url.Values{"ids": {strings.Join(batch, ",")}}, &body)
			if errors.Is(err, ErrNotFound) {
				return nil
			}
			results[i] = body.Data
			return err
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	for _, batch := range results {
		for _, r := range batch {
			id := r.ID
			// Rating resources have been observed with an "r." prefix on
			// the rated resource's ID.
			if !want[id] && want[strings.TrimPrefix(id, "r.")] {
				id = strings.TrimPrefix(id, "r.")
			}
			if r.Attributes.Value != 0 {
				out[id] = r.Attributes.Value
			}
		}
	}
	return out, nil
}

// SetRating rates resource id of type typ with RatingFavorite or
// RatingSuggestLess. Prefer Favorite for favouriting catalog items: it is
// what the web player sends and also adds songs to the library.
func (c *Client) SetRating(ctx context.Context, typ, id string, value int) error {
	if value != RatingFavorite && value != RatingSuggestLess {
		return fmt.Errorf("apple music: invalid rating %d", value)
	}
	body := map[string]any{"type": "rating", "attributes": map[string]int{"value": value}}
	return c.Do(ctx, Request{Method: http.MethodPut, URL: "/v1/me/ratings/" + url.PathEscape(typ) + "/" + url.PathEscape(id), Body: body}, nil)
}

// DeleteRating clears the rating of resource id of type typ. Clearing an
// unrated resource succeeds.
func (c *Client) DeleteRating(ctx context.Context, typ, id string) error {
	err := c.Do(ctx, Request{Method: http.MethodDelete, URL: "/v1/me/ratings/" + url.PathEscape(typ) + "/" + url.PathEscape(id)}, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

// AddToLibrary adds catalog resources to the user's library, keyed by type
// (TypeSongs, TypeAlbums, TypePlaylists, TypeMusicVideos), and returns the
// library ID Apple gave each one (keyed by catalog type and ID; absent
// when Apple reports none).
func (c *Client) AddToLibrary(ctx context.Context, ids map[string][]string) (map[ItemKey]string, error) {
	q := url.Values{"representation": {"ids"}}
	for typ, list := range ids {
		if len(list) > 0 {
			q.Set("ids["+typ+"]", strings.Join(list, ","))
		}
	}
	out := map[ItemKey]string{}
	if len(q) == 1 {
		return out, nil
	}
	var body struct {
		Meta struct {
			// operation.<type>.<catalog ID> = [library resource]
			Operation map[string]map[string][]struct {
				ID string `json:"id"`
			} `json:"operation"`
		} `json:"meta"`
	}
	if err := c.Do(ctx, Request{Method: http.MethodPost, URL: c.ampBase() + "/v1/me/library", Query: q}, &body); err != nil {
		return nil, err
	}
	for typ, byID := range body.Meta.Operation {
		for id, lib := range byID {
			if len(lib) > 0 {
				out[ItemKey{typ, id}] = lib[0].ID
			}
		}
	}
	return out, nil
}

// LibraryCatalogID maps library resource id of type typ (TypeSongs,
// TypeAlbums, TypeArtists, TypePlaylists: the collection name under
// /v1/me/library) to its catalog ID. Resources with no catalog equivalent
// (uploads, private playlists) return an error matching ErrNotFound.
func (c *Client) LibraryCatalogID(ctx context.Context, typ, id string) (string, error) {
	var body Page[Resource[struct{}]]
	if err := c.Get(ctx, "/v1/me/library/"+url.PathEscape(typ)+"/"+url.PathEscape(id)+"/catalog", nil, &body); err != nil {
		return "", err
	}
	r, err := first(body.Data, "catalog "+typ+" for "+id)
	return r.ID, err
}
