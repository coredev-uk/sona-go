package applemusic

import "strings"

// Prefixes of Apple's library and station IDs: a library resource's ID
// says which kind it is ("i.…" song, "l.…" album, "r.…" artist, "p.…"
// playlist), and a station or radio episode starts with "ra.".
const (
	LibrarySongPrefix     = "i."
	LibraryAlbumPrefix    = "l."
	LibraryArtistPrefix   = "r."
	LibraryPlaylistPrefix = "p."
	StationPrefix         = "ra."
)

// IsLibrarySongID reports whether id is a library song's ("i.…").
func IsLibrarySongID(id string) bool { return strings.HasPrefix(id, LibrarySongPrefix) }

// IsLibraryAlbumID reports whether id is a library album's ("l.…").
func IsLibraryAlbumID(id string) bool { return strings.HasPrefix(id, LibraryAlbumPrefix) }

// IsLibraryArtistID reports whether id is a library artist's ("r.…").
func IsLibraryArtistID(id string) bool { return strings.HasPrefix(id, LibraryArtistPrefix) }

// IsLibraryPlaylistID reports whether id is a library playlist's ("p.…").
func IsLibraryPlaylistID(id string) bool { return strings.HasPrefix(id, LibraryPlaylistPrefix) }

// IsStationID reports whether id is a station or radio episode ("ra.…").
func IsStationID(id string) bool { return strings.HasPrefix(id, StationPrefix) }
