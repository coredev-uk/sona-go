# Contributing to Sona

Thanks for helping. Read this first, then
[docs/architecture.md](docs/architecture.md) (how Sona is put together) and
the doc for the area you touch. Everyone taking part follows the
[Code of Conduct](CODE_OF_CONDUCT.md); security problems go through
[SECURITY.md](SECURITY.md), not public issues.

Bugs and feature ideas: open an issue with the matching form. For anything
larger than a fix, open an issue before writing code, so the design can be
agreed first.

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

### Private mode

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

### devenv scripts

| Script       | What it runs                                                                                                                                                                                                                                                                                                                                |
| ------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `dev`        | Builds `cmd/devloop` into `bin/devloop` and runs it on `./cmd/sona` with your arguments after `--`: it builds the app with `-tags dev,sqlite_fts5` into `bin/sona-dev`, runs it, and rebuilds and restarts it when a Go file of the app changes (the old app keeps running during the build; a failed build prints its errors and leaves it; the restart lands on the same screen, scroll and window). Keys: `r` rebuilds, `q` quits. Design tokens and `.po` files reload in the running app without a build. See docs/ui-architecture.md, "The development loop". |
| `gallery`    | The same loop on `./cmd/uigallery`: the component gallery (every kit component, light and dark, with knobs), restarting like `dev`. |
| `build`      | `go build -tags mygo_noinspector,sqlite_fts5 -ldflags "-X main.version=$(cat VERSION) -X github.com/egoist/mygo.production=1" -o bin/sona ./cmd/sona`: no inspector in the binary, MyGo in production mode. |
| `gen`        | `sqlc generate`. Commit the result. |
| `fmt`        | gofumpt on every tracked Go file except `internal/db/dbq`. |
| `check`      | The static half of the checks below: `go vet`, `go vet -tags dev,sqlite_fts5` and `-tags probe,sqlite_fts5` (development and probe builds type-check too), the Windows and macOS type-checks, `golangci-lint --new-from-rev=origin/main`, `go tool mygo vet internal/ui` (build values kept past their frame), the catalogue check (`i18ntool extract -check`) and the path-case check (`cmd/pathcase`). |
| `tests`      | `go test -race ./...` (inside `dbus-run-session` on Linux, so no test reaches your session bus; extra arguments go to `go test`), including the `ui.Tester` tests, then `go test -race -tags dev,sqlite_fts5 ./internal/devinstance ./internal/ui/devstate ./internal/ui/app ./internal/app ./internal/mpris` (the tests of dev-only code). Not named `test`: the shell's `test` builtin would shadow it. |
| `release`    | `build/linux/release/release.sh`, the same as Taskfile's `release`: builds every Linux package (.deb, .rpm, Arch, AppImage) on Ubuntu 24.04 in podman/docker and smoke-tests each on a clean distro container (Xvfb, `MYGO_GPU=0`). |
| `play-spike` | `go run ./cmd/playspike`: the DRM path regression (needs `MUSIC_USER_TOKEN`); `play-spike -preview` needs none.                                                                                                                                                                                                                             |
| `auth-spike` | `go run ./cmd/authspike`: the Apple sign-in flow from the terminal.                                                                                                                                                                                                                                                                         |
| `headless`   | `bash e2e/headless.sh "$@"`: runs a command (`headless -- <cmd>`) in a private headless session: sway (headless backend, pixman), private XDG directories (empty: write the fixture library into them first, `headless -- sh -c 'go run ./e2e/profile && <cmd>'`), its own session bus with the AT-SPI bus, the desktop portals stubbed (no 12 s wait for a portal that never starts) and a `wtype` keep-alive that gives the seat a keyboard. `WAYLAND_DISPLAY` is set for the command; the session and its files are removed afterwards. |
| `e2e`        | `headless -- go test -tags e2e -count=1 ./e2e/...`: the end-to-end smoke and the accessibility check (see "UI tests and the end-to-end smoke"). Extra arguments go to `go test`. |
| `orca-walk`  | Walks the app with a real Orca in `headless`: `E2E_ORCA=1 E2E_ORCA_BIN=<orca> headless -- go test -tags e2e -count=1 -v -run '^TestOrca' ./e2e/`, with Orca built from the pinned nixpkgs on first use (it is not in the dev shell). What it says is written to `bin/orca-out` (`E2E_OUT`); see docs/accessibility.md, "Listening with Orca". Not part of `e2e` or `tests`: it only runs when asked. |
| `probe`      | Builds the probe build (`-tags probe,sqlite_fts5`) into `bin/sona-probe` and runs `sona -probe` with your arguments: the Albums scroll probe in the app's own process (see "Measuring UI performance"). It needs a profile with albums whose covers are cached or reachable: the fixture library (`go run ./e2e/profile`) has covers only behind the e2e harness's local server (`SONA_E2E_CDN`), so outside the e2e tests the probe waits 60 s, prints "the covers were not all drawn", measures nothing and exits 1. On empty private directories it fails the same way with "showed no tiles". Run it as `headless -- probe` on a profile you copied into the private directories, or on your desktop session's own (keep that run short). |

`package` is only Taskfile's (`task package`: .deb, .rpm and Arch with nfpm, and
the AppImage): it packages a build of the machine it runs on. A package built
on NixOS would link against `/nix/store`, so release packages come from `release`.
The macOS and Windows release builds are `build/darwin/bundle.sh` and
`build/windows/package.sh`, run by `release.yml` (see "Releases").

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

### Pinned inputs

`devenv.yaml` pins nixpkgs to one nixos-unstable revision and devenv's
modules to one revision, so every machine gets the same toolchain. To bump
nixpkgs, put the current nixos-unstable head
(`git ls-remote https://github.com/NixOS/nixpkgs refs/heads/nixos-unstable`)
in `devenv.yaml` and in `flake.nix` (the flake's package builds with the same
nixpkgs), run `devenv update` and `nix flake update`, then `check`, `tests`,
`play-spike`, `nix build` and the app, and commit `devenv.yaml`, `devenv.lock`,
`flake.nix` and `flake.lock` together.

### macOS

`devenv shell` works on Apple Silicon (`aarch64-darwin`): the Linux-only
packages (GTK3 and the GL libraries, PipeWire, playerctl, gcc, dbus) and
`LD_LIBRARY_PATH` are left out; MyGo draws through AppKit there, and GStreamer keeps core,
base, good (`osxaudiosink`), bad and libav, on the same plugin path as on
Linux. Intel Macs (`x86_64-darwin`) are not supported: the pinned nixpkgs
(26.11) and devenv 2.4 dropped them.

devenv sets one macOS deployment target on macOS: `MACOSX_DEPLOYMENT_TARGET`
= nixpkgs' (14.0), and `GOFLAGS` adds
`-ldflags=-extldflags=-mmacosx-version-min=14.0`, which overrides the 10.13
that some cgo directives pass to the linker; without it ld warns "object
file … was built for newer macOS version (14.0) than being linked (11.0)" for
every cgo object. A `go build -ldflags=…` of your own replaces GOFLAGS'
`-ldflags`; add `-extldflags=-mmacosx-version-min=$MACOSX_DEPLOYMENT_TARGET`
to it (`build/darwin/Taskfile.yml` does).

The app builds and runs on macOS from source as an unsigned `bin/sona` (no
Apple Developer account, no `.app` bundle; the release's `Sona.app` and `.dmg`
come from `build/darwin/bundle.sh`, with GStreamer bundled): its UI,
everything Go does (sign-in, library, sync, catalog) and playback through
the same CDM host as Linux, with the CDM Sona downloads from Google for
macOS (`mac_arm64`, see "Widevine CDM") and Core Audio output
(`osxaudiosink`: the device chosen in Settings → Playback → Output, else
the system's default output, followed when it changes). Media keys and Now
Playing work (`internal/nowplaying`), and so do the sleep inhibitor (MyGo's
`Power.KeepAwake`: an IOPMAssertion `pmset -g assertions` lists), the power
source for "on battery" (`Power.IsOnBattery`, polled) and the file chooser
for the settings export and import (`Dialog`). Notifications
(UNUserNotificationCenter: a title, a text and a click, no artwork or
buttons) and Start Sona when you log in (SMAppService) need an app bundle in
a folder macOS lists (`/Applications`, `~/Applications`; from `/tmp` it
refuses the notifications): the release's `Sona.app`, or a development
build wrapped by `build/darwin/devbundle.sh bin/sona-dev ~/Applications/SonaDev.app`
(run `SonaDev.app/Contents/MacOS/sona` inside the devenv shell; it has its
own identifier, and a login item you turn on is listed in System Settings →
General → Login Items & Extensions until you turn it off again). A bare
`bin/sona` has neither, and Settings says so ("Only when Sona runs from
Sona.app"). `sona <command>` and single instance work over a socket in
Sona's state folder (docs/cli.md). Apple's licence server grants licences to
this unsigned host: full tracks play on Apple Silicon.

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
go vet ./... && go test ./...
SONA_PRIVATE=1 dev                   # a development build, started
```

`build` writes `bin/sona` for macOS (Taskfile's `build` does the same through
`build/darwin/Taskfile.yml`); `SONA_PRIVATE=1 ./bin/sona` runs it. Expect the window with every page; Settings
without the controls macOS can't support from a bare binary (start at login
and notifications, which need Sona.app); the keyring backend
"Keychain"; data under `~/Library/Application Support/Sona` and
`~/Library/Caches/Sona` (log: `State/sona.log`). The D-Bus tests skip (no
`dbus-daemon`).

The CDM: Sona and `playspike` download it themselves; no browser is
needed. To check the download and the library by hand:

```sh
cdm=$(mktemp -d)
go run ./cmd/cdmfetch -dir "$cdm"                 # prints version=… path=…/mac_arm64/libwidevinecdm.dylib
ls -l "$cdm"/*/mac_arm64/                           # the dylib and its .sig, side by side
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
`MUSIC_USER_TOKEN`; always in private mode; it uses the downloaded CDM;
audio plays on the default output, or on `-device <uid>`, a Core Audio
device UID from the `output` lines playspike prints once playback starts):

```sh
SONA_PRIVATE=1 go run ./cmd/playspike -seek 90 -gap=false
SONA_PRIVATE=1 go run ./cmd/playspike -activity   # only with SONA_PRIVATE=1
```

It prints `cdm source source=download` with the library under
`~/Library/Application Support/Sona/widevine/`, then each `licence
exchange` (HTTP status, Apple's status and message, the licence's length;
never the token or the licence), and ends with `licence: granted`,
`licence: refused (<why>)` or `licence: not requested (<why>)`.

A stream station (Apple's live radio, or a local broadcaster such as KISS,
`ra.1445918457`) plays with `-station` for `-listen` (default 12s), and
each change of what its stream says is on air is printed as playback
reaches it; it fails unless the station is still playing and played at
least nine tenths of that time:

```sh
SONA_PRIVATE=1 go run ./cmd/playspike -station ra.1445918457 -listen 2m
```

To see a computer's first run again (the CDM download, its load check and
GStreamer's first plugin scan), quit Sona and remove the downloaded CDM and
GStreamer's registry, then start it:

```sh
rm -rf ~/Library/Application\ Support/Sona/widevine
rm -f "${XDG_CACHE_HOME:-$HOME/.cache}"/gstreamer-1.0/registry.*.bin   # or $GST_REGISTRY_1_0 when set
SONA_PRIVATE=1 dev
grep -E 'widevine:|audio stack' ~/Library/Application\ Support/Sona/State/sona.log
```

The log shows `widevine: downloading`, `package verified`, `load check
passed` with its `took` (the CDM alone), `installed`, and separately
`playback: audio stack ready` with the plugin scan's `took`.

## Checks

Run before sending a change (the devenv `check` and `tests` scripts run all
of it; CI, `.github/workflows/ci.yml`, runs the same on every push and pull
request except `golangci-lint`, `go tool mygo vet internal/ui` and the catalogue check, so run those yourself):

```sh
go vet ./...
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go vet ./...   # see "Platforms"
go run ./cmd/typecheck -os darwin -arch arm64 ./...
go test -race ./...    # CI's pull requests skip -race for internal/ui's page and app packages (below)
golangci-lint run --new-from-rev=origin/main ./...   # must be clean
go tool mygo vet internal/ui   # build values kept past their frame
go run ./cmd/i18ntool extract -check   # the catalogue; go test checks it too
go run ./cmd/pathcase                  # no paths differing only by case
```

Real-CDM tests (`internal/playback/widevine`, `internal/playback/cdmhost`,
`internal/diag`, `internal/playback/cdm`) find the CDM through
`internal/playback/cdm/cdmtest`: the CDM installed in `SONA_CDM_CACHE` (default
`<user cache dir>/sona-tests/widevine`, shared by every checkout),
downloaded and verified there on first use. They skip only when the
download cannot reach the network; `SONA_CDM_REQUIRE=1` turns that skip into
a failure. CI ("Resolve the Widevine CDM version") runs
`go run ./cmd/cdmfetch -version`, restores `~/.cache/sona-ci/widevine` with
`actions/cache` keyed `widevine-<os>-<arch>-<version>`, and ("Install the
Widevine CDM") fills a miss with
`go run ./cmd/cdmfetch -dir ~/.cache/sona-ci/widevine`, exporting
`SONA_CDM_CACHE` and `SONA_CDM_REQUIRE=1`. The playback shard then runs them like any test, and `skips.sh` fails the job
if one of them skipped (the CDM tests are in no allow-list).

### Preview playback (tests without a Music User Token)

`MUSIC_USER_TOKEN` is not a test credential: Apple refreshes it now and then, so a stored copy (`.env`, a repository secret) goes stale. Tests, CI and smoke runs that need audio play Apple's 30-second song previews instead, which need only the developer token Sona reads from Apple's web player: no `.env`, no Widevine CDM, no licence. (Apple's web player does the same when signed out: MusicKit JS `instance.previewOnly = true`, i.e. `playbackMode` `PREVIEW_ONLY`.)

- The app: `SONA_PREVIEW=1` (any build) plays every song as its preview, through a playback host that loads no CDM (still sandboxed, still without network). The player bar shows "Preview" beside the title. Songs are looked up in the account's storefront, or the locale's when signed out. Library uploads and radio have no preview and are skipped. Nothing reports a preview as a play: no Apple play activity, history or scrobble.
- `go run ./cmd/playspike -preview [-storefront gb] [-device <sink>]` plays a song's preview, watches it advance, plays it out and starts the next one, with no `.env` and no CDM (none is downloaded). It ends with `licence: not requested` and fails if any webPlayback, certificate or licence request went out.
- `TestPreviewPlaysLive` (`internal/playback/native`) plays a real preview end to end into a fakesink. It skips only when Apple can't be reached; `SONA_PREVIEW_REQUIRE=1` (CI) turns that skip into a failure.
- The full DRM path keeps its own deliberate check with a Music User Token: `go run ./cmd/playspike -seek 90` and the live workflow.

After touching playback or Apple request code, also run the DRM path
regression `go run ./cmd/playspike -seek 90` (needs `MUSIC_USER_TOKEN`).
Every playback run in tests, smokes and live checks uses private mode
(`SONA_PRIVATE=1`; development builds start private and `playspike` is
private by default), so nothing is reported to Apple or scrobbled.
Route long listening tests to your own PipeWire null sink, not your
speakers, and remove only the sinks you created:

```sh
pw-cli create-node adapter '{ factory.name=support.null-audio-sink node.name=sona-test media.class=Audio/Sink object.linger=true audio.position=[FL FR] }'
SONA_PRIVATE=1 go run ./cmd/playspike -device sona-test -seek 90
pw-cli destroy sona-test
```

When running the app for a check that doesn't need your desktop, start it
under `dbus-run-session` with private `XDG_CONFIG_HOME`, `XDG_DATA_HOME`,
`XDG_STATE_HOME` and `XDG_CACHE_HOME`, so it neither clashes with a running
Sona (single instance, MPRIS name) nor touches your real settings and library. Set
the XDG variables outside `dbus-run-session` (`env XDG_…=… dbus-run-session
-- bin/sona`), so services the private bus starts (the Secret Service, the
portals) use them too; started the other way round, GNOME Keyring opens your
real, locked keyring and Sona waits on its unlock prompt (the window shows
"Signing in…", and the run reads your real keyring).

The CDM host sandbox tests (`TestChildCannotOpenInetSockets`,
`TestSandboxConfinesChild/namespaces`) need unprivileged user namespaces in
which the child may change its mounts. Where the system refuses them (EPERM
or EACCES: Flatpak, user namespaces disabled, or Ubuntu 24.04's AppArmor
restriction `kernel.apparmor_restrict_unprivileged_userns=1`) they skip and
say why; any other failure fails them. To run them on Ubuntu, lift the
restriction for the session with
`sudo sysctl -w kernel.apparmor_restrict_unprivileged_userns=0`. CI does, and
sets `SONA_CDMHOST_REQUIRE_NAMESPACES=1`, which turns a skip into a failure.

Packaging changes: `bash build/linux/release/release.sh` (build every package
on Ubuntu 24.04 and smoke-test it on clean Ubuntu, Fedora and Arch
containers, plus the AppImage on Fedora) is the packaging check; it needs
podman or docker and takes a while, so CI runs it. `task package` packages
your local build for a look at the files (a devenv build links `/nix/store`
and is not a release). Before a release, work through
`docs/release-checklist.md`.

Flatpak changes (`build/linux/flatpak`, `go.mod`): `bash build/linux/flatpak/build.sh`
builds the Flatpak offline from the working tree and installs it `--user`, and
`devenv shell -- bash build/linux/flatpak/smoke.sh` runs it in the headless
session. Both need flatpak-builder with the GNOME 51 runtime, SDK and Go
extension from Flathub (`docs/flatpak.md`, "Build and test"). After a `go.mod`
change, regenerate `build/linux/flatpak/go-sources.json` with
`go run build/linux/flatpak/gosources.go > build/linux/flatpak/go-sources.json`;
the dispatch-only `Flatpak` workflow builds it in CI.

### UI tests and the end-to-end smoke

- **`ui.Tester` tests** (`go test ./internal/ui/...`, no display) run over `internal/ui/uitest`: `uitest.Env(t)` is a `module.Env` over real services on a temporary SQLite library seeded with a synthetic fixture (`internal/ui/uitest/fixture`: invented artists, albums, songs and playlists, generated covers; never Apple's data), a fake playback engine, a fake artwork CDN, a Poster the test drains and a fake clock. The Core is hermetic: an empty in-memory keyring, an HTTP transport that fails any host the test does not serve (`Harness.Network.Serve` adds one), and home, XDG and D-Bus environment under the test's temporary directory, so a test never sees your Last.fm session, GitHub or your session bus. `docs/ui-architecture.md` ("Testing with `ui.Tester`", "Tests") has the API.
- **Golden images** are few: the shell with Albums, light and dark, at 1132×1253 (`internal/ui/app/testdata/golden/`). `uitest.Golden` compares by the mean colour of 8×8-pixel blocks (another machine's text antialiasing passes; a moved edge, a changed colour or a missing cover does not). When a change moves the picture on purpose, review it and run `go test ./internal/ui/app -run Golden -update`; commit the new PNGs and say in the PR why. A failing run keeps the picture and a diff in the test's artifact directory: `$SONA_TEST_ARTIFACTS/<test name>` when that variable is set (CI's `.github/ci/test.sh` sets it and uploads the directory), else the test's `t.ArtifactDir()` (`go test -artifacts -outputdir DIR`). Or ask CI: a maintainer adds the `update-goldens` label to the pull request and the bot commits the new images (`golden-update.yml`). On a Mac (or any machine whose text differs from CI's), update goldens in the CI image: `podman run --rm -v "$PWD":/src -w /src -e MYGO_GPU=0 ghcr.io/coredev-uk/sona-ci:$(build/ci/tag.sh) go test ./internal/ui/... -update`. The image is amd64 and arm64: an arm64 machine (Apple silicon, through Podman's VM) may draw text a hair differently from CI's amd64 runners, and CI is the reference, so push and check the run. Tests get the directory from `internal/ui/uitest/artifacts` and only when they fail, because `go test` never caches a run that uses `-artifacts`.
- **The end-to-end smoke** (`e2e/`, build tag `e2e`) drives the built app from outside: `e2e` script, or `headless -- go test -tags e2e ./e2e/...`. It builds `bin/sona-e2e` with `-tags probe,sqlite_fts5` (MyGo's automation plugin and a seam that exists only in probe builds: `SONA_E2E=1` swaps in the fake playback engine and `SONA_E2E_CDN=host:port` sends all the app's HTTP to a local server of the fixture's covers, so nothing reaches Apple), starts it on the fixture profile (`go run ./e2e/profile` writes the same profile into private XDG directories), and checks first paint, Albums, playing an album, Space, a capture and, from the AT-SPI bus, the shell's landmarks by role and name. `E2E_OUT=<dir>` keeps the capture (`smoke.png`) and the app's stderr; `E2E_BIN=<path>` runs an existing binary. `headless` needs `sway`, `wtype` and `at-spi2-core` (devenv has them; elsewhere install them with your package manager). Never run a probe build outside tests: it can be driven by anything that opens its socket.
- **The Orca walk** (`e2e/orca_test.go`, `e2e/orca_walk_test.go`; `orca-walk` script) runs a real Orca beside the app in the same headless session and presses keys with `wtype`, as a keyboard does: Tab and the arrows through the sidebar, the Albums page, an album, the queue, Now Playing, the Seek slider, the account dialog and the first-run setup. Orca writes every utterance to its debug file (`--debug-file`) whether or not a speech server is there, so the run needs no audio and no speech-dispatcher; a private `orca-customizations.py` makes that file line-buffered, which Orca does not (8 KB blocks). The test checks what it said against `docs/accessibility.md`; `E2E_OUT` keeps `orca-walk.md` and `orca-setup.md` (what each key was answered with, and where Orca put the focus) and Orca's whole log. It needs `E2E_ORCA=1` (otherwise it skips) and Orca (`E2E_ORCA_BIN`, else `orca` on the PATH). CI runs it only when dispatched (`orca` or `all`), never on a pull request, and it is not a required check: its words and timing are Orca's, so a new Orca can move them.

### CI and workflows

`.github/workflows/`: `ci.yml` (every push to main, every pull request and a
nightly schedule: "Vet and type-check" (vet for the host, development and
probe builds, and the macOS and Windows type-checks); the Linux tests, in
pools that `.github/ci/shard.sh` packs by measured time (`timings.txt`):
core, playback, and the native UI's, split into `ui-race` (`ui/store`,
`ui/kit`, `ui/lyricsync`, which start goroutines) and `ui` (the page and app
packages). A pull request runs `-race` on core, playback and `ui-race` and
none on `ui`; main and the nightly run every pool under `-race` (the nightly
with `-count=1`, so the test cache can't hide what changed outside the code)
and `ui` without it too, which keeps the caches pull requests restore warm.
The required checks keep their names: "Go vet and test (-race)" gathers vet,
core and playback, "UI tests (goldens)" the UI shards; plus the end-to-end
smoke in headless sway, "E2E (headless sway)". macOS: a pull request runs one
job, "macOS build and test (pull request)" (production build, go vet, core and
playback tests with the real CDM and Seatbelt); main, the nightly and a
dispatch run the build, vet and every test shard on their own jobs, and
"Windows build" (windows-2025: `go vet` with cgo, `sona.exe`)),
`images.yml` (also builds the CI image, below),
`golden-update.yml` (the `update-goldens` label on a pull request of this
repository: regenerates the golden images of the failing golden tests in the CI
image, commits them to the branch as bot-cecil and removes the label; the
branch's code runs without secrets, the commit job runs none of it; needs the
`golden-update` environment's App secrets),
`cache-prune.yml` (after each CI run on main: keeps the newest Go cache of
each kind and deletes the rest, so the 10 GB cap never evicts what matters; a
`workflow_run` job that checks out and runs nothing from the repository),
`pr-title.yml` (Conventional Commits title),
`live.yml` (weekly Apple Music contract tests, private mode),
`release-please.yml` (the release PR and tag, as bot-cecil), `release.yml`
(builds, smoke-tests, attests and publishes the Linux, macOS and Windows
releases; nightly, the rc of main as a prerelease; dispatch = dry run, a
prerelease with `-f prerelease=rc.1`, or the nightly now with `-f nightly=true`),
`release-secrets.yml` (dispatch: checks the `release`
environment's secrets are set, publishes nothing), `renovate.yaml`
(dependency updates; maintained with Renovate).

**The CI image.** The Linux vet, type-check, core, UI and end-to-end jobs run
in `ghcr.io/coredev-uk/sona-ci` (`build/ci/Containerfile`: Ubuntu 26.04, the
GTK, GStreamer, sway, AT-SPI, PipeWire and font packages, Go and gotestsum at
`go.mod`'s versions; amd64 and arm64), so they start with the toolchain ready
and the system libraries, which the UI goldens depend on, change only when the
image is rebuilt. `images.yml` builds it when `build/ci/**` or `go.mod`
changes (on a pull request of this repository too: the CI job named "CI image"
waits for it), weekly and on dispatch; `build/ci/tag.sh` names the tag from the
Containerfile and the Go and gotestsum versions. A run pins the digest it finds
and keys Go's caches by it. Each kind of job (vet, e2e, one shard of a test pool with or without `-race`) has its own Go cache, saved only by a run on main, and `cache-prune.yml` keeps the newest of each: 7.5 GB of the repository's 10 GB in 34 entries after a main run (measured 2026-11-05: the macOS `-race` caches 1.7 GB, the UI shards' 1.8 GB, macOS's GStreamer installer 1.0 GB, the rest under 0.6 GB each); the pull-request jobs restore them and save nothing. The playback job stays on the bare runner (it
lifts the user-namespace restriction of the host). To rebuild it by hand:
`podman build $(build/ci/tag.sh args) -f build/ci/Containerfile build/ci`.

Rules for workflow changes: trigger pull requests with `pull_request`, never
`pull_request_target` or `workflow_run` (`cache-prune.yml` is the one
`workflow_run`: it reacts to pushes to main only and runs no repository code); start from `permissions: contents:
read` and widen per job only where needed; pin every action to a full commit
SHA with the version in a comment (`uses: actions/checkout@<sha> # v7.0.1`;
Renovate keeps pins current); `persist-credentials: false` on every checkout
that doesn't push; pass expressions to scripts through `env:`, never inline
in `run:`; secrets live in environments (`release`: Last.fm key/secret,
Discord ID, deploys from main and `v*` tags; `live`: `MUSIC_USER_TOKEN`, main
only), so no pull request run can read them. Run `actionlint` on changed
workflows. The test jobs run `.github/ci/test.sh` (gotestsum: a failing test
is rerun twice; one that passes on a rerun passes the job and is listed in the
job summary as flaky) and then `.github/ci/skips.sh`, which fails the job on
any skipped test that `.github/ci/skips-linux.txt` (or `skips-darwin.txt`)
does not name: a skip means a missing prerequisite or a test nobody runs, so a
test that cannot run on a platform is added to the list with its reason, and
anything else is fixed in the job's setup. Go's caches are restored by the key
they were saved under (the CI image's digest, or the runner image's version on
the bare runner, is part of the Linux key, so an image update never serves a
cached pass), and `test.sh` leaves out
`-artifacts` so unchanged packages report `(cached)`. Threat model: `docs/security-review.md`, "Workflows".

### Desktop VM tests

`tests/vm` holds NixOS VM tests that run Sona in real GNOME and KDE Plasma sessions (headless QEMU, no network, private mode) and check what the release checklist's desktop rows ask: first paint, the title bar's buttons, launcher entry, single instance, MPRIS in the shell, notifications, tray, X11 (XWayland), HiDPI scaling and `sona private status`. They need Linux with KVM and Nix (no devenv):

```sh
nix-build tests/vm -A gnome     # or -A plasma; -A all runs every test
nix-build tests/vm -A sona      # the Nix package the VMs install
```

Screenshots land in `result/`. `tests/vm/README.md` lists each check, the checklist rows it covers, the interactive driver, and how to update the package's hashes after a lockfile change. They never run on pull requests: a CI run takes about an hour of runner time, so start the "VM tests" workflow by hand (`workflow_dispatch`) before a major release, or run them locally.

### Measuring UI performance

MyGo draws every frame itself, so frame cost is measured in the process, not in a browser. `MYGO_FRAME_STATS=all` makes MyGo log one line per presented frame (`mygo: frame N took … ms: build …, layout …, paint …, present …`); `MYGO_FRAME_STATS=8` only the frames slower than 8 ms, which a scroll through Albums or Home should not log on a desktop GPU. `SONA_INSPECTOR=1` opens MyGo's inspector beside the content (element boxes, styles, frame times). `startup: first paint` in `<state>/sona.log` is the time from process start to the first presented frame; compare several starts, not one.

**The probe build.** `go build -tags probe,sqlite_fts5 -o bin/sona-probe ./cmd/sona` links MyGo's automation plugin (`cmd/sona/automation_probe.go`; never a release build, and never with `mygo_noinspector`, which drops it). It listens only when `MYGO_AUTOMATION=<socket path>` is set, and drives the app through its accessibility tree and input events, as a touchpad or a screen reader would. Two tools use it and print the same table:

- `bin/sona-probe -probe [-speed 2000] [-max 20000] [-runs 3] [-idle 10s] [-interval ms]` measures the Albums scroll in the app's own process and quits; `-startup` quits once the covers are on screen. The probe starts the app again with `MYGO_FRAME_STATS=all` and a private automation socket (MyGo reads both while its packages initialise), takes the frame lines off the log, and scrolls through wheel events over that socket.
- `go run ./cmd/uiprobe -socket $MYGO_AUTOMATION -log $XDG_STATE_HOME/sona/sona.log` does the same from outside against a probe build you started with `MYGO_AUTOMATION=… MYGO_FRAME_STATS=all` (the frame lines are then in `sona.log`; the pid and the process start come from the log's `startup: process start` line). `uiprobe -firstpaint run1/sona.log run2/sona.log …` summarises `startup: first paint` over several starts (min, median, max). Home, New and the motion artwork scenarios of v0.2.0's uiprobe come back once those pages exist.

What the scroll does (`internal/ui/probe/measure`): wait until the tiles in view show pictures (the probe samples the window's capture at the top left of each tile (a cell taller than wide inside the page's scroll container): a decoded cover has far more colours than the placeholder; that is the start-up line's "covers on screen"), wait until no frame has been drawn for 500 ms and the process tree is idle, take the display's frame interval (`-interval`, else the refresh rate of the output the window is on, asked of sway over `SWAYSOCK`, else measured from idle frames: the pointer moving on and off a sidebar link draws one small frame per refresh; never from frames that redraw the grid, which would stretch it and hide dropped frames; a value more than 10% from 16.67 ms is warned about; wlroots' headless output reports a refresh of 0 until the sway config sets a mode with one, `output HEADLESS-1 mode --custom 2560x1440@60Hz`, so without it the probe measures), scroll down and back up at `-speed` DIPs per second over `-max` DIPs (the page's end if nearer), `-runs` times, then watch the process at rest for `-idle`. A frame exists only when something changed, so a gap of more than 1.5 intervals between two frames counts the intervals it spans as dropped frames, as v0.2.0's probe did for `requestAnimationFrame`. The table has the gap p50 and p95, dropped frames, frames over 50 ms, the CPU of the process and its children while scrolling and at rest, MyGo's CPU time per frame (build, layout and paint) and present time, and RSS and PSS at rest. The first row is a control: the same wheel stream over the top bar, where nothing scrolls, which is what the input path costs. Per-pass lines go to stderr; the median of the passes is the table. The probe always ends: if the covers are not drawn within 60 s (offline, an empty artwork cache) it prints what it has and fails with status 1 without scrolling; an unanswered automation request times out after a minute; the whole probe has an overall limit; and an app that does not shut down within 15 s of the probe is stopped with status 1. `uiprobe` fails the same way.

Run it headless, as every automated run (private XDG dirs, `dbus-run-session`, `SONA_PRIVATE=1`; the `headless` script wraps the compositor), on a sway headless output with the window floated at the size the baseline used, `swaymsg '[app_id="dev.coredev.Sona"] floating enable, resize set 1132 1253, move position 0 0'` (or a `for_window` rule in the sway config). A library of the size you compare against matters: the scroll covers `min(-max, the page)` DIPs, and the frames per pass follow from it.

- Compare before and after on the same machine, with the same library (about 650 albums makes a useful grid), the same window size and the same `-speed`, and confirm on a desktop session: the headless session draws on the CPU (llvmpipe, `frames: drawn in memory` in the output), so its CPU figures are not a desktop GPU's and its frame times only show relative change. `MYGO_FRAME_STATS=all` itself costs a log line and a few counters per frame.
- `-probe` counts its own wheel driver in the CPU figures; `uiprobe` does not (its driver is another process).

### Measuring memory

Memory is measured per process, never as one figure: the app and the playback host child (the Widevine CDM, GStreamer) are two rows in Activity Monitor and in `top`, and "400 MB" means different fixes depending on which holds it. Settings → Diagnostics → Copy diagnostics has a **Memory** section for exactly this (`internal/services/system_memory.go`, `internal/procmem`): for the app and the running host, the resident size and the footprint the platform's own tools show (macOS: `proc_pid_rusage`'s physical footprint, Activity Monitor's Memory column; Linux: the proportional set size), and the Go runtime's account of the app (heap in use, idle and returned to the OS). A report from a Mac that pastes this section tells which process holds the memory, and whether it is Go's heap (Sona can shrink it) or native memory (the GPU's buffers, GStreamer, the CDM).

`cmd/memprobe` measures the same on Linux, with a scripted run against a probe build. The probe build writes the Go heap profile and the runtime's figures into `SONA_MEMPROFILE_DIR` on `SIGUSR1` (`internal/memprobe`; the host child, which cannot write files, logs its figures into `sona.log`), and memprobe signals the whole process tree, reads each process's `/proc/<pid>/smaps` (RSS, PSS, anonymous memory, the mappings that hold the most) and prints one JSON line per process:

```
SONA_MEMPROFILE_DIR=$PWD/prof MYGO_AUTOMATION=$PWD/a.sock SONA_PREVIEW=1 SONA_PRIVATE=1 bin/sona-probe &
go run ./cmd/memprobe -pid $! -socket $PWD/a.sock -dir $PWD/prof -o $PWD/keep \
    link:Discover wait:20s press:"Play Perfecth" wait:15s press:"Open now playing" wait:15s measure:now-playing
go tool pprof -sample_index=inuse_space -top bin/sona-probe keep/now-playing/<pid>.heap.pb.gz
```

Steps are `wait:<duration>`, `link:`/`button:`/`toggle:`/`tab:`/`press:<label>` (through the accessibility tree), `wheel:<dy>`, `scroll:<n>x<dy>`, `measure:<name>`, `dump` (the tree's labels) and `shot:<file>`. Run it as every automated run: `headless -- …`, private XDG dirs, `SONA_PRIVATE=1`, audio to your own PipeWire null sink (`PIPEWIRE_REMOTE`/`PULSE_SERVER` as absolute paths to the real sockets reach it from the sandboxed child: a symlink in the private runtime directory does not), a copy of a real library for the Albums scrolls. Three things decide whether two runs compare:

- **The GL driver.** libglvnd picks NVIDIA's EGL on some runs and Mesa's on others (NVIDIA's mapped 60 to 70 MB more); pin Mesa's with `__EGL_VENDOR_LIBRARY_FILENAMES=…/50_mesa.json GALLIUM_DRIVER=llvmpipe MYGO_GPU=1` (the headless session draws on llvmpipe, whose textures are ordinary process memory, a stand-in for a unified-memory GPU), and run each scenario three times: PSS moves by ±15 MB from run to run with what Mesa touched.
- **The scale.** The headless output is 1x; `swaymsg output HEADLESS-1 scale 2` before starting the app gives the 2x of a Retina Mac, where a cover is four times the bytes.
- **Linux is not macOS.** Mesa's libraries (50 MB), GTK and the GL texture cache are Linux-only costs; on macOS MyGo draws with Metal, drops every image texture once the window has been idle for 2 s and keeps up to three window-sized frames while it animates. What carries over to macOS is Go's heap, the artwork and bitmap caches, the GStreamer pipelines and the CDM host; the rest needs a Mac, and the Diagnostics section above is how to get its numbers.

### Debugging the UI

- Development builds (`-tags dev`, as the `dev` script makes) start private; `SONA_INSPECTOR=1 dev` opens MyGo's inspector (F12 or Ctrl+Shift+I toggles it). Other builds never show it.
- `bin/sona` (or `go run ./cmd/sona`) starts the app from a checkout: `SONA_PRIVATE=1 SONA_PREVIEW=1 bin/sona` plays 30-second previews without an account. MyGo loads GTK3 and the GL libraries by soname: inside devenv `LD_LIBRARY_PATH` carries them; elsewhere install GTK 3, Pango, libepoxy and a GL driver.
- `go tool mygo vet internal/ui` finds build values kept past their frame, the usual cause of a view that updates one frame late.
- UI behaviour is tested headless with `ui.Tester` (`go test ./internal/ui/...`): it renders views without a window, drives keys and clicks, and takes image snapshots.

### Secret scanning

CI runs [gitleaks](https://github.com/gitleaks/gitleaks) on every pull request and every push to `main` (`.github/workflows/secret-scan.yml`): over every commit of every branch and tag, your pull request's own commits included, and over the pull request's title and body. `.gitleaks.toml` adds Sona's rules to gitleaks' defaults: JWTs (the Apple developer token), Music User Tokens, the Last.fm key and secret, the Discord application ID, and credential files (`.env`, `.p8`, `.pem`, keys, keystores). The public log shows only each finding's rule, commit, file and line. Run it before pushing with `nix run nixpkgs#gitleaks -- git --config .gitleaks.toml --redact .` (CI pins gitleaks 8.30.1). Test fixtures that need token-shaped input use obviously fake values; an allowlist entry in `.gitleaks.toml` names the file and the exact fake value, with a comment saying why it is fake. A commit that ever held a real secret stays public through its pull request even after a squash merge or force-push: rotate the secret first, then tell the maintainer. Details: `docs/security-review.md`, "Secret scanning".

## Branches and pull requests

- One change per branch and pull request. Parallel work is easiest in
  separate worktrees (`git worktree add ../sona-go-<slug> -b <type>/<slug>`);
  give each concurrently running worktree private XDG dirs.
- Commit messages and PR titles follow
  [Conventional Commits](https://www.conventionalcommits.org): `feat`, `fix`,
  `perf`, `refactor`, `docs`, `test`, `ci`, `build`, `chore` or `revert`,
  with an optional scope (`fix(playback): …`). CI fails a PR with a
  non-conforming title; PRs are squash-merged under that title.
- Never commit or push on `main`: `devenv shell` sets `core.hooksPath` to
  `.githooks/`, whose hooks refuse both. The maintainer merges pull requests
  through the project's review bot once CI passes; don't merge them yourself.
- `main` is protected by a ruleset: changes land only through a pull request,
  squash-merged (linear history), with these checks green: "Go vet and test
  (-race)", "Conventional
  Commits title" and "gitleaks". Nobody bypasses it but the review bot, and
  only through a pull request. Force pushes and deleting `main` are
  blocked. Release tags (`v*`) can't be moved or deleted. A first pull
  request from someone outside the project waits for the maintainer to
  approve its CI run. Report vulnerabilities privately (SECURITY.md).
- Fill in the PR template: what changed, why, evidence (commands and their
  output; screenshots in light and dark themes for UI changes, without
  personal account data), tests, docs, and anything left.
- Update the docs describing what you change in the same PR.
- Generated files are never edited or merged by hand. After rebasing onto
  `main`, regenerate the catalogue (`go run ./cmd/i18ntool extract`) and
  `internal/db/dbq` (`gen`), then commit them.

## Releases

Releases are automated with release-please
(`.github/workflows/release-please.yml`, config in
`release-please-config.json`). Your PR title is what it reads: PRs are
squash-merged, so the title becomes the commit subject. `feat` titles land
under Features, `fix` under Bug Fixes, `perf` under Performance and `revert`
under Reverts in `CHANGELOG.md`; `refactor`, `docs`, `test`, `ci`, `build`
and `chore` stay out of the changelog. Mark a breaking change with `!`
(`feat!: …`) or a `BREAKING CHANGE:` footer. release-please keeps one
release PR open that bumps `VERSION` and `CHANGELOG.md`; a maintainer merges
it, which tags `v<VERSION>` and runs `release.yml`: the Linux packages
(.deb, .rpm, Arch, AppImage), the macOS app for Apple silicon (`.dmg`,
GStreamer bundled, ad-hoc signed; `build/darwin`) and the Windows x64
installer and portable zip (`build/windows`), each smoke-tested, then the
signed update manifests, one `SHA256SUMS` and build attestations, published
with the release. release-please and the publish step act
as the bot-cecil GitHub App, so its release PR gets the same checks as yours.
Never edit `VERSION` or `CHANGELOG.md` by hand. Changing the packaging or
`release.yml`? Dispatch `release.yml` on your branch (`gh workflow run
release.yml --ref <branch>`): it builds and smoke-tests every platform and
publishes nothing (`-f prerelease=rc.1` publishes a test prerelease instead). Steps, dry run and the App's permissions:
`docs/release-checklist.md`, "Releasing".

### Self-updates (macOS and Windows)

`internal/update` and `UpdatesService` update a macOS `.app` or a Windows
folder in place (design: `docs/architecture.md`, "Updates"). What a release
needs:

- The public key is `internal/update/update.pub` (embedded; empty means
  updates are off). The private key is the `SONA_UPDATE_KEY` secret of the
  `release` environment, from `go tool mygo keygen`; losing it strands the
  installs that exist. Never print it or commit it.
- Release builds link MyGo's updater with `-ldflags "-X
  github.com/egoist/mygo.packageUpdateFeed=https://github.com/coredev-uk/sona-go/releases/latest/download/update-<target>.json
  -X github.com/egoist/mygo.packageUpdateKey=$(cat internal/update/update.pub)"`.
  Other builds leave both unset.
- The update archive is a gzip tar (`sona-<version>-darwin-arm64.tar.gz` with
  one top-level `Sona.app`; `sona-<version>-windows-amd64.tar.gz` with the
  folder's files at top level). `go run ./cmd/releasesign -target T -archive
  FILE -version V -url URL [-notes FILE] -out update-T.json` signs it
  (key from `$SONA_UPDATE_KEY`); the publish job signs last, after the
  archives are uploaded.
- Tests run against an `httptest` fake release server (`internal/update`,
  `internal/services/updates_self_test.go`): the check, a bad signature
  refused, no key (off), a development build never checking, and the rollback
  decision. The real run (install, relaunch, rollback) is the phase 5 update
  and rollback runs on a Mac and a Windows machine.

## Dependency updates

Renovate opens pull requests for Go modules and GitHub
Actions (MyGo's are always left for review), and a review bot checks and merges them; the pinned devenv inputs
are bumped by hand (see "Pinned inputs"). You
don't need to update dependencies yourself; a change that needs a newer
dependency bumps it in the same PR.

## Module contract

A feature is a Go service plus a UI package, each self-contained:

- Go: `internal/services/<feature>.go` (the service; the UI calls its
  exported methods) + `<feature>_dto.go` (its DTOs and events) + one line in
  `internal/services/registry.go`. Features never call each other's
  services. Full contract: `internal/services/doc.go`.
- UI: `internal/ui/features/<name>` exporting the module (routes, sidebar
  entries, shortcuts, event handlers) + one line in
  `internal/ui/app/modules.go`. Full contract: `docs/ui-architecture.md`.
- Settings: one nested struct in `internal/config/settings.go` with its
  defaults and `normalized()`; apply changes live through
  `Settings.OnChange`.

## Platforms

Linux is the full platform. macOS and Windows build and run the UI, with
every OS-specific part behind a seam that reports itself unsupported there
until it is implemented; macOS development builds also play through the CDM
host. Windows has no playback yet.

- In `cmd/sona`, `main.go` is portable; `platform_linux.go` and `platform_other.go` build
  what differs per OS: the `sona <command>` transport and single instance
  (`internal/app`: D-Bus on Linux, a socket in the state folder on macOS
  and Windows; docs/cli.md), the app options, and `services.Platform`:
  media controls (MPRIS on Linux; MyGo's NowPlaying on macOS and Windows through
  `internal/nowplaying`, `nowplaying_desktop.go`, never linked on Linux), notifications, the sleep inhibitor, the
  on-battery monitor, the autostart entry, the file chooser, and the
  title bar facts; the tray icon (and its template on macOS). The tray
  itself is the UI's (`internal/ui/app`, `mygo.NewTray`), reported through
  `services.Windows.HasTray`. The
  playback engine and the CDM host child hook (`newEngine`, `childMain`,
  `cdmLoadCheck`) are `engine_cdm.go`'s on Linux and macOS (the CDM host
  engine, loading the downloaded CDM) and `engine_other.go`'s elsewhere
  (`playback.Unsupported`, and no download). A nil seam is
  unsupported; the service using it does nothing or returns an error
  wrapping `errors.ErrUnsupported` (error code `unsupported`), and
  `PlatformService.Capabilities` tells the UI, which hides or
  explains the control (`internal/services/doc.go`,
  `docs/ui-architecture.md`).
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
  GStreamer and the C++ shim, which a macOS build has anyway. Inside them, what differs is in `_linux.go`/`_darwin.go`
  files: the sink and the output device (`gst/output_<os>.go`; PipeWire's
  device provider in `devices_linux.c`, Core Audio's devices and the
  default-output watch in `devices_darwin.c`; PipeWire's relink detection in
  `relink_linux.c`), the sandbox (`cdmhost/sandbox_linux.go`,
  `sandbox_fs_linux.go`; `sandbox_darwin.go` and the portable
  `seatbelt.go`). MyGo itself is pure Go on every OS (it loads GTK by soname on Linux), so the UI packages need no tags. Portable Go implementations
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
  uses `go vet`. macOS uses `go run ./cmd/typecheck -os darwin -arch
arm64 ./...`, which type-checks Sona's packages from source and only names
  any third-party package that needs cgo. Real builds need cgo for
  `mattn/go-sqlite3` everywhere; MyGo is pure Go on macOS and Windows.
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
  type-checks every Sona package, `main` included; third-party Objective-C
  would still fail without Apple's SDK.

- A real Windows build cross-compiles from Linux with zig as the C
  compiler (`nix shell nixpkgs#zig`):
  `GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC='zig cc -target
x86_64-windows-gnu' go build -tags mygo_noinspector,sqlite_fts5 -ldflags '-H
windowsgui -X github.com/egoist/mygo.production=1' ./cmd/sona`. zig ignores the `-mwindows` Go
  passes for `-H windowsgui`, so wrap it to pass
  `-Wl,--subsystem,windows` instead, or the exe opens a console.

### Title bar and window controls

`internal/windowchrome` decides who draws the main window's title bar
(`docs/design.md`, "Title bar"), and the shell (`internal/ui/app/shell.go`,
`chrome`) sets MyGo's window options from it as the window is created:
macOS keeps its traffic lights inset into Sona's top bar (a hidden title
bar with `TrafficLightPosition`); Windows and the stacking Linux desktops
(`XDG_CURRENT_DESKTOP` and friends) get a hidden title bar under Sona's top
bar, with MyGo's window buttons in the desktop's layout (GTK's
`gtk-decoration-layout`, which MyGo reads and follows; Windows' caption
buttons); tiling compositors (their `SWAYSOCK`,
`HYPRLAND_INSTANCE_SIGNATURE`, `I3SOCK`, `NIRI_SOCKET`, or their desktop
name) and unknown desktops keep the desktop's, and so does KDE Plasma
under Wayland (`WAYLAND_DISPLAY` set, `GDK_BACKEND` not `x11`): KWin
decorates a window whose title bar is hidden, so a hidden one would sit
under KWin's own; Plasma on X11 gets the hidden title bar. Settings → General → Title
bar overrides it from the next start ("sona-no-buttons" hides MyGo's
buttons too). Sona opens no window menu on a right click. To check it:

- Linux, tiling: run Sona in sway (headless: `WLR_BACKENDS=headless
WLR_LIBINPUT_NO_DEVICES=1 sway`, then the app with `WAYLAND_DISPLAY`,
  `dbus-run-session` and private XDG dirs): the desktop's frame, no
  hidden title bar.
- Linux, stacking: a headless GNOME Shell needs no system changes:
  `dbus-run-session -- env GSETTINGS_BACKEND=keyfile gnome-shell --headless
--wayland --no-x11 --virtual-monitor 1600x1000 --wayland-display=gnome-0`
  (`nix build nixpkgs#gnome-shell`), then the app on `WAYLAND_DISPLAY=gnome-0`
  with `XDG_CURRENT_DESKTOP=GNOME` and the tiling variables unset. Set
  `gtk-decoration-layout` in the app's `$XDG_CONFIG_HOME/gtk-3.0/settings.ini`
  to try layouts (`appmenu:close`, GNOME's default; `close,minimize,maximize:`).
  The `OverviewActive` property of `org.gnome.Shell` leaves the overview,
  and `org.gnome.Shell.Screenshot` takes screenshots from a caller owning
  `org.freedesktop.impl.portal.desktop.gnome`. Check the buttons, a double
  click on the top bar (maximise and restore), dragging, resizing from the
  edges and corners, and Close against the close action.
- Windows: the zig cross-build below under wine 11 with a Windows 7 prefix
  in a wine virtual desktop (`wine explorer
/desktop=sona,1600x1000 sona.exe`): the caption buttons at the top right,
  flush with the edges.
- macOS: on a Mac only:
  the traffic lights centred on the top bar's height beside the sidebar's
  mark, the top bar and the sidebar's header dragging the window, a double
  click zooming, full screen, the menu bar item and its menu (title case:
  "Not Playing", "Private Listening"), the menu bar (Go lists the pages and
  Back and Forward, every item shows its key, a rebinding in Settings →
  Shortcuts changes it, and a key shown in a menu runs once), closing to
  the background (the Dock icon or the menu bar item brings it back), and
  the mini player.

## Architecture rules

- Go owns all Apple Music traffic, tokens, sync, caching, MPRIS and artwork.
  The UI never holds tokens and never calls `*.apple.com`.
- Cold start reads SQLite only; the network and the playback host start
  after the first paint.
- Secrets go to the system keyring (Secret Service, Keychain, Credential
  Manager through go-keyring) or the explicit 0600 file fallback, never to
  the database, logs, settings or committed files.
- Linux performance: MyGo draws on the GPU, so no layout, text shaping or
  image work per frame; artwork is decoded at the exact size shown, and
  backdrops are pre-blurred in Go.

## DRM rules (absolute)

- Protected playback runs in the CDM host child process
  (`internal/playback/cdmhost`). It loads the Widevine CDM Sona downloaded
  and decrypts audio only through the CDM's own `Decrypt` API; music video
  pictures only through the CDM's own video decoder
  (`DecryptAndDecodeFrame`), in SD.
- Decrypted audio never leaves the child: it is never written to disk and
  never crosses the pipe to the parent. Audio processing happens in the
  child, after decryption.
- The only media that leaves the child is decoded music video pictures,
  for display: sent to the parent when due, kept in memory (the latest
  one), never written to disk. Never the compressed or decrypted video
  bitstream.
- The child makes no network requests; the parent fetches everything and
  passes bytes in.
- The CDM is never bundled (in packages or the binary), redistributed or
  patched. It is downloaded only from Google's servers, with the version
  and SHA-512 from Mozilla's pinned manifest and the CRX3 signatures
  verified, and stored in Sona's data dir. Browser, system and
  user-supplied copies are never used.
- Never extract keys or use reimplemented CDMs (`openwv`, extracted device
  keys). Do not try to make Widevine work inside the UI process.

## Strings and translations

Sona takes Apple Music's own translations for every message that means
the same as one of Apple's web strings (docs/ui-architecture.md,
"Strings and locales"). `internal/ui/i18n/locales/apple-keys.json` maps
those messages (English msgid and context) to Apple's string keys, with
placeholder renames; `apple-keys-rejected.json` lists reviewed matches that
do not hold in Sona's context, with the reason. Neither file holds Apple's
text, and Apple's text is never committed or bundled: the app fetches
Apple's bundles from music.apple.com at run time and caches them
(`internal/i18n/applestrings`), and `cmd/i18ntool` fetches them on every
run. The mapping is built into the app, and
`go test ./internal/i18n/applestrings` fails when it no longer fits `en.po`
(a mapped message changed or removed: remove its entry from
`apple-keys.json`, or bring the message back, then re-run `match`).

### Adding or changing a string

The UI's text is written where it is shown, with a string literal:

```go
i18n.T("Add to library")
i18n.T("{count, plural, one {# song} other {# songs}}", "count", n)
i18n.TC("keyboard key", "Home") // context: the same word, another meaning
```

The English text is the message's identity (its msgid), so the message and
the context must be literals (or literals joined with `+`): a variable or a
`fmt.Sprintf` result cannot reach the catalogue, and `extract` rejects it.
Put the whole sentence in one message, with `{name}` arguments and ICU
plurals, never a sentence built from pieces: translators cannot reorder
words they never see together. Changing an English message's wording makes
it a new message, and drops its Apple mapping and its translations.

After adding, changing or removing a call, from the repository root:

```sh
go run ./cmd/i18ntool extract          # writes en.po and en-XA.po
go run ./cmd/i18ntool extract -check   # fails, with the changes, when they are stale
```

`extract` reads every `i18n.T` and `i18n.TC` call under `internal/ui` (test
files and `testdata` excepted) and writes `internal/ui/i18n/locales/en.po`
and `en-XA.po` whole: one entry for each message the code asks for, sorted,
with the Go files that ask (`#: internal/ui/features/…/page.go`) and the
translators' notes (below); a message no call asks for is dropped. The
files are generated, so never edit them. The devenv `check` script and
`go test ./cmd/i18ntool` run the check. It also fails when
`apple-keys.json` maps a message the UI no longer asks for: remove its
entry (or bring the message back). v0.2.0's messages keep their text
byte for byte, so Apple's mapping and any translation of them go on
working; `go test ./internal/i18n/applestrings` pins the 147 mappings
v0.2.0 had.

**Translators' notes.** A comment that starts with `translators:`, directly
above the call or ending on the line the call ends on, becomes the message's
`#.` note in the catalogue, which translators read:

```go
// translators: Equaliser preset
return i18n.T("Flat")

// translators: {0} is the number of songs.
// translators: Said by screen readers when a song starts.
label := i18n.T("{0, plural, one {# song} other {# songs}}", 0, n)
```

Each line starting with `translators:` is one note (the lines after it, up
to the next marker, continue it). Say what the message means where it is
not clear from the text: a word that is a preset, a name or an environment
variable to keep as is, who reads it (screen readers) and what each
argument is. A message asked for in several places gets the notes of all of
them, sorted. Other comments are not notes.

The pseudo-locale finds text that skipped `i18n.T`, is cut or is built from
pieces: `en-XA` shows each message accented, longer and in brackets
(`[Ĥöɱé one]`). `go test ./internal/ui/app` builds the main pages in it and
fails when a message of the catalogue is shown in plain English.

Before writing new English copy, check Apple's wording (its en-GB strings,
keys like `ASE.Web.Music.*`) and use it where the meaning matches. After
`go run ./cmd/i18ntool extract`, from the repository root:

```sh
go run ./cmd/i18ntool match     # counts; proposals for unmapped messages
```

The proposals land in `internal/ui/i18n/locales/apple/proposals.json`
(gitignored; it shows Apple's en-GB text next to each message). Review each
one in context (where the message is shown, `#:` in `en.po`): set
`"decision"` to `"accept"`, or to `"reject"` with a `"reason"`, change
`"key"` to one of `"alternatives"` when another of Apple's keys fits
better, then:

```sh
go run ./cmd/i18ntool match -apply internal/ui/i18n/locales/apple/proposals.json
```

This copies only the message, key and placeholders into the mapping (or the
pair into the rejected list) and checks them against `en.po`. Reject a match
when the meaning differs (a keyboard key against a page name, browser
navigation against skipping a track), when Apple's text is styled for
another place (upper-case badges, lower case only fits mid-sentence), or
when Apple's text drops something Sona's says. `match` also fails when a mapped key has gone from
Apple's en-GB bundle and notes mappings whose English no longer matches.

Wording and case. Sona's English is sentence case (docs/design.md) and
stays so: `match` ignores case, curly quotes, ellipses and closing
punctuation, so "Add to library" maps to Apple's "Add to Library" and a
title-case Apple string is no reason to reject a match. Adopt Apple's
wording, in sentence case, whenever Apple's string names the same action,
concept or state in the same kind of UI place, even when the words differ
(verb, articles, "Back" for "Go back", "Delete from library" for "Remove
from library", no closing ellipsis on a dialog opener, "Top 25 songs by
city"). Keep Sona's own wording only when the meaning differs, Apple's text
would mislead in Sona's context (a keyboard Home against the Home page, the
iPod wheel's Forward against history navigation, Apple's upper-case
badges) or the placeholders cannot map (Apple's "1 Playlist" fixes the
count in its singular form). Record each such refusal in
`apple-keys-rejected.json` with its reason. A message and its siblings
should use one verb: when a button says "Delete from library", its
confirmation, toast and failure message say "delete" too. Most of Sona's
messages still have no Apple counterpart (Apple's en-GB bundle is about
1,000 strings, Sona has about 1,700 of its own), so coverage stays far below
the 90% a locale needs; check Apple's wording before writing a new message,
not afterwards.

```sh
go run ./cmd/i18ntool import    # what the app would get, every locale
```

`import` finds each locale's bundle from music.apple.com's current index
script and runs the app's conversion and checks: `@@name@@` placeholders
become Sona's `{name}`, Apple's `.one`/`.other`/… keys ICU plurals, and a
message whose placeholders do not agree with Sona's is refused. It writes
`internal/ui/i18n/locales/apple/<locale>.po` (gitignored, each entry tagged
`#. apple-key: …`) for review in context, with a per-locale summary (mapped
keys found, missing or refused, coverage of Sona's messages) and each
refusal's reason. It fails when a mapped key has gone from en-GB. The app
never reads these files.

To see Apple's strings in the app while a locale is below the 90% it needs
(`selectableShare` in `internal/ui/i18n`), lower that
value in a local build only; the cache is `$XDG_CACHE_HOME/sona/strings`.

### Go's own text

Text Go shows itself, outside the UI packages (the tray menu, the mini
player's title, desktop notification buttons, file chooser titles and
filters, the sleep inhibitor's reason, and the command line's help, manual
page, completions, output and errors), lives in Go's catalogue,
`internal/i18n/l10n`, never in the UI's. Services never send text to the
UI: they send codes the UI words (`internal/services/errors.go`).
Mark a message by its type, `l10n.Message`, and show it through the user's
`*l10n.Printer` (`Deps.Language.Printer()` in services,
`Options.Language` in the app shell, `Env.Printer` in the command line):

```go
const msgQuit l10n.Message = "Quit Sona" // a translator note

label := p.T(msgQuit)
line := p.Sprintf(msgSearchNone, term) // msgSearchNone = "no results for %q"
```

A typed constant or variable, a conversion `l10n.Message("…")` or the
values of a `map[K]l10n.Message` or `[]l10n.Message` literal are messages;
their value must be a string literal (or literals joined with `+`), or the
name of another message. The declaration's comment becomes the
translator's note. Messages are `fmt` formats: a translation keeps the
English verbs and may reorder them (`%[2]s`), and the app ignores one
whose verbs differ (extraction flags such messages `go-format` so
translators and their tools know). Text that is the same in every language
stays a plain string: command and option names, usage lines, file names,
protocol strings (MPRIS errors), logs and the diagnostics bundle.

After adding or changing a message, from the repository root:

```sh
go run ./cmd/i18ntool extract          # writes internal/i18n/l10n/locales/en.po
go run ./cmd/i18ntool extract -check   # fails, with the changes, when it is stale
```

`en.po` is generated (sorted, with the files each message comes from and
an empty `msgstr`); `go test ./internal/i18n/l10n/extract` and the devenv
`check` script fail when it is out of date. A translation is
`internal/i18n/l10n/locales/<tag>.po`: the same msgids with `msgstr`
filled in (fuzzy entries are skipped), embedded in the binary and picked
for the Language setting, else the system's languages, as the UI
picks its catalogue.

## Design rules

`docs/design.md` is the visual direction. Design every page fresh: Apple's
apps are functional references only, never layouts to copy. Fast before pretty;
light and dark follow the system; everything works from the keyboard; states
(signed out, syncing, offline, no CDM) are stated plainly.

## Licence

By contributing you agree your contribution is licensed under the AGPL-3.0
(`LICENSE`). Code copied from elsewhere must carry a compatible licence and
its notice, and gets a row in `THIRD_PARTY_NOTICES.md`; the vendored
Chromium CDM headers keep theirs
(`internal/playback/widevine/LICENSE.chromium`, BSD-3-Clause).
