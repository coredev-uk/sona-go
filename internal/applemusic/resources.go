package applemusic

import (
	"strconv"
	"strings"
)

// Artwork is an artwork descriptor. URL is a template containing {w} and
// {h} (and sometimes {f}); store it raw and size it with Sized.
type Artwork struct {
	URL        string `json:"url"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	BgColor    string `json:"bgColor"`
	TextColor1 string `json:"textColor1"`
	TextColor2 string `json:"textColor2"`
}

// Template returns the raw URL template, "" for a nil artwork.
func (a *Artwork) Template() string {
	if a == nil {
		return ""
	}
	return a.URL
}

// Sized fills a {w}x{h} artwork template (format jpg when {f} is present).
func Sized(template string, w, h int) string {
	return strings.NewReplacer("{w}", strconv.Itoa(w), "{h}", strconv.Itoa(h), "{f}", "jpg").Replace(template)
}

// PlayParams identifies a playable resource. Library resources carry
// IsLibrary and, for songs, the CatalogID they map to; library playlists
// carry the catalog GlobalID.
type PlayParams struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	IsLibrary   bool   `json:"isLibrary"`
	CatalogID   string `json:"catalogId"`
	GlobalID    string `json:"globalId"`
	ReportingID string `json:"reportingId"`
}

// Notes is Apple's editorial notes / description object.
type Notes struct {
	Short    string `json:"short"`
	Standard string `json:"standard"`
}

// Text returns Standard, falling back to Short; "" for nil.
func (n *Notes) Text() string {
	if n == nil {
		return ""
	}
	if n.Standard != "" {
		return n.Standard
	}
	return n.Short
}

// AlbumAttributes are the album attributes Sona uses (catalog and library).
type AlbumAttributes struct {
	Name           string      `json:"name"`
	ArtistName     string      `json:"artistName"`
	Artwork        *Artwork    `json:"artwork"`
	GenreNames     []string    `json:"genreNames"`
	ReleaseDate    string      `json:"releaseDate"`
	TrackCount     int         `json:"trackCount"`
	ContentRating  string      `json:"contentRating"`
	IsSingle       bool        `json:"isSingle"`
	IsCompilation  bool        `json:"isCompilation"`
	IsComplete     bool        `json:"isComplete"`
	RecordLabel    string      `json:"recordLabel"`
	Copyright      string      `json:"copyright"`
	EditorialNotes *Notes      `json:"editorialNotes"`
	UPC            string      `json:"upc"`
	URL            string      `json:"url"`
	DateAdded      string      `json:"dateAdded"` // library only, RFC 3339
	IsPrerelease   bool        `json:"isPrerelease"`
	PlayParams     *PlayParams `json:"playParams"`
	// Display badges (see SongAttributes).
	AudioTraits          []string `json:"audioTraits"`
	IsAppleDigitalMaster bool     `json:"isAppleDigitalMaster"`
	IsMasteredForItunes  bool     `json:"isMasteredForItunes"`
}

// ArtistAttributes are the artist attributes Sona uses. Library artists
// carry only Name.
type ArtistAttributes struct {
	Name       string   `json:"name"`
	Artwork    *Artwork `json:"artwork"`
	GenreNames []string `json:"genreNames"`
	URL        string   `json:"url"`
	// Artist page only (extend=artistBio,origin,bornOrFormed).
	ArtistBio    string `json:"artistBio,omitempty"` // may contain HTML
	Origin       string `json:"origin,omitempty"`
	BornOrFormed string `json:"bornOrFormed,omitempty"`
}

// PlaylistAttributes are the playlist attributes Sona uses.
type PlaylistAttributes struct {
	Name             string      `json:"name"`
	CuratorName      string      `json:"curatorName"`
	Description      *Notes      `json:"description"`
	Artwork          *Artwork    `json:"artwork"`
	PlaylistType     string      `json:"playlistType"`
	LastModifiedDate string      `json:"lastModifiedDate"`
	DateAdded        string      `json:"dateAdded"` // library only
	CanEdit          bool        `json:"canEdit"`
	IsPublic         bool        `json:"isPublic"`
	HasCatalog       bool        `json:"hasCatalog"`
	URL              string      `json:"url"`
	PlayParams       *PlayParams `json:"playParams"`
	// Playlist page and library edits (Playlists slice).
	CanDelete      bool     `json:"canDelete"`
	Tags           []string `json:"tags"`       // library, extend[library-playlists]=tags; "favorited"
	TrackCount     int      `json:"trackCount"` // catalog, extend=trackCount
	IsChart        bool     `json:"isChart"`
	EditorialNotes *Notes   `json:"editorialNotes"`
	AudioTraits    []string `json:"audioTraits"` // display badges (see SongAttributes)
}

// Relationship is a to-many relationship or a paged collection: Next is the
// path of the following page ("" on the last page).
type Relationship[T any] struct {
	Href string `json:"href"`
	Next string `json:"next"`
	Data []T    `json:"data"`
}

// View is an artist view such as top-songs or full-albums.
type View[T any] struct {
	Href       string `json:"href"`
	Next       string `json:"next"`
	Attributes struct {
		Title string `json:"title"`
	} `json:"attributes"`
	Data []T `json:"data"`
}

// Song is a catalog song ("songs") or library song ("library-songs").
// Library songs fetched with relate=albums,artists carry the library album
// and artist IDs in Relationships.
type Song struct {
	Resource[SongAttributes]
	Relationships struct {
		Albums  Relationship[Album]  `json:"albums"`
		Artists Relationship[Artist] `json:"artists"`
		Catalog Relationship[Song]   `json:"catalog"`
		// Song page only (include=credits,music-videos).
		Credits     Relationship[CreditCategory] `json:"credits,omitzero"`
		MusicVideos Relationship[MusicVideo]     `json:"music-videos,omitzero"`
	} `json:"relationships"`
	// Meta.Popularity is set on album tracks (meta[albums:tracks]=popularity).
	Meta SongMeta `json:"meta,omitzero"`
}

// CatalogID returns the catalog song ID: the ID itself for a catalog song,
// playParams.catalogId for a library song ("" for uploads).
func (s Song) CatalogID() string {
	if s.Type == "songs" {
		return s.ID
	}
	if p := s.Attributes.PlayParams; p != nil && p.CatalogID != "" {
		return p.CatalogID
	}
	if len(s.Relationships.Catalog.Data) > 0 {
		return s.Relationships.Catalog.Data[0].ID
	}
	return ""
}

// Album is a catalog album ("albums") or library album ("library-albums").
type Album struct {
	Resource[AlbumAttributes]
	Relationships struct {
		Tracks  Relationship[Song]   `json:"tracks"`
		Artists Relationship[Artist] `json:"artists"`
		Catalog Relationship[Album]  `json:"catalog"`
	} `json:"relationships"`
	// Views are the album page's sections (AlbumPage only).
	Views AlbumViews `json:"views,omitzero"`
}

// CatalogID returns the catalog album ID (via relate=catalog for library
// albums, "" when unknown).
func (a Album) CatalogID() string {
	if a.Type == "albums" {
		return a.ID
	}
	if len(a.Relationships.Catalog.Data) > 0 {
		return a.Relationships.Catalog.Data[0].ID
	}
	return ""
}

// Artist is a catalog artist ("artists") or library artist ("library-artists").
type Artist struct {
	Resource[ArtistAttributes]
	Relationships struct {
		Albums  Relationship[Album]  `json:"albums"`
		Catalog Relationship[Artist] `json:"catalog"`
	} `json:"relationships"`
	// Views are the artist page's sections (ArtistPage only).
	Views ArtistViews `json:"views,omitzero"`
}

// CatalogID returns the catalog artist ID ("" when unknown).
func (a Artist) CatalogID() string {
	if a.Type == "artists" {
		return a.ID
	}
	if len(a.Relationships.Catalog.Data) > 0 {
		return a.Relationships.Catalog.Data[0].ID
	}
	return ""
}

// Playlist is a catalog playlist ("playlists") or library playlist
// ("library-playlists").
type Playlist struct {
	Resource[PlaylistAttributes]
	Relationships struct {
		Tracks  Relationship[Song]     `json:"tracks"`
		Catalog Relationship[Playlist] `json:"catalog"`
	} `json:"relationships"`
}

// CatalogID returns the catalog playlist ID (playParams.globalId for library
// playlists, "" for private ones).
func (p Playlist) CatalogID() string {
	if p.Type == "playlists" {
		return p.ID
	}
	if pp := p.Attributes.PlayParams; pp != nil && pp.GlobalID != "" {
		return pp.GlobalID
	}
	if len(p.Relationships.Catalog.Data) > 0 {
		return p.Relationships.Catalog.Data[0].ID
	}
	return ""
}
