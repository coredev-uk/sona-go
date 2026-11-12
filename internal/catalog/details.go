package catalog

import (
	"context"
	"strconv"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/db/dbq"
)

// Detail pages beyond Album and Artist: artist see-all views, the song
// page and music videos.

// ArtistView returns one page of catalog artist id's view (see
// applemusic.ArtistView for T), cached per view and page.
func ArtistView[T any](ctx context.Context, c *Catalog, id, view string, offset, limit int) (applemusic.View[T], error) {
	key := id + "|" + view + "|" + strconv.Itoa(offset) + "|" + strconv.Itoa(limit)
	return cached(ctx, c, "artist-view", key, ArtistTTL, func(ctx context.Context, sf string) (applemusic.View[T], error) {
		return applemusic.ArtistView[T](ctx, c.api, sf, id, view, offset, limit)
	}, nil)
}

// SongPage returns catalog song id with its albums, artists, credits,
// music videos and lyrics excerpt (applemusic.SongPage).
func (c *Catalog) SongPage(ctx context.Context, id string) (applemusic.Song, error) {
	return cached(ctx, c, "song-page", id, SongTTL, func(ctx context.Context, sf string) (applemusic.Song, error) {
		return applemusic.SongPage(ctx, c.api, sf, id)
	}, func(q *dbq.Queries, s applemusic.Song, now int64) error {
		_, err := StoreSong(ctx, q, s, now)
		return err
	})
}

// MusicVideo returns catalog music video id with its more-by-artist and
// more-in-genre views (applemusic.MusicVideoPage).
func (c *Catalog) MusicVideo(ctx context.Context, id string) (applemusic.MusicVideo, error) {
	return cached(ctx, c, "music-video", id, VideoTTL, func(ctx context.Context, sf string) (applemusic.MusicVideo, error) {
		return applemusic.MusicVideoPage(ctx, c.api, sf, id)
	}, nil)
}
