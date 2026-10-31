package services

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/coredev-uk/sona-go/internal/config"
)

// EventSettingsChanged carries the saved config.Settings after
// UpdateSettings.
const EventSettingsChanged = "settings:changed"

func init() { application.RegisterEvent[config.Settings](EventSettingsChanged) }

// Diagnostics describes the running app for the Diagnostics page.
type Diagnostics struct {
	App      DiagApp      `json:"app"`
	WebKit   DiagWebKit   `json:"webkit"`
	Playback DiagPlayback `json:"playback"`
	Database DiagDatabase `json:"database"`
	Keyring  DiagKeyring  `json:"keyring"`
	Paths    DiagPaths    `json:"paths"`
	// ArtworkCacheBytes is the artwork disk cache's size.
	ArtworkCacheBytes int64 `json:"artworkCacheBytes"`
	// Env lists the environment toggles that change rendering or playback.
	Env []EnvVar `json:"env"`
}

type DiagApp struct {
	Version   string `json:"version"`
	GoVersion string `json:"goVersion"`
	Platform  string `json:"platform"` // e.g. linux/amd64
	Session   string `json:"session"`  // XDG_SESSION_TYPE (wayland, x11)
	PID       int    `json:"pid"`
	// StartupMs is process start → first paint ("" until painted: 0).
	StartupMs int64 `json:"startupMs"`
}

type DiagWebKit struct {
	Version string `json:"version"` // runtime WebKitGTK version, "" if unknown
}

// DiagPlayback is the playback stack as the CDM host child reports it.
type DiagPlayback struct {
	// Engine is the playback engine's kind ("cdm"), or "" when this
	// platform has none: playback is unavailable and the CDM fields stay
	// empty.
	Engine string `json:"engine"`
	// CDM is the Widevine CDM's source, version and download state
	// (CDMService.Status).
	CDM CDMStatus `json:"cdm"`
	// The CDM fields below are the probe child's report of the CDM a new
	// playback host would load (empty with no CDM ready).
	CDMPath       string `json:"cdmPath"`
	CDMVersion    string `json:"cdmVersion"`
	CDMOK         bool   `json:"cdmOk"` // the CDM initialised in a probe child
	CDMError      string `json:"cdmError"`
	CDMInterface  int    `json:"cdmInterface"`
	HostInterface int    `json:"hostInterface"`
	GStreamer     string `json:"gstreamer"`
	Decoder       string `json:"decoder"`
	Sink          string `json:"sink"`
	HostRunning   bool   `json:"hostRunning"` // the playback child is up
	HostPID       int    `json:"hostPid"`
	// AudioQuality is the AAC bitrate Sona plays (kbps, the setting).
	// Sona never plays lossless or Dolby Atmos: Apple refuses those keys
	// to the desktop Widevine CDM.
	AudioQuality int `json:"audioQuality"`
	// SandboxKind is how the playback child is confined: "linux" (its own
	// network and mount namespace, Landlock, the seccomp filter that
	// refuses network sockets), "seatbelt" (macOS: sandbox-exec with
	// Sona's profile: no network, no access to Sona's folder or the
	// keychain), "none" (macOS without a working sandbox-exec: the child
	// runs unconfined) or "" when the probe child never started.
	SandboxKind string `json:"sandboxKind"`
	// Sandboxed: every layer of SandboxKind is in place. SandboxNote says
	// what is missing otherwise ("" when the probe child never started).
	Sandboxed   bool   `json:"sandboxed"`
	SandboxNote string `json:"sandboxNote"`
	// CDMBlocked: the child refused to load the CDM because its seccomp
	// filter is missing (cdmhost.ErrSandbox, Linux only); SandboxNote says
	// why.
	CDMBlocked bool `json:"cdmBlocked"`
}

type DiagDatabase struct {
	Path          string `json:"path"`
	SizeBytes     int64  `json:"sizeBytes"` // including the WAL
	SchemaVersion int64  `json:"schemaVersion"`
	Counts        Counts `json:"counts"`
	LastSync      int64  `json:"lastSync"` // complete library sync, Unix ms
}

type DiagKeyring struct {
	Backend string `json:"backend"`
	// Unencrypted: the token is in a 0600 file because Secret Service is
	// unavailable. The UI must show a warning.
	Unencrypted bool `json:"unencrypted"`
}

type DiagPaths struct {
	Config   string `json:"config"`
	Data     string `json:"data"`
	Cache    string `json:"cache"`
	Art      string `json:"art"`
	State    string `json:"state"`
	Settings string `json:"settings"`
	LogFile  string `json:"logFile"`
	Queue    string `json:"queue"`
}

// EnvVar is an environment toggle and its value in this process.
type EnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Set   bool   `json:"set"`
}

// DiagBundle is the redacted diagnostics bundle for a bug report.
type DiagBundle struct {
	Text string `json:"text"`
	// Path is where the bundle was saved ("" if saving failed).
	Path string `json:"path"`
}

// SettingsImport reports an ImportSettings call.
type SettingsImport struct {
	// Path is the imported file ("" when the user cancelled).
	Path string `json:"path"`
	// Changed lists the settings the file changed, as dotted settings
	// names ("theme", "desktop.closeAction"); empty when they all
	// matched already.
	Changed []string `json:"changed"`
}
