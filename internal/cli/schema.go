package cli

// The JSON every command answers with: the result of a Reply, and what
// --json prints. These schemas are stable: fields may be added, never
// renamed, removed or changed in meaning (docs/cli.md lists them).
// Durations and positions are seconds; the volume is a percentage.

// Status is `sona status` and the answer to play, pause, toggle, next
// and prev.
type Status struct {
	// Running is false only when no Sona runs (printed by the client).
	Running bool `json:"running"`
	// State is "idle" (nothing loaded), "loading", "playing", "paused",
	// "ended" or "error".
	State string `json:"state"`
	// Track is the selected song; null when none.
	Track *Track `json:"track"`
	// Position is how far into Track playback is, in seconds, as of the
	// moment the status was taken.
	Position float64 `json:"position"`
	Volume   int     `json:"volume"`
	Shuffle  bool    `json:"shuffle"`
	// Repeat is "off", "all" or "one".
	Repeat string `json:"repeat"`
	// Private: private listening is on (nothing is reported or scrobbled).
	Private bool `json:"private"`
	// Preview: Track plays as its 30-second preview.
	Preview bool `json:"preview"`
	// Error describes the latest playback failure; "" when none.
	Error string `json:"error"`
}

// Track is a song in the status or the queue.
type Track struct {
	// ID is the Apple Music catalogue ID, or the library ID ("i.…") of a
	// song only in the library.
	ID     string `json:"id"`
	Title  string `json:"title"`
	Artist string `json:"artist"`
	Album  string `json:"album"`
	// Duration in seconds; 0 when unknown.
	Duration float64 `json:"duration"`
	Explicit bool    `json:"explicit"`
	// Artwork is a 600×600 image URL on Apple's artwork CDN; "" when none.
	Artwork string `json:"artwork"`
}

// Queue is `sona queue`: every song in play order.
type Queue struct {
	// Index is the playing song's place in Tracks; -1 when none.
	Index  int          `json:"index"`
	Tracks []QueueTrack `json:"tracks"`
}

// QueueTrack is one place in the queue.
type QueueTrack struct {
	Track
	// AutoPlay: the song came from the station or AutoPlay, not from you.
	AutoPlay bool `json:"autoPlay"`
}

// SearchResults is `sona search`.
type SearchResults struct {
	Term    string         `json:"term"`
	Results []SearchResult `json:"results"`
}

// SearchResult is one search result.
type SearchResult struct {
	// Kind is "song", "album", "artist", "playlist", "station",
	// "music-video" or "curator".
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	Subtitle string `json:"subtitle"`
	Explicit bool   `json:"explicit"`
	// URL is the music.apple.com link (for `sona open`); "" when unknown.
	URL string `json:"url"`
}

// Volume is `sona volume`.
type Volume struct {
	Volume int `json:"volume"`
}

// Like is `sona like`: the playing song and whether it is a favourite.
type Like struct {
	Track Track `json:"track"`
	Liked bool  `json:"liked"`
}

// Private is `sona private`.
type Private struct {
	On bool `json:"on"`
	// Source is what decides it: "setting", "development" (a development
	// build starts private for the session) or "environment"
	// (SONA_PRIVATE=1).
	Source string `json:"source"`
	// Saved is the saved setting.
	Saved bool `json:"saved"`
}

// Opened is `sona open`: what the link opened.
type Opened struct {
	// Kind is "song", "album", "artist", "playlist", "library-playlist",
	// "library-album", "music-video", "station" or "curator".
	Kind string `json:"kind"`
	ID   string `json:"id"`
	// Storefront is the link's storefront ("us"); "" when it has none.
	Storefront string `json:"storefront"`
}

// Status states.
const (
	StateIdle    = "idle"
	StateLoading = "loading"
	StatePlaying = "playing"
	StatePaused  = "paused"
	StateEnded   = "ended"
	StateError   = "error"
)

// Private sources.
const (
	SourceSetting     = "setting"
	SourceDevelopment = "development"
	SourceEnvironment = "environment"
)

// NotRunning is the status printed when no Sona runs.
func NotRunning() Status { return Status{State: StateIdle, Repeat: "off"} }
