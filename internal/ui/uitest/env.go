// Package uitest is the seam of the native UI's tests: Env builds the
// module.Env a feature or the shell is tested over, with real services on a
// temporary SQLite library seeded with the synthetic fixture
// (uitest/fixture), the unsupported-engine-free fake playback engine, a fake
// artwork CDN that serves generated covers, a Poster that runs where the
// test says, and a fake clock. Tester wraps ui.NewTester for a view over
// it, and Golden compares a frame with an image kept in testdata.
//
// The Core it builds is hermetic (hermetic.go): an empty in-memory keyring,
// an HTTP transport that serves only the fixtures' hosts and fails any other
// request at once, and the home, XDG and D-Bus environment under the test's
// temporary directory.
//
// The tests that use it run without a display, on MyGo's CPU renderer
// (Env sets MYGO_GPU=0), with go test.
package uitest

import (
	"context"
	"log/slog"
	"net/http"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/artwork"
	"github.com/coredev-uk/sona-go/internal/auth"
	"github.com/coredev-uk/sona-go/internal/auth/devtoken"
	authstore "github.com/coredev-uk/sona-go/internal/auth/store"
	"github.com/coredev-uk/sona-go/internal/catalog"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/db"
	ilib "github.com/coredev-uk/sona-go/internal/library"
	"github.com/coredev-uk/sona-go/internal/playback/coordinator"
	"github.com/coredev-uk/sona-go/internal/playback/fakeengine"
	"github.com/coredev-uk/sona-go/internal/privatemode"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/fixture"
	"github.com/coredev-uk/sona-go/internal/windowchrome"
)

// Epoch is where a Clock starts.
var Epoch = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// Option changes what Env builds.
type Option func(*options)

type options struct {
	albums    []fixture.Album
	playlists []fixture.Playlist
	seed      bool
	core      bool
	dark      bool
	apple     http.Handler
	version   string
}

// Albums seeds the library with these albums instead of the fixture's
// (and no playlists): album i is added i+1 days after the epoch.
func Albums(a ...fixture.Album) Option {
	return func(o *options) { o.albums, o.playlists = a, nil }
}

// Empty leaves the library empty: a signed-out library with nothing to show.
func Empty() Option { return func(o *options) { o.seed = false } }

// Bare builds an Env without services: no Core, no library, no engine. It
// has the Bus, Queries, the playback mirror, the artwork loader (with no
// source: every cover fails) and the Post of a Poster that runs where the
// test says. What a feature's view needs when it only reads the mirror.
func Bare() Option { return func(o *options) { o.core = false } }

// Dark runs the theme dark (it is light otherwise).
func Dark() Option { return func(o *options) { o.dark = true } }

// Version sets the app version the Core and the Env report ("test" by
// default, which the update check skips as unreleased).
func Version(v string) Option { return func(o *options) { o.version = v } }

// Harness is everything Env wired, for what a test must reach: the
// database to change, the engine to ask what it played, the clock.
type Harness struct {
	Env      *module.Env
	Core     *services.Core // nil with Bare
	DB       *db.DB         // nil with Bare
	Settings *config.Store  // nil with Bare
	Bus      *store.Bus
	Engine   *fakeengine.Engine // nil with Bare
	Coord    *coordinator.Coordinator
	Art      *artwork.Cache // nil with Bare
	Covers   *fixture.CDN   // what the artwork cache's HTTP reaches
	Clock    *Clock
	Main     *MainThread
	// Network is every HTTP client's transport: it serves the artwork CDN
	// (Covers) and what a test adds with Serve, and fails any other request.
	Network *Network
	// Keyring is the Core's secret store, empty: the machine's is not reachable.
	Keyring *Keyring
	Dir     string

	linkMu sync.Mutex
	opened []string
}

func (h *Harness) open(u string) error {
	h.linkMu.Lock()
	defer h.linkMu.Unlock()
	h.opened = append(h.opened, u)
	return nil
}

// Opened is every URL the UI asked the system browser to open (Env.OpenURL).
func (h *Harness) Opened() []string {
	h.linkMu.Lock()
	defer h.linkMu.Unlock()
	return append([]string(nil), h.opened...)
}

var (
	mu        sync.Mutex
	harnesses = map[*module.Env]*Harness{}
)

// Of is the Harness that built env, nil for any other Env.
func Of(env *module.Env) *Harness {
	mu.Lock()
	defer mu.Unlock()
	return harnesses[env]
}

// Env builds the environment of a test: see New.
func Env(t testing.TB, opts ...Option) *module.Env { return New(t, opts...).Env }

// New builds the Harness of a test, its resources released when t ends. The
// theme is light (Dark: dark) and returns to the system's afterwards.
func New(t testing.TB, opts ...Option) *Harness {
	t.Helper()
	t.Setenv("MYGO_GPU", "0")
	// English, whatever the machine says: text and numbers are asserted.
	t.Setenv("LC_ALL", "en_US.UTF-8")
	t.Setenv("LANGUAGE", "")
	o := options{albums: fixture.Albums(), playlists: fixture.Playlists(), seed: true, core: true, version: "test"}
	for _, f := range opts {
		f(&o)
	}
	if o.dark {
		theme.SetMode(theme.ModeDark)
	} else {
		theme.SetMode(theme.ModeLight)
	}
	t.Cleanup(func() { theme.SetMode(theme.ModeSystem) })

	h := &Harness{Clock: NewClock(), Main: &MainThread{}, Covers: &fixture.CDN{}, Dir: t.TempDir()}
	isolate(t, filepath.Join(h.Dir, "env"))
	h.Network, h.Keyring = NewNetwork(), NewKeyring()
	h.Network.Serve(fixture.CDNHost, h.Covers)
	denyDefaultTransport(t, h.Network)
	h.Bus = store.NewBus(nil)
	h.Bus.SetPoster(h.Main.Post)
	var art *artwork.Cache
	if o.core {
		art = h.buildCore(t, &o)
	}
	loader := kit.NewArtLoader(art, h.Main.Post) // a nil cache has no cover: every one fails
	t.Cleanup(func() { loader.Close(); loader.Wait() })
	h.Env = &module.Env{
		Core: h.Core, Bus: h.Bus, Queries: store.NewClient(h.Main.Post), Play: store.NewPlayback(h.Bus, h.Clock.Now),
		Art: loader, Actions: kit.NewActionRegistry(), Players: module.NewPlayers(h.Core, h.Clock.Now), Post: h.Main.Post,
		Surfaces: store.NewSurfaces(), OpenURL: h.open, Log: slog.New(slog.DiscardHandler), Version: o.version,
	}
	mu.Lock()
	harnesses[h.Env] = h
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		delete(harnesses, h.Env)
		mu.Unlock()
	})
	return h
}

// buildCore opens and seeds the library and starts the services over it.
func (h *Harness) buildCore(t testing.TB, o *options) *artwork.Cache {
	t.Helper()
	ctx := t.Context()
	d, err := db.Open(ctx, filepath.Join(h.Dir, "sona.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if o.seed {
		if err := fixture.Seed(ctx, d, o.albums, o.playlists); err != nil {
			t.Fatal(err)
		}
	}
	art, err := artwork.New(artwork.Options{Dir: filepath.Join(h.Dir, "art"), HTTP: h.Network.Client()})
	if err != nil {
		t.Fatal(err)
	}
	// Runs after everything registered later (the loader's workers are gone
	// by then) and before the temporary directory is removed: a cover fetch
	// whose caller was cancelled may still be writing into it.
	t.Cleanup(art.Wait)
	settings, err := config.OpenStore(filepath.Join(h.Dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := settings.Update(func(s *config.Settings) {
		s.AutoPlay = false // a queue ends: nothing is fetched from Apple
		s.Language = "en"
		s.Theme = config.ThemeLight
		if o.dark {
			s.Theme = config.ThemeDark
		}
	}); err != nil {
		t.Fatal(err)
	}
	acfg := auth.Config{
		HTTP: h.Network.Client(), Dev: staticDev{}, APIBase: "http://127.0.0.1:1",
		Open:     func() (authstore.Store, error) { return noAccount{}, nil },
		Accounts: services.AccountStore{DB: d},
	}
	if o.apple != nil {
		h.serveApple(o.apple)
		acfg.APIBase = applemusic.BaseURL
		acfg.Open = func() (authstore.Store, error) { return signedIn{}, nil }
	}
	authm, err := auth.NewManager(acfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(authm.Close)
	var (
		api *applemusic.Client
		cat *catalog.Catalog
	)
	if o.apple != nil {
		api = applemusic.New(h.Network.Client(), staticDev{}, authm.UserToken)
		cat = catalog.New(d, api, appleStorefront(true), slog.New(slog.DiscardHandler))
	}
	h.Engine = fakeengine.New()
	h.Engine.Now = h.Clock.Now
	h.Coord, err = coordinator.New(coordinator.Options{Engine: h.Engine, Now: h.Clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Coord.Close() })
	core := services.New(services.Deps{
		Paths:    config.Paths{Config: h.Dir, Data: h.Dir, Cache: h.Dir, Art: h.Dir, State: h.Dir},
		Settings: settings, DB: d, Auth: authm, API: api, Catalog: cat, Storefront: appleStorefront(o.apple != nil), Library: ilib.New(d, nil), Art: art, Engine: h.Engine, Coord: h.Coord,
		Private: privatemode.New(privatemode.Options{}), Gate: services.NewGate(time.Hour, nil),
		Keyring: h.Keyring, Transport: h.Network,
		Emit: h.Bus, Windows: noWindows{}, Logger: slog.New(slog.DiscardHandler), Version: o.version, Started: Epoch,
	})
	t.Cleanup(core.Close)
	authm.Start()
	h.Core, h.DB, h.Settings, h.Art = core, d, settings, art
	return art
}

// staticDev is a developer token source that never touches Apple.
type staticDev struct{}

func (staticDev) Token(context.Context) (devtoken.Token, error) {
	return devtoken.Token{JWT: "dev", Expires: time.Now().Add(time.Hour)}, nil
}
func (staticDev) Invalidate() {}

// noAccount is a keyring holding no sign-in.
type noAccount struct{}

func (noAccount) Get() (authstore.Record, error) { return authstore.Record{}, authstore.ErrNotFound }
func (noAccount) Set(authstore.Record) error     { return nil }
func (noAccount) Delete() error                  { return nil }
func (noAccount) Backend() authstore.Backend     { return authstore.BackendFile }

// noWindows is a window control with no window: the services only follow it.
type noWindows struct{ services.Windows }

func (noWindows) WatchMain(func(windowchrome.WindowState)) {}
func (noWindows) WatchNavigate(func(bool))                 {}
func (noWindows) DeveloperTools() bool                     { return false }
