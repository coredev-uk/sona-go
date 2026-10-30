package applemusic

import (
	"context"
	"net/http"
	"net/url"
	"strings"
)

// FavoritesPath is the web player's favourites collection on amp-api.
const FavoritesPath = "/v1/me/favorites"

// Favorite marks catalog resources of type typ (TypeSongs, TypeAlbums,
// TypeArtists, TypePlaylists, TypeMusicVideos) as favourites. Apple sets
// their rating to 1 and, for songs, also adds them to the library (the
// library copy stays after Unfavorite).
func (c *Client) Favorite(ctx context.Context, typ string, ids []string) error {
	return c.favorites(ctx, http.MethodPost, typ, ids)
}

// Unfavorite removes catalog resources of type typ from the favourites
// (their rating is cleared; library copies stay).
func (c *Client) Unfavorite(ctx context.Context, typ string, ids []string) error {
	return c.favorites(ctx, http.MethodDelete, typ, ids)
}

func (c *Client) favorites(ctx context.Context, method, typ string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	q := url.Values{"ids[" + typ + "]": {strings.Join(ids, ",")}}
	return c.Do(ctx, Request{Method: method, URL: c.ampBase() + FavoritesPath, Query: q}, nil)
}
