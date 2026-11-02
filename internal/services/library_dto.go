package services

import "github.com/wailsapp/wails/v3/pkg/application"

// Library events.
const (
	EventSyncProgress = "sync:progress" // SyncProgress
	EventSyncDone     = "sync:done"     // SyncDone
	// EventItemState carries an item's new ItemState after a change made
	// outside the UI (`sona like`).
	EventItemState = "library:item-state"
)

func init() {
	application.RegisterEvent[SyncProgress](EventSyncProgress)
	application.RegisterEvent[SyncDone](EventSyncDone)
	application.RegisterEvent[ItemState](EventItemState)
}

// SongPage, AlbumPage, ArtistPage and PlaylistPage are list pages; Total
// is the size of the whole list.
type SongPage struct {
	Items []Song `json:"items"`
	Total int    `json:"total"`
}

type AlbumPage struct {
	Items []Album `json:"items"`
	Total int     `json:"total"`
}

type ArtistPage struct {
	Items []Artist `json:"items"`
	Total int      `json:"total"`
}

type PlaylistPage struct {
	Items []Playlist `json:"items"`
	Total int        `json:"total"`
}

// MusicVideoPage is a page of library music videos; Total is the size of
// the whole list.
type MusicVideoPage struct {
	Items []MusicVideo `json:"items"`
	Total int          `json:"total"`
}

// LibraryEntry is one Recently Added item: Kind "album" with Album, or
// "playlist" with Playlist (the other is null).
type LibraryEntry struct {
	Kind     string    `json:"kind"`
	Album    *Album    `json:"album"`
	Playlist *Playlist `json:"playlist"`
}

// LibraryEntryPage is a page of Recently Added; Total is the size of the
// whole list.
type LibraryEntryPage struct {
	Items []LibraryEntry `json:"items"`
	Total int            `json:"total"`
}

// ItemRef names an item for LibraryService.States, Favorite, SuggestLess,
// Remove and CopyLink: Kind is the Apple catalog type ("songs", "albums",
// "artists", "playlists", "music-videos"; "stations" for CopyLink), ID and
// Source as in the item's DTO, and its CatalogID / LibraryID when known
// (the DTOs carry both).
type ItemRef struct {
	Kind      string `json:"kind"`
	ID        string `json:"id"`
	Source    Source `json:"source"`
	CatalogID string `json:"catalogId"`
	LibraryID string `json:"libraryId"`
}

// ItemState is an item's place in the library and its rating, for the
// ItemRef with the same Kind and ID.
type ItemState struct {
	Kind      string `json:"kind"`
	ID        string `json:"id"`
	InLibrary bool   `json:"inLibrary"`
	// LibraryID is the library copy's ID; "" when not in the library (or
	// just added and not yet known).
	LibraryID string `json:"libraryId"`
	// Rating is 1 (Favourite), -1 (Suggest Less) or 0.
	Rating int `json:"rating"`
}

// Counts are the library list sizes.
type Counts struct {
	Songs     int `json:"songs"`
	Albums    int `json:"albums"`
	Artists   int `json:"artists"`
	Playlists int `json:"playlists"`
}

// ListeningHistory is the History page: plays grouped by local day,
// newest first.
type ListeningHistory struct {
	Days []HistoryDay `json:"days"`
	// Since is the start of the oldest day covered (Unix ms); More reports
	// older plays on this computer (ask for more days).
	Since int64 `json:"since"`
	More  bool  `json:"more"`
	// Apple reports whether Apple's recently played list (the account's
	// other devices) was merged in; false when signed out or offline.
	Apple bool `json:"apple"`
}

// HistoryDay is one local day of plays, newest first.
type HistoryDay struct {
	// Day is local midnight (Unix ms); 0 for the plays from other devices
	// newer than every play on this computer.
	Day     int64          `json:"day"`
	Entries []HistoryEntry `json:"entries"`
}

// HistoryEntry is one play. Plays from the account's other devices come
// from Apple without a time: PlayedAt and PlayedMs are 0.
type HistoryEntry struct {
	PlayedAt   int64  `json:"playedAt"`
	Song       Song   `json:"song"`
	PlayedMs   int64  `json:"playedMs"`
	SourceName string `json:"context"` // what it was played from (album or playlist ID)
}

// SyncProgress is the "sync:progress" payload: Done of Total items of
// Domain (artists, albums, songs, playlists, playlist-tracks, ratings).
type SyncProgress struct {
	Domain   string `json:"domain"`
	Done     int    `json:"done"`
	Total    int    `json:"total"`
	Finished bool   `json:"finished"` // the domain completed (or failed: ErrorCode)
	// ErrorCode is why the domain failed; "" otherwise.
	ErrorCode ErrorCode `json:"errorCode"`
}

// SyncDone is the "sync:done" payload.
type SyncDone struct {
	OK         bool      `json:"ok"`
	ErrorCode  ErrorCode `json:"errorCode"` // why the sync failed; "" when OK
	Counts     Counts    `json:"counts"`
	FinishedAt int64     `json:"finishedAt"`
}

// DomainSync is the stored state of one sync domain.
type DomainSync struct {
	Domain      string `json:"domain"`
	InProgress  bool   `json:"inProgress"` // an interrupted run will resume
	Done        int    `json:"done"`
	Total       int    `json:"total"`
	LastStarted int64  `json:"lastStarted"`
	LastSuccess int64  `json:"lastSuccess"`
	// Failed is set when the domain's last run failed (sync:done said why).
	Failed bool `json:"failed"`
}

// SyncStatus summarises the library sync.
type SyncStatus struct {
	Running bool `json:"running"`
	// LastSuccess is when every domain had last completed (the oldest
	// domain success), 0 when the library never fully synced.
	LastSuccess int64        `json:"lastSuccess"`
	Domains     []DomainSync `json:"domains"`
}
