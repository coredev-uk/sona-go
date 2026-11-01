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
- `go build ./cmd/sona` needs `frontend/dist` (build the frontend first).
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

### Private mode

Private mode for development and testing: a development build (any build
without the `production` tag: `wails3 dev`, `go run ./cmd/sona`, plain `go build`)
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
| `play-spike` | `go run ./cmd/playspike`: the DRM path regression (needs `MUSIC_USER_TOKEN`); `play-spike -preview` needs none.                                                                                                                                                                                                                            |
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
in `devenv.yaml`, run `devenv update`, then `check`, `tests`, `play-spike`
and the app, and commit `devenv.yaml` and `devenv.lock` together.

### macOS

`devenv shell` works on Apple Silicon (`aarch64-darwin`): the Linux-only
packages (GTK4, WebKitGTK, libsoup, glib-networking, PipeWire, playerctl,
gcc, dbus) and the GIO module path are left out, and GStreamer keeps core,
base, good (`osxaudiosink`), bad and libav, on the same plugin path as on
Linux. Intel Macs (`x86_64-darwin`) are not supported: the pinned nixpkgs
(26.11) and devenv 2.4 dropped them.

devenv sets one macOS deployment target on macOS: `MACOSX_DEPLOYMENT_TARGET`
= nixpkgs' (14.0), and `GOFLAGS` adds
`-ldflags=-extldflags=-mmacosx-version-min=14.0`, which overrides the 10.13
that Wails' cgo directives pass to the linker; without it ld warns "object
file … was built for newer macOS version (14.0) than being linked (11.0)" for
every cgo object. A `go build -ldflags=…` of your own replaces GOFLAGS'
`-ldflags`; add `-extldflags=-mmacosx-version-min=$MACOSX_DEPLOYMENT_TARGET`
to it (`build/darwin/Taskfile.yml` does).

The app builds and runs on macOS as an unsigned development build (no
Apple Developer account, no `.app` bundle, no packaging): its UI,
everything Go does (sign-in, library, sync, catalog) and playback through
the same CDM host as Linux, with the CDM Sona downloads from Google for
macOS (`mac_arm64`, see "Widevine CDM") and Core Audio output
(`osxaudiosink`, always the system's default output). There are no media
controls, notifications, sleep inhibit, autostart, file chooser or `sona`
commands yet (see "Platforms"). Apple's licence server grants licences to
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
audio plays on the default output, and `-device` is refused on macOS):

```sh
SONA_PRIVATE=1 go run ./cmd/playspike -seek 90 -gap=false
SONA_PRIVATE=1 go run ./cmd/playspike -activity   # only with SONA_PRIVATE=1
```

It prints `cdm source source=download` with the library under
`~/Library/Application Support/Sona/widevine/`, then each `licence
exchange` (HTTP status, Apple's status and message, the licence's length;
never the token or the licence), and ends with `licence: granted`,
`licence: refused (<why>)` or `licence: not requested (<why>)`.

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
request except `golangci-lint` and `i18n:check`, so run those yourself):

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
`internal/playback/cdm/cdmtest`: the CDM installed in `SONA_CDM_CACHE` (default
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
real, locked keyring and Sona waits on its unlock prompt before showing a
window.

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

### CI and workflows

`.github/workflows/`: `ci.yml` (every push to main and pull request: the
Linux Go job "Go vet and test (-race)" and "Frontend typecheck, lint,
format, test, build"; plus "macOS build and test" (devenv on macos-15:
production build, go vet, real-CDM and sandbox tests, `go test -race`) and
"Windows build" (windows-2025: frontend, `go vet` with cgo, `sona.exe`),
which take several minutes and so never run on pull requests: they run
after each merge to main, before a release, and when dispatched), `pr-title.yml` (Conventional Commits title),
`live.yml` (weekly Apple Music contract tests, private mode), `release.yml`
(releases), `release-secrets.yml` (dispatch: checks the `release`
environment's secrets are set, publishes nothing), `renovate.yaml`
(dependency updates; maintained with Renovate).

Rules for workflow changes: trigger pull requests with `pull_request`, never
`pull_request_target` or `workflow_run`; start from `permissions: contents:
read` and widen per job only where needed; pin every action to a full commit
SHA with the version in a comment (`uses: actions/checkout@<sha> # v7.0.1`;
Renovate keeps pins current); `persist-credentials: false` on every checkout
that doesn't push; pass expressions to scripts through `env:`, never inline
in `run:`; secrets live in environments (`release`: Last.fm key/secret,
Discord ID, deploys from main and `v*` tags; `live`: `MUSIC_USER_TOKEN`, main
only), so no pull request run can read them. Run `actionlint` on changed
workflows. Threat model: `docs/security-review.md`, "Workflows".

### Desktop VM tests

`tests/vm` holds NixOS VM tests that run Sona in real GNOME and KDE Plasma sessions (headless QEMU, no network, private mode) and check what the release checklist's desktop rows ask: first paint, the title bar's buttons, launcher entry, single instance, MPRIS in the shell, notifications, tray, X11 (XWayland), HiDPI scaling and `sona private status`. They need Linux with KVM and Nix (no devenv):

```sh
nix-build tests/vm -A gnome     # or -A plasma; -A all runs every test
nix-build tests/vm -A sona      # the Nix package the VMs install
```

Screenshots land in `result/`. `tests/vm/README.md` lists each check, the checklist rows it covers, the interactive driver, and how to update the package's hashes after a lockfile change. They never run on pull requests: a CI run takes about an hour of runner time, so start the "VM tests" workflow by hand (`workflow_dispatch`) before a major release, or run them locally.

### Secret scanning

CI runs [gitleaks](https://github.com/gitleaks/gitleaks) on every pull request and every push to `main` (`.github/workflows/secret-scan.yml`): over every commit of every branch and tag, your pull request's own commits included, and over the pull request's title and body. `.gitleaks.toml` adds Sona's rules to gitleaks' defaults: JWTs (the Apple developer token), Music User Tokens, the Last.fm key and secret, the Discord application ID, and credential files (`.env`, `.p8`, `.pem`, keys, keystores). The public log shows only each finding's rule, commit, file and line. Run it before pushing with `nix run nixpkgs#gitleaks -- git --config .gitleaks.toml --redact .` (CI pins gitleaks 8.30.1). Test fixtures that need token-shaped input use obviously fake values; an allowlist entry in `.gitleaks.toml` names the file and the exact fake value, with a comment saying why it is fake. A commit that ever held a real secret stays public through its pull request even after a squash merge or force-push: rotate the secret first, then tell the maintainer. Details: `docs/security-review.md`, "Secret scanning".

## Branches and pull requests

- One change per branch and pull request. Parallel work is easiest in
  separate worktrees (`git worktree add ../sona-go-<slug> -b <type>/<slug>`);
  a new worktree needs `pnpm install` in `frontend/` and a frontend build
  before `go build ./cmd/sona`. Give each concurrently running worktree its own
  `WAILS_VITE_PORT`.
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
  (-race)", "Frontend typecheck, lint, format, test, build", "Conventional
  Commits title" and "gitleaks". Force pushes and deleting `main` are
  blocked. Release tags (`v*`) can't be moved or deleted. A first pull
  request from someone outside the project waits for the maintainer to
  approve its CI run. Report vulnerabilities privately (SECURITY.md).
- Fill in the PR template: what changed, why, evidence (commands and their
  output; screenshots in light and dark themes for UI changes, without
  personal account data), tests, docs, and anything left.
- Update the docs describing what you change in the same PR.
- Generated files are never edited or merged by hand. After rebasing onto
  `main`, regenerate `frontend/bindings`, the Lingui catalogues
  (`pnpm i18n:extract` in `frontend/`) and `internal/db/dbq` (`gen`), then
  commit them.

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
it, which tags `v<VERSION>` and runs `release.yml` to build and publish the
packages. Never edit `VERSION` or `CHANGELOG.md` by hand. Steps:
`docs/release-checklist.md`, "Releasing".

## Dependency updates

Renovate opens pull requests for Go modules, pnpm packages and GitHub
Actions, and a review bot checks and merges them; the pinned devenv inputs
are bumped by hand (see "Pinned inputs"). You
don't need to update dependencies yourself; a change that needs a newer
dependency bumps it in the same PR.

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
every OS-specific part behind a seam that reports itself unsupported there
until it is implemented; macOS development builds also play through the CDM
host. Windows has no playback yet.

- In `cmd/sona`, `main.go` is portable; `platform_linux.go` and `platform_other.go` build
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
  windowsgui' ./cmd/sona` after a production frontend build, with `go tool
  wails3 generate syso` (its `.syso` in `cmd/sona`) for the icon and
  manifest. zig ignores the `-mwindows` Go
  passes for `-H windowsgui`, so wrap it to pass
  `-Wl,--subsystem,windows` instead, or the exe opens a console. It needs
  the WebView2 runtime to show its window.

### Title bar and window controls

`internal/windowchrome` decides who draws the main window's title bar
(`docs/design.md`, "Title bar"): macOS keeps its traffic lights inset into
Sona's top bar (`MacTitleBarHiddenInset`); Windows and the stacking Linux
desktops (`XDG_CURRENT_DESKTOP` and friends) get a frameless window with
Sona's own buttons in the desktop's layout (KWin's `kwinrc` on KDE
Plasma, GTK's `gtk-decoration-layout` elsewhere, read through
`internal/shell`); tiling compositors (their `SWAYSOCK`,
`HYPRLAND_INSTANCE_SIGNATURE`, `I3SOCK`, `NIRI_SOCKET`, or their desktop
name) and unknown desktops keep the desktop's. Settings → Desktop → Title
bar overrides it from the next start. To check it:

- Linux, tiling: run Sona in sway (headless: `WLR_BACKENDS=headless
  WLR_LIBINPUT_NO_DEVICES=1 sway`, then the app with `WAYLAND_DISPLAY`,
  `dbus-run-session` and private XDG dirs): no buttons, and the window is
  not frameless.
- Linux, stacking: a headless GNOME Shell needs no system changes:
  `dbus-run-session -- env GSETTINGS_BACKEND=keyfile gnome-shell --headless
  --wayland --no-x11 --virtual-monitor 1600x1000 --wayland-display=gnome-0`
  (`nix build nixpkgs#gnome-shell`), then the app on `WAYLAND_DISPLAY=gnome-0`
  with `XDG_CURRENT_DESKTOP=GNOME` and the tiling variables unset. Set
  `gtk-decoration-layout` in the app's `$XDG_CONFIG_HOME/gtk-4.0/settings.ini`
  to try layouts (`appmenu:close`, GNOME's default; `close,minimize,maximize:`).
  The `OverviewActive` property of `org.gnome.Shell` leaves the overview,
  and `org.gnome.Shell.Screenshot` takes screenshots from a caller owning
  `org.freedesktop.impl.portal.desktop.gnome`. Check the buttons, a double
  click on the top bar (maximise and restore), the window menu on a right
  click, dragging, resizing from the edges and corners, and Close against
  the close action.
- Windows: the zig cross-build below under wine 11 with a Windows 7 prefix
  and the WebView2 runtime, in a wine virtual desktop (`wine explorer
  /desktop=sona,1600x1000 sona.exe`): the caption buttons at the top right,
  flush with the edges.
- macOS: on a Mac only:
  the traffic lights centred on the top bar's controls, the top bar
  dragging the window, a double click zooming, full screen, and the mini
  player.

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
  and decrypts only through the CDM's own `Decrypt` API.
- Decrypted audio never leaves the child: it is never written to disk and
  never crosses the pipe to the parent. Audio processing happens in the
  child, after decryption.
- The child makes no network requests; the parent fetches everything and
  passes bytes in.
- The CDM is never bundled (in packages or the binary), redistributed or
  patched. It is downloaded only from Google's servers, with the version
  and SHA-512 from Mozilla's pinned manifest and the CRX3 signatures
  verified, and stored in Sona's data dir. Browser, system and
  user-supplied copies are never used.
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
its notice, and gets a row in `THIRD_PARTY_NOTICES.md`; the vendored
Chromium CDM headers keep theirs
(`internal/playback/widevine/LICENSE.chromium`, BSD-3-Clause).
