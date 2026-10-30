package applemusic

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

// AlbumViews are the album page's sections (views=…), each the first page
// of a collection.
type AlbumViews struct {
	MoreByArtist     View[Album]      `json:"more-by-artist,omitzero"`
	YouMightAlsoLike View[Album]      `json:"you-might-also-like,omitzero"`
	AppearsOn        View[Playlist]   `json:"appears-on,omitzero"`
	OtherVersions    View[Album]      `json:"other-versions,omitzero"`
	RelatedVideos    View[MusicVideo] `json:"related-videos,omitzero"`
}

// SongMeta is per-track metadata on album tracks.
type SongMeta struct {
	// Popularity is 0–1 relative to the album (meta[albums:tracks]=popularity).
	Popularity float64 `json:"popularity,omitempty"`
}

// albumViews are the views AlbumPage asks for.
var albumViews = []string{"more-by-artist", "you-might-also-like", "appears-on", "other-versions", "related-videos"}

// AlbumPage returns catalog album id from the web player's API: every
// track (with its artists, popularity and audio traits), the album's
// artists, its editorial notes, and the page sections in Views.
func AlbumPage(ctx context.Context, c *Client, sf, id string) (Album, error) {
	q := AMPQuery{
		Views:   albumViews,
		Include: []string{"artists"},
		Fields:  map[string][]string{"artists": {"name", "url", "artwork"}},
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
	if err := followAMP(ctx, c, &a.Relationships.Tracks, url.Values{"include[songs]": {"artists"}}); err != nil {
		return Album{}, fmt.Errorf("album %s tracks: %w", id, err)
	}
	return a, nil
}

// followAMP appends the remaining pages of an amp-api relationship (from
// rel.Next) to rel.Data, 100 items per request, sending extra with each
// page, and clears rel.Next.
func followAMP[T any](ctx context.Context, c *Client, rel *Relationship[T], extra url.Values) error {
	for rel.Next != "" {
		u, err := url.Parse(rel.Next)
		if err != nil {
			return err
		}
		q := u.Query()
		for k, v := range extra {
			q[k] = v
		}
		q.Set("limit", strconv.Itoa(defaultPageSize))
		q.Set("platform", "web")
		var p Relationship[T]
		if err := c.Get(ctx, c.ampBase()+u.Path, q, &p); err != nil {
			return err
		}
		rel.Data = append(rel.Data, p.Data...)
		if p.Next == rel.Next || len(p.Data) == 0 {
			break
		}
		rel.Next = p.Next
	}
	rel.Next = ""
	return nil
}
