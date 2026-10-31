# Contributing to Sona

Read this, then `AGENTS.md` (the binding rules) and the doc for the area you
touch.

## Environment

Everything runs through [devenv](https://devenv.sh) 2.4 or newer
(`devenv.yaml` refuses an older CLI). It pins Go 1.26, Node 24 + pnpm and
GStreamer, plus GTK4/WebKitGTK 6 on Linux:

```sh
devenv shell                     # interactive shell
devenv shell -- <command>        # one command
# Without installing devenv. Plain `nixpkgs#devenv` follows your registry,
# which may carry an older devenv.
nix run github:NixOS/nixpkgs/nixpkgs-unstable#devenv -- shell
```

It sets `CGO_ENABLED=1`, `GOFLAGS=-tags=sqlite_fts5`, the GStreamer plugin
path (core, base, good, bad, libav and, on Linux, PipeWire; never the host's
plugins) and, on Linux, glib-networking's GIO module path, and loads `.env`.

- `go tool wails3 build` builds the frontend, regenerates the TypeScript
  bindings and writes `bin/sona` (production; `DEV=true` for a debug build).
  `go tool wails3 dev` runs with Vite hot reload.
- `go tool wails3 generate bindings -clean=true -ts` regenerates
  `frontend/bindings` after changing an exported service method or DTO.
  Commit the result.
- `sqlc generate -f internal/db/sqlc.yaml` after changing queries; migrations
  live in `internal/db/migrations` (goose).
- `go build .` needs `frontend/dist` (build the frontend first).
- The version is the `VERSION` file; the build injects it with
  `-ldflags -X main.version` (`bin/sona --version`).

Local secrets: `.env` (ignored) may hold `MUSIC_USER_TOKEN` for
`cmd/playspike` and live tests. Never print, log or commit it.

Private mode for development and testing: a development build (any build
without the `production` tag: `wails3 dev`, `go run .`, plain `go build`)
starts with private listening on, whatever the saved setting says. Toggling it
then lasts for the session only and never writes the setting; the toggle reads
"Private (development build)". `SONA_PRIVATE=1` forces private mode in any
build (it cannot be turned off for that run); `SONA_PRIVATE=0` makes a
development build follow the saved setting, for a deliberate live check.
`cmd/playspike -activity` is private by default (`-private=false` reports the
plays to the account), and the live tests refuse to run without
`SONA_PRIVATE=1`. Every test, smoke or live check that plays music runs in
private mode.

### devenv scripts

| Script       | What it runs                                                                                                                                                                                                                                                                                             |
| ------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `dev`        | `go tool wails3 dev -config ./build/config.yml -port $WAILS_VITE_PORT` (port 9245 when unset), as Taskfile's `dev`: Vite hot reload, rebuild and restart on Go changes. Extra arguments go to `wails3 dev`.                                                                                              |
| `build`      | `go tool wails3 build`: frontend, bindings, `bin/sona` (production; `build DEV=true` for a debug build).                                                                                                                                                                                                 |
| `gen`        | `sqlc generate` and `wails3 generate bindings -clean=true -ts`. Commit the result.                                                                                                                                                                                                                       |
| `fmt`        | gofumpt on every tracked Go file except `internal/db/dbq`, then `prettier --write src`. Installs the frontend dependencies first when `package.json` or the lockfile changed.                                                                                                                            |
| `check`      | The static half of the checks below: installs the frontend dependencies and builds `frontend/dist` (development build) when their sources changed, since Go embeds it; then `go vet`, the Windows and macOS type-checks, `golangci-lint --new-from-rev=origin/main`, the path-case check, tsc, eslint, `prettier --check` and `i18n:check`. |
| `tests`      | The same frontend step, then `go test -race ./...` (inside `dbus-run-session` on Linux, so no test reaches your session bus; extra arguments go to `go test`) and vitest. Not named `test`: the shell's `test` builtin would shadow it.                                                                  |
| `release`    | `build/linux/release/release.sh`, the same as `go tool wails3 task release`: release packages built on Ubuntu 24.04 in podman or docker (falling back to `nix run nixpkgs#podman`) and smoke-tested, into `bin/release`.                                                                                 |
| `play-spike` | `go run ./cmd/playspike`: the DRM path regression (needs `MUSIC_USER_TOKEN`).                                                                                                                                                                                                                            |
| `auth-spike` | `go run ./cmd/authspike`: the Apple sign-in flow from the terminal.                                                                                                                                                                                                                                      |

The frontend step in `check` and `tests` is Taskfile's
(`common:install:frontend:deps`, `common:build:frontend`), which also
regenerates `frontend/bindings` when Go sources changed, as `build` does.
`package` is only Taskfile's (`go tool wails3 package`): it packages a build
of the machine it runs on, which is what `release` runs inside its Ubuntu
container. A package built on NixOS would link against `/nix/store`.

### Widevine CDM in development

devenv ships no CDM. Sona and `playspike` find one the way the app does for
users (`widevine.FindCDM`): Google Chrome under `/opt/google` or on `PATH`,
the `WidevineCdm` component directory of a Chrome, Chromium, Brave, Vivaldi
or Edge profile under `~/.config`, or a Firefox or Zen `gmp-widevinecdm`
profile directory (`~/.mozilla/firefox`, `~/.zen`, `~/.config/zen`). Any of
these browsers, once it has downloaded its CDM, is enough. To use a specific
copy, set `SONA_WIDEVINE_CDM=/path/to/libwidevinecdm.so` (or the directory
holding it) in the environment or in `.env`; Settings → Playback → CDM path
does the same in the app. `playspike` prints the one it used (`cdm path`).

### Pinned inputs

`devenv.yaml` pins nixpkgs to one nixos-unstable revision and devenv's
modules to one revision, so every machine gets the same toolchain. To bump
nixpkgs, put the current nixos-unstable head
(`git ls-remote https://github.com/NixOS/nixpkgs refs/heads/nixos-unstable`)
in `devenv.yaml`, run `devenv update`, then `check`, `tests`, `play-spike`
and the app, and commit `devenv.yaml` and `devenv.lock` together.

### macOS

`devenv shell` works on Apple Silicon (`aarch64-darwin`): the Linux-only
packages (GTK4, WebKitGTK, libsoup, glib-networking, PipeWire, playerctl,
gcc, dbus) and the GIO module path are left out, and GStreamer keeps core,
base, good (`osxaudiosink`), bad and libav. Intel Macs (`x86_64-darwin`)
are not supported: the pinned nixpkgs (26.11) and devenv 2.4 dropped them.

The app builds and runs on macOS with its UI and everything Go does
(sign-in, library, sync, catalog), but without playback, media controls,
notifications, sleep inhibit, autostart, the file chooser or `sona`
commands yet (see "Platforms"); the Taskfile and `dev` still target Linux,
so build it directly. From a clean clone:

```sh
devenv shell -- bash -c '
  pnpm -C frontend install --frozen-lockfile && pnpm -C frontend build &&
  go build -o bin/sona . && go vet ./... && go test ./...'
SONA_PRIVATE=1 ./bin/sona
```

That is a development build (private listening on). Expect the window with
every page; Settings without the controls macOS can't support yet (the
equaliser, Sound Check, crossfade, the output device, the CDM path, start at
login, notifications, keeping the Mac awake); Settings → Playback and
Diagnostics saying playback isn't available on this platform; the keyring
backend "Keychain"; data under `~/Library/Application Support/Sona` and
`~/Library/Caches/Sona` (log: `State/sona.log`). The D-Bus tests skip (no
`dbus-daemon`).

## Checks

Run before sending a change (CI runs the same on every push and pull request,
`.github/workflows/ci.yml`; the devenv `check` and `tests` scripts run all of
it):

```sh
go vet ./...
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go vet ./...   # see "Platforms"
go run ./cmd/typecheck -os darwin -arch arm64 ./...
go test -race ./...
golangci-lint run --new-from-rev=origin/main ./...   # must be clean
cd frontend
node scripts/check-path-case.mjs   # no paths or module names differing only by case
pnpm exec tsc --noEmit
pnpm exec eslint src
pnpm exec prettier --check src     # --write to fix
pnpm i18n:check
pnpm exec vitest run
```

Real-CDM tests (`internal/playback/widevine`, `internal/playback/cdmhost`,
`internal/diag`) run only when `SONA_WIDEVINE_CDM` names a CDM (see "Widevine
CDM in development"); CI skips them.
After touching playback or Apple request code, also run the DRM path
regression `go run ./cmd/playspike -seek 90` (needs `MUSIC_USER_TOKEN`).
Every playback run in tests, smokes and live checks uses private mode
(`SONA_PRIVATE=1`; development builds start private and `playspike` is
private by default), so nothing is reported to Apple or scrobbled.
Route long listening tests to your own PipeWire null sink, not your
speakers, and remove only the sinks you created.

When running the app for a check that doesn't need your desktop, start it
under `dbus-run-session` with private `XDG_CONFIG_HOME`, `XDG_DATA_HOME`,
`XDG_STATE_HOME` and `XDG_CACHE_HOME`, so it neither clashes with a running
Sona (single instance, MPRIS name) nor touches your real settings and library.

The CDM host sandbox tests (`TestChildCannotOpenInetSockets`,
`TestSandboxConfinesChild/namespaces`) need unprivileged user namespaces in
which the child may change its mounts. Where the system refuses them (EPERM
or EACCES: Flatpak, user namespaces disabled, or Ubuntu 24.04's AppArmor
restriction `kernel.apparmor_restrict_unprivileged_userns=1`) they skip and
say why; any other failure fails them. To run them on Ubuntu, lift the
restriction for the session with
`sudo sysctl -w kernel.apparmor_restrict_unprivileged_userns=0`. CI does, and
sets `SONA_CDMHOST_REQUIRE_NAMESPACES=1`, which turns a skip into a failure.

Packaging changes: `bash build/linux/release/release.sh` builds every package
on Ubuntu 24.04 and smoke-tests it on clean Ubuntu, Fedora and Arch
containers. Before a release, work through `docs/release-checklist.md`.

## Branches and pull requests

- One change per branch and pull request. Parallel work is easiest in
  separate worktrees (`git worktree add ../sona-go-<slug> -b <type>/<slug>`);
  a new worktree needs `pnpm install` in `frontend/` and a frontend build
  before `go build .`. Give each concurrently running worktree its own
  `WAILS_VITE_PORT`.
- Commit messages and PR titles follow
  [Conventional Commits](https://www.conventionalcommits.org): `feat`, `fix`,
  `perf`, `refactor`, `docs`, `test`, `ci`, `build`, `chore` or `revert`,
  with an optional scope (`fix(playback): …`). CI fails a PR with a
  non-conforming title; PRs are squash-merged under that title.
- Fill in the PR template: what changed, why, evidence (commands and their
  output; screenshots in light and dark themes for UI changes, without
  personal account data), tests, docs, and anything left.
- Update the docs describing what you change in the same PR.
- Generated files are never edited or merged by hand. After rebasing onto
  `main`, regenerate `frontend/bindings`, the Lingui catalogues
  (`pnpm i18n:extract` in `frontend/`) and `internal/db/dbq` (`gen`), then
  commit them.

## Module contract

A feature is a Go service plus a frontend module, each self-contained:

- Go: `internal/services/<feature>.go` (the service; exported methods are the
  bindings) + `<feature>_dto.go` (its DTOs and events) + one line in
  `internal/services/registry.go`. Features never call each other's
  services. Full contract: `internal/services/doc.go`.
- Frontend: `frontend/src/features/<name>/index.ts` exporting the module
  (routes, nav items, Go event handlers) + one line in
  `frontend/src/app/modules.ts`. Full contract:
  `docs/frontend-architecture.md`.
- Settings: one nested struct in `internal/config/settings.go` with its
  defaults and `normalized()`; apply changes live through
  `Settings.OnChange`.

## Platforms

Linux is the full platform. macOS and Windows build and run the UI, with
every OS-specific part behind a seam that reports itself unsupported there:

- `main.go` is portable; `platform_linux.go` and `platform_other.go` build
  what differs per OS: the playback engine (`newEngine`: the CDM host engine
  on Linux, `playback.Unsupported` elsewhere), the CDM host child hook, the
  `sona <command>` transport and single instance (`internal/app` over
  D-Bus), the Wails options, and `services.Platform`: media controls (MPRIS),
  notifications, the sleep inhibitor, the on-battery monitor, the autostart
  entry, the file chooser, the tray. A nil seam is unsupported; the service
  using it does nothing or returns an error wrapping
  `errors.ErrUnsupported` (error code `unsupported`), and
  `PlatformService.Capabilities` tells the frontend, which hides or
  explains the control (`internal/services/doc.go`,
  `docs/frontend-architecture.md`).
- Build tags: a file for one OS is named `_linux.go`, `_darwin.go` or
  `_windows.go` (Go applies the constraint from the name); the fallback for
  the rest is `_other.go` with the complementary `//go:build` line
  (`!linux`). A file for a group of systems carries an explicit line and a
  descriptive name: `//go:build unix` (`internal/discord/ipc_unix.go`),
  `//go:build !darwin && !windows` (XDG paths in
  `internal/config/paths_xdg.go`, the Secret Service backend name in
  `internal/auth/store/keyring_other.go`). Tests that only make sense on one
  OS follow their file.
- Code that only compiles on Linux (Linux syscalls, cgo against Linux
  libraries) is tagged: the whole CDM host engine (`internal/playback/gst`,
  `widevine`, `cdmhost`, `native`) and `cmd/playspike` carry
  `//go:build linux`, as do WebKitGTK (`webkit_linux.go`) and GTK
  (`internal/shell/*_linux.go`). Portable Go implementations of Linux
  protocols (the D-Bus clients in `internal/mpris`, `internal/desktop/*`,
  `internal/power`, `internal/app`; the XDG autostart entry) stay untagged,
  so they type-check and their tests run everywhere; only
  `platform_linux.go` uses them.
- Per-OS conventions: config paths (`internal/config`: XDG; `~/Library/...`
  on macOS; `%APPDATA%`/`%LOCALAPPDATA%` on Windows), the keyring backend
  (go-keyring: Secret Service, Keychain, Credential Manager), the Discord
  IPC socket (Unix sockets; unsupported on Windows, which uses a named
  pipe).
- Type-checking: every package, tests included, must type-check for
  `windows/amd64` and `darwin/arm64` with cgo off (CI and `check`). Windows
  uses `go vet`. On macOS Wails' `pkg/application` is Objective-C behind
  cgo, so `go vet` stops there; `go run ./cmd/typecheck -os darwin -arch
  arm64 ./...` type-checks Sona's packages from source and only names the
  third-party packages that need cgo. Real builds need cgo on both:
  `mattn/go-sqlite3` everywhere, and Wails on macOS (Cocoa and WebKit from
  Apple's SDK, so macOS builds happen on a Mac). Wails on Windows is pure Go
  (its own WebView2 loader).
- A real Windows build cross-compiles from Linux with zig as the C
  compiler (`nix shell nixpkgs#zig`; Wails' own cross image uses it too):
  `GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC='zig cc -target
  x86_64-windows-gnu' go build -tags production,sqlite_fts5 -ldflags '-H
  windowsgui' .` after a production frontend build, with `go tool wails3
  generate syso` for the icon and manifest. zig ignores the `-mwindows` Go
  passes for `-H windowsgui`, so wrap it to pass
  `-Wl,--subsystem,windows` instead, or the exe opens a console. It needs
  the WebView2 runtime to show its window.

## Architecture rules

- Go owns all Apple Music traffic, tokens, sync, caching, MPRIS and artwork.
  The frontend never holds tokens and never calls `*.apple.com`.
- Cold start reads SQLite only; the network and the playback host start
  after the first paint.
- Secrets go to the system keyring (Secret Service, Keychain, Credential
  Manager through go-keyring) or the explicit 0600 file fallback, never to
  the database, logs, settings or committed files.
- Linux performance: no `backdrop-filter` or large CSS blurs on scrolling
  content; backdrops are pre-blurred in Go.

## DRM rules (absolute)

- Protected playback runs in the CDM host child process
  (`internal/playback/cdmhost`). It loads the user's installed Widevine CDM
  and decrypts only through the CDM's own `Decrypt` API.
- Decrypted audio never leaves the child: it is never written to disk and
  never crosses the pipe to the parent. Audio processing happens in the
  child, after decryption.
- The child makes no network requests; the parent fetches everything and
  passes bytes in.
- Never redistribute, bundle, download or patch the CDM. Never extract keys or
  use reimplemented CDMs. Do not try to make Widevine work in WebKitGTK.

## Design rules

`docs/design.md` is the visual direction. Design every page fresh: Apple's
apps are functional references only, never layouts to copy. Fast before pretty;
light and dark follow the system; everything works from the keyboard; states
(signed out, syncing, offline, no CDM) are stated plainly.

## Licence

By contributing you agree your contribution is licensed under the AGPL-3.0
(`LICENSE`). Code copied from elsewhere must carry a compatible licence and
its notice; the vendored Chromium CDM headers keep theirs
(`internal/playback/widevine/LICENSE.chromium`, BSD-3-Clause).
