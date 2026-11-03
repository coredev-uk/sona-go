// Package services is Sona's service surface: frontend-shaped
// wrappers over the core packages (auth, catalog, library, playback
// coordinator, artwork, settings), and the start-up policy that keeps cold
// start local until the first paint.
//
// # Module contract (for feature work)
//
// One feature = one service, in its own files:
//
//   - <feature>.go: the service type (exported methods are the bindings;
//     everything else unexported), and its constructor
//     func new<Feature>Service(e *Env) *<Feature>Service.
//   - <feature>_dto.go: the feature's own DTOs, its event name constants
//     (plain declarations: the app shell registers each event payload with
//     its UI bridge, see cmd/sona/events_wails.go while Wails remains).
//   - One line in registry.go: {"<feature>", svc(new<Feature>Service)}
//     (the registry returns the typed instances; Core.Services lists them,
//     Service[T] finds one).
//     No other file changes; features never call each other's services.
//
// Shared pieces every feature uses:
//
//   - model.go: DTOs shared by more than one feature — Source
//     ("library"|"catalog"), Art (Go-served /art/<key>/<w>x<h>.jpg URLs,
//     long edge Small 96 / Medium 400 / Large 1000 px, at the shape Apple
//     serves the artwork: square covers, 16:9 frames, wide banners),
//     Header (backdrop + colours + motion: a
//     Go-served /motion/ URL of Apple's motion cover), Song, Album,
//     Artist, Playlist (the card-level types), AlbumDetail, ArtistDetail,
//     PlaylistDetail, TrackRef. A type moves here when a second feature
//     needs it.
//   - mappers.go: mapper (Env.m) turns applemusic resources (am*) and
//     library rows (lib*) into those DTOs; artOf/header build artwork
//     URLs, motionOf motion artwork URLs (internal/motion). New resource
//     kinds get their mapper here.
//   - env.go: Env embeds Deps (DB, Auth, API, Catalog, Library, Syncer,
//     Art, Engine, Coord, History, Private, Settings, Storefront, Gate, Emit,
//     Logger, Now, OpenURL, Focus, Focused (main window has focus),
//     Windows (the desktop shell: main/mini player windows), Platform
//     (the OS integrations, below), Paths, Version, …) and provides
//     Report/Fail (route Apple 401/403s to auth triage — call it on every
//     failed Apple request), OnNetwork (work after first paint; never touch
//     the network earlier), OnSignOut (wipe steps, before the user-data
//     wipe), OnClose (shutdown steps), Go (start a background goroutine:
//     a panic is recovered and logged with its stack; never a bare `go`),
//     and members (catalog item → in the library).
//   - errors.go: every returned error reaches JS as err.cause =
//     {code, message}; the frontend words the code (an ErrorCode, bound
//     as a TypeScript enum) and keeps message, Go's English, for logs.
//     Go's text never reaches the UI: a DTO field that explains a failure
//     carries an ErrorCode, or its own string enum for a non-error reason
//     (StationUnavailable, playback.SandboxGap), never free text
//     (errors_test.go walks the bound types). New classes of failure the
//     user needs to tell apart get a sentinel error, a Code constant and
//     a CodeOf case, plus wording in frontend/src/shared/lib/errors.ts.
//   - Text Go shows itself, outside the web view (notification buttons,
//     file chooser titles and filters, the sleep inhibitor's reason), is
//     an l10n.Message shown through Deps.Language.Printer() when it is
//     shown, so it follows the Language setting (CONTRIBUTING.md, "Go's
//     own text").
//   - applemusic: one file per Apple domain; new amp-api endpoints are
//     one-liners with applemusic.AMP / AMPResource / AMPResources and
//     AMPQuery{Views, Include, Extend, With, Fields, Limit, Offset, Extra}
//     (platform=web is always sent).
//
// Audio (audio.go) applies config.Settings.Audio and Settings.Crossfade to
// the engine: the equaliser, the output device, Sound Check and the
// crossfade length are sent at start and from a Settings.OnChange
// observer; it offers the band/preset table, a live
// equaliser preview while a control is dragged (not saved) and the list
// of PipeWire outputs (from the engine). Volume stays the slider value everywhere (settings,
// coordinator, MPRIS); only the engine maps it through
// playback.VolumeGain. The player owns it: the player bar, MPRIS and
// `sona volume` set it on the coordinator, PlaybackService saves it into
// the settings a second after it settles (and on quit), and
// SystemService.UpdateSettings keeps the saved volume whatever it is sent.
//
// Platform (platform.go) is what the OS offers beyond the windows, one
// small seam per concern, built by cmd/sona's platform_<os>.go: media controls
// (MPRIS), notifications, the sleep inhibitor, the on-battery monitor, the
// autostart entry, the file chooser, plus whether there is a tray and a
// command transport (`sona <command>`, single instance), and how title bars
// work (TitleBar: macOS's inset buttons, or Sona's or the desktop's title
// bar and the desktop's button layout). DesktopService offers the title
// bar's buttons (MinimiseWindow, ToggleMaximiseWindow, CloseWindow),
// ShowWindowMenu and WindowState ("window:state"). A nil seam is
// unsupported on this platform (or, for the D-Bus ones, in a Linux
// session without a bus): its feature does nothing or fails with an error
// wrapping errors.ErrUnsupported (CodeUnsupported). Features never reach
// D-Bus, files or OS APIs for these directly. Deps.Engine is the platform's
// playback engine: the CDM host engine on Linux and macOS (cmd/sona's
// engine_cdm.go), playback.Unsupported elsewhere (every play fails with
// playback.ErrUnsupported; the
// coordinator pauses on it rather than skipping the queue); features ask
// it for optional abilities by interface (audioEngine, engineDiagnoser,
// qualitySetter).
//
// PlatformService.Capabilities (platform_dto.go) is the capability list
// the frontend reads once at start: the engine's kind and what it supports
// (volume, equaliser, Sound Check, crossfade, output device, private
// listening), which Platform seams exist, whether the web view is
// WebKitGTK (its rendering workarounds apply), and the main window's title
// bar (WindowControls, from Platform.TitleBar: internal/windowchrome). It is fixed for the process;
// whether a supported service is reachable right now stays in
// "integrations:status". A new seam is a Platform field, its use in the
// feature, and a Capabilities field the frontend gates its control on
// (docs/frontend-architecture.md, "Platform capabilities").
//
// CDM (cdm.go) is the Widevine CDM the CDM engine loads, which Sona
// downloads and updates by itself (Deps.CDM, internal/playback/cdm): the
// download or update check starts from OnNetwork, after the first paint.
// Status returns its CDMStatus (state, version,
// path, error code, progress, checkedAt), Retry checks again now, and
// "cdm:status" carries every change. Without a CDM engine Status is empty
// and Retry fails with CodeUnsupported.
//
// Private (private.go) is private listening (internal/privatemode) for the
// frontend: State, Set and "private:changed". Deps.Private is the one Mode
// every play reporter asks with the start of the listen (Mode.Since): the
// play-activity reporter, History, the scrobbler and the Discord presence;
// a toggle applies from the next event and a listen private at any point
// stays private. A development build's toggle never writes the setting.
// A song playing as its 30-second preview (SONA_PREVIEW=1, PlaybackState
// preview) never reaches a reporter: the coordinator reports no activity
// and no OnPlayed for it.
//
// First-run setup (onboarding.go): StartRoute picks the main window's
// first route before it is created, from the settings and SQLite only
// (config.Onboarding.Completed, the cached account): the setup
// (OnboardingRoute) on a first start, else Settings → General's page. The
// frontend saves the completed flag through SystemService.UpdateSettings.
//
// Pre-release albums (release.go): Album.Prerelease and Album.ReleaseAt
// (local midnight on the release date) mark an album not out yet; its
// songs not out yet are not playable and PlaybackService refuses them with
// code "not_released". Reminders (reminders.go, table release_reminders)
// give one desktop notification when such an album comes out, or after
// the first paint of the next start when Sona wasn't running then; the
// notification's words come from the frontend, and without
// Platform.Notifications Set fails with "unsupported". Pre-add is
// LibraryService.Add: Apple's web player pre-adds with the same call.
//
// Lyrics (lyrics.go): LyricsService.Lyrics(songID) asks the lyrics
// sources switched on in config.Settings.Lyrics (internal/lyrics: Apple
// Music, Musixmatch, LRCLIB, NetEase) in parallel, matching them by the
// catalog song's title, artist, album, length and ISRC (read through the
// catalog cache), and returns the one with the finest timing (word, line,
// plain; the order breaks ties) with a translation into
// Settings.Lyrics.TranslationLanguage (the account's language when "").
// Lyrics.Provider names the source. Every source's answer is cached in
// SQLite (lyrics_cache, emptied with the catalogue cache by
// StorageService.Clear); Musixmatch's anonymous user token is a keyring
// secret ("musixmatch"). Songs no source has lyrics for fail with code
// not_found.
//
// Concerts (concerts.go): ConcertsService.Concerts lists Apple's concerts near
// the chosen city, filtered by days, genre and (signed in) the user's own
// artists, ForArtist an artist's upcoming concerts, Places searches
// internal/places's own list of cities for the picker and SetLocation
// saves the choice (config.Concerts, announced as settings:changed; ""
// returns to the storefront's main city). Apple wants an area as a
// geohash and is told that and nothing else of the place; lists are cached
// with discover_cache's rules, keyed by area, filters and day. Ticket links
// are https only. Home's concerts shelf is a Section of kind "concerts"
// with no items, filled by the frontend from Concerts.
//
// DTO rules: durations and timestamps in integer milliseconds (timestamps
// as Unix ms, 0 when unknown); artwork and motion artwork only as
// Go-served URLs (never Apple templates, Apple media URLs or tokens); IDs
// as strings with an explicit Source where an item can be either; empty
// lists as [] not null.
//
// main builds Deps and calls New; Core then exposes Services() for
// registration, Activate (launch links → "app:open"), StartMediaControls
// and Close.
package services
