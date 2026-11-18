# UI architecture

This page maps `internal/ui`: the packages, the module contract, the store, the kit, the theme, the shell and every feature. `docs/architecture.md` has the process model and the platform seams, `docs/design.md` the visual direction and `internal/services/doc.go` the service contracts.

Sona's interface is Go all the way down: [MyGo](https://github.com/egoist/mygo)'s native UI (`github.com/egoist/mygo/ui`, v0.9.0; GTK3 on Linux, AppKit on macOS, Win32 on Windows) draws on the GPU from the process that holds the services, the library and the playback coordinator. There is no web view, no IPC and no generated bindings: a view calls a typed service and reads the DTO it returns.

## Contents

- [Layout](#layout)
- [The module contract](#the-module-contract)
  - [Env](#env)
  - [What a module declares](#what-a-module-declares)
- [Store, queries and events](#store-queries-and-events)
- [Strings and locales](#strings-and-locales)
- [Theme tokens](#theme-tokens)
  - [Tokens](#tokens)
  - [Themes](#themes)
  - [Using the theme](#using-the-theme)
- [The kit](#the-kit)
  - [Conventions and shared helpers](#conventions-and-shared-helpers)
  - [Buttons and controls](#buttons-and-controls)
  - [Artwork and motion](#artwork-and-motion)
  - [Artists of a song](#artists-of-a-song)
  - [Overlays and menus](#overlays-and-menus)
  - [Lists, tiles, selection and drag](#lists-tiles-selection-and-drag)
  - [Media widgets](#media-widgets)
- [Testing with `ui.Tester`](#testing-with-uitester)
- [The development loop](#the-development-loop)
- [Conventions](#conventions)
- [The app shell](#the-app-shell-internaluiapp)
  - [Start-up and windows](#start-up-and-windows)
  - [Routes, links and history](#routes-links-and-history)
  - [Shortcuts and the macOS menu bar](#shortcuts-and-the-macos-menu-bar)
- [Features](#features)
  - [The player bar](#the-player-bar-featuresplayer)
  - [Now Playing and lyrics](#now-playing-and-lyrics-featuresplayer-lyricsync)
  - [The mini player](#the-mini-player-featuresminiplayer)
  - [Account side: private listening, Widevine, updates, reminders and motion](#account-side-private-listening-widevine-updates-reminders-and-motion)
  - [Music videos](#music-videos-featuresvideo)
  - [Library](#library-featureslibrary)
  - [Settings](#settings-featuressettings)
  - [Account and sign-in](#account-and-sign-in-internaluiaccount-featuresauth)
  - [First-run setup](#first-run-setup-featuresonboarding)
  - [Equaliser](#equaliser-featuresequalizer)
  - [Commands: the palette and the shortcut help](#commands-the-palette-and-the-shortcut-help-featurescommands)
  - [Playlists](#playlists-featuresplaylist)
  - [Search](#search-featuressearch)
  - [Concerts](#concerts-featuresconcerts)
  - [New releases](#new-releases-featuresnewreleases)
  - [Browse: Home, Discover, Radio](#browse-home-discover-radio-featureshome-featuresdiscover-featuresradio)
  - [Catalogue pages](#catalogue-pages-featuresalbum-featuresartist-featuressong)
- [Tests](#tests)

## Layout

```text
cmd/sona ─► internal/ui/app ─► internal/ui/features/<name> ─► internal/ui/{module,kit,store,theme,i18n,media,account}
                                                   │                └─► internal/services (typed services, DTOs)
internal/ui/app ─► internal/ui/module (types only)
```

| Package | Holds |
| --- | --- |
| `internal/ui/app` | The windows (main, mini player, tray), the macOS menu bar, the sidebar and top bar, the status banner, the router, the shortcut dispatcher. Builds the `Env` and the module list (`modules.go`) |
| `internal/ui/module` | The contract between the shell and a feature: `Env`, `Module`, `Route`, `NavItem`, `Shortcut`, `Slots`. Types only: no windows, no state |
| `internal/ui/features/<name>` | One package per feature (album, library, player…). Features never import each other |
| `internal/ui/account` | The sign-in and account views that the account dialog (`features/auth`) and the first-run setup (`features/onboarding`) share. A shared UI package like `kit`, so two features need not import each other |
| `internal/ui/kit` | Components shared by features (Button, Artwork…), built on MyGo widgets and the theme |
| `internal/ui/media` | What MyGo lacks of the system's media stack: motion artwork players and the music video's pictures |
| `internal/ui/lyricsync` | The timing logic of synced lyrics, shared by Now Playing, full-screen lyrics and the mini player |
| `internal/ui/store` | The event bus, cached queries and the playback mirror: everything that comes from Go and changes |
| `internal/ui/theme` | Design tokens (colours light and dark, type, radii, space, motion), their application to MyGo's theme, Lucide icons, the interface font |
| `internal/ui/i18n` | `T`, `TC`, `ErrorText`, the locale and its formatters, over `locales/en.po` |
| `internal/ui/devstate` | Dev builds only: the restore file of the development loop |
| `internal/ui/uitest` | Test harness: a real `module.Env` over a seeded library, `access` (accessibility checks), `fixture`, `artifacts` |
| `internal/ui/probe` | The `-tags probe` build's automation seam (see [Tests](#tests)) |

Package notes:

- **`app`.** `modules.go` has one line per feature. Its order sets the order of routes, event handlers, shortcuts and slots; nav items sort by their own `Order`; a duplicate module ID panics at start-up.
- **`account`.** The sign-in reducer, the credentials, code, browser and token forms, the signed-in view and the keyring notices.
- **`media`.** Motion artwork plays through MyGo's `ui/video` on macOS and Windows (`motion_native.go`: AVFoundation, Media Foundation), through GStreamer by cgo on Linux (`motion_gst.go`) and not at all without cgo (`motion_none.go`).
- **`media.OpenMotion`.** Turns the motion cache's file into a looping, muted `ui.VideoPlayer`: `Ready` at once on Linux, and on macOS and Windows when the system's player has its first picture.
- **`media.Feed`.** Hands the CDM child's I420 frames to a bitmap: `Picture.YUV` is the image `Bitmap.UpdateYUV` shows.

A feature reaches another feature's state only through `store` (the playback mirror, the queries) or a hook in `module.Env`. A new feature is a Go service (`internal/services/<feature>.go`, `<feature>_dto.go` and one `registry.go` line) and a package under `internal/ui/features` with one `modules.go` line.

## The module contract

A feature is a `module.Factory`, `func(env *module.Env) module.Module`. It keeps its state in its own struct; its views are methods on it.

```go
func New(env *module.Env) module.Module {
    l := &library{env: env}
    return module.Module{
        ID:     "library",
        Routes: []module.Route{{Pattern: "/library/albums", Title: func(*ui.Route) string { return i18n.T("Albums") }, Build: l.albums}},
        Nav:    []module.NavItem{{Section: module.NavLibrary, Label: func() string { return i18n.T("Albums") }, Icon: "disc-3", To: "/library/albums", Order: 1}},
        Init:   l.init,
    }
}
```

### Env

`Env` is built once by `app.New` (and by `uitest.New` in tests) and its fields never change. In an `Env` built by hand without services, `Queue` and `ItemStates` are nil. Without a shell, `Motion`, `Nav`, `HasRoute` and `Shortcuts` are nil.

**Services and state**

| Field | What it is |
| --- | --- |
| `Core` | Typed service lookup: `services.Service[services.LibraryService](env.Core)` returns the `*LibraryService` |
| `Bus` | Go's events; `store.On` subscribes |
| `Queries` | The cached queries; `Invalidate(prefixes...)` |
| `Play` | `store.Playback`, the playback mirror |
| `Post` | Runs a func on the main thread and redraws: how a goroutine hands its outcome to a view |
| `Log`, `Version` | The logger and the app version |
| `Dev` | True in a `-tags dev` build |

**Items**

| Field | What it is |
| --- | --- |
| `Art` | The artwork loader |
| `Actions` | The item action registry |
| `Players` | `kit.ItemPlayers`, the shared item-play registry. Discover registers how songs, albums, playlists, videos and radio stations play; the shared item components play through it |
| `ItemsCtx(menus)` | The context those components take: router, route check, browser, players, the account's explicit restriction, artwork, motion and what plays |
| `ItemStates` | `kit.ItemStates`: the one library state and rating layer that every item menu, header and rating button reads and changes through. It follows `library:item-state` itself |
| `Queue` | `kit.QueueCommands`: every feature that plays a collection or adds to the queue goes through it, so replacing a long queue of the user's asks first or offers Undo |

**Shell**

| Field | What it is |
| --- | --- |
| `Nav` | The main window's router |
| `HasRoute` | `func(path string) bool`: whether a registered module's route pattern matches the path (`/album/{id}` matches `/album/42`). A feature renders a link only when it is true, so nothing links to a page that is not registered. Call it while building a view: the routes of modules built later are not known in a factory |
| `Shortcuts()` | Every shortcut with the keys it answers to now |
| `Motion` | The `kit.MotionPolicy` that the motion covers and tiles take |
| `Surfaces` | `store.Surfaces`: the open state of the account dialog and the first-run setup. Any "Sign in" action calls `Surfaces.OpenAccount()`; Settings' "Set up Sona again" calls `Surfaces.OpenSetup(step)`; while the setup is open, `OpenAccount` moves it to its Apple Music step instead |
| `Video` | `store.VideoView`: whether the music video view is open (`Open`, `Close`, `Toggle`, `IsOpen`), the window's full screen it toggles, and the source of its pictures. Any feature that starts a video calls `Video.Open`; the video feature draws the view |
| `OpenURL`, `Copy` | The system browser and the clipboard |

**Between features**

| Field | What it is |
| --- | --- |
| `PlayerControls`, `PlayerNotice` | `module.Point`s (see [What a module declares](#what-a-module-declares)) |
| `ConcertsShelf` | Home's concerts shelf |
| `ArtistConcerts` | `func(c *ui.Context, artistID string, first bool)`: the artist page's "Upcoming concerts" |

The concerts module sets `ConcertsShelf` and `ArtistConcerts` in its factory. They build nothing while it is not registered.

### What a module declares

- **Routes** use MyGo's router patterns (`/album/{id}`, `/settings/{section...}`). The route memory, titles and the back and forward buttons are the shell's.
- **Nav** entries sit in the primary, library or footer section and sort by `Order`. `To` is one of the module's own patterns. Each entry gets a go-to shortcut (`Keys`, else Ctrl+1…9 or Alt+1…9 by position). `Count`, when set, is a number the link carries at its end while it is above 0 and in its name for assistive technology ("New releases, 3 new"); it is read every frame, so it only peeks at a mirror.
- **Init** runs once after the shell connects, on the main thread: subscriptions (`store.On`) and first snapshots. A module's `Init` subscribes to each event, then calls `env.Queries.Invalidate(store.Key{"library"})` or the like.
- **Actions** (`[]kit.ActionDef`) are the module's item actions, single (`Target` a kind) and bulk (`Target: kit.KindSelection`). The shell registers every module's into one `kit.ActionRegistry` (`Env.Actions`), which `kit.ActionMenus` lists and shortcuts and the palette share by ID. A duplicate ID panics at start-up.

**Shortcuts**

- A shortcut has a stable `ID` (`<module>.<action>`). The settings store a user's bindings by ID, so an ID is never renamed.
- `Keys` use the dispatcher's spelling (`Space`, `Ctrl+K`, `Shift+ArrowRight`).
- One key handler in the shell dispatches them: page-scoped ones skip text fields, and the first match whose scope and `When` allow it runs.
- `Unavailable` says why a key does nothing (the key is taken). `NoPalette` keeps a shortcut out of the command palette.

**Slots** are places in the shell that a module builds into:

| Slot | Where |
| --- | --- |
| `Aside` | Right of the page |
| `Overlay` | Covers the page column. Its optional `Close` is called when the page changes while it is open: back, forward, a swipe or a link show the new page, not a page hidden behind it |
| `Bottom` | The player bar |
| `Layer` | Over the whole window. While `LayerOpen`, the shell builds it as a modal overlay: the window behind takes no input and assistive technology reads the layer alone. The page is still built and drawn, so first paint is reported as always |
| `SidebarFooter`, `SidebarSection` | In the sidebar |
| `TopBarSearch` | The top bar's search field |
| `TopBarAccount` | The top bar's trailing end: the account menu |
| `Mini` | The mini player window's whole content (the first module's wins) |

A slot func builds nothing when it has nothing to show. A route or nav item exists only when its module is registered: there are no placeholders.

**Points** are places inside another feature's view, as Slots are places in the shell.

- A `module.Point` (zero value ready) is filled while the factories run with `Add(order, build)`. Its owner builds it with `Build(c)`, in order.
- `Env.PlayerControls` is the player bar's right group, before mute and the volume. Private listening is order 10; the sleep timer, lyrics and queue buttons add theirs.
- `Env.PlayerNotice` is the line under the title while a song waits for the Widevine CDM.
- An `Env` is shared by pointer, so what a module adds is seen by the feature that builds the point, and no shell code is involved.

## Store, queries and events

All callbacks a UI author passes run on the main thread. Everything in `store` is safe from any goroutine unless noted. Results and events come back through a `store.Poster`, which `app` supplies (`Window.Update` on every window: it runs the func on the main thread and redraws) and tests supply synchronously.

### Events

`store.Bus` implements `services.Emitter`: the services emit into it from any goroutine. Events emitted before `SetPoster` are queued and delivered in order; afterwards each is posted in emission order.

Subscribe with a typed handler, `store.On(env.Bus, services.EventPlaybackState, func(s services.PlaybackState) {…})`. A payload of another type is logged once and dropped.

### Queries

`store.Use(env.Queries, key, fetch, opts…)` returns the cached `Result[T]` (`Data`, `Err`, `HasData`, `Loading`, `Fetching`). It starts a fetch on a goroutine when there is no data, or when the data is stale as a view starts using it.

Call it from the view every frame. A view that goes unbuilt for a second and is built again is a new mount (TanStack's observer mounting), and only then is stale data refetched, with the old data showing meanwhile.

```go
lib := services.Service[services.LibraryService](l.env.Core)
r := store.Use(l.env.Queries, store.Key{"library", "albums", l.sort},
    func(ctx context.Context) (services.AlbumPage, error) { return lib.Albums(ctx, l.sort, false, "", 0, 1000) },
    store.Options{Stale: store.Forever})
```

**Keys**

- A key is a path (`["library","albums",…]`) and holds every input of the fetch.
- `Invalidate(prefixes…)` marks the queries below each prefix stale (all, with none). It refetches the ones used in the last minute at once and cancels a fetch in flight; the rest refetch when next used.

**Options**

- `Stale`: 0 is stale at once; `store.Forever` is never stale.
- `Keep`: an unused query is dropped after this (0 is five minutes). Dropping cancels the fetch's context.
- `NoRetry`: the error stays until `Invalidate` or a new mount. Use it for network reads whose failure is an answer.

**Fetch rules**

- Fetches of one key never run twice at once.
- A failed query with no data is asked again only after 5 s, so a broken service is not asked every frame.
- A panicking fetch becomes the query's error.

**Writing**

- `Set` writes data (optimistic updates, event payloads) and cancels a fetch in flight, whose answer predates the write.
- `SetIfEmpty` writes a snapshot only while nothing is there (check and write under one lock).
- `Peek` reads without fetching; `Reset` drops everything (sign-out).

### Playback mirror

`store.Playback` follows `EventPlaybackState` and `EventPlaybackQueue`. It offers `State()`, `Queue()` and `Position()`. `Position()` is extrapolated from the last snapshot while playing (`PositionMs + now − At`, never before the snapshot, clamped to the duration when known).

`Seed` sets the mirror from the services' snapshots before the first event. `Patch` changes it locally while a command is in flight, until Go's next snapshot.

## Strings and locales

Every string the user reads (text, labels, tooltips, toasts, menu and command labels) goes through `internal/ui/i18n` and lands in the English catalogue `internal/ui/i18n/locales/en.po`.

```go
i18n.T("Add music by {name} to collect it here.", "name", artist)
i18n.TC("verb", "Play")                                        // with a msgctxt
i18n.T("{0, plural, one {# album} other {# albums}}", 0, n)    // positional names: "0" or 0
```

### The msgid rule

The message and the context MUST be string literals at the call site. The English text is the msgid: the catalogue's identity of the message and the source of Apple's mapping.

- `go run ./cmd/i18ntool extract` reads `internal/ui` with `go/ast` and collects every `i18n.T("…")` and `i18n.TC("…", "…")`. A non-literal argument is an error.
- `extract` writes `en.po` and `en-XA.po` whole: one entry per message the code asks for, sorted, with the Go files as references. A message no call asks for is dropped.
- `extract -check` (part of `check`) fails when either file is stale or a mapped message has gone.
- Positional placeholders are `{0}`, `{1}`; the Go call names them `"0"`, `"1"`.

Go's own catalogue (`internal/i18n/l10n`: notifications and file dialogs) is separate. `extract` keeps both.

### Messages and lookup

Messages are ICU MessageFormat, parsed and formatted by MyGo (`i18n.ParseMessage`): named and positional arguments, `plural` and `select` with CLDR's rules, `#`, number and date formats. A message that does not parse panics under test and in dev builds. An Apple or community translation that does not parse is left out and logged once.

The locale shown looks a message up in this order:

1. Apple's translation (`services.AppleTranslations`, set with `i18n.SetApple`, keyed by `applestrings.MessageID(msg, ctx)`).
2. The community catalogue (`i18n.LoadCommunity`, a `.po` keyed by the English msgids).
3. English, the call site's own text.

`en-XA` is MyGo's pseudo-locale (accented, stretched).

### Choosing the locale

The app's `locale.go` chooses over two helpers. `TranslationCandidates` lists the account's storefront language, then the system's, up to the first English one. `SelectableShare` (0.9) is the share of Apple's strings a language must cover to be offered. The language the user picked in Settings shows whatever its share.

`LanguageOptions` builds the Language setting's list. `SetLocale`, `SetApple` and `SetRegional` redraw through `i18n.SetPoster`; `SetRegional` takes the system's and the account's tags, so an English UI on an en-GB system keeps 24-hour clocks.

### Formatting

All formatters work in the shown locale. The date, clock and duration formatters follow the regional format locale (`Locales()[0]`), never the catalogue's language.

| Function | What it writes |
| --- | --- |
| `FormatNumber`, `FormatDate(t, DateMedium)`, `FormatClock` | Numbers, dates and clock times |
| `FormatDuration` | `m:ss` |
| `FormatRunningTime`, `FormatTimeLeft` | "1 hr, 12 min", "18 days, 6 hours" (`Intl.DurationFormat` short and long) |
| `ShortReleaseDate(t, now)` | "19 Nov"; the year only when it is not this year's |
| `RelativeTime(t, now)` | Freshness: never, just now, N min./hr. ago, yesterday, N days ago. For lines such as "Synced …" and "Updated 12 min. ago" of a fetched page |
| `RelativeDay(t, now, loc)` | When something was, in calendar days read in `loc`: today's minutes and hours, yesterday, "on Tuesday" for two to six days, "last week" to 13 days, "earlier this month", "in March" this year, "last year", then "in 2023". For dates of the user's things, such as a playlist's "Updated …" |
| `CalendarDays`, `NextMidnight` | The arithmetic under `RelativeDay`, which History's day headings share |
| `FormatDateTime` | The full date and time: the exact time behind a `RelativeDay` label |
| `FormatBytes` | Sizes ("12 kB") |
| `List`, `ListOr` | A hand-keyed table of CLDR's list words |
| `Fold` | Lower case without accents: what a filter folds both sides to |
| `Collator()` | `golang.org/x/text/collate` |

MyGo has no CLDR unit names, so `durations.go` keeps a table of CLDR 48.2.0's words (the version MyGo's tables come from) for 14 languages. It has en-001, en-CA, fr-CA, Traditional and Hong Kong Chinese rows, and English for any other language.

Other helpers: `i18n.Meta`, `SongCount` and `CatalogueLine` (the middle-dot meta line, the song count and the catalogue line, one message each). `i18n.SplitLink` splits a message with a `<0>link</0>` (or `<0>link<1/></0>`) and `i18n.StripTags` drops the markers. `i18n.CDMPercent` is the download's percentage.

### Errors

Go sends codes, never text. `i18n.ErrorText(code)` words a `services.ErrorCode` (an unknown or empty code gets "Something went wrong."). `i18n.ErrorMessage(err)` words an error, preferring Apple's own wording of a sign-in failure Sona has no code for. A test checks that every `Code*` constant of `internal/services/errors.go` has its own text in `en.po`.

### Apple's translations

- `apple-keys.json` maps Sona's msgids to Apple Music's string keys; `apple-keys-rejected.json` records the rejected pairs.
- Both sit in `internal/ui/i18n/locales/` with `en.po`, which Go embeds (package `locales`; `internal/i18n/applestrings` imports it).
- `i18ntool match` proposes mappings, `match -apply` moves reviewed ones in, and `import` converts Apple's bundles for review (gitignored).
- Sona fetches Apple's text at run time; nothing of it is committed.
- `docs/translations.md` has the workflow.

## Theme tokens

`internal/ui/theme/tokens.json` is the one source of colours, type, radii, space, metrics and motion. It is embedded; dev builds read the file, which `theme.Reload` applies on the next frame.

### Tokens

- **Colours**, light and dark: `ground`, `surface`, `raised`, `hover`, `active`, `line`, `control-line`, `ink`, `ink-muted`, `ink-faint`, `amber`, `amber-ink`, `on-amber`, `danger`, `positive`, `avatar-N-bg/fg`.
- **Roles and derived**: the shadcn roles (`primary`, `ring`…), `play`, `play-ink`, `on-play`, and `edge`, `selection`, `scrollbar`, `scrollbar-hover`.
- **Values** are `#rrggbb[aa]`, another token's name, or `name/N` (that token at N% opacity).
- **Light/dark differences are tokens.** `scrim`, `dialog-border`, `input-bg`, `invalid-ring`, `select-hover`, `badge-destructive`, `switch-off`, `switch-thumb`, `switch-thumb-on`, `tab-active-bg`, `tab-active-border` and `link-tile-border` are colour tokens, not `theme.Dark()` branches. Polarity itself (the album sleeve's choice of `palette.Light` or `Dark`, MyGo's base theme) still reads `theme.Dark()`.

### Themes

`tokens.json` is Sona's own theme and the base of every other (`theme.SetDocument` replaces the base in development builds and tests). A theme is data (`internal/ui/theme/themefile.go`):

```json
{"format": 1, "id": "…", "name": "…", "author": "…", "light": {}, "dark": {}}
```

`author`, `light` and `dark` are optional. A variant holds `color`, `scalar` and `shadow` overrides (`site/src/content/docs/reference/theme-format.md`). A variant a theme leaves out is Sona's own, and a theme with one variant pins `theme.Dark()` to it.

**Registry** (`registry.go`)

- The 12 built-ins are `themes/*.json`, embedded, parsed and validated at start-up: `sona`, `graphite`, `paper`, `contrast`, `oled`, then the community palettes `catppuccin`, `dracula`, `gruvbox`, `nord`, `rose-pine`, `solarized` and `tokyo-night`.
- `theme.Themes()` lists the built-ins and the user's themes. `theme.Lookup(id)` finds one. `theme.SetTheme(id)` (beside `SetMode`) chooses; `kit.ApplyAppearance` calls both from the settings (`Settings.ThemeID`, default `sona`).
- `theme.Active()` is the theme in use; `theme.Requested()` is the ID the settings ask for. An ID no theme has shows Sona's own and is kept, so the choice returns with its file.
- `theme.Revision()` counts changes of the tokens (the album page's palette is keyed on it). `(*Theme).Tokens(dark)` resolves any theme for the gallery's pictures (`kit.ThemePreview`), cached per revision.

**Accents** (`accents.go`)

- Sona's own five themes (`sona`, `graphite`, `paper`, `contrast`, `oled`) carry an `accents` map. For each of `amber`, `gold`, `lime`, `sky`, `teal` and `rose` it holds the three spot tokens (`amber`, `amber-ink`, `on-amber`) for each variant the theme gives, designed, not computed.
- `Theme.Accents()` lists the ones offered. A theme's own colour is not offered again, an accent that fails `Validate` in any variant is left out, and a test holds the shipped set whole.
- `Theme.WithAccent(id)` is the theme with the accent laid over; `Theme.AccentColors(id, dark)` gives a swatch's fill and glyph.
- `theme.SetAccent(id)` (beside `SetTheme`) chooses the accent over the active theme; `theme.ActiveAccent()` reports it. `kit.ApplyAppearance` sets it from `Settings.Accents[ThemeID]` (theme ID to accent ID), so switching theme restores that theme's accent.
- A user's theme file with accents is refused.
- Settings → Appearance shows an Accent row under the gallery (`features/settings/accents.go`), only when the active theme offers accents. It is a radio group (one Tab stop, arrows choose) of named swatches, "Theme default" first, with a check in the chosen one's disc.

**Validate** (`validate.go`)

- `theme.ParseTheme(data)` returns the theme or the `Problem`s that refuse it: the format, unknown token names, unresolvable values, and `theme.Validate(tokens)` for each variant given. The floors are written in `docs/design.md`, "Themes".
- Each `Problem` is structured (kind, mode, token, measured and needed values). Settings words it with `i18n.T`.
- A refused theme is never loaded. The tests hold every built-in to the same function.

**User files and hot reload**

- `services.ThemesService` (`internal/services/themes.go`) lists `<config dir>/themes/*.json`: at most 100 files, 256 KiB each. The folder is created when Settings opens it.
- It polls the files' names, sizes and times every two seconds after the first paint and emits `themes:changed` with the files.
- `app.followThemes` lists them once off the main thread at start-up. On every event it calls `theme.LoadUser(files)` on the main thread.
- `LoadUser` parses and validates each file. It keeps the last good version of a file whose edit is refused (`UserEntry.Kept`, with the problems), refuses a theme whose ID another already has (the built-ins first, then files by name), and re-applies the chosen theme, so an edit shows on the next frame.
- The UI window redraws because the bus delivers on the main thread through the poster.

**Settings** (Appearance, `features/settings/themes.go`)

- The Light/Dark/System mode row (off for a one-look theme).
- The gallery: one radio group named Theme (`ui.RadioGroup` of cards). Tab reaches the chosen one; the arrows choose and save at once.
- The user's themes under it. A refused file is a disabled card that says why; a refused edit keeps the earlier version's card with a note.
- The folder row with "Open themes folder" (`ThemesService.OpenFolder`), and a line when the chosen theme isn't there.

### Using the theme

**Reading tokens**

- `theme.C("ink")`, `theme.Current().Type["title"]`, `.Radius["md"]`, `.Metric["sidebar"]`, `.Motion["feedback"]`: pages never hard-code a colour or a size.
- An unknown colour panics in tests and dev builds and is magenta in production.

**Applying the theme**

- `theme.Apply(c)` at the top of every view sets the `ui.Theme` MyGo's widgets draw from, in Sona's colours and font. It builds a new theme only when something changed.
- It follows the mode (`SetMode`: `ModeSystem`, `ModeLight`, `ModeDark`; `Dark()` reports what frames draw).
- It follows the desktop's text scale and high contrast (`Theme.WithPreferences` with those two only): MyGo's stronger borders, muted text, opaque focus ring and bolder scroll bars, and the larger font. The desktop's accent is left out; amber is Sona's.
- `theme.ReduceEffects()` is the setting (no backdrops, solid surfaces). `theme.ReducedMotion()` is the settings or the desktop's preference; the kit skips its transitions under it.

**Scoping colours** (`theme.Scope(overrides, children)`)

- It builds children with some colour tokens replaced; `C` looks in the innermost scope that names a token first. A page printed in its cover's colours uses it to redeclare `ink`, `hover`, `amber`… for what is inside.
- Only colours read while children builds follow it, not ones read inside a `Draw` callback. Pass a map built once, never one per frame.
- `theme/palette` is the album palette: OKLCH steps, the sleeve surface one tone step off the ground, 4.5:1 text pairs, the accent (chroma ≥ 0.06) and `ensureContrast`/`AccentOf`. It works from Apple's `services.ArtworkColors` (`bgColor`, `textColor1..4`) and `Header.Colors`.
- `palette.ForAlbum` gives a `Sleeve` and an optional `Accent`, whose `Overrides()` are the maps for `Scope`. `Themes` there equals tokens.json for the six tokens it checks against (a test).
- `features/album/colours.go` builds them once per cover, theme and layout (cached on the page's state). `body.go` applies the accent to the page and the sleeve's tokens to the sleeve.

**Shared builders** (`text.go`)

- `theme.Text(c, s, style)` is a `ui.Text` in a type token's size, line height, weight and tracking. `theme.StyleText` does the same to a text built another way.
- `theme.Fade(e, "feedback"|"press")` eases an element's colours over a motion token unless motion is reduced. `theme.Spin(icon, on)` turns a loading icon once a second unless it is.
- `theme.Hairline(c)` is the 1 px `--line` rule.
- A feature that needs a text with more modifiers wraps `theme.Text` in a one-line helper; it does not copy the chain.

**Icons**

- `theme.Icon("play")` returns a Lucide icon (ISC, `theme/icons`, the version in its README) by its kebab-case name. The names a React port would use (`ChevronRight`, `Loader2Icon`) resolve through `icons/ALIASES.txt`.
- `theme.Glyph(c, name, size, theme.GlyphOpts{Color, Inherit, Filled, SolidOnly})` is the one seam for an icon that macOS draws as an SF Symbol (MyGo's `plugins/sfsymbols`). Every other name, and every name on Linux and Windows, is `ui.Icon` of the Lucide icon.
- `kit.Button`, `PlayButton`, `MenuItem` and the palette draw their icons through `Glyph`, as do the direct sites of mapped or caller-chosen icons. An icon that is never mapped stays a plain `ui.Icon(c, theme.Icon(…))`.

The macOS symbol rules (`symbols_darwin.go`):

- A name in `theme`'s Lucide → SF Symbol map (`symbols.go`, `SymbolName`; `docs/design.md`, "Icons") is a symbol, at the Regular weight and its natural scale over 1.2 times the glyph's box (the box itself is the Lucide icon's). The Lucide icon is its fallback.
- `Filled` uses the symbol's `.fill` variant, or draws a Lucide icon solid through `theme.IconFilled`. With `SolidOnly` (which a `Button`'s `Filled` sets), only an icon that has a solid form (`theme.HasSolid`: play, pause, star, thumbs-down, moon) is filled.
- A symbol takes no text colour, so `Color` is always given. `Inherit` says it is only the colour around the glyph, which the Lucide icon then keeps taking (with its button's colour transition).
- The glyph is decoration: the symbol's own image name ("play.fill") is cleared, so a button reads its label once. Name a glyph that means something with `Role(ui.RoleImage).Label(…)`.

**Fonts.** `theme.RegisterFonts()` registers Schibsted Grotesk (variable TTF, SIL OFL 1.1, `theme/fonts`) before the first window opens.

## The kit

Components are functions taking the `*ui.Context`: `kit.Button(c, label, kit.ButtonOpts{…})` returns the element, so `.Clicked()` reads as MyGo's. Give repeated elements a stable key, `c.Key(id)`.

### Conventions and shared helpers

**Page helpers** (what the feature pages build on)

- `kit.PageFrame`: embed it in a scroll page's view state. `Scroll(c, identity, body)` is the scroll container with the shell's gutters, the reset to the top when the identity changes and the width tracking. `kit.PageScroll` and `kit.PageGutter` are the same for the pages that size by the window; `PagePadTop`, `PagePadBottom`, `PageNarrow`.
- `Env.Routed(path)`: `HasRoute` that is false in a hand-built Env; the one way a feature asks whether to link.
- `Env.TrackOpts(menus, restricted)`: the base `TrackListOpts` of a song list (art, motion, menu, what plays, links to artist, album and profile pages that exist).
- `ActionMenus.Trigger` (the button and its right-click), `HasMenu`, `kit.MenusIn`; `kit.ShelfSeeAll`, `ShelfDisclosure`.
- `kit.QueryError`, `DetailQueryError`, `BackAction`: an error block whose Try again invalidates keys.
- `store.KeyLibrary…`, `LibraryArtist`, `LibraryAlbum`, `CatalogArtist`, `CatalogAlbum`, `LibrarySearch`: the query keys several features share. Spell them there, as prefix invalidation depends on it.
- `kit.Bar` (a flat placeholder bar), `kit.ParseHex`, `kit.WidthTracker`, `kit.Entry`, `kit.ReleaseTitle`.
- `kit.Fade(e, show)` fades a play button or mark in and out. `kit.HoverRow(row)` gives a list row its hover background and reports whether it is hot. `kit.ToastFailure(what, err)` is the error toast of a failed command.

**Playback helpers** (new files beside what they serve)

- `kit.Transport` is the playback command set that the bar, the mini player and the video view share: `Toggle`, `Next`, `Prev`, `SeekTo`, `SeekBy`, `SetVolume`, `ToggleMute`. The mirror shows a seek or a volume at once, the service runs off the main thread, and a failure goes to `Transport.Fail` (a notice in the mini player, a queued toast in the bar; nil is `kit.ToastFailure`). Each caller keeps its own guards (the bar's `refuse`, the video's `Controls.Seek`).
- The mute button and the M key share `store.Playback.Unmuted` everywhere.
- `kit.SeekDrag` is the seek bar while dragged (`Pos`, `Update`, `Tick`). `kit.PlayPauseIcon`, `kit.VolumeIcon`, `kit.LiveMarker` and `(*ActionMenus).PopupFor` are the bar widgets the three share. `kit.AlertLine` is the sentence with a danger icon, and `kit.InfoLine` the one with an info icon on a quiet wash (a status, for a limit the user should know before they act); `kit.ListenBrainzTokenHint` is the token field's hint.
- `store.Latest[T]` sends values one at a time, the newest last (the equaliser's preview and saves, the lyrics choices). `store.Mirror` and `store.Seed` keep a query in step with an event and a one-off snapshot.
- `module.ShortcutGroups` and `module.GroupTitle` order and name the shortcut groups of the palette, the help and Settings. `lyricsync.SpanCache` and `lyricsync.Dur` serve the sung line and the lyrics sheet.

**The wheel over the volume** (`kit.VolumeWheel`, `volumewheel.go`)

- MyGo hands a scroll to the innermost element under the pointer that takes its input (`HandleInput`, `InputScroll` with `DX`, `DY`, `Precise` for a touchpad and `Momentum`) before any scroll container moves, so no stand-in is needed. The bar and the video view put a `ui.Row` around the mute button and the slider (no role, not a Tab stop) and `Attach` it.
- A notch (`!Precise`) is one `kit.WheelNotch` (2%, 50 notches from 0 to 100%). `Precise` travel is proportional at the same rate, 1% per 25 DIPs (`kit.NotchDIP` = 50, MyGo's Linux notch).
- One rate serves both because on Wayland a high-resolution wheel or a compositor scroll factor reaches MyGo as fractional, `Precise` deltas: the flag cannot tell a wheel from a touchpad.
- `kit.VolumeStep` (5%) is only the step of `player.volume-up`/`down`.
- `Precise` travel is kept in the wheel until it adds up to 1%, and dropped on a turn and at 0 or 100%.
- Whichever of `DY` and `DX` is larger counts. The volume rises where the view moves up or left (the system's scroll direction already applied; right to left flips the sideways one).
- Momentum is taken and changes nothing. Ctrl, Alt and Super leave the event to what zooms.
- The change goes through `Transport.SetVolume`, the slider's path, so the mirror, the service, MPRIS and the saved volume follow.
- `OnScroll` lets the video view wake its faded controls. Nothing runs while nobody scrolls.

### Buttons and controls

**Button**

- Variants: `Default` (ink-filled), `Secondary`, `Ghost`, `Outline`, `Link`, `Play` (round amber) and `Destructive`. Sizes: `SizeXS` (24), `SizeSM` (32), `SizeMD` (36), `SizeLG` (40).
- Icon-only when the label is empty: a square at the size's height, named by `A11yLabel`, which is also its tooltip unless `Tip` says another.
- `Pressed` makes it a toggle (`Quiet` keeps its colours). `Filled` draws the solid play, pause, star and thumbs-down glyphs. `Muted` is the quiet ink of the rating buttons.
- A disabled button is MyGo's 50%. The disabled Play disc keeps `--hover` and faint ink: `DimDisabled(false)` turns the dimming off for it, so it is right over a cover wash too.
- The face eases over 120 ms (the `feedback` motion) unless motion is reduced. The focus ring is MyGo's, in the ring token, on keyboard focus only.
- `IconButton(c, icon, label, opts)` is the icon-only shorthand; `PlayButton(c, label, size)` is the bare amber disc tiles use.
- `CopyButton(c, copy, value, CopyOpts{Label, Done})` (`kit/copy.go`) is a 24px ghost icon button for an identifier next to its value: a click calls `Env.Copy` and toasts `Done`, or toasts the clipboard error. Text people copy is `Selectable()` (`docs/design.md`, "Selectable text").

**Controls** (`kit/controls_*.go`). Every one takes the ring, ink and `--control-line` from the tokens, takes strings through `i18n.T`, and has a `Label` for assistive technology.

*Marks and tooltips*

- `Tooltip(c, anchor, text)` is an ink bubble with an arrow over MyGo's `TooltipBase` (hover or keyboard focus; Escape hides it first). The delay is 400 ms, the `tooltip` motion token set as `Theme.TooltipDelay` by `theme.Apply` (MyGo's default is 0.6 s).
- `Badge` (six variants, `Interactive` for an anchor), `ExplicitMark`, `Tag`, `PreviewMark`, `SpatialMark`, `SDMark`, `UploadedBadge` (an upload among catalogue songs; `UploadedLabel` adds ", uploaded" to its row's accessible name), `Badges` (Explicit, Apple's audio labels from `TraitLabels`, extras), `Separator`, `Skeleton` (pulses unless motion is reduced).

*Fields*

| Control | Notes |
| --- | --- |
| `Input` | 36 high, `--control-line` border, ring border on focus, `Invalid`, `Disabled` |
| `SearchField` | Glass, Clear, Escape |
| `TextArea` | The multi-line field on Input's face: `Rows`, default 3 |
| `Combobox` | Type to filter, Down shows all |
| `Switch` | Default and small, 140 ms thumb |
| `Select` | `SelectOption`, `SelectLabel`, `SelectSeparator`; a long list scrolls; the check ends the chosen row |
| `Segmented[V]` | 2–4 exclusive choices as a group of toggle buttons, each its own Tab stop; `Changed` on every click |
| `Slider` | 4 px track, 12 px ink thumb always drawn, 4 px ring on keyboard focus; `Vertical`, `Step` |
| `ProgressBar` | 0..1; negative is the unmoving empty track |
| `Tabs` + `TabPanel` | Default and `Line`, `Vertical` |

*Header controls* report what was asked and leave state and service calls to the caller: `RatingButtons` returns a `RatingAction`, `LibraryButton` a `LibraryAction`, `PlayButtons` reports `(play, shuffle)`.

*Page furniture*

- `EmptyState` (`FrameBox`, `FrameCovers`; one action `EmptyAction{Label, Run}`).
- `ErrorBlock`: worded from Go's error code by `i18n.ErrorText`; its title and reason are a `RoleAlert`.
- `DetailSkeleton`, `DetailError` (not found, signed out, other).
- `PageHeader`: `PageHeaderCounted` keeps the subline's line while a list's count loads, so the actions beside it do not move.
- `CollectionHeader`: cover, a `Wash` (`*BackdropOpts`) and a `Cover` slot for the motion cover. The header is built inside the media kit's backdrop, which bleeds with it: an absolutely placed child of the header would be drawn over the title and swallow its clicks.
- `Notes` (clamped, "Read more" only when it overflows) and `ArtistLinks` (see [Artists of a song](#artists-of-a-song)).

**Avatars**

- `PersonAvatar` is a round picture or the person's initials (`Initials`: first letter of the first and last word, whole grapheme clusters; 38% of the diameter, semibold) on one of eight tints (`AvatarTone`: FNV-1a of the lower-case NFC name mod 8, `avatar-N-bg/fg` tokens). It is decorative.
- `Avatar` (24, 32, 40 DIPs, muted fallback text, `Badge`), `AvatarGroup` (overlap 8 DIPs, `+n`) and `SonaMark` (amber disc, surface spindle hole) are shadcn's avatar and the brand mark.

### Artwork and motion

**Artwork** shows a DTO's art URL (`/art/<key>/…`) or an Apple template at an exact size. The 1px line at 6% ink (`ArtOpts.Edge`), round (`Round`) and non-square (`Aspect`) boxes follow `docs/design.md`, "Artwork".

*Loading*

- `ArtLoader` (built by the app over `artwork.Cache`) resolves the URL to its template with `artwork.Cache.Template`. It asks for the cover at `size × scale` device pixels and decodes it with `ui.DecodeBitmap` on worker goroutines.
- The scale is that of the window the view builds in. Each window learns its own from its paints, so the mini player on a screen of another density gets covers of its own size once it has painted; its first pass asks at the scale painted last.
- The result reaches the main thread through the Poster. `Bitmap.ReleasePixels` drops the CPU pixels once the cover has been drawn twice.
- Until the cover arrives, and when it fails, a `--hover` square with a 40%-ink glyph shows. A failed cover is asked for again after 5 s and 30 s.

*Cache*

- Covers are kept up to a count (800) and a weight (`maxArtBytes`, 96 MiB: each cover's decoded pixels and the JPEG its bitmap keeps to decode them again). The least recently asked for are dropped.
- The weight limit exists because the count alone let a scroll through a library's whole Albums grid keep every cover (239 MB of Go heap at 2x for 739 albums).
- A cover a window's current pass asks for is never dropped, so a page that wants more than the weight at once keeps it rather than decoding what it shows again.

*Priority*

- 16 covers are in flight at once (their downloads share HTTP/2 connections, `artwork.Options.Fetches`) with one decode per CPU at a time.
- The workers take the most wanted cover first (`artRank`): covers whose box showed in the last frame (`Element.InView`), then the rest of the page (below the fold, a long list's overscan), then covers only prefetched. Each group is served in the order asked. A prefetch never starts while a cover on screen is being worked on.
- `kit.PrefetchArtwork` prefetches: a long `CoverGrid` prefetches the two rows past its built ones, a shelf its next row while it is on screen, and Discover's hero its next card.
- A pass of the view (the builds between two paints; `ArtLoader.Pass` in the app's root view, else a window's first `Artwork` after a paint) that was painted without asking for a cover calls it off. A queued cover leaves the queue, one in flight has its context cancelled, and the artwork cache stops downloading a source nobody waits for any more. Asking again starts it again.

*Options*

- `ArtOpts.Fallback` builds the fallback in place of the glyph (PersonAvatar's initials).
- `Contain` shows the whole picture at the shape its URL names (a banner stays wide) and decodes it at that size.
- `Aspect` is any width over height, raw (Apple's 1060:608 hero crop) or worked out. The box's height is rounded to whole DIPs (`kit.ArtBox`), so the picture lands on whole pixels; a caller that frames the cover in a box of its own (Home's hero banner, Discover's wide thumbnails) sizes that box with `kit.ArtBox` rather than rounding by hand.
- `FluidArtwork` fills its container's width at the aspect of `ArtOpts` (square, 16:9, a banner's) and decodes for the largest width, whose box is rounded the same way.
- `ArtLoader` also loads backdrops (`/art/<key>/backdrop.jpg`, the pre-blurred JPEG from `artwork.Cache.Backdrop`, decoded at its own size) through the same queue, cache and retries.

**Backdrop** is a box whose background is the cover's dominant colour at once, then the Go pre-blurred image at 110% fading in over 300 ms (a new source is decoded before it replaces the last). It sits under a solid scrim of the surface or ground token at `backdrop-scrim`, and draws nothing under Reduce effects. It holds its children, which draw over it.

**MotionPolicy** (`NewMotionPolicy(bus, post, MotionOptions{Files, Setting, Power, Art})`, which the app builds as `Env.Motion`) says whether motion may play.

- It checks the setting, Reduce effects, reduced motion, battery (`services.EventPower`) and a decoder (`media.Codec`).
- The decoder is probed on a goroutine the first time `Allowed` is asked, so GStreamer's start-up and plugin scan on Linux never run on the main thread; motion stays off until it answers. It is "avc" at once on macOS and Windows, where `ui/video` cannot be asked, and a system with no video framework turns motion off at the first open.
- `SetVisible` (window hidden), `Cover` (Now Playing, full-screen lyrics, a modal) and the cover's own `InView` pause playing videos.
- It owns the decoders. It opens them off the main thread (`media.OpenMotion`: the file straight from `motion.Cache.Local`, no HTTP) and closes those whose view stopped building them (a sweep, as there is no unmount).
- `Report` is the native `MotionReport` for Diagnostics.

**MotionCover** is the still `Artwork` with the looping muted video over it.

- It fades in over 300 ms once the player is `Ready`. A system's player loads in the background, so the cover builds the video transparent until its first picture came, and the slot is `opening`, not `ready`.
- The still stays if the video fails (the player's `Err`, which Diagnostics' "Last video" row shows).

**TileMotion** (`TileMotion(c, policy, motion, size, opts)`) is a tile's motion.

- The tile says it is hot (hovered or focused; `Focus` marks focus only). The video opens after a 400 ms dwell and goes when the tile cools; nothing is built at rest.
- One tile plays at a time: a hovered tile takes over from a focused one, an owner that failed or is no longer built is gone, and waiting tiles are woken by the owner's release (nothing polls).
- It opens at the tile's own size for the scale of `MotionOptions.Art` (`motionPx(size, scale)`, as a cover's), so the rendition is never narrower than the tile draws.
- Both draw the video in the still's own box (same size, `Fit(Cover)`, radius and edge), so the motion sits exactly where the still sits. Neither letterboxes, tints nor frames it (Apple's Album Motion Guidelines: `docs/apple-music-reference.md` §4.1, "Motion artwork").

### Artists of a song

`kit.ArtistLinks` and `kit.SongArtists` serve `services.Song.Artists`, `Album.Artists`, `QueueItem.Artists` and `StationSeed.Artists`.

**Data**

- Apple's artists relationship, in Apple's order, as `Artist{ID, Name}` (catalogue artists; `docs/apple-music-reference.md` §4.4, "Several artists"). `ArtistID` stays the first, for code that needs one.
- The catalogue mappers fill `Artists` only when the artists came named (`include=artists`). Every list and page request has it, and `Client.Song` and the batched `Client.Songs` send it. The catalogue cache keys a song as `song-artists`, so a copy cached without names is not read back.
- A "feat." artist is in the title, not the relationship, so it is not offered. A compilation under "Various Artists" has no artist, so its credit is text.
- A queue slot fetched from the catalogue (`PlayItems` of a chart or search, a station's batch, a music video) carries `Artists` from the response.

**Library songs**

- Library songs know their library artist alone, and a queue slot played from a stored row only its first artist's ID. `CatalogService.SongArtists(ids)` (catalogue IDs → artists) is the lookup. It uses `catalog.Songs`'s batching (#284: one request per hundred, the rest read from `catalog_cache`, a week), so nothing is asked per song.
- It runs lazily, never during sync. `kit.SongArtists` (`Env.SongArtists`) is asked as a row, the player or a menu is built. It queues the song's catalogue ID and answers one batched call per frame's worth of asks. The answer is kept for the session and a failure is tried again a minute later.
- Only a song with a catalogue ID whose artist string may name several (`kit.MayNameSeveral`: it holds `" & "` or `", "`, as Apple joins them) is asked about. An answer of one artist leaves the song's first artist as it was. Until the answer comes the song reads as one artist.
- **Not persisted.** The catalogue cache already keeps the copy for a week, a migration would put a second one in `tracks`, and a stored copy would age without Apple's artists changing it.

**`ArtistLinks(c, artists, text, colour, ArtistLinksOpts)`**

- `text` is Apple's artist string and is all that is shown.
- No artists, or `Open` nil (from `Env.ArtistOpener()`, nil when the artist page is not registered): text.
- One artist: a link (`ui.Link`; `Static` makes it plain text with a link role where the list is one Tab stop).
- Several: a menu button (`RoleMenuButton`, `HasPopup` menu, `Expanded` while open, described "Choose an artist") that opens a `kit.Menu` of the artists (`MenuOpts.Start` lines its start up with the text's), each row opening that artist through `Open`.
- Enter, Space or a click opens the menu on its first artist; Up and Down move, Enter chooses, Escape closes and the focus returns to the button.
- A track row's chooser is not a Tab stop (the list is one), and a Ctrl or Shift click on it selects the row, as on its other links.
- `Style`, `Hover` and `Wire` let a surface give it its own type, hover colour and the pointer handling of what surrounds it (the player's song menu).

**Surfaces**

- The player bar's artist (`player.artist`), Now Playing's (it closes the view as the page opens), the video view's caption.
- `kit.TrackList`'s artist cell and `TrackShelf`'s rows (`TrackListOpts.Artists`, `ItemsCtx.Artists`).
- The album header's byline (the catalogue page's `Artists`, else the album's, else its first), the song and music-video pages.
- A station's songs and its seed line. Several artists make that line a wrapping row of its parts, as a menu button cannot sit in text.
- Tile subtitles are text under a tile that is itself the link, so they are unchanged.

**The item menu's "Go to artist"** is an `ActionDef` with `Choices`. With more than one choice the row is a sub-menu (`MenuSub`) with one entry per artist; with one it is the plain row. A library song's artists arrive while the menu is open, from the same resolver, so its row turns into the sub-menu then.

### Overlays and menus

**Dialog**, **DialogFooter**, **ConfirmDialog** (`Confirm` + `ConfirmHost`)

- A modal over a scrim (the `scrim` token: black 50 %, 70 % in dark in Sona's own theme, never blurred). The panel is centred (max 32 rem, radius 8, border, float shadow, 200 ms zoom-in unless motion is reduced) with a top-right X "Close".
- Escape or a scrim click closes it. The page behind is inert, Tab stays inside and closing returns the focus to the opener.
- `Confirm(opts, func(ok bool))` is a confirm with a callback: an alertdialog (no X, max 420) whose confirm button has the focus. Cancel, Escape and asking again settle false. Main thread only.

**Popover** is a panel below its anchor (above where there is no room), in popover colours. A press outside or Escape closes it and the press goes on.

**Menu** (state) and `MenuItem` are the drawn dropdown and context menu: action, checkbox, radio, label, separator, sub-menu, shortcut hint, icon, filled icon, destructive, disabled with a reason line.

- MyGo's `PopoverMenu` has no icons or reason lines, so the kit draws its own on `ui.Overlay`.
- Up/Down/Home/End and type-ahead move past disabled rows. Right/Left open and close sub-menus. Enter/Space choose. Escape (sub-menu first), Tab and a press outside close.
- A menu opened from the keyboard starts on its first row.
- The chosen row runs one frame after the menu is gone, so a dialog it opens returns the focus to the page.

**ActionRegistry** and `ActionDef` share action IDs across menus, shortcuts and the palette (`player.play-next`, `library.favorite.song`…).

- One `ActionDef` is a single action (`Target` a kind: song, album, playlist, artist, station, music-video) or a bulk one (`Target: KindSelection`, `Kinds`).
- Its fields: `Label`, `Icon`, `Filled`, `Group` (queue, item, rating), `Order`, `Available`, `Enabled`, `DisabledReason`, `Data` (`MenuData`, loaded once per ID when the menu opens, off the main thread) and `Run`.
- `ActionContext` carries the items, the data, `Container` and `Navigate`.
- Modules list them in `Module.Actions`; the shell registers them into `Env.Actions` (a duplicate ID panics). `DescribeItems`, `Sections` and `TracksOf` follow the reference.

**ActionMenus** (`NewActionMenus(env.Actions, selection)`)

- An item's "More for <name>" button is `Trigger(c, row, target, opts)`.
  - It is ghost while the row is hovered or focused, outline for a header. Its menu is aligned to its end, 224 wide.
  - **A ⋯ button implies right-click.** `Trigger` takes the element the button belongs to and wires it too. A right-click on it opens the same menu at the pointer. Shift+F10 or the Menu key while the focus is within it open it below it, starting on the first entry.
  - The signature makes the owner an argument, so a surface cannot draw the button through `Trigger` without naming what to wire. The sidebar draws its own ⋯ and wires it with `Menu.OpenOn`.
  - `Popup` draws the point menus, called once per list or page after the rows.
- What `row` is: a list row or tile; for a page header, the box around its artwork and title (a right-click on either, or on any part of the head that takes no press of its own, opens the item's menu; its buttons keep theirs). `TriggerOpts.Select` and `Index` make a right-click on an item outside the selection select it alone first (lists with a `Selection`). The trailing arguments of `Trigger` list children that take the pointer themselves (a title that is a link, a result row's open button): MyGo hands a right-click to the innermost interactive element, so without it the click would never reach `row`.
- `ActionMenus.Row` wires an item that has a menu but no button (the radio board's playing rows, the bar's cover); `Menu.OpenOn(row)` does the same for a `kit.Menu` ⋯ button of its own (the sidebar's pins and folders). Both are for a surface that cannot use `Trigger`; a ⋯ button drawn any other way needs one of them.
- The rule is tested with `access.CheckMenuWays` (`uitest/access`): for an item it opens the menu with the button, with a right-click at the title and at the artwork, and with Shift+F10 and the Menu key, and fails when a way opens no menu, a different one, or a menu away from the pointer. Each surface that draws a ⋯ button has such a check in its package: most in `menus_test.go`, Discover in `front_test.go`, Search in `itemrow_menu_test.go`, and the radio on-air, artist release and Home rows in their older right-click tests.
- A row inside a multi-selection (`SelectionScope.GroupWith`) lists the bulk actions for all of them under "<what> selected" and runs once with every item and `Container.Indices`.
- A list that shows its collection sorted or filtered gives `ActionContainer.Order` (row i is item `Order[i]`), so `Index` and `Indices` always name places in the collection. A positional action (Remove from playlist) needs that.
- Kinds with no registered action get no trigger.

### Developer actions (`features/inspect`)

A development build (`-tags dev`) adds one module, `inspect` (`app/modules_dev.go`), and nothing else: a release build holds neither the module nor the service behind it.

- **One action per kind** (`inspect.song`, `inspect.album`…), registered through `Module.Actions` in the `kit.GroupDeveloper` group, the last section of every menu. Every surface with an item menu (tile, row, header, card; right-click, the "more" button) lists "Inspect" with no code of its own, so right-click coverage follows the item menus (#337).
- **The panel** is a plain `kit.Dialog` built in the module's `Layer` slot: the item's kind, ID and artwork URL, then the mapped DTO, Apple's resource as Go's catalogue cache holds it, and the SQLite row of a library item. The text is a text field whose edits are undone, so it selects, copies and scrolls. Copy as JSON, Copy ID and Copy artwork URL (the item's largest size) follow. It is not translated or designed for end users.
- **The data** comes from `InspectService.Inspect` (`services/inspect_dev.go`, dev tag): the mapped item the UI passes, `catalog.Catalog.Raw` (`catalog/raw_dev.go`, the cache's body, never a request to Apple) and the library row. Every text passes through `logging.Redact`; no request header is ever held, and the UI still never calls Apple. An item whose page Sona has not fetched shows "not in Go's catalog cache".
- **The release check:** `inspect`'s release test asserts `cmd/sona`'s dependencies leave the package out, and `app`'s module-list test (`devModules`) lists no extra module without the tag.

**Toaster** (`ShowToast`, `ToastSuccessf`, `ToastErrorf`, `CloseToast`, `Unavailable`) is sonner's look on MyGo's toast manager.

- Popover colours, success/info/warning/error/loading icons, bottom right 16 from the right and 88 from the bottom, a "Notifications" region and polite announcements. An ID replaces a toast, so a held key never stacks.
- `ShowToast` is safe from any goroutine. The Toaster, mounted with `ConfirmHost` by `OverlayLayer` in the shell, shows it on the next frame.

**Palette** (`PaletteSource`, `PaletteSection`, `PaletteEntry`), `Keycaps`, `RankCommands`, `MatchScore`: the command palette's frame, UI only. A dialog 14 % from the top with one combobox field over a grouped list (Up/Down wrap, Enter runs after the dialog has closed, Esc closes and the focus returns), keycaps and hints. The commands feature supplies the sections.

### Lists, tiles, selection and drag

**CoverGrid**, **CoverRow**, **CoverShelf**, **Shelf**, **CoverTile** (`CoverItem`, `CoverState`) are cover tiles at least 160 DIPs wide, 24 across and 28 down, at an exact width (`CoverColumns`) so artwork is decoded at the size it shows.

- `CoverGrid(c, st, count, at, opts)` takes `at(i) *CoverItem` (nil: a loading tile).
- With a `Pane` (the page's scroll container: its `ScrollState` and bounds), a grid of more than 200 tiles builds only the rows in view and three beyond (the row being dragged stays built). It reports the tiles built through `OnRange` and keeps the same tiles at the top when the columns change.
- A tile is one focusable source: click or Enter opens; Ctrl/Shift+click select; arrows move; Shift+arrows extend; Ctrl+A; Escape. Its play button, motion cover, release badge, explicit mark and `ActionMenus` more button belong to it.
- `CoverRow` shows as many tiles as fit in `Rows` rows. `CoverShelf` is a `Shelf` (`See all`, or `ShelfToggle` Show all/fewer in place) over a row.
- A tile's play button and more button are elements beside the opener, not inside it (`DragOpts.After`). A button or link is a leaf on the accessibility tree, so controls inside it would be unreachable by assistive technology. They are always on the tree and Tab reaches the play button after the opener.

**TrackList** (`TrackListState`, `TrackListOpts`, `TrackColumns`, `TrackSortState`) is the track table.

- The header has sortable Title/Artist/Album, and Duration when `TrackSortState.Duration` is set (a list that sorts itself). `TrackSortState.Toggle` is the click's cycle for such a list: ascending, descending, then none, the list's own order.
- `TrackListOpts.Rank` (a `*TrackRank`) turns the leading "#" column into rank numerals: 56 DIP wide, the `title` type style (24/30) in the ink colour, tabular figures, right-aligned (`RowNumbers` is ignored). The hovered row still swaps the numeral for Play. Rank is the position in the list as given; a host that sorts the rows itself sets `TrackRank.Of(i)` to return the song's own chart position, so the numerals travel with the songs (nil `Of`: the numerals follow the shown order, so a re-sorted list is numbered afresh). Discover's charts (the overview's Top songs and `/charts/songs`) use it, unsorted, so rank is the chart position.
- Rows are 44 DIP. It also has disc and work headings, `Popular`, "Added by", the current-song mark, skeleton rows, and highlight and scroll to it.
- `TrackListOpts.HideUploadMark` leaves the "Uploaded" badge and the ", uploaded" name off uploaded rows (a list made only of uploads); the row's Play button carries the same name.
- One Tab stop (only the focused row is focusable). Arrows, Page Up/Down, Home/End move it; Enter and double-click play; Ctrl+click/Shift+click/Shift+arrows/Ctrl+A select; selected runs draw as one block.
- More than 200 rows in a `Pane` are built by row (12 beyond).
- `TrackListState.BlockTop` and `ScrollToBlock` give the album's disc index its jump (work headings are measured from the last frame, so wrapped ones count).

**Selection** (`SelectionState` pure model, `Selection` per list, `Rows`, `RowKeys`, `KeysOf`)

- Keys survive re-sorting. A press outside the list clears the selection (an open menu, a pending `Confirm` and the list's own `Hold` option keep it).
- `GroupWith` makes it a `SelectionScope` for `ActionMenus`.
- **GridNav** is the keyboard navigation of a list or grid: arrows, pages, ends, and focus requests that wait for a virtual row to be built.

**Drag** (`DragItem`, `DragPayload`, `DropTarget`, `DropOn`, `DragActive`, `DragPinned`, `DropIndex`) runs over MyGo's value drag.

- A press becomes a drag after 6 DIPs. The item stays drawn by a twin while MyGo's copy of it is empty.
- The ghost card (artwork, title, caption saying what a drop does) is built in an `Overlay` 14 DIPs right of and 10 below the pointer, flipped near the window's right edge.
- Targets accept or decline, describe the drop and get `Hover`. A declining inner target lets the nearest accepting one around it take the drop.
- Escape cancels. The scroll container under the pointer scrolls near its edge (MyGo's, 32 DIPs).
- One engine per process, main thread only.

### Media widgets

**VideoFeed and VideoSurface** show the playing music video.

- A goroutine takes the latest picture from `video.Surface`, checks that its planes are whole and hands it to the main thread, which updates one bitmap in place with `Bitmap.UpdateYUV`. Pictures are not handed over while the view is not drawn.
- The I420 planes are as the child sent them (BT.709 limited range), converted by the GPU; the CPU renderer and captures convert as they need it. Sona has no CPU conversion of its own; the main thread only interleaves the chroma planes for the GPU.
- A frame whose planes are shorter than its size says is dropped (`UpdateYUV` panics on one). Memory only, never a backlog.
- `Restart` forgets the picture and takes only pictures shown from now on (a new queue entry must never show the one before's last picture). `HasPicture`, `PictureSize` and `Pictures` (a count, for stall detection) are what the view reads.

**GradientText and SweepText** are text with a fill that sweeps along it: the text drawn again over itself clipped to the reached part, then eight slices of a soft edge (half an em). They are shaped once and painted from the clock.

- `SweepText` is the word-timed lyric line. It is shaped in the lyrics' language (the part's; the romanisation's for the pronunciation), as a `Text` with that `Lang` is: Han characters take that language's glyphs and Korean lines break between words (MyGo's keep-all for `ko`). The mini player's sung line carries the lyrics' language the same way.

**EqualiserBars** is the now-playing mark: three amber bars, each rising and falling between a quarter of its height and all of it on a period of its own (smootherstep legs: quick through the middle, slow at the turns). They move only while playing, uncovered and with motion not reduced.

- Each bar is drawn once at full height and squeezed from its foot with `Painter.Transform(ui.Scaling(1, level), …)`. `Painter.Fill` snaps a rectangle's edges to whole device pixels, so a bar resized per frame would step a pixel at a time; a transform is applied to the pixels, so the top edge is anti-aliased and moves by fractions of a pixel.
- `Painter.AnimationFrame` asks for the display's rate: only the glyph's own Draw callback is painted again, and the page is not built, laid out or painted whole.

## Testing with `ui.Tester`

`ui.NewTester(view, width, height)` runs a view without a window or a display, in `go test`:

- `tt.Click("Save")`, `tt.Key(0, ui.KeyTab)`, `tt.Focused(…)`.
- `tt.Find(text-or-label)` for a box; elements are found by their text or `Label`, so every interactive element has a translated label.
- `tt.Advance(d)` moves its clock (animations, `c.After`). `tt.SetScale`, `tt.SetDark`, `tt.SetPreferences`.
- `tt.Image()` is the last frame.

### Patterns

- Views take what they need from a struct (the feature's state), so a test builds the struct and drives `tt` against its view. Call `theme.Apply(c)` first, and `theme.SetMode` for light or dark.
- Off the main thread, tests give `store.Client`, `store.Bus` and `kit.ArtLoader` a Poster that queues funcs and run them on the test goroutine, as the app runs them on the main thread (`-race` is part of `tests`). Store tests move a fake clock (`Client.now`) instead of sleeping.
- A test never sleeps for the UI: it advances the tester's clock, which stands still until `Advance` but starts at the wall clock (line a pulse up with `tt.Now()`). Goroutines are waited for, not slept for: a post (`MainThread.Wait`, `Until`), a channel the fake closes, `ArtLoader.Pending()`, or a `testing/synctest` bubble with `Session.Quiesce`. Deadlines only guard against a hang (`docs/testing.md`, "Timing in tests").

### `uitest`

`internal/ui/uitest` wraps the tester for features. `uitest.Env(t)` (or `uitest.New(t, opts...)`, whose `Harness` also hands out the database, the engine and the clock) returns a `module.Env` over:

- a real `services.Core` on a temporary SQLite library seeded with the synthetic fixture (`uitest/fixture`: 24 invented albums by 12 invented artists, three of them singles and three explicit, their songs and three playlists; never Apple's data);
- `playback/fakeengine` under the real coordinator;
- an artwork cache whose HTTP serves generated covers (`fixture.CDN`);
- a Poster that queues what goroutines post until the test drains it (`Harness.Main`), and a fake clock (`Harness.Clock`).

Options: `Albums(...)` for a library of your own, `Empty()`, `Bare()` (no services: the bus, the playback mirror and the artwork loader only), `Dark()`. `Env` sets `MYGO_GPU=0` (the CPU renderer), English and the light theme, and restores the theme afterwards.

`h.Tester(t, view, w, h)` (or `uitest.Tester(t, env, view, w, h)`) is a `ui.Tester` plus:

- `Until(what, cond)`: drains and draws frames until the goroutines delivered, `uitest.HangGuard` (30 s) at most.
- `Shows(texts...)`, `Drain()`, and `Settle()` (frames until the picture stops changing).

Other helpers: `uitest.Transport` (a recording fake of the playback commands, over `uitest.Recorder`), `uitest.Nav`, `Harness.Play` (a playing song seeded into the mirror), and the `Session` helpers `MustClick`, `Want`, `Lacks`, `Emit`, `Hover` and `Pump`. The Tester's own `Click` tries every control of the name.

### `uitest/access`

`internal/ui/uitest/access` reads what assistive technology sees of a `ui.Tester` window (roles, names, descriptions, states, actions; `access.Read`). It lives beside `uitest` rather than in it so the kit's own tests can use it.

- `access.NoNestedControls` fails for a control nested inside a button or link, which MyGo reads as one piece and so hides.
- `access.TabWalk` walks Tab round the window and fails for a stop that is not on the tree or a trap.
- `access.InOrder`, `access.StopLabels` and `Tree.MustFind` serve the checks every feature repeats.
- Each feature package keeps its pages' tree, Tab-order, Escape and announcement checks in `a11y_test.go` (`site/src/content/docs/contributing/accessibility.md`).

### `uitest/overlay`

`internal/ui/uitest/overlay` asserts where one mark lies against what it sits on, beside `uitest` for the same reason as `access`. `overlay.Same(t, what, overlay, target)` fails unless the overlay's bounds equal its target's; `OnArtwork` finds the target in the picture (the box of the fixture cover's saturated pixels) instead of the layout; `Inside`, `InsetBottomLeft`, `Centred` and `WholePixels` check a disc in its corner, a glyph in its button and every edge on a device pixel. Each overlay that sits on artwork (a track row's wash, a tile's play disc, a home row's wash, the sidebar's shuffle button) has a test running them at scale 1 and 2; `docs/testing.md`, "Overlay checks and the marks contact sheet", lists them and the contact sheet of 4× crops of every small mark.

### Golden images

`go test ./internal/ui/...` takes seconds. Golden images are few. `uitest.Golden(t, tt.Image(), name)` compares with `testdata/golden/<name>.png` beside the test.

- It compares the mean colour of 8×8-pixel blocks, so another machine's text antialiasing matches but a moved edge, a changed colour or a missing cover does not. A block differs when a mean channel is more than 12 off; the test fails when more than 0.3% of the blocks differ.
- `go test ./internal/ui/app -update` rewrites them when a change is on purpose.
- A failure saves the picture and a diff (differing blocks red) in the test's artifact directory: `$SONA_TEST_ARTIFACTS/<test name>` when that variable is set (as in CI, which uploads it), else `t.ArtifactDir()` (see `go test -artifacts -outputdir DIR`). Tests ask `internal/ui/uitest/artifacts` for it, and only when they fail, so passing runs stay cacheable.
- Golden images are compared on Linux only. Pango shapes the text there; Core Text and DirectWrite place glyphs differently, which a tolerance should not hide. Elsewhere `Golden` skips the test with that reason, after the test's behaviour assertions have run.
- Today the app's goldens are the shell with Albums, light and dark, at 1132×1253, and the marks contact sheet (`internal/ui/app/testdata/golden`).

## The development loop

`devenv shell -- dev` runs `cmd/devloop` on `./cmd/sona`; `devenv shell -- gallery` runs it on `./cmd/uigallery`.

```bash
devenv shell -- dev                # the app
devenv shell -- gallery            # the component gallery
devenv shell -- dev -- <app args>  # arguments after -- go to the app
```

The loop builds the target with `-tags dev,sqlite_fts5` into `bin/<name>-dev`, starts it, and watches `internal/`, `cmd/`, `build/`, `go.mod` and `go.sum` by polling (100 ms).

- A change to a file of a package the target is made of (`go list -deps`, so a sibling tool's files build nothing) builds again while the old app keeps running.
- A build that succeeds quits the app with SIGTERM (so it saves where it was), installs the binary and starts it with the same arguments and environment.
- A build that fails prints the compiler's output and leaves the app alone.
- `r` builds and restarts at once; `q` quits the app and the loop. The app quitting by itself (Quit from the tray or MPRIS; closing the window only hides it to the tray by default) ends the loop. A crash waits for the next change.
- `-ldflags ""` keeps DWARF for a debugger. `devloop` prints how long each restart took, from the save to the app's `startup: first paint`.

### Speed

A restart is the compile of the changed packages and their importers (about 1 s for a change under `features/library`), the link, and the start.

The link is cgo's. SQLite and the playback host's C++ shim make Go call the system C++ compiler, and before linking Go asks it half a dozen times whether it knows a flag by linking a trivial program (0.3 s each under nix's compiler wrapper, 1.5 s in all). `devloop` builds with `-ldflags "-w -extld=<itself>"` and answers those probes from a file after the first time (`os.UserCacheDir()/sona-devloop/extld`). `-w` drops DWARF (another second).

### Restore

`internal/ui/devstate` and `app/restore_dev.go` (dev builds only) bring the app back to where it was.

- When the app quits it writes `$XDG_STATE_HOME/sona/dev-restore.json`: the route and query, the scroll offset of each page, the flags pages registered (`devstate.Flag("queue.open", &q.open)`, for an open overlay or aside) and the window's size and place (Wayland gives no position).
- The next start applies it before the first frame, so a restart lands on the same screen and offset. A page calls `devstate.Scroll(c, &pos)` before the container that tracks `pos`; the offset is asked for until the page is tall enough (its data arrives after the first frame).
- `SONA_DEV_FRESH=1` ignores the file.
- Another dev-tagged program calls `devstate.SetName("<name>")` first and gets `dev-restore-<name>.json` (the gallery's is `dev-restore-uigallery.json`), so it never overwrites the app's.

### Two worktrees side by side

In a dev build, `SONA_DEV_INSTANCE=<name>` (letters, digits and underscores kept, the rest become `_`; `internal/devinstance`) separates the instances:

- The restore file moves to `sona-<name>/dev-restore.json`.
- `.instance_<name>` is appended to the single-instance D-Bus name (`dev.coredev.Sona`) and to the MPRIS name (`org.mpris.MediaPlayer2.sona`). Each worktree then has its own primary instance; `sona status` and the other commands reach the one the same variable names; `playerctl -p sona.instance_<name>` reaches the right player.
- A production build ignores the variable.
- Data, settings and the cache are not separated: give each worktree its own XDG dirs (`docs/development.md`, "Live playback and the session bus").

### Live tokens and catalogues

A dev build reads `internal/ui/theme/tokens.json` from the source tree at start (`SONA_SOURCE`, which `devloop` sets, else where the compiler found the source). It watches that file and `internal/ui/i18n/locales/*.po` and calls `theme.Reload` or `i18n.LoadCommunity` on a change. The next frame shows it, with no build and no restart. An invalid file is logged and the old tokens stay. `devloop` does not build for these files.

### The gallery

`cmd/uigallery` shows every kit component, light and dark (the toolbar's switch), at a scale (the toolbar's slider, 50–200 %), with knobs. The sections list in the sidebar. Under `devloop` the gallery restarts on its section, mode and window like the app.

A kit PR adds its own `cmd/uigallery/<area>.go`:

```go
func init() { register(section{name: "Tabs", build: func(c *ui.Context, g *gallery) { … }}) }
```

`g.dark` is the mode and `g.art` is a `kit.ArtLoader` that serves a generated cover for any `src` (`coverURL(n, delay, fail, nonce)` makes one that loads late or fails). `helpers.go` has `text`, `heading`, `note`, `labelled`, `knob` and `panel`.

## Conventions

- Views are methods on the feature's state struct; build values (`ui.Element`) are never stored. `go tool mygo vet internal/ui` in `check` enforces this: it takes the directory, and runs `go vet` and MyGo's analyzers over everything under it.
- Off the main thread: service calls, decoding, file I/O. Back through `store` or the Poster only.
- Logs never carry tokens or personal data (`AGENTS.md`, "Hard rules").
- Performance rules (`AGENTS.md`, "Hard rules"): no layout, text shaping or image work per frame; `Scroll` and an own virtualisation for long lists; artwork at the exact size, off the main thread; backdrops are pre-blurred in Go.

## The app shell (`internal/ui/app`)

### Start-up and windows

`main` builds the core, then `mygo.App.WhenReady` calls `app.New(Options{Core, Settings, Bus, Windows, Art, Log, Version, Dev, Icon})` on the main thread. `mygo.App.Run` returns when the window closes, and `App.Close` releases the art loader and writes a pending window size.

Two things exist before the core because the services keep them:

- The event bus (`store.NewBus`, `Deps.Emit`): events emitted before the window exists queue and flush in order.
- The window control (`app.NewWindows`, `Deps.Windows`, `Focus` and `Focused`): `Windows` implements `services.Windows` for the main window and does nothing until `New` attaches it.

`Deps.Clipboard` is MyGo's clipboard (`mygo.Clipboard.Write`); `Deps.OpenURL` is `mygo.Shell.OpenExternal`.

`New` builds, in this order:

1. The `Env`: the bus with `Window.Update` as its Poster, `store.Client`, the `store.Playback` mirror, the `kit.ArtLoader`, and `Env.Post` (the same Poster, for a goroutine to hand a result to the view). With services it also builds `Env.Queue` and the one `Env.ItemStates`.
2. The modules of `modules.go`.
3. The router and the shortcut dispatcher.
4. The main window: the remembered size and maximised state from `config.Settings.Window`, 940×600 minimum, title "Sona". The inspector opens only in a dev build with `SONA_INSPECTOR=1`.

Then, on the main thread, it follows the settings (theme mode, reduce effects, the language), seeds the mirror from `PlaybackService.State` and `Queue`, and runs every module's `Init`.

**Layout**

- The sidebar (232 DIP, `surface`): the mark and "Sona" in its 48 DIP header, the primary links, then a scrolling middle with the "Library" heading, the library links and the modules' `SidebarSection`s, then the `SidebarFooter`s and the footer links.
- A 1 DIP `line`.
- The main column: the top bar (48 DIP, `surface`: MyGo's back and forward buttons on the router, the `TopBarSearch` slot, room to grab) over a 1 DIP `line` over the page row. The page row holds the routed page on `ground`, the `Aside`s, and each open `Overlay` covering the page. The page stays built, hidden, so it keeps its scroll.
- The `Bottom` slots run across the window under both. A `Layer` is drawn over everything and, while its `LayerOpen` holds, the window under it takes no input.
- A page owns its scroll container.

**Page changes** fade (`ui.TransitionFade`, 150 ms), as MyGo's default does with less motion. MyGo's default otherwise slides a page nested in the one before by path. For you is `/`, which every path nests in, so every change from or to For you would slide while most others fade.

**Links** are `ButtonBase` with the link role, active on their page and below it unless `Exact`. The keyboard and the screen reader get them by their label.

**Motion**

- `New` builds `Env.Motion` over the motion cache (`Options.Motion`, `Deps`' `motion.Cache`), the Motion artwork setting (read live: `followSettings` keeps it current), `MotionService.Power` and the art loader.
- The window's show, restore, hide and minimise events call `SetVisible`.
- At the end of every frame the shell holds one `Cover()` while a `Layer` slot is open, an `Overlay` slot is open or any `kit.Dialog` (the confirm host and the palette included) was built (`kit.TakeDialogsBuilt` counts them), so the videos under a modal pause.
- `MotionService.Report` gets `Env.Motion.Report()` for the bug-report bundle. The shell sends it whenever it changed, from three seconds after the first frame (building it starts the decoder probe, which must not compete with the first paint).
- `App.Close` closes the policy and its decoders.

**First paint.** The first frame the root paints calls `SystemService.FirstPaint()` once, off the main thread. It logs `startup: first paint` (`since_start_ms`) and opens the network gate.

**Language.** The Language setting (else the system's languages, the first that Apple Music's translations cover to `i18n.SelectableShare`) is looked up with `TranslationsService.Apple` off the main thread and shown through the Poster (`SetApple`, `SetLocale`). A new `EventAppleTranslations` or a changed Language setting chooses again.

**Window size.** The main window's `OnResize`, `OnMaximize`, `OnUnmaximize` and full-screen events feed `observeWindow`. The size is only taken while the window is neither maximised nor full screen, so un-maximising returns to it. A change is written to `Settings.Window` half a second after it stops, and on close. Wayland gives no position, so none is kept.

**Windows and the tray** (`windows.go`, `tray.go`, `shell.go`)

- `Windows` (`services.Windows`) runs every call on the main thread (`mygo.RunOnMain`).
- Closing the main window or the mini player (`OnClose`) hides it and keeps playing when there is a tray and Settings → General says "background"; otherwise, and always without a tray, it quits.
- `Windows.Quit` (the tray, MPRIS Quit) and any app quit (Cmd+Q, the app menu, SIGTERM; `OnBeforeQuit`) let every window close.
- `ShowMain` (the tray, MPRIS Raise, a second launch, the Dock icon through `OnActivate`) hides the mini player and shows, restores and focuses the main window.
- The mini player (`Slots.Mini`, the first module's) is a 420×132 DIP frameless, fixed-size, always-on-top window that never maximises. It is made on first use and then hidden, never destroyed. `ToggleMini` shows it in the main window's place, and `post` redraws it beside the main window while it shows.
- `Env.Window` follows the main window's `OnFocus`/`OnBlur` and full screen, with a redraw on each.
- `Options.Hidden` (`--hidden`) creates the window hidden when there is a tray. The network opens at once (no first frame comes); motion and `Env.Window.Focused` start off.

The tray (`mygo.NewTray`, made before the window; libayatana-appindicator3 on Linux, a template menu bar icon on macOS):

- Its menu: the song, Play or Pause, Next, Previous, private listening, Show Sona, Mini Player, Quit Sona. It is written again on `playback:state`, `private:changed`, the mini player's state and the language.
- Items are in title case on macOS, as menu bar items are ("Not Playing", "Private Listening"), and in sentence case on Linux and Windows ("Not playing", "Private listening"). `trayState.titleCase` (set from `runtime.GOOS`) picks between two messages of the catalogue, the way the mini player's "Mini Player" and "Mini player" are two, so a translation of either is kept and neither is derived from the other.
- A primary click on the icon shows Sona (`OnClick(ShowMain)`: KDE's click, GNOME's double click). The icon is destroyed on `OnWillQuit`.
- `Windows.HasTray` is `Capabilities.Tray`. MyGo reports a tray whenever libayatana-appindicator3 loads, so on GNOME without the AppIndicator extension the icon is invisible and a `--hidden` start stays invisible until a relaunch or MPRIS Raise.

**Title bar.** `windowchrome.Platform.Resolve` of the Title bar setting picks, as the window is created:

| Setting | Result |
| --- | --- |
| macOS | Buttons inset (`TitleBarHidden` with `TrafficLightPosition`, centred on the 48 DIP top bar) |
| `sona` | A hidden title bar with MyGo's window buttons in the desktop's layout (`TitleBarHeight` 48) |
| `sona-no-buttons` | The same with `HideWindowButtons` |
| `desktop` | The desktop's own |

"Automatic" is the desktop's own on tiling compositors, unknown desktops and KDE Plasma under Wayland, where KWin decorates a window whose title bar is hidden.

- With a hidden title bar the top bar and the sidebar's header drag the window (`DragWindow`, a double click maximises). They keep `c.TitleBar()`'s room for the buttons, swapped by direction (`PaddingStart`/`PaddingEnd`). The video view's header does the same and drags the window while it is the top strip.
- The mini player's Close goes on the side `gtk-decoration-layout` puts it (KWin's kwinrc first on KDE; read through GTK in `cmd/sona/gtklayout_linux.go`).

**Status banner** (`banner.go`): one `RoleStatus` line under the top bar for the keyring waiting or unavailable (with `account.KeyringRetry`), a token Apple stopped accepting (Sign in), Apple refusing Sona (Diagnostics) and offline.

**System now playing.** MyGo's now playing stays unlinked on Linux, where MPRIS is Go's only player. On macOS and Windows Go's `internal/nowplaying` drives `mygo.NowPlaying` (media keys, Control Center, the volume flyout), wired in `cmd/sona/nowplaying_desktop.go`.

### Routes, links and history

**Routes and the start page**

- Every module's `Routes` are matched in registration order by one `ui.Router.View`. A path nothing matches replaces itself with the start page.
- The start page is, in order: `Options.StartPath` when given; `Desktop.StartPage` when a module registered exactly that route; the first of `config.StartPages` (Home (`/`) first) a module registered; else the first sidebar link (primary, then library, then footer, by `Order`).
- The window opens on `Desktop.LastRoute` instead when `Desktop.ResumeLastRoute` is on and a module still shows that route.
- At a first start (`Options.FirstRun`) it opens on `/welcome`, which opens the first-run setup and replaces itself with the start page.

**Route memory.** A route that stays a second (`routeSaveDelay`, counted on the frame clock) goes to `DesktopService.SetLastRoute`, which keeps it only while resuming is on. `/welcome` is never kept.

**Deep links**

- `services.EventAppOpen` comes from the command line, a second launch, and on macOS `mygo.App.OnOpenURL`. That handler is registered only in `cmd/sona/openurl_darwin.go`: MyGo's launch-argument scan would deliver each launch link twice on Linux and Windows once URL schemes are declared.
- The event pushes the `routeForOpen` route (album, a song within its album `?song=`, song, artist, playlist, music video, station, curator) when a module shows it, and is ignored otherwise.

**History gestures.** Back and forward are MyGo's router's.

- The top bar's buttons, Alt+Left and Alt+Right (Cmd+[ and Cmd+] on macOS), and the keys `KeyBack` and `KeyForward` (a mouse's side buttons, keyboards that have them, and on macOS the swipe MyGo reports from `swipeWithEvent:`).
- AppKit sends that swipe for the three-finger choice of System Settings → Trackpad → More Gestures → "Swipe between pages".
- The router takes the keys with the focus anywhere, in a text field too. They do nothing behind a modal dialog or layer.
- While a module `Overlay` (Now Playing) is open, a change of the page's location (the query too: a second search is another entry) closes it (`Overlay.Close`): the page that came is behind it.

**`kit.Swipe`** (the root's `HandleInput`) covers the other macOS choices.

- "Scroll left or right with two fingers" (the default) and "Swipe with two or three fingers" send scrolling and no swipe, so `kit.Swipe` turns that into the same two moves.
- A precise (touchpad) scroll that travels 12 DIPs and then at least twice as far sideways as up or down is a swipe. It goes back (fingers to the right; to the left in a right-to-left window) or forward (the other way) once it has travelled 120 DIPs, one page per gesture.
- Momentum events never count, and a gap of 200 ms ends a gesture where the system sends none.
- It is on only on macOS, and only while the setting lets a two-finger scroll swipe. The shell reads the `AppleEnableSwipeNavigateWithScrolls` default (`defaults read -g`, on when absent) off the main thread at start and each time the window takes the focus (`refreshSwipe`).
- A gesture that begins over a container that scrolls sideways and can scroll that way (`kit.ScrollSideways`, which a shelf wider than its box must use instead of `ui.ScrollHorizontal`) is the container's.

MyGo limits this works around, for a maintainer's upstream report:

- `InputEvent` has no gesture phase (began, ended, momentum began; `NSEvent.phase`), so a gesture is inferred from the event times.
- It has no `isDirectionInvertedFromDevice`, so the direction is right for macOS's default "Natural scrolling" and swapped with it off.
- There is no API for `NSEvent.isSwipeTrackingFromScrollEventsEnabled`, the system's choice of swipe gesture, which Sona reads from the defaults instead.
- `Tester` cannot send momentum events (`Scroll` is precise, `Wheel` is not, and neither sets `Momentum`), so momentum is tested on `kit.Swipe` directly.

### Shortcuts and the macOS menu bar

**Dispatch**

- One dispatcher asks MyGo for every chord each frame (`c.Shortcut`), with the user's `config.Settings.Shortcuts` over the defaults. It runs the first shortcut bound to a pressed chord whose `When` holds. An `Unavailable` one takes the key, toasts and announces why.
- MyGo gives a focused text input the keys it types, and a focused button or slider Enter, Space and the arrows, first.
- Each link gets `nav.<path>` (`nav.library.albums`): Ctrl+1…9 by position in the primary section, Alt+1…9 in the library's, unless the item names `Keys`. The shell's own is `shell.sidebar` (Ctrl+B).
- Back and forward are MyGo's (Cmd+[ and Cmd+] on macOS, Alt+Left and Alt+Right elsewhere, the mouse's side buttons), not shortcuts of ours.
- MyGo reports a key held down as presses, so `Shortcut.Repeat` has no effect yet.

**macOS keys**

- A focused text field takes Ctrl+B/F/P/N/H/D/A/E/K itself (AppKit's emacs bindings). So the default chords written `Ctrl+X` are bound as `Meta+X`, Command (`platformKeys`, applied to the registered defaults only; labels follow: ⌘, spoken "Command"). A binding the user saved is kept as written.
- A few commands whose keys are lone letters gain a Command chord on macOS only (`macKeys` in `shortcuts.go`, added by `platformKeys` after the others), so that the menu bar can show a key for them: shuffle, repeat, Now Playing, Lyrics, Queue.

**Rebinding**

- The dispatcher rebinds on every `settings:changed` (`dispatcher.rebind`): a saved binding applies at once, no restart.
- `Env.Shortcuts()` lists every registered shortcut (modules', navigation and the shell's) as a `module.BoundShortcut`: the `Shortcut` and the keys it answers to now (the user's binding or its defaults), in registration order. Settings → Shortcuts and Find a setting read it.
- Settings → Shortcuts refuses the router's own back and forward keys (`routerChord`) with a reason, as it refuses Tab: the router takes them before the dispatcher, so a shortcut on one would never run.

**The macOS menu bar** (`menubar.go`; macOS only: Linux and Windows draw no menu bar)

`buildMenuBar` is a pure function from `menuInput` to a tree of `menuEntry`. `menuInput` holds `Env.Shortcuts()`, the sidebar's pages, what plays, private listening, whether the history can go back or forward, and the sidebar, mini player and full screen state.

| Menu | Items |
| --- | --- |
| Sona | About, Check for Updates…, Settings…, Account Settings…, Services, Hide, Hide Others, Show All, Quit |
| File | New Playlist, Open Link from Clipboard…, Close Window |
| Edit | The edit roles (which MyGo enables while the focus can act on them), and Find…, which focuses the search field |
| View | The sidebar, Now Playing, Lyrics, Queue, Video, full screen |
| Go | Back and Forward, the sidebar's pages in its order (each with the key it answers to now), and the Library pages in a Library submenu |
| Controls | Play or pause, next, previous, skip ahead and back, favourite, shuffle, repeat, volume, mute, Private Listening, Sleep Timer |
| Window | Minimize, Zoom, Mini Player, Equaliser, Sona, Bring All to Front, the window list |
| Help | Keyboard Shortcuts, Command Palette, Report a Bug… (opens Settings → Diagnostics) |

- `TestEveryShortcutIsInTheMenuBarOrLeftOutOnPurpose` fails when a module registers a shortcut that is in no menu and not on its short list of keys of a page's own (Escape to close the overlay, the lyrics timing).
- An item runs the shortcut's own `Run` (`invoke`, what the dispatcher runs; the palette is the same). It is disabled while its `When` does not hold or it is `Unavailable`, and takes its key equivalent from the keys bound now, so a rebinding in Settings → Shortcuts changes it (`settings:changed` refreshes the bar).
- A menu takes its key before the window sees it, so a key shown in a menu is run once, by the menu, and the window's dispatcher does not see it.
- The text fields are MyGo's, not AppKit's, so only a Command chord that no text field edits with is a key equivalent (`macAccelerator`). A lone key (Space, `S`, `?`), Shift or Option with one, and Command with an arrow, Home, End, Page Up or Down, Backspace or Delete, stay the dispatcher's and show no key.
- Back and Forward are the router's own (Cmd+[ and Cmd+]). They are no shortcut of Sona's, so Settings → Shortcuts does not list them and their items show the router's keys as they are.
- Labels go through `i18n.T`, so they follow the language.
- `menuBar` writes the model into MyGo's menus (`App.SetMenu`). When the playback mirror, private listening, the settings (bindings), the language, the mini player, the sidebar, full screen or the history change, or an item was chosen, it writes only what differs (`SetLabel`, `SetAccelerator`, `SetEnabled`, `SetChecked`).

## Features

Each feature is a package under `internal/ui/features` with one `modules.go` line.

### The player bar (`features/player`)

The bar is a `Bottom` slot: 72 DIP, `surface`, over a wash of the cover, the transport deck of `docs/design.md` (Shell). It draws `store.Playback`.

**Layout**

- The seek bar runs along its whole top edge (`seekStrip`): a `ui.SliderBase` out of the flow, 12 DIP of hit area, a 3 DIP track (5 under the pointer or the keyboard) and a 10 DIP thumb that is always drawn. A live station has a 1 DIP `line` there instead.
- The cover (`kit.Artwork`, 48 DIP), the title and the line under it.
- The line under the title is artist · album. Each is a link to its page when that page is ported, and they are independent of each other. The album links to `/album/{id}` without the song marked, since the title's link marks it. In Tab order: artist, then album.
- The line also carries a live station's marker and what is on air, the sign-in, Widevine and error notices, and the "Preview" mark of a 30-second preview.
- Beside the title the bar shows the quiet "Spatial" tag (`kit.SpatialMark`: `Tag`, no colour of its own, the tooltip is its description) while `PlaybackState.Spatial` says the item plays as its Spatial Audio rendition. It sits in a clipped wrapper that shrinks a hundred times faster than the title, so it gives way first in a narrow bar and never moves the transport; Now Playing shows it with the other marks.
- The transport at the centre: the elapsed time, shuffle, previous, the 44 DIP play disc, next, repeat, the length. Then the volume.
- The seek bar is built after the transport, and the right group (`volumeControls`, out of the flow at the bar's right edge over a spacer of its width) after it. The Tab order follows the build order (out-of-flow elements come after the flow ones): Repeat, Seek, the right group.

**Behaviour**

- The position is `Position()` and ticks every 250 ms while playing. A drag shows where it will go and seeks once, when the pointer lets go.
- Every command is a goroutine over `services.PlaybackService` (through the small `transport` interface the tests fake). A failure comes back through `Env.Post` and shows as a toast titled by the command and worded by `i18n.ErrorText` of its code.
- Controls Go says do not apply (the snapshot's `Controls`) are disabled, with the reason (a live station, a station) in the tooltip.

**Shortcuts**

| ID | Keys |
| --- | --- |
| `player.toggle` | Space |
| `player.next`, `player.previous` | Ctrl+Right, Ctrl+Left |
| `player.seek-forward`, `player.seek-back` | Shift+Right, Shift+Left (10 s) |
| `player.volume-up`, `player.volume-down` | Ctrl+Up, Ctrl+Down |
| `player.mute` | M |
| `player.shuffle` | S |
| `player.repeat` | R |

#### Volume

`volumeControls`: the mute button and the slider sit in one row (`volume-control`) that carries `kit.VolumeWheel`. The wheel or a trackpad swipe over either, or the 4 DIP gap, changes the volume (`docs/design.md`, Shell). The mini player has no volume.

#### Queue panel

`queue_*.go`, `Slots.Aside`: 320 DIP beside the page while `view.queueOpen` and Now Playing is closed. Escape closes it and the focus returns to the bar's Queue button.

- A 48 DIP header: the heading and `queueTools` (Clear while the user has songs queued, and the AutoPlay toggle, dimmed with its reason where it does not apply).
- Under it the running order (`queue_list.go`, `queue_row.go`): lines of `queueLines` (`queue_logic.go`), each with its own height and offset (`line.y`, so the window to build is found by binary search, `queueGeom.lineAt`).
- The lines are: the "Now playing" heading and the playing song's 72 DIP row; the "Next" heading (its detail line is `queueSummary`, beside the page only) with the user's songs as 52 DIP numbered rows; then the station's or AutoPlay's heading and rows, which keep their place.
- The three groups are lists of their own (`queueGeom.groupOf`). A row that changes group is built again, so the focus is handed on.
- The list is one Tab stop. Arrows, Home and End move the focus, Enter plays, Delete removes, Alt+Up and Alt+Down move a user's song, and the Menu key and Shift+F10 open its menu.
- A row drags by its grip, and the whole panel is a drop target (`kit.DropOn`): a drop moves a row to the gap, or adds what came from anywhere (songs, videos, albums and playlists).
- Every change goes through `Env.Queue` (`kit.QueueCommands`), the one place that asks before replacing a long queue.

#### Playing item's menu

`playing_menu.go`, `kit.PlayingTarget`/`PlayingItem` (the one mapping a queue row's drag payload uses too).

- Right-click, Shift+F10 and the Menu key on the bar's cover, title and artist, Now Playing's cover, song copy and artist/album links, and the mini player's cover and title/artist open the action menu of the playing entry. It is the one a row of that song opens (`kit.ActionMenus` over `Env.Actions`), by kind (a song's, a music video's, a live station's; none while nothing plays).
- Now Playing also has a "More for <title>" menu button (`RoleMenuButton`, `Expanded` while open) in its actions row, the way in without a pointer. The bar has none (no room: Shift+F10 on the cover).
- The entry is not in a list, so the menu has no container actions. Each surface owns one `ActionMenus` drawn by a single `Popup` per frame.
- MyGo hands a right-click to the innermost interactive element, so a clickable child (the bar's cover button, the title and artist `ui.Link`s, Now Playing's `outLink`s) calls `Row` itself.
- `ActionMenus.Navigate` makes a page a chosen action opens close Now Playing first. `ActionMenus.Before` lets the mini player bring the main window back before any action but a queue one (its router, dialogs and toasts are the main window's).
- The mini player's window is the drag handle, so its cover and title take only the secondary press (`HandleInput`, never `RightClicked`, which would make them interactive and stop the drag). Its keyboard way in is Shift+F10 or the Menu key with the focus in the window, on the root.

#### Sleep timer

`sleep_timer.go`. The moon button and its radio menu stop playback in 15, 30, 45, 60 or 120 minutes, at the end of the song or of the queue (`PlaybackService.SetSleep`, fading out over the last 10 seconds). The timer is Go's (`PlaybackState.Sleep`): the button reads it, filled while one runs and showing the minutes left, and ticks on the minute boundary of the countdown.

#### Rating

`rating_buttons.go`. Favourite (and Suggest Less outside the compact bar) of the playing song, through `Env.ItemStates` (`kit.RatingButtons`: a change shows at once, Go's answer replaces it). Nothing is drawn for what has no song to rate: live radio, a music video, a song outside Apple Music. The favourite shortcut reads the state off the main thread first, then rates or undoes.

#### Announcer

`kit.TrackAnnouncer` tells screen readers, politely and once, that a new track plays (`Now playing: {name} by {artist}`, "a preview" for a 30-second clip) and, once per track and code, that it cannot play. Play and pause are never announced. The bar and the mini player each hold one, as only the window that is shown is read.

#### Slider commit and mute memory

- `kit.SliderCommitted(sl)` is true once the user has finished changing a slider: the frame after the pointer let go of it, wherever the pointer is (a drag that drifts off the thin slider is still held), or the frame of a key's change. The seek bars (the bar, the mini player, the video view) seek on it, so a drag seeks once.
- `store.Playback.Unmuted()` is the volume an unmute returns to (`SetUnmuted` remembers the last non-zero one; `DefaultUnmuted` 0.8 before any mute). The bar's mute button, the M key and the video view share it.

### Now Playing and lyrics (`features/player`, `lyricsync`)

Now Playing is the player's `Overlay` slot. It is open while `view.overlay` is `overlayNowPlaying`. The panels' shared state is `view.go`: the overlay, the pane, the queue beside the page, `fromRoom`.

Full-screen lyrics are its `Layer`, open (`LayerOpen`) as `overlayRoom` with a song that has lyrics. The window behind is inert and the layer's wrapper takes no input while closed.

Both are built only while open, so scroll and focus start again on each opening. The view closes itself when the queue is cleared.

| Key | Action |
| --- | --- |
| `L` | Lyrics (hidden while a music video plays) |
| `N` | Now Playing |
| `V` | The music video view (`Env.Video`) |
| Escape | Scope `now-playing`: room to Now Playing to the page |
| `Shift+L` | Full screen |
| `[`, `]`, `\` | Timing |

#### Layout

The layout follows the view's own box as last drawn (`measureLayout`, `nowPlayingLayoutFor`).

- Under 880 DIP it is stacked: the song as a header over the pane. Otherwise the sleeve column sits beside the pane, divided by a 1 DIP fold (`line`) the view's whole height.
- The sleeve column is the cover's width and `sleeveX` (48 DIP) at each side. The cover and its copy are one block held `paneTop` (24 DIP) down from the column's top, the same top inset as the pane's, so the cover and the tabs start on one line at any height. A block taller than the column is cut by the column's `Clip`.
- The pane has `paneTop` above, 8 DIP below and `paneEdge` (12) at its sides. Its header is `paneHeadH` (47 DIP) of tabs and Close over a rule (`paneRule`, from `paneInset` to `paneInset` so that it starts and ends where the words do). A pane's foot is `paneFootH` (52 DIP) under another.
- The cover is 168 stacked. Otherwise it is the smaller of a width class (320 from 880, 400 from 1000, 480 from 1500, 560 from 1800) and a height class (240, 320 from 640, 400 from 720, 480 from 900, 560 from 1050). This is a pure function with a table test (`TestNowPlayingLayoutFollowsItsBoxNotTheWindow`).
- The body is the whole column up to `maxBodyWidth` (1760 DIP), centred beyond it.

#### Cover and content

- The wash is `kit.Backdrop` (tone ground) as an absolute `PassThrough` sibling.
- The cover is `kit.MotionCover` over the one rendition asked for at `maxCover` (560 DIPs, the largest class; 1120 px at 2x, never narrower than the cover), with `Overlay` set so it plays while the view covers the page. It is used when `Env.Motion` allows motion and the album has some (`MotionService.Album`, cached an hour, never asked while motion is off).
- A music video's cover is a 16:9 `FluidArtwork`; a local broadcaster's is the art of the song on air.
- The Now Playing view (`nowplaying*.go`) owns: the song copy, the live station lines (the programme from the radio page's `["radio","station",id]` query), the broadcaster block, Favourite and Suggest Less (the `rating` hook), "View credits", and the artist and album links (only where `Env.HasRoute` knows the page).
- The tabs are `ui.TabsBase` (one Tab stop, arrows, Home and End). A station, a music video or a song outside the catalogue has only Up next (a heading), whose body is the queue panel's list, summary and tools (the `queueView` hook the queue code sets; the pane is empty until it does).

#### Lyrics data

- `store.UseLyrics` over a `store.LyricsSource` (`LyricsService.Lyrics`) reads the one cache `store.KeyLyrics` shared with the song page. It is always a `services.Lyrics` and never stale: a provider change invalidates the prefix. A song with none (`not_found`) is cached as empty lyrics, not an error asked again every five seconds.
- The translation and pronunciation choices are `config.Settings.LyricsView`, read from the settings cache (`store.UseSettings`) and saved through `SystemService.SetLyricsView` off the main thread (an export leaves them out).
- The per-song timing offset is in memory for the session and shared by every window (`lyricsync.Session`).

#### `lyricsync`

`internal/ui/lyricsync` is the pure timing logic with its tests: the sung line, rests and verses, the voices of a duet, word progress, `NextChange`, and `Shown` (a line under the translation and pronunciation choices). It has no UI, so the mini player's line can use it.

#### The sheet

`lyrics_sheet.go`, shared by Now Playing and the room, is a `ui.Scroll` of verses of lines.

- A synced line is a button that seeks to its begin plus the offset (one Tab stop; Up, Down, Home and End move between lines). A plain one is a paragraph.
- The view is built again only when the sung line changes (a `c.After` for the next line or rest).
- Word-timed texts are `kit.SweepText`s: one per part and line, shaped once per line and width (again when the width changes) and painted from the clock in a `Draw` callback.
- That callback asks for the next frame only while a word is filling. Between words, after the last and before the first, it sleeps with `Painter.After` until the next change (a word beginning or ending, the line ending, the voice resting).
- Under reduced motion the words are zero-length, so each lights whole as it begins, and the next change wakes the view instead of frames.

*Why the fill is rate-limited.* The Go side of a frame is 0.2 ms (no build, no layout, "painted again"). What a frame costs is the window's presentation, which on a CPU rasteriser (llvmpipe in a VM or a headless session, `MYGO_GPU=0`) is proportional to frames a second: 60 took 50% of a core, 30 took 29%. So the fill is painted at the display's rate (`Painter.AnimationFrame`) where `Painter.Accelerated()` says a GPU draws, and at 30 a second (`AnimationFrameRate(30)`) where the CPU does. The native sweep fills every word, not only held notes.

*The reading band.* The band (14%–72% of the sheet's height) holds the words still. When the anchor line leaves it, the sheet turns to put the anchor's verse (or the line, when the verse is too long) at the band's top. It jumps the first time and under reduced motion, else `ScrollTo(…, Smooth)`. A scroll the user made (the offset moved with no glide of ours) or an arrow key keeps it still for 4 s.

#### Full-screen lyrics

- The same sheet at room size (`roomLook`: `clamp(36, 4.4vw, 84)` bold in a 24 ch column, 7vw of side padding, 12% fades), lines as paragraphs.
- A footer strip sits under a 1 DIP rule with the cover (48 DIP), the song, the timing nudge and Exit full screen.
- It puts the window in full screen through `Env.Video.SetFullscreen` (the video view's own window binding, so no second hook) and leaves it only if it entered it.

#### Focus on close

- The bar's cover, Lyrics and Show video buttons are the openers of what they open (`view.opening`, `ui.Handle`s bound in `bar.go`). Closing Now Playing (Escape, Close, a toggle) puts the keyboard focus back on the opener when it was inside the view.
- `returnFocus`, called by the bar every frame, takes its request back a frame later, so a control that is gone never takes the focus when it returns.
- A view a key opened has no opener and leaves the focus with the page.
- Full-screen lyrics close into Now Playing, on its Full screen button. The queue beside the page returns to the bar's Queue button.
- The video view's opener is `store.VideoView`'s. A Play control that opens it is bound to a handle (`kit.CoverPlay.RunFrom` gets the tile's own, `kit.PlayButtonsOpts.Bind` takes the page's, `kit.ItemPlayers.PlayItemFrom`/`PlayVideoFrom` and the video player `RegisterVideo` pass it on). The view calls `OpenFrom` with it, so closing the view returns the focus to that Play button.

#### Tests

`lyricsync` has its own timing tests. The player's `nowplaying_test.go` runs the views over `uitest` with a fake lyrics source. It covers the layout and cover policy, panes, loading, not found and failure with retry, the sheet following the line, the user-scroll hold, click to seek under an offset, the timing nudge and reset, translation and pronunciation, keyboard between lines, the room, Escape levels and golden images (light and dark). `NP_EVIDENCE_DIR=<dir> go test ./internal/ui/features/player -run Evidence` writes the PR's pictures.

### The mini player (`features/miniplayer`)

`Slots.Mini` is the whole content of the shell's second window (about 420×132 DIP, frameless, always on top, created hidden on first use). The root is the window's handle (`DragWindow`) over the cover's wash (`kit.Backdrop`).

**Layout**

- The cover fills the window's full height (132 DIP) at the leading edge with no margin.
- Beside it a text column (12 DIP from the other edges) holds the title (16/22), the artist, the third line, and a foot row.
- The window's own buttons trail the title: Open Sona (`DesktopService.ShowMainWindow`) and Close mini player (`CloseMiniPlayer`). Close is outermost on the side the platform puts it: `WindowControls.Native == "inset"` or "close" in `Start` put it first, before the title.
- The foot row has the time left (or a Live marker for a live station) at the start and the transport (Previous, Play or Pause, Next, with the reasons of `kit.ControlUnavailable` as the bar says them) at the trailing side.
- Progress is one seek slider, a 2 DIP hairline along the whole foot of the window (over the cover too, amber on a 12% ink track). Its hit area is 12 DIP high and it grows to 4 with a thumb under the pointer and while keyboard-focused. A live station has none.
- It is a local `ui.SliderBase`, with the same Seek label, value text, arrow keys and commit-once-on-release as the kit slider.

**State.** The mini player mirrors `store.Playback`, sends the bar's commands through `PlaybackService` off the main thread, and holds its own `kit.TrackAnnouncer` (the main window's bar is not built while it is hidden).

**Third line and failures**

- The mini player has no room for toasts. A failed command replaces the third line for four seconds ("Couldn't skip: …", announced once; a sign-in failure says to sign in to Apple Music in Sona).
- The third line is the notice, else the line being sung (`lyricsync`: the line from `SungLine` on the lyrics clock, its words lit whole in amber as they begin, two lines at most, woken by a timer at the next line, rest or word; the per-song timing offset is shared with Now Playing), else what needs the user or what plays (the album, "Music video", "Live radio").

**The module** also adds the player bar's Mini player button (`Env.PlayerControls`, order 30; `DesktopService.ToggleMiniPlayer`) and the shortcut `miniplayer.toggle` (Ctrl+Shift+M, "Show the mini player", View). The shell's window calls have no error to return, so a call that panics is the failure.

### Account side: private listening, Widevine, updates, reminders and motion

Five modules whose state other features read. The shared parts sit where features may import them:

- `store`: `UsePrivate`, `UseCDM`, `UseUpdates`, `UseReminders` (queries under the keys `private`, `cdm`, `updates` and `reminders`, never stale, kept current by the owning module's events), plus `PrivateForced`, `CDMProgress`, `CDMCanRetry`, `ReminderFor`. `Peek` reads a query without fetching and `SetIfEmpty` writes a snapshot only while nothing newer is there.
- `i18n`: `CDMProblem` and `CDMFailedState` word the failure codes of `internal/playback/cdm`; `FormatPercent`.
- `kit`: `SetPrivate`, `RetryCDM`, `SetReminder`, `RemindButton`. The service call runs off the main thread, with a toast titled by what was attempted and worded by the error code.

The account menu, Settings, the setup, the player bar and an album page use those without importing the five packages.

**Private** (`features/private`)

- `private:changed` fills the mirror; a snapshot only fills it while it is empty (an event that lands first wins).
- The player bar's pill is added to `Env.PlayerControls`: a ghost 28 DIP toggle with the hat glasses. Off, it is a muted glyph alone. On, it is a chip with a 1 DIP dashed `control-line` outline ("off the record"; the element's own `BorderDashed`) holding the glyph and "Private" in ink, no fill (never amber). The word shows from 1100 DIP of window width, and "Private (development build)" from 1280.
- Forced by `SONA_PRIVATE=1` or not yet known, it stays focusable and does nothing (the tooltip says why; unknown is dimmed).
- The command `private.toggle` (playback group, no keys, bindable) is unavailable until Go reports the state and while forced.

**CDM** (`features/cdm`)

- `cdm:status` fills the mirror and refreshes the `diagnostics` queries only when the state, version or code changed, not on a progress step.
- The notice in `Env.PlayerNotice` is the line under the title while the song waits:
  - Downloading: "Waiting for Widevine to download", a 48×3 DIP meter (`kit.ProgressBar`, empty while the size is unknown) and the percent.
  - Failed: a 6 DIP `danger` dot and "Couldn't get Widevine · Retry in Settings" (the failure's words as its tooltip), or "Songs can't play on this computer".
  - Ready: a check and "Widevine is ready. Press play to start."
- A link to Settings → Playback appears only once that route exists (`Env.HasRoute`), plain text before.

**Updates** (`features/updates`)

- The `SidebarFooter` notice is a slip: a `raised` block (radius 6, padding 10, inset 10 from the sidebar's edges) with "New version" in meta, "Sona {latest}" at 14/500 (the status; assistive technology still reads "Sona {latest} is available") and a full-width ink button. It shows only while `UpdateStatus.Available`.
- The button is "Install and restart" where `UpdateStatus.SelfUpdate` (macOS on Apple silicon and Windows on x64 installs; `UpdatesService.Install`; a failure toasts and the notice says "Couldn't install Sona {latest}"). Elsewhere it is "View" (`UpdatesService.OpenRelease`; a failed open toasts).
- After a failed install (`UpdateStatus.InstallError`) the button reads "Try again" and an outline "View release" sits under it, so the notice is a way out and not a loop of retries. The cause is in the Diagnostics bundle's Updates section.
- `updates:available` and `updates:install` write the query. Download progress shows as a bar inside the slip in place of the button, and "Restarting into Sona {latest}…" is the slip's line once installed.
- Settings → General → Updates words why an install can't update itself (`UpdateStatus.Disabled`: no signing key, not writable) and has the Test versions switch (`updates.prereleases`). Linux packages and AppImages keep updating through their own tools (`docs/architecture.md`, "Updates").

**Reminders** (`features/reminders`)

- `reminders:changed` writes the list.
- The album action `reminders.toggle` (item group, order 25, bell icon) reads "Remind me on {date}" / "Turn off reminder for {date}", the day from `i18n.ShortReleaseDate` of `Album.ReleaseAt`; it is plain "Remind me" / "Turn off reminder" without a date.
- It is listed for upcoming albums with a full release date, a catalogue ID and notifications, and greyed out until the list has loaded with the menu.
- The album page's button is `kit.RemindButton`, which sets and removes through the same `kit.SetReminder` ("Out now · {artist}" is the notification's body).

**Add-ons** (`features/addons`)

- It shows what running add-ons add (docs/addons.md). Settings → Add-ons is the settings feature's; the two never import each other.
- It reads `AddonsService.Contributions` off the main thread at start, on `addons:changed` and when the language changes. One read runs at a time; a change meanwhile asks for one more.
- **Item actions.** `ActionRegistry.ReplacePrefix("addon.", …)` swaps every add-on action in one step. Each is in `kit.GroupAddon`, after Sona's own and before the developer's. The entry reads "{label} · {add-on name}". An action for two kinds is registered once per kind, as `<action id>/<kind>`.
- **Commands.** `Env.AddonCommands` feeds the palette's "Add-ons" group (`module.AddonsGroup`). They have no keys and are not in the shortcut help or Settings → Shortcuts.
- **Text.** Every label is cleaned again here (control and bidirectional characters out, at most 60 bytes), whatever the service did.
- **Invocation.** Never on the main thread. The same action chosen again while it runs does nothing. The answer is a toast titled with the add-on's name, at most one a second per add-on, with a fixed ID so it never stacks.
- **Links.** `cli.SafeLink` is checked when the answer arrives and again when the button is clicked. The button reads "Open {host}". Nothing opens before the click.
- **Notices.** `addons:notice` shows a warning toast when Sona switches an add-on off.
- **`sona addons dev`.** `addons:dev-ask` opens a `kit.Confirm` with the add-on's name, the folder and the permissions; `AnswerDev` carries the answer. A question of the person's own is never replaced: the answer is no. `addons:dev-ask-end` closes the dialog if it is still the one on screen.

**Settings → Add-ons** (`features/settings/addons.go`, section `addons`)

- A trust note, "Add-ons folder" (Open and Look again), then one group per add-on under an h3 of its name.
- Rows of a valid add-on: the switch, Permissions, its settings rows, Log. A refused add-on has the reason and nothing to turn on.
- **The switch.** Turning it on opens an approval panel under the row, announced as a group. "Turn on" enables with exactly the permissions listed. Cancel and Escape do nothing. Turning it off calls `Disable` at once.
- **Rows.** A toggle and a choice save at once, a text row with Save or Enter. A secret is never sent to the window: it shows "Saved" and a Remove button.
- **Names.** Every button carries the add-on's name, so two add-ons never share an accessible name.
- The list and the rows are two queries, `["addons","list"]` and `["addons","rows"]`, fresh for 2 seconds so that coming back to the page reads the folder again.
- Goldens: `settings-addons-light`, `settings-addons-dark`, `settings-addons-approval-light`.

**Motion** has no feature package. The policy, the power mirror and the provider are `kit.MotionPolicy` (`Env.Motion`: the setting, Reduce effects, reduced motion, `power:changed` and the decoder probe) and the app's motion code, whose `Report()` feeds the bug-report bundle (`MotionService.Report`).

- Settings → Diagnostics builds its motion group from `Report()` once `MotionPolicy.Probed()` is true. Before the decoder probe answers, an empty codec would read as "cannot decode", so the page shows no group.
- The Power row (followed, unreadable, source, ignored, UPower) and the download and cache rows come from `MotionService.Status()`.

### Music videos (`features/video`)

`/music-video/{id}` and the video view.

#### The page

`VideoPage` shows a skeleton, then:

- **The lead stage** across the whole content column: 16:9 at the column's width while that is no taller than 60% of the window's visible height, clamped to 360 to 640. Past that the stage stays full width at that height and the whole 16:9 picture is centred in it on the `surface` token like a cinema screen, never cropped. The picture is requested from `kit.Artwork` at exactly its whole-DIP box, with no wash behind it.
- **The caption row** under it at the column's width. The title block is at the left: title, artist links, "Music video · year · duration · genre" with the Explicit, 4K and HDR tags. The actions are at the right, stacked under it below 720 wide: Play where `Capabilities.Playback.Video`, Watch in Apple Music, Play the song, and the item menu where videos play.
- The note on standard definition (or that videos open in Apple Music) with the song line, Notes, "More by {artist}" (See all only with an artist ID and a registered `/artist/{id}/view/music-videos`) and "More like this" as rows of 16:9 tiles from 296 wide, and the release date.
- The skeleton is the picture, the caption bars and a row of tiles at those sizes.

Data is `store.Use` over `CatalogService.MusicVideo` (key `["catalog","music-video",id]`, which the view's "Watch in Apple Music" shares). Links go only to registered pages (`Env.HasRoute`).

#### The view

The view is a `Layer` slot, open while `Env.Video.IsOpen()` and a music video is the playing item.

- Pictures come from `video.Surface` in the engine, which the CDM host child feeds. `store.VideoView.Frames()` (a `media.FrameSource`, which `*video.Surface` is) hands the surface to a `kit.VideoFeed` that the view starts when it opens and stops (forgetting the picture) when it closes. A new queue entry restarts it, so an earlier video's last picture never shows.
- Pictures are held in memory, the latest only, and never written to disk.
- The stage letterboxes the picture on black at its own shape (16:9 before the first one, with the video's cover as a poster). A 48 DIP strip over it holds only Full screen and Close.
- The foot under it has the seek bar (time, slider, time left) across the full width, then one row. The caption is at the left on the ground: the title with the SD mark (a link that closes the view), the artist (also a link), and the status ("Loading the video…", then "Buffering…" when loading or when no picture came for a second). Previous, play or pause and next are in the exact middle (the caption and the volume cell share the leftover width), and the volume is at the right. Tab follows that reading order.
- The controls fade after 3 s without the pointer or a key while playing (not while paused, loading, buffering, on a problem or with a control focused from the keyboard).
- Where the platform cannot play videos, or playback failed, the picture's place says why with a Watch in Apple Music button, and Try again unless Apple's own refusal (HD only, licence) makes it pointless.

**Keys.** They belong to the view while the focus is in it (it takes the focus when it opens, and the window behind is inert).

| Key | Action |
| --- | --- |
| Space or K | Play and pause |
| Left, Right | Seek 5 s |
| F | Toggle full screen |
| M | Mute (only where Sona sets the volume) |
| Escape | Leave full screen first, then close |

They are element shortcuts of the view, which MyGo delivers before the window's, so the player's Space and M do not run too. A focused button, link or slider keeps Space, and Shift+arrows fall through to the player's 10 s seek.

**Full screen and focus**

- `Env.Video` owns the window's full screen. Closing the view leaves it only if the view entered it.
- However the view closes (Escape, Close video, the toggle key, the bar's "Show video" again), the focus goes back to the control that opened it. An opener binds a `ui.Handle` (`Element.Bind`) and opens through `Env.Video.OpenFrom(&h)` / `ToggleFrom(&h)`.
- The layer calls `Env.Video.ReturnFocus(c)` in each frame it builds closed. It asks for the focus once and takes the request back the next frame, so an opener that is no longer built never takes the focus later.
- With no opener (`Open`, `Toggle`, a play from a page) or none left, nothing is focused and Tab starts at the page.
- The view closes when the queue moves on from the video, and when the play that opened it never names a video in the mirror within 2 s.
- A play that opens the view goes through `Env.Queue.PlayVideos` with `Env.Video.Open` as its callback.

Tests are headless: fake pictures on a `video.Surface`, a fake `PlaybackService`.

### Library (`features/library`)

Eight pages under the sidebar's Library section, the sync mirror and its sidebar indicator, and the item actions every list uses over `Env.ItemStates`. `/library/made-for-you` has no page: its route replaces itself with `/`.

| Page | Order | Icon |
| --- | --- | --- |
| Recently Added | 5 | `clock-3` |
| Artists | 10 | `mic-vocal` |
| Albums | 20 | `disc-3` |
| Songs | 30 | `music` |
| Music Videos | 40 | `clapperboard` |
| Favourite Songs | 60 | `star` |
| History | 65 | `history` |
| Playlists | 70 | `list-music` |

#### Lists

- Albums (`?sort=dateAdded|title|artist|releaseDate&desc=&type=albums|singles`), Songs (`?sort=title|artist|album|dateAdded&desc=`) and Playlists (`?sort=dateAdded|lastModified|title&desc=`) take their order from the URL. `listSpec.parse` drops what it does not know and `Router.Replace` writes it. Recently added and Favourite songs have no order.
- Each reads pages of 200 through `usePaged` (`store.Use` per page, key `["library",kind,sort,desc,page,type?]`, never stale until invalidated). The first page is always read; the others only for the rows the grid or list reports built (`OnRange`) and their neighbours. So the list has its full length from the first page and the scroll bar is stable.
- Albums, Playlists and Recently added are a `kit.CoverGrid`; Songs and Favourite songs are a `kit.TrackList`. Both are built in the page's one scroll container (`scrollPage`, which hands the kit its `Pane`). Selection, keyboard navigation, menus and drag are the kit's.
- A tile or row links to a page only when `env.HasRoute` says it exists.
- Songs play through `PlaybackService.PlayLibrary(sort, desc, start)`, the others through the queue commands.

#### The gate

`gate.go` is what a page shows instead of its list:

- A read error (`kit.ErrorBlock`, a title per kind, Try again).
- "Sign in to see your …", with a Sign in action only when the shell offers an account dialog (`library.openAccount`).
- "Your library is syncing" while a sync runs or none ever finished.
- "No … in your library yet" with the page's hint.
- Nothing while the first page loads, the account is unknown or restoring, or there are rows.

Loading shows 16 skeleton rows or 12 skeleton tiles.

#### Artists

A master and detail page.

- A 320 DIP list of every library artist (`LibraryService.ArtistIndex` once, folded, sorted and filed under letters by `artists_index.go`) with a filter field. The field is a combobox: Up, Down, Page keys, and Enter or Right at the end of the text open the artist; Escape clears.
- The open artist (`?artist=`) sits beside it with Play and Shuffle, the Full artist page link, the item menu, and their library albums (`ArtistAlbums`, newest release first) under "Albums" and "Singles and EPs".
- Below 880 DIPs the list and the artist take turns, with a Back button.

#### Page header and rail

- Every page builds `pageHead` (`header.go`): title and count on one baseline, controls at the right, a 1px rule.
- Albums and Songs build their page with `scrollPageRail`, which puts the letter rail (`rail.go`) beside the scroll container and keeps its 32px slot even when there is no rail.
- The rail's letters come from the page held when the list fits in one page, else from `searchLetters` (bisection over `Albums`/`Songs` with limit 1, query key `["library","albums"|"songs","letters",sort,…,total]`, so a sync refreshes it).
- A jump sets the page's scroll offset from `pageView.gridTop` (a zero-height anchor box built before the list and read back after layout) and the grid's row height or `TrackRowHeight`. Artists jump to the letter heading's row offset in its own list.
- The sync footer (`sync.go`) is a one-line status with a green dot and a Sync now button when idle, and a `--hover` card with the domain, its count and a 3px bar while a sync runs or after it failed.

#### Recently added and History

- **Recently added** files the list under Today, Past week, Past month and Earlier by `DateAdded` (calendar days, as History counts them), one `kit.CoverGrid` and one selection per group. The groups come from the first page; what lies past it is older than all of it, so it joins Earlier. The pages fetched follow the union of the groups' built tiles.
- **History** groups `LibraryService.History(days)` by day (30 days; "Show earlier days" adds 30 while the earlier days keep showing) as a timeline. Each day is a row with the day's heading and counts in a 168 DIP rail, a 1px line down the rail's right edge with a dot at the heading, and one `kit.TrackList` to the right. The query sits under `["library","recent"]` so a new playing song refreshes it.
- **Music videos** page through `MusicVideos` in 100s (stale after 10 minutes) and play where the platform plays videos. The service reads the synced copy (the `music-videos` sync domain), so the page lists them with no connection and the app's `Offline` flag does not hide them. Playing still needs a connection.
  - **Reloads.** The page reloads when a sync finishes the domain, and when an item is removed or the flag clears.
  - **Offline state.** Before the first sync of them has completed the list comes from Apple, so when that request fails for lack of a connection (loaded videos or not), or the auth status says `Offline` and nothing has loaded, the page shows "Music videos need a connection" with Try again in place of the grid.
  - **Try again** while offline re-runs the sign-in check (`AuthService.Revalidate`, as Account's Check now does) and then reads the videos. The page leaves the state as soon as Apple answers, rather than when the check's own retry (2 s doubling to 5 minutes) clears the flag.
  - **Recovery.** A successful load shows the grid whatever the flag says.
  - Online it carries no note, like every other library page.

#### Sync

- `EventSyncProgress` and `EventSyncDone` feed `syncStore`. A `SyncStatus` answer older than an event is dropped.
- A finished domain refreshes its lists and the counts; `sync:done` refreshes everything under `["library"]`.
- `playlists:changed` with `Tree` (a playlist created or deleted here) refreshes Recently added (`keyRecentlyAdded`, `["library","recently-added"]`). The playlist feature refreshes its own lists, and features don't import each other.
- The sidebar footer (`Slots.SidebarFooter`) shows, with an account, "Syncing <domain>" with the count and a bar, or "Synced <when>" / "Library sync failed" / "Library not synced yet" with Sync now or Try again.
- `EventItemState` (a rating or library change made by `sona like`, `dislike` or `library`) is `ItemStates.Remember`: it writes the item's state and refreshes Favourite songs.

#### Item actions

`actions.go`, as `Module.Actions`:

- IDs: `library.favorite.*`, `library.suggest-less.*`, `library.copy-link.*`, `library.add.*` and `library.remove.*` for songs, albums, artists and playlists; `library.copy-link.station`; and the bulk `library.favorite.many`, `library.add.many`, `library.remove.many`.
- Labels and availability read the item's state from `Env.ItemStates`, which a menu loads once for every item in it as one `LibraryService.States` call.
- Delete from library is offered for an item in the library. An item that names its library copy (`Source` library, or a `LibraryID`) is in the library whatever Apple's catalogue `inLibrary` says (Apple reports a partly added or just added album as not in it, and a library answer can leave an ID out), until the user's own removal says otherwise: `LibraryService.States` answers so, and before a state loads the single and bulk actions read the item the same way, so every album under Library, the album page's header menu included, offers it.
- A change shows at once and is undone when Go refuses it. Delete asks first (`kit.Confirm`) and every outcome toasts.
- Delete from library takes effect before Apple answers. `LibraryService.Remove`:
  1. hides the item in the local library first (`Library.Hide`): its membership, ratings, search entries and playlist places; with the last song of an album the album; with the last album or song of an artist the artist. A pre-added album stays;
  2. emits `library:changed` so every list re-reads (`ItemStates.Follow` invalidates the membership lists);
  3. sends Apple's DELETE.
  - When Apple refuses, `Library.Restore` puts everything back, `library:changed` is emitted again and the failure toast shows.
  - The album page of an album that has left the library says "This album is no longer in your library" (with the album's Apple Music page when it has one), not an empty track list.
- Playing and queueing go through `Env.Queue` (`kit.QueueCommands`).

### Settings (`features/settings`)

`/settings?section=…` with the footer link `Settings` (gear, order 10, Ctrl+,). The page has an index column and the section in 800 DIP.

- The index column holds the title, Find a setting, and the section list with each section's state in one line (`summaries.go`).
- A bare `/settings` opens the section shown last in the run. Choosing one `Replace`s the location, so Back leaves Settings.
- The breakpoints read the page's own width (the window's less the sidebar, measured at the last layout; `settings.width`), never the window's.
  - Below 960 DIP the index wraps above the section and the summaries are for assistive technology only. The index is held in view (outside the section's scroll container) only from 700 DIP of height.
  - A group keeps its heading in a left margin only where the section's column beside the index is at least `marginRoom` (728 DIP: the margin, its gap and rows that fit a title beside two buttons); otherwise the heading sits above its rows.
- `?section=account` opens the account dialog (`Env.Surfaces.OpenAccount`) and replaces itself with `/settings`.

#### Rows

`rows.go`:

- `row` is `SettingRow`: the 1 DIP line, title, description, and the control at the right or stacked under the text. The control is named by the title and described by the description.
- `unavailableRow` is the row of a setting the platform can't back (`Not available on <OS> yet.` or a reason of its own). `group` is an h3 group; `loading` is the skeleton.
- Every row has an ID, the find entry's.
- Every control saves at once through `s.save` (`kit.UpdateSettings`: optimistic, rolled back with a toast "Couldn't save the setting").
- `settings` keeps the live queries the sections share (`{"settings"}`, `{"capabilities"}`, `{"scrobble","status"}`, `{"integrations","status"}`, `{"cdm"}`, `{"updates"}`), kept current by their events in `Init`. `Init` also refetches lyrics (`{"catalog","lyrics"}`) when the lyrics settings change.
- The services behind them are interface fields tests replace (the real scrobbler reads the machine's keyring).

#### Find a setting

`find.go`, `findui.go`.

- `settingIndex` lists every row in page order, then every registered shortcut (`shortcut-<id>`).
- `findSettings` folds case and accents, then requires every word of the query in the title, group, section or keywords. It ranks: title starts with, a title word starts with, title contains, the rest; then section, then page order. Entries whose section doesn't show them (`When`) are left out.
- The field, Down into the results, Up/Down/Home/End among them, Enter taking the first, Escape clearing and the announced count all work as for a combobox.
- A pick `Replace`s the section. `focusSetting` waits up to 3 s for the row to be built, scrolls it into view, focuses its control (or the row) and washes it for 2 s.
- A test checks that every entry has its row on its section.

#### Sections

| Section | Contents |
| --- | --- |
| General | Language (offered when a second language is selectable), Starting Sona, Window, While music plays, Updates |
| Playback | Widevine; Sound (quality, output, Sound Check, Spatial Audio, Equaliser, Crossfade); Listening (private listening, replacing the queue); Lyrics (sources with order, word timing, translation); Advanced |
| Appearance | Theme, Reduce effects, Motion artwork |
| Connections | Scrobbling (Last.fm authorisation in the browser, ListenBrainz token form, Disconnect) and Discord |
| Shortcuts | The filter, Reset all and the key recorder |
| Storage | Usage, the artwork cache limit, Clear caches, Compact now, the settings file (export, import) |
| Diagnostics | The checks (`SystemService.Diagnostics`, on every open), Motion artwork diagnostics, Copy diagnostics (the redacted bundle on the clipboard), Open log folder, Developer tools |

**Developer tools** is MyGo's inspector. `Open the inspector` calls `DesktopService.OpenDeveloperTools` when `Capabilities.DeveloperTools` says the run has it (a development build started with `SONA_INSPECTOR=1`), and is an unavailable row otherwise.

#### Previous version

`settings/previous.go`; Settings → Diagnostics → Updates, between the checks' groups and Developer tools.

- One row, built only while `UpdatesService.Previous` (a query under `diagnostics/previous`, asked on every open) says a previous version is kept and restorable: "{version} is kept from your last update." (or "The version from your last update is kept." when its number can't be read) and "Restore…".
- The button asks first (`kit.Confirm`, held busy): "Go back to {version}?", "Sona restarts on {version}. Your settings, sign-in and library are kept.", "Restore and restart".
- `UpdatesService.RestorePrevious` runs off the main thread. On success Sona quits and restarts, so the question stays up as "Restoring…".
- A failure closes the question, and the row says why in a `RoleAlert` ("Couldn't restore the previous version. {reason}", worded from `restore_failed`). The next press tries again.
- Linux, development builds and installs Sona can't write to have no row.
- The sidebar notice (`features/updates`) shows only while `UpdateStatus.Notify()`: a newer release that is not `UpdateStatus.Declined`, the version the user just went back from.

#### The key recorder

`shortcuts.go`, `shortcuts_logic.go`.

- A window shortcut claims a key before MyGo's `HandleInput` sees it. So a recording row asks for every key with every modifier set as an element-level `Shortcut` on its own button (an element in the focus chain gets a key before the window's).
- A `?` with no `Key` (GTK, AppKit) no `Shortcut` can ask for. The recording button takes it in its `HandleInput` (`kit.IsShiftSlash`, `kit.TakeShiftSlash`, so the dispatcher lets it go) and records `?`.
- The chords it writes are the dispatcher's spellings (`Ctrl+K`, `Shift+ArrowRight`, `Space`, `?`).
- `shortcuts_logic.go` holds the registry rules: resolve, conflicts, rebind with steal.
- A change is saved as `config.Settings.Shortcuts` by ID and applies at once (`dispatcher.rebind`).

#### Tests

- The rig (`settings_test.go`) is the page over `uitest`'s full Env at a router. Section tests drive clicks and keys and read the saved settings or the fake service behind them.
- A behaviour test covers every setting that changes what the app does (theme, reduce effects, language, private listening, start page, close action, quality, crossfade, rebinding…).
- Also: the find index, the summaries and the logic tests (settings-file, motion diagnostics, shortcut chords and rules), and two golden images (light, dark).

### Account and sign-in (`internal/ui/account`, `features/auth`)

#### `internal/ui/account`

One `Section` per mount (the dialog's body, the setup's Apple Music step) over a small `API` interface that `*services.AuthService` satisfies (tests fake it).

`Section.Build(c, hosted)` is `AccountSection`. It shows one of:

- the error block or placeholder while the status query (`store.KeyAuth`, `["auth"]`, which `features/auth` keeps current and every feature reads with `store.UseAuth`) has nothing;
- the signed-in view: storefront, language, where the token is kept, the unencrypted and offline notices, Sign out;
- the keyring notice, the state notices and the sign-in flow.

`hosted` leaves the heading and the Sign in button to the host: the setup builds the button in its foot with `FootAction`.

**The sign-in flow** is a pure reducer (`signin.go`): credentials, the code step for a trusted device or a text or call, the phone choice, done.

- Errors are classified by Go's stable error code (`classifySignInError`), never by message text. Each has its own title, detail and next step.
- The password is cleared the moment Sign in is pressed and lives only in the goroutine making the call. The code and the pasted token are the same: none is stored, logged, put in a query key or an error.
- A password exchange reports `signing_in` too, so the browser-waiting view shows only when no native attempt is in flight.

#### `features/auth`

The module around it:

- The `auth.account` command ("Account settings", no default keys).
- The `auth:changed` handler, the one writer of `store.KeyAuth`: the status goes into the query. `authInvalidations` decides what refreshes: signing in or out invalidates `library`, `catalog` and `diagnostics`; a new storefront `catalog`; a keyring backend change `diagnostics`; intermediate states nothing. The previous status is the module's own.
- The account menu in `Slots.TopBarAccount`: the avatar, its tooltip and state line, the attention dot, and the menu on `kit.Menu` with each fact in its header led by a glyph. The Private listening checkbox uses `MenuItem.KeepOpen`, so its tick is the confirmation.
- The account dialog (`kit.Dialog` over `Surfaces.AccountOpen`, bare, 680 DIP): two panels in one scroll. The lead has the Sona mark, one heading and three lines on what signing in adds; then the account's panel with `account.Section`. The panels stack in a window too narrow for both. The Apple Account field is focused on open.

The menu and the dialog share one `Section`, so only one sign-out confirm can exist.

#### Tests

- The reducer, error copy and status helpers have unit tests. `account`'s behaviour tests drive the forms over a fake `API`.
- Its end-to-end tests run the real `services.AuthService` and `auth.Manager` over fake Apple endpoints (`uitest.AuthConfig` points the harness's manager at `appleidtest` for idmsa and the activation endpoints and an `httptest` server for the Music API).
- They cover: password; password with a wrong then the right code; an account without devices (a call to a phone); the loopback browser fallback (driven from `Harness.Opened()`: the page opened, Open again, Copy link, Cancel); a rejected then a valid pasted token; and a keyring that was unavailable. Each checks the account view afterwards and that no password, code or token reached the logs, `Opened()` or `Copied()`.
- The e2e smoke (`e2e/account_test.go`) opens the dialog from the avatar on the built app. Signing in there needs Apple's endpoints, and the probe build has no seam for fake ones: it needs `SONA_E2E_APPLE` to set `auth.Config.AppleID`, `APIBase` and a static developer token from the smoke's local server.

### First-run setup (`features/onboarding`)

The setup is `Slots.Layer` (open while `Surfaces.SetupOpen`): a scrim over the whole window, which is inert behind it, and a centred card.

**The card**

- The head: "Set up Sona", "n of 7", Skip setup.
- The step rail (`stepRail`, 192px at the card's leading side, one Tab stop; arrows, Home and End move, Enter goes there). A card narrower than 720px or too short for it takes `stepStrip` under its head instead, the same list and keys; `cardWidth` decides.
- The step beside it.
- The foot: Back, "Get started", "Continue", "Not now", "Done". The Apple Music step's Sign in sits in it.

**Steps**, in order:

| Step | Contents |
| --- | --- |
| Welcome | |
| Apple Music | `account.Section`, hosted |
| Playback | The Widevine block with its download progress and Try again (asks through the shared `kit.RetryCDMWith` and reads "Retrying…" until it returns), then audio quality, Sound Check, Spatial Audio (where `Capabilities.Playback.Spatial`), crossfade, private listening |
| Connections | Last.fm and ListenBrainz, Discord |
| Appearance | The theme cards drawn from `theme.TokensFor`, reduce effects, motion artwork |
| Desktop | Close action, autostart, notifications, keep awake: each only where the platform's capabilities say so, the rest named |
| Done | |

**Behaviour**

- The step is `Surfaces.SetupStep`, so a language change that rebuilds the view keeps it.
- Every control saves at once through `kit.UpdateSettings` (the cache and the theme follow; a failed save restores and toasts). Reaching Done marks `onboarding.completed` once.
- Escape on a step asks "Skip the rest of the setup?" (`kit.Confirm`); on Done it leaves.
- The route `/welcome?step=` opens the setup at that step and replaces itself with the start page.
- On a first start (`services.StartRoute`: no account, setup neither finished nor skipped) `cmd/sona` passes `Options.FirstRun` and the window opens on it.

**Tests.** The steps and Widevine view logic have unit tests. The controls save through the real `SystemService` on the uitest Env, with fakes for the scrobbler, integrations, CDM and translations (the tests never touch the keyring, the network or a file chooser). Goldens cover the welcome and Appearance steps, light and dark, at the wide card (the narrow strip is pinned by a behaviour test). `e2e/account_test.go`'s `TestFirstRun` starts the built app on a profile whose setup was never finished (`e2e/profile -first-run`), walks to the Apple Music step and skips.

### Equaliser (`features/equalizer`)

`/equalizer` has no sidebar link. The player bar's button, Settings → Playback and `equalizer.open` (Ctrl+Shift+E) reach it. The shortcut exists only where `PlatformService.Capabilities().Playback.Equalizer` holds.

#### The page

A graphic equaliser:

- **The header**: title, the chosen curve as its catalogue line, the On/Off switch (`audio.equalizer.enabled`).
- **The presets**: a wrapping row of 32px chips, one radio group. "Your curve" is set a little apart, then `AudioService.Equalizer().Presets`. Choosing one saves it and turns the equaliser on. The selected one is the only Tab stop; the arrows move and select in reading order, wrapping. The chosen chip is ink-filled while the equaliser is on and keeps the selected tone with an ink outline while it is off.
- **The bands panel** below, at the page column's width (1120 at most): the preamp fader, a rule, then the ten band faders. Each fader is a `kit.OriginSlider` (a `Slider` whose fill runs from the 0 dB line, or from the top for the cut-only preamp) at 224 DIPs, with its value above and its frequency below. The dB scale (+12, 0 dB, −12, and the unit of the names, Hz) sits to their right.

**The response curve**

- It is drawn in a box behind the ten faders, as tall as one fader, so that its three rules are the faders' ends and 0 dB line and its curve passes through the thumbs.
- The box is in the flow and the faders lie over it (an absolutely placed row), so a drag is never taken by the drawing.
- The curve is `response.go`'s model of GStreamer's `equalizer-10bands` (biquads at 48 kHz: low shelf, peaking, high shelf). So the drawing is what the host applies. It is computed again only when the gains change and drawn into paths reused between frames.
- The headroom note says how far the level drops ahead of the bands.

#### Data flow

- Dragging a fader previews. The draft curve is sent through `AudioService.PreviewEqualizer`, one call in flight and the latest curve last (`store.Latest[T]`, errors ignored), and held as the draft until the pointer lets go, when `SystemService.UpdateSettings` saves it.
- Moving a band switches the preset to "custom" and starts the user's curve from the preset's gains. A preset is never changed.
- The equaliser info is fetched once (`{"audio","equalizer"}`, kept for the app's lifetime); settings use `{"settings"}`.

**States.** Where the platform has no equaliser the header says "Not available on this platform yet" and a panel of the bands panel's size takes its place and says so. A load failure shows the error block with a retry under the same header. While it loads, the chips and the panel are bars of their final sizes with the three rules, so nothing moves when the data arrives.

### Commands: the palette and the shortcut help (`features/commands`)

Two surfaces over one list. The module's `Layer` slot builds `kit.Palette` (Ctrl+K, `commands.palette`, kept out of the palette itself) and a `kit.Dialog` (`?`, `commands.help`, keywords "help keys bindings"). Opening one closes the other, so only one command surface shows.

Both read `Env.Shortcuts()`: every module's shortcuts, the shell's and each sidebar link's go-to, with the keys they answer to under the user's bindings now (`module.BoundShortcut`). So a rebinding or a new module's shortcut shows at once.

#### The `?` chord

- `?` is the chord Shift+/ (`kit.ParseChord`, which the dispatcher's `parseChord` wraps).
- On GTK and AppKit, MyGo (v0.9.0) gives Shift with punctuation no `Key`, only its position (`InputEvent.Code`), and matches shortcuts by `Key` alone. So Sona reads `?` there by position.
- `kit.ShiftSlash` notes Shift and the slash key with no `Key` (`kit.IsShiftSlash`). It is observed on the shell's root and an open layer (`ObserveInput`). The dispatcher reads it as the chord Shift+/ in the next frame, unless the focus takes text or the focused element took it itself (`kit.TakeShiftSlash`; the key recorder's `HandleInput`, which records `?`).
- Windows' press has its `Key`, which the shortcut takes, so nothing runs twice.
- The help's dialog observes its own, to close on it (`ShiftSlash.Pressed`).
- The position is where US and UK layouts have `?`, so on GTK and AppKit (`kit.SlashByPosition`) the command's label says so while it answers to `?` (`site/src/content/docs/contributing/accessibility.md`, "Focus").

#### Palette

The frame is `kit.Palette` (field, grouped list, hint foot of Move, Run, Close). The feature supplies what is in it.

- **An entry** is a launcher row: its icon in a quiet 32 DIP tile (`surface` tone, radius 6; the `command` glyph when a shortcut names none), the label, and up to two key chords at the end. The tile is the width of a library item's cover, so a list that mixes both keeps one leading column (`iconTile`, passed as the entry's `Leading`).
- **Nothing typed**: every shortcut that is not `NoPalette`, whose `When` holds and that is not `Unavailable`. Groups: Pages, Playback, View, Navigation, General (empty groups left out). The places to go come first, so Enter on opening goes somewhere rather than pausing the music.
- **Typed**:
  - "Commands", ranked by `kit.RankCommands` (label, then keywords), each with its group as a second line since no heading carries it.
  - "In your library", after a 150 ms pause and from two characters: `SearchService.Library`, 4 per kind, the first 8 of artists, albums, playlists and songs, with a 32 DIP cover and "Kind · subtitle". The last answer stays until the next arrives.
  - "Apple Music" with "Search Apple Music for “term”".
- A library item opens its page; a song plays alone (`PlaybackService.PlayItems`; a failure is a toast). The Apple Music entry goes to `/search?q=`.
- An entry whose page no module registered (`Env.HasRoute`) is left out rather than pretending.
- Picking runs after the palette has closed and the focus is back where it was. Escape closes and returns it.
- The window's shortcut handler does not see keys while a dialog is open (MyGo), so the palette's and the sheet's own chord close them from inside the dialog (`kit.ChordPressed`).

#### Help

- An 820 DIP sheet, 10 % from the top so it does not move as the list shortens.
- The head is the "Keyboard shortcuts" title with a `kit.SearchField` ("Filter shortcuts") on its right. Under a hairline the groups run in two columns (one under 720 DIP), each a title over a rule and its rows.
- A row is the label and its keycaps or a quiet "Not set", in a key column of one width per group (`keycapsWidth` estimates it from the chords, without measuring text), so the caps of a group start on one edge.
- The foot holds "Change any of them in Settings." (the word links to `/settings?section=shortcuts` when that page exists) and the keys that move: ↓ Browse, Esc Close.
- The filter takes the focus after the dialog has noted where it was. A shortcut stays when every word typed is in its label, keywords, group title or chords (as the caps spell them and as they are spoken), ignoring case and accents.
- The count is announced (or that none match), the list returns to its top, and a filter that leaves nothing says so.
- Down in the filter moves the focus into the "Shortcuts" group, which scrolls with the arrows and Page Down; Up at its top goes back.
- Escape clears a filled filter, then closes. Close and Escape return the focus.
- "?" in a focused field is text. So the dialog's `ObserveInput` (`helpSheet.observe`) notes the press of Shift and the slash key itself, by position or with its `Key`, and the sheet closes on it while the filter holds nothing (or only the typed "?"). After other text a "?" stays text. From the list or the link `ShiftSlash.Pressed` closes it.

#### Chords and macOS

A text field keeps macOS's Control letters (Control+K kills the line), so the palette's own chord never reaches the window's handler from its search field there. `kit.Palette.Chords` (set by the module each frame to the keys it answers to) hands them to the field's `OnTextKey`, which closes the palette.

#### Not here

The mini player's shortcut is `miniplayer.toggle` (`features/miniplayer`). Item actions (`Env.Actions`) are not commands: they act on an item.

### Playlists (`features/playlist`)

Registered after video and before radio and the player. It owns:

- the `/playlist/{id}` page;
- the sidebar's playlist sections (`Slots.SidebarSection`);
- the dialogs (`Slots.Aside`: `dialogHost` builds the open `kit.Dialog` and nothing else, so it takes no room);
- the playlist item actions (`Module.Actions`): add to playlist for songs, albums, playlists and selections, remove from playlist inside an editable container, pin and unpin per kind, edit details, move to folder, delete from library.

#### State and data

- `playlists` holds `Env`, the `playlistsAPI` and `playbackAPI` seams (tests fake them), `prefs`, `edits`, and the `dialogs`, `sidebar` and `page` states.
- Query keys sit under `library` so a sync invalidates them: `["library","playlist-tree"]`, `["library","pins"]`, `["library","playlist-recent-targets"]`, `["library","playlist-suggestions",id]`. A playlist's detail is `["library","playlist",id]` for `p.` IDs, else `["catalog","playlist",id]`; collaboration sits under it.
- `playlists:changed` invalidates the changed playlists except one being edited here, the tree, counts and library playlists when the tree changed, and the pins.

**Tree logic** (`tree.go`, tested against `tree.test.ts`)

- Folders come before playlists at every level, by Apple's position then name. Unknown parents go at the top.
- Pinned and non-editable playlists are left out of the folder tree (their folders count them), except collaborative ones (`TreePlaylist.Collaborative`, stored by the sync), which are the user's own whether hosted or joined and stay in their folder.
- Sections: Apple Music (Apple's: the personal mixes, then the playlists the user cannot edit) and own (editable, or collaborative). The mixes come first in Apple's position order, the others by name. `isPersonalMix` (a `pl.pm-` catalogue ID) is the one test for a personal mix and does not depend on the section: Delete is not offered for a mix, and any other place that must tell a mix reuses it. A joined collaborative playlist is not editable (`CanEdit` false), so the edit, move and drop-in actions stay off it; Delete and Change cover stay.

#### Sidebar

- Sections: Pinned, Apple Music, Playlists. They wait for pins and tree so a pinned playlist never flashes in its section.
- Open folders and folded sections persist in `config.Settings.Sidebar` (`OpenFolders`, `FoldedSections`). They are read at start and saved through `SystemService.SetSidebar` (one save at a time, newest state last; it changes nothing else in the settings). Settings exports leave the key out: it describes this computer's library view. A section folded under the old Made for you ID (`made-for-you`) is read as the merged Apple Music section folded: `config.Sidebar` normalises it at load and the next save writes only `apple-music`.
- Playlist rows and pinned editable playlists are `kit.DropOn` targets that accept any payload whose items hold songs, albums or playlists (a queue row's payload carries its song and an `Origin`, which is ignored here). They describe the drop ("Add to …") and add through `PlaylistsService.Add`.
- The Pinned section and the "New playlist" row are targets that exist while a matching drag is on. A drag resting on a closed folder opens it after 600 ms.

**Shuffle from the artwork**

- A library playlist row, and a pinned playlist, has a 20 DIP button over its cover ("Shuffle {playlist}", `shuffleButton`). It is a sibling of the row's link (never inside it, so it is on the accessibility tree), shown with `kit.Fade` while the row is hovered or has the focus within, and ignoring input while hidden, with a tooltip.
- It is no Tab stop. The keyboard shuffles with Shift+Enter on the focused row or the row's menu item `playlists.shuffle` (listed only for sidebar rows, whose `ActionContainer` kind is `sidebar`).
- All three go through `playPin` (the songs from `PinTracks`, then `Queue.ShuffleSongs`, so replacing a long queue asks or offers Undo as the page's Shuffle does).
- `shuffleReason` disables it with its reason: no songs ("No songs to play"), or `PlaybackState.AuthRequired` ("Sign in again to play"). A CDM still downloading is not one, as playing is what retries it.

#### Dialogs

- One at a time (`dialogs.show` replaces): the add picker (search, recents, groups by folder path, arrow keys), new playlist and edit details (one form), folder new and rename, move to folder.
- A dialog takes the focus on its second built frame, after MyGo noted what to return it to (an `AutoFocus` in the first frame is remembered as the opener).
- Deleting a playlist or folder asks through `kit.Confirm`.

**The cover picker** (`covers.go`)

- *Change cover…* in the item menu of any library playlist (`playlists.cover`), and *Cover* beside *Name and description* in edit mode.
- A radio group of option tiles from `PlaylistsService.CoverOptions`: Automatic, None, Your image, and Apple Music's nine designs of the playlist's generated cover when Apple generates one (else a line saying why there are none). Four columns, `GridNav` with arrow keys, scrolling in a short window. Applied at once through `SetCover`.
- *Choose an image…* runs `UploadCover` (the file chooser lives in the service, `Platform.Files`). A failure stays beside the button as a status.
- The covers are Sona's own (`docs/apple-music-reference.md` §5.8, "Playlist artwork") and the dialog says so.

#### Page

- Loading skeleton; error with Retry and a link back to Playlists.
- **Header** (`header.go`; `kit.CollectionHeader` is not used): the cover beside the title, byline and description with the actions on its bottom edge, then the ledger strip. The strip has kind, songs, running time, a dated cell and "Updated …". The dated cell is the library's `dateAdded` (omitted for catalogue playlists and for personal mixes, `pl.pm-…`): "Created …" for a playlist the user made (editable, or a collaborative one they host; `stampKindOf`), "Added …" for one they added or joined; "today", "yesterday", else a long date. "Updated …" is left out when it is on the dated cell's day or before it. Each is a tooltip tab stop with the exact date and time. "Find in playlist" sits at its right end.
- The track list with an editable container, the empty states, the suggested-songs rows (the table's own columns), featured artists (small round tiles) and more-from-curator.
- Links to other pages render only when `Env.HasRoute` says they exist.

**Edit mode**

- It swaps Play and Shuffle for Done and "Name and description" and shows the editor: a header row and a grid of rows with drag handle, a "Starts" column, Alt+Up/Down, Delete, a 2 DIP drop line and polite announcements.
- Moves and removals are optimistic (`trackedits.go`): shown at once, and sent in order per playlist. `edits.serial` also hands each answer to the main thread in that order: the next edit starts only once the previous answer was posted.
- Go's answer replaces the optimistic one unless later edits are queued. A failure rolls back by refetching (a conflict says the playlist changed on another device).

#### Sort and find

`view.go`, for catalogue and library playlists alike. The page holds every track, so it sorts and filters in the UI with `viewOrder(tracks, key, desc, query) []int`, a pure function that answers original indices.

- It returns the songs matching every word of the query (folded, in Name, Artist or Album) in the order of the key (Title, Artist, Album, Duration, or none: the playlist's order). The sort is stable, so songs that compare equal keep their playlist order whichever way it runs, and digits read as numbers.
- A header click cycles ascending, descending, playlist order (`TrackSortState.Toggle`) and is announced.
- The `kit.SearchField` ("Find in playlist", in a search landmark) shows "N of M songs" while it holds a query, announces its count, replaces the table with a message when nothing matches, and Escape clears it.
- `page.view` memoises the answer per tracks, key and query.
- Everything the page does with a song goes through the listed songs or maps back. Play, Shuffle and row activation queue the songs as listed (a query narrows the queue to the songs found). While a query is active `headerActions` names Play and Shuffle for them (`kit.PlayButtonsOpts.PlayLabel` and `ShuffleLabel`: "Play the N songs found", "Shuffle the N songs found"). Remove from playlist (`ActionContainer.Order`) and "Added by" use the songs' positions in the playlist, because `RemoveTracks` and `MoveTrack` are positional.
- Edit mode shows the playlist whole and in its own order: entering or leaving it clears sort and query and hides the controls. The state resets in `page.enter` with the rest of the page's.

**Decisions**

- **No "Date added" sort.** Neither `Song` nor `playlist_tracks` records when a track joined the playlist (`library_items.date_added` is when it joined the library, another thing).
- **No Ctrl+F on the page.** The global `search.focus` (`/`, `Ctrl+F`) keeps it. The dispatcher runs the first shortcut bound to a chord whose `When` allows it (search registers before playlist). A page-scoped Ctrl+F would need a dispatcher rule that page shortcuts win, or a `When` on `search.focus` that knows the playlist page (features do not import each other), and the settings page would list the pair as a conflict. The field is a click or Tab away.

#### Collaboration

Read only, for `pl.u-` playlists (network, cached five minutes). The byline becomes the collaborators stack, the track list shows who added each song (`addedByRows`: the next unclaimed attribution of the song's catalogue ID), and the Collaborators dialog lists current and former collaborators.

### Search (`features/search`)

The `Search` nav item (icon `search`, primary, order 20), the top bar's search field (the `TopBarSearch` slot, `field.go`) and the routes `/search?q&scope`, `/search/{group}?q` and `/profile/{id}`.

#### The field

A combobox input over `kit.Input` named "Search" (max 520 DIP).

- Suggestions open on focus, click and typing. Hints follow typing after 150 ms (`SearchService.Hints` for Apple Music, `Library(term, 3)` for the library scope; five-minute cache, previous data kept while typing).
- The list's runs (completions, top results, recent searches) have headings with an icon. An item row ends in its kind's icon and the active row in a chevron (a play triangle on a song that has no page here, which Enter plays). An empty field lists up to six recent searches.
- Up/Down/Enter/Escape/Tab and hover work as in a combobox (`combobox.go`: `splitMatch`, `moveActive`).
- `search.focus` (`/`, `Ctrl+F`) focuses it and selects its text.
- The field's text follows the route's `q`; the scope (catalogue or library) is the URL's.

**Choosing a result opens its page, a song's included** (`itemPath`: `/song/{id}`). Picking navigates only to pages `Env.HasRoute` knows.

- A library song opens the page of its catalogue copy (`Song.CatalogID`). An upload has no page, so it plays in place.
- Enter or a click on a song suggestion opens `/song/{id}`, so the chevron tells the truth.
- **Shift+Enter** plays the active song as a queue of one (`Env.Queue.PlaySongs`, as the sidebar's Shift+Enter shuffles a playlist). `OnTextKey` notes the modifier on the Enter that `Submitted` reports. On any other kind it opens as Enter does.
- The suggestion row is an option, so it holds no button of its own: the modifier is the keyboard's way to play, and a pointer user plays from the results page's Play controls or the row's menu.

#### The page

`page.go`, `results.go`. The term is its title (32/36) with the scope switch on its baseline.

- With no query it shows Browse categories and recent searches.
- With one, the results sit in Apple's order (or the library's four groups) beside a rail (`railList`, 200 DIPs, outside the scroll container so it stays put). The rail lists the groups found with their counts as jump buttons: a jump scrolls the group to the top and takes the focus to it, and the group in view is marked. Under 960 DIPs the rail is a wrapping row of the same buttons above the results.
- **Top results** are a best-match panel (the first result: a 120 DIP cover, kind, name at 24/650, catalogue line) with up to four compact rows beside it (below it under 640 DIPs).
- The panel and the rows open the result's page (a `RoleLink` named by name and catalogue line). Playing is separate: the best match's amber `kit.PlayButton` disc ("Play {name}") beside the panel, a song row's ghost Play button (`itemRow`, shown on hover and focus, after the link in the Tab order), and the action menu.
- Only a result with no page here plays as a whole (`RoleButton` "{name}, Play": a library upload, a station that plays when opened), with the disc drawn into the panel.
- Artists are one row of round portraits (104 DIPs at least). The other groups are denser cover shelves (136 DIPs at least), each under a hairline. Songs are a `TrackList` (the Album column from 720 DIPs; as on the album and playlist pages, activating a row plays and the artist and album cells link). "See all" appears when a group has more.
- Loading is the same layout in skeleton. Nothing found offers the other scope as its one action.
- The group page (`group.go`) loads pages as its end comes within a screen. The profile page (`profile.go`) shows a listener and the playlists they share.

#### Recent searches

They live in Go (`SearchService.Recent`, `RememberTerm`, `RememberItem`, `ForgetRecent`, `ClearRecent`). A remembered song opens its song page again (a library song is never remembered). Remembering is best-effort and never delays opening. `Recent` resolves a remembered library playlist's cover when the list is read (`Env.playlistCover`: the user's choice, else Apple's artwork or the mosaic), not from the artwork stored when it was remembered, so a mosaic playlist shows its mosaic and a changed cover shows at once.

### Concerts (`features/concerts`)

`/concerts?when&from&to&genre&yours` with the `Concerts` nav item (icon `ticket`, primary, order 25).

- **The header**: the title with the location button beside it, the city and "Updated" line.
- **The filters**: Dates select with typed From/To `YYYY-MM-DD` fields, Genres, "Your artists".
- **The coming-days strip** (`agenda.go`): the next 14 days, a button for each day with shows. A button scrolls the agenda to its day with `ScrollIntoView` and focuses its first control through the roving state. The strip is one Tab stop of its own.
- **The agenda** is grouped by the venue's month and day as ticket stubs. Days, weekdays and times are read in the venue's IANA zone (`dates.go`). The groups, strip and each row's words are built once per listing and kept in `viewOf` until the listing, filters, day or language change.
- "Show more concerts" in steps of 100; skeleton, error and empty states.

The agenda is one Tab stop with roving arrows (`roving.go`). A row has the start, the headliner's portrait, the line-up link (when `/artist/{id}` is registered), venue and distance, a "Your artist" mark, and a ticket button or menu that opens the seller in the system browser (`Env.OpenURL`).

The location dialog (`location.go`) searches Sona's own city list (`ConcertsService.Places`), saves with `SetLocation` and can return to the storefront's main city.

`auth:changed` invalidates the listings only when the signed-in state changes. `ArtistConcerts` and `ConcertsShelf` (`upcoming.go`) are the artist page's "Upcoming concerts" and Home's shelf (compact rows with a date column and rule), set on `Env` by this module.

### New releases (`features/newreleases`)

`/new-releases` with the `New releases` nav item (icon `calendar`, primary, order 27: after Concerts, so the positional Ctrl+1…9 keys before it stay), the page of the artist tracker (`ArtistTrackerService`; `docs/architecture.md`, "Artist tracker"; its look is `docs/design.md`, "New releases"). The sidebar link is always there: the page says plainly when the tracker is off and links to Settings.

- **State.** `artisttracker:changed` writes `store.KeyNewReleases` (`store.UseNewReleases`: items, unread count, when it was last checked); `Init` seeds it from `ArtistTrackerService.Releases`, which reads SQLite only. The nav item's `Count` peeks at it, so the sidebar shows the unread count without a fetch. Reads of the settings (`store.UseSettings`) tell the page whether the tracker is on.
- **The page** (`page.go`) groups the list (`groupsOf`: Coming soon, soonest first; This week; Earlier) and builds each group as a table of rows (`row.go`), all in one `kit.Selection`, `kit.GridNav` and `kit.ActionMenus`, as the artist page's release list is: rows select, drag, open their item menu on right-click, Shift+F10 or the Menu key, and move with the arrows. A row is one Tab stop; the Delete key removes it.
- **Marking read** is by opening a release (`MarkRead`, then the album page), the header's *Mark all as read*, or the menu; *Clear list* and *Remove* dismiss (`Dismiss`, `Clear`). Every call runs off the main thread (`newReleases.do`) and its outcome comes back as `artisttracker:changed`.
- **Actions.** `newreleases.mark-read` and `newreleases.dismiss` (album target, item group, orders 80 and 81) are listed only for a row of this list: the rows pass `kit.ActionContainer{Kind: "new-releases"}`. An upcoming release also carries `reminders.toggle` (Remind me).
- **Opening from a notification.** `app:open` with kind `new-releases` and no ID (the summary notification) opens the page; `routeForOpen` answers it before it looks for an ID.
- **Settings.** The two switches are rows of Settings → General (`releases.go`), ids `artist-tracker` and `artist-tracker-notify`; the second is an unavailable row where `Capabilities.Notifications` is false.

### Browse: Home, Discover, Radio (`features/home`, `features/discover`, `features/radio`)

Each is a package with one `modules.go` line (home, discover, …, radio). All three build on the kit's item components (`ItemShelf`, `ItemGrid`, `EditorialSections`, `WideRow`, `StationList`, `TrackShelf`, `TrackList`, `CollectionHeader`) and take their `kit.ItemsCtx` from `Env.ItemsCtx` each frame.

Data is `store.Use` with keys such as `["discover","home"]`, `["discover","page","grouping",…]`, `["radio","live"]`. The service calls sit behind small interfaces in each package so a test can fake them. Queue-replacing plays go through `Env.Queue`.

#### Home

`/` ("For You", Sparkles, order 10, exact); `/for-you/{id}` shows one shelf in full.

- **The plan** (`plan.go`): the lead is the first `feature` shelf with items; three plain shelves stay open; the rest go to the "More for you" index. Recently Played, station-only shelves, features and heroes never do.
- **The lead band** is a `kit.Backdrop` over the lead cover's pre-blurred backdrop: the page header as one line, a ruled head, the lead pick with its cover large, and Apple's other picks as one unnumbered running order of eight rows beside it (a row of cover and name over the order below 900 DIPs).
- **Shelf heads** are the local ruled head (`rule.go`: heading, hairline, "See all" or "Show all"), which replaces `kit.Shelf` on Home.
- **Shelf kinds**:
  - `HeroCard` shows a hero shelf of one item.
  - `itemRows` (roving focus, Space plays, Enter opens, selection, menus) draws the running order (`Inline`: name and reason side by side, one column, hairlines, no rank), the station shelves (rows, two deep) and Recently Played (`Tile`: a strip of covers in one row). A library playlist in that strip arrives with the cover it has in Library → Playlists (mosaic, Apple's or the user's choice), resolved by the service (`DiscoverService.libraryCover`); the UI draws `Item.Art` as for any card.
  - Cover shelves are `kit.CoverRow` under the ruled head.
  - `ShelfIndex` is the ledger of the rest with a strip of thumbnails.
- **The header** says when the page was fetched ("Picked for you…", "Couldn't refresh · from…") and has the Refresh button (`RefreshHome`, spinner unless motion is reduced).
- Signed out, offline, restoring, error and empty each have their state. The local shelves (Recently played from Apple or this computer's history, Recently added, Your playlists) follow.
- `discover:home` writes the page into the cache; `auth:changed` drops the discover queries.

#### Discover

Routes: `/new` (Compass, order 15), `/charts` (order 17), `/room/{id}?sort`, `/curator/{id}`, `/multiroom/{id}`, `/grouping/{id}`, `/charts/{chart}?genre&chartId`.

- `bands.go` sorts a page's sections into Songs and albums, Playlists, Radio, Videos and Browse. Every page opens with `masthead` (title at the display size, the issue line on its baseline, the page's controls at the end).
- **The front page** (`front.go`): the lead card at the column's whole width, in the hero shape up to 540 DIP tall and wider from there (to the banner's 4:1; `layoutFor`, `artHeight`, `artAspect`). Its caption has the pager at its end. Under it is a strip of four featured cards a page (`layout.go`).
- **The rotation** moves the lead every 7 s on the clock the view reads (`c.Now()`: tests advance it). The lead cross-fades over 400 ms. The rotation holds while:
  - the pointer is in the hero or the keyboard has focus in it (a control a click focused does not hold: `front.keyboard`, gathered from the hero's controls' `FocusVisible`);
  - a menu or a modal is open;
  - the window is hidden or unfocused (`Env.Window.Focused()`: the shell redraws on each change; it sets off again from a whole interval);
  - the hero is off screen.

  It stops under reduced motion.
- **The contents** are a wrapping row of columns, one per band: its jump button with its shelf count over a jump button for each titled shelf. From a 1100 DIP page a band's heading stands in a 200 DIP margin column beside its shelves.
- Rooms page their items by 100, charts by 50, as the list scrolls.
- The module registers the players of items (`Env.Players`): a song alone, an album or playlist from its first track, a music video alone, a track shelf as the queue.

#### Radio

`/radio` (order 18) and `/station/{id}`. The Radio page is Live now (the programme guide), then the bands Stations (with "Your stations" for a signed-in account), Shows and episodes and Browse.

**The guide** (`onair.go` over the arithmetic in `timeline.go` and `guide.go`), re-read every 30 s by `refresh`, never per frame:

- `programmes` joins back-to-back slots of one title.
- `windowAt` takes the stretch of time the track's width allows.
- `window.blocks` places and clips programmes, and `block.labelOf` keeps their words off the now line.
- `cellsFor` turns a station's schedule into the cells a lane draws.

**Layout.** `rail.go` lays a block out as a ruled head: the heading in a rail as wide as the guide's station column (`railW`; sticky, and over the content below `railMin`), the content beside it. So the guide's track, the local broadcasters (the shared `kit.StationList`, which `liveNow` wraps itself so the head can be the rail's) and every band's shelves start on one edge.

**The station page** (`station.go`) is the identity column (sticky; stacked under `stationSplit`) beside the lists:

- the note of a station Sona can't play;
- "On air" for a broadcaster;
- the live schedule as a vertical timeline (`slot` and `rail`, drawn in one `Draw` per row);
- "Plays first" for a track station.

The module registers the station player and the actions `radio.play-station` and "Create station" (songs, albums and artists with a catalogue ID), and invalidates the recent stations when a new station starts.

### Catalogue pages (`features/album`, `features/artist`, `features/song`)

`/album/{id}` (`?song=<id>` marks and scrolls to a song), `/artist/{id}`, `/artist/{id}/view/{view}` and `/song/{id}`, over the library or catalogue services. A library id reads `LibraryService`, a catalogue id `CatalogService`; a library album or artist with a catalogue id also loads the catalogue page for Apple's sections.

- Every page query is `store.Forever`. Each page has its skeleton and `DetailError` with Retry.
- The album's layout follows the content width (384, 320, 256 or 168 DIP cover) and its countdown re-reads every minute.
- `album.go-to`, `artist.go-to` and `song.view-credits` are the song rows' actions.
- Cross-page links render only where `Env.HasRoute` says the page exists (`kit.ArtistLinks` takes `ArtistLinksOpts{HasRoute}`).

#### Item state

Rating, add and remove with toasts, and one batched `States` call a frame, are `kit.ItemStates` (`kit/song_itemstate.go`): the one layer for every page, list and the player's rating buttons.

- `app.New` and `uitest.New` build one `Env.ItemStates`, which follows `library:item-state` (`Remember`) and refreshes the library lists when an item joins or leaves the library.
- It offers `Load`, `Change` (optimistic `store.Set`, which cancels a fetch in flight), `Rate`, `Add`, `Remove` and the bulk `RateMany`, `AddMany`, `RemoveMany`, with the header's `LibraryButton` and `RatingButtons`.

#### The song page

Built by `frame`, which loaded and loading share.

- From 860 DIPs of content, the cover, title, artists, album and actions sit in a `kit.Backdrop` spine (cover 240, 280 from 1300, 320 from 1600: `coverSize`, so the picture is asked for at three exact boxes) beside the body (details ledger, credits grid, lyrics). Narrower, they are a band (cover 136) over it.
- The width is the scroll container's as last laid out (`kit.PageFrame`, the window less the sidebar before the first layout).
- The people to a row of a credit group (1 to 4) come from the room the column has beside the cover (`shapeOf`, `bodyFor`): the body is 760 wide up to three to a row, 960 with four.

**Lyrics** are `store.UseLyrics` under `store.KeyLyrics(id)` (`["catalog","lyrics",id]`, always a `services.Lyrics`), the one cache the song page and Now Playing read.

#### Tracklist and layout details

- `TrackListState.BlockTop` and `ScrollToBlock` give the album's disc index its jump (work headings are measured from the last frame, so wrapped ones count).
- The album's margin (disc index and fine print) uses `Element.Sticky` (top 16) inside the listing grid, when the previous frame's height fits the pane. It is first in the tree so Tab meets the disc index before the songs.
- The band's dissolve and the far end's deepening are `LinearGradient` stops built once per theme and per cover.
- The page scroll has no padding of its own, so the band is flush to the pane's edges and each part sets its gutters.

#### Formatting, playing and tests

- Running times, the countdown and release dates are `i18n.FormatRunningTime`, `FormatTimeLeft` and `ShortReleaseDate`, on the regional format locale (`i18n.Locales()[0]`). A relative time is `i18n.RelativeTime` (freshness) or `i18n.RelativeDay` (calendar words), a playback time `i18n.FormatDuration` (m:ss) and a size `i18n.FormatBytes` ("12 kB"). No feature keeps its own copy of these.
- Collection play goes through `Env.Queue` (`kit.QueueCommands`): `PlaySongs`, `ShuffleSongs`, `PlayVideos`, `QueueItems` and `Replace`, which ask first or offer Undo before replacing a long queue. Features call it on the main thread.
- Pure logic (compilation rule, writers, classical grouping, release counts, credit titles) has Go unit tests, and so has the cover-colour palette (`theme/palette`).

## Tests

**Shell and feature tests**

- The shell's tests build fake modules and drive the view over `uitest`'s tester: nav entries, clicks, Ctrl and Alt chords, first paint once, the settings' start page and bindings, slots, text fields keeping their keys.
- A test pins the real module list in `modules.go`'s order and that a duplicate module ID panics. Its golden test runs the real modules over the fixture library.
- The player's tests run the bar over a fake `transport` on a bare `uitest` Env.
- The library's run over the full Env (`newPageRig`: the real module's routes, actions and subscriptions over a real `services.Core` on the seeded SQLite library, the fake engine, the covers' fake CDN). They cover every page's data, sort and type in the URL, paging, selection and keyboard navigation, the actions and their toasts, sync events and the sidebar indicator, the gate's states, the play button and empty states. The library's golden images are Albums, Songs and Artists, light and dark.
- `uitest.Apple(handler)` signs the Core in to a fake Apple Music account and serves the handler on the harness network at `uitest.AppleHost`. Search and concerts tests answer with JSON shaped like Apple's. `Harness.Opened()` lists the URLs sent to `Env.OpenURL`.
- Fetch goroutines are waited for with `Session.Until` (a deadline); everything else advances the tester's clock.

### Hermetic tests

A `uitest` Core is hermetic: whatever the machine running the tests holds (a signed-in keyring, a network, a session bus, Sona's own settings), the services see none of it.

- **Keyring.** `Harness.Keyring` is an empty in-memory secret store handed over `Deps.Keyring`, so the Secret Service is never asked (a maintainer's real Last.fm session is not found).
- **Network.** `Harness.Network` is the one `http.RoundTripper` behind every HTTP client the Core is built with: `Deps.Transport` (lyrics and scrobbling under the first-paint gate, the update check without it: it runs only from `OnNetwork` or on request), the artwork cache's client and the auth manager's.
  - It serves the fixture CDN's host and what a test adds with `Network.Serve` or `ServeHandler` (an in-process handler: a GitHub release, say).
  - It fails any other request at once with an `ErrOffline` that names the method, host and path and is a `net.Error` (the services class it as `network`). `Network.Blocked` lists what it refused.
- **Environment.** `uitest.New` points the process environment at the test's temporary directory for the length of the test:
  - `HOME` and the XDG directories (what `os.UserConfigDir`, the autostart file and Discord's socket search read).
  - The cache directory is one per user, `<user cache>/sona-tests/uitest`, beside Sona's own and kept between runs: MyGo registers the interface font once per process from a file it writes there, and fontconfig keeps its cache there. On macOS `$HOME/Library/Caches` of the test's home is a link to it, since `os.UserCacheDir` ignores `XDG_CACHE_HOME` there; on Windows `%LocalAppData%` is it.
  - `DBUS_SESSION_BUS_ADDRESS` and `DBUS_SYSTEM_BUS_ADDRESS` at a socket nobody listens on: a D-Bus call fails at once instead of reaching the real bus. The `Platform` the Core gets is empty, so no MPRIS, notification, power or autostart code runs at all.
  - No Discord application and a made-up Last.fm key, so what the Core offers does not depend on the machine.
- **`http.DefaultTransport`** is the same `Network` for the length of the test (loopback excepted, for `httptest` servers; restored afterwards), so a client built on `http.DefaultClient` fails fast too.
- **New services.** A new service that reaches outside the process takes the seam from `Deps` (`Env.gated`/`Env.transport` for HTTP, `Env.openSecret` for a keyring secret), never `http.DefaultClient`, `store.OpenSecret` or a platform package directly. It adds its case to `uitest/hermetic_test.go`, which covers the scrobblers, the ListenBrainz check, the update check and `http.DefaultClient`.

### End-to-end smoke

`e2e/` (build tag `e2e`) is the built app seen from outside.

```bash
headless -- go test -tags e2e ./e2e/...   # the devenv `e2e` script; CI job "E2E (headless sway)"
```

- It starts a headless sway with a private session bus and AT-SPI bus (`e2e/headless.sh`).
- It builds `bin/sona-e2e` with `-tags probe,sqlite_fts5`: MyGo's automation plugin, a fake playback engine and an HTTP seam that sends all the app's traffic to a local server of the fixture's covers (`cmd/sona/e2e_probe.go`, `SONA_E2E=1`, `SONA_E2E_CDN`).
- It starts the app on the fixture profile with no network, and drives it through the automation socket (`e2e/automation`): first paint, Albums listing the fixture albums, a click on an album's play button, the player bar playing, Space pausing and resuming, a captured PNG, a clean quit on SIGTERM.
- A second test reads the shell's landmarks (application and window, sidebar link, history buttons, page region, album play buttons, the player's transport and sliders) from the AT-SPI bus by role and name (`e2e/atspi`, D-Bus only).
- `E2E_OUT=<dir>` keeps the capture and the app's stderr; `E2E_BIN` skips the build.
- The probe build never ships (`probe` and `mygo_noinspector` exclude each other).
