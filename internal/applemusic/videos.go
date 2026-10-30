package applemusic

import "context"

// TypeMusicVideos is the catalog music video type; library copies are
// "library-music-videos".
const (
	TypeMusicVideos        = "music-videos"
	TypeLibraryMusicVideos = "library-music-videos"
)

// MusicVideoAttributes are the music video attributes Sona uses (catalog
// and library). Artwork is a 16:9 frame, not a square cover.
type MusicVideoAttributes struct {
	Name             string      `json:"name"`
	ArtistName       string      `json:"artistName"`
	AlbumName        string      `json:"albumName"`
	Artwork          *Artwork    `json:"artwork"`
	DurationInMillis int64       `json:"durationInMillis"`
	ReleaseDate      string      `json:"releaseDate"`
	GenreNames       []string    `json:"genreNames"`
	ContentRating    string      `json:"contentRating"`
	Has4K            bool        `json:"has4K"`
	HasHDR           bool        `json:"hasHDR"`
	VideoTraits      []string    `json:"videoTraits"`
	EditorialNotes   *Notes      `json:"editorialNotes"`
	ISRC             string      `json:"isrc"`
	URL              string      `json:"url"`
	PlayParams       *PlayParams `json:"playParams"`
}

// MusicVideo is a catalog ("music-videos") or library
// ("library-music-videos") music video. The detail request fills Views.
type MusicVideo struct {
	Resource[MusicVideoAttributes]
	Relationships struct {
		Artists Relationship[Artist]     `json:"artists"`
		Albums  Relationship[Album]      `json:"albums"`
		Songs   Relationship[Song]       `json:"songs"`
		Catalog Relationship[MusicVideo] `json:"catalog"`
	} `json:"relationships"`
	Views struct {
		MoreByArtist View[MusicVideo] `json:"more-by-artist"`
		MoreInGenre  View[MusicVideo] `json:"more-in-genre"`
	} `json:"views"`
}

// CatalogID returns the catalog music video ID (playParams.catalogId or
// relate=catalog for library copies, "" when unknown).
func (v MusicVideo) CatalogID() string {
	if v.Type == TypeMusicVideos {
		return v.ID
	}
	if p := v.Attributes.PlayParams; p != nil && p.CatalogID != "" {
		return p.CatalogID
	}
	if len(v.Relationships.Catalog.Data) > 0 {
		return v.Relationships.Catalog.Data[0].ID
	}
	return ""
}

// MusicVideoPage returns catalog music video id with its artists, the song
// it belongs to, and the more-by-artist and more-in-genre views.
func MusicVideoPage(ctx context.Context, c *Client, sf, id string) (MusicVideo, error) {
	return AMPResource[MusicVideo](ctx, c, sf, TypeMusicVideos, id, AMPQuery{
		Views:   []string{"more-by-artist", "more-in-genre"},
		Include: []string{"artists", "songs", "albums"},
		Extend:  []string{"editorialArtwork"},
		Fields:  map[string][]string{"artists": {"name", "url", "artwork"}},
	})
}
