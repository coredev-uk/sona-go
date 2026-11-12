package store

// The keys of the library and catalogue queries more than one feature reads
// or invalidates. Prefix invalidation (Invalidate(KeyLibrary)) depends on
// every user of a key spelling it the same way, so they are spelled here.
var (
	KeyLibrary             = Key{"library"}
	KeyCatalog             = Key{"catalog"}
	KeyDiscover            = Key{"discover"}
	KeyLibraryCounts       = Key{"library", "counts"}
	KeyLibraryRecent       = Key{"library", "recent"}
	KeyLibraryFavourites   = Key{"library", "favorite-songs"}
	KeyLibraryPlaylistsAll = Key{"library", "playlists"}
)

// LibraryArtist is a library artist's query.
func LibraryArtist(id string) Key { return Key{"library", "artist", id} }

// LibraryAlbum is a library album's query.
func LibraryAlbum(id string) Key { return Key{"library", "album", id} }

// LibraryPlaylists is a page of the library's playlists in a sort order.
func LibraryPlaylists(sort string, desc bool, n int) Key {
	return Key{"library", "playlists", sort, desc, n}
}

// LibrarySearch is the library's matches for term, perKind of each kind.
func LibrarySearch(term string, perKind int) Key {
	return Key{"library", "search", term, perKind}
}

// CatalogArtist is a catalogue artist's query.
func CatalogArtist(id string) Key { return Key{"catalog", "artist", id} }

// CatalogAlbum is a catalogue album's query.
func CatalogAlbum(id string) Key { return Key{"catalog", "album", id} }
