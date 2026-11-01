package coordinator

import (
	"context"
	"time"

	"github.com/coredev-uk/sona-go/internal/playback"
)

// Item is one playable track with the metadata the UI and MPRIS show.
type Item struct {
	ID       string        `json:"id"` // passed to Engine.Play: catalog ("1440…") or library ("i.…") song ID
	Title    string        `json:"title"`
	Artist   string        `json:"artist"`
	Album    string        `json:"album"`
	Duration time.Duration `json:"duration"`
	Artwork  string        `json:"artwork"` // Apple artwork template ({w}x{h})
	// Explicit: Apple rates the song explicit; it is skipped while the
	// account restricts explicit content (SetRestricted).
	Explicit bool `json:"explicit,omitempty"`
	// Container is the album, playlist, artist or station the item was
	// queued from (zero when none), reported with its plays.
	Container Container `json:"container,omitzero"`
}

// ContainerKind is the kind of collection an item was queued from.
type ContainerKind string

const (
	ContainerAlbum    ContainerKind = "album"
	ContainerPlaylist ContainerKind = "playlist"
	ContainerArtist   ContainerKind = "artist"
	ContainerStation  ContainerKind = "station"
)

// Valid reports whether k is a known kind.
func (k ContainerKind) Valid() bool {
	return k == ContainerAlbum || k == ContainerPlaylist || k == ContainerArtist || k == ContainerStation
}

// Container identifies a collection by kind and Apple ID: catalog
// ("1440…", "pl.…", "ra.…") or library ("l.…", "p.…").
type Container struct {
	Kind ContainerKind `json:"kind"`
	ID   string        `json:"id"`
}

// Repeat is the queue repeat mode.
type Repeat string

const (
	RepeatOff Repeat = "off"
	RepeatAll Repeat = "all" // wrap from the last item to the first
	RepeatOne Repeat = "one" // replay the current item when it ends
)

func (r Repeat) valid() bool { return r == RepeatOff || r == RepeatAll || r == RepeatOne }

// Entry is a queue slot. UID is stable while the slot exists (also across
// shuffle and moves), so it suits list keys; the same Item may occupy
// several slots.
type Entry struct {
	Index int    `json:"index"`
	UID   uint64 `json:"uid"`
	Item  Item   `json:"item"`
	// Auto: the slot came from the station (Snapshot.Station), not from
	// the user; upcoming auto slots always follow the user's.
	Auto bool `json:"auto"`
	// Added: the user added the slot by hand (PlayNext, Append, Insert)
	// rather than it coming with the collection SetQueue played.
	Added bool `json:"added"`
}

// Station is the radio station feeding the queue: one the user started
// (PlayStation) or the AutoPlay station continuing after the user's queue.
type Station struct {
	ID       string `json:"id"` // "ra.…"
	Name     string `json:"name"`
	AutoPlay bool   `json:"autoPlay"`
}

// StationSource supplies station tracks. Its methods do network requests;
// the coordinator calls them without holding locks, from its own
// goroutines, with a context that Close cancels.
type StationSource interface {
	// NextTracks returns the station's next batch of items (it advances
	// the station).
	NextTracks(ctx context.Context, stationID string) ([]Item, error)
	// Continue returns the AutoPlay station continuing after seeds (the
	// last items of the user's queue, oldest first, with the containers
	// they were queued from).
	Continue(ctx context.Context, seeds []Item) (Station, error)
}

// Snapshot is the now-playing state for the UI ("playback:state") and MPRIS.
type Snapshot struct {
	State   playback.State `json:"state"`
	Current *Entry         `json:"current"` // nil when nothing is selected
	Index   int            `json:"index"`   // of Current in Queue; -1 when none
	// Position is extrapolated to At; while State is playing, consumers
	// extrapolate further (Position + now - At) instead of polling.
	Position time.Duration `json:"position"`
	Duration time.Duration `json:"duration"`
	At       time.Time     `json:"at"`
	Volume   float64       `json:"volume"`
	Shuffle  bool          `json:"shuffle"`
	Repeat   Repeat        `json:"repeat"`
	Context  string        `json:"context"` // SetQueue label, e.g. album or playlist ID
	Queue    []Entry       `json:"queue"`   // play order
	// QueueVersion changes whenever Queue does, so frequent consumers can
	// skip resending an unchanged queue.
	QueueVersion uint64 `json:"queueVersion"`
	History      []Item `json:"history"` // items played this session, newest first
	// Error describes the latest playback failure; it clears once an item
	// plays. After MaxFailures consecutive failures State is error and
	// playback stops.
	Error string `json:"error,omitempty"`
	// AuthRequired reports that Apple rejected the credentials; playback is
	// paused until the user signs in again and resumes.
	AuthRequired bool `json:"authRequired"`
	// NoCDM reports that the engine has no Widevine CDM ready yet
	// (playback.ErrNoCDM: downloading, or the download failed); the item
	// stays selected and paused until a play succeeds.
	NoCDM bool `json:"noCdm"`
	// Preview: the current item plays as its 30-second preview
	// (playback.Status.Preview); nothing reports it as a play.
	Preview bool `json:"preview"`
	// OnAir is what the current live broadcaster's stream says is on air,
	// as heard (playback.Status.OnAir); zero when it says nothing.
	OnAir playback.OnAir `json:"onAir"`
	// Station is the station feeding the queue (nil when none).
	Station *Station `json:"station"`
	// AutoPlay: when the user's queue ends, similar music keeps playing.
	AutoPlay bool `json:"autoPlay"`
	// Restricted: the account restricts explicit content; explicit items
	// are skipped.
	Restricted bool `json:"restricted"`
	// Sleep is the sleep timer (nil when off).
	Sleep *Sleep `json:"sleep"`
	// CanUndoReplace: UndoReplace can bring back the queue the last
	// SetQueue or PlayStation replaced.
	CanUndoReplace bool `json:"canUndoReplace"`
}

// SleepMode is when the sleep timer stops playback.
type SleepMode string

const (
	SleepOff   SleepMode = ""      // no timer
	SleepAfter SleepMode = "after" // after a duration (Sleep.At)
	SleepTrack SleepMode = "track" // when the current item ends
	SleepQueue SleepMode = "queue" // when the last of the user's items ends
)

// SleepFade is how long the sleep timer fades the volume out before it
// stops playback.
const SleepFade = 10 * time.Second

// Sleep is the sleep timer: playback fades out over the last SleepFade
// and pauses; the volume is restored afterwards.
type Sleep struct {
	Mode SleepMode `json:"mode"`
	At   time.Time `json:"at,omitzero"` // SleepAfter: when playback pauses
}

// Saved is the persisted queue. A restore puts it back paused.
type Saved struct {
	Items     []Item        `json:"items"`              // play order
	Original  []int         `json:"original,omitempty"` // unshuffled order as indices into Items; only with Shuffle
	Index     int           `json:"index"`
	Position  time.Duration `json:"position"`
	PlayedFor time.Duration `json:"playedFor"` // listening time of the current item so far
	Shuffle   bool          `json:"shuffle"`
	Repeat    Repeat        `json:"repeat"`
	Context   string        `json:"context"`
	Volume    float64       `json:"volume"`
	Station   *Station      `json:"station,omitempty"`
	Auto      []int         `json:"auto,omitempty"`  // indices into Items of station slots
	Added     []int         `json:"added,omitempty"` // indices into Items of slots the user added
}

// Store persists the queue across restarts.
type Store interface {
	// LoadQueue returns the saved queue, or nil if there is none.
	LoadQueue(ctx context.Context) (*Saved, error)
	SaveQueue(ctx context.Context, s Saved) error
}

// ActivityKind classifies an Activity.
type ActivityKind int

const (
	// ActivityStart: audio of Item began (or resumed) at Position.
	ActivityStart ActivityKind = iota + 1
	// ActivityEnd: audio of Item stopped at Position for Reason.
	ActivityEnd
	// ActivitySeek: the playing Item jumped from From to Position.
	ActivitySeek
)

// EndReason says why an item stopped playing.
type EndReason string

const (
	EndNatural      EndReason = "natural"       // played to its end
	EndSkipForward  EndReason = "skip-forward"  // Next
	EndSkipBackward EndReason = "skip-backward" // Previous to an earlier item
	EndSelected     EndReason = "selected"      // another item was chosen (Jump, SetQueue, Remove)
	EndPaused       EndReason = "paused"        // Pause, or the engine paused (auth)
	EndStopped      EndReason = "stopped"       // Stop
	EndFailed       EndReason = "failed"        // the engine failed mid-item
	EndExited       EndReason = "exited"        // the coordinator closed while playing
)

// Activity is a listening transition, the input for play-activity
// reporting. Every ActivityStart is followed by exactly one ActivityEnd
// for the same Item; ActivitySeek happens only in between.
type Activity struct {
	Kind     ActivityKind
	Item     Item
	Position time.Duration // start or end position; the seek target
	From     time.Duration // ActivitySeek origin
	Duration time.Duration // the item's duration as playing
	Reason   EndReason     // ActivityEnd
	At       time.Time
	Mode     PlayMode // the queue's modes at the time
}

// AutoPlayMode says whether AutoPlay continues the queue.
type AutoPlayMode int

const (
	AutoPlayOff         AutoPlayMode = iota + 1 // AutoPlay is off
	AutoPlayOn                                  // similar music follows the user's queue
	AutoPlayUnsupported                         // AutoPlay is on, but a station plays (it cannot continue one)
)

// PlayMode is the queue's shuffle, repeat and AutoPlay state.
type PlayMode struct {
	Shuffle  bool
	Repeat   Repeat
	AutoPlay AutoPlayMode
}
