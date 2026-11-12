package applemusic

import (
	"context"
	"fmt"
	"net/url"
)

// AlbumViews are the album page's sections (views=…), each the first page
// of a collection. AudioExtras ("More to hear") are radio episodes about
// the album.
type AlbumViews struct {
	MoreByArtist     View[Album]      `json:"more-by-artist,omitzero"`
	YouMightAlsoLike View[Album]      `json:"you-might-also-like,omitzero"`
	AppearsOn        View[Playlist]   `json:"appears-on,omitzero"`
	OtherVersions    View[Album]      `json:"other-versions,omitzero"`
	RelatedVideos    View[MusicVideo] `json:"related-videos,omitzero"`
	AudioExtras      View[Station]    `json:"audio-extras,omitzero"`
}

// SongMeta is per-track metadata on album tracks.
type SongMeta struct {
	// Popularity is 0–1 relative to the album (meta[albums:tracks]=popularity).
	Popularity float64 `json:"popularity,omitempty"`
}

// albumViews are the views AlbumPage asks for.
var albumViews = []string{"more-by-artist", "you-might-also-like", "appears-on", "other-versions", "related-videos", "audio-extras"}

// AlbumPage returns catalog album id from the web player's API: every
// track (with its artists, popularity and audio traits), the album's
// artists and genres, its editorial notes, its motion artwork, and the
// page sections in Views.
func AlbumPage(ctx context.Context, c *Client, sf, id string) (Album, error) {
	q := AMPQuery{
		Views:   albumViews,
		Include: []string{"artists", "genres"},
		Extend:  []string{"editorialVideo"},
		Fields: map[string][]string{
			"artists": {"name", "url", "artwork"},
			"genres":  {"name", "parentId"},
		},
		Extra: url.Values{
			"include[songs]":        {"artists"},
			"include[music-videos]": {"artists"},
			"meta[albums:tracks]":   {"popularity"},
		},
	}
	a, err := AMPResource[Album](ctx, c, sf, TypeAlbums, id, q)
	if err != nil {
		return Album{}, err
	}
	if err := followNext(ctx, c, &a.Relationships.Tracks, c.ampBase(), url.Values{"include[songs]": {"artists"}, "platform": {"web"}}); err != nil {
		return Album{}, fmt.Errorf("album %s tracks: %w", id, err)
	}
	return a, nil
}
