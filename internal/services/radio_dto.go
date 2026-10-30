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
// (in start order, from the programme on air), and its header artwork.
type StationDetail struct {
	Station  Station        `json:"station"`
	Schedule []StationEvent `json:"schedule"`
	Header   Header         `json:"header"`
}

// Create Station seeds (RadioService.CreateStation).
const (
	SeedSong   = "song"
	SeedArtist = "artist"
	SeedAlbum  = "album"
)
