package playback

import (
	"errors"
	"fmt"
)

// KindCDM names the Widevine CDM host + GStreamer engine
// (internal/playback/native) in Capabilities.Kind.
const KindCDM = "cdm"

// Capabilities is what an engine supports beyond playing, for the
// frontend's capability list.
type Capabilities struct {
	Kind         string // KindCDM, or "" for no engine (Unsupported)
	Volume       bool   // SetVolume changes the level
	Equalizer    bool   // the engine applies an equalizer.Curve
	SoundCheck   bool   // the engine applies tracks' Sound Check gain
	Crossfade    bool   // the engine crossfades into an armed Next
	OutputDevice bool   // the engine plays to a chosen OutputDevice
	// PrivateListening: Sona itself sends every play report for this
	// engine, so private listening can withhold them.
	PrivateListening bool
}

// ErrUnsupported reports that this platform or build has no playback
// engine. It wraps errors.ErrUnsupported.
var ErrUnsupported = fmt.Errorf("playback is not available on this platform yet: %w", errors.ErrUnsupported)

// ErrNoCDM reports that the engine has no Widevine CDM ready to load: Sona
// is still downloading it, or the download failed (internal/playback/cdm
// says why). Nothing can play until it is ready; the player keeps the item
// and waits for the user to try again.
var ErrNoCDM = errors.New("playback: no Widevine CDM is ready")

// OutputDevice is an audio output an engine can play to.
type OutputDevice struct {
	Name        string // the engine's target id (PipeWire node.name)
	Description string // human-readable name
	Default     bool   // the system's default output
}

// Diagnosis is an engine's check of its playback stack, shown in
// Diagnostics. Fields an engine doesn't have stay empty.
type Diagnosis struct {
	HostRunning bool // a playback host process is running
	HostPID     int

	CDMPath       string
	CDMVersion    string
	CDMInterface  int // Chromium CDM interface version in use
	HostInterface int // host interface version the CDM asked for

	GStreamer string // version
	Decoder   string // AAC decoder element
	Sink      string // audio sink element

	// Sandbox is how the host is confined: SandboxLinux (namespaces,
	// Landlock, seccomp), SandboxSeatbelt (macOS sandbox-exec) or
	// SandboxNone (unconfined, macOS only); "" when no host reported.
	Sandbox     string
	Sandboxed   bool   // every layer of Sandbox is in place
	SandboxNote string // why a layer is missing
	// Blocked: the sandbox could not be set up, so the CDM was not loaded.
	Blocked bool

	// Realtime is the scheduling the running host's audio threads got:
	// RealtimeGranted, RealtimeRaised, RealtimeDenied, RealtimeUnavailable
	// or RealtimeUnsupported; "" while no host has started its output.
	Realtime         string
	RealtimeVia      string // who answered: "RTKit" or "portal"
	RealtimePriority int    // the SCHED_RR priority (granted) or nice level (raised)
	RealtimeNote     string // why realtime was refused or is unavailable
}

// Diagnosis.Sandbox values.
const (
	SandboxLinux    = "linux"
	SandboxSeatbelt = "seatbelt"
	SandboxNone     = "none"
)

// Diagnosis.Realtime values.
const (
	RealtimeGranted     = "granted"     // realtime scheduling (SCHED_RR)
	RealtimeRaised      = "raised"      // realtime refused; a raised nice level instead
	RealtimeDenied      = "denied"      // both refused
	RealtimeUnavailable = "unavailable" // neither RTKit nor the Realtime portal answered
	RealtimeUnsupported = "unsupported" // not asked for on this platform
)
