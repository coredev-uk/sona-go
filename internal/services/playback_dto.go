package services

import "github.com/wailsapp/wails/v3/pkg/application"

// Playback events.
const (
	EventPlaybackState = "playback:state" // PlaybackState (latest wins)
	EventPlaybackQueue = "playback:queue" // QueueState, only when the queue changed
)

func init() {
	application.RegisterEvent[PlaybackState](EventPlaybackState)
	application.RegisterEvent[QueueState](EventPlaybackQueue)
}

// QueueItem is one queue slot. UID is stable while the slot exists, so it
// suits list keys. ID/Source identify the song for navigation.
type QueueItem struct {
	UID        uint64 `json:"uid"`
	ID         string `json:"id"`
	Source     Source `json:"source"`
	CatalogID  string `json:"catalogId"`
	LibraryID  string `json:"libraryId"`
	Name       string `json:"name"`
	Artist     string `json:"artistName"`
	Album      string `json:"albumName"`
	AlbumID    string `json:"albumId"` // in Source; "" when unknown
	ArtistID   string `json:"artistId"`
	DurationMs int64  `json:"durationMs"`
	Art        Art    `json:"art"`
	Backdrop   string `json:"backdrop"`
	// Auto: the slot was added by the station or AutoPlay, not the user.
	Auto bool `json:"auto"`
	// Live: a live radio stream (no duration, cannot seek).
	Live bool `json:"live"`
	// Added: the user queued the song by hand (Play Next, Add to Queue)
	// rather than it coming with the album or playlist that was played.
	Added bool `json:"added"`
	// Explicit: Apple rates the song explicit (skipped while the account
	// restricts explicit content).
	Explicit bool `json:"explicit"`
}

// PlayFrom names where PlayItems' songs come from. Kind is "album",
// "playlist" or "artist" for an Apple collection (ID its catalog or
// library ID; plays are reported with it), "" for anything else (ID is
// then a label such as "search").
type PlayFrom struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// QueueContainer is the collection the current song was queued from:
// Kind album, playlist, artist or station, and its ID.
type QueueContainer struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// SleepTimer is the sleep timer. Mode is "after" (playback pauses at
// EndsAt, Unix ms), "track" (when the current song ends) or "queue"
// (when the last song the user queued ends).
type SleepTimer struct {
	Mode   string `json:"mode"`
	EndsAt int64  `json:"endsAt"` // 0 unless Mode is "after"
}

// QueueStation is the station feeding the queue: one the user started, or
// AutoPlay continuing after the user's queue.
type QueueStation struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	AutoPlay bool   `json:"autoPlay"`
}

// PlaybackState is the now-playing state ("playback:state", latest wins).
// PositionMs is exact at At (Unix ms); while State is "playing" the UI
// extrapolates PositionMs + (now - At). Events are sent on changes only.
type PlaybackState struct {
	// State is idle, loading, playing, paused, ended or error.
	State      string     `json:"state"`
	Current    *QueueItem `json:"current"`
	Index      int        `json:"index"` // of Current in the queue, -1 when none
	PositionMs int64      `json:"positionMs"`
	DurationMs int64      `json:"durationMs"`
	At         int64      `json:"at"`
	Volume     float64    `json:"volume"` // 0..1
	Shuffle    bool       `json:"shuffle"`
	Repeat     string     `json:"repeat"` // off, all, one
	// Context is the PlayItems label (album or playlist ID) the queue came
	// from.
	Context      string `json:"context"`
	QueueLength  int    `json:"queueLength"`
	QueueVersion uint64 `json:"queueVersion"`
	CanNext      bool   `json:"canNext"`
	CanPrevious  bool   `json:"canPrevious"`
	// Error is the latest playback failure (clears once an item plays).
	Error string `json:"error"`
	// AuthRequired: Apple rejected the credentials; playback is paused
	// until the user signs in again and resumes.
	AuthRequired bool `json:"authRequired"`
	// NoCDM: the Widevine CDM isn't ready yet (downloading, or the
	// download failed: "cdm:status" says which); the item waits, paused,
	// until a play succeeds.
	NoCDM bool `json:"noCdm"`
	// AutoPlay: similar music continues when the queue ends.
	AutoPlay bool `json:"autoPlay"`
	// Station feeding the queue, nil when none.
	Station *QueueStation `json:"station"`
	// Container is the collection the current song came from, nil when
	// none.
	Container *QueueContainer `json:"container"`
	// Sleep is the sleep timer, nil when off.
	Sleep *SleepTimer `json:"sleep"`
	// CanUndoReplace: UndoReplace can bring back the queue the last play
	// of a collection or station replaced.
	CanUndoReplace bool `json:"canUndoReplace"`
}

// QueueState is the queue ("playback:queue", sent when Version changes).
type QueueState struct {
	Version uint64      `json:"version"`
	Index   int         `json:"index"`
	Items   []QueueItem `json:"items"`
	// History is what played this session, newest first.
	History []QueueItem `json:"history"`
	// Station feeding the queue (its slots are Auto), nil when none.
	Station *QueueStation `json:"station"`
}
