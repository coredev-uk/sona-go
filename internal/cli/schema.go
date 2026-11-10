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

// Lyrics is `sona lyrics`: the playing song's lyrics.
type Lyrics struct {
	// Track is the song the lyrics are of.
	Track Track `json:"track"`
	// State and Position are the playback state and position (seconds)
	// when the lyrics were handed over.
	State    string  `json:"state"`
	Position float64 `json:"position"`
	// Provider is the source of the lyrics: "apple", "musixmatch",
	// "lrclib" or "netease".
	Provider string `json:"provider"`
	// Timing is "word" (every word timed), "line" (every line timed) or
	// "none" (plain text).
	Timing string `json:"timing"`
	// Synced: the lines have times (Timing is "word" or "line").
	Synced bool `json:"synced"`
	// Lines are in the order sung, timed lines by start.
	Lines []LyricLine `json:"lines"`
}

// LyricLine is one line of lyrics. Start and End are seconds into the
// song (0 and 0 when the lyrics are not synced); End is at least the time
// the line's words end, which may be after the next line starts.
type LyricLine struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
	// Background is the background vocals sung with the line; "" when none.
	Background string `json:"background"`
	// Agent is the voice: "v1", "v2" (a duet's second voice), …; "" when
	// the lyrics don't say.
	Agent string `json:"agent"`
	// Part is the song part ("Verse", "Chorus", …); "" when not known.
	Part string `json:"part"`
	// Translation and Transliteration are the line in the lyrics'
	// translation language and in Latin script; "" when the line has none.
	Translation     string `json:"translation"`
	Transliteration string `json:"transliteration"`
}

// LyricsNow is a line of `sona lyrics --follow`: what is being sung now.
type LyricsNow struct {
	// Running is false only in the last line, when Sona has quit (or was
	// not running).
	Running bool `json:"running"`
	// State and Track are the playback state and the selected song (null
	// when none), as in Status.
	State string `json:"state"`
	Track *Track `json:"track"`
	// Position is how far into the song playback is, in seconds, when the
	// line was printed.
	Position float64 `json:"position"`
	// Lyrics says what the follower knows of the song's lyrics: "ready",
	// "loading" (being fetched) or "none" (nothing is playing, the song
	// has no lyrics or they could not be fetched).
	Lyrics   string `json:"lyrics"`
	Provider string `json:"provider"`
	Timing   string `json:"timing"`
	Synced   bool   `json:"synced"`
	// Index is the place of Line in the lyrics' lines; -1 when no line is
	// sung (before the first, in a rest, plain lyrics, no lyrics).
	Index int `json:"index"`
	// Line is the line sung now and Next the line that comes after it
	// (the first one, before it); null when there is none.
	Line *LyricLine `json:"line"`
	Next *LyricLine `json:"next"`
	// Text is what `sona lyrics --follow` prints: Line's text, or its
	// translation or transliteration with --translation and
	// --transliteration; "" when no line is sung.
	Text string `json:"text"`
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
