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
// error, and Capabilities reports it absent so the UI hides or
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
	// KeepRunning returns the hold that keeps the app at full speed while
	// music plays or loads with no window showing (macOS: App Nap would
	// otherwise throttle the timers, preload and media keys the coordinator
	// keeps). It is not the sleep inhibitor: the computer stays free to
	// sleep, and no setting turns it off. Nil where the system does nothing
	// of the kind, so nothing reports it.
	KeepRunning func(why func() string) RunningKeeper
	// Power opens the power source monitor (UPower on Linux).
	Power func() (PowerMonitor, error)
	// Autostart is the login item.
	Autostart Autostart
	// Files is the native file chooser.
	Files FileChooser
	// LinkHandler is the system's choice of the app that opens Apple Music
	// links (music://, musics://, itms://, itmss://): whether it is Sona
	// and making it Sona. Nil where Sona can't be chosen (Windows).
	LinkHandler LinkHandler
	// Commands: `sona <command>` (Core.Commands) reaches the running
	// instance and a second launch forwards its links to it (internal/app:
	// D-Bus on Linux, a local socket elsewhere).
	Commands bool
	// NeedsAppBundle: this is a macOS run outside Sona.app (a source build,
	// bin/sona). macOS ties notifications and the login item to the app
	// bundle, so Notifications and Autostart are nil for that reason;
	// Capabilities says so and the UI explains it instead of saying the
	// platform lacks them.
	NeedsAppBundle bool
	// PlainNotifications: the system's notifications show the title and
	// text and a click that opens Sona, not the artwork nor the Next
	// button (macOS: MyGo's notifications have neither).
	PlainNotifications bool
	// MotionArtwork: the build decodes motion artwork (internal/ui/media
	// has a decoder: GStreamer on Linux and macOS, none on Windows).
	MotionArtwork bool
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
	// Show replaces the shown notification with n. It fails with
	// ErrNotificationsDenied where the system lets Sona show none.
	Show(ctx context.Context, n notify.Notification) error
	// Dismiss removes it.
	Dismiss(ctx context.Context) error
	// Close stops delivering actions.
	Close()
}

// ErrNotificationsDenied is what a Notifier's Show fails with when the
// system doesn't let Sona show notifications (macOS: the user said no at
// the first one, or turned them off in System Settings). The integrations
// report it as IntegrationDenied, so Settings can say where to turn them on.
var ErrNotificationsDenied = errors.New("notifications are turned off for Sona")

// SleepInhibitor keeps the system awake while held.
type SleepInhibitor interface {
	Acquire(ctx context.Context) error
	Release() error
	// Backend names what holds the inhibitor ("" when none is held).
	Backend() string
	Close() error
}

// RunningKeeper keeps the app from being throttled while held (macOS App
// Nap). Hold and Release are idempotent.
type RunningKeeper interface {
	Hold()
	Release()
	Close()
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

// LinkHandler is the desktop's setting of which app opens Apple Music links
// (app.LinkSchemes). Only an installed Sona can be it: a build the desktop
// doesn't know as an application (a source build, a development bundle, an
// AppImage that isn't integrated) reports errors.ErrUnsupported from both
// methods.
type LinkHandler interface {
	// Default reports whether Sona opens all of the links, and the name of
	// the app that opens them instead when it isn't Sona ("" when none or
	// unknown).
	Default() (sona bool, other string, err error)
	// MakeDefault makes Sona open the links, as far as the desktop lets an
	// app choose; ctx bounds the wait for the desktop's answer.
	MakeDefault(ctx context.Context) error
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
// UI hides or explains what can't work here instead of showing it
// broken.
type PlatformService struct {
	caps     Capabilities
	titleBar windowchrome.Platform
	windows  Windows // nil without a window control (tests)
	layout   sync.Once
}

func newPlatformService(e *Env) *PlatformService {
	p := e.Platform
	pc := e.Engine.Capabilities()
	return &PlatformService{titleBar: p.TitleBar, windows: e.Windows, caps: Capabilities{
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
			Video:            pc.Video,
		},
		MediaControls:      p.MediaControls != nil,
		Notifications:      p.Notifications != nil,
		Autostart:          p.Autostart != nil,
		SleepInhibit:       p.Inhibitor != nil,
		PowerState:         p.Power != nil,
		MotionArtwork:      p.MotionArtwork,
		FileChooser:        p.Files != nil,
		LinkHandler:        p.LinkHandler != nil,
		CLI:                p.Commands,
		NeedsAppBundle:     p.NeedsAppBundle,
		PlainNotifications: p.PlainNotifications,
		DiscordPresence:    discord.Supported,
		DeveloperTools:     e.Windows != nil && e.Windows.DeveloperTools(),
		WindowControls: WindowControls{
			Native:    p.TitleBar.Native,
			Automatic: p.TitleBar.Automatic,
			Desktop:   p.TitleBar.Desktop,
			Style:     e.Settings.Get().TitleBar.Style,
			Start:     []string{},
			End:       []string{},
		},
	}}
}

// Capabilities returns what this platform and build support. It is fixed
// for the process; whether a supported service is reachable right now
// (the notification server, the inhibitor) is in "integrations:status".
// The desktop's window button layout is read on the first call, which
// comes from the UI, once the GUI toolkit runs; the tray is the shell's,
// up from before the first window.
func (s *PlatformService) Capabilities() Capabilities {
	s.layout.Do(func() {
		if s.titleBar.Layout == nil {
			return
		}
		l := s.titleBar.Layout()
		s.caps.WindowControls.Start = append([]string{}, l.Start...)
		s.caps.WindowControls.End = append([]string{}, l.End...)
	})
	caps := s.caps
	caps.Tray = s.windows != nil && s.windows.HasTray()
	return caps
}
