package services

import (
	"context"
	"log/slog"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/coredev-uk/sona-go/internal/app"
	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/artwork"
	"github.com/coredev-uk/sona-go/internal/auth"
	"github.com/coredev-uk/sona-go/internal/catalog"
	"github.com/coredev-uk/sona-go/internal/cli"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/i18n/applestrings"
	"github.com/coredev-uk/sona-go/internal/library"
	"github.com/coredev-uk/sona-go/internal/motion"
	"github.com/coredev-uk/sona-go/internal/mpris"
	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/cdm"
	"github.com/coredev-uk/sona-go/internal/playback/coordinator"
	"github.com/coredev-uk/sona-go/internal/privatemode"
)

// Deps are the core components main builds and the services wrap.
type Deps struct {
	Paths     config.Paths
	QueueFile string
	Settings  *config.Store
	DB        *db.DB
	Auth      *auth.Manager
	API       *applemusic.Client
	Catalog   *catalog.Catalog
	Library   *library.Library
	Syncer    *library.Syncer
	Art       *artwork.Cache
	Motion    *motion.Cache // motion artwork (nil: none offered)
	Engine    playback.Engine
	// CDM keeps the CDM engine's Widevine CDM installed (nil: the engine
	// needs none).
	CDM *cdm.Manager
	// AppleStrings keeps Apple Music's translations of Sona's messages
	// (nil: none served).
	AppleStrings *applestrings.Store
	Coord        *coordinator.Coordinator
	History      *History
	// Private is private listening, which every play reporter asks
	// (required).
	Private *privatemode.Mode
	// Storefront is the catalog storefront in use (the account's, or the
	// locale's when signed out), for direct applemusic calls.
	Storefront func() string
	// Gate holds network requests until the first paint.
	Gate   *Gate
	Emit   Emitter
	Logger *slog.Logger
	Now    func() time.Time // default time.Now

	OpenURL func(string) error // system browser / file manager
	Focus   func()             // raise the main window
	// Focused reports whether the main window is shown and focused.
	Focused func() bool
	// Windows is the desktop shell's window control (internal/shell).
	Windows Windows
	// Platform is the OS integrations (main's platform_<os>.go); nil
	// fields are unsupported here.
	Platform Platform

	Version string    // app version
	WebKit  string    // runtime WebKitGTK version
	Started time.Time // process start
}

// Core holds the bound services and the hooks main drives.
type Core struct {
	env      *Env
	services []application.Service
	open     *opener
	commands *commandServer

	media     MediaControls
	mediaQuit chan struct{}
	mediaDone chan struct{}
}

// New builds every registered feature (registry.go). Register Services()
// with the application.
func New(d Deps) *Core {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Logger == nil {
		d.Logger = slog.New(slog.DiscardHandler)
	}
	env := &Env{Deps: d, m: mapper{art: d.Art, motion: d.Motion, now: d.Now}, members: newMembership(d.DB)}
	c := &Core{env: env, open: &opener{emit: d.Emit, focus: d.Focus, log: d.Logger}}
	env.firstPaint = c.firstPaint
	env.OnNetwork(func() { d.Logger.Info("startup: network open") })
	for _, f := range features {
		c.services = append(c.services, f.build(env))
	}
	// After every feature's sign-out step: drop everything tied to the
	// user. Artwork and the catalog cache are storefront data and stay.
	d.Auth.OnSignOut("user data", func(ctx context.Context) error {
		env.members.clear()
		return d.DB.WipeUserData(ctx)
	})
	c.commands = newCommandServer(env, c.open, c.services)
	return c
}

// Services returns the Wails services to register, in registry order.
func (c *Core) Services() []application.Service { return c.services }

// firstPaint delivers held app:open links and opens the network.
func (c *Core) firstPaint() {
	c.open.markReady()
	c.env.Gate.Open()
}

// StartMediaControls publishes the coordinator through the platform's
// media controls (MPRIS on Linux), with player (NewMPRISPlayer) taking
// their commands, and keeps them updated until Close. It does nothing
// where the platform has none, and logs when they can't start.
func (c *Core) StartMediaControls(player mpris.Player) {
	if c.env.Platform.MediaControls == nil {
		return
	}
	mc, err := c.env.Platform.MediaControls(player)
	if err != nil {
		c.env.Logger.Warn("media controls unavailable", "err", err)
		return
	}
	c.media = mc
	c.mediaQuit, c.mediaDone = make(chan struct{}), make(chan struct{})
	snaps, cancel := c.env.Coord.Subscribe()
	c.env.Go("media controls", func() {
		defer close(c.mediaDone)
		defer cancel()
		runMediaControls(mc, snaps, c.mediaQuit, c.env.Art, c.env.Logger)
	})
}

// Activate handles a launch: the process's own argv (forwarded false) or
// one forwarded by a second instance.
func (c *Core) Activate(act app.Activation, forwarded bool) { c.open.activate(act, forwarded) }

// Commands answers `sona <command>` (internal/cli) for the platform's
// command transport.
func (c *Core) Commands() cli.Server { return c.commands }

// Close stops background work (features' OnClose steps, then the media
// controls). The caller then closes the coordinator, engine and database.
func (c *Core) Close() {
	c.env.close()
	if c.media != nil {
		close(c.mediaQuit)
		<-c.mediaDone
		if err := c.media.Close(); err != nil {
			c.env.Logger.Warn("media controls close", "err", err)
		}
	}
}
