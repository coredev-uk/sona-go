package services

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"

	"github.com/coredev-uk/sona-go/internal/app"
	"github.com/coredev-uk/sona-go/internal/desktop/notify"
	"github.com/coredev-uk/sona-go/internal/discord"
	"github.com/coredev-uk/sona-go/internal/mpris"
	"github.com/coredev-uk/sona-go/internal/power"
	"github.com/coredev-uk/sona-go/internal/windowchrome"
)

// Platform is what the operating system offers Sona beyond its windows,
// built by main's platform_<os>.go. A nil field means this platform (or
// this session: Linux without a session bus) has no implementation: the
// feature using it does nothing or fails with an errors.ErrUnsupported
// error, and Capabilities reports it absent so the frontend hides or
// explains the control.
type Platform struct {
	// MediaControls publishes now-playing and takes transport commands
	// from the desktop (MPRIS on Linux); Core.StartMediaControls calls it
	// once with the coordinator's player.
	MediaControls func(mpris.Player) (MediaControls, error)
	// Notifications connects to the notification server; onAction
	// receives a clicked action's key (notify.DefaultAction for the body).
	// It is called again after a failure (the server may start later).
	Notifications func(ctx context.Context, onAction func(key string)) (Notifier, error)
	// Inhibitor returns the sleep inhibitor held while music plays; why
	// is the reason it shows, in the user's language when it is taken.
	Inhibitor func(why func() string) SleepInhibitor
	// Power opens the power source monitor (UPower on Linux).
	Power func() (PowerMonitor, error)
	// Autostart is the login item.
	Autostart Autostart
	// Files is the native file chooser.
	Files FileChooser
	// Tray: the shell shows a tray icon (close to the tray is possible).
	Tray bool
	// Commands: `sona <command>` (Core.Commands) reaches the running
	// instance and a second launch forwards its links to it (internal/app:
	// D-Bus on Linux, a local socket elsewhere).
	Commands bool
	// TitleBar is how the main window's title bar works here: the system's
	// buttons inset into Sona's top bar (macOS), or Sona's own title bar
	// or the desktop's (Windows, Linux), and the desktop's button layout
	// (internal/windowchrome).
	TitleBar windowchrome.Platform
}

// MediaControls is the desktop's now-playing and media-key integration.
type MediaControls interface {
	// Update publishes the playback state; the implementation diffs it.
	Update(mpris.Snapshot)
	Close() error
}

// Notifier shows one replaceable notification: the track notification, or
// one release reminder (each reminder has its own).
type Notifier interface {
	// Show replaces the shown notification with n.
	Show(ctx context.Context, n notify.Notification) error
	// Dismiss removes it.
	Dismiss(ctx context.Context) error
	// Close stops delivering actions.
	Close()
}

// SleepInhibitor keeps the system awake while held.
type SleepInhibitor interface {
	Acquire(ctx context.Context) error
	Release() error
	// Backend names what holds the inhibitor ("" when none is held).
	Backend() string
	Close() error
}

// PowerMonitor follows the computer's power source.
type PowerMonitor interface {
	// State is the power source now and how it was decided.
	State() power.State
	// Run calls fn with each change until Close.
	Run(fn func(power.State))
	Close() error
}

// Autostart is the entry that starts Sona at login.
type Autostart interface {
	// Enabled reports whether the entry exists and is enabled.
	Enabled() (bool, error)
	// Enable writes (or rewrites) the entry to start this build at login,
	// with no window when hidden.
	Enable(hidden bool) error
	// Disable removes the entry; a missing entry is not an error.
	Disable() error
}

// FileChooser is the desktop's file dialog. Both return "" without an
// error when the user cancels.
type FileChooser interface {
	OpenFile(ctx context.Context, opts app.OpenFileOptions) (string, error)
	SaveFile(ctx context.Context, opts app.SaveFileOptions) (string, error)
}

// unsupported is the error for a Platform feature this platform or session
// lacks; it wraps errors.ErrUnsupported (CodeUnsupported).
func unsupported(what string) error {
	return fmt.Errorf("%s isn't available on this system: %w", what, errors.ErrUnsupported)
}

// PlatformService reports what this platform and build support, so the
// frontend hides or explains what can't work here instead of showing it
// broken.
type PlatformService struct {
	caps     Capabilities
	titleBar windowchrome.Platform
	layout   sync.Once
}

func newPlatformService(e *Env) *PlatformService {
	p := e.Platform
	pc := e.Engine.Capabilities()
	return &PlatformService{titleBar: p.TitleBar, caps: Capabilities{
		OS: runtime.GOOS,
		Playback: PlaybackCapabilities{
			Available:        pc.Kind != "",
			Engine:           pc.Kind,
			Volume:           pc.Volume,
			Equalizer:        pc.Equalizer,
			SoundCheck:       pc.SoundCheck,
			Crossfade:        pc.Crossfade,
			OutputDevice:     pc.OutputDevice,
			PrivateListening: pc.PrivateListening,
		},
		MediaControls:   p.MediaControls != nil,
		Notifications:   p.Notifications != nil,
		Tray:            p.Tray,
		Autostart:       p.Autostart != nil,
		SleepInhibit:    p.Inhibitor != nil,
		PowerState:      p.Power != nil,
		FileChooser:     p.Files != nil,
		CLI:             p.Commands,
		DiscordPresence: discord.Supported,
		WebKitGTK:       e.WebKit != "",
		WindowControls: WindowControls{
			Native:     p.TitleBar.Native,
			Automatic:  p.TitleBar.Automatic,
			Desktop:    p.TitleBar.Desktop,
			Style:      e.Settings.Get().TitleBar.Style,
			WindowMenu: p.TitleBar.WindowMenu,
			Start:      []string{},
			End:        []string{},
		},
	}}
}

// Capabilities returns what this platform and build support. It is fixed
// for the process; whether a supported service is reachable right now
// (the notification server, the inhibitor) is in "integrations:status".
// The desktop's window button layout is read on the first call, which
// comes from the page, once the GUI toolkit runs.
func (s *PlatformService) Capabilities() Capabilities {
	s.layout.Do(func() {
		if s.titleBar.Layout == nil {
			return
		}
		l := s.titleBar.Layout()
		s.caps.WindowControls.Start = append([]string{}, l.Start...)
		s.caps.WindowControls.End = append([]string{}, l.End...)
	})
	return s.caps
}
