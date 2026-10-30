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

// ClearResult reports what ClearCache removed.
type ClearResult struct {
	ArtworkBytes   int64 `json:"artworkBytes"`
	CatalogEntries int64 `json:"catalogEntries"`
}
