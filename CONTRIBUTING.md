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
`cmd/playspike` and live tests. Never print, log or commit it. It may also
hold `SONA_LASTFM_KEY`, `SONA_LASTFM_SECRET` and `SONA_DISCORD_CLIENT_ID`:
production builds compile them in (`-X`, with `-trimpath` so the flags stay
out of the binary's build info), development builds read them from the
environment at runtime. Never pass them in `-ldflags` of a build without
`-trimpath`: Go records the flags in the build info and Wails' development
builds log them as `Build Info:` (the log handler and the diagnostics bundle
redact `-ldflags` values, but don't rely on that).

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

### Widevine CDM

devenv ships no CDM. The app downloads it itself, with no prompt, once the
network opens after the first paint: the version and SHA-512 come from
Mozilla's Firefox GMP manifest, the package from Google's servers, and its
CRX3 signatures are verified (`internal/playback/cdm`; see
`docs/security-review.md`, "Widevine CDM downloader"). It lives in
`<data>/widevine/<version>/<platform>/` behind `current.json` (`linux_x64`;
`mac_arm64` or `mac_x64` on macOS, where the `.sig` file stays beside the
dylib), is checked for updates daily (a library that no longer matches the
SHA-256 recorded at install is removed and downloaded again; 3 failed host
starts roll back to the previous version), and Settings → Playback shows its
state with a Retry button. Browser and system copies are never used.

- `SONA_WIDEVINE_CDM=/path/to/libwidevinecdm.so` (`.dylib` on macOS, or the
  directory holding it), in the environment or `.env`, is an explicit
  developer override: the app and tools load that file instead and download
  nothing.
- `go run ./cmd/cdmfetch` downloads, verifies and installs the CDM the way
  the app does: `-dir DIR` installs elsewhere (default Sona's
  `<data>/widevine`), `-platform goos/goarch` fetches another platform's,
  and `-version` only prints the manifest's version for the platform.
- `playspike` uses the downloaded CDM (installing it on first use);
  `-cdm LIB` overrides it like `SONA_WIDEVINE_CDM`, `-cdm-dir DIR` picks the
  install directory. It prints the one it used (`cdm source source=download`).

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
base, good (`osxaudiosink`), bad and libav, on the same plugin path as on
Linux. Intel Macs (`x86_64-darwin`) are not supported: the pinned nixpkgs
(26.11) and devenv 2.4 dropped them.

The app builds and runs on macOS as an unsigned development build (no
Apple Developer account, no `.app` bundle, no packaging): its UI,
everything Go does (sign-in, library, sync, catalog) and playback through
the same CDM host as Linux, with the CDM Sona downloads from Google for
macOS (`mac_arm64`, see "Widevine CDM") and Core Audio output
(`osxaudiosink`, always the system's default output). There are no media
controls, notifications, sleep inhibit, autostart, file chooser or `sona`
commands yet (see "Platforms"). Whether Apple licenses an unsigned macOS
host is what `playspike` answers.

The CDM host child runs under macOS's sandbox (`sandbox-exec` with Sona's
profile: no network, no writes outside the temporary folders, Sona's folder
and the keychain out of reach; `docs/security-review.md`, "macOS
sandbox"). Where `/usr/bin/sandbox-exec` is missing or refuses the profile,
it runs unsandboxed and Diagnostics → Host sandbox says "Not sandboxed" and
why. `SONA_SEATBELT=0` turns the sandbox off for one run, to tell a sandbox
problem from a playback one.

Setup, from a clean clone (Nix from the Determinate or the upstream
installer, devenv 2.4 or newer):

```sh
git clone https://github.com/coredev-uk/sona-go.git && cd sona-go
devenv version                       # 2.4.0 or newer
devenv shell                         # the first run downloads the toolchain
# inside the shell:
gst-inspect-1.0 osxaudiosink avdec_aac | grep -E '^  (Name|Long-name)'
pnpm -C frontend install --frozen-lockfile
go vet ./... && go test ./...
SONA_PRIVATE=1 dev                   # the app with Vite hot reload
```

`dev` (and `build`) build `bin/sona` for macOS through
`build/darwin/Taskfile.yml`; `build DEV=true && SONA_PRIVATE=1 ./bin/sona`
runs it without hot reload. Expect the window with every page; Settings
without the controls macOS can't support yet (the output device, start at
login, notifications, keeping the Mac awake); the keyring backend
"Keychain"; data under `~/Library/Application Support/Sona` and
`~/Library/Caches/Sona` (log: `State/sona.log`). The D-Bus tests skip (no
`dbus-daemon`).

The CDM: Sona and `playspike` download it themselves; no browser is
needed. To check the download and the library by hand:

```sh
go run ./cmd/cdmfetch -dir /tmp/sona-cdm          # prints version=… path=…/mac_arm64/libwidevinecdm.dylib
ls -l /tmp/sona-cdm/*/mac_arm64/                    # the dylib and its .sig, side by side
# Loads Google's CDM and makes a licence request in-process (no Apple, no
# token); downloads into the test cache first:
SONA_CDM_REQUIRE=1 go test -count=1 -v -run 'TestCDM$' ./internal/playback/widevine
# Only if the downloaded dylib fails to load, report (don't change the file):
xattr -l <path>; codesign -dv --verbose=2 <path>; file <path>
# The sandbox:
ls -l /usr/bin/sandbox-exec && sandbox-exec -p '(version 1)(allow default)(deny network*)' /usr/bin/true && echo sandbox-exec works
go test -count=1 -v -run TestSeatbeltConfinesChild ./internal/playback/cdmhost
```

The DRM path regression and licence check (`.env` holding
`MUSIC_USER_TOKEN`; always in private mode; no `SONA_WIDEVINE_CDM`, so it
uses the downloaded CDM; audio plays on the default output, and `-device`
is refused on macOS):

```sh
env -u SONA_WIDEVINE_CDM SONA_PRIVATE=1 go run ./cmd/playspike -seek 90 -gap=false
env -u SONA_WIDEVINE_CDM SONA_PRIVATE=1 go run ./cmd/playspike -activity   # only with SONA_PRIVATE=1
```

It prints `cdm source source=download` with the library under
`~/Library/Application Support/Sona/widevine/`, then each `licence
exchange` (HTTP status, Apple's status and message, the licence's length;
never the token or the licence), and ends with `licence: granted`,
`licence: refused (<why>)` or `licence: not requested (<why>)`.

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
`internal/diag`, `internal/playback/cdm`) find the CDM through
`internal/playback/cdm/cdmtest`: the `SONA_WIDEVINE_CDM` override, else the
CDM installed in `SONA_CDM_CACHE` (default
`<user cache dir>/sona-tests/widevine`, shared by every checkout),
downloaded and verified there on first use. They skip only when the
download cannot reach the network; `SONA_CDM_REQUIRE=1` turns that skip into
a failure. CI ("Resolve the Widevine CDM version") runs
`go run ./cmd/cdmfetch -version`, restores `~/.cache/sona-ci/widevine` with
`actions/cache` keyed `widevine-<os>-<arch>-<version>`, and ("Install the
Widevine CDM") fills a miss with
`go run ./cmd/cdmfetch -dir ~/.cache/sona-ci/widevine`, exporting
`SONA_CDM_CACHE` and `SONA_CDM_REQUIRE=1`. "Real Widevine CDM tests (verbose,
must not skip)" then runs `TestCDM`, `TestProbe`, `TestSessionChallenge`,
`TestPlayClearTrack`, `TestNextTrackAdvances`, `TestChildExit` and
`TestChildCrashInBundle` with `-v`.
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
every OS-specific part behind a seam that reports itself unsupported there; macOS development builds also
play through the CDM host.

- `main.go` is portable; `platform_linux.go` and `platform_other.go` build
  what differs per OS: the `sona <command>` transport and single instance
  (`internal/app` over D-Bus), the Wails options, and `services.Platform`:
  media controls (MPRIS), notifications, the sleep inhibitor, the
  on-battery monitor, the autostart entry, the file chooser, the tray. The
  playback engine and the CDM host child hook (`newEngine`, `childMain`,
  `cdmLoadCheck`) are `engine_cdm.go`'s on Linux and macOS (the CDM host
  engine, loading the downloaded CDM) and `engine_other.go`'s elsewhere
  (`playback.Unsupported`, and no download). A nil seam is
  unsupported; the service using it does nothing or returns an error
  wrapping `errors.ErrUnsupported` (error code `unsupported`), and
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
- Code that only compiles on some systems is tagged. The CDM host engine
  (`internal/playback/gst`, `widevine`, `cdmhost`, `native`) and
  `cmd/playspike` carry `//go:build linux || (darwin && cgo)`: cgo against
  GStreamer and the C++ shim, which a macOS build has anyway (Wails needs
  cgo there). Inside them, what differs is in `_linux.go`/`_darwin.go`
  files: the sink and the output device (`gst/output_<os>.go`; PipeWire's
  device provider in `devices_linux.c`, its relink detection in
  `relink_linux.c`), the sandbox (`cdmhost/sandbox_linux.go`,
  `sandbox_fs_linux.go`; `sandbox_darwin.go` and the portable
  `seatbelt.go`). WebKitGTK (`webkit_linux.go`) and GTK
  (`internal/shell/*_linux.go`) are Linux-only. Portable Go implementations
  of Linux protocols (the D-Bus clients in `internal/mpris`,
  `internal/desktop/*`, `internal/power`, `internal/app`; the XDG autostart
  entry) stay untagged, so they type-check and their tests run everywhere;
  only `platform_linux.go` uses them.
- Per-OS conventions: config paths (`internal/config`: XDG; `~/Library/...`
  on macOS; `%APPDATA%`/`%LOCALAPPDATA%` on Windows), the keyring backend
  (go-keyring: Secret Service, Keychain, Credential Manager), the Discord
  IPC socket (Unix sockets; unsupported on Windows, which uses a named
  pipe), the audio sink (`pipewiresink`; `osxaudiosink`), the CDM library
  (`libwidevinecdm.so`; `libwidevinecdm.dylib`).
- Type-checking: every package, tests included, must type-check for
  `windows/amd64` and `darwin/arm64` with cgo off (CI and `check`). Windows
  uses `go vet`. On macOS Wails' `pkg/application` is Objective-C behind
  cgo, so `go vet` stops there; `go run ./cmd/typecheck -os darwin -arch
  arm64 ./...` type-checks Sona's packages from source and only names the
  third-party packages that need cgo. Real builds need cgo on both:
  `mattn/go-sqlite3` everywhere, and Wails on macOS (Cocoa and WebKit from
  Apple's SDK, so macOS builds happen on a Mac). Wails on Windows is pure Go
  (its own WebView2 loader).
- The macOS CDM host engine builds only with cgo, so the cgo-off
  type-check skips it. After touching it, type-check it with cgo on from
  Linux, with zig as a macOS C compiler. Linux's GStreamer headers lack
  the macOS-only `gst/gstmacos.h` that `gst/gst.h` includes on Apple
  targets, so take it from the same GStreamer's source:

  ```sh
  zig=$(nix build --no-link --print-out-paths nixpkgs#zig)/bin/zig
  inc=$(mktemp -d) && mkdir -p "$inc/gst"
  src=$(nix build --no-link --print-out-paths "github:NixOS/nixpkgs/$(sed -n 's|.*github:NixOS/nixpkgs/||p' devenv.yaml)#gst_all_1.gstreamer.src")
  tar -xOf "$src" --wildcards '*/gst/gstmacos.h' > "$inc/gst/gstmacos.h"
  devenv shell -- env CGO_CFLAGS="-I$inc" CC="$zig cc -target aarch64-macos" \
    CXX="$zig c++ -target aarch64-macos" go run ./cmd/typecheck -os darwin -arch arm64 -cgo ./...
  ```

  It compiles the C and C++ (shim, GStreamer glue) for macOS and
  type-checks every Sona package, `main` included; only Wails' Objective-C
  fails, without Apple's SDK.
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
  (`internal/playback/cdmhost`). It loads the Widevine CDM Sona downloaded
  (or the developer override) and decrypts only through the CDM's own `Decrypt` API.
- Decrypted audio never leaves the child: it is never written to disk and
  never crosses the pipe to the parent. Audio processing happens in the
  child, after decryption.
- The child makes no network requests; the parent fetches everything and
  passes bytes in.
- The CDM is never bundled (in packages or the binary), redistributed or
  patched. It is downloaded only from Google's servers, with the version
  and SHA-512 from Mozilla's pinned manifest and the CRX3 signatures
  verified, and stored in Sona's data dir. Browser and system copies are
  never used; `SONA_WIDEVINE_CDM` is a developer override only.
- Never extract keys or use reimplemented CDMs (`openwv`, extracted device
  keys). Do not try to make Widevine work in WebKitGTK.

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
