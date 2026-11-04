# Release checklist

Releases come from release-please ("Releasing" below). Work through §1 and
§2 on the release PR's branch before merging it. Automated gates come first;
the desktop matrix needs real sessions (GNOME and KDE in VMs, Hyprland on
real hardware). Record each run in the results table at the end: date, Sona
version, distro/session, result, and notes for anything not passed.

## Releasing

A release has Linux packages (.deb, .rpm, Arch, AppImage), a macOS app for
Apple silicon (`.dmg`) and Windows x64 builds (a per-user installer and a
portable `.zip`), plus the signed update manifests that macOS and Windows
installs update from ("Platforms in a release" below).

`.github/workflows/release-please.yml` runs on every push to `main` while
the repository is public (it skips itself while private). release-please
reads the Conventional Commits subjects merged since the last release (PRs
are squash-merged, so these are the PR titles) and keeps one release PR,
`chore(main): release X.Y.Z` on `release-please--branches--main`, which bumps
`VERSION`, `CHANGELOG.md` and `.release-please-manifest.json`. Settings live
in `release-please-config.json`; the action is pinned by SHA and Renovate
updates it. Its dry run is the CLI version the action bundles (17.6.0 for
release-please-action v5.0.0):
`npx release-please@17.6.0 release-pr --dry-run --debug --repo-url=coredev-uk/sona-go --token="$(gh auth token)"`.

| PR title type                                                                 | Changelog section     | Bump before 1.0                |
| ----------------------------------------------------------------------------- | --------------------- | ------------------------------ |
| `feat`                                                                        | Features              | minor                          |
| `fix`, including Renovate's `fix(deps)`                                       | Bug Fixes             | patch                          |
| `perf`                                                                        | Performance           | patch                          |
| `revert`                                                                      | Reverts               | patch                          |
| `refactor`, `docs`, `test`, `ci`, `build`, `chore` (Renovate's `chore(deps)`) | hidden                | none on their own              |
| `type!:` or a `BREAKING CHANGE:` footer                                       | ⚠ BREAKING CHANGES, first | minor (`bump-minor-pre-major`) |

Hidden types never open a release PR by themselves: Renovate's `chore(deps)`
updates (Actions, development dependencies) wait for the next release, while
`fix(deps)` (direct Go modules, which ship in the binary) open or update a patch release PR. 1.0.0 is a choice, not a bump: set
`"release-as": "1.0.0"` in the package entry of `release-please-config.json`
for that release and remove it afterwards. To reword a merged PR's changelog
line, put `BEGIN_COMMIT_OVERRIDE`, the new Conventional Commits line(s) and
`END_COMMIT_OVERRIDE` in that PR's body; the next run uses them.

Merging a release:

1. Run §1 and §2 against the release PR's head.
2. release-please pushes the release PR as the bot-cecil GitHub App, so CI
   and the PR title check start on it like on any other PR, and re-run on
   every push to its branch. Wait for the three required checks ("Go vet and
   test (-race)", "Conventional Commits title", "gitleaks"; CONTRIBUTING.md,
   "Pull requests"). If a push changed nothing in
   the PR and the checks are missing (the release PR opened before the App
   token was in use), close and reopen it to start them.
3. Merge it through Clarence (`@bot-clarence merge`, AGENTS.md) with its title
   unchanged: release-please finds it by the title and the
   `autorelease: pending` label.
4. The Release Please run for that merge tags `vX.Y.Z` on the merge commit,
   creates a draft GitHub release holding the changelog entry (the PR's
   label becomes `autorelease: tagged`), as bot-cecil, and calls
   `release.yml`:
   - `plan`: the tag must equal `VERSION`.
   - `checks`: the Linux CI jobs (`ci.yml` with `linux-only`: Go vet and
     test, UI tests, E2E).
   - `linux`, `macos` and `windows` (environment `release`, in parallel):
     each builds the platform's files with `VERSION` in `main.version` and
     the file names and smoke-tests them ("Platforms in a release"); the
     build provenance of each job's checksummed files is attested; the files
     are kept as the `linux-release`, `macos-release` and `windows-release`
     run artifacts. `linux` also writes its `SHA256SUMS`.
   - `publish` (environment `release`, as bot-cecil): checks the Linux
     checksums, signs the update manifests, writes one `SHA256SUMS` over
     everything and attests it; the packages, the `.dmg`, the Windows files,
     the update archives and manifests, the AppImage `.zsync`, the AUR
     `sona-bin` files and `SHA256SUMS` are uploaded to the draft
     (`--clobber`) and it is published as `Sona X.Y.Z`, marked latest.
     `releases/latest` never returns a draft, so the update notice,
     AppImageUpdate and the self-updaters only see a release once its files
     are on it.
5. Check a download: `sha256sum -c SHA256SUMS` and
   `gh attestation verify sona_X.Y.Z_amd64.deb --repo coredev-uk/sona-go`
   (any of the files; the attestation is the build's signed provenance, from
   the workflow's OIDC token, not from the App).
6. Copy `aur/PKGBUILD` and `aur/.SRCINFO` from `sona-bin-<v>-aur.tar.gz` into
   the AUR `sona-bin` repo and push there by hand.

If the build fails on something transient, "Re-run failed jobs" on that
Release Please run builds the same tag again; the publish job uploads over
what an earlier attempt attached and leaves an already published release
alone. If it needs a code change, delete only the draft
(`gh release delete vX.Y.Z --yes`, keeping the tag) and merge the fix:
release-please falls back to the `vX.Y.Z` tag as the last release and the
fix ships as the next patch release. Never revert the merged release PR:
`VERSION` and the tag would disagree.

### Dry run

Before merging a release (and after any change to `release.yml`,
`release.sh`, `build/darwin`, `build/windows` or the packaging), build the
packages without publishing:

    gh workflow run release.yml --ref <branch>            # add -f checks=true to run the CI jobs first
    gh run watch                                          # then:
    gh run download <run-id> -n linux-release             # also -n macos-release, -n windows-release

Dispatching `release.yml` with `prerelease` empty builds and smoke-tests the
same files on all three platforms, uploads them as the `linux-release`,
`macos-release` and `windows-release` run artifacts (kept 7 days) and neither
attests, signs nor publishes. It runs without the `release` environment, which only
deploys from `main`, so these builds have no Last.fm or Discord
credentials compiled in. `release-secrets.yml` checks those secrets.

### Platforms in a release

`release.yml` runs `plan`, then `checks` (ci.yml with `linux-only`: the macOS
and Windows development builds in ci.yml never hold a release up), then the
`linux`, `macos` and `windows` jobs in parallel, then `publish`.

| Job       | Runner        | Builds                                                             | Smoke test in the job                                                                 |
| --------- | ------------- | ------------------------------------------------------------------ | ------------------------------------------------------------------------------------- |
| `linux`   | ubuntu-26.04  | the Linux packages (`build/linux/release/release.sh`)              | each package on clean Ubuntu, Fedora and Arch containers                              |
| `macos`   | macos-15      | `Sona-<v>-macos-arm64.dmg`, `sona-<v>-darwin-arm64.tar.gz` (`build/darwin/bundle.sh`) | `build/darwin/smoke.sh`: `--version`, first paint, motion artwork decode and a preview played from the bundle's GStreamer, no `/nix`, `/opt/homebrew` or `/Library/Frameworks` paths, `codesign --verify` |
| `windows` | windows-2025  | `Sona-<v>-windows-x64.zip`, `Sona-<v>-windows-x64-setup.exe`, `sona-<v>-windows-amd64.tar.gz` (`build/windows/package.sh`) | `build/windows/smoke.sh`: the zip's `--version` and first paint; silent install, files and `music:`/`itms:` keys; silent uninstall |
| `publish` | ubuntu-26.04  | `update-darwin-arm64.json`, `update-windows-amd64.json`, `SHA256SUMS` | checks the Linux checksums; attests the manifests and the combined `SHA256SUMS`       |

- **macOS app:** plain Go and Xcode's clang against the official GStreamer
  framework (the version and package hashes are pinned in
  `build/darwin/gstreamer.sha256`; change both together, in the same 1.28
  minor as devenv's). `bundle.sh` copies the framework's libraries and the plugins Sona
  uses into `Sona.app/Contents/Frameworks/GStreamer` (a plain folder: codesign
  takes a `.framework` without an Info.plist, or a folder named like
  `gstreamer-1.0`, for a malformed bundle), thins them to arm64, rewrites
  install names to relative paths, signs ad hoc (`codesign --force --deep -s -`,
  no hardened runtime: the Widevine library must load) and checks nothing
  points at the build machine. The app sets `GST_PLUGIN_SYSTEM_PATH_1_0`,
  `GST_PLUGIN_SCANNER` and `GST_REGISTRY_1_0` at start (`cmd/sona/bundle_darwin.go`).
  The minimum macOS is `build/darwin/MIN_MACOS`, devenv's `darwinMinVersion`
  (keep them equal). To build one on a Mac:
  `build/darwin/install-gstreamer.sh` (sudo, installs under `/Library/Frameworks`;
  or `--extract DIR` for no install and `GSTREAMER_ROOT` pointing into DIR),
  then `build/darwin/bundle.sh bin/macos`, then `build/darwin/smoke.sh bin/macos`
  (build with `SONA_SMOKE_TOOLS=1` for the motion and preview checks). On a Mac
  whose Xcode tools cannot link, build inside devenv with `SONA_DEVENV_BUILD=1`
  (it points devenv's copies of macOS's own libraries back at `/usr/lib`;
  CI never sets it).
- **Windows:** Go and the runner's MinGW-w64 gcc (for `go-sqlite3`) and
  NSIS from Chocolatey; MyGo is pure Go there. Playback is unsupported on
  Windows and so is motion artwork: the Media Foundation decoder is not
  written (it needs a COM H.264 transform driven from syscalls, which cannot
  be tested without a Windows machine), `internal/ui/media` reports no codec,
  and covers show their stills.
- **Update archives:** `sona-<v>-darwin-arm64.tar.gz` holds `Sona.app/` alone;
  `sona-<v>-windows-amd64.tar.gz` holds the install folder's files at the top
  level. Both are what MyGo's updater installs; the `.dmg`, `.zip` and
  installer are for people.
- **Signing the manifests:** `go run ./cmd/releasesign` signs each archive's
  SHA-256 with the Ed25519 key in the `release` environment's secret
  `SONA_UPDATE_KEY` (the file `go tool mygo keygen` wrote; the public key is
  `internal/update/update.pub`, linked into the macOS and Windows builds).
  Manifests are signed last, after the archives are uploaded. **Without the
  secret** (a fork, a run outside the environment) the step is skipped, says so
  in the run summary, and the release has no `update-<target>.json`, so those
  installs do not self-update.
- **Release notes:** a release's draft holds release-please's changelog entry;
  `publish` appends `.github/release-notes/install.md` (first-run steps for
  macOS and Windows, verifying downloads).
- **Dry run and prereleases:** dispatch `release.yml` (`gh workflow run
  release.yml --ref <branch>`). With `prerelease` empty it builds and
  smoke-tests all three platforms and publishes nothing (the files are the
  `linux-release`, `macos-release` and `windows-release` artifacts). With
  `prerelease=rc.1` it builds `v<VERSION>-rc.1` from the branch in the
  `release` environment, signs the manifests and publishes a GitHub
  prerelease, which updates only builds with Settings → Updates → Test
  versions on. The environment's deployment branch rules must allow the
  branch.

### Platform checks on real machines

Before merging a release PR, install the release candidate's files:

- **macOS 14+ on Apple silicon:** the `.dmg` opens; Sona drags to
  Applications; the first open is blocked, then works through "Open Anyway" or
  `xattr -dr com.apple.quarantine /Applications/Sona.app`; `music://` and
  `itms://` links open Sona; sign-in works; a preview plays; motion artwork
  moves; Settings, sign-in and the queue survive a restart.
- **Windows 11 x64:** the installer runs without administrator rights and
  makes a Start-menu entry; SmartScreen's "More info" → "Run anyway" works;
  `music:` and `itms:` links open Sona; the portable zip runs from a folder
  the user can write; the uninstaller removes the app and the URL handlers and
  keeps the user's data.
- **Updates (macOS and Windows):** install rc.N, publish rc.N+1 (dispatch with
  `prerelease=rc.N+1`), turn Test versions on, see the notice, "Install and
  restart", and check settings, sign-in and the queue survive. A deliberately
  broken build rolls back.

### bot-cecil App permissions

release-please and the publish job act as the bot-cecil GitHub App (the same
App Renovate uses), through a token minted by
`actions/create-github-app-token` in each job from the repository secrets
`RENOVATE_APP_CLIENT_ID` and `RENOVATE_APP_PRIVATE_KEY`. The App must be
installed on this repository with at least these repository permissions, all
of which Renovate's token (`renovate.yaml`) already requests:

| Permission     | Level | Used by                                                            |
| -------------- | ----- | ------------------------------------------------------------------ |
| Contents       | write | release-please: release branch, tag, draft release; publish job: upload and publish |
| Pull requests  | write | release-please: open and update the release PR                     |
| Issues         | write | release-please: the `autorelease:` labels on the release PR        |

Each job requests only what it uses (`permission-*` inputs), so the token is
narrower than the installation. The build provenance needs no App
permission: it uses the workflow's `id-token: write` and
`attestations: write`.

### Repository settings release-please needs

Squash merging only, with the commit message defaulting to "Pull request
title" (title `PR_TITLE`, body blank), since the default "commit or PR
title" takes a lone commit's subject instead of the checked PR title.
"Allow GitHub Actions to create and approve pull requests" is not needed:
the App opens the PR. The `release` environment allows branch `main`, the
ref of the release-please run that calls `release.yml`. `release.yml` is
called only by `release-please.yml` (no `push: tags` trigger), so pushing a
tag builds nothing; the `release tags` ruleset keeps `v*` tags immutable.

## 1. Automated gates

- [ ] `VERSION` holds the new version (the release PR writes it); nothing
      else carries it (the build scripts read it: `build/linux/Taskfile.yml`
      injects it into the binary and nfpm, `build/darwin` and
      `build/windows` take it for the app, the file names and the
      installer).
- [ ] CI is green on the release PR's head (`.github/workflows/ci.yml`: "Go vet
      and test (-race)" on Ubuntu with apt GStreamer and user namespaces
      allowed, so the CDM host sandbox tests run, plus the Windows and macOS
      type-checks; "UI tests (goldens)"; "E2E (headless sway)"; and, where
      they run (on dispatch, or on every push once the repository is
      public), "macOS build and test" and "Windows build").
- [ ] Locally in devenv: `go test -race ./internal/playback/...` with the real
      CDM (found by `cdmtest`; `SONA_CDM_REQUIRE=1` so none skip) and `go run ./cmd/playspike -seek 90`.
- [ ] `bash build/linux/release/release.sh` (devenv `release`) passes: builds
      .deb, .rpm, Arch package, AppImage + .zsync, the AUR files and
      SHA256SUMS, and smoke-tests the .deb (Ubuntu 24.04), .rpm and AppImage
      (Fedora 45) and the Arch package through the AUR PKGBUILD (Arch Linux):
      `sona --version` prints the version, every library resolves, the
      GStreamer elements exist (appsrc, avdec_aac, playbin3, h264parse and
      the rest of `smoke.sh`'s list), and the app paints its first frame
      under Xvfb.
- [ ] The dry run ("Dry run") on the release PR's branch passes.
- [ ] After the merge, the release workflow (`release.yml`, called by
      `release-please.yml`) re-runs the same, attests the files and
      publishes the GitHub release; then the checksum and attestation check
      and the AUR push ("Releasing", steps 5 and 6).

## 2. Desktop matrix

Install the package for that distro on a clean VM (or a real Hyprland
session), sign in, and play something. Each check names how to verify it.

The VM column says what the desktop VM tests (`tests/vm`, README there)
check on GNOME 50 and KDE Plasma 6.8 (both Wayland, plus XWayland). They
run for major releases only, since a run takes about an hour of runner time:
locally with `nix-build tests/vm -A all`, or by starting the "VM tests"
workflow by hand. They never run on pull requests.
"Manual" rows need a person or a signed-in, playing session; "part" rows
still need the hand check for what the VM can't show.

| #   | Check | How | VM |
| --- | ----- | --- | -- |
| D1  | Package installs with only its declared dependencies | `.deb` on Ubuntu 24.04 GNOME (`sudo apt install ./sona_*.deb`), `.rpm` on Fedora KDE (`sudo dnf install ./sona-*.rpm`), Arch package or `sona-bin` on Arch; AppImage on any. `sona --version` prints the version | Part: the Nix package, and the AppImage under `appimage-run` (`-A appimage`). .deb/.rpm/Arch: `release.sh`'s container smoke tests |
| D2  | Launcher entry and icon | Sona appears in the app grid / launcher with its icon; the running window groups under it (app id `dev.coredev.Sona`) | Yes: GNOME's app system (name, icon, window's app); Plasma's Kickoff search starts it, KWin class `dev.coredev.Sona` |
| D3  | CDM downloaded | Settings → Diagnostics shows the CDM version and Source "Downloaded from Google"; on Linux arm64 (no CDM from Google) the playback area says songs can't play on this computer | No (offline). The VMs check that a failed CDM check offline leaves Sona running |
| D4  | Full-track playback | Play an album track to the end; seek; next/previous; audio on the default PipeWire/Pulse output | Manual |
| D5  | MPRIS in the shell's media widget | GNOME: the media card in the notification/calendar panel shows title, artist, artwork and working play/pause/next/previous (while a live station plays, next, previous and seek are greyed; on any station shuffle and loop read off). KDE: the Media Player plasmoid / system-tray media controls do the same. Hyprland: `playerctl -p sona metadata`, bar widget. Media keys work | Part: one MPRIS name, metadata, `Quit`; GNOME's media card shows the track (OCR). Plasma's popup, artwork, the buttons with real playback and media keys stay manual |
| D6  | Notifications | Track-change notification appears with artwork (Settings → General → Track notifications on); none while the window is focused if that is the setting | Part: Sona's notification client against GNOME's and Plasma's servers (replace, image, GNOME's default action). The app's own track-change trigger needs playback: manual |
| D16 | Sign-in flows | Native sign-in with 2FA; loopback and token-paste fallbacks reachable | Manual |
| D17 | Large-library sync | An account with a large library syncs fully; lists scroll smoothly; restart shows it from cache | Manual |
| D18 | Queue, stations, live radio | Queue add/reorder/remove; start a song station and a live radio station; both play and advance | Manual |
| D7  | Tray | GNOME needs the AppIndicator extension (state if absent: no tray, close quits or minimises per setting). KDE: tray icon, menu Play/Next/Previous/Show/Mini player/Quit work. Hyprland: in the bar's tray (StatusNotifierWatcher) | Part: the item and its menu are registered and shown (GNOME with the extension, Plasma). Clicking the menu entries stays manual |
| D8  | Single instance across two real launches | Launch from the launcher, then launch again (launcher and `sona music://music.apple.com/…` in a terminal): the second exits at once, the first window comes forward and opens the link; still one window and one `org.mpris.MediaPlayer2.sona` | Yes, except "comes forward and opens the link" (no network to open it) |
| D9  | `music://` links | Clicking a `music://` / `itms://` link in a browser opens it in Sona | Manual |
| D10 | Wayland scaling | Native Wayland session at 100 %, 150 % (fractional) and 200 %: text and artwork sharp, no blurry upscaling, layout fits; move between monitors with different scales | Part: 200 % on GNOME, 200 % and 150 % on Plasma (fits the screen; sharpness by screenshot). Mixed-scale monitors manual |
| D11 | X11 session | GNOME on Xorg / Plasma X11 (or `GDK_BACKEND=x11` under XWayland): window, playback, MPRIS, tray, single instance all work | Part: XWayland on both (window, title bar, MPRIS, tray, single instance). nixpkgs' GNOME 50 and Plasma 6.8 have no X11 session; playback manual |
| D12 | Secrets | Token stored in GNOME Keyring / KWallet (Settings shows Secret Service); sign out removes it; restart stays signed in | Manual |
| D13 | Sleep inhibit | While playing, the session does not suspend on idle (`systemd-inhibit --list` shows Sona); paused, it does | Manual |
| D14 | Restart persistence | Quit and relaunch: signed in, library from cache before any network, last route/queue restored per settings | Part: the saved queue is restored after each start. The rest manual |
| D15 | Uninstall | Removing the package leaves no launcher entry; user data stays in XDG dirs | Manual (NixOS installs declaratively) |

## 3. Results

GNOME and KDE rows run in the desktop VM tests where the VM column says
so; the rest stay open until someone runs them by hand. Add one row per
run, with the evidence for anything not obvious from the check itself.

| Date | Version | Distro / session | Checks | Result | Notes |
| ---- | ------- | ---------------- | ------ | ------ | ----- |

## 4. Known issues

Upstream bugs Sona ships with, tracked here until they are reported and
fixed upstream. Re-check each one on every release. None is open today: the
only entry is historical.

### Blank mini player without a font DPI (WebKitGTK second web view) — historical

> Historical: found in the Wails/WebKitGTK app that preceded the native UI.
> The native app has no web view and draws its mini player itself
> (`features/miniplayer`), so nothing of Sona's is affected any more. The
> record is kept for the upstream report; the entries below describe that
> old app, not a current bug. Drop the section once WebKitGTK is fixed or
> the report is filed.

- **Symptom:** the mini player window opens at its size with its title, but
  shows only the background colour. Its page loads (the buttons are in the
  DOM and respond), but `innerWidth` and `innerHeight` are 0 and
  `devicePixelRatio` is `Infinity`. The main window is fine. First seen on
  sway; it is not sway-specific.
- **Cause:** WebKitGTK scales a web view's page by `fontDPI() / 96`.
  `fontDPI()` is GTK's `gtk-xft-dpi` when set; when it is -1 (no settings
  portal, GSettings or XSettings value, for example `GDK_DEBUG=no-portals`
  or a `dbus-run-session` without xdg-desktop-portal) WebKit falls back to
  the monitor's physical DPI. Headless and nested wlroots outputs report
  0 mm (`wl_output.geometry(0, 0, 0, 0, …)`), so the DPI is infinite and
  the page zoom becomes infinite. The first web view is created before
  WebKit has screen data and keeps 96 dpi, so only later views are hit.
  With a monitor that reports its size the later views are zoomed by
  its DPI / 96 instead (weston headless, 1 mm per pixel: a 640×442 view
  reports 2418×1670 at `devicePixelRatio` 0.26). Unchanged on WebKit
  `main` (2026-11-01: `ScreenManagerGtk.cpp`, `PlatformScreenGtk.cpp`
  `fontDPI()`, `WebKitWebViewBase.cpp` `refreshInternalScaling`).
- **Versions:** WebKitGTK 2.54.1 (ABI 6.0), GTK 4.22.4, sway 1.12, labwc
  0.9.7, weston 15.0.1. nixpkgs-unstable and master also carry 2.54.1.
  Wails is not involved.
- **Repro without Sona or Wails:** `docs/upstream/webkitgtk-second-view/two.c`
  (build and run commands in its header).
- **Results** (headless sway, pixman unless noted; second view's page):
  with `GDK_DEBUG=no-portals` 0×0 every run; also with the GLES renderer,
  output scale 2, both views in one window, the second window created on
  the first one's `map`, `WEBKIT_DISABLE_COMPOSITING_MODE=1`,
  `WEBKIT_DISABLE_DMABUF_RENDERER=1`, and `GSK_RENDERER=gl|ngl|cairo`.
  With the settings portal, or `gtk-xft-dpi=98304` in `settings.ini`, it
  gets its size. labwc behaves like sway; weston zooms as above. Headless
  Hyprland 0.56 did not start here, so the earlier "Hyprland works" result
  (a session with the portal) has no headless control.
- **Workaround (gone):** before creating the mini player, the Wails shell set
  `gtk-xft-dpi` to 96 dpi when GTK has none (`internal/shell/fontdpi_linux.go`, deleted with the shell; MyGo draws its own text, so the native app needs none).
  GTK renders at 96 dpi in that case anyway, so only WebKit's fallback
  changes. Checked with the Wails app under `GDK_DEBUG=no-portals` on headless sway:
  the mini player reports 420×132 at `devicePixelRatio` 1 and renders
  (before: 0×0, `Infinity`, blank).
- **Upstream:** not reported yet; the report text is in the pull request
  that added the repro (`docs/upstream/webkitgtk-second-view/two.c`, build and
  run commands in its header).
