// Command sona is the Sona desktop app: MyGo's native UI (GTK3 on Linux,
// AppKit, Win32) drawn by the same process that holds all Apple Music
// networking, credentials, caching and playback. What differs per OS (the
// playback engine, desktop integrations, single instance) is built in
// platform_<os>.go.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"

	"github.com/coredev-uk/sona-go/build"
	"github.com/coredev-uk/sona-go/internal/app"
	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/artwork"
	"github.com/coredev-uk/sona-go/internal/auth"
	"github.com/coredev-uk/sona-go/internal/auth/devtoken"
	"github.com/coredev-uk/sona-go/internal/auth/store"
	"github.com/coredev-uk/sona-go/internal/catalog"
	"github.com/coredev-uk/sona-go/internal/cli"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/diag"
	"github.com/coredev-uk/sona-go/internal/i18n/applestrings"
	"github.com/coredev-uk/sona-go/internal/i18n/l10n"
	"github.com/coredev-uk/sona-go/internal/library"
	"github.com/coredev-uk/sona-go/internal/logging"
	"github.com/coredev-uk/sona-go/internal/motion"
	"github.com/coredev-uk/sona-go/internal/playback/activity"
	"github.com/coredev-uk/sona-go/internal/playback/cdm"
	"github.com/coredev-uk/sona-go/internal/playback/coordinator"
	"github.com/coredev-uk/sona-go/internal/playback/video"
	"github.com/coredev-uk/sona-go/internal/privatemode"
	"github.com/coredev-uk/sona-go/internal/services"
	uiapp "github.com/coredev-uk/sona-go/internal/ui/app"
	uistore "github.com/coredev-uk/sona-go/internal/ui/store"
)

// AppID is the application id: the desktop file is dev.coredev.Sona.desktop
// and MPRIS' DesktopEntry must match it; on Linux it is also GLib's program
// name, the window's app_id (appid_linux.go). Single instance is
// internal/app's over D-Bus (platform_linux.go), not MyGo's.
const AppID = "dev.coredev.Sona"

// version is the app version, set from the VERSION file at build time
// (-ldflags "-X main.version=…", build/linux/Taskfile.yml). Plain `go build`
// and `go run` leave "dev".
var version = "dev"

// paintFallback opens the network even if the window never reports its
// first paint.
const paintFallback = 10 * time.Second

func main() {
	// The CDM host re-executes this binary as its child process; in the
	// child this never returns. It must run before anything else.
	childMain()

	for _, arg := range os.Args[1:] {
		if arg == "--version" {
			fmt.Println("sona", version)
			return
		}
	}
	// `sona play`, `sona status` and the other commands (internal/cli) act
	// on the running instance and exit; they never start Sona.
	if cli.IsCommand(os.Args[1:]) {
		os.Exit(cli.Run(context.Background(), os.Args[1:], cli.Env{
			Stdout: os.Stdout, Stderr: os.Stderr, Dial: dialInstance, Version: version, Printer: cliPrinter(),
		}))
	}

	started := time.Now()
	if err := run(started); err != nil {
		fmt.Fprintln(os.Stderr, "sona:", err)
		os.Exit(1)
	}
	probeExit() // a failed `sona -probe` exits non-zero (probe builds only)
}

// cliPrinter is the command line's language: the Language setting
// (Settings → General) when one is chosen, else the system's languages.
// An unreadable settings file means Automatic.
func cliPrinter() *l10n.Printer {
	if paths, err := config.ResolvePaths(os.Getenv); err == nil {
		if s, err := config.LoadSettings(paths.SettingsFile()); err == nil && s.Language != config.LanguageAutomatic {
			return l10n.For([]string{s.Language})
		}
	}
	return l10n.For(l10n.SystemLanguages())
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
	// An update that keeps failing before its first paint is undone here,
	// before anything else starts.
	if rollBackFailedUpdate(paths, version, log) {
		return nil
	}
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

	// Single instance: a second launch forwards its arguments and exits.
	act := app.CurrentActivation(os.Args[1:])
	plat, err := openPlatform(paths, act, log)
	if errors.Is(err, app.ErrForwarded) {
		log.Info("forwarded to the running instance")
		return nil
	}
	if err != nil {
		return err
	}
	defer plat.Close()

	ctx := context.Background()
	d, err := db.Open(ctx, paths.DatabaseFile())
	if err != nil {
		return err
	}
	defer d.Close()

	// The user's language (Settings → General → Language, else the
	// system's) for Go's own text and Apple's content language.
	lang := l10n.NewLanguage(settings.Get().Language)
	lang.DetectSystem()
	settings.OnChange(func(prev, next config.Settings) {
		if prev.Language != next.Language {
			lang.SetSetting(next.Language)
		}
	})

	// One HTTP/2 client for every Apple request; its transport holds
	// requests until the first paint so cold start stays local.
	gate := services.NewGate(paintFallback, log)
	httpClient := &http.Client{Transport: gate.Transport(newTransport()), Timeout: 60 * time.Second}

	dev := devtoken.NewWebPlayer(httpClient)
	// The keyring is opened and read in the background (Start, below): an
	// unlock prompt or a Secret Service that never answers must not hold
	// the window. Validation runs through the gated client.
	authm, err := auth.NewManager(auth.Config{
		HTTP: httpClient, Dev: dev,
		Open:     func() (store.Store, error) { return store.Open(paths.Config, log) },
		Accounts: services.AccountStore{DB: d}, Logger: log,
	})
	if err != nil {
		return err
	}
	defer authm.Close()

	api := applemusic.New(httpClient, dev, authm.UserToken)
	// Apple's content in the user's language where the account's
	// storefront offers it (l=), else the storefront's default.
	api.Language = func() string {
		def, supported := authm.Languages()
		return applemusic.ContentLanguage(lang.Preferred(), def, supported)
	}
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
	// by Go.
	mot, err := motion.New(motion.Options{Dir: filepath.Join(paths.Cache, "motion"), HTTP: &http.Client{Transport: gate.Transport(newTransport()), Timeout: 2 * time.Minute}, Logger: log})
	if err != nil {
		return err
	}

	// The Widevine CDM: downloaded from Google after the first paint (the
	// gated client), kept up to date in the data directory. Only an engine
	// that needs it starts it.
	cdmMgr, err := cdm.New(cdm.Options{
		Dir:       paths.WidevineDir(),
		HTTP:      &http.Client{Transport: gate.Transport(newTransport())},
		LoadCheck: cdmLoadCheck(log.With("component", "widevine")),
		Logger:    log.With("component", "widevine"),
	})
	if err != nil {
		return err
	}
	defer cdmMgr.Close()

	// Apple Music's own translations of Sona's messages: fetched from
	// music.apple.com after the first paint (the gated client), for the
	// locales the UI asks for, and cached; never bundled.
	appleStrings, err := applestrings.New(applestrings.Options{
		Dir:    filepath.Join(paths.Cache, "strings"),
		HTTP:   &http.Client{Transport: gate.Transport(newTransport()), Timeout: time.Minute},
		Logger: log.With("component", "strings"),
	})
	if err != nil {
		return err
	}
	defer appleStrings.Close()

	cur := settings.Get()
	// Music video pictures: the engine shows each on vid as it falls due
	// (memory only); the video view (internal/ui/features/video) draws them.
	vid := video.NewSurface()
	engine := newEngine(api, cur, log, cdmMgr, storefront, vid)
	engine = e2eEngine(engine, log) // the e2e smoke's probe build only
	defer engine.Close()

	// GStreamer's plugin registry is readied in a child after the first
	// paint, off the play path: on a computer's first run its scan can
	// take longer than the CDM download.
	audioCtx, stopAudio := context.WithCancel(context.Background())
	defer stopAudio()
	gate.OnOpen(func() { go prepareAudio(audioCtx, log.With("component", "playback")) })

	// Private listening: the one switch every play reporter asks (Apple
	// play activity, local history, scrobbles, Discord). Development builds
	// start private as a session override that never writes the setting;
	// SONA_PRIVATE=1 forces it, SONA_PRIVATE=0 opts a development build out.
	override, err := privatemode.Env(os.Getenv(privatemode.EnvVar))
	if err != nil {
		log.Warn("private listening", "err", err)
	}
	private := privatemode.New(privatemode.Options{
		Saved: cur.Private.Enabled, Dev: devMode, Override: override,
		Save: func(on bool) error {
			_, err := settings.Update(func(s *config.Settings) { s.Private.Enabled = on })
			return err
		},
	})
	pst := private.State()
	log.Info("startup: private listening", "on", pst.On, "source", pst.Source)

	// Apple play-activity reporting (what the web player sends, so plays
	// count in Recently Played and recommendations). Undelivered events are
	// kept in the state dir, without tokens.
	reporter, err := activity.New(activity.Options{
		Client:     api,
		Storefront: func() string { return authm.Status().StorefrontID },
		StatePath:  filepath.Join(paths.State, "play-activity.json"),
		AppVersion: version,
		Logger:     log.With("component", "activity"),
		Private:    private.Since,
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
	history := &services.History{DB: d, Logger: log, Private: private.Since}
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
	// An engine without a volume (none on this platform) has nothing to
	// apply it to.
	if engine.Capabilities().Volume {
		if err := coord.SetVolume(ctx, cur.Volume); err != nil {
			log.Warn("apply volume", "err", err)
		}
	}
	log.Info("startup: core ready", "since_start_ms", time.Since(started).Milliseconds())

	// The event bus the services emit into and the window control they
	// keep exist before the core; the UI connects them to the window when
	// MyGo is ready.
	bus := uistore.NewBus(log)
	windows := uiapp.NewWindows(devMode)

	core := services.New(services.Deps{
		Paths: paths, QueueFile: queueFile, Settings: settings, DB: d, Auth: authm, API: api,
		Catalog: cat, Library: lib, Syncer: syncer, Art: art, Motion: mot, Engine: engine, CDM: cdmMgr, AppleStrings: appleStrings, Language: lang, Coord: coord,
		History: history, Private: private, Storefront: storefront, Gate: gate, Emit: bus, Clipboard: clipboardText, Logger: log,
		OpenURL: mygo.Shell.OpenExternal, Focus: windows.ShowMain, Focused: windows.Focused, Windows: windows, Platform: plat.Services(windows),
		Transport: newTransport(), Version: version, Started: started, SelfUpdater: selfUpdaterDep(),
	})
	defer core.Close() // runs first: stops sync, events and MPRIS
	// After the services subscribed to auth changes, so none misses the
	// restored sign-in.
	authm.Start()
	// MPRIS' Quit comes from a bus call: Windows.Quit only posts the quit
	// to the main loop (mygo.App.Quit), since shutdown closes the media
	// controls, which must not wait on the call that asked for it.
	core.StartMediaControls(services.NewMPRISPlayer(coord, windows.ShowMain, windows.Quit, log.With("component", "mpris")))

	core.Activate(act, false)
	plat.Serve(core.Activate, core.Commands())
	openURLs(core.Activate)

	setApplicationID(log)
	mygo.App.SetName("Sona")
	mygo.App.SetVersion(version)
	// A first start (no account, setup neither finished nor skipped) opens
	// on the first-run setup; the other start routes are the shell's.
	firstRun := false
	if route, err := services.StartRoute(ctx, d, settings.Get()); err != nil {
		log.Warn("start route", "err", err)
	} else {
		firstRun = route == services.OnboardingRoute
	}
	var ui *uiapp.App
	mygo.App.WhenReady(func() {
		// --hidden (the XDG autostart entry's) or, on macOS, the login item
		// the user asked to start in the tray (login_darwin.go).
		hidden := act.Hidden() || loginStartsHidden(cur.Desktop)
		ui = uiapp.New(uiapp.Options{
			Core: core, Settings: settings, Bus: bus, Windows: windows, Art: art, Motion: mot, Video: vid, Log: log.With("component", "ui"),
			Version: version, Dev: devMode, Icon: build.AppIcon, TrayIcon: build.TrayIcon, TrayTemplate: trayTemplate(),
			OpenURL: mygo.Shell.OpenExternal, Copy: clipboardText,
			FirstRun: firstRun, Hidden: hidden,
		})
		log.Info("startup: window created", "since_start_ms", time.Since(started).Milliseconds(), "hidden", hidden)
	})
	err = mygo.App.Run()
	if ui != nil {
		ui.Close() // before core.Close and coord.Close
	}
	return err
}

// clipboardText puts text on the system clipboard (services.Deps.Clipboard).
func clipboardText(text string) bool {
	return mygo.Clipboard.Write(transfer.TextData(text)) == nil
}

// newTransport is the HTTP/2 keep-alive transport behind every client.
func newTransport() *http.Transport {
	return e2eTransport(&http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          64,
		MaxIdleConnsPerHost:   16,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
	})
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
