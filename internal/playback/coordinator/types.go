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
	// last items of the user's queue, oldest first); queueContext is the
	// SetQueue label they came from.
	Continue(ctx context.Context, seeds []Item, queueContext string) (Station, error)
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
	// Station is the station feeding the queue (nil when none).
	Station *Station `json:"station"`
	// AutoPlay: when the user's queue ends, similar music keeps playing.
	AutoPlay bool `json:"autoPlay"`
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
	Auto      []int         `json:"auto,omitempty"` // indices into Items of station slots
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
}
