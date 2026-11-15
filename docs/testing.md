# Testing reference

The full detail behind the [testing page on the site](https://sona.coredev.uk/contributing/test/): the checks, preview playback, the real-CDM tests, the UI tests and the end-to-end smoke, the desktop VM tests and secret scanning.

## Checks

Run before sending a change (the devenv `check` and `tests` scripts run all
of it; CI, `.github/workflows/ci.yml`, runs the same on every push and pull
request except `golangci-lint`, `go tool mygo vet internal/ui` and the catalogue check, so run those yourself):

```sh
go vet ./...
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go vet ./...   # see `docs/platforms.md`
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
a failure. CI (the `.github/actions/setup-cdm` action) resolves the version
(`go run ./cmd/cdmfetch -version`; the Linux job keys by ISO week instead),
restores `~/.cache/sona-ci/widevine` with `actions/cache` keyed
`widevine-<os>-<arch>-<version or week>`, and fills a miss with
`go run ./cmd/cdmfetch -dir ~/.cache/sona-ci/widevine`, exporting
`SONA_CDM_CACHE`; the job sets `SONA_CDM_REQUIRE=1`. The playback shard then runs them like any test, and `skips.sh` fails the job
if one of them skipped (the CDM tests are in no allow-list).

## Preview playback (tests without a Music User Token)

`MUSIC_USER_TOKEN` is not a test credential: Apple refreshes it now and then, so a stored copy (`.env`, a CI secret) goes stale. Tests, CI and smoke runs that need audio play Apple's 30-second song previews instead, which need only the developer token Sona reads from Apple's web player: no `.env`, no Widevine CDM, no licence. (Apple's web player does the same when signed out: MusicKit JS `instance.previewOnly = true`, i.e. `playbackMode` `PREVIEW_ONLY`.)

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

On Windows the same child runs in an AppContainer (`sandbox_windows_test.go`: a probe child that must fail to connect, read outside its folders, write under the profile or start a process, and must load the CDM; the whole host suite runs through the sandbox because the child loads no CDM outside it). A machine that refuses AppContainers skips the probe tests and fails the host ones; CI sets `SONA_CDMHOST_REQUIRE_SANDBOX=1`, which turns the skip into a failure. A test binary alone honours `SONA_CDMHOST_ALLOW_UNSANDBOXED=1` to load the CDM outside the sandbox. `TestRemoveSandbox` covers the uninstaller's cleanup (`sona --remove-sandbox`, `cdmhost.RemoveSandbox`) on a profile of its own, never `sona.cdmhost`; a new grant of the sandbox's SID to a folder (`grantAccess`) must be one `RemoveSandbox` also removes.

Packaging changes: `bash build/linux/release/release.sh` (build every package
on Ubuntu 24.04 and smoke-test it on clean Ubuntu, Fedora and Arch
containers, plus the AppImage on Fedora) is the packaging check; it needs
podman or docker and takes a while, so CI runs it. `task package` packages
your local build for a look at the files (a devenv build links `/nix/store`
and is not a release). Before a release, work through
`docs/maintainers/release.md`.

Flatpak changes (`build/linux/flatpak`, `go.mod`): `bash build/linux/flatpak/build.sh`
builds the Flatpak offline from the working tree and installs it `--user`, and
`devenv shell -- bash build/linux/flatpak/smoke.sh` runs it in the headless
session. Both need flatpak-builder with the GNOME 51 runtime, SDK and Go
extension from Flathub (`docs/maintainers/flatpak.md`, "Build and test"). After a `go.mod`
change, regenerate `build/linux/flatpak/go-sources.json` with
`go run build/linux/flatpak/gosources.go > build/linux/flatpak/go-sources.json`;
the dispatch-only `Flatpak` workflow builds it in CI.

## UI tests and the end-to-end smoke

- **`ui.Tester` tests** (`go test ./internal/ui/...`, no display) run over `internal/ui/uitest`: `uitest.Env(t)` is a `module.Env` over real services on a temporary SQLite library seeded with a synthetic fixture (`internal/ui/uitest/fixture`: invented artists, albums, songs and playlists, generated covers; never Apple's data), a fake playback engine, a fake artwork CDN, a Poster the test drains and a fake clock. The Core is hermetic: an empty in-memory keyring, an HTTP transport that fails any host the test does not serve (`Harness.Network.Serve` adds one), and home, XDG and D-Bus environment under the test's temporary directory, so a test never sees your Last.fm session, GitHub or your session bus. `docs/ui-architecture.md` ("Testing with `ui.Tester`", "Tests") has the API.
- **Golden images** are few: the shell with Albums, light and dark, at 1132×1253 (`internal/ui/app/testdata/golden/`). `uitest.Golden` compares by the mean colour of 8×8-pixel blocks (another machine's text antialiasing passes; a moved edge, a changed colour or a missing cover does not). When a change moves the picture on purpose, review it and run `go test ./internal/ui/app -run Golden -update`; commit the new PNGs and say in the PR why. A failing run keeps the picture and a diff in the test's artifact directory: `$SONA_TEST_ARTIFACTS/<test name>` when that variable is set (CI's `.github/ci/test.sh` sets it and uploads the directory), else the test's `t.ArtifactDir()` (`go test -artifacts -outputdir DIR`). Or ask a maintainer to regenerate them on your pull request. On a Mac (or any machine whose text differs from CI's), update goldens in the CI image: `podman run --rm -v "$PWD":/src -w /src -e MYGO_GPU=0 ghcr.io/coredev-uk/sona-ci:$(build/ci/tag.sh) go test ./internal/ui/... -update`. The image is amd64 and arm64: an arm64 machine (Apple silicon, through Podman's VM) may draw text a hair differently from CI's amd64 runners, and CI is the reference, so push and check the run. Tests get the directory from `internal/ui/uitest/artifacts` and only when they fail, because `go test` never caches a run that uses `-artifacts`.
- **The end-to-end smoke** (`e2e/`, build tag `e2e`) drives the built app from outside: `e2e` script, or `headless -- go test -tags e2e ./e2e/...`. It builds `bin/sona-e2e` with `-tags probe,sqlite_fts5` (MyGo's automation plugin and a seam that exists only in probe builds: `SONA_E2E=1` swaps in the fake playback engine and `SONA_E2E_CDN=host:port` sends all the app's HTTP to a local server of the fixture's covers, so nothing reaches Apple), starts it on the fixture profile (`go run ./e2e/profile` writes the same profile into private XDG directories), and checks first paint, Albums, playing an album, Space, a capture and, from the AT-SPI bus, the shell's landmarks by role and name. `E2E_OUT=<dir>` keeps the capture (`smoke.png`) and the app's stderr; `E2E_BIN=<path>` runs an existing binary. `headless` needs `sway`, `wtype` and `at-spi2-core` (devenv has them; elsewhere install them with your package manager). Never run a probe build outside tests: it can be driven by anything that opens its socket.
- **The Orca walk** (`e2e/orca_test.go`, `e2e/orca_walk_test.go`; `orca-walk` script) runs a real Orca beside the app in the same headless session and presses keys with `wtype`, as a keyboard does: Tab and the arrows through the sidebar, the Albums page, an album, the queue, Now Playing, the Seek slider, the account dialog and the first-run setup. Orca writes every utterance to its debug file (`--debug-file`) whether or not a speech server is there, so the run needs no audio and no speech-dispatcher; a private `orca-customizations.py` makes that file line-buffered, which Orca does not (8 KB blocks). The test checks what it said against `docs/accessibility.md`; `E2E_OUT` keeps `orca-walk.md` and `orca-setup.md` (what each key was answered with, and where Orca put the focus) and Orca's whole log. It needs `E2E_ORCA=1` (otherwise it skips) and Orca (`E2E_ORCA_BIN`, else `orca` on the PATH). CI runs it only when dispatched (`orca` or `all`), never on a pull request, and it is not a required check: its words and timing are Orca's, so a new Orca can move them. The CI job runs in the CI image with Orca (and procps) installed on top, its .debs cached per image, and restores the e2e job's Go caches (it saves none). The harness reads Orca's log as it grows (only the new bytes, each entry parsed once; `e2e/orca_log_test.go` tests that reader, in the e2e job) and takes a key's answer as complete when Orca's event queue is empty and it has said or queued nothing but a slider's value tick for 0.6 s (the longest quiet inside an answer on CI was 0.22 s), so each of the walk's 97 keys costs under a second.

## Desktop VM tests

`tests/vm` holds NixOS VM tests that run Sona in real GNOME and KDE Plasma sessions (headless QEMU, no network, private mode) and check what the release checklist's desktop rows ask: first paint, the title bar's buttons, launcher entry, single instance, MPRIS in the shell, notifications, tray, X11 (XWayland), HiDPI scaling and `sona private status`. They need Linux with KVM and Nix (no devenv):

```sh
nix-build tests/vm -A gnome     # or -A plasma; -A all runs every test
nix-build tests/vm -A sona      # the Nix package the VMs install
```

Screenshots land in `result/`. `tests/vm/README.md` lists each check, the checklist rows it covers, the interactive driver, and how to update the package's hashes after a lockfile change. They never run on pull requests: a CI run takes about an hour of runner time, so start the "VM tests" workflow by hand (`workflow_dispatch`) before a major release, or run them locally.

## Secret scanning

CI runs [gitleaks](https://github.com/gitleaks/gitleaks) on every pull request and every push to `main` (`.github/workflows/secret-scan.yml`): over every commit of every branch and tag, your pull request's own commits included, and over the pull request's title and body. `.gitleaks.toml` adds Sona's rules to gitleaks' defaults: JWTs (the Apple developer token), Music User Tokens, the Last.fm key and secret, the Discord application ID, and credential files (`.env`, `.p8`, `.pem`, keys, keystores). The public log shows only each finding's rule, commit, file and line. Run it before pushing with `nix run nixpkgs#gitleaks -- git --config .gitleaks.toml --redact .` (CI pins gitleaks 8.30.1). Test fixtures that need token-shaped input use obviously fake values; an allowlist entry in `.gitleaks.toml` names the file and the exact fake value, with a comment saying why it is fake. A commit that ever held a real secret stays public through its pull request even after a squash merge or force-push: rotate the secret first, then tell the maintainer. Details: `docs/security-review.md`, "Secret scanning".
