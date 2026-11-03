package services

// Detail pages served by CatalogService: album, artist (and its see-all
// views), song and music video.

// CatalogAlbum is a catalog album page: the album and its tracks
// (AlbumDetail) plus Apple's sections for it. Notes is plain text with
// paragraphs separated by blank lines.
type CatalogAlbum struct {
	AlbumDetail
	NotesShort string `json:"notesShort"`
	// Artists are every artist credited on the album (Album.ArtistID is the
	// first).
	Artists []Artist `json:"artists"`
	// PopularIDs are the IDs of the album's most played tracks (at most 3).
	PopularIDs []string `json:"popularIds"`
	URL        string   `json:"url"` // music.apple.com page
	// ArtworkColors are Apple's colours for the cover.
	ArtworkColors ArtworkColors `json:"artworkColors"`
	// Genres are every genre Apple files the album under, the main one
	// (Album.Genre) first, without the root "Music".
	Genres []string `json:"genres"`
	// Sections, each empty when Apple has none.
	MoreByArtist  []Album      `json:"moreByArtist"`
	Similar       []Album      `json:"similar"` // "you might also like"
	OtherVersions []Album      `json:"otherVersions"`
	AppearsOn     []Playlist   `json:"appearsOn"` // playlists featuring the album
	Videos        []MusicVideo `json:"videos"`
	MoreToHear    []Station    `json:"moreToHear"` // radio episodes about the album
}

// Artist page view names (ArtistShelf.View, CatalogService.ArtistView).
const (
	ViewTopSongs       = "top-songs"
	ViewFeaturedAlbums = "featured-albums"
	ViewFullAlbums     = "full-albums"
	ViewSingles        = "singles"
	ViewLiveAlbums     = "live-albums"
	ViewCompilations   = "compilation-albums"
	ViewAppearsOn      = "appears-on-albums"
	ViewPlaylists      = "playlists"
	ViewMusicVideos    = "music-videos"
	ViewTopMusicVideos = "top-music-videos"
	ViewSimilarArtists = "similar-artists"
)

// ShelfKind says which list of an ArtistShelf or ArtistViewPage is filled.
type ShelfKind string

const (
	ShelfSongs     ShelfKind = "songs"
	ShelfAlbums    ShelfKind = "albums"
	ShelfPlaylists ShelfKind = "playlists"
	ShelfVideos    ShelfKind = "videos"
	ShelfArtists   ShelfKind = "artists"
)

// ArtistShelf is one section of an artist page: the first items of view
// View, in the list Kind names (the others are empty). More reports that
// CatalogService.ArtistView has further items.
type ArtistShelf struct {
	View      string       `json:"view"`
	Kind      ShelfKind    `json:"kind"`
	More      bool         `json:"more"`
	Albums    []Album      `json:"albums"`
	Playlists []Playlist   `json:"playlists"`
	Videos    []MusicVideo `json:"videos"`
	Artists   []Artist     `json:"artists"`
}

// CatalogArtist is a catalog artist page. Bio is plain text with paragraphs
// separated by blank lines; Origin and BornOrFormed are Apple's free text
// ("Abingdon, Oxfordshire, England", "1985").
type CatalogArtist struct {
	Artist        Artist        `json:"artist"`
	Header        Header        `json:"header"`
	Bio           string        `json:"bio"`
	Origin        string        `json:"origin"`
	BornOrFormed  string        `json:"bornOrFormed"`
	Genres        []string      `json:"genres"`
	URL           string        `json:"url"`
	LatestRelease *Album        `json:"latestRelease"` // nil when Apple has none
	TopSongs      []Song        `json:"topSongs"`
	TopSongsMore  bool          `json:"topSongsMore"`
	Shelves       []ArtistShelf `json:"shelves"` // in display order, empty ones left out
}

// ArtistViewPage is one page of an artist view (a see-all list): the list
// Kind names is filled. Next is the offset of the following page, -1 on
// the last one.
type ArtistViewPage struct {
	View      string       `json:"view"`
	Kind      ShelfKind    `json:"kind"`
	Next      int          `json:"next"`
	Songs     []Song       `json:"songs"`
	Albums    []Album      `json:"albums"`
	Playlists []Playlist   `json:"playlists"`
	Videos    []MusicVideo `json:"videos"`
	Artists   []Artist     `json:"artists"`
}

// Credit is one credited person with their roles on a song. Art is the
// person's picture when Apple has one (all "" otherwise: the UI draws their
// initials).
type Credit struct {
	Name  string   `json:"name"`
	Roles []string `json:"roles"`
	Art   Art      `json:"art"`
}

// CreditGroup is one category of a song's credits. Kind is "performer",
// "composer-and-lyrics" or "production-and-engineering" (others may
// appear; Title is Apple's upper-case label for them).
type CreditGroup struct {
	Kind    string   `json:"kind"`
	Title   string   `json:"title"`
	Credits []Credit `json:"credits"`
}

// CatalogSong is a catalog song page. Album is nil for songs Apple lists on
// no album; LyricsExcerpt is a few lines separated by "\n" ("" when the
// song has no lyrics).
type CatalogSong struct {
	Song                Song          `json:"song"`
	Header              Header        `json:"header"`
	Album               *Album        `json:"album"`
	Artists             []Artist      `json:"artists"`
	Composer            string        `json:"composer"`
	ISRC                string        `json:"isrc"`
	URL                 string        `json:"url"`
	HasTimeSyncedLyrics bool          `json:"hasTimeSyncedLyrics"`
	LyricsExcerpt       string        `json:"lyricsExcerpt"`
	Credits             []CreditGroup `json:"credits"`
	Videos              []MusicVideo  `json:"videos"`
	// MoreByArtist are other albums by the song's artist (not its own).
	MoreByArtist []Album `json:"moreByArtist"`
}

// CatalogVideo is a catalog music video page. Song is the audio track the
// video belongs to (nil when Apple links none). Video playback is not
// supported; Video.URL opens it on music.apple.com.
type CatalogVideo struct {
	Video        MusicVideo   `json:"video"`
	Header       Header       `json:"header"`
	Notes        string       `json:"notes"`
	Artists      []Artist     `json:"artists"`
	Song         *Song        `json:"song"`
	MoreByArtist []MusicVideo `json:"moreByArtist"`
	MoreInGenre  []MusicVideo `json:"moreInGenre"`
}
