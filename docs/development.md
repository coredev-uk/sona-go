# Development reference

The full detail behind the [contributing pages on the site](https://sona.coredev.uk/contributing/setup/): the environment, every devenv script, private mode, live playback, the Widevine CDM, and debugging the UI. `AGENTS.md` has the short rules; this page has the reasons and recipes.

## Environment

Everything runs through [devenv](https://devenv.sh) 2.4 or newer
(`devenv.yaml` refuses an older CLI). It pins Go 1.27 and
GStreamer, plus GTK3, Pango and the GL libraries MyGo loads on Linux:

```sh
devenv shell                     # interactive shell
devenv shell -- <command>        # one command
# Without installing devenv. Plain `nixpkgs#devenv` follows your registry,
# which may carry an older devenv.
nix run github:NixOS/nixpkgs/nixpkgs-unstable#devenv -- shell
```

With [direnv](https://direnv.net), the checked-in `.envrc` loads the same
environment whenever you `cd` into the checkout or a worktree: run
`direnv allow` once in each, and `go`, `dev`, `check` and the other scripts
work without the `devenv shell --` prefix.

It sets `CGO_ENABLED=1`, `GOFLAGS=-tags=sqlite_fts5`, the GStreamer plugin
path (core, base, good, bad, libav and, on Linux, PipeWire; never the host's
plugins) and, on Linux, `LD_LIBRARY_PATH` for GTK3 and friends (MyGo loads
them by soname, so they must come from the same nixpkgs pin as GStreamer:
one GLib per process), and loads `.env`.

- `build` writes `bin/sona` (production: no inspector, MyGo in production
  mode); `dev` builds the `dev` tag into `bin/sona-dev` and runs it. A plain
  `go build ./cmd/sona` is not a dev build: only `-tags dev` is (dev builds
  start private, show the inspector with `SONA_INSPECTOR=1`, panic on a
  message that doesn't parse). `-tags` replaces devenv's `GOFLAGS`, so a build with its own `-tags`
  names `sqlite_fts5` too.
- `sqlc generate -f internal/db/sqlc.yaml` after changing queries; migrations
  live in `internal/db/migrations` (goose).
- The version is the `VERSION` file; the build injects it with
  `-ldflags -X main.version` (`bin/sona --version`).

Local secrets: `.env` (ignored) may hold `MUSIC_USER_TOKEN` for
`cmd/playspike` and live tests. Never print, log or commit it. It may also
hold `SONA_LASTFM_KEY`, `SONA_LASTFM_SECRET` and `SONA_DISCORD_CLIENT_ID`:
production builds compile them in (`-X`, with `-trimpath` so the flags stay
out of the binary's build info), development builds read them from the
environment at runtime. Never pass them in `-ldflags` of a build without
`-trimpath`: Go records the flags in the build info (the log handler and the
diagnostics bundle redact `-ldflags` values, but don't rely on that).


## Private mode

Private mode for development and testing: a development build (any build
with the `dev` tag, as the `dev` script builds it) starts with private listening on, whatever the saved setting says. Toggling it
then lasts for the session only and never writes the setting; the toggle reads
"Private (development build)". `SONA_PRIVATE=1` forces private mode in any
build (it cannot be turned off for that run); `SONA_PRIVATE=0` makes a
development build follow the saved setting, for a deliberate live check.
`cmd/playspike -activity` is private by default (`-private=false` reports the
plays to the account), and the live tests refuse to run without
`SONA_PRIVATE=1`. Every test, smoke or live check that plays music runs in
private mode.

## devenv scripts

| Script       | What it runs                                                                                                                                                                                                                                                                                                                                |
| ------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `dev`        | Builds `cmd/devloop` into `bin/devloop` and runs it on `./cmd/sona` with your arguments after `--`: it builds the app with `-tags dev,sqlite_fts5` into `bin/sona-dev`, runs it, and rebuilds and restarts it when a Go file of the app changes (the old app keeps running during the build; a failed build prints its errors and leaves it; the restart lands on the same screen, scroll and window). Keys: `r` rebuilds, `q` quits. A first argument `-background` (`dev -background -- <app args>`) starts the app without taking the focus (`MYGO_DEV_BACKGROUND=1`), so a restart on a save leaves it in your editor. Design tokens and `.po` files reload in the running app without a build. See docs/ui-architecture.md, "The development loop". |
| `gallery`    | The same loop on `./cmd/uigallery`: the component gallery (every kit component, light and dark, with knobs), restarting like `dev`. |
| `build`      | `go build -tags mygo_noinspector,sqlite_fts5 -ldflags "$(bash build/ldflags.sh --no-credentials --goos "$(go env GOOS)")" -o bin/sona ./cmd/sona`: no inspector in the binary, MyGo in production mode (`build/ldflags.sh` is the one place the link flags are assembled: `-w -s`, the version, production mode, and per OS the macOS deployment target or `-H windowsgui`; `--no-credentials` leaves the Last.fm and Discord values out, which only `-trimpath` builds may carry). |
| `docs`       | `cd site && npm ci && npm run dev`: the documentation site with live reload (see `site/src/content/docs/contributing/documentation.md`). Node is only in the dev shell for this; the app's build and `check` don't use it. |
| `gen`        | `sqlc generate`. Commit the result. |
| `fmt`        | gofumpt on every tracked Go file except `internal/db/dbq`. |
| `check`      | The static half of the checks (`docs/testing.md`): `go vet`, `go vet -tags dev,sqlite_fts5` and `-tags probe,sqlite_fts5` (development and probe builds type-check too), the Windows and macOS type-checks, `golangci-lint --new-from-rev=origin/main`, `go tool mygo vet internal/ui` (build values kept past their frame), the catalogue check (`i18ntool extract -check`) and the path-case check (`cmd/pathcase`). |
| `tests`      | `go test -race ./...` (inside `dbus-run-session` on Linux, so no test reaches your session bus; extra arguments go to `go test`), including the `ui.Tester` tests, then `go test -race -tags dev,sqlite_fts5 ./internal/devinstance ./internal/ui/devstate ./internal/ui/app ./internal/ui/features/song ./internal/app ./internal/mpris` (the tests of dev-only code). Not named `test`: the shell's `test` builtin would shadow it. |
| `release`    | `build/linux/release/release.sh`, the same as Taskfile's `release`: builds every Linux package (.deb, .rpm, Arch, AppImage) on Ubuntu 24.04 in podman/docker and smoke-tests each on a clean distro container (Xvfb, `MYGO_GPU=0`). |
| `play-spike` | `go run ./cmd/playspike`: the DRM path regression (needs `MUSIC_USER_TOKEN`); `play-spike -preview` needs none.                                                                                                                                                                                                                             |
| `auth-spike` | `go run ./cmd/authspike`: the Apple sign-in flow from the terminal.                                                                                                                                                                                                                                                                         |
| `headless`   | `bash e2e/headless.sh "$@"`: runs a command (`headless -- <cmd>`) in a private headless session: sway (headless backend, pixman), private XDG directories (empty: write the fixture library into them first, `headless -- sh -c 'go run ./e2e/profile && <cmd>'`), its own session bus with the AT-SPI bus, the desktop portals stubbed (no 12 s wait for a portal that never starts) and a `wtype` keep-alive that gives the seat a keyboard. `WAYLAND_DISPLAY` is set for the command; the session and its files are removed afterwards. |
| `e2e`        | `headless -- go test -tags e2e -count=1 ./e2e/...`: the end-to-end smoke and the accessibility check (see `docs/testing.md`, "UI tests and the end-to-end smoke"). Extra arguments go to `go test`. |
| `orca-walk`  | Walks the app with a real Orca in `headless`: `E2E_ORCA=1 E2E_ORCA_BIN=<orca> headless -- go test -tags e2e -count=1 -v -run '^TestOrca' ./e2e/`, with Orca built from the pinned nixpkgs on first use (it is not in the dev shell). What it says is written to `bin/orca-out` (`E2E_OUT`); see docs/accessibility.md, "Listening with Orca". Not part of `e2e` or `tests`: it only runs when asked. |
| `probe`      | Builds the probe build (`-tags probe,sqlite_fts5`) into `bin/sona-probe` and runs `sona -probe` with your arguments: the Albums scroll probe in the app's own process (see `docs/performance.md`, "Measuring UI performance"). It needs a profile with albums whose covers are cached or reachable: the fixture library (`go run ./e2e/profile`) has covers only behind the e2e harness's local server (`SONA_E2E_CDN`), so outside the e2e tests the probe waits 60 s, prints "the covers were not all drawn", measures nothing and exits 1. On empty private directories it fails the same way with "showed no tiles". Run it as `headless -- probe` on a profile you copied into the private directories, or on your desktop session's own (keep that run short). |

`package` is only Taskfile's (`task package`: .deb, .rpm and Arch with nfpm, and
the AppImage): it packages a build of the machine it runs on. A package built
on NixOS would link against `/nix/store`, so release packages come from `release`.
The macOS and Windows release builds are `build/darwin/bundle.sh` and
`build/windows/package.sh`, run by `release.yml` (see `docs/maintainers/repository.md`, "Release automation").

## Live playback and the session bus

Run live audio against your own PipeWire null sink, never the default sink or
the microphone. Create it with `pw-cli create-node adapter '{
factory.name=support.null-audio-sink node.name=<name> media.class=Audio/Sink
object.linger=true audio.position=[FL FR] }'`, and target it with
`pipewiresink target-object` (`-device` in playspike; `PIPEWIRE_NODE` pins a
stream and breaks device-move tests). Record from its monitor with `pw-record
--target <name> -P '{ stream.capture.sink=true }' out.wav`: without
`stream.capture.sink`, `pw-record` falls back to the default source, which is
the microphone. Check the links with `pw-link -l`. Destroy only the nodes you
created; if WirePlumber stops linking streams, ask before restarting it.

Only one Sona may own the real session bus (the single-instance lock and the
MPRIS name). Keep real-session runs short and quit the app yourself. Every
other run uses `dbus-run-session` and private XDG directories, never the real
`~/.config/sona` or `~/.local/share/sona` (copy data into the private
directories when you need it). Worktrees running at once must not share
runtime state: private XDG dirs, and a separately named null sink. Two dev
builds on one session bus also need `SONA_DEV_INSTANCE=<name>` (dev builds
only): it suffixes the single-instance and MPRIS D-Bus names and the
dev-restore directory, and `sona status` reaches the instance the same
variable names.

## Widevine CDM

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

A new version becomes current only after a load check: a sandboxed child
that loads the CDM and nothing else (no GStreamer), bounded at a minute.
Only a CDM that fails to load or initialise is deleted (and, when another
version works, never fetched again); a check that runs out of time or
cannot start its child keeps the verified files, and the next check or
Retry loads them again without downloading (`load_check_incomplete`). A
play that comes while the first version is past its download and being
checked waits up to 20 s for it instead of failing at once. Separately,
after the first paint a short-lived child readies GStreamer's plugin
registry (`playback: audio stack ready took=…` in the log), so the first
play doesn't wait for the plugin scan, which on a fresh Mac can take over
a minute.

- There is no override: the app and tools only ever load the CDM Sona's
  own downloader installed. Where Mozilla's manifest lists no CDM (Linux on
  arm64), playback is unavailable with the code `unsupported_platform`.
- `go run ./cmd/cdmfetch` downloads, verifies and installs the CDM the way
  the app does: `-dir DIR` installs elsewhere (default Sona's
  `<data>/widevine`), `-platform goos/goarch` fetches another platform's,
  and `-version` only prints the manifest's version for the platform.
- `playspike` uses the downloaded CDM (installing it on first use);
  `-cdm-dir DIR` picks the install directory. It prints the one it used (`cdm source source=download`).

## Debugging the UI

- Development builds (`-tags dev`, as the `dev` script makes) start private; `SONA_INSPECTOR=1 dev` opens MyGo's inspector (F12 or Ctrl+Shift+I toggles it). Other builds never show it.
- `bin/sona` (or `go run ./cmd/sona`) starts the app from a checkout: `SONA_PRIVATE=1 SONA_PREVIEW=1 bin/sona` plays 30-second previews without an account. MyGo loads GTK3 and the GL libraries by soname: inside devenv `LD_LIBRARY_PATH` carries them; elsewhere install GTK 3, Pango, libepoxy and a GL driver.
- `go tool mygo vet internal/ui` finds build values kept past their frame, the usual cause of a view that updates one frame late.
- UI behaviour is tested headless with `ui.Tester` (`go test ./internal/ui/...`): it renders views without a window, drives keys and clicks, and takes image snapshots.
