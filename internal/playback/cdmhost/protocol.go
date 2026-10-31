//go:build linux || (darwin && cgo)

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

	"github.com/coredev-uk/sona-go/internal/playback/equalizer"
)

// EnvChild marks a process as the CDM host child.
const EnvChild = "SONA_CDMHOST"

// envLogLevel passes the parent's log level to the child.
const envLogLevel = "SONA_CDMHOST_LOG"

// envCDMDir passes the directory of the CDM library the parent will load
// to the child, so its filesystem view keeps it reachable and read-only
// (sandbox_fs.go) wherever it lies.
const envCDMDir = "SONA_CDMHOST_CDM_DIR"

// envCDMOnly starts a child that loads the CDM and nothing else (the
// load check, LoadCheck): it never initialises GStreamer, whose plugin
// scan on a first run takes far longer than loading the CDM and says
// nothing about it.
const envCDMOnly = "SONA_CDMHOST_CDM_ONLY"

// Errors returned by Host calls.
var (
	// ErrStale reports a request for a track generation that a later Load
	// superseded.
	ErrStale = errors.New("cdmhost: track superseded")
	// ErrExited reports that the child process is gone.
	ErrExited = errors.New("cdmhost: host process exited")
	// ErrNoUsableKey reports a licence that left the session without a usable key.
	ErrNoUsableKey = errors.New("cdmhost: licence has no usable key")
	// ErrEnded reports a Next that came too late: the current track has
	// already played out (an EventEnded follows) or started fading.
	ErrEnded = errors.New("cdmhost: the current track has ended")
	// ErrSandbox reports that the child did not load the CDM because its
	// seccomp filter is missing or does not keep it off the network (see
	// Sandbox).
	ErrSandbox = errors.New("cdmhost: the playback sandbox could not be set up, so the Widevine CDM was not loaded")
	// ErrCDMLoad reports that the Widevine CDM library is missing or
	// failed to load or initialise in the child.
	ErrCDMLoad = errors.New("cdmhost: the Widevine CDM failed to load")
)

// Media is a track for Load and Next.
type Media struct {
	Init []byte // fMP4 init segment; nil on Load keeps the current one (a seek)
	// Duration is the track's length. When known, the encoder's priming
	// and padding around it are trimmed, so consecutive tracks join
	// sample-exact; 0 plays every sample (live streams).
	Duration time.Duration
	Gain     float64 // Sound Check gain (linear, 0 = unity), applied while SetTrackGain is on
}

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
	Sandbox       Sandbox // how the child is confined
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
	// EventAdvance: the track queued with Next is now heard: the previous
	// one played out (or started crossfading into it) with no gap. Track
	// is the new current track, Position exact at At.
	EventAdvance
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
	// Duration is the length of a library upload's file, reported with
	// its EventStarted or, once known, an EventPosition (0 for HLS
	// tracks, whose length the parent knows).
	Duration time.Duration
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
	opEqualizer    op = "equalizer"
	opDevice       op = "device"
	opLoadFile     op = "load-file"
	opFileData     op = "file-data"
	opFileEnd      op = "file-end"
	opTrackGain    op = "track-gain"
	opNext         op = "next"
	opAudio        op = "audio"
)

// request is a parent→child frame.
type request struct {
	ID      uint64
	Op      op
	CDMPath string
	Sink    string        // audio sink override for opInit
	Data    []byte        // certificate, licence, init segment or fragment
	KID     []byte        // opSession: 16-byte key ID or a whole Widevine pssh box; opFeed: key ID override
	Init    []byte        // opFeed: a stream's new init segment, applied before Data
	Shift   time.Duration // opFeed: subtracted from the fragment's timestamps
	Session string
	Track   uint64
	Start   time.Duration
	Volume  float64
	EQ      *equalizer.Curve // opInit, opEqualizer
	Device  string           // opInit, opDevice: PipeWire sink node.name; "" follows the default
	Size    int64            // opLoadFile: the file's length in bytes
	Offset  int64            // opFileData: where Data goes in the file
	Gain    float64          // opLoad, opLoadFile, opNext: the track's Sound Check gain (linear; 0 = unity)
	Enabled bool             // opTrackGain: apply tracks' Sound Check gain
	// opLoad, opNext: the track's duration (see Media); opNext: the
	// crossfade into it, 0 for a gapless join.
	Duration time.Duration
	Fade     time.Duration
}

// reply answers request ID.
type reply struct {
	ID        uint64
	Err       string
	Code      string // "stale", "no-usable-key", "ended", "sandbox", "cdm-load"
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
	codeEnded       = "ended"
	codeSandbox     = "sandbox"
	codeCDMLoad     = "cdm-load"
)
