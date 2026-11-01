package applemusic

import (
	"context"
	"net/url"
	"strconv"
	"strings"
)

// ArtistViews are the artist page's sections (views=…), each the first
// page of a collection; ArtistView pages through one of them.
type ArtistViews struct {
	LatestRelease     View[Album]      `json:"latest-release,omitzero"`
	TopSongs          View[Song]       `json:"top-songs,omitzero"`
	FeaturedAlbums    View[Album]      `json:"featured-albums,omitzero"`
	FullAlbums        View[Album]      `json:"full-albums,omitzero"`
	Singles           View[Album]      `json:"singles,omitzero"`
	LiveAlbums        View[Album]      `json:"live-albums,omitzero"`
	CompilationAlbums View[Album]      `json:"compilation-albums,omitzero"`
	AppearsOnAlbums   View[Album]      `json:"appears-on-albums,omitzero"`
	Playlists         View[Playlist]   `json:"playlists,omitzero"`
	TopMusicVideos    View[MusicVideo] `json:"top-music-videos,omitzero"`
	MusicVideos       View[MusicVideo] `json:"music-videos,omitzero"`
	SimilarArtists    View[Artist]     `json:"similar-artists,omitzero"`
}

// Artist view names, as used by ArtistView and in Sona's see-all routes.
const (
	ViewLatestRelease   = "latest-release"
	ViewTopSongs        = "top-songs"
	ViewFeaturedAlbums  = "featured-albums"
	ViewFullAlbums      = "full-albums"
	ViewSingles         = "singles"
	ViewLiveAlbums      = "live-albums"
	ViewCompilations    = "compilation-albums"
	ViewAppearsOnAlbums = "appears-on-albums"
	ViewPlaylists       = "playlists"
	ViewTopMusicVideos  = "top-music-videos"
	ViewMusicVideos     = "music-videos"
	ViewSimilarArtists  = "similar-artists"
)

// artistViews are the views ArtistPage asks for.
var artistViews = []string{
	ViewLatestRelease, ViewTopSongs, ViewFeaturedAlbums, ViewFullAlbums, ViewSingles,
	ViewLiveAlbums, ViewCompilations, ViewAppearsOnAlbums, ViewPlaylists,
	ViewTopMusicVideos, ViewMusicVideos, ViewSimilarArtists,
}

// ArtistPage returns catalog artist id from the web player's API with its
// biography, origin and formation date, its motion artwork, and the page
// sections in Views (the "classic" views, simpler than the canvas the web
// app renders).
func ArtistPage(ctx context.Context, c *Client, sf, id string) (Artist, error) {
	return AMPResource[Artist](ctx, c, sf, TypeArtists, id, AMPQuery{
		Views:  artistViews,
		Extend: []string{"artistBio", "bornOrFormed", "origin", "editorialVideo"},
		Extra: url.Values{
			"include[songs]":        {"albums,artists"},
			"include[albums]":       {"artists"},
			"include[music-videos]": {"artists"},
		},
	})
}

// ArtistView returns one page (offset, limit up to 100) of artist id's
// view (one of the View* names), decoded as T: Song for top-songs,
// Playlist for playlists, MusicVideo for the video views, Artist for
// similar-artists, Album for the rest. Next is "" on the last page.
func ArtistView[T any](ctx context.Context, c *Client, sf, id, view string, offset, limit int) (View[T], error) {
	q := AMPQuery{
		With:   []string{"attributes"},
		Limit:  max(1, min(limit, 100)),
		Offset: max(offset, 0),
		Extra: url.Values{
			"include[songs]":        {"albums,artists"},
			"include[albums]":       {"artists"},
			"include[music-videos]": {"artists"},
		},
	}
	return AMP[View[T]](ctx, c, CatalogPath(sf, TypeArtists, id, "view", view), q)
}

// NextOffset returns the offset query parameter of a view's or
// relationship's next link, or -1 when there is no next page.
func NextOffset(next string) int {
	if next == "" {
		return -1
	}
	_, query, _ := strings.Cut(next, "?")
	v, err := url.ParseQuery(query)
	if err != nil {
		return -1
	}
	n, err := strconv.Atoi(v.Get("offset"))
	if err != nil {
		return -1
	}
	return n
}
