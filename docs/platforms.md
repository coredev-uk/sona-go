# Platforms

How Sona builds and runs on Linux, macOS and Windows: the platform seams, build tags, type-checking, the title bar and the macOS setup. [architecture.md](architecture.md) §9 has the design.

## Contents

- [Platform support](#platform-support)
- [Platform seams](#platform-seams)
- [Build tags](#build-tags)
- [Per-OS conventions](#per-os-conventions)
- [Type-check and cross-build](#type-check-and-cross-build)
  - [Type-check every package](#type-check-every-package)
  - [Type-check the macOS CDM host from Linux](#type-check-the-macos-cdm-host-from-linux)
  - [Cross-build Windows from Linux](#cross-build-windows-from-linux)
- [Title bar and window controls](#title-bar-and-window-controls)
  - [Who draws the title bar](#who-draws-the-title-bar)
  - [Check the title bar](#check-the-title-bar)
- [macOS](#macos)
  - [Environment](#environment)
  - [What works from source](#what-works-from-source)
  - [The sandbox](#the-sandbox)
  - [Set up from a clean clone](#set-up-from-a-clean-clone)
  - [Check the CDM](#check-the-cdm)
  - [Check the DRM path](#check-the-drm-path)
  - [See the first run again](#see-the-first-run-again)

## Platform support

Sona ships on Linux, macOS and Windows, and all three play through the CDM host. Every OS-specific part sits behind a seam that reports itself unsupported where a platform lacks it, and the UI hides or explains that control.

**Windows** takes:

- cgo, the official GStreamer MinGW package and the CDM shim DLL (`build/windows/install-gstreamer.sh` and `build/windows/build-shim.sh`, both used by `.github/actions/setup-windows`);
- the shim DLL beside the executable, or at `SONA_CDM_SHIM` in a development or test build;
- the GStreamer `bin` directory on `PATH` in a development build.

The host child runs in an AppContainer. `SONA_CDMHOST_ALLOW_UNSANDBOXED=1` lets a test binary run it unconfined. `cmd/playspike` works there too (`-sink fakesink` on a computer with no audio device).

**Motion artwork** plays through the platform where there is one:

- Windows uses Media Foundation, with MyGo's `ui/video`. There is no cgo, no GStreamer and no video plugin in a package: `package.sh` and `smoke.sh` fail if one is.
- macOS is the same with AVFoundation.
- Linux keeps Sona's own GStreamer decoder (`motion_gst.go`).

`internal/ui/media`'s tests make their fixture clip with the installation's `gst-launch-1.0` (on `PATH`) and play it through `ui/video`.

## Platform seams

In `cmd/sona`, `main.go` is portable. `platform_linux.go` and `platform_other.go` build what differs per OS:

- the `sona <command>` transport and single instance (`internal/app`): D-Bus on Linux, a socket in the state folder on macOS and Windows (see the [command line reference](../site/src/content/docs/reference/cli.md));
- the app options;
- `services.Platform`: media controls, notifications, the sleep inhibitor, the on-battery monitor, the autostart entry, the file chooser, the title bar facts and the tray icon (and its template on macOS).

Details of those:

- Media controls are MPRIS on Linux. On macOS and Windows they are MyGo's NowPlaying through `internal/nowplaying` (`nowplaying_desktop.go`), never linked on Linux.
- The tray itself is the UI's (`internal/ui/app`, `mygo.NewTray`), reported through `services.Windows.HasTray`.
- The playback engine and the CDM host child hook (`newEngine`, `childMain`, `cdmLoadCheck`) are `engine_cdm.go`'s on Linux and in cgo builds for macOS and Windows: the CDM host engine, loading the downloaded CDM. Elsewhere (a cgo-off type-check build, other systems) they are `engine_other.go`'s (`playback.Unsupported`, and no download).

A nil seam is unsupported. The service using it does nothing or returns an error wrapping `errors.ErrUnsupported` (error code `unsupported`). `PlatformService.Capabilities` tells the UI, which hides or explains the control (`internal/services/doc.go`, [ui-architecture.md](ui-architecture.md)).

## Build tags

- **One OS.** A file is named `_linux.go`, `_darwin.go` or `_windows.go`; Go applies the constraint from the name. The fallback for the rest is `_other.go` with the complementary `//go:build` line (`!linux`).
- **A group of systems.** The file carries an explicit line and a descriptive name: `//go:build unix` (`internal/discord/ipc_unix.go`) or `//go:build !darwin && !windows` (XDG paths in `internal/config/paths_xdg.go`, the Secret Service backend name in `internal/auth/store/keyring_other.go`).
- **Tests** that only make sense on one OS follow their file.
- **Code that only compiles on some systems is tagged.** The CDM host engine (`internal/playback/gst`, `widevine`, `cdmhost`, `native`) and `cmd/playspike` carry `//go:build linux || (cgo && (darwin || windows))`: cgo against GStreamer and the C++ shim (on Windows, the MSVC-built shim DLL).
- **MyGo** is pure Go on every OS (it loads GTK by soname on Linux), so the UI packages need no tags. The exception is the motion artwork decoder in `internal/ui/media`: `motion_gst.go` (`linux && cgo`), `motion_native.go` (macOS and Windows: AVFoundation and Media Foundation through MyGo's `ui/video`) and `motion_none.go` for the rest.
- **Portable Go implementations of Linux protocols** stay untagged, so they type-check and their tests run everywhere. These are the D-Bus clients in `internal/mpris`, `internal/desktop/*`, `internal/power` and `internal/app`, and the XDG autostart entry. Only `platform_linux.go` uses them.

Inside the tagged packages, what differs is in `_linux`, `_darwin` and `_windows` files:

| What | Files |
| --- | --- |
| The sink and the output device | `gst/output_<os>.go` |
| The device list (PipeWire's device provider; Core Audio's devices and its default-output watch; WASAPI render endpoints from Core Audio's `IMMDeviceEnumerator`, without GStreamer) | `gst/devices_<os>.c` |
| Whether the chosen WASAPI endpoint can play, and the endpoint watch that moves the output when it leaves or comes back (MMDevice API, inside the CDM host's AppContainer) | `gst/route_windows.c` |
| PipeWire's relink detection (a stub on macOS and Windows, which have no sound server that relinks streams) | `gst/relink_<os>.c` |
| The add-on sandbox | `addons/sandbox/linux*.go` (the init that Sona's own executable becomes, `sandbox.Main`), `darwin.go` with the portable `seatbelt.go`, and `windows.go` over `internal/appcontainer` |
| The sandbox | `cdmhost/sandbox_linux.go`, `sandbox_fs_linux.go`, `sandbox_darwin.go` with the portable `seatbelt.go`, and `sandbox_windows.go`, `appcontainer_windows.go`, `sandbox_remove_windows.go`, over `internal/appcontainer` (the AppContainer, its access grants and the job object, with the portable `acl.go`) |

## Per-OS conventions

| What | Linux | macOS | Windows |
| --- | --- | --- | --- |
| Config paths (`internal/config`) | XDG | `~/Library/...` | `%APPDATA%`, `%LOCALAPPDATA%` |
| Keyring backend (`store.SystemKeyring`) | Secret Service (go-keyring) | Keychain (Security.framework; items only Sona may read without a prompt) | Credential Manager (go-keyring) |
| Discord IPC | Unix socket owned by the user | Unix socket owned by the user | Named pipe `\\.\pipe\discord-ipc-N` whose server runs as the user |
| Track notifications | `org.freedesktop.Notifications` (`internal/desktop/notify`) | MyGo: UNUserNotificationCenter, from Sona.app only | MyGo: notification-area balloons, shown as toasts |
| Start at login | XDG autostart entry (`app.XDGAutostart`, passes `--hidden`) | MyGo: SMAppService login item, from Sona.app only | MyGo: the user's `Run` key, which Task Manager can turn off |
| Stay awake while playing | logind `sleep` inhibitor, portal fallback (`internal/desktop/inhibit`) | MyGo: `Power.KeepAwake`, an IOPMAssertion | MyGo: `Power.KeepAwake`, a `SystemRequired` power request |
| Keep the screen on in Full-Screen Lyrics | MyGo: `Power.KeepAwake` with the display, through the XDG portal's `Inhibit` (suspend and idle), else `org.freedesktop.ScreenSaver`; a desktop with neither holds nothing | MyGo: `Power.KeepAwake` with the display (`IdleDisplaySleepDisabled`) | MyGo: `Power.KeepAwake` with the display (`PowerRequestDisplayRequired`) |
| On battery (motion artwork) | UPower (`internal/power`) | MyGo: `Power.IsOnBattery`, polled | MyGo: `Power.IsOnBattery`, polled |
| File chooser (settings export and import) | xdg-desktop-portal's FileChooser (`app.Portal`) | MyGo: `Dialog`, NSOpenPanel and NSSavePanel | MyGo: `Dialog`, the shell's IFileOpenDialog and IFileSaveDialog |
| Open Apple Music links in Sona | `xdg-mime` default for the desktop entry (`app.XDGLinkHandler`) | NSWorkspace, from Sona.app only (`linkhandler_darwin.go`) | The user's URL classes, unless their choice in Windows' settings or a packaged app holds a scheme (`app.WindowsLinkHandler`) |
| Window material and appearance (`internal/ui/app/material_darwin.go`, `material_windows.go`, `material_other.go`) | None | Liquid Glass (macOS 26+) or the sidebar material behind the sidebar, the app's appearance following Sona's mode, and the mini player's rounded corners ([ui-architecture.md](ui-architecture.md), "Window materials") | Mica (Windows 11 22H2+, build 22621) behind the sidebar, the app's appearance following Sona's mode; none before |
| Audio sink | `pipewiresink` | `osxaudiosink` | `wasapi2sink` |
| A chosen output that is gone | PipeWire links the stream to the default (`target.object`) until it is back | The output is rebuilt on the default output, and back on the device when it returns (Core Audio's device list) | The output is rebuilt on the default (wasapi2sink with no device, which Windows moves with the default), and back on the endpoint when it returns (the MMDevice endpoint watch) |
| Audio thread priority (Diagnostics → Audio priority) | The parent asks RTKit or the Realtime portal for the host's PipeWire threads (`cdmhost/realtime_linux.go`) | Not asked for or measured: Core Audio's I/O thread is realtime already | `wasapi2sink`'s render thread registers with MMCSS itself; the parent reads its base priority (`cdmhost/realtime_windows.go`) |
| CDM library | `libwidevinecdm.so` | `libwidevinecdm.dylib` | `widevinecdm.dll` |

On Linux, the `.deb` and `.rpm` (not the Arch package or the AppImage) install an AppArmor profile, `/etc/apparmor.d/sona` (`build/linux/apparmor/sona`), that lets `/usr/bin/sona` create the user namespaces the playback host's sandbox needs; Ubuntu 23.10 and later refuse them otherwise (`kernel.apparmor_restrict_unprivileged_userns=1`). `postinstall.sh` loads it and `preremove.sh` unloads it. An AppImage cannot carry a profile (its mount point changes), so on Ubuntu its playback host lacks the private filesystem view and, below Landlock ABI 9, can reach the session bus: Diagnostics says so ([security-review.md](security-review.md#f12-on-ubuntu-the-cdm-host-child-could-reach-the-session-bus)).

## Type-check and cross-build

### Type-check every package

Every package, tests included, must type-check for `windows/amd64` and `darwin/arm64` with cgo off. CI and `check` do both.

- Windows uses `go vet`.
- macOS uses `go run ./cmd/typecheck -os darwin -arch arm64 ./...`, which type-checks Sona's packages from source and only names any third-party package that needs cgo.

Real builds need cgo for `mattn/go-sqlite3` everywhere. MyGo is pure Go on macOS and Windows.

### Type-check the macOS CDM host from Linux

The macOS CDM host engine builds only with cgo, so the cgo-off type-check skips it. After touching it, type-check it with cgo on from Linux, with zig as a macOS C compiler.

Linux's GStreamer headers lack the macOS-only `gst/gstmacos.h` that `gst/gst.h` includes on Apple targets. Take it from the same GStreamer's source:

```sh
zig=$(nix build --no-link --print-out-paths nixpkgs#zig)/bin/zig
inc=$(mktemp -d) && mkdir -p "$inc/gst"
src=$(nix build --no-link --print-out-paths "github:NixOS/nixpkgs/$(sed -n 's|.*github:NixOS/nixpkgs/||p' devenv.yaml)#gst_all_1.gstreamer.src")
tar -xOf "$src" --wildcards '*/gst/gstmacos.h' > "$inc/gst/gstmacos.h"
devenv shell -- env CGO_CFLAGS="-I$inc" CC="$zig cc -target aarch64-macos" \
  CXX="$zig c++ -target aarch64-macos" go run ./cmd/typecheck -os darwin -arch arm64 -cgo ./...
```

It compiles the C and C++ (shim, GStreamer glue) for macOS and type-checks every Sona package, `main` included. Third-party Objective-C would still fail without Apple's SDK.

### Cross-build Windows from Linux

A real Windows build cross-compiles from Linux with zig as the C compiler (`nix shell nixpkgs#zig`):

```sh
GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC='zig cc -target x86_64-windows-gnu' \
  go build -tags mygo_noinspector,sqlite_fts5 \
  -ldflags '-H windowsgui -X github.com/egoist/mygo.production=1' ./cmd/sona
```

zig ignores the `-mwindows` Go passes for `-H windowsgui`. Wrap it to pass `-Wl,--subsystem,windows` instead, or the exe opens a console.

## Title bar and window controls

### Who draws the title bar

`internal/windowchrome` decides who draws the main window's title bar ([design.md](design.md), "Title bar"). The shell (`internal/ui/app/shell.go`, `chrome`) sets MyGo's window options from it as the window is created.

| Platform | Result |
| --- | --- |
| macOS | Traffic lights inset into Sona's top bar (a hidden title bar with `TrafficLightPosition`). |
| Windows | A hidden title bar under Sona's top bar, with Windows' caption buttons. |
| Stacking Linux desktops (`XDG_CURRENT_DESKTOP` and friends) | A hidden title bar under Sona's top bar, with MyGo's window buttons in the desktop's layout. MyGo reads and follows GTK's `gtk-decoration-layout`. |
| Tiling compositors | The desktop's frame. Sona detects them by `SWAYSOCK`, `HYPRLAND_INSTANCE_SIGNATURE`, `I3SOCK`, `NIRI_SOCKET` or the desktop name. |
| Unknown desktops | The desktop's frame. |
| KDE Plasma on Wayland (`WAYLAND_DISPLAY` set, `GDK_BACKEND` not `x11`) | The desktop's frame: KWin decorates a window whose title bar is hidden, so a hidden one would sit under KWin's own. |
| KDE Plasma on X11 | A hidden title bar. |

Settings → General → Title bar overrides the choice from the next start. "sona-no-buttons" hides MyGo's buttons too. Sona opens no window menu on a right click.

### Check the title bar

**Linux, tiling.** Run Sona in sway: the desktop's frame, no hidden title bar.

1. Start headless sway: `WLR_BACKENDS=headless WLR_LIBINPUT_NO_DEVICES=1 sway`.
2. Run the app with `WAYLAND_DISPLAY`, `dbus-run-session` and private XDG dirs.

**Linux, stacking.** A headless GNOME Shell needs no system changes (`nix build nixpkgs#gnome-shell`).

1. Start it:

   ```sh
   dbus-run-session -- env GSETTINGS_BACKEND=keyfile gnome-shell --headless \
     --wayland --no-x11 --virtual-monitor 1600x1000 --wayland-display=gnome-0
   ```

2. Run the app on `WAYLAND_DISPLAY=gnome-0` with `XDG_CURRENT_DESKTOP=GNOME` and the tiling variables unset.
3. To try layouts, set `gtk-decoration-layout` in the app's `$XDG_CONFIG_HOME/gtk-3.0/settings.ini`: `appmenu:close` (GNOME's default) or `close,minimize,maximize:`.
4. Check the buttons, a double click on the top bar (maximise and restore), dragging, resizing from the edges and corners, and Close against the close action.

Two D-Bus helpers: the `OverviewActive` property of `org.gnome.Shell` leaves the overview, and `org.gnome.Shell.Screenshot` takes screenshots from a caller owning `org.freedesktop.impl.portal.desktop.gnome`.

**Windows.** Run the zig cross-build (see [Cross-build Windows from Linux](#cross-build-windows-from-linux)) under wine 11 with a Windows 7 prefix, in a wine virtual desktop: `wine explorer /desktop=sona,1600x1000 sona.exe`. The caption buttons sit at the top right, flush with the edges.

**macOS.** On a Mac only. Check:

- the traffic lights centred on the top bar's height beside the sidebar's mark;
- the top bar and the sidebar's header dragging the window;
- a double click zooming, and full screen;
- the menu bar item and its menu, in title case: "Not Playing", "Private Listening";
- the menu bar: Go lists the pages and Back and Forward, every item shows its key, a rebinding in Settings → Shortcuts changes it, and a key shown in a menu runs once;
- closing to the background, which the Dock icon or the menu bar item reverses;
- the mini player.

## macOS

### Environment

`devenv shell` works on Apple Silicon (`aarch64-darwin`).

- The Linux-only packages (GTK3 and the GL libraries, PipeWire, playerctl, gcc, dbus) and `LD_LIBRARY_PATH` are left out. MyGo draws through AppKit there.
- GStreamer keeps core, base, good (`osxaudiosink`), bad and libav, on the same plugin path as on Linux.
- Intel Macs (`x86_64-darwin`) are not supported: the pinned nixpkgs (26.11) and devenv 2.4 dropped them.

devenv sets one deployment target: `MACOSX_DEPLOYMENT_TARGET` is nixpkgs' (14.0). `GOFLAGS` adds `-ldflags=-extldflags=-mmacosx-version-min=14.0`, which overrides the 10.13 that some cgo directives pass to the linker. Without it, ld warns "object file … was built for newer macOS version (14.0) than being linked (11.0)" for every cgo object.

A `go build -ldflags=…` of your own replaces GOFLAGS' `-ldflags`. Add `-extldflags=-mmacosx-version-min=$MACOSX_DEPLOYMENT_TARGET` to it (`build/darwin/Taskfile.yml` does).

### What works from source

The app builds and runs on macOS from source as an unsigned `bin/sona`: no Apple Developer account and no `.app` bundle. The release's `Sona.app` and `.dmg` come from `build/darwin/bundle.sh`, with GStreamer bundled.

From source you get:

- the UI and everything Go does (sign-in, library, sync, catalog);
- playback through the same CDM host as Linux, with the CDM Sona downloads from Google for macOS (`mac_arm64`, see [development.md](development.md), "Widevine CDM");
- Core Audio output (`osxaudiosink`): the device chosen in Settings → Playback → Output, else the system's default output, followed when it changes;
- media keys and Now Playing (`internal/nowplaying`);
- the sleep inhibitor (MyGo's `Power.KeepAwake`: an IOPMAssertion that `pmset -g assertions` lists);
- App Nap kept off while music plays or loads (`Power.KeepRunning`, which does not keep the Mac awake and needs no setting);
- the power source for "on battery" (`Power.IsOnBattery`, polled);
- the file chooser for the settings export and import (`Dialog`);
- `sona <command>` and single instance, over a socket in Sona's state folder (see the [command line reference](../site/src/content/docs/reference/cli.md)).

Apple's licence server grants licences to this unsigned host: full tracks play on Apple Silicon.

Notifications and Start Sona when you log in need an app bundle:

- Notifications use UNUserNotificationCenter: a title, a text and a click, no artwork or buttons. Start at login uses SMAppService.
- The bundle must be in a folder macOS lists (`/Applications` or `~/Applications`). From `/tmp` it refuses the notifications.
- Use the release's `Sona.app`, or wrap a development build: `build/darwin/devbundle.sh bin/sona-dev ~/Applications/SonaDev.app`.
- Run `SonaDev.app/Contents/MacOS/sona` inside the devenv shell. It has its own identifier, and a login item you turn on is listed in System Settings → General → Login Items & Extensions until you turn it off again.
- A bare `bin/sona` has neither, and Settings says so ("Only when Sona runs from Sona.app").

### The sandbox

The CDM host child runs under macOS's sandbox: `sandbox-exec` with Sona's profile. It allows no network and no writes outside the temporary folders, and keeps Sona's folder and the keychain out of reach ([security-review.md](security-review.md), "macOS sandbox").

Where `/usr/bin/sandbox-exec` is missing or refuses the profile, the child runs unsandboxed. Diagnostics → Host sandbox says "Not sandboxed" and why. `SONA_SEATBELT=0` turns the sandbox off for one run, to tell a sandbox problem from a playback one.

### Set up from a clean clone

You need Nix (from the Determinate or the upstream installer) and devenv 2.4 or newer.

```sh
git clone https://github.com/coredev-uk/sona-go.git && cd sona-go
devenv version                       # 2.4.0 or newer
devenv shell                         # the first run downloads the toolchain
# inside the shell:
gst-inspect-1.0 osxaudiosink avdec_aac | grep -E '^  (Name|Long-name)'
go vet ./... && go test ./...
SONA_PRIVATE=1 dev                   # a development build, started
```

`build` writes `bin/sona` for macOS; `task darwin:build` makes the `-trimpath` build the packages use (`build/darwin/Taskfile.yml`). `SONA_PRIVATE=1 ./bin/sona` runs it.

Expect:

- the window with every page;
- Settings without the controls macOS can't support from a bare binary (start at login and notifications, which need Sona.app);
- the keyring backend "Keychain";
- data under `~/Library/Application Support/Sona` and `~/Library/Caches/Sona` (log: `State/sona.log`);
- the D-Bus tests skipping, because there is no `dbus-daemon`.

### Check the CDM

Sona and `playspike` download the CDM themselves; no browser is needed. To check the download and the library by hand:

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

### Check the DRM path

The DRM path regression and licence check needs `.env` holding `MUSIC_USER_TOKEN`. Always run it in private mode. It uses the downloaded CDM, and audio plays on the default output, or on `-device <uid>`: a Core Audio device UID from the `output` lines playspike prints once playback starts.

```sh
SONA_PRIVATE=1 go run ./cmd/playspike -seek 90 -gap=false
SONA_PRIVATE=1 go run ./cmd/playspike -activity   # only with SONA_PRIVATE=1
```

It prints:

- `cdm source source=download`, with the library under `~/Library/Application Support/Sona/widevine/`;
- each `licence exchange`: HTTP status, Apple's status and message, and the licence's length (never the token or the licence);
- a last line of `licence: granted`, `licence: refused (<why>)` or `licence: not requested (<why>)`.

A stream station (Apple's live radio, or a local broadcaster such as KISS, `ra.1445918457`) plays with `-station` for `-listen` (default 12s). Each change of what its stream says is on air is printed as playback reaches it. It fails unless the station is still playing and played at least nine tenths of that time:

```sh
SONA_PRIVATE=1 go run ./cmd/playspike -station ra.1445918457 -listen 2m
```

### See the first run again

To see a computer's first run again (the CDM download, its load check and GStreamer's first plugin scan), quit Sona and remove the downloaded CDM and GStreamer's registry, then start it:

```sh
rm -rf ~/Library/Application\ Support/Sona/widevine
rm -f "${XDG_CACHE_HOME:-$HOME/.cache}"/gstreamer-1.0/registry.*.bin   # or $GST_REGISTRY_1_0 when set
SONA_PRIVATE=1 dev
grep -E 'widevine:|audio stack' ~/Library/Application\ Support/Sona/State/sona.log
```

The log shows `widevine: downloading`, `package verified`, `load check passed` with its `took` (the CDM alone) and `installed`. Separately it shows `playback: audio stack ready` with the plugin scan's `took`.
