# Platforms

How Sona builds and runs on macOS, Windows and Linux: the seams, the build tags, type-checking, the macOS setup and the title bar. `docs/architecture.md` §9 has the design.

## Platform seams and build tags

Linux is the full platform. macOS and Windows build and run the UI, with
every OS-specific part behind a seam that reports itself unsupported where it
isn't implemented; both also play through the CDM host. On Windows that takes
cgo, the official GStreamer MinGW package and the CDM shim DLL
(`build/windows/install-gstreamer.sh`, `build/windows/build-shim.sh`, both used
by `.github/actions/setup-windows`); the shim DLL is found beside the
executable, or at `SONA_CDM_SHIM` in a development or test build, and a
development build needs the GStreamer `bin` directory on `PATH`. The host
child runs in an AppContainer (`SONA_CDMHOST_ALLOW_UNSANDBOXED=1` lets a test
binary run it unconfined); `cmd/playspike` works there too (`-sink fakesink`
on a computer with no audio device). Motion artwork plays there through
Media Foundation, with MyGo's `ui/video` (no cgo, no GStreamer, no video plugin in a package: `package.sh` and
`smoke.sh` fail if one is); `internal/ui/media`'s tests make their fixture clip
with the installation's `gst-launch-1.0` (on `PATH`) and play it through
`ui/video`. macOS is the same with AVFoundation; Linux keeps Sona's
own GStreamer decoder (`motion_gst.go`).

- In `cmd/sona`, `main.go` is portable; `platform_linux.go` and `platform_other.go` build
  what differs per OS: the `sona <command>` transport and single instance
  (`internal/app`: D-Bus on Linux, a socket in the state folder on macOS
  and Windows; site/src/content/docs/reference/cli.md), the app options, and `services.Platform`:
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

## Title bar and window controls

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
- Windows: the zig cross-build (see "Platform seams and build tags") under wine 11 with a Windows 7 prefix
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

## macOS

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
macOS (`mac_arm64`, see `docs/development.md`, "Widevine CDM") and Core Audio output
(`osxaudiosink`: the device chosen in Settings → Playback → Output, else
the system's default output, followed when it changes). Media keys and Now
Playing work (`internal/nowplaying`), and so do the sleep inhibitor (MyGo's
`Power.KeepAwake`: an IOPMAssertion `pmset -g assertions` lists), App Nap
kept off while music plays or loads (`Power.KeepRunning`, which does not keep
the Mac awake and needs no setting), the power
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
Sona's state folder (site/src/content/docs/reference/cli.md). Apple's licence server grants licences to
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

