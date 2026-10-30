// Command sona is the Sona desktop app: a Wails v3 shell (GTK4 + WebKitGTK 6
// on Linux) around a React frontend, with all Apple Music networking,
// credentials, caching and playback in Go.
package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/coredev-uk/sona-go/internal/app"
	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/artwork"
	"github.com/coredev-uk/sona-go/internal/auth"
	"github.com/coredev-uk/sona-go/internal/auth/devtoken"
	"github.com/coredev-uk/sona-go/internal/auth/store"
	"github.com/coredev-uk/sona-go/internal/catalog"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/diag"
	"github.com/coredev-uk/sona-go/internal/library"
	"github.com/coredev-uk/sona-go/internal/logging"
	"github.com/coredev-uk/sona-go/internal/motion"
	"github.com/coredev-uk/sona-go/internal/mpris"
	"github.com/coredev-uk/sona-go/internal/playback/activity"
	"github.com/coredev-uk/sona-go/internal/playback/cdmhost"
	"github.com/coredev-uk/sona-go/internal/playback/coordinator"
	"github.com/coredev-uk/sona-go/internal/playback/native"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/shell"
)

// The production frontend build. `go tool wails3 build` (or
// `pnpm -C frontend build`) must run before `go build`.
//
//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

// The tray icon: a small PNG, since StatusNotifierItem sends pixels over
// D-Bus.
//
//go:embed build/linux/icons/hicolor/64x64/apps/sona.png
var trayIcon []byte

// AppID is the GTK application id. GTK4 also uses it as the Wayland app_id,
// so the desktop file is dev.coredev.Sona.desktop and MPRIS DesktopEntry
// must match it. Wails' GtkApplication runs non-unique; single-instance is
// handled by internal/app over D-Bus, so Wails' SingleInstance option must
// stay off.
const AppID = "dev.coredev.Sona"

// version is the app version, set from the VERSION file at build time
// (-ldflags "-X main.version=…", build/linux/Taskfile.yml). Plain `go build`
// and `go run` leave "dev".
var version = "dev"

// paintFallback opens the network even if the frontend never reports its
// first paint.
const paintFallback = 10 * time.Second

func main() {
	// The CDM host re-executes this binary as its child process; in the
	// child this never returns. It must run before anything else.
	cdmhost.ChildMain()

	for _, arg := range os.Args[1:] {
		if arg == "--version" {
			fmt.Println("sona", version)
			return
		}
	}

	started := time.Now()
	if err := run(started); err != nil {
		fmt.Fprintln(os.Stderr, "sona:", err)
		os.Exit(1)
	}
}

func run(started time.Time) error {
	paths, err := config.DefaultPaths()
	if err != nil {
		return err
	}
	log, closeLog, err := logging.New(logging.Options{File: paths.LogFile(), Dev: devMode})
	if err != nil {
		return err
	}
	defer closeLog()
	defer log.Info("shutdown complete")
	slog.SetDefault(log)
	log.Info("startup: process start", "version", version, "pid", os.Getpid())
	// Fatal crashes (in any goroutine) also go to the state dir's crash
	// file; a panic unwinding run is logged with its stack first.
	if err := diag.CaptureCrashes(paths.State, log); err != nil {
		log.Warn("crash capture unavailable", "err", err)
	}
	defer diag.LogPanic(log)

	settings, err := config.OpenStore(paths.SettingsFile())
	if err != nil {
		log.Warn("settings unreadable, using defaults", "path", paths.SettingsFile(), "err", err)
	}
	// WebKit reads these when the first web view is created.
	if err := settings.Get().Diagnostics.ApplyEnv(); err != nil {
		log.Warn("apply WebKit diagnostics", "err", err)
	}

	// Single instance: a second launch forwards its arguments and exits.
	act := app.CurrentActivation(os.Args[1:])
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		log.Warn("no D-Bus session bus: single-instance and MPRIS disabled", "err", err)
		conn = nil
	}
	var instance *app.Instance
	if conn != nil {
		defer conn.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		instance, err = app.Acquire(ctx, conn, act, log)
		cancel()
		switch {
		case errors.Is(err, app.ErrForwarded):
			log.Info("forwarded to the running instance")
			return nil
		case err != nil:
			log.Warn("single-instance guard unavailable", "err", err)
		default:
			defer instance.Close()
		}
	}

	ctx := context.Background()
	d, err := db.Open(ctx, paths.DatabaseFile())
	if err != nil {
		return err
	}
	defer d.Close()

	// One HTTP/2 client for every Apple request; its transport holds
	// requests until the first paint so cold start stays local.
	gate := services.NewGate(paintFallback, log)
	httpClient := &http.Client{Transport: gate.Transport(newTransport()), Timeout: 60 * time.Second}

	creds, err := store.Open(paths.Config, log)
	if err != nil {
		return fmt.Errorf("credentials store: %w", err)
	}
	dev := devtoken.NewWebPlayer(httpClient)
	authm, err := auth.NewManager(auth.Config{
		HTTP: httpClient, Dev: dev, Store: creds,
		Accounts: services.AccountStore{DB: d}, Logger: log,
	})
	if err != nil {
		return err
	}
	defer authm.Close()
	// Local only: reads the stored token and cached account; validation
	// runs in the background through the gated client.
	if err := authm.Start(ctx); err != nil {
		log.Warn("auth start", "err", err)
	}

	api := applemusic.New(httpClient, dev, authm.UserToken)
	fallbackSF := localeStorefront()
	storefront := func() string {
		if sf := authm.Status().StorefrontID; sf != "" {
			return sf
		}
		return fallbackSF
	}
	cat := catalog.New(d, api, storefront, log)
	lib := library.New(d, api)
	syncer := library.NewSyncer(d, api, log, 0)

	// Artwork comes from Apple's public CDN without credentials: a separate
	// client, held by the same gate (cached artwork still serves at once).
	art, err := artwork.New(artwork.Options{Dir: paths.Art, HTTP: &http.Client{Transport: gate.Transport(newTransport()), Timeout: 60 * time.Second}, Logger: log, MaxBytes: int64(settings.Get().Storage.ArtCacheMiB) << 20})
	if err != nil {
		return err
	}
	// Motion artwork: unencrypted video from Apple's media CDN, fetched
	// by Go and served at /motion/ like artwork.
	mot, err := motion.New(motion.Options{Dir: filepath.Join(paths.Cache, "motion"), HTTP: &http.Client{Transport: gate.Transport(newTransport()), Timeout: 2 * time.Minute}, Logger: log})
	if err != nil {
		return err
	}

	cur := settings.Get()
	engine := native.New(native.Options{
		Client:      api,
		CDMPath:     cur.CDMPath,
		Logger:      log,
		IdleTimeout: time.Duration(cur.EngineIdleMinutes) * time.Minute,
		Quality:     applemusic.Quality(cur.AudioQuality),
	})
	defer engine.Close()

	// Apple play-activity reporting (what the web player sends, so plays
	// count in Recently Played and recommendations). Undelivered events are
	// kept in the state dir, without tokens.
	reporter, err := activity.New(activity.Options{
		Client:     api,
		Storefront: func() string { return authm.Status().StorefrontID },
		StatePath:  filepath.Join(paths.State, "play-activity.json"),
		AppVersion: version,
		Logger:     log.With("component", "activity"),
	})
	if err != nil {
		return err
	}
	// Runs after coord.Close (which delivers the final end event) and
	// before engine.Close.
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := reporter.Close(ctx); err != nil {
			log.Warn("play activity: close", "err", err)
		}
	}()

	queueFile := filepath.Join(paths.State, "queue.json")
	history := &services.History{DB: d, Logger: log}
	coord, err := coordinator.New(coordinator.Options{
		Engine:     engine,
		Logger:     log.With("component", "coordinator"),
		Store:      coordinator.FileStore{Path: queueFile},
		OnPlayed:   history.Record,
		OnActivity: reporter.Observe,
	})
	if err != nil {
		return err
	}
	defer coord.Close() // saves the queue; runs before engine.Close
	history.Context = func() string { return coord.Snapshot().Context }
	if err := coord.Restore(ctx); err != nil {
		log.Warn("restore queue", "err", err)
	}
	if err := coord.SetVolume(ctx, cur.Volume); err != nil {
		log.Warn("apply volume", "err", err)
	}
	log.Info("startup: core ready", "since_start_ms", time.Since(started).Milliseconds())

	wapp := application.New(application.Options{
		Name:         "Sona",
		Description:  "Apple Music for the Linux desktop",
		Icon:         appIcon,
		Logger:       log,
		MarshalError: services.MarshalError,
		Assets: application.AssetOptions{
			Handler:        application.AssetFileServerFS(assets),
			Middleware:     artMiddleware(art, mot),
			DisableLogging: !devMode,
		},
		Linux: application.LinuxOptions{
			// Also becomes the program name (g_set_prgname) and Wayland app_id.
			ApplicationID: AppID,
		},
	})

	// The windows and the tray. --hidden (the autostart entry's choice)
	// starts with no window: playback, MPRIS and the tray only.
	sh := shell.New(shell.Options{
		App: wapp, Settings: settings, Coord: coord, Logger: log.With("component", "shell"),
		WindowIcon: appIcon, TrayIcon: trayIcon, Background: windowBackground(cur.Theme),
		Hidden: act.Hidden(),
	})
	defer sh.Close() // after core.Close, before coord.Close
	log.Info("startup: window created", "since_start_ms", time.Since(started).Milliseconds(), "webkitgtk", webkitVersion(), "hidden", act.Hidden())

	core := services.New(services.Deps{
		Paths: paths, QueueFile: queueFile, Settings: settings, DB: d, Auth: authm, API: api,
		Catalog: cat, Library: lib, Syncer: syncer, Art: art, Motion: mot, Engine: engine, Coord: coord,
		History: history, Storefront: storefront, Gate: gate, Emit: wapp.Event, Logger: log,
		OpenURL: wapp.Browser.OpenURL, Focus: sh.ShowMain, Focused: sh.MainFocused, Windows: sh, Bus: conn,
		Version: version, WebKit: webkitVersion(), Started: started,
	})
	defer core.Close() // runs first: stops sync, events and MPRIS
	for _, s := range core.Services() {
		wapp.RegisterService(s)
	}
	if conn != nil {
		// The shell quits asynchronously: shutdown closes the MPRIS server,
		// which must not wait on the D-Bus call that asked for it.
		mlog := log.With("component", "mpris")
		srv, err := mpris.New(conn, services.NewMPRISPlayer(coord, sh.ShowMain, sh.Quit, mlog), mlog)
		if err != nil {
			log.Warn("mpris unavailable", "err", err)
		} else {
			core.StartMPRIS(srv)
		}
	}

	core.Activate(act, false)
	if instance != nil {
		go func() {
			for a := range instance.Activations() {
				core.Activate(a, true)
			}
		}()
	}

	return wapp.Run()
}

// newTransport is the HTTP/2 keep-alive transport behind every client.
func newTransport() *http.Transport {
	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          64,
		MaxIdleConnsPerHost:   16,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
}

// artMiddleware serves the artwork cache at /art/ and motion artwork at
// /motion/ ahead of the frontend.
func artMiddleware(art, mot http.Handler) application.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasPrefix(r.URL.Path, "/art/"):
				art.ServeHTTP(w, r)
			case strings.HasPrefix(r.URL.Path, "/motion/"):
				mot.ServeHTTP(w, r)
			default:
				next.ServeHTTP(w, r)
			}
		})
	}
}

// localeStorefront guesses the catalog storefront for browsing while
// signed out from the locale's territory (en_GB.UTF-8 → gb), else "us".
func localeStorefront() string {
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		v := os.Getenv(k)
		if v == "" {
			continue
		}
		v, _, _ = strings.Cut(v, ".")
		v, _, _ = strings.Cut(v, "@")
		if _, terr, ok := strings.Cut(v, "_"); ok && len(terr) == 2 {
			return strings.ToLower(terr)
		}
		break
	}
	return "us"
}

// windowBackground is the colour GTK paints before the first frame: the
// design's ground colour for the configured theme (dark for "system", the
// common case on this desktop), so start-up does not flash white.
func windowBackground(theme string) application.RGBA {
	if theme == config.ThemeLight {
		return application.NewRGB(0xf4, 0xf5, 0xf3)
	}
	return application.NewRGB(0x12, 0x15, 0x13)
}
