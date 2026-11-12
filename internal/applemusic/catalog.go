package applemusic

import (
	"fmt"
	"net/url"
	"strings"
)

// AMPBaseURL is the web player's API host. Lyrics are only served here; the
// public api.music.apple.com host rejects them for the web player token.
const AMPBaseURL = "https://amp-api.music.apple.com"

// Resource types, as used in search types, ratings and add-to-library.
const (
	TypeSongs            = "songs"
	TypeAlbums           = "albums"
	TypeArtists          = "artists"
	TypePlaylists        = "playlists"
	TypeLibrarySongs     = "library-songs"
	TypeLibraryAlbums    = "library-albums"
	TypeLibraryArtists   = "library-artists"
	TypeLibraryPlaylists = "library-playlists"
)

// CatalogPath builds "/v1/catalog/{sf}/{parts...}" with each part escaped.
func CatalogPath(sf string, parts ...string) string { return scopedPath("catalog", sf, parts...) }

// scopedPath builds "/v1/{scope}/{sf}/{parts...}" with each part escaped.
func scopedPath(scope, sf string, parts ...string) string {
	var b strings.Builder
	b.WriteString("/v1/")
	b.WriteString(scope)
	b.WriteByte('/')
	b.WriteString(url.PathEscape(sf))
	for _, p := range parts {
		b.WriteByte('/')
		b.WriteString(url.PathEscape(p))
	}
	return b.String()
}

func first[T any](data []T, what string) (T, error) {
	if len(data) == 0 {
		var zero T
		return zero, fmt.Errorf("%w: %s", ErrNotFound, what)
	}
	return data[0], nil
}
