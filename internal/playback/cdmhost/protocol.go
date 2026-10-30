// Package cdmhost runs protected playback in a child process: the same
// executable re-executed with EnvChild set. The child loads the Widevine CDM,
// decrypts the encrypted fMP4 fragments the parent sends, and plays the clear
// AAC through GStreamer. It makes no network requests, and decrypted audio
// never leaves it. Parent and child talk over the child's stdin/stdout with
// gob-encoded frames: requests with replies, plus child→parent events.
package cdmhost

import (
	"errors"
	"time"
)

// EnvChild marks a process as the CDM host child.
const EnvChild = "SONA_CDMHOST"

// envLogLevel passes the parent's log level to the child.
const envLogLevel = "SONA_CDMHOST_LOG"

// Errors returned by Host calls.
var (
	// ErrStale reports a request for a track generation that a later Load
	// superseded.
	ErrStale = errors.New("cdmhost: track superseded")
	// ErrExited reports that the child process is gone.
	ErrExited = errors.New("cdmhost: host process exited")
	// ErrNoUsableKey reports a licence that left the session without a usable key.
	ErrNoUsableKey = errors.New("cdmhost: licence has no usable key")
)

// Info describes a running host.
type Info struct {
	PID           int
	CDMPath       string
	CDMVersion    string
	CDMInterface  int // Chromium CDM interface version in use
	HostInterface int // host interface version the CDM asked for
	GStreamer     string
	Decoder       string
	Sink          string
}

// EventKind classifies a child event.
type EventKind int

const (
	// EventStarted: audio of Track is playing; Position is exact at At.
	EventStarted EventKind = iota + 1
	// EventPosition: periodic position sample of Track while playing.
	EventPosition
	// EventEnded: Track played out after EndOfTrack.
	EventEnded
	// EventBuffering: playback of Track stalled (Buffering) or resumed.
	EventBuffering
	// EventError: playback of Track failed.
	EventError
	// EventMessage: the CDM has a message (usually a licence renewal) for Session.
	EventMessage
)

// Event is sent by the child.
type Event struct {
	Kind      EventKind
	Track     uint64
	Position  time.Duration
	At        time.Time
	Buffering bool
	Err       string
	Session   string
	Type      int // widevine.MessageType of an EventMessage
	Data      []byte
}

type op string

const (
	opInit         op = "init"
	opCert         op = "cert"
	opSession      op = "session"
	opUpdate       op = "update"
	opCloseSession op = "close-session"
	opLoad         op = "load"
	opFeed         op = "feed"
	opEnd          op = "end"
	opPlay         op = "play"
	opPause        op = "pause"
	opStop         op = "stop"
	opVolume       op = "volume"
	opPosition     op = "position"
)

// request is a parent→child frame.
type request struct {
	ID      uint64
	Op      op
	CDMPath string
	Sink    string // audio sink override for opInit
	Data    []byte // certificate, licence, init segment or fragment
	KID     []byte // opSession: 16-byte key ID or a whole Widevine pssh box; opFeed: key ID override
	Init    []byte        // opFeed: a stream's new init segment, applied before Data
	Shift   time.Duration // opFeed: subtracted from the fragment's timestamps
	Session string
	Track   uint64
	Start   time.Duration
	Volume  float64
}

// reply answers request ID.
type reply struct {
	ID        uint64
	Err       string
	Code      string // "stale", "no-usable-key"
	Info      *Info
	Session   string
	Challenge []byte
	Position  time.Duration
	HasPos    bool
}

// frame is a child→parent frame: exactly one field is set.
type frame struct {
	Reply *reply
	Event *Event
}

const (
	codeStale       = "stale"
	codeNoUsableKey = "no-usable-key"
)
