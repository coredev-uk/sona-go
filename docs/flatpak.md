# Flatpak

## Verdict

**Viable, with conditions.** On the GNOME 51 runtime, a Flatpak build of Sona loads the Widevine CDM, plays a full protected track (AAC decoded in the CDM host child and sent to a sink), owns `org.mpris.MediaPlayer2.sona`, keeps single-instance forwarding, and reaches the Secret Service. The conditions:

1. **The CDM host's namespace layer.** Inside Flatpak the CDM host child never gets its own user and network namespace. Flatpak's seccomp filter refuses `clone(CLONE_NEWUSER)`. Sona then uses its documented fallback: the inet seccomp filter only, with Diagnostics showing the reason. To get the network namespace back, use the Flatpak portal (`flatpak-spawn --sandbox --no-network`). See [CDM host sandbox](#cdm-host-sandbox).
2. **Flathub needs an offline build from source.** The manifest in `build/linux/flatpak/` repackages the release `.deb`. That is fine for local and self-hosted builds, but [Flathub does not accept it](https://docs.flathub.org/docs/for-app-authors/requirements#building-from-source). The Go side does build from source in `org.gnome.Sdk//51` (tested). The remaining work is the vendored module and pnpm source lists.
3. **The CDM needs no host access.** Sona downloads the Widevine CDM from Google into its own data directory (`internal/playback/cdm`), which the Flatpak can always write, so the manifest grants no browser directories (see [CDM access](#cdm-access)).

A manifest exists at `build/linux/flatpak/dev.coredev.Sona.yml`, and its build and install are tested.

## Required finish-args

| Permission | Reason |
| --- | --- |
| `--socket=wayland`, `--socket=fallback-x11`, `--share=ipc`, `--device=dri` | GTK4 + WebKitGTK window and GL. [Recommended set](https://docs.flatpak.org/en/latest/sandbox-permissions.html#standard-permissions) for Wayland-capable apps. |
| `--share=network` | Apple Music API, licences and segments, the loopback sign-in redirect (`internal/auth/mut`). |
| `--filesystem=xdg-run/pipewire-0` | `pipewiresink`, the first sink tried (`internal/playback/gst/gst.go`: `Sinks = pipewiresink, pulsesink, autoaudiosink`). |
| `--socket=pulseaudio` | `pulsesink` fallback. Also the only audio socket a portal-spawned subsandbox can share (see below). |
| `--own-name=org.mpris.MediaPlayer2.sona` | `internal/mpris/mpris.go` `BusName`. Flatpak only grants `org.mpris.MediaPlayer2.$FLATPAK_ID` by default ([docs](https://docs.flatpak.org/en/latest/sandbox-permissions.html#d-bus-access)), and that would be `org.mpris.MediaPlayer2.dev.coredev.Sona`. |
| `--talk-name=org.freedesktop.secrets` | go-keyring calls the Secret Service directly (see [Secrets](#secrets)). |
| `--talk-name=org.freedesktop.Notifications` | `internal/desktop/notify` calls the notification server directly, not `org.freedesktop.portal.Notification`. |
| `--talk-name=org.kde.StatusNotifierWatcher` | Tray (`internal/shell/tray.go`, Wails `SystemTray`). |

**No permission needed:**
- **Single-instance name `dev.coredev.Sona`** (`internal/app/instance.go`). It equals the app ID, which Flatpak grants by default. Verified: `RequestName` returns 1 (primary owner).
- **Portals.** `org.freedesktop.portal.*` is always reachable. This covers the sleep inhibitor's portal fallback (`internal/desktop/inhibit`, used because logind is on the system bus), the settings import/export file chooser (`internal/app/portal.go`), and opening URLs. Wails runs `xdg-open`, and in the runtime that is the [flatpak-xdg-utils](https://github.com/flatpak/flatpak-xdg-utils) binary that forwards to the OpenURI portal.
- **`org.freedesktop.Flatpak` / `--talk-name` for the Flatpak portal.** `flatpak-spawn --sandbox` from the app worked without either.

**Tray name.** Wails also requests `org.kde.StatusNotifierItem-<pid>-1`. The sandbox denies it, and Wails treats that as non-critical: `systemtray_linux.go:338-342` logs it and continues, and registration with the watcher goes by object path. Inside the sandbox the PID is 2 (Sona's log shows `"pid":2`). If a tray host needs the well-known name, add `--own-name=org.kde.StatusNotifierItem-2-1`. Not tested on a real tray host.

**Sleep/wake events.** Wails watches logind's `PrepareForSleep` on the system bus. In the sandbox it logs `Failed to connect to system bus; sleep/wake events will not fire`. Sona does not subscribe to those events (no match for `PowerEvent|SystemWillSleep|ApplicationDidWake` in `internal/`, `cmd/sona/`), so no `--system-talk-name` is needed.

## CDM access

Sona downloads the Widevine CDM itself from Google's servers into `<data>/widevine` (inside the Flatpak, the app's own `~/.var/app/dev.coredev.Sona/data/sona/widevine`), verified as described in [security-review.md](security-review.md), "Widevine CDM downloader". Browser and system copies are never used, so the manifest needs no `--filesystem` grant for a CDM and no browser directory is exposed. No setting or environment variable can point Sona at another library.

The `dlopen` checks below were made with a host copy of the same CDM; they apply equally to the downloaded one.

**`dlopen` inside the runtime works.**
- The CDM needs only `libdl.so.2 libpthread.so.0 libm.so.6 libgcc_s.so.1 libc.so.6 ld-linux-x86-64.so.2`, with symbol versions up to `GLIBC_2.17`. The runtime ships glibc 2.44.
- Inside the app sandbox the CDM loaded, reported its version, and played a full track.
- The CDM is never bundled in the Flatpak or patched ([AGENTS.md](../AGENTS.md) DRM rules).

## CDM host sandbox

**What `internal/playback/cdmhost` does today** (`sandbox.go`, `sandbox_fs.go`, `host.go`; full description in [architecture.md](architecture.md) §4.4 and [security-review.md](security-review.md) F2):
- **Namespaces.** `spawn` starts the child with `namespaceAttr()`: `Cloneflags: CLONE_NEWUSER|CLONE_NEWNET|CLONE_NEWNS`, uid/gid mapped onto themselves. In the mount namespace the child hides `/run`, `/tmp`, `$XDG_RUNTIME_DIR` (except the audio socket) and makes the home directory read-only. If the `fork/exec` fails, it logs `cdmhost: no namespaces for the playback host; starting it without`, restarts the child without namespaces, and records `Sandbox.NetNS=false` with the reason.
- **Landlock.** Applied with or without namespaces: no writes outside `/dev`, no TCP/UDP, `connect()` only to the audio socket (ABI 9), abstract sockets and signals scoped (ABI 6).
- **Seccomp.** The child installs a seccomp filter (TSYNC, `no_new_privs`) that allows `socket` only for `AF_UNIX` and denies ptrace, `process_vm_*`, io_uring, mounts and namespaces, then tests itself (`InetDenied`). Without the filter it does not load the CDM.
- **Diagnostics.** `Info.Sandbox` carries `NetNS`, `MountNS`, `Landlock`, `Seccomp`, `InetDenied` and `Reason` to Diagnostics.

**Conflict with Flatpak.** Flatpak's seccomp filter, applied to every app, blocks sub-namespaces unconditionally, including without `--devel` ([`common/flatpak-run.c`](https://github.com/flatpak/flatpak/blob/main/common/flatpak-run.c), "Don't allow subnamespace setups"):
- `unshare`, `setns`, `mount`, `pivot_root` → EPERM;
- `clone` with `CLONE_NEWUSER` set → EPERM;
- `clone3` → ENOSYS.

`--devel` only lifts `perf_event_open`, `personality` and `ptrace` (`syscall_nondevel_blocklist`). Measured in the sandbox:
- `clone(CLONE_NEWUSER|CLONE_NEWNET)`, `clone(CLONE_NEWNET)` and `unshare(...)` all return `Operation not permitted`.
- Go's `os/exec` with those `Cloneflags` fails with `fork/exec ...: operation not permitted`.
- Stacking a second seccomp filter works: after installing it, `socket(AF_INET)` and `socket(AF_INET6)` return EPERM and `AF_UNIX` still works.

Sona's own sandbox test, run inside the Flatpak, reports `{NetNS:false Seccomp:true InetDenied:true Reason:no network namespace (user namespaces unavailable: fork/exec ...: operation not permitted)}`.

**Options:**
- **(a) Rely on the fallback (works today, no code change).** The child runs inside the app's Flatpak sandbox, shares its network namespace, and is confined by Sona's Landlock domain (where the kernel has it: no writes, `connect()` only to the audio socket from ABI 9) and seccomp filter (no inet or netlink sockets). Diagnostics shows "no network namespace". On kernels without Landlock ABI 9 the child can still reach the Unix sockets the app sandbox can, including the filtered session bus.
- **(b) Flatpak portal subsandbox.** Use [`org.freedesktop.portal.Flatpak.Spawn`](https://github.com/flatpak/flatpak/blob/main/data/org.freedesktop.portal.Flatpak.xml), the `flatpak-spawn` tool shipped in the runtime, with flags `SANDBOX (4) | NO_NETWORK (8) | WATCH_BUS (16)`. Because `flatpak-portal` runs on the host, it creates the namespace with bwrap and the in-sandbox seccomp rule does not apply. Measured:
  - `--no-network` → `connect: Network is unreachable`.
  - `--sandbox-expose-path-ro=<cdm dir>` (sent as an `O_PATH` fd, `sandbox-expose-fd-ro`) → the CDM `dlopen`s. Without it, the path does not exist.
  - The subsandbox gets `/app`, so the same `/app/bin/sona` binary runs as the child.
  - stdin/stdout pipes pass through.
  - `--sandbox-flag=share-sound` (2) shares `pulse/native` but not `pipewire-0`, so the child would use `pulsesink`.
  - The subsandbox gets no home files, no D-Bus unless flag 8, and no display. That is stricter than (a).
  - **Cost.** cdmhost needs a Flatpak branch (detect `/.flatpak-info`, exec `flatpak-spawn` with `--env=` for the child marker and log level, since the portal does not forward the environment).
  - **PID.** `cmd.Process.Pid` becomes the `flatpak-spawn` PID, not the child's, which affects the PID and PSS figures in Diagnostics.
  - **Untested:** a full playback session through the subsandbox (only `dlopen` + `GetCdmVersion` + pipes were tested), and signal forwarding by `flatpak-spawn` when Sona exits.
- **(c) zypak-style.** [zypak](https://github.com/refi64/zypak) is an `LD_PRELOAD` shim that redirects Chromium's own sandbox to the Flatpak sandbox through the same Spawn portal. Sona has no Chromium sandbox to redirect, so zypak would only be an indirect form of (b).

**Recommendation: (b).** It restores a real empty network namespace, and it also takes away the child's home files and D-Bus access, which no non-Flatpak configuration offers. Implement it as the Flatpak path in `cdmhost.spawn`, with (a) as the automatic fallback when the portal is missing. Until (b) is in place, (a) is what Flatpak users get. It is safe for the rule "the CDM host makes no network requests", because inet sockets are refused, and Diagnostics already shows the missing layer.

## Secrets

- **What Sona uses.** go-keyring v0.2.8 connects to `org.freedesktop.secrets` on the session bus (`secret_service/secret_service.go`: `serviceName = "org.freedesktop.secrets"`, `dbus.SessionBus()`). That is the Secret Service API.
- **Why the Secret portal does not fit.** [`org.freedesktop.portal.Secret`](https://github.com/flatpak/xdg-desktop-portal/blob/main/data/org.freedesktop.portal.Secret.xml) has a single method, `RetrieveSecret`. It writes a per-app master secret to an fd. The app (in practice libsecret's local backend) uses it to encrypt its own file inside the sandbox ([GNOME blog, 2019](https://blogs.gnome.org/mclasen/2019/10/03/some-flatpak-updates/)). go-keyring does not implement that, so a "Secret portal" route does not apply to Sona's current code.

The options:
- **`--talk-name=org.freedesktop.secrets`** (in the manifest). Verified: with it, `OpenSession` on a private gnome-keyring succeeds from the sandbox. Without it, the call fails with `ServiceUnknown`. The MUT goes into the user's keyring as on other packages, under service `sona`, key `mut`.
- **No Secret Service on the bus.** `store.Open` falls back to the 0600 file inside the sandbox's `XDG_CONFIG_HOME`. Seen in the sandbox log: `secret service unavailable; storing credentials unencrypted` with `path: ~/.var/app/dev.coredev.Sona/config/sona/credentials.json`. Flatpak removes it on `flatpak uninstall --delete-data`.
- **A real portal backend** would mean implementing `RetrieveSecret` plus a local encrypted store in `internal/auth/store`. Not needed for viability.

## Audio

- **Runtime:** `org.gnome.Platform//51` (GStreamer 1.28.6) contains every element Sona uses: `appsrc`, `appsink` (same plugin), `avdec_aac`, `audioconvert`, `audioresample`, `volume`, `pipewiresink`, `pulsesink`, `autoaudiosink`. `fdkaacdec` and `faad` are missing, but they are only fallbacks.
- **WebKit's media players** (motion artwork) need `webvttenc` from gst-plugins-bad (plugin `subenc`); without it WebKit logs "WebKit wasn't able to find a WebVTT encoder" once per player. The runtime carries it: `libgstsubenc.so` and `libgstvideoparsersbad.so` are in its `/usr/lib/x86_64-linux-gnu/gstreamer-1.0` (229 plugins, checked in the installed `org.gnome.Platform//51`), so the manifest adds nothing for it.
- **AAC without extra extensions:**
  - With `org.freedesktop.Platform.codecs-extra` installed, `libgstlibav.so` links to `/usr/lib/x86_64-linux-gnu/codecs-extra/lib/libavcodec.so.62`.
  - With it uninstalled, `libgstlibav.so` links to the runtime's own `libavcodec.so.62`, and `avdec_aac` is still present.
  - So `org.freedesktop.Platform.ffmpeg-full` is not needed. The GNOME runtime installs `codecs-extra` automatically as a related extension.
- **Sockets.** `--filesystem=xdg-run/pipewire-0` for `pipewiresink`, `--socket=pulseaudio` for `pulsesink`. In the test sandbox (no PipeWire), the child chose `pulsesink` against a private PulseAudio null sink.

## Runtime choice

**Runtime:** `org.gnome.Platform//51` with `org.gnome.Sdk//51`, the newest GNOME branch on Flathub (branches 49, 50 and 51 are listed). SDK `pkg-config` versions: `gtk4 4.24.1`, `webkitgtk-6.0 2.54.1`, `libsoup-3.0 3.8.0`, `gstreamer-1.0 1.28.6`, `glib-2.0 2.90.1`; glibc 2.44. Both WebKitGTK 6.0 and GTK4 come from this runtime. The freedesktop runtime has no WebKitGTK.

**Repackaged release binary:**
- It needs `GLIBC_2.34` at most and `GLIBCXX_3.4.21`.
- `ldd` inside the sandbox finds every library (0 "not found").
- It starts and paints (`startup: first paint`).

**Built from source in the SDK:** links against these libraries directly.

## music:// and desktop integration

- **Exported desktop file.** It keeps `MimeType=x-scheme-handler/music;...;itmss;`. flatpak-builder rewrites `Exec` to `flatpak run ... --file-forwarding dev.coredev.Sona @@u %u @@`.
- **Icon.** `rename-icon: sona` renames the icons to `dev.coredev.Sona`. `StartupWMClass=dev.coredev.Sona` is unchanged.
- **URL forwarding.** A second `flatpak run dev.coredev.Sona music://...` forwarded to the running instance and exited 0. Sona logged `forwarded to the running instance`.
- **Still needed for Flathub:** a metainfo file. It is not written yet.

## Build approach

**1. Repackage the release `.deb`** (the current manifest). This is the local and self-hosted path.
- `build/linux/release/release.sh` builds the `.deb` on Ubuntu 24.04. The manifest unpacks `/usr/bin/sona`, the desktop file and the icons into `/app`.
- The release binary only uses sonames that the GNOME runtime provides (see Runtime).
- `build/linux/flatpak/build.sh [build-dir]` (default `/tmp/sona-flatpak/build`) copies `bin/release/sona_$(cat VERSION)_amd64.deb` (or `$SONA_DEB`) to `bin/flatpak/sona.deb`, the fixed name the manifest reads, so `VERSION` stays the only version source. It runs `flatpak-builder --user --install` with `--state-dir=<build-dir>.state`, outside the repo.

**2. Build from source** (needed for [Flathub](https://docs.flathub.org/docs/for-app-authors/requirements#no-network-access-during-build): no network during the build, every dependency listed as a source).
- **Go:** `org.freedesktop.Sdk.Extension.golang//26.08` (Go 1.27.3). `go.mod` asks for `go 1.26.10`.
  - Generate vendored module sources with [`flatpak-go-mod`](https://github.com/dennwc/flatpak-go-mod), the tool named in [flatpak-builder-tools/go-modules](https://github.com/flatpak/flatpak-builder-tools/tree/master/go-modules), and build with `-mod=vendor`.
  - Tested: `go build -tags production,sqlite_fts5` of the full app inside `org.gnome.Sdk//51` with `GOPROXY=off` (local module cache) succeeds, and the binary prints `sona 0.1.0-flatpaksdk` inside the sandbox.
  - cgo against the runtime's GTK 4.24 only produced deprecation warnings (`gdk_x11_*`).
- **Frontend:** `org.freedesktop.Sdk.Extension.node24//26.08` plus [`flatpak-node-generator pnpm frontend/pnpm-lock.yaml`](https://github.com/flatpak/flatpak-builder-tools/tree/master/node).
  - The lockfile is `lockfileVersion: '9.0'`, and the release container uses pnpm 12.9.0.
  - The tool's README says: for pnpm v12, pass `--pnpm-store-version v11` and run `pnpm install --offline --frozen-lockfile --trust-lockfile`. Not tried here.
- **Build steps:** `pnpm run build` → `frontend/dist`, then `go build` with `-ldflags "-X main.version=..."`, as in `build/linux/Taskfile.yml`. `wails3` binding generation is only needed if the generated bindings are not committed.

## What remains

1. **cdmhost:** add the `flatpak-spawn --sandbox --no-network --watch-bus --sandbox-flag=share-sound --sandbox-expose-path-ro=<cdm dir>` path when `/.flatpak-info` exists, keeping today's fallback. Report the child's real PID. Test a full playback session through it.
2. **Flathub source build:** `flatpak-go-mod` sources and `modules.txt`, `flatpak-node-generator pnpm` sources, a metainfo file, and a source-built manifest replacing the `.deb` module.
3. **Real-desktop checks:** tray (and whether `--own-name=org.kde.StatusNotifierItem-2-1` is needed), notifications, PipeWire sink on a live session, Wayland, Secret Service on GNOME and KDE, `org.freedesktop.portal.Secret`, music:// from a browser.
4. **Diagnostics wording for Flatpak:** "no network namespace (Flatpak)" instead of the raw `fork/exec` error.
