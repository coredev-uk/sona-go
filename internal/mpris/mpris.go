// Package mpris exports Sona as an MPRIS 2.2 media player
// (org.mpris.MediaPlayer2.sona) on the D-Bus session bus.
//
// The Server reflects a Snapshot pushed through Update and forwards control
// requests (media keys, playerctl, desktop shells) to a Player. It implements
// org.mpris.MediaPlayer2, org.mpris.MediaPlayer2.Player,
// org.freedesktop.DBus.Properties and org.freedesktop.DBus.Introspectable at
// /org/mpris/MediaPlayer2. Track lists and playlists are not exposed.
//
// Spec: https://specifications.freedesktop.org/mpris-spec/latest/
package mpris

import (
	"context"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	// BusName is the well-known name Sona owns while the Server is running.
	BusName = "org.mpris.MediaPlayer2.sona"
	// ObjectPath is where the MPRIS interfaces are exported.
	ObjectPath dbus.ObjectPath = "/org/mpris/MediaPlayer2"
	// Identity is the human-readable player name.
	Identity = "Sona"
	// DesktopEntry is the desktop file id (dev.coredev.Sona.desktop), which
	// matches the GTK application id and so the Wayland app_id.
	DesktopEntry = "dev.coredev.Sona"

	// NoTrack is the mpris:trackid reported when nothing is loaded.
	NoTrack dbus.ObjectPath = "/org/mpris/MediaPlayer2/TrackList/NoTrack"
	// trackPrefix roots Sona's track object paths. /org/mpris is reserved.
	trackPrefix = "/dev/coredev/sona/track/"
)

// PlaybackStatus is the MPRIS PlaybackStatus value.
type PlaybackStatus string

const (
	StatusPlaying PlaybackStatus = "Playing"
	StatusPaused  PlaybackStatus = "Paused"
	StatusStopped PlaybackStatus = "Stopped"
)

// LoopStatus is the MPRIS LoopStatus value.
type LoopStatus string

const (
	LoopNone     LoopStatus = "None"
	LoopTrack    LoopStatus = "Track"
	LoopPlaylist LoopStatus = "Playlist"
)

// Track describes the current item for xesam/mpris metadata.
type Track struct {
	// ID is the Apple Music song ID (catalog "1440..." or library "i.…").
	// It becomes mpris:trackid through TrackPath.
	ID          string
	Title       string
	Artist      string
	Album       string
	AlbumArtist string
	Length      time.Duration
	// ArtURL is the cached artwork as a file:// URL; empty omits mpris:artUrl.
	ArtURL string
	// URL is the public music.apple.com link; empty omits xesam:url.
	URL string
}

// Snapshot is the player state the Server publishes. Position is exact at
// UpdatedAt; while Status is StatusPlaying the Server extrapolates it at query
// time, so callers need not push position ticks.
type Snapshot struct {
	Status    PlaybackStatus // empty means StatusStopped
	Loop      LoopStatus     // empty means LoopNone
	Shuffle   bool
	Volume    float64 // 0..1
	Position  time.Duration
	UpdatedAt time.Time // zero means "now" when Update receives it
	Track     *Track    // nil when nothing is loaded

	CanGoNext     bool
	CanGoPrevious bool
	CanPlay       bool
	CanPause      bool
	CanSeek       bool
	// CanControl false makes every other Can* report false and turns all
	// control methods into no-ops, as the spec requires.
	CanControl bool
}

// Player receives control requests from D-Bus clients. Calls arrive on
// godbus goroutines, possibly concurrently, each with a context bounded by
// the Server's call timeout. The Server applies the spec's Can* gating before
// calling, so a Player only acts. State changes are reported back through
// Server.Update, not through return values.
type Player interface {
	PlayPause(ctx context.Context) error
	Play(ctx context.Context) error
	Pause(ctx context.Context) error
	Stop(ctx context.Context) error
	Next(ctx context.Context) error
	Previous(ctx context.Context) error
	// Seek moves the playhead by offset (may be negative). Per spec, a target
	// before zero seeks to zero and one past the end acts like Next.
	Seek(ctx context.Context, offset time.Duration) error
	// SetPosition seeks to pos within trackID (Track.ID). The Server already
	// dropped requests for a stale track or an out-of-range pos.
	SetPosition(ctx context.Context, trackID string, pos time.Duration) error
	// SetVolume sets the volume, already clamped to 0..1.
	SetVolume(ctx context.Context, v float64) error
	SetLoopStatus(ctx context.Context, l LoopStatus) error
	SetShuffle(ctx context.Context, on bool) error
	// Raise brings the main window to the front.
	Raise(ctx context.Context) error
	// Quit exits the application.
	Quit(ctx context.Context) error
	// Snapshot returns the current state. The Server reads it once in New;
	// afterwards Update is authoritative.
	Snapshot() Snapshot
}

// TrackPath maps a song ID to its mpris:trackid object path. Bytes outside
// [A-Za-z0-9] are escaped as _XX (lowercase hex), so the mapping is
// injective: "i.abc" → /dev/coredev/sona/track/i_2eabc. An empty ID maps to
// NoTrack.
func TrackPath(id string) dbus.ObjectPath {
	if id == "" {
		return NoTrack
	}
	const hex = "0123456789abcdef"
	var b strings.Builder
	b.Grow(len(trackPrefix) + len(id))
	b.WriteString(trackPrefix)
	for i := range len(id) {
		c := id[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
			b.WriteByte(c)
		default:
			b.WriteByte('_')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&0x0f])
		}
	}
	return dbus.ObjectPath(b.String())
}
