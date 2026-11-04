# UI architecture

Sona's interface is Go all the way down: [MyGo](https://github.com/egoist/mygo)'s native UI (`github.com/egoist/mygo/ui`; GTK3 on Linux, AppKit on macOS, Win32 on Windows) drawing on the GPU from the same process that holds the services, the library and the playback coordinator. There is no web view, no IPC and no generated bindings: a view calls a typed service and reads the DTO it returns. This page is the map of `internal/ui`; `docs/architecture.md` has the process model and the platform seams, `docs/design.md` the visual direction and `internal/services/doc.go` the service contracts.

## Layout

```
cmd/sona ─► internal/ui/app ─► internal/ui/features/<name> ─► internal/ui/{module,kit,store,theme,i18n}
                                                   │                └─► internal/services (typed services, DTOs)
internal/ui/app ─► internal/ui/module (types only)
```

| Package | Holds |
|---|---|
| `internal/ui/app` | The windows, the sidebar and top bar, the router, the shortcut dispatcher (the tray is phase 3); builds the Env and the module list (`modules.go`: one line per feature, in the order the sidebar and the help list them) |
| `internal/ui/module` | The contract between the shell and a feature: `Env`, `Module`, `Route`, `NavItem`, `Shortcut`, `Slots`. Types only: no windows, no state |
| `internal/ui/features/<name>` | One package per feature (album, library, player…), the same names the React app's modules had. Features never import each other |
| `internal/ui/kit` | Components shared by features (Button, Artwork…), built on MyGo widgets and the theme |
| `internal/ui/store` | The event bus, cached queries and the playback mirror: everything that comes from Go and changes |
| `internal/ui/theme` | Design tokens (colours light and dark, type, radii, space, motion), their application to MyGo's theme, Lucide icons, the interface font |
| `internal/ui/i18n` | `T`, `TC`, `ErrorText`, the locale and its formatters, over `locales/en.po` |

A feature reaches another feature's state only through `store` (the playback mirror, the queries) or a hook in `module.Env`. Nothing under `internal/ui` imports Wails or `internal/shell`, which no longer exist. A new feature is a Go service (`internal/services/<feature>.go`, `<feature>_dto.go` and one `registry.go` line) and a package under `internal/ui/features` with one `modules.go` line.

## The module contract

A feature is a `module.Factory`, `func(env *module.Env) module.Module`, and keeps its state in its own struct; its views are methods on it.

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

`Env` is built once by `app.New` and never changes: `Core` (typed service lookup, `services.Service[services.LibraryService](env.Core)` returns the `*LibraryService`), `Bus`, `Queries`, `Play`, `Art`, `Nav` (the main window's router), `Log`, `Version`, `Dev`.

- **Routes** use MyGo's router patterns (`/album/{id}`, `/settings/{section...}`). The route memory, titles and the back and forward buttons are the shell's.
- **Nav** entries sit in the primary, library or footer section and sort by `Order`; `To` is one of the module's own patterns. Each gets a go-to shortcut (`Keys`, else Ctrl+1…9 or Alt+1…9 by position).
- **Shortcuts** have a stable `ID` (`<module>.<action>`): the settings store a user's bindings by it, so an ID is never renamed. `Keys` use v0.2.0's spelling (`Space`, `Ctrl+K`, `Shift+ArrowRight`). One key handler in the shell dispatches them: page-scoped ones skip text fields, the first match whose scope and `When` allow runs. `Unavailable` says why a key does nothing (the key is taken); `NoPalette` keeps a shortcut out of the command palette.
- **Slots** are places in the shell a module builds into: `Aside` (right of the page), `Overlay` (covers the page column), `Bottom` (the player bar), `Layer` (over the whole window; the window behind is inert while `LayerOpen`), `SidebarFooter`, `SidebarSection`, `TopBarSearch`. A func builds nothing when it has nothing to show. A route or nav item exists only when its module is registered: there are no placeholders.
- **Init** runs once after the shell connects, on the main thread: subscriptions (`store.On`) and first snapshots.

Phase 1's kit overlays add `Module.Actions` (`[]kit.ActionDef`, the item action registry); the field does not exist before `kit.ActionDef` does.

## Store, queries and events

All callbacks a UI author passes run on the main thread; everything in `store` is safe from any goroutine unless noted. Results and events come back through a `store.Poster`, which `app` supplies (`Window.Update` on every window: it runs the func on the main thread and redraws) and tests supply synchronously.

**Events.** `store.Bus` implements `services.Emitter`: the services emit into it from any goroutine. Events emitted before `SetPoster` are queued and delivered in order; afterwards each is posted in emission order. Subscribe with a typed handler, `store.On(env.Bus, services.EventPlaybackState, func(s services.PlaybackState) {…})`; a payload of another type is logged once and dropped.

**Queries.** `store.Use(env.Queries, key, fetch, opts…)` returns the cached `Result[T]` (`Data`, `Err`, `HasData`, `Loading`, `Fetching`) and starts a fetch on a goroutine when there is no data or the data is stale as a view starts using it. Call it from the view every frame: a view that goes unbuilt for a second and is built again is a new mount (TanStack's observer mounting), and only then is stale data refetched, with the old data showing meanwhile.

```go
lib := services.Service[services.LibraryService](l.env.Core)
r := store.Use(l.env.Queries, store.Key{"library", "albums", l.sort},
    func(ctx context.Context) (services.AlbumPage, error) { return lib.Albums(ctx, l.sort, false, "", 0, 1000) },
    store.Options{Stale: store.Forever})
```

- **Keys** mirror v0.2.0's TanStack keys (`["library","albums",…]`) and hold every input of the fetch. `Invalidate(prefixes…)` marks the queries below each prefix stale (all, with none), refetches the ones used in the last minute at once and cancels a fetch in flight; the rest refetch when next used. A module's `Init` ports v0.2.0's `eventHandlers` line for line: `store.On(…)` then `env.Queries.Invalidate(store.Key{"library"})`.
- **Options.** `Stale` (0: stale at once; `store.Forever` is v0.2.0's `staleTime: Infinity`) and `Keep` (an unused query is dropped after this, 0 is five minutes; dropping cancels the fetch's context). Fetches of one key never run twice at once; a failed query with no data is asked again only after 5 s, so a broken service is not asked every frame; a panicking fetch becomes the query's error.
- `Set` writes data (optimistic updates, event payloads); `Reset` drops everything (sign-out).

**Playback mirror.** `store.Playback` follows `EventPlaybackState` and `EventPlaybackQueue`: `State()`, `Queue()`, and `Position()`, extrapolated from the last snapshot while playing (`PositionMs + now − At`, never before the snapshot, clamped to the duration when known; v0.2.0's `livePosition`). `Seed` sets it from the services' snapshots before the first event; `Patch` changes it locally while a command is in flight, until Go's next snapshot.

## Strings and locales

Every string the user reads (text, labels, tooltips, toasts, menu and command labels) goes through `internal/ui/i18n` and lands in the English catalogue `internal/ui/i18n/locales/en.po`.

```go
i18n.T("Add music by {name} to collect it here.", "name", artist)
i18n.TC("verb", "Play")                                        // with a msgctxt
i18n.T("{0, plural, one {# album} other {# albums}}", 0, n)    // Lingui's positional names: "0" or 0
```

- **The msgid rule.** The message and the context MUST be string literals at the call site: the English text is the msgid, the catalogue's identity of the message and the source of Apple's mapping. `go run ./cmd/i18ntool extract` reads `internal/ui` with `go/ast` and collects every `i18n.T("…")` and `i18n.TC("…", "…")`; a non-literal argument is an error. Until the catalogue is made exact (phase 3) `extract` only adds msgids it finds to `en.po` and `en-XA.po`, never removes one, and leaves every existing byte alone; `extract -check` (part of `check`) fails when a literal is missing. The msgids and contexts of v0.2.0 carry over byte for byte, positional placeholders included (`{0}`, `{1}`: the Go call names them `"0"`, `"1"`).
- **Messages** are ICU MessageFormat, parsed and formatted by MyGo (`i18n.ParseMessage`): named and positional arguments, `plural` and `select` with CLDR's rules, `#`, number and date formats. A message that does not parse panics under test and in dev builds, and an Apple or community translation that does not parse is left out (logged once).
- **Lookup order** for the locale shown: Apple's translation (`services.AppleTranslations`, set with `i18n.SetApple`, keyed by `applestrings.MessageID(msg, ctx)`), then the community catalogue (`i18n.LoadCommunity`, a `.po` keyed by the English msgids), then English, the call site's own text. `en-XA` is MyGo's pseudo-locale (accented, stretched).
- **Choosing the locale** is `ChooseTranslation` over `TranslationCandidates` (the account's storefront language, then the system's, up to the first English one) and `SelectableShare` (0.9), ported from v0.2.0's `locale.ts` with its tests; the language the user picked in Settings shows whatever its share. `LanguageOptions` builds the Language setting's list. `SetLocale`, `SetApple`, `SetRegional` (the system's and the account's tags, so an English UI on an en-GB system keeps 24-hour clocks) redraw through `i18n.SetPoster`.
- **Formatting:** `FormatNumber`, `FormatDate(t, DateMedium)`, `FormatClock`, `FormatDuration` (`m:ss`), `List`/`ListOr` (a hand-keyed table of CLDR's list words), `Collator()` (`golang.org/x/text/collate`), all in the shown locale.
- **Errors.** Go sends codes, never text: `i18n.ErrorText(code)` words a `services.ErrorCode` with v0.2.0's msgids (an unknown or empty code gets "Something went wrong."), `i18n.ErrorMessage(err)` an error, preferring Apple's own wording of a sign-in failure Sona has no code for. A test checks that every `Code*` constant of `internal/services/errors.go` has its own text in `en.po`.
- **Apple's translations.** `apple-keys.json` maps Sona's msgids to Apple Music's string keys; `apple-keys-rejected.json` records the rejected pairs. Both sit in `internal/ui/i18n/locales/` with `en.po`, which Go embeds (package `locales`: `internal/i18n/applestrings` imports it). `i18ntool match` proposes mappings, `match -apply` moves reviewed ones in, `import` converts Apple's bundles for review (gitignored). Sona fetches Apple's text at run time; nothing of it is committed. CONTRIBUTING.md, "Strings and translations", has the workflow.
- Go's own catalogue (`internal/i18n/l10n`, notifications and file dialogs) is separate and unchanged: `extract` keeps both.

## Theme tokens

`internal/ui/theme/tokens.json` (embedded; dev builds read the file, which `theme.Reload` applies on the next frame) is the one source of colours, type, radii, space, metrics and motion. Colour names are v0.2.0's custom properties of `shared/styles/index.css` without `--`, light and dark (`ground`, `surface`, `raised`, `hover`, `active`, `line`, `control-line`, `ink`, `ink-muted`, `ink-faint`, `amber`, `amber-ink`, `on-amber`, `danger`, `positive`, `avatar-N-bg/fg`), the shadcn roles (`primary`, `ring`…), the role names (`play`, `play-ink`, `on-play`) and derived ones (`edge`, `selection`, `scrollbar`). A value is `#rrggbb[aa]`, another token's name, or `name/N` (that token at N% opacity).

- `theme.C("ink")`, `theme.Current().Type["title"]`, `.Radius["md"]`, `.Metric["sidebar"]`, `.Motion["feedback"]`: pages never hard-code a colour or a size. An unknown colour panics in tests and dev builds and is magenta in production.
- `theme.Apply(c)` at the top of every view sets the `ui.Theme` MyGo's widgets draw from, in Sona's colours and font, following the mode (`SetMode`: `ModeSystem`, `ModeLight`, `ModeDark`; `Dark()` reports what frames draw) and the desktop's text scale; it builds a new theme only when something changed.
- `theme.ReduceEffects()` is the setting (no backdrops, solid surfaces), `theme.ReducedMotion()` the settings or the desktop's preference; the kit skips its transitions under the latter.
- `theme.Icon("play")` returns a Lucide icon (ISC, `theme/icons`, the version in its README) by its kebab-case name; the names v0.2.0's code used (`ChevronRight`, `Loader2Icon`) resolve through `icons/ALIASES.txt`. `theme.RegisterFonts()` registers Schibsted Grotesk (variable TTF, SIL OFL 1.1, `theme/fonts`) before the first window opens.

## The kit

Components are functions taking the `*ui.Context`: `kit.Button(c, label, kit.ButtonOpts{…})` returns the element, so `.Clicked()` reads as MyGo's. Give repeated elements a stable key, `c.Key(id)`. PR 0.2 ships the two the foundation is checked with; phase 1 adds the rest (docs: the plan's §3.3).

- **Button**: `Default` (ink-filled), `Ghost`, `Outline`, `Play` (round amber) and `Destructive`; `SizeMD` (36), `SizeSM` (32), `SizeXS` (24); icon-only when the label is empty (a square, named by `A11yLabel`, which is also its tooltip); `Pressed` makes it a toggle. The face eases over 120 ms (the `feedback` motion) unless motion is reduced; the focus ring is MyGo's, in the ring token.
- **Artwork** shows a DTO's art URL (`/art/<key>/…`) or an Apple template at an exact size. `ArtLoader` (built by the app over `artwork.Cache`) resolves the URL to its template with `artwork.Cache.Template`, asks for the cover at `size × scale` device pixels, and decodes it with `ui.DecodeBitmap` on worker goroutines (newest request first); the result reaches the main thread through the Poster, and `Bitmap.ReleasePixels` drops the CPU pixels once the cover has been drawn twice. Until then, and when it fails, a `--hover` square with a 40%-ink glyph shows; a failed cover is asked for again after 5 s and 30 s. The 1px line at 6% ink (`ArtOpts.Edge`), round (`Round`) and non-square (`Aspect`) boxes follow `docs/design.md`, "Artwork". Covers are kept up to a cap and the least recently asked for are dropped.

## Testing with `ui.Tester`

`ui.NewTester(view, width, height)` runs a view without a window or a display, in `go test`: `tt.Click("Save")`, `tt.Key(0, ui.KeyTab)`, `tt.Focused(…)`, `tt.Find(text-or-label)` for a box, `tt.Advance(d)` to move its clock (animations, `c.After`), `tt.SetScale`, `tt.SetDark`, `tt.SetPreferences`, and `tt.Image()` for the last frame. Elements are found by their text or `Label`, so every interactive element has a translated label. The patterns the foundation's tests use:

- Views take what they need from a struct (the feature's state), so a test builds the struct and drives `tt` against its view; `theme.Apply(c)` first, `theme.SetMode` for light or dark.
- Off the main thread, tests give `store.Client`, `store.Bus` and `kit.ArtLoader` a Poster that queues funcs and run them on the test goroutine, as the app runs them on the main thread (`-race` is part of `tests`). Store tests move a fake clock (`Client.now`) instead of sleeping.
- A test never sleeps for the UI: it advances the tester's clock. (Fetch goroutines are the exception: poll with a deadline.)
- `internal/ui/uitest` wraps this for features. `uitest.Env(t)` (or `uitest.New(t, opts...)`, whose `Harness` also hands out the database, the engine and the clock) returns a `module.Env` over a real `services.Core` on a temporary SQLite library seeded with the synthetic fixture (`uitest/fixture`: 24 invented albums by 12 invented artists, three of them singles and three explicit, their songs and three playlists; never Apple's data), `playback/fakeengine` under the real coordinator, an artwork cache whose HTTP serves generated covers (`fixture.CDN`), a Poster that queues what goroutines post until the test drains it (`Harness.Main`), and a fake clock (`Harness.Clock`). Options: `Albums(...)` for a library of your own, `Empty()`, `Bare()` (no services: the bus, the playback mirror and the artwork loader only), `Dark()`. `h.Tester(t, view, w, h)` (or `uitest.Tester(t, env, view, w, h)`) is a `ui.Tester` plus `Until(what, cond)` (drains and draws frames until the goroutines delivered, 10 s at most), `Shows(texts...)`, `Drain()` and `Settle()` (frames until the picture stops changing). `Env` sets `MYGO_GPU=0` (the CPU renderer), English, and the light theme, and restores the theme afterwards.

`go test ./internal/ui/...` takes seconds. Golden images are few (plan §15): `uitest.Golden(t, tt.Image(), name)` compares with `testdata/golden/<name>.png` beside the test, by the mean colour of 8×8-pixel blocks, so another machine's text antialiasing matches but a moved edge, a changed colour or a missing cover does not (a block differs when a mean channel is more than 12 off; the test fails when more than 0.3% of the blocks differ), and `go test ./internal/ui/app -update` rewrites them when a change is on purpose. A failure saves the picture and a diff (differing blocks red) in the test's artifact directory (`go test -artifacts -outputdir DIR`; CI uploads it). Golden images are compared on Linux only (Pango shapes the text there; Core Text and DirectWrite place glyphs differently, which a tolerance should not hide): elsewhere `Golden` skips the test with that reason, after the test's behaviour assertions have run. Today: the shell with Albums, light and dark, at 1132×1253 (`internal/ui/app/testdata/golden`).

## The development loop

`devenv shell -- dev` runs `cmd/devloop` on `./cmd/sona`; `devenv shell -- gallery` runs it on `./cmd/uigallery`. The loop builds the target with `-tags dev,sqlite_fts5` into `bin/<name>-dev`, starts it, and watches `internal/`, `cmd/`, `build/`, `go.mod` and `go.sum` by polling (100 ms). A change to a file of a package the target is made of (`go list -deps`, so a sibling tool's files build nothing) builds again while the old app keeps running; a build that succeeds quits the app with SIGTERM (so it saves where it was), installs the binary and starts it with the same arguments and environment; a build that fails prints the compiler's output and leaves the app alone. `r` builds and restarts at once, `q` quits the app and the loop; the app quitting by itself (a closed window) ends the loop, a crash waits for the next change. Arguments after `--` go to the app; `-ldflags ""` keeps DWARF for a debugger. `devloop` prints how long each restart took, from the save to the app's `startup: first paint`.

- **Speed.** A restart is the compile of the changed packages and their importers (about 1 s for a change under `features/library`), the link, and the start. The link is cgo's: SQLite and the playback host's C++ shim make Go call the system C++ compiler, and before linking Go asks it half a dozen times whether it knows a flag by linking a trivial program (0.3 s each under nix's compiler wrapper, 1.5 s in all). `devloop` builds with `-ldflags "-w -extld=<itself>"` and answers those probes from a file after the first time (`os.UserCacheDir()/sona-devloop/extld`); `-w` drops DWARF (another second).
- **Restore** (`internal/ui/devstate`, `app/restore_dev.go`; dev builds only). When the app quits it writes `$XDG_STATE_HOME/sona/dev-restore.json`: the route and query, the scroll offset of each page, the flags pages registered (`devstate.Flag("queue.open", &q.open)`, for an open overlay or aside) and the window's size and place (Wayland gives no position). The next start applies it before the first frame, so a restart lands on the same screen and offset: a page calls `devstate.Scroll(c, &pos)` before the container that tracks `pos`, and the offset is asked for until the page is tall enough (its data arrives after the first frame). `SONA_DEV_FRESH=1` ignores the file. Another dev-tagged program calls `devstate.SetName("<name>")` first and gets `dev-restore-<name>.json` (the gallery's is `dev-restore-uigallery.json`), so it never overwrites the app's.
- **Two worktrees side by side.** In a dev build, `SONA_DEV_INSTANCE=<name>` (letters, digits and underscores kept, the rest become `_`; `internal/devinstance`) writes the restore file to `sona-<name>/dev-restore.json` and appends `.instance_<name>` to the single-instance D-Bus name (`dev.coredev.Sona`) and to the MPRIS name (`org.mpris.MediaPlayer2.sona`), so each worktree has its own primary instance, `sona status` and the other commands reach the one the same variable names, and `playerctl -p sona.instance_<name>` the right player. A production build ignores the variable. Data, settings and the cache are not separated: give each worktree its own XDG dirs, as AGENTS.md says.
- **Live tokens and catalogues.** A dev build reads `internal/ui/theme/tokens.json` from the source tree at start (`SONA_SOURCE`, which `devloop` sets, else where the compiler found the source), watches it and `internal/ui/i18n/locales/*.po`, and calls `theme.Reload` or `i18n.LoadCommunity` on a change: the next frame shows it, no build and no restart. An invalid file is logged and the old tokens stay. `devloop` does not build for these files.
- **The gallery** (`cmd/uigallery`) shows every kit component, light and dark (the toolbar's switch), at a scale (the toolbar's slider, 50–200 %), with knobs; the sections list in the sidebar. A kit PR adds its own `cmd/uigallery/<area>.go`: `func init() { register(section{name: "Tabs", build: func(c *ui.Context, g *gallery) { … }}) }`. `g.dark` is the mode and `g.art` a `kit.ArtLoader` that serves a generated cover for any `src` (`coverURL(n, delay, fail, nonce)` makes one that loads late or fails). `helpers.go` has `text`, `heading`, `note`, `labelled`, `knob` and `panel`. Under `devloop` the gallery restarts on its section, mode and window like the app.

## Conventions

- Views are methods on the feature's state struct; build values (`ui.Element`) are never stored (`go tool mygo vet internal/ui` in `check`: it takes the directory, and runs `go vet` and MyGo's analyzers over everything under it).
- Off the main thread: service calls, decoding, file I/O. Back through `store` or the Poster only.
- Logs never carry tokens or personal data (AGENTS.md).
- Performance rules (AGENTS.md "Linux Performance Rules"): no layout, text shaping or image work per frame; `Scroll` and an own virtualisation for long lists; artwork at the exact size, off the main thread; backdrops are pre-blurred in Go.

## The app shell (`internal/ui/app`)

`main` builds the core, then `mygo.App.WhenReady` calls `app.New(Options{Core, Settings, Bus, Windows, Art, Log, Version, Dev, Icon})` on the main thread; `mygo.App.Run` returns when the window closes and `App.Close` releases the art loader and writes a pending window size. Two things exist before the core because the services keep them: the event bus (`store.NewBus`, `Deps.Emit`: events emitted before the window exists queue and flush in order) and the window control (`app.NewWindows`, `Deps.Windows`, `Focus` and `Focused`: `Windows` implements `services.Windows` for the main window and does nothing until `New` attaches it). `Deps.Clipboard` is MyGo's clipboard (`mygo.Clipboard.Write`), `Deps.OpenURL` is `mygo.Shell.OpenExternal`.

`New` builds, in this order: the `Env` (the bus with `Window.Update` as its Poster, `store.Client`, the `store.Playback` mirror, the `kit.ArtLoader`, `Env.Post`: the same Poster, for a goroutine to hand a result to the view), the modules of `modules.go`, the router, the shortcut dispatcher, and the main window (the remembered size and maximised state from `config.Settings.Window`, 940×600 minimum, title "Sona", the inspector only in a dev build with `SONA_INSPECTOR=1`). Then, on the main thread, it follows the settings (theme mode, reduce effects, the language), seeds the mirror from `PlaybackService.State` and `Queue`, and runs every module's `Init`.

- **Routes and the start page.** Every module's `Routes` are matched in registration order by one `ui.Router.View`; a path nothing matches replaces itself with the start page. The window opens on `Desktop.StartPage` when a module registered that route, else on the first sidebar link (primary, then library, then footer, by `Order`): until Home is ported that is Albums. Resuming the last route, the first-run setup and deep links are phase 3.
- **Layout** (`AppShell.tsx`): the sidebar (232 DIP, `surface`; the mark and "Sona" in its 48 DIP header, the primary links, then a scrolling middle with the "Library" heading, the library links and the modules' `SidebarSection`s, then the `SidebarFooter`s and the footer links) | a 1 DIP `line` | the main column: the top bar (48 DIP, `surface`: MyGo's back and forward buttons on the router, the `TopBarSearch` slot, room to grab) over a 1 DIP `line` over the page row (the routed page on `ground`, the `Aside`s, and each open `Overlay` covering the page, which stays built, hidden, so it keeps its scroll). The `Bottom` slots run across the window under both; a `Layer` is drawn over everything and, while its `LayerOpen` holds, the window under it takes no input. A route or link exists only when its module is registered. A page owns its scroll container.
- **Links** are `ButtonBase` with the link role, active on their page and below it unless `Exact`; the keyboard and the screen reader get them by their label.
- **Shortcuts.** One dispatcher asks MyGo for every chord each frame (`c.Shortcut`), with the user's `config.Settings.Shortcuts` over the defaults, and runs the first shortcut bound to a pressed chord whose `When` holds; an `Unavailable` one takes the key and toasts and announces why. MyGo gives a focused text input the keys it types and a focused button or slider Enter, Space and the arrows first, which is what v0.2.0's `keyAllowed` did by hand. Each link gets `nav.<path>` (`nav.library.albums`): Ctrl+1…9 by position in the primary section, Alt+1…9 in the library's, unless the item names `Keys`. The shell's own is `shell.sidebar` (Ctrl+B). Back and forward are MyGo's (Alt+Left and Alt+Right, the mouse's side buttons), not shortcuts of ours. MyGo reports a key held down as presses, so `Shortcut.Repeat` has no effect yet.
- **First paint.** The first frame the root paints calls `SystemService.FirstPaint()` once, off the main thread, which logs `startup: first paint` (`since_start_ms`) and opens the network gate.
- **Language.** The Language setting (else the system's languages, the first that Apple Music's translations cover to `i18n.SelectableShare`) is looked up with `TranslationsService.Apple` off the main thread and shown through the Poster (`SetApple`, `SetLocale`); a new `EventAppleTranslations` or a changed Language setting chooses again.
- **Window size.** The main window's `OnResize`, `OnMaximize`, `OnUnmaximize` and full-screen events feed `observeWindow` (v0.2.0's rule: the size is only taken while the window is neither maximised nor full screen, so un-maximising returns to it); a change is written to `Settings.Window` half a second after it stops, and on close. Wayland gives no position, so none is kept.
- **Not here yet (phase 3).** The tray, the mini player, close to the background, `--hidden` (the window always opens), deep links, route memory, Sona's own title bar buttons, and MyGo's now playing (MPRIS stays Go's: `linkNowPlaying` is never linked).

### The player bar (`features/player`)

The bar is a `Bottom` slot: 72 DIP, `surface`, a `line` above. The cover (`kit.Artwork`, 48 DIP), the title and the line under it (artist · album; the sign-in, Widevine and error notices; a 30-second preview carries the "Preview" mark), the transport (shuffle, previous, play or pause, next, repeat), the seek bar and the volume. It draws `store.Playback`: the position is `Position()` and ticks every 250 ms while playing; a drag shows where it will go and seeks once, when the pointer lets go. Every command is a goroutine over `services.PlaybackService` (through the small `transport` interface the tests fake); a failure comes back through `Env.Post` and shows as a toast titled by the command and worded by `i18n.ErrorText` of its code. Controls Go says do not apply (the snapshot's `Controls`) are disabled, with the reason (a live station, a station) in the tooltip. Shortcuts keep v0.2.0's IDs and keys: `player.toggle` (Space), `player.next`/`previous` (Ctrl+Right/Left), `player.seek-forward`/`seek-back` (Shift+Right/Left, 10 s), `player.volume-up`/`down` (Ctrl+Up/Down), `player.mute` (M), `player.shuffle` (S), `player.repeat` (R). The queue, lyrics, Now Playing, sleep timer, favourite and mini player buttons come with their features.

### Library: Albums (`features/library`)

`/library/albums` with the library section's `Albums` link (icon `disc-3`, order 20) and the page of v0.2.0's `LibraryAlbumsPage`: the title and "N albums" (or singles and EPs), the Album type control (All, Albums, Singles & EPs) and the Sort select, whose choices live in the URL (`?sort=…&desc=…&type=…`, `Router.Replace`), then the cover grid. Data is `store.Use` over `LibraryService.Albums` in pages of 200 (key `["library","albums",sort,desc,page,type?]`, never stale until invalidated) and `Counts`; `EventSyncProgress` (a finished domain: its lists, the counts), `EventSyncDone` (everything under `library`) and `EventItemState` invalidate as v0.2.0's module did. The grid is built by hand over `ui.Scroll` and `TrackScroll`: MyGo's `List` keeps its place by row, not by an offset, and the header scrolls with the covers; only the rows showing and three beyond are built, the tile width is the exact one (minimum 160, gap 24) and `kit.Artwork` decodes covers at that size. A tile's hover play button plays the album (`LibraryService.Album`, then `PlaybackService.PlayItems` with `PlayFrom{album, id}`); the tile itself goes nowhere until the album page is ported. An empty library says why (signed out, syncing, nothing yet) with a row of empty cover slots; a type with nothing in it keeps the page and offers Show all albums.

### Tests

The shell's tests build fake modules and drive the view over `uitest`'s tester (nav entries, clicks, Ctrl and Alt chords, first paint once, the settings' start page and bindings, slots, text fields keeping their keys); its golden test runs the real modules over the fixture library. The player's run the bar over a fake `transport` on a bare `uitest` Env; the library's over the full Env: a real `services.Core` on the seeded SQLite library, the fake engine, the covers' fake CDN: sort, type, paging, sync events, the play button, empty states. Fetch goroutines are waited for with `Session.Until` (a deadline); everything else advances the tester's clock.

The end-to-end smoke (`e2e/`, build tag `e2e`) is the built app seen from outside. `headless -- go test -tags e2e ./e2e/...` (the devenv `e2e` script; CI job "E2E (headless sway)") starts a headless sway with a private session bus and AT-SPI bus (`e2e/headless.sh`), builds `bin/sona-e2e` with `-tags probe,sqlite_fts5` (MyGo's automation plugin, a fake playback engine and an HTTP seam that sends all the app's traffic to a local server of the fixture's covers: `cmd/sona/e2e_probe.go`, `SONA_E2E=1`, `SONA_E2E_CDN`), starts it on the fixture profile with no network, and drives it through the automation socket (`e2e/automation`): first paint, Albums listing the fixture albums, a click on an album's play button, the player bar playing, Space pausing and resuming, a captured PNG, a clean quit on SIGTERM. A second test reads the shell's landmarks (application and window, sidebar link, history buttons, page region, album play buttons, the player's transport and sliders) from the AT-SPI bus by role and name (`e2e/atspi`, D-Bus only). `E2E_OUT=<dir>` keeps the capture and the app's stderr; `E2E_BIN` skips the build. The probe build never ships (`probe` and `mygo_noinspector` exclude each other).
