// Package services is Sona's Wails service surface: frontend-shaped
// wrappers over the core packages (auth, catalog, library, playback
// coordinator, artwork, settings), and the start-up policy that keeps cold
// start local until the first paint.
//
// # Module contract (for feature work)
//
// One feature = one bound Wails service, in its own files:
//
//   - <feature>.go: the service type (exported methods are the bindings;
//     everything else unexported), and its constructor
//     func new<Feature>Service(e *Env) *<Feature>Service.
//   - <feature>_dto.go: the feature's own DTOs, its event name constants
//     and an init() registering each event payload with
//     application.RegisterEvent (so bindings carry the type).
//   - One line in registry.go: {"<feature>", svc(new<Feature>Service)}.
//     No other file changes; features never call each other's services.
//
// Shared pieces every feature uses:
//
//   - model.go: DTOs shared by more than one feature — Source
//     ("library"|"catalog"), Art (Small 96 / Medium 400 / Large 1000 px
//     Go-served /art/ URLs), Header (backdrop + colours + motion: a
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
//     {code, message}; add a Code constant and an ErrorCode case for new
//     classes of failure.
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
// playback.VolumeGain.
//
// Platform (platform.go) is what the OS offers beyond the windows, one
// small seam per concern, built by main's platform_<os>.go: media controls
// (MPRIS), notifications, the sleep inhibitor, the on-battery monitor, the
// autostart entry, the file chooser, plus whether there is a tray and a
// command transport (`sona private …`, single instance). A nil seam is
// unsupported on this platform (or, for the D-Bus ones, in a Linux
// session without a bus): its feature does nothing or fails with an error
// wrapping errors.ErrUnsupported (CodeUnsupported). Features never reach
// D-Bus, files or OS APIs for these directly. Deps.Engine is the platform's
// playback engine: the CDM host engine on Linux, playback.Unsupported
// elsewhere (every play fails with playback.ErrUnsupported; the
// coordinator pauses on it rather than skipping the queue); features ask
// it for optional abilities by interface (audioEngine, engineDiagnoser,
// qualitySetter).
//
// PlatformService.Capabilities (platform_dto.go) is the capability list
// the frontend reads once at start: the engine's kind and what it supports
// (volume, equaliser, Sound Check, crossfade, output device, private
// listening), which Platform seams exist, and whether the web view is
// WebKitGTK (its rendering workarounds apply). It is fixed for the process;
// whether a supported service is reachable right now stays in
// "integrations:status". A new seam is a Platform field, its use in the
// feature, and a Capabilities field the frontend gates its control on
// (docs/frontend-architecture.md, "Platform capabilities").
//
// CDM (cdm.go) is the Widevine CDM the CDM engine loads, which Sona
// downloads and updates by itself (Deps.CDM, internal/playback/cdm): the
// download or update check starts from OnNetwork, after the first paint.
// Status returns its CDMStatus (state, source download|override, version,
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
