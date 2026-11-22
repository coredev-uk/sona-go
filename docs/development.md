# Development reference

The recipes and reasons behind the [contributing pages](https://sona.coredev.uk/contributing/setup/): the environment, every devenv script, private mode, live playback, the Widevine CDM and debugging the UI. [AGENTS.md](../AGENTS.md) has the short rules.

## Contents

- [The environment](#the-environment)
  - [What devenv sets](#what-devenv-sets)
  - [Builds and generated code](#builds-and-generated-code)
  - [Local secrets](#local-secrets)
- [Private mode](#private-mode)
- [devenv scripts](#devenv-scripts)
  - [Script summary](#script-summary)
  - [dev and gallery](#dev-and-gallery)
  - [build](#build)
  - [check and tests](#check-and-tests)
  - [headless, e2e and orca-walk](#headless-e2e-and-orca-walk)
  - [probe](#probe)
  - [Packages](#packages)
- [Live playback and the session bus](#live-playback-and-the-session-bus)
  - [Use a null sink](#use-a-null-sink)
  - [Keep the session bus to one Sona](#keep-the-session-bus-to-one-sona)
- [Widevine CDM](#widevine-cdm)
  - [Download and updates](#download-and-updates)
  - [The load check](#the-load-check)
  - [Tools and overrides](#tools-and-overrides)
- [Debugging the UI](#debugging-the-ui)

## The environment

Everything runs through [devenv](https://devenv.sh) 2.4 or newer; `devenv.yaml` refuses an older CLI. It pins Go 1.27 and GStreamer, plus GTK3, Pango and the GL libraries MyGo loads on Linux. The short version is [Set Up Your Environment](https://sona.coredev.uk/contributing/setup/).

```sh
devenv shell                     # interactive shell
devenv shell -- <command>        # one command
# Without installing devenv. Plain `nixpkgs#devenv` follows your registry,
# which may carry an older devenv.
nix run github:NixOS/nixpkgs/nixpkgs-unstable#devenv -- shell
```

With [direnv](https://direnv.net), the checked-in `.envrc` loads the same environment whenever you `cd` into the checkout or a worktree. Run `direnv allow` once in each, and `go`, `dev`, `check` and the other scripts work without the `devenv shell --` prefix.

### What devenv sets

- `CGO_ENABLED=1` and `GOFLAGS=-tags=sqlite_fts5`.
- The GStreamer plugin path: core, base, good, bad, libav and, on Linux, PipeWire. Never the host's plugins.
- On Linux, `LD_LIBRARY_PATH` for GTK3 and friends. MyGo loads them by soname, so they must come from the same nixpkgs pin as GStreamer: one GLib per process.
- The variables in `.env`.

### Builds and generated code

- `build` writes `bin/sona`: production, with no inspector and MyGo in production mode.
- `dev` builds the `dev` tag into `bin/sona-dev` and runs it.
- A plain `go build ./cmd/sona` is not a dev build; only `-tags dev` is. Dev builds start private, show the inspector with `SONA_INSPECTOR=1` and panic on a message that doesn't parse.
- `-tags` replaces devenv's `GOFLAGS`, so a build with its own `-tags` names `sqlite_fts5` too.
- Run `sqlc generate -f internal/db/sqlc.yaml` after changing queries. Migrations live in `internal/db/migrations` (goose).
- The version is the `VERSION` file. The build injects it with `-ldflags -X main.version` (`bin/sona --version`).

### Local secrets

`.env` is ignored by git. It may hold `MUSIC_USER_TOKEN` for `cmd/playspike` and live tests. Never print, log or commit it.

It may also hold `SONA_LASTFM_KEY`, `SONA_LASTFM_SECRET` and `SONA_DISCORD_CLIENT_ID`:

- Production builds compile them in (`-X`, with `-trimpath` so the flags stay out of the binary's build info).
- Development builds read them from the environment at run time. devenv puts them there on Linux and macOS; on Windows, `cmd/devloop` hands them to the app (see [dev and gallery](#dev-and-gallery)).
- Never pass them in `-ldflags` of a build without `-trimpath`. Go records the flags in the build info; the log handler and the diagnostics bundle redact `-ldflags` values, but don't rely on that.

Values are `KEY=value` lines. An `export ` prefix, `#` comments and blank lines are allowed. A value can be bare (trimmed, ended by ` #`), `'single quoted'` (literal) or `"double quoted"` (`\n`, `\t`, `\"`, `\\`, `\$` escapes). There is no `$VAR` expansion and no value spans lines.

A new worktree has no `.env`. Under devenv, symlink the main checkout's when you need it there (`ln -s ../sona-go/.env .env`). On Windows you need not: `devloop` falls back to the main checkout's `.env`. A symlink there needs Developer Mode or an elevated shell.

## Private mode

Every test, smoke or live check that plays music runs in private mode. See also [Build and Run](https://sona.coredev.uk/contributing/build-and-run/#use-private-mode).

- A development build (any build with the `dev` tag, as the `dev` script builds it) starts with private listening on, whatever the saved setting says.
- Toggling it then lasts for the session only and never writes the setting. The toggle reads "Private (development build)".
- `SONA_PRIVATE=1` forces private mode in any build, and it cannot be turned off for that run.
- `SONA_PRIVATE=0` makes a development build follow the saved setting, for a deliberate live check.
- `cmd/playspike -activity` is private by default; `-private=false` reports the plays to the account.
- The live tests refuse to run without `SONA_PRIVATE=1`.

## devenv scripts

### Script summary

Run each as `devenv shell -- <script>`, or bare under direnv.

| Script | Does |
| --- | --- |
| `dev` | Builds and runs the app, and rebuilds it on every change. |
| `gallery` | The same loop on the component gallery. |
| `build` | Builds the production binary, `bin/sona`. |
| `docs` | Runs the documentation site with live reload: `cd site && npm ci && npm run dev`. |
| `gen` | `sqlc generate`. Commit the result. |
| `fmt` | gofumpt on every tracked Go file except `internal/db/dbq`. |
| `check` | The static checks, gofumpt formatting included. |
| `tests` | `go test -race ./...` and the tests of dev-only code. |
| `release` | `build/linux/release/release.sh`, the same as Taskfile's `release`. |
| `play-spike` | `go run ./cmd/playspike`: the DRM path regression (needs `MUSIC_USER_TOKEN`). `play-spike -preview` needs none. |
| `auth-spike` | `go run ./cmd/authspike`: the Apple sign-in flow from the terminal. |
| `headless` | Runs a command in a private headless session. |
| `e2e` | The end-to-end smoke and the accessibility check. |
| `orca-walk` | Walks the app with a real Orca. |
| `probe` | The Albums scroll probe in the app's own process. |

`docs` needs Node, which is in the dev shell only for this script. The app's build and `check` don't use it. See [Writing Docs](https://sona.coredev.uk/contributing/documentation/).

`release` builds every Linux package (.deb, .rpm, Arch, AppImage) on Ubuntu 24.04 in podman or docker. It smoke-tests each on a clean distro container (Xvfb, `MYGO_GPU=0`).

### dev and gallery

`dev` builds `cmd/devloop` into `bin/devloop` and runs it on `./cmd/sona` with your arguments after `--`. The loop:

1. Builds the app with `-tags dev,sqlite_fts5` into `bin/sona-dev` (`bin\sona-dev.exe` on Windows) and runs it.
2. Rebuilds and restarts it when a Go file of the app changes.
3. Keeps the old app running during the build. A failed build prints its errors and leaves the old app.
4. Restarts on the same screen, scroll and window.

Keys: `r` rebuilds, `q` quits. A first argument `-background` (`dev -background -- <app args>`) starts the app without taking the focus (`MYGO_DEV_BACKGROUND=1`), so a restart on a save leaves it in your editor. Design tokens and `.po` files reload in the running app without a build. See [ui-architecture.md](ui-architecture.md), "The development loop".

`gallery` is the same loop on `./cmd/uigallery`: the component gallery, with every kit component, light and dark, with knobs. It restarts like `dev`.

On Windows, where devenv doesn't run, build and start the loop yourself with the cgo environment of [platforms.md](platforms.md), "Platform support": `go build -o bin\devloop.exe .\cmd\devloop`, then `bin\devloop.exe` with the same flags.

Outside devenv (no `DEVENV_ROOT`), `devloop` also does devenv's `dotenv` job: each time it starts the app, it reads `.env` from the checkout, or in a worktree without one, from the main checkout. The app gets each variable its environment doesn't already set; a variable set in the environment, even to empty, wins. `go build` never gets them, so they can't be linked into a dev build. The loop logs the file and the names it passed, never a value. Under devenv it leaves `.env` to devenv.

### build

`build` runs:

```sh
go build -tags mygo_noinspector,sqlite_fts5 -ldflags "$(bash build/ldflags.sh --no-credentials --goos "$(go env GOOS)")" -o bin/sona ./cmd/sona
```

There is no inspector in the binary and MyGo runs in production mode. `build/ldflags.sh` is the one place the link flags are assembled:

- `-w -s` and the version;
- production mode;
- per OS, the macOS deployment target or `-H windowsgui`.

`--no-credentials` leaves the Last.fm and Discord values out, which only `-trimpath` builds may carry.

### check and tests

`check` is the static half of the checks ([testing.md](testing.md)):

- `go vet`, plus `go vet -tags dev,sqlite_fts5` and `-tags probe,sqlite_fts5`, so development and probe builds type-check too.
- The Windows and macOS type-checks.
- `golangci-lint --new-from-rev=origin/main`.
- `go tool mygo vet internal/ui`, for build values kept past their frame.
- The catalogue check (`i18ntool extract -check`) and the path-case check (`cmd/pathcase`).

`tests` runs `go test -race ./...`, including the `ui.Tester` tests. On Linux it runs inside `dbus-run-session`, so no test reaches your session bus. Extra arguments go to `go test`.

It then runs the tests of dev-only code:

```sh
go test -race -tags dev,sqlite_fts5 ./internal/devinstance ./internal/ui/devstate ./internal/ui/app ./internal/ui/features/song ./internal/ui/features/discover ./internal/app ./internal/mpris
```

The script is not named `test` because the shell's `test` builtin would shadow it.

### headless, e2e and orca-walk

`headless` is `bash e2e/headless.sh "$@"`. It runs a command (`headless -- <cmd>`) in a private headless session with:

- sway (headless backend, pixman);
- private XDG directories, which start empty: write the fixture library into them first with `headless -- sh -c 'go run ./e2e/profile && <cmd>'`;
- its own session bus, with the AT-SPI bus;
- the desktop portals stubbed, so there is no 12 s wait for a portal that never starts;
- a `wtype` keep-alive that gives the seat a keyboard.

`WAYLAND_DISPLAY` is set for the command. The session and its files are removed afterwards.

`e2e` runs `headless -- go test -tags e2e -count=1 ./e2e/...`: the end-to-end smoke and the accessibility check (see [testing.md](testing.md), "UI tests and the end-to-end smoke"). Extra arguments go to `go test`.

`orca-walk` walks the app with a real Orca in `headless`:

```sh
E2E_ORCA=1 E2E_ORCA_BIN=<orca> headless -- go test -tags e2e -count=1 -v -run '^TestOrca' ./e2e/
```

Orca is built from the pinned nixpkgs on first use; it is not in the dev shell. What it says is written to `bin/orca-out` (`E2E_OUT`); see [the accessibility rules](../site/src/content/docs/contributing/accessibility.md), "Listening with Orca". It is not part of `e2e` or `tests`: it only runs when asked.

### probe

`probe` builds the probe build (`-tags probe,sqlite_fts5`) into `bin/sona-probe` and runs `sona -probe` with your arguments: the Albums scroll probe in the app's own process (see [performance.md](performance.md), "Measure UI performance").

It needs a profile with albums whose covers are cached or reachable. The fixture library (`go run ./e2e/profile`) has covers only behind the e2e harness's local server (`SONA_E2E_CDN`), so outside the e2e tests:

- the probe waits 60 s, prints "the covers were not all drawn", measures nothing and exits 1;
- on empty private directories it fails the same way with "showed no tiles".

Run it as `headless -- probe` on a profile you copied into the private directories, or on your desktop session's own (keep that run short).

### Packages

`package` is only Taskfile's (`task package`: .deb, .rpm and Arch with nfpm, and the AppImage). It packages a build of the machine it runs on. A package built on NixOS would link against `/nix/store`, so release packages come from `release`.

The macOS and Windows release builds are `build/darwin/bundle.sh` and `build/windows/package.sh`, run by `release.yml` (see [maintainers/repository.md](maintainers/repository.md), "Release automation").

The macOS `.dmg` opens as a designed drag-to-Applications window: `build/darwin/dmg.sh` makes it with a pinned `dmgbuild` (`build/darwin/install-dmgbuild.sh`, which `bundle.sh` runs first), from the art in `build/darwin/dmg`. To change the art, edit `background.svg` and run `devenv shell -- build/darwin/dmg/render.sh` (resvg, which devenv provides); [maintainers/release.md](maintainers/release.md), "Platforms in a release", has the rest.

## Live playback and the session bus

### Use a null sink

Run live audio against your own PipeWire null sink, never the default sink or the microphone.

1. Create the sink:

   ```sh
   pw-cli create-node adapter '{ factory.name=support.null-audio-sink node.name=<name> media.class=Audio/Sink object.linger=true audio.position=[FL FR] }'
   ```

2. Target it with `pipewiresink target-object` (`-device` in playspike). `PIPEWIRE_NODE` pins a stream and breaks device-move tests.
3. Record from its monitor. Without `stream.capture.sink`, `pw-record` falls back to the default source, which is the microphone:

   ```sh
   pw-record --target <name> -P '{ stream.capture.sink=true }' out.wav
   ```

4. Check the links with `pw-link -l`.

Destroy only the nodes you created. If WirePlumber stops linking streams, ask before restarting it.

### Keep the session bus to one Sona

Only one Sona may own the real session bus: the single-instance lock and the MPRIS name. Keep real-session runs short and quit the app yourself.

Every other run uses `dbus-run-session` and private XDG directories, never the real `~/.config/sona` or `~/.local/share/sona`. Copy data into the private directories when you need it.

Worktrees running at once must not share runtime state: give each private XDG dirs and a separately named null sink.

Two dev builds on one session bus also need `SONA_DEV_INSTANCE=<name>` (dev builds only). It suffixes the single-instance and MPRIS D-Bus names and the dev-restore directory, and `sona status` reaches the instance the same variable names.

## Widevine CDM

devenv ships no CDM. The app downloads it itself, with no prompt, once the network opens after the first paint. Browser and system copies are never used.

### Download and updates

- The version and SHA-512 come from Mozilla's Firefox GMP manifest, the package from Google's servers, and its CRX3 signatures are verified (`internal/playback/cdm`; see [security-review.md](security-review.md), "Widevine CDM downloader").
- It lives in `<data>/widevine/<version>/<platform>/` behind `current.json`. The platform is `linux_x64`, or `mac_arm64` or `mac_x64` on macOS, where the `.sig` file stays beside the dylib.
- It is checked for updates daily. A library that no longer matches the SHA-256 recorded at install is removed and downloaded again. Three failed host starts roll back to the previous version.
- Settings → Playback shows its state with a Retry button.

### The load check

A new version becomes current only after a load check: a sandboxed child that loads the CDM and nothing else (no GStreamer), bounded at a minute.

- Only a CDM that fails to load or initialise is deleted. When another version works, it is never fetched again.
- A check that runs out of time or cannot start its child keeps the verified files. The next check or Retry loads them again without downloading (`load_check_incomplete`).
- A play that comes while the first version is past its download and being checked waits up to 20 s for it instead of failing at once.

Separately, after the first paint a short-lived child readies GStreamer's plugin registry (`playback: audio stack ready took=…` in the log). The first play then doesn't wait for the plugin scan, which on a fresh Mac can take over a minute.

### Tools and overrides

- There is no override: the app and tools only ever load the CDM Sona's own downloader installed. Where Mozilla's manifest lists no CDM (Linux on arm64), playback is unavailable with the code `unsupported_platform`.
- `go run ./cmd/cdmfetch` downloads, verifies and installs the CDM the way the app does.
  - `-dir DIR` installs elsewhere (default Sona's `<data>/widevine`).
  - `-platform goos/goarch` fetches another platform's.
  - `-version` only prints the manifest's version for the platform.
- `playspike` uses the downloaded CDM, installing it on first use. `-cdm-dir DIR` picks the install directory. It prints the one it used (`cdm source source=download`).

## Debugging the UI

- Development builds (`-tags dev`, as the `dev` script makes) start private. `SONA_INSPECTOR=1 dev` opens MyGo's inspector; F12 or Ctrl+Shift+I toggles it. Other builds never show it.
- **Inspect** is in every item's menu (right-click a song, album, playlist, artist, station or music video). It shows the item's IDs, artwork URLs, what Sona mapped, what Apple sent as Go's cache holds it, and what an add-on receives for it ("What an add-on gets": the `invoke` target and, for a song, the `now-playing` track, as protocol JSON), with Copy as JSON, Copy ID and Copy artwork URL. Redacted like the logs. A development build always has it. A release build has it while **Settings → Diagnostics → Developer tools** is on (off by default), so add-on developers can use it on an installed Sona (`docs/ui-architecture.md`, "Developer actions"; `docs/addons.md`, "Developing an add-on").
- `bin/sona` (or `go run ./cmd/sona`) starts the app from a checkout: `SONA_PRIVATE=1 SONA_PREVIEW=1 bin/sona` plays 30-second previews without an account.
- MyGo loads GTK3 and the GL libraries by soname. Inside devenv `LD_LIBRARY_PATH` carries them; elsewhere install GTK 3, Pango, libepoxy and a GL driver.
- `go tool mygo vet internal/ui` finds build values kept past their frame, the usual cause of a view that updates one frame late.
- UI behaviour is tested headless with `ui.Tester` (`go test ./internal/ui/...`): it renders views without a window, drives keys and clicks, and takes image snapshots.
