# Testing reference

The detail behind the [Test Your Change](https://sona.coredev.uk/contributing/test/) page: the checks, preview playback, the real-CDM and sandbox tests, packaging checks, the UI tests and end-to-end smoke, the desktop VM tests, fuzzing and secret scanning.

## Contents

- [Checks](#checks)
  - [Real-CDM tests](#real-cdm-tests)
- [Timing in tests](#timing-in-tests)
- [Preview playback](#preview-playback)
  - [Run checks that play music](#run-checks-that-play-music)
  - [Run the app privately](#run-the-app-privately)
- [Sandbox tests](#sandbox-tests)
  - [Linux and Ubuntu](#linux-and-ubuntu)
  - [Windows](#windows)
- [Packaging checks](#packaging-checks)
  - [Linux packages](#linux-packages)
  - [Flatpak](#flatpak)
- [UI tests and the end-to-end smoke](#ui-tests-and-the-end-to-end-smoke)
  - [ui.Tester tests](#uitester-tests)
  - [Golden images](#golden-images)
  - [Overlay checks and the marks contact sheet](#overlay-checks-and-the-marks-contact-sheet)
  - [The end-to-end smoke](#the-end-to-end-smoke)
  - [The Orca walk](#the-orca-walk)
- [Desktop VM tests](#desktop-vm-tests)
- [Fuzzing](#fuzzing)
  - [When a target finds something](#when-a-target-finds-something)
  - [Writing a target](#writing-a-target)
  - [The targets](#the-targets)
- [Secret scanning](#secret-scanning)

## Checks

Run these before sending a change. The devenv `check` and `tests` scripts run all of it.

```sh
go vet ./...
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go vet ./...   # see platforms.md
go run ./cmd/typecheck -os darwin -arch arm64 ./...
go test -race ./...    # CI's pull requests skip -race for internal/ui's page and app packages (see maintainers/ci-workflows.md)
golangci-lint run ./...   # the whole tree; must print 0 issues; devenv pins the version CI installs
# a package lints only in the build that compiles it, so lint each build too (CI does; cgo off for Windows and macOS, as the type-checks):
golangci-lint run --build-tags dev,sqlite_fts5 ./...
golangci-lint run --build-tags probe,sqlite_fts5 ./...
golangci-lint run --build-tags e2e,sqlite_fts5 ./...
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 golangci-lint run ./...
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 golangci-lint run ./...
go tool mygo vet internal/ui   # build values kept past their frame
go run ./cmd/i18ntool extract -check   # the catalogue; go test checks it too
go run ./cmd/pathcase                  # no paths differing only by case
go tool govulncheck -show verbose -tags sqlite_fts5 ./...   # reachable vulnerabilities; needs the network
sh build/gofumpt.sh   # lists the tracked Go files gofumpt (go.mod's, `go tool gofumpt`) would change, and fails; `fmt` fixes them
```

CI (`.github/workflows/ci.yml`, the "Vet and type-check" job) runs what `check` runs, so a change that fails `check` fails there too: the vets, the type-checks, `golangci-lint`, `go tool mygo vet internal/ui`, the catalogue check, the path-case check and the gofumpt check. `golangci-lint` runs on the whole tree on every run, a push to main included, in each of the six builds above (the host, dev, probe, e2e, Windows and macOS). `govulncheck` is the one CI runs that `check` doesn't: on pull requests, the nightly and releases (not on a push to main), it needs the network and today's vulnerability database (policy: `maintainers/ci-workflows.md`).

### Real-CDM tests

These tests need the Widevine CDM:

- `internal/playback/widevine`
- `internal/playback/cdmhost`
- `internal/diag`
- `internal/playback/cdm`

They find it through `internal/playback/cdm/cdmtest`: the CDM installed in `SONA_CDM_CACHE` (default `<user cache dir>/sona-tests/widevine`, shared by every checkout), downloaded and verified there on first use. They skip only when the download cannot reach the network. `SONA_CDM_REQUIRE=1` turns that skip into a failure.

CI (the `.github/actions/setup-cdm` action):

1. Resolves the version with `go run ./cmd/cdmfetch -version`. The Linux job keys by ISO week instead.
2. Restores `~/.cache/sona-ci/widevine` with `actions/cache`, keyed `widevine-<os>-<arch>-<version or week>`.
3. Fills a miss with `go run ./cmd/cdmfetch -dir ~/.cache/sona-ci/widevine`, and exports `SONA_CDM_CACHE`.

The job sets `SONA_CDM_REQUIRE=1`. The playback shard then runs the tests like any test, and `skips.sh` fails the job if one of them skipped (the CDM tests are in no allow-list).

## Timing in tests

CI runners are shared, run everything under `-race`, and so does `tests` on a busy workstation. A test therefore waits for the state it checks, never for an amount of time ([architecture.md](architecture.md), "Timing"). The check that this holds: `devenv shell -- tests` passes five times in a row while another `go test -race ./...` runs beside it.

- **Wait on what the test checks.** An event, a channel the fake closes, a post to the main thread (`uitest.MainThread.Wait` and `Drain`, `uitest.Session.Until`), the artwork loader's `Pending()` (the kit's `mainThread.settle`), a text on screen. Never `time.Sleep(20 * time.Millisecond)` to let a goroutine finish.
- **Wait for the last step, not the first.** Code that publishes a state and then finishes its work (the add-on supervisor marks a run crashed, then saves the switch, then posts the entry; a development run leaves the runners, then ends its log buffer) shows the first state before the rest is there. Wait on the last step (the entry, the runner's `done`, `waitSwitchedOff`) before reading any of them.
- **Wait for the whole text of what you click.** `HasText` and `Shows` match parts of a text, so `Shows("Play")` holds on the bar's "Player" and "Open now playing" before the Play button is there, and the click that follows finds the old Pause. `ShowsExactly` matches a whole text or label, as `MustClick` does: wait with it for a control the test clicks next.
- **Move a clock the test owns.** The `ui.Tester`'s clock stands still until `Advance`; the harness has `Clock`; the services' clock is `uitest.ServiceClock(now)` (a test that crosses a release date moves it with the page's own); stores and loaders take a `now`. Goroutines with timers of their own (the item states' 16 ms batch, the playback coordinator, the probe) run under `testing/synctest`: fake time moves only when every goroutine in the bubble waits, and `synctest.Wait` (`Session.Quiesce`) is the barrier for "everything that was going to happen has happened". A test that a thing did not happen ends in such a barrier, or in an operation ordered after what would have happened, not in a sleep.
- **A deadline is a hang guard, not a speed limit.** It is `uitest.HangGuard` (30 s; `hangGuard` in a package that cannot import `uitest`, such as `kit`) and only turns a lost wake-up into a failure. A test never asserts that something took less than some milliseconds: it asserts what would make it slow (a request count, a plan with no table scanned per row). `Session.Settle` is bounded by frames and by progress, not by its own run time, and `devenv shell -- tests` gives each package 30 minutes (`-timeout 30m`, where `go test`'s default is 10), the full race pass's Linux UI shards (nightly, release) 18 and the nightly's macOS UI shards 25, because a UI package under load takes several times its idle run.
- **The tester's clock starts at the wall clock.** What is drawn from the clock modulo a period (the skeleton's pulse, the equaliser's bars) depends on when the test began. Line the clock up with `tt.Now()` and `Advance`, or pin the state, as `TestSkeletonPulsesUnlessMotionIsReduced` does.
- **A timer in the code under test needs a seam.** Where a batch waits on a real timer (`SongArtists`), the test sets a wait too long to fire and calls the flush itself, or runs in a bubble. A wait that a service measures on `Env.Now` (the development run's lease) is moved with a clock the test owns (`testClock`, set as `Env.Now`): the service's own ticker then only decides how soon it notices.
- **Settle after the data.** `Session.Settle` knows covers and posts only. `Until` the page's own text first, or the golden is a picture of the loading state.
- **Leave no process-wide switch behind.** `theme.SetReducedMotion`, `SetReduceEffects` and `SetMode` are process-wide: a test that sets one restores it in `t.Cleanup`, or a later test draws something else.

## Preview playback

`MUSIC_USER_TOKEN` is not a test credential: Apple refreshes it now and then, so a stored copy (`.env`, a CI secret) goes stale. Tests, CI and smoke runs that need audio play Apple's 30-second song previews instead. They need only the developer token Sona reads from Apple's web player: no `.env`, no Widevine CDM, no licence.

Apple's web player does the same when signed out: MusicKit JS `instance.previewOnly = true`, i.e. `playbackMode` `PREVIEW_ONLY`.

- **The app.** `SONA_PREVIEW=1` (any build) plays every song as its preview, through a playback host that loads no CDM (still sandboxed, still without network).
  - The player bar shows "Preview" beside the title.
  - Songs are looked up in the account's storefront, or the locale's when signed out.
  - Library uploads and radio have no preview and are skipped.
  - Nothing reports a preview as a play: no Apple play activity, history or scrobble.
- **`playspike`.** `go run ./cmd/playspike -preview [-storefront gb] [-device <sink>]` plays a song's preview, watches it advance, plays it out and starts the next one, with no `.env` and no CDM (none is downloaded). It ends with `licence: not requested`, and fails if any webPlayback, certificate or licence request went out.
- **`TestPreviewPlaysLive`** (`internal/playback/native`) plays a real preview end to end into a fakesink. It skips only when Apple can't be reached; `SONA_PREVIEW_REQUIRE=1` (CI) turns that skip into a failure.
- **The full DRM path** keeps its own deliberate check with a Music User Token: `go run ./cmd/playspike -seek 90` and the live workflow.

### Run checks that play music

After touching playback or Apple request code, also run the DRM path regression `go run ./cmd/playspike -seek 90` (needs `MUSIC_USER_TOKEN`).

Every playback run in tests, smokes and live checks uses private mode: `SONA_PRIVATE=1`, though development builds start private and `playspike` is private by default. Nothing is then reported to Apple or scrobbled.

Route long listening tests to your own PipeWire null sink, not your speakers, and remove only the sinks you created:

```sh
pw-cli create-node adapter '{ factory.name=support.null-audio-sink node.name=sona-test media.class=Audio/Sink object.linger=true audio.position=[FL FR] }'
SONA_PRIVATE=1 go run ./cmd/playspike -device sona-test -seek 90
pw-cli destroy sona-test
```

The [Test Your Change](https://sona.coredev.uk/contributing/test/#check-live-playback) page has the short recipe, and [development.md](development.md) the details.

### Run the app privately

When you run the app for a check that doesn't need your desktop, start it under `dbus-run-session` with private `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME` and `XDG_CACHE_HOME`. It then neither clashes with a running Sona (single instance, MPRIS name) nor touches your real settings and library.

Set the XDG variables outside `dbus-run-session`:

```sh
env XDG_…=… dbus-run-session -- bin/sona
```

The services the private bus starts (the Secret Service, the portals) then use them too. Started the other way round, GNOME Keyring opens your real, locked keyring and Sona waits on its unlock prompt. The window shows "Signing in…", and the run reads your real keyring.

## Sandbox tests

### Linux and Ubuntu

The CDM host sandbox tests (`TestChildCannotOpenInetSockets`, `TestSandboxConfinesChild/namespaces`) need unprivileged user namespaces in which the child may change its mounts.

- Where the system refuses them, they skip and say why. Refusals are EPERM or EACCES: Flatpak, user namespaces disabled, or Ubuntu 24.04's AppArmor restriction `kernel.apparmor_restrict_unprivileged_userns=1`. Any other failure fails them.
- To run them on Ubuntu, lift the restriction for the session: `sudo sysctl -w kernel.apparmor_restrict_unprivileged_userns=0`.
- CI does that, and sets `SONA_CDMHOST_REQUIRE_NAMESPACES=1`, which turns a skip into a failure.
- The shipped configuration is the restriction on, with the package's AppArmor profile (`build/linux/apparmor/sona`, installed as `/etc/apparmor.d/sona`). To run the tests that way on an Ubuntu 24.04 machine, build the test binary (`go test -c ./internal/playback/cdmhost`), install it as `/usr/bin/sona` (the path the profile matches), load the profile (`sudo apparmor_parser -r /etc/apparmor.d/sona`) and run it as an ordinary user with `-test.run 'TestSandboxConfinesChild|TestChildCannotOpenInetSockets'`. The `namespaces` subtest then runs and passes under the default sysctl. Without the profile it skips, and `landlock only` shows the session bus reachable below Landlock ABI 9 (`reaches sockets: true`).

The add-on sandbox's tests (`internal/addons/sandbox`, in the same CI job) need the same user namespaces and follow the same rules: they skip where the system refuses, and `SONA_ADDON_REQUIRE_SANDBOX=1` (CI) fails them instead. The test binary is the add-on too: `TestMain` calls `sandbox.Main`, the sandbox starts the binary again as an add-on's init, and the add-on (the binary with `SONA_SANDBOX_TEST_PROBE` set) reports what it could reach. The supervisor's own tests (`internal/services`, `internal/addons`) run their fake add-ons without the sandbox through `SONA_ADDON_UNSANDBOXED=1`, which only a test binary honours; a simulated sandbox failure tests the refusal, and the unsandboxed start of Sona's own add-ons.

### Windows

On Windows the same child runs in an AppContainer. `sandbox_windows_test.go` uses a probe child that must:

- fail to connect;
- fail to read outside its folders;
- fail to write under the profile or start a process;
- load the CDM.

The whole host suite runs through the sandbox, because the child loads no CDM outside it. A machine that refuses AppContainers skips the probe tests and fails the host ones. CI sets `SONA_CDMHOST_REQUIRE_SANDBOX=1`, which turns the skip into a failure.

A test binary alone honours `SONA_CDMHOST_ALLOW_UNSANDBOXED=1` to load the CDM outside the sandbox.

`TestRemoveSandbox` (in `internal/appcontainer`) covers the uninstaller's cleanup (`sona --remove-sandbox`, `cdmhost.RemoveSandbox`) on a profile of its own, never `sona.cdmhost`. A new grant of the sandbox's SID to a folder (`appcontainer.Grant`) must be one `appcontainer.Remove` also removes. `TestRemoveProfiles` (in `internal/addons/sandbox`) does the same for the add-ons' profiles (`sandbox.RemoveProfiles`); the choice of which add-on IDs to remove (`sandbox.ProfileIDs`) is plain Go and tested on every OS. The package's tests skip on a machine that refuses AppContainers; `SONA_APPCONTAINER_REQUIRE=1` (CI) fails them instead.

## Packaging checks

### Linux packages

`bash build/linux/release/release.sh` is the packaging check. It builds every package on Ubuntu 24.04 and smoke-tests it on clean Ubuntu, Fedora and Arch containers, plus the AppImage on Fedora. On Arch the smoke also runs `namcap` on the AUR PKGBUILD and the built package, and fails on any error or warning except the run-time dependencies MyGo and GStreamer load by soname (listed by name in `smoke.sh`). It needs podman or docker and takes a while, so CI runs it.

`task package` packages your local build for a look at the files. A devenv build links `/nix/store` and is not a release. Before a release, work through [maintainers/release.md](maintainers/release.md).

### Flatpak

For changes to `build/linux/flatpak` or `go.mod`:

- `bash build/linux/flatpak/build.sh` builds the Flatpak offline from the working tree and installs it `--user`.
- `devenv shell -- bash build/linux/flatpak/smoke.sh` runs it in the headless session.
- Both need flatpak-builder with the GNOME 51 runtime, SDK and Go extension from Flathub ([maintainers/flatpak.md](maintainers/flatpak.md), "Build and test").

After a `go.mod` change, regenerate the sources file:

```sh
go run build/linux/flatpak/gosources.go > build/linux/flatpak/go-sources.json
```

The dispatch-only `Flatpak` workflow builds it in CI.

## UI tests and the end-to-end smoke

### ui.Tester tests

`go test ./internal/ui/...` needs no display. The tests run over `internal/ui/uitest`: `uitest.Env(t)` is a `module.Env` over real services on a temporary SQLite library, with:

- a synthetic fixture (`internal/ui/uitest/fixture`: invented artists, albums, songs and playlists, generated covers; never Apple's data);
- a fake playback engine, a fake artwork CDN, a Poster the test drains and a fake clock.

The Core is hermetic:

- an empty in-memory keyring;
- an HTTP transport that fails any host the test does not serve (`Harness.Network.Serve` adds one);
- home, XDG and D-Bus environment under the test's temporary directory, so a test never sees your Last.fm session, GitHub or your session bus.

[ui-architecture.md](ui-architecture.md) ("Testing with `ui.Tester`", "Tests") has the API.

The `TestInteraction…` tests (`interaction_test.go` in `kit`, `app` and each feature package) pin what a click, double-click, Enter, Shift+Enter, Space and the Play control do for each kind on each surface; [design.md](design.md#choosing-and-playing-the-interaction-table) is the table they back. A `// DEPARTS:` comment marks a pinned behaviour that breaks the rule and has an issue; change the test with the fix. A change to what choosing or playing an item does changes the table and its test together.

### Golden images

Golden images are few: the shell with Albums, light and dark, at 1132×1253, and the marks contact sheet (below), all in `internal/ui/app/testdata/golden/`. `uitest.Golden` compares by the mean colour of 8×8-pixel blocks. Another machine's text antialiasing passes; a moved edge, a changed colour or a missing cover does not.
Only Linux compares them. A test whose whole job is the picture starts with `if runtime.GOOS != "linux" { t.Skip(…) }` (`TestCollectionsGolden`, `TestFullScreenLyricsGoldens`), so a Mac or a Windows runner does not build and draw the view to skip at the end; `uitest.Golden` skips as well, for the tests that assert something before their picture.

When a change moves the picture on purpose, review it and run `go test ./internal/ui/app -run Golden -update`. Commit the new PNGs and say in the pull request why. You can also ask a maintainer to regenerate them on your pull request.

Pixel readings of a drawn glyph (the sidebar shuffle's centring) are Linux's too: they read Lucide's shape, where macOS draws the system's SF Symbol. The Play triangle's centring runs everywhere, as `theme.symbolTune` places the SF Symbol ([design.md](design.md#icons)). Such a test skips with "pixel readings are asserted on Linux only" (the allow-list in `.github/ci/skips-darwin.txt` takes that reason), after anything it asserts without pixels: the shuffle's button still lies on its artwork on every system.

A failing run keeps the picture and a diff in the test's artifact directory:

- `$SONA_TEST_ARTIFACTS/<test name>` when that variable is set (CI's `.github/ci/test.sh` sets it and uploads the directory);
- else the test's `t.ArtifactDir()` (`go test -artifacts -outputdir DIR`).

Tests get the directory from `internal/ui/uitest/artifacts`, and only when they fail, because `go test` never caches a run that uses `-artifacts`.

Run a golden's test binary inside `devenv shell` (`devenv shell -- go test …`, or `devenv shell -- /path/to/pkg.test`). MyGo draws Linux text with Pango, which it finds through the shell's `LD_LIBRARY_PATH`. A binary started outside it has no text engine: registering the font fails with "this system has no text engine to add fonts to" (the kit's goldens report it on every test), and a picture drawn anyway has boxes, icons and fills but no text at all.

On a Mac, or any machine whose text differs from CI's, update goldens in the CI image. The tag is writable by any workflow of the repository; the build provenance is not, so check it before you run the image (a change of your own to `build/ci` is built as `pr-<tag>` and has none: build it by hand, `docs/maintainers/ci-workflows.md`):

```sh
image=ghcr.io/coredev-uk/sona-ci:$(build/ci/tag.sh)
podman pull "$image"
gh attestation verify "oci://$image" -R coredev-uk/sona-go \
  --signer-workflow coredev-uk/sona-go/.github/workflows/images.yml --source-ref refs/heads/main
podman run --rm -v "$PWD":/src -w /src -e MYGO_GPU=0 "$image" go test ./internal/ui/... -update
```

The image is amd64 and arm64. An arm64 machine (Apple silicon, through Podman's VM) may draw text a hair differently from CI's amd64 runners. CI is the reference, so push and check the run.

### Overlay checks and the marks contact sheet

A flat fixture tile and a golden compared in 8×8 blocks hide a mark that sits one pixel off its cover, so the small marks have two checks of their own.

**`uitest/overlay`.** `internal/ui/uitest/overlay` (beside `uitest`, as `uitest/access` is, so the kit's own tests can use it) asserts where one mark lies against what it sits on. Rectangles are a `ui.Tester`'s (`Find`), in DIPs, and every check allows 1/100 DIP:

- `Same(t, what, overlay, target)`: the overlay's bounds equal its target's ("this wash covers exactly that artwork").
- `OnArtwork(t, what, img, scale, r)`: the same, but the target is found in the picture: the box of the cover's saturated pixels. The fixture's covers are gradients between two hues and the ground, the washes and the glyphs are grey, so the cover is where its colour is, whatever the layout says. Measure with the mark at rest. `ArtBounds` is the box itself.
- `Inside`, `InsetBottomLeft`, `Centred`: a disc inside its cover, 8 DIPs from the corner, in the middle of a button.
- `WholePixels(t, what, r, scale)`: every edge of `r` falls on a device pixel at that scale.

Every overlay that sits on artwork has a test that runs them at scale 1 and 2:

| Overlay | Test |
| --- | --- |
| a track row's play wash and the playing glyph | `kit` `TestTrackRowOverlaysLieOnTheirCover` |
| a tile's play disc | `kit` `TestTilePlayDiscSitsInItsCoversCorner` |
| a home row's play wash and a home tile's play disc | `home` `TestRowOverlaysLieOnTheirCover` |
| the sidebar's shuffle button | `playlist` `TestShuffleButtonLiesOnTheArtworkAndItsGlyphIsCentred` |

A new overlay on artwork gets a case in the test of its package. Covers for these tests are the fixture's or the kit's evidence palette (`paletteArt`), never a flat colour.

**The contact sheet.** `internal/ui/app/testdata/golden/marks-contact-sheet.png` is every small mark that sits on or beside artwork, cropped from the shell over the synthetic library and magnified so one DIP is 4 pixels, with no smoothing, so a half-pixel offset, a lopsided glyph or a corner that does not match its cover shows. Each row is a mark: a tile's play disc (hover and keyboard focus), its release badge, a station's play mark, the Explicit mark, a track row's play wash, playing glyph and focus ring, a select's chevron, the check beside the chosen theme, the player bar's disc and its tooltip, and the sidebar's shuffle button. The columns are light and dark at 1x and 2x (1x is magnified four times, 2x twice). Look at it when you add or change a mark, and review a change to it as you review a golden.

- `go test ./internal/ui/app -run MarksContactSheet` draws it and compares it like a golden; `-update` redraws it after a change on purpose. It runs with reduced motion on, so the playing glyph rests and the picture settles.
- It runs without `-race` only: sixteen renders of the whole shell take about 5 min under the race detector and seconds without, and a picture has no concurrency to check. So `tests` (all `-race`) skips it, and the CI `ui` shards without `-race`, on every pull request and push, draw it. Run the command above after changing a mark.
- A mark is one entry in `sheetRoutes` (`internal/ui/app/marks_sheet_test.go`): the page it is on (the shell's, or a view of the kit's components over the shell's `Env` for what needs Apple's data, such as a catalogue tile), the state to show (hover, focus) and where to crop.
- It is a golden: Linux only, compared in blocks, so another machine's text antialiasing passes.

### The end-to-end smoke

The smoke (`e2e/`, build tag `e2e`) drives the built app from outside. Run the `e2e` script, or `headless -- go test -tags e2e ./e2e/...`.

It builds `bin/sona-e2e` with `-tags probe,sqlite_fts5`. That adds MyGo's automation plugin and a seam that exists only in probe builds:

- `SONA_E2E=1` swaps in the fake playback engine.
- `SONA_E2E_CDN=host:port` sends all the app's HTTP to a local server of the fixture's covers, so nothing reaches Apple.

It starts the app on the fixture profile (`go run ./e2e/profile` writes the same profile into private XDG directories). It checks first paint, Albums, playing an album, Space, a capture and, from the AT-SPI bus, the shell's landmarks by role and name.

- `E2E_OUT=<dir>` keeps the capture (`smoke.png`) and the app's stderr.
- `E2E_BIN=<path>` runs an existing binary.
- `headless` needs `sway`, `wtype` and `at-spi2-core`. devenv has them; elsewhere install them with your package manager.

Never run a probe build outside tests: anything that opens its socket can drive it.

### The Orca walk

The walk (`e2e/orca_test.go`, `e2e/orca_walk_test.go`; the `orca-walk` script) runs a real Orca beside the app in the same headless session. It presses keys with `wtype`, as a keyboard does: Tab and the arrows through the sidebar, the Albums page, an album, the queue, Now Playing, the Seek slider, the account dialog and the first-run setup.

How it reads Orca:

- Orca writes every utterance to its debug file (`--debug-file`) whether or not a speech server is there, so the run needs no audio and no speech-dispatcher.
- A private `orca-customizations.py` makes that file line-buffered, which Orca does not (8 KB blocks).
- The harness reads the log as it grows: only the new bytes, each entry parsed once. `e2e/orca_log_test.go` tests that reader, in the e2e job.
- It takes a key's answer as complete when Orca's event queue is empty and it has said or queued nothing but a slider's value tick for 0.6 s. The longest quiet inside an answer on CI was 0.22 s, so each of the walk's 97 keys costs under a second.

What it checks and keeps:

- The test checks what Orca said against [the accessibility rules](../site/src/content/docs/contributing/accessibility.md).
- `E2E_OUT` keeps `orca-walk.md` and `orca-setup.md` (what each key was answered with, and where Orca put the focus) and Orca's whole log.

How to run it:

- It needs `E2E_ORCA=1` (otherwise it skips) and Orca (`E2E_ORCA_BIN`, else `orca` on the PATH).
- CI runs it only when dispatched (`orca` or `all`), never on a pull request, and it is not a required check: its words and timing are Orca's, so a new Orca can move them.
- The CI job runs in the CI image with Orca (and procps) installed on top, its .debs cached per image. It restores the e2e job's Go caches and saves none.

## Desktop VM tests

`tests/vm` holds NixOS VM tests that run Sona in real GNOME and KDE Plasma sessions (headless QEMU, no network, private mode). They check what the release checklist's desktop rows ask:

- first paint and the title bar's buttons;
- launcher entry and single instance;
- MPRIS in the shell, notifications and the tray;
- X11 (XWayland) and HiDPI scaling;
- `sona private status`.

They need Linux with KVM and Nix (no devenv):

```sh
nix-build tests/vm -A gnome     # or -A plasma; -A all runs every test
nix-build tests/vm -A sona      # the Nix package the VMs install
```

Screenshots land in `result/`. `tests/vm/README.md` lists each check, the checklist rows it covers, the interactive driver, and how to update the package's hashes after a lockfile change.

They never run on pull requests: a CI run takes about an hour of runner time. Start the "VM tests" workflow by hand (`workflow_dispatch`) before a major release, or run them locally.

## Fuzzing

The parsers that read bytes from outside Sona (a CDN, Apple's API, a lyrics provider, a release asset, a file the user picks) have Go fuzz targets, `func FuzzXxx(f *testing.F)` in the package's tests. A target does more than check that the parser does not panic. Its doc comment lists the invariants it asserts, and the target fails on a broken one: allocation bounded by the input (the bounds of F21 and F22 in [security-review.md](security-review.md)), offsets and slices within the buffer, counts consistent with sizes, numbers that do not wrap, and a round trip wherever the package has a writer.

**The seeds run in every test run.** A target's seed corpus is committed under `testdata/fuzz/<Target>/` in Go's corpus format, a few small files each: valid inputs built by the package's test helpers (or taken from its scrubbed `testdata`), and the shape of each advisory that was found in that parser (the file names say which). `go test ./...` runs them as ordinary tests, so `check`, `tests` and CI run them on every change, and a fix for a parser bug keeps its input as a seed.

**The search runs weekly in CI** (`.github/workflows/fuzz.yml`, Sundays 04:41 UTC, and "Run workflow"): one job per target, `go test -fuzz` for two minutes each (the dispatch takes another `fuzztime`). `.github/ci/fuzz-targets.sh` finds the targets with `go test -list '^Fuzz'` in every package that has one, so a new target is picked up without a change to the workflow. A target that finds an input fails its job, and the new input (an untracked file under `testdata/fuzz`) is kept for 14 days as the artifact `fuzz-crashers-<package>-<target>`. No issue is opened: see "When a target finds something".

Run a target yourself:

```sh
go test -run '^$' -list '^Fuzz' ./internal/...                            # every target
go test -run 'Fuzz' ./internal/playback/fmp4/                             # a package's seeds, as tests
go test -run '^$' -fuzz '^FuzzParseInit$' -fuzztime 2m ./internal/playback/fmp4/   # search: one target of one package
```

`-fuzz` takes one target in one package, and `-parallel` limits the workers (the default is `GOMAXPROCS`). The "execs/sec" figure drops to 0 for tens of seconds while the engine minimises a new input; that is not a hang (`-fuzzminimizetime` shortens it). Go keeps what it learns in its build cache (`go env GOCACHE`), and only a failing input is written into the repository.

### When a target finds something

`go test` writes the input to `testdata/fuzz/<Target>/<hash>` and fails; `go test -run '<Target>/<hash>' <package>` reproduces it.

1. **Decide whether a hostile server, file or peer can reach it.** The parsers here read what a compromised CDN, a broken TLS session or a crafted download hands them, so most of what a target finds is reachable. A reachable crash, unbounded allocation or wrong result is a vulnerability: report it as [SECURITY.md](../SECURITY.md) says, and keep the input out of public branches, issues and logs until the fix is released. The failing run of the weekly job, its log and its artifact are public to anyone who can read the repository's Actions.
2. **Fix the parser**, not the target: an invariant that fails is a claim about the parser, and weakening it hides the bug. Change a target only where the invariant itself was wrong, and say why in its comment.
3. **Commit the input as a seed** with the fix, under a name that says what it is (`wrapping-saio-x52q`, not the hash), and add a plain unit test beside the code that states the property in words.

### Writing a target

- Put it in the package's `_test.go` files (`fuzz_test.go`, or beside the tests it shares helpers with), and take its seeds from those tests' builders: nothing personal, nothing downloaded.
- Write what it asserts in its doc comment, as a list, and make the assertions independent of the code under test where you can (re-walk the boxes, re-compute the bound, write the value out and parse it back).
- Keep an execution fast. Where the parser's own limit is large (artwork's 40 megapixels), the target skips decoding past a small size after it has checked the parser's refusal, and says so (`fuzzPixels`).
- Commit three to eight seeds, a few KB each. Go's corpus format is `go test fuzz v1` followed by one line per argument (`[]byte("…")`, `string("…")`, `uint8(3)`); generate the files with a throwaway test and delete it.
- The package must build with the default tags and without cgo, as the weekly job does.

### The targets

| Target | Package | Reads | Asserts |
| --- | --- | --- | --- |
| **Playback and the CDM** | | | |
| `FuzzWalkProto` | `internal/playback/cdm` | protobuf fields of a CRX3 header | no field is longer than its input; nested fields parse or fail |
| `FuzzVerifyCRX3` | `internal/playback/cdm` | a CRX3 package | an accepted archive lies inside the file; every failure is `CodeSignature` |
| `FuzzParseManifest` | `internal/playback/cdm` | Mozilla's Widevine manifest | a release has a dotted-number version, a size of 1 to 64 MiB, a SHA-512 and only HTTPS URLs on Google's hosts, once each |
| `FuzzParseInit` | `internal/playback/fmp4` | an fMP4 init segment | track ID and timescale not 0; kept slices lie in the input; IV size 0, 8 or 16; picture and audio fields in range; the same answer twice; `Priming` not negative |
| `FuzzParseFragment` | `internal/playback/fmp4` | a fragment, with an init (the track's samples among other tracks') | 1 to 65536 samples, of a track some traf names; sample data lies in the buffer; encrypted samples have an IV; subsamples cover their sample, and fit in the buffer; the input is not changed |
| `FuzzMovieDuration` | `internal/playback/fmp4` | the head of an MP4 | a duration is positive, and does not wrap |
| `FuzzLoudness` | `internal/playback/fmp4` | an init's `tlou`/`alou`, a file's `iTunNORM` | loudness and peaks are finite and in the range their coding has; the tag is a part of the input |
| `FuzzParse` | `internal/playback/hls` | a media playlist | segments numbered and timed without gaps or wrapping; byte ranges end within int64 and make a valid `Range` header; keys have a method and a 16-byte key ID; `SegmentAt` stays in range; writing the segments out and parsing them back gives the same ones |
| `FuzzParseMaster` | `internal/playback/hls` | a master playlist | variants have a URI; `BestAAC`, `SelectVideo` and `SelectAudio` return what their documentation says, whatever the bandwidths |
| `FuzzParseAttributes` | `internal/playback/hls` | an attribute list | writing the list out quoted and parsing it back gives the same list |
| **Artwork and motion artwork** | | | |
| `FuzzFlatten` | `internal/motion` | the fragmented MP4 of a motion rendition (one file, or an init and a segment) | the output is a plain MP4 whose sample table locates every sample; the samples are at most the input's size; the timeline starts at 0; the output is bounded by the input; the same output twice |
| `FuzzParsePlaylists` | `internal/motion` | a master or media playlist | `choose` returns the rendition its rules name; a media playlist with a key is refused, and one that is read lists at least one file |
| `FuzzDecodeImage` | `internal/artwork` | a source image (JPEG or PNG) | a header with no area or over the pixel limit is refused with `ErrTooManyPixels` before any pixel is allocated; a decoded picture has the declared size and can be scaled |
| `FuzzPrepareCover` | `internal/artwork` | an uploaded cover | a refusal is `ErrCoverImage`; a cover is a JPEG that decodes, square, at most 1024 px and at least 300, and is itself a valid cover |
| `FuzzExifOrientation` | `internal/artwork` | a JPEG's Exif header | an orientation of 1 to 8 |
| **Data parsers** | | | |
| `FuzzParseLRC` | `internal/lyrics` | LRC text (offset, enhanced word tags, plain text) | lines in time order; times within 0–24 h; output bounded by the input; word timing means words on every line |
| `FuzzFromAppleTTML` | `internal/lyrics` | Apple TTML mapped for the UI | known timing; times in range; line and word counts bounded by the document; the document kept |
| `FuzzLRCLIBRecord` | `internal/lyrics` | LRCLIB JSON | success means lyrics with text; times in range; output bounded |
| `FuzzNetEaseLyric` | `internal/lyrics` | NetEase lyric JSON (with translation and romanisation) | the same; no credit lines |
| `FuzzMusixmatchMacro` | `internal/lyrics` | Musixmatch macro answer (richsync, subtitles, lyrics) | known timing; times in range; output bounded |
| `FuzzDecodeMapResponse` | `internal/applemusic` | normalised amp-api responses and editorial nodes | references resolve to at most as many nodes; nodes keep valid raw JSON and round-trip; a typed decode of the wrong shape errors |
| `FuzzDecodeResourcePage` | `internal/applemusic` | `Page[T]` bodies from `AMPResource` | no data is `ErrNotFound`; otherwise the first element, with no error |
| `FuzzParseTTML` | `internal/applemusic` | lyrics TTML | times within 0–24 h; the document kept; localisation lookups return the right kind |
| `FuzzParseBundleModule` | `internal/i18n/applestrings` | Apple's string bundle (a JS module, never run) | a bundle or an error, never both; entries bounded; valid UTF-8; the same answer twice |
| `FuzzLocaleBundles` | `internal/i18n/applestrings` | the index script's locale table | every URL is the given origin, `/assets/` and a bare file name |
| `FuzzMessages` | `internal/i18n/applestrings` | Apple message text into Sona's ICU messages | Apple's text cannot add an ICU argument or unbalance braces |
| `FuzzParseExport` | `internal/config` | a settings export | only the named import errors; an accepted file leaves local settings alone, normalises idempotently and exports back to the same settings |
| `FuzzExportRoundTrip` | `internal/config` | any `Settings` JSON, exported then imported | `Export` never fails; the document has no local keys; the import is the normalised settings plus the local ones |
| `FuzzParseManifest` | `internal/update` | `update-<target>.json`: decoded, checked, its signed statement verified | accepted means the signed version, digest and signature, a usable URL and size, and the right channel or tag; never accepted under another key |
| `FuzzVerifyStatement` | `internal/update` | a statement's signature fields | nil means exactly the signed version, target, digest and signature; errors are `ErrSignature`; nothing verifies under another key |
| `FuzzParseReleaseInfo` | `internal/update` | `release.json` | nil means exactly the signed record; nothing verifies under another key |
| `FuzzReleaseTag` | `internal/update` | GitHub's `releases.atom` | a tag returned is a version with no `/`, `?` or `#` |
| `FuzzCheckArchive` | `internal/update` | an update's gzip tar (in `releasesign`) | errors are `ErrArchive`; an accepted archive has only safe names and links, no links on Windows, and the right layout |
| `FuzzAnonymiseText` | `internal/diag` | the diagnostics string pass | no e-mail, Apple ID, URL query, fragment or userinfo, API-path query or home folder in the output; idempotent; bounded growth |
| `FuzzAnonymiseValue` | `internal/diag` | one value under a key | identifying keys give a stable `label~hex6`; other kinds leak nothing and are idempotent |
| `FuzzAnonymiseLogLine` | `internal/diag` | a log line | a JSON record stays an object with no leaking string value; any other line is the string pass's output |

Details: [security-review.md](security-review.md), "Fuzzing"; the workflow: [maintainers/ci-workflows.md](maintainers/ci-workflows.md).

## Secret scanning

CI runs [gitleaks](https://github.com/gitleaks/gitleaks) on every pull request and every push to `main` (`.github/workflows/secret-scan.yml`). It scans every commit of every branch and tag, your pull request's own commits included, and the pull request's title and body.

`.gitleaks.toml` adds Sona's rules to gitleaks' defaults:

- JWTs (the Apple developer token);
- Music User Tokens;
- the Last.fm key and secret;
- the Discord application ID;
- credential files (`.env`, `.p8`, `.pem`, keys, keystores).

The public log shows only each finding's rule, commit, file and line.

- Run it before pushing: `nix run nixpkgs#gitleaks -- git --config .gitleaks.toml --redact .` (CI pins gitleaks 8.30.1).
- Test fixtures that need token-shaped input use obviously fake values. An allowlist entry in `.gitleaks.toml` names the file and the exact fake value, with a comment saying why it is fake.
- A commit that ever held a real secret stays public through its pull request even after a squash merge or force-push. Rotate the secret first, then tell the maintainer.

Details: [security-review.md](security-review.md), "Secret scanning".
