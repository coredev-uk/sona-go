package services

// StationEvent is one scheduled programme of a live station.
type StationEvent struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	StartMs     int64  `json:"startMs"` // Unix ms
	EndMs       int64  `json:"endMs"`   // Unix ms
	Art         Art    `json:"art"`
}

// StationDetail is a station page: the station, a live station's schedule
// (in start order, from the programme on air), its header artwork, and
// for a track station what it was made from (Seed, nil for personal,
// mood and genre stations) and the songs it plays first (Upcoming: the
// batch the next play of the station starts with; empty when signed out
// or when Apple did not answer).
type StationDetail struct {
	Station  Station        `json:"station"`
	Schedule []StationEvent `json:"schedule"`
	Header   Header         `json:"header"`
	Seed     *StationSeed   `json:"seed"`
	Upcoming []Song         `json:"upcoming"`
}

// StationSeed is the song, album or artist a track station plays music
// like. Subtitle is the artist of a song or album ("" for an artist).
// Only: the station plays only this artist's songs, not music like them.
type StationSeed struct {
	Kind     string `json:"kind"` // SeedSong, SeedAlbum or SeedArtist
	ID       string `json:"id"`   // catalog ID
	Name     string `json:"name"`
	Subtitle string `json:"subtitle"`
	ArtistID string `json:"artistId"` // of a song or album, "" when unknown
	Only     bool   `json:"only"`
	Art      Art    `json:"art"`
}

// Create Station seeds (RadioService.CreateStation).
const (
	SeedSong   = "song"
	SeedArtist = "artist"
	SeedAlbum  = "album"
)
