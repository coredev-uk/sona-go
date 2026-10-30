package services

// DTOs are shaped for the frontend: durations and timestamps in integer
// milliseconds (timestamps as Unix ms, 0 when unknown), artwork as
// Go-served /art/ URLs (never Apple templates), IDs as strings.

// Source says where an item's ID lives.
type Source string

const (
	SourceLibrary Source = "library" // ID is a library ID ("i.…", "l.…", "p.…", "r.…")
	SourceCatalog Source = "catalog" // ID is a catalog ID ("1440…", "pl.…")
)

// Artwork sizes handed to the UI, in device pixels (the CSS size ×2).
const (
	artSmall  = 96   // list rows, queue, now-playing bar (48 CSS px)
	artMedium = 400  // grid tiles (up to 200 CSS px)
	artLarge  = 1000 // detail headers, full-screen now playing
)

// Art is an item's artwork at the sizes the UI renders. Every URL is ""
// when the item has no artwork. Other sizes are served by replacing the
// final "<size>.jpg" path element (16–2048).
type Art struct {
	Small  string `json:"small"`
	Medium string `json:"medium"`
	Large  string `json:"large"`
}

// Traits are an item's audio badges from Apple's attributes (audioTraits,
// isAppleDigitalMaster). They are labels only: Sona streams AAC.
type Traits struct {
	Lossless           bool `json:"lossless"`
	HiResLossless      bool `json:"hiResLossless"`
	Atmos              bool `json:"atmos"`
	Spatial            bool `json:"spatial"`
	AppleDigitalMaster bool `json:"appleDigitalMaster"`
}

// Song is a track from the library or the catalog.
type Song struct {
	// ID is the item's ID in Source. PlaybackService takes {id, source}.
	ID        string `json:"id"`
	Source    Source `json:"source"`
	CatalogID string `json:"catalogId"` // "" for uploads
	LibraryID string `json:"libraryId"` // "" for catalog songs
	Name      string `json:"name"`
	Artist    string `json:"artistName"`
	Album     string `json:"albumName"`
	// AlbumID and ArtistID are in the same Source as the song ("" when
	// unknown).
	AlbumID     string `json:"albumId"`
	ArtistID    string `json:"artistId"`
	DurationMs  int64  `json:"durationMs"`
	TrackNumber int    `json:"trackNumber"`
	DiscNumber  int    `json:"discNumber"`
	ReleaseDate string `json:"releaseDate"` // YYYY-MM-DD or ""
	Genre       string `json:"genre"`
	Explicit    bool   `json:"explicit"`
	HasLyrics   bool   `json:"hasLyrics"`
	// Playable is false for catalog songs without play parameters (not
	// streamable in the storefront).
	Playable bool   `json:"playable"`
	Traits   Traits `json:"traits"`
	Art      Art    `json:"art"`
}

// Album is an album from the library or the catalog.
type Album struct {
	ID          string `json:"id"`
	Source      Source `json:"source"`
	CatalogID   string `json:"catalogId"`
	LibraryID   string `json:"libraryId"`
	Name        string `json:"name"`
	Artist      string `json:"artistName"`
	ArtistID    string `json:"artistId"`
	ReleaseDate string `json:"releaseDate"`
	Genre       string `json:"genre"`
	TrackCount  int    `json:"trackCount"`
	Explicit    bool   `json:"explicit"`
	Single      bool   `json:"single"`
	DateAdded   int64  `json:"dateAdded"` // library only
	Traits      Traits `json:"traits"`
	Art         Art    `json:"art"`
}

// Artist is an artist from the library or the catalog.
type Artist struct {
	ID        string `json:"id"`
	Source    Source `json:"source"`
	CatalogID string `json:"catalogId"`
	LibraryID string `json:"libraryId"`
	Name      string `json:"name"`
	Genre     string `json:"genre"`
	Art       Art    `json:"art"`
}

// Playlist is a playlist from the library or the catalog.
type Playlist struct {
	ID           string `json:"id"`
	Source       Source `json:"source"`
	CatalogID    string `json:"catalogId"` // "" for private library playlists
	LibraryID    string `json:"libraryId"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Curator      string `json:"curatorName"`
	CanEdit      bool   `json:"canEdit"`
	TrackCount   int    `json:"trackCount"`
	LastModified int64  `json:"lastModified"`
	DateAdded    int64  `json:"dateAdded"` // library only
	Art          Art    `json:"art"`
	Traits       Traits `json:"traits"`
}

// MusicVideo is a music video from the catalog or the library. Its Art is
// a 16:9 frame (the Go variants keep the aspect: Medium is 400 px wide).
// Video playback is not supported; URL opens it on music.apple.com.
type MusicVideo struct {
	ID          string `json:"id"`
	Source      Source `json:"source"`
	CatalogID   string `json:"catalogId"`
	LibraryID   string `json:"libraryId"`
	Name        string `json:"name"`
	Artist      string `json:"artistName"`
	ArtistID    string `json:"artistId"` // catalog artist ID, "" when unknown
	Album       string `json:"albumName"`
	DurationMs  int64  `json:"durationMs"`
	ReleaseDate string `json:"releaseDate"`
	Genre       string `json:"genre"`
	Explicit    bool   `json:"explicit"`
	Has4K       bool   `json:"has4K"`
	HasHDR      bool   `json:"hasHDR"`
	URL         string `json:"url"` // music.apple.com page, "" for library-only items
	Art         Art    `json:"art"`
}

// Header is the hero artwork of a detail page: a pre-blurred backdrop and
// 2–3 dominant colours ("#rrggbb", most prominent first; empty when they
// could not be computed in time), and Apple's looping motion cover as a
// Go-served /motion/<key>/<size>.mp4 URL ("" when the item has none;
// catalog album and playlist pages only).
type Header struct {
	Backdrop string   `json:"backdrop"`
	Colors   []string `json:"colors"`
	Motion   string   `json:"motion"`
}

// AlbumDetail is an album with its tracks in disc/track order.
type AlbumDetail struct {
	Album      Album  `json:"album"`
	Tracks     []Song `json:"tracks"`
	DurationMs int64  `json:"durationMs"` // sum of the tracks
	Label      string `json:"recordLabel"`
	Copyright  string `json:"copyright"`
	Notes      string `json:"notes"` // editorial notes (catalog), may contain HTML-free text
	// InLibrary: the album (or part of it) is in the user's library. For a
	// catalog album found there, Album.LibraryID is the library copy's ID
	// ("" right after LibraryService.Add, until the sync stores it).
	InLibrary bool   `json:"inLibrary"`
	Header    Header `json:"header"`
}

// ArtistDetail is an artist page. Catalog artists carry TopSongs and their
// full discography in Albums; library artists carry their library Albums
// and Songs.
type ArtistDetail struct {
	Artist   Artist  `json:"artist"`
	TopSongs []Song  `json:"topSongs"`
	Albums   []Album `json:"albums"`
	Songs    []Song  `json:"songs"`
	Header   Header  `json:"header"`
}

// TrackRef names a song to queue: its ID in Source (as in Song.ID and
// Song.Source).
type TrackRef struct {
	ID     string `json:"id"`
	Source Source `json:"source"`
}

// Station kinds (Station.Kind).
const (
	StationTracks    = "tracks"    // algorithmic: artist, song, mood, personal, genre, AutoPlay
	StationLive      = "live"      // Apple Music live radio (Apple Music 1, Hits, …)
	StationBroadcast = "broadcast" // third-party live broadcaster (TuneIn)
	StationEpisode   = "episode"   // on-demand radio show episode
)

// Station is a radio station or radio episode ("ra.…" catalog ID).
type Station struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Kind is StationTracks, StationLive, StationBroadcast or StationEpisode.
	Kind string `json:"kind"`
	// Playable: Sona can play it. Unavailable says why not ("" when
	// playable).
	Playable    bool   `json:"playable"`
	Unavailable string `json:"unavailable"`
	Provider    string `json:"provider"` // stationProviderName, e.g. "Apple Music", "TuneIn"
	Tagline     string `json:"tagline"`
	Notes       string `json:"notes"`
	ShowID      string `json:"showId"` // radio show (apple-curator) of an episode, "" if none
	ShowName    string `json:"showName"`
	HostName    string `json:"hostName"`
	AirTimeMs   int64  `json:"airTimeMs"`  // episode's original broadcast start (Unix ms), 0 if none
	DurationMs  int64  `json:"durationMs"` // episodes only
	URL         string `json:"url"`        // music.apple.com link, "" if none
	Art         Art    `json:"art"`
}

// Item kinds: what an Item is and which page it opens.
const (
	ItemAlbum      = "album"       // Album set; /album/$id
	ItemPlaylist   = "playlist"    // Playlist set; /playlist/$id
	ItemSong       = "song"        // Song set; plays in place
	ItemArtist     = "artist"      // Artist set; /artist/$id
	ItemStation    = "station"     // Station set; /station/$id
	ItemMusicVideo = "music-video" // MusicVideo set; /music-video/$id
	ItemCurator    = "curator"     // apple-curator or curator; /curator/$id
	ItemRoom       = "room"        // editorial "See all" grid; /room/$id
	ItemMultiroom  = "multiroom"   // campaign page; /multiroom/$id
	ItemGrouping   = "grouping"    // editorial hub; /grouping/$id
	ItemCharts     = "charts"      // Charts; ID is a genre ID ("" for all music)
	ItemLink       = "link"        // only on the web (videos, concerts): URL
)

// Item is one card of any kind on a recommendation or editorial shelf, a
// chart, a room or a search result. Exactly one of the typed fields is set
// for the kinds that have one (see the Item* constants), so a card can play
// or offer the item's actions; the flat fields are what the card shows.
type Item struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Name string `json:"name"`
	// Subtitle is the artist, curator, host or provider line.
	Subtitle string `json:"subtitle"`
	// Caption is Apple's reason or editorial badge in sentence case ("Made
	// for you", "New album"), "" when none.
	Caption string `json:"caption"`
	// Art is the item's own artwork: square, round for artists, 16:9 for
	// videos.
	Art Art `json:"art"`
	// Wide is wide editorial artwork for hero cards (Go variants keep the
	// aspect; empty when the item has none); Color its background
	// ("#rrggbb", "" when unknown) for letterboxing.
	Wide     Art    `json:"wide"`
	Color    string `json:"color"`
	Explicit bool   `json:"explicit"`
	URL      string `json:"url"` // music.apple.com page, "" when unknown

	Song       *Song       `json:"song,omitempty"`
	Album      *Album      `json:"album,omitempty"`
	Playlist   *Playlist   `json:"playlist,omitempty"`
	Artist     *Artist     `json:"artist,omitempty"`
	Station    *Station    `json:"station,omitempty"`
	MusicVideo *MusicVideo `json:"musicVideo,omitempty"`
}

// Section kinds: how an editorial or recommendation section is presented.
const (
	SectionHero    = "hero"    // wide editorial cards (Item.Wide)
	SectionFeature = "feature" // large cards with captions (Home's "notes" shelves)
	SectionShelf   = "shelf"   // a row of cover cards
	SectionTracks  = "tracks"  // songs, as a compact track list
	SectionBricks  = "bricks"  // category tiles linking to curators, rooms, multirooms
	SectionLinks   = "links"   // text links (Items without art)
	SectionLive    = "live"    // live radio stations
	SectionText    = "text"    // a paragraph (Text)
)

// Section is one block of an editorial page or Home.
type Section struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Title string `json:"title"` // "" for untitled heroes
	Text  string `json:"text"`  // SectionText only
	Items []Item `json:"items"`
	// Room is the room ID of the section's "See all" grid ("" for none).
	Room string `json:"room"`
	// SeeAll: a Home shelf with a "See all" view (by Section.ID).
	SeeAll bool `json:"seeAll"`
}

// EditorialPage is an editorial page: a grouping (New, Radio, hubs), a
// curator (genre, radio show, brand) or a multiroom (campaign).
type EditorialPage struct {
	Kind     string    `json:"kind"` // ItemGrouping, ItemCurator or ItemMultiroom
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	Subtitle string    `json:"subtitle"` // curator kind or show host, "" otherwise
	Notes    string    `json:"notes"`    // editorial description, plain text
	Art      Art       `json:"art"`      // square logo or cover, may be empty
	Wide     Art       `json:"wide"`     // banner, may be empty
	Color    string    `json:"color"`
	URL      string    `json:"url"`
	Sections []Section `json:"sections"`
	// FetchedAt is when the page was fetched from Apple (Unix ms); Stale
	// is set when a cached copy past its lifetime is served because Apple
	// could not be reached.
	FetchedAt int64 `json:"fetchedAt"`
	Stale     bool  `json:"stale"`
}
