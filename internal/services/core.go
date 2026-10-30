package services

import (
	"context"
	"log/slog"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/coredev-uk/sona-go/internal/app"
	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/artwork"
	"github.com/coredev-uk/sona-go/internal/auth"
	"github.com/coredev-uk/sona-go/internal/catalog"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/library"
	"github.com/coredev-uk/sona-go/internal/motion"
	"github.com/coredev-uk/sona-go/internal/mpris"
	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/coordinator"
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
	Coord     *coordinator.Coordinator
	History   *History
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
	// Bus is the session bus (nil without one), for portals and other
	// desktop services.
	Bus *dbus.Conn

	Version string    // app version
	WebKit  string    // runtime WebKitGTK version
	Started time.Time // process start
}

// Core holds the bound services and the hooks main drives.
type Core struct {
	env      *Env
	services []application.Service
	open     *opener

	mpris     *mpris.Server
	mprisQuit chan struct{}
	mprisDone chan struct{}
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
	env := &Env{Deps: d, m: mapper{art: d.Art, motion: d.Motion}, members: newMembership(d.DB)}
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
	return c
}

// Services returns the Wails services to register, in registry order.
func (c *Core) Services() []application.Service { return c.services }

// firstPaint delivers held app:open links and opens the network.
func (c *Core) firstPaint() {
	c.open.markReady()
	c.env.Gate.Open()
}

// StartMPRIS exports the MPRIS player over the coordinator on srv (built
// with NewMPRISPlayer) and keeps it updated until Close.
func (c *Core) StartMPRIS(srv *mpris.Server) {
	c.mpris = srv
	c.mprisQuit, c.mprisDone = make(chan struct{}), make(chan struct{})
	snaps, cancel := c.env.Coord.Subscribe()
	c.env.Go("mpris", func() {
		defer close(c.mprisDone)
		defer cancel()
		runMPRIS(srv, snaps, c.mprisQuit, c.env.Art, c.env.Logger)
	})
}

// Activate handles a launch: the process's own argv (forwarded false) or
// one forwarded by a second instance.
func (c *Core) Activate(act app.Activation, forwarded bool) { c.open.activate(act, forwarded) }

// Close stops background work (features' OnClose steps, then MPRIS). The
// caller then closes the coordinator, engine and database.
func (c *Core) Close() {
	c.env.close()
	if c.mpris != nil {
		close(c.mprisQuit)
		<-c.mprisDone
		if err := c.mpris.Close(); err != nil {
			c.env.Logger.Warn("mpris close", "err", err)
		}
	}
}
