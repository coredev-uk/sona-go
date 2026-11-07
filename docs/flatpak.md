# Flatpak

## Verdict

**Viable, with conditions.** On the GNOME 51 runtime, a Flatpak build of Sona loads the Widevine CDM, plays a full protected track (AAC decoded in the CDM host child and sent to a sink), owns `org.mpris.MediaPlayer2.sona`, keeps single-instance forwarding, and reaches the Secret Service. The conditions:

1. **The CDM host's namespace layer.** Inside Flatpak the CDM host child never gets its own user and network namespace. Flatpak's seccomp filter refuses `clone(CLONE_NEWUSER)`. Sona then uses its documented fallback: the inet seccomp filter only, with Diagnostics showing the reason. To get the network namespace back, use the Flatpak portal (`flatpak-spawn --sandbox --no-network`). See [CDM host sandbox](#cdm-host-sandbox).
2. **Flathub needs an offline build from source.** The manifest in `build/linux/flatpak/` builds Sona from source in `org.gnome.Sdk//51`, with the Go toolchain from `org.freedesktop.Sdk.Extension.golang` and every Go module as a checksummed source (`go-sources.json`), so the build runs offline as [Flathub requires](https://docs.flathub.org/docs/for-app-authors/requirements#no-network-access-during-build). Built, installed and smoke-tested on NixOS (see [Build and test](#build-and-test)). The remaining Flathub work is a metainfo file and a `git` source at a tag in place of the working-tree `dir` source. The native UI needs no Node or pnpm sources.
3. **The CDM needs no host access.** Sona downloads the Widevine CDM from Google into its own data directory (`internal/playback/cdm`), which the Flatpak can always write, so the manifest grants no browser directories (see [CDM access](#cdm-access)).

The manifest is `build/linux/flatpak/dev.coredev.Sona.yml`. It builds offline, installs and starts (first paint, MPRIS, single instance, CDM download, preview playback into a PipeWire null sink) with flatpak 1.18.4 and flatpak-builder 1.4.4 on NixOS; [Build and test](#build-and-test) has the commands and what was and was not run.

## Required finish-args

| Permission | Reason |
| --- | --- |
| `--socket=wayland`, `--socket=fallback-x11`, `--share=ipc`, `--device=dri` | GTK3 window (MyGo) and GL. [Recommended set](https://docs.flatpak.org/en/latest/sandbox-permissions.html#standard-permissions) for Wayland-capable apps. |
| `--share=network` | Apple Music API, licences and segments, the loopback sign-in redirect (`internal/auth/mut`). |
| `--filesystem=xdg-run/pipewire-0` | `pipewiresink`, the first sink tried (`internal/playback/gst/output_linux.go`: `Sinks = pipewiresink, pulsesink, autoaudiosink`). |
| `--socket=pulseaudio` | `pulsesink` fallback. Also the only audio socket a portal-spawned subsandbox can share (see below). |
| `--own-name=org.mpris.MediaPlayer2.sona` | `internal/mpris/mpris.go` `BusName`. Flatpak only grants `org.mpris.MediaPlayer2.$FLATPAK_ID` by default ([docs](https://docs.flatpak.org/en/latest/sandbox-permissions.html#d-bus-access)), and that would be `org.mpris.MediaPlayer2.dev.coredev.Sona`. |
| `--talk-name=org.freedesktop.secrets` | go-keyring calls the Secret Service directly (see [Secrets](#secrets)). |
| `--talk-name=org.freedesktop.Notifications` | `internal/desktop/notify` calls the notification server directly, not `org.freedesktop.portal.Notification`. |
| `--talk-name=org.kde.StatusNotifierWatcher` | Tray (MyGo's libayatana-appindicator3, a StatusNotifierItem). |

**No permission needed:**
- **Single-instance name `dev.coredev.Sona`** (`internal/app/instance.go`). It equals the app ID, which Flatpak grants by default. Verified: `RequestName` returns 1 (primary owner).
- **Portals.** `org.freedesktop.portal.*` is always reachable. This covers the sleep inhibitor's portal fallback (`internal/desktop/inhibit`, used because logind is on the system bus), the settings import/export file chooser (`internal/app/portal.go`), the Realtime portal through which Sona raises the playback host's audio thread (`internal/playback/cdmhost/realtime_linux.go`: RTKit sits on the system bus, out of the Flatpak's reach), and opening URLs. MyGo runs `xdg-open`, and in the runtime that is the [flatpak-xdg-utils](https://github.com/flatpak/flatpak-xdg-utils) binary that forwards to the OpenURI portal.
- **`org.freedesktop.Flatpak` / `--talk-name` for the Flatpak portal.** `flatpak-spawn --sandbox` from the app worked without either.

**Tray name.** MyGo's tray (libayatana-appindicator3 0.5.94 built by the manifest) registers with the watcher by object path and bus name, not by a well-known name. Measured against a stand-in `org.kde.StatusNotifierWatcher` on the session bus (the bus log of `flatpak run --log-session-bus`): the only names the app requested were `dev.coredev.Sona` and `org.mpris.MediaPlayer2.sona`, none was refused, and the watcher received `RegisterStatusNotifierItem("/org/ayatana/NotificationItem/mygo_2_1")` from the app's unique name. So `--own-name=org.kde.StatusNotifierItem-<pid>-1` is not needed with this library; the sandbox PID is 2 (Sona's log shows `"pid":2`). Not tested against a real tray host (GNOME's AppIndicator extension, KDE).

**Sleep/wake events.** Sona does not subscribe to logind's `PrepareForSleep` or any other sleep/wake event (no match for it in the Go sources), so no `--system-talk-name` is needed. The earlier Wails shell did watch it and logged `Failed to connect to system bus; sleep/wake events will not fire` in the sandbox; that shell is gone.

## CDM access

Sona downloads the Widevine CDM itself from Google's servers into `<data>/widevine` (inside the Flatpak, the app's own `~/.var/app/dev.coredev.Sona/data/sona/widevine`), verified as described in [security-review.md](security-review.md), "Widevine CDM downloader". Browser and system copies are never used, so the manifest needs no `--filesystem` grant for a CDM and no browser directory is exposed. No setting or environment variable can point Sona at another library.

The `dlopen` checks below were made with a host copy of the same CDM; they apply equally to the downloaded one.

**`dlopen` inside the runtime works.**
- The CDM needs only `libdl.so.2 libpthread.so.0 libm.so.6 libgcc_s.so.1 libc.so.6 ld-linux-x86-64.so.2`, with symbol versions up to `GLIBC_2.17`. The runtime ships glibc 2.44.
- Inside the app sandbox the CDM loaded, reported its version, and played a full track.
- The CDM is never bundled in the Flatpak or patched ([AGENTS.md](../AGENTS.md) DRM rules).

## CDM host sandbox

**What `internal/playback/cdmhost` does today** (`sandbox_linux.go`, `sandbox_fs_linux.go`, `host.go`; full description in [architecture.md](architecture.md) §4.4 and [security-review.md](security-review.md) F2):
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
- **Motion artwork** decodes through Sona's own pipeline (`internal/ui/media`: `decodebin`, `avdec_h264`, `videoconvert`; parsers from gst-plugins-bad). The runtime carries `libgstvideoparsersbad.so` in its `/usr/lib/x86_64-linux-gnu/gstreamer-1.0` (checked in the installed `org.gnome.Platform//51`), so the manifest adds nothing for it.
- **AAC without extra extensions:**
  - With `org.freedesktop.Platform.codecs-extra` installed, `libgstlibav.so` links to `/usr/lib/x86_64-linux-gnu/codecs-extra/lib/libavcodec.so.62`.
  - With it uninstalled, `libgstlibav.so` links to the runtime's own `libavcodec.so.62`, and `avdec_aac` is still present.
  - So `org.freedesktop.Platform.ffmpeg-full` is not needed. The GNOME runtime installs `codecs-extra` automatically as a related extension.
- **Sockets.** `--filesystem=xdg-run/pipewire-0` for `pipewiresink`, `--socket=pulseaudio` for `pulsesink`. With a PipeWire socket in the sandbox the playback host chose `pipewiresink` with `avdec_aac` (log: `playback host started … "sink":"pipewiresink","decoder":"avdec_aac","preview":true`). In an earlier sandbox without PipeWire it chose `pulsesink` against a private PulseAudio null sink.

## Runtime choice

**Runtime:** `org.gnome.Platform//51` with `org.gnome.Sdk//51`, the newest GNOME branch on Flathub (branches 49, 50 and 51 are listed). SDK `pkg-config` versions: `gstreamer-1.0 1.28.6`, `glib-2.0 2.90.1`; glibc 2.44. MyGo's UI needs GTK3, libepoxy, AT-SPI, libsecret, EGL and XTest by soname; the installed `//51` runtime has `libgtk-3.so.0`, `libepoxy.so.0`, `libatspi.so.0`, `libsecret-1.so.0`, `libEGL.so.1` and `libXtst.so.6` in `/usr/lib/x86_64-linux-gnu` (checked with `flatpak run --command=sh`), and `ldd /app/bin/sona` reports nothing missing. The tray's libayatana-appindicator3 is not in the runtime, so the manifest builds it (with libayatana-indicator, ayatana-ido and libdbusmenu-gtk3) as modules. libdbusmenu's 2016 `configure` needs intltool and dbus-glib, which the SDK lacks, so two more build-time modules provide them (the same as [flathub/shared-modules](https://github.com/flathub/shared-modules); dbus-glib stays, as libdbusmenu-glib links it). No WebKitGTK, GTK4 or libsoup is needed.

**Built from source in the SDK:** `go build` with the flags of `build/linux/Taskfile.yml` (`mygo_noinspector,sqlite_fts5`, `-trimpath`, `-X main.version=$(cat VERSION)`), linking the runtime's GStreamer through cgo. The binary prints `sona <VERSION>` inside the sandbox and starts. The `.deb` the earlier manifest repackaged is no longer involved, so a Flatpak build needs no podman or release container.

## music:// and desktop integration

- **Exported desktop file.** It keeps `MimeType=x-scheme-handler/music;...;itmss;`. flatpak-builder rewrites `Exec` to `flatpak run ... --file-forwarding dev.coredev.Sona @@u %u @@`.
- **Default handler.** Settings → General → Open Apple Music links in Sona isn't offered from inside the sandbox, which can't change the host's default applications: the row says so, and the desktop's own Default Applications settings do it.
- **Icon.** `rename-icon: sona` renames the icons to `dev.coredev.Sona`. `StartupWMClass=dev.coredev.Sona` is unchanged.
- **URL forwarding.** A second `flatpak run dev.coredev.Sona music://...` forwarded to the running instance and exited 0. Sona logged `forwarded to the running instance`.
- **Still needed for Flathub:** a metainfo file (with screenshots and a release entry, which must follow `VERSION`). It is not written yet.

## Build and test

**Setup, once** (any distro with flatpak and flatpak-builder; on NixOS `build.sh` borrows them from `nix shell nixpkgs#flatpak nixpkgs#flatpak-builder nixpkgs#bubblewrap`):

```sh
flatpak --user remote-add --if-not-exists flathub https://dl.flathub.org/repo/flathub.flatpakrepo
flatpak --user install flathub org.gnome.Platform//51 org.gnome.Sdk//51 \
  org.freedesktop.Sdk.Extension.golang//26.08
```

**Build and install** (`--user`; about 2 minutes warm, with the module downloads of the tray libraries on the first run):

```sh
bash build/linux/flatpak/build.sh [build-dir]    # default /tmp/sona-flatpak/build; state next to it
SONA_FLATPAK_BUNDLE=sona.flatpak bash build/linux/flatpak/build.sh   # also writes a bundle
flatpak run dev.coredev.Sona --version           # sona <VERSION>
```

- **Offline Go modules.** `go-sources.json` lists each module's `.mod` and `.zip` from proxy.golang.org with its sha256. The manifest's build command points `GOPROXY=file://$PWD/go-proxy` at the tree those sources lay out (`GOSUMDB=off`: the file proxy has no checksum database, `go.sum` still verifies every module). Regenerate it after any `go.mod` or `go.sum` change (Renovate does not):

  ```sh
  devenv shell -- go run build/linux/flatpak/gosources.go > build/linux/flatpak/go-sources.json
  ```

  A stale list fails the build with a missing module. `.info` files are left out: the proxy does not serve them byte for byte, and `go build` does not read them. The list has the modules of every OS (Windows-only ones too); that costs a few hundred kilobytes, not a pruned build graph.
- **Source tree.** The `dir` source copies the working tree minus `.git`, `.devenv`, `bin` and the like. Flathub would use `type: git` with the release tag and commit instead.
- **NixOS.** flatpak runs without a system helper; bubblewrap works unprivileged here (user namespaces are on). The shell may carry devenv's `LD_LIBRARY_PATH`, whose newer glibc breaks nixpkgs' flatpak (`version 'GLIBC_2.43' not found`): `build.sh` and `smoke.sh` drop it for flatpak. `flatpak --user install` can print `cp: cannot create regular file …/exports/share/icons/hicolor/index.theme: Permission denied` on a reinstall, because the `index.theme` left by the first install is read-only (mode 444, copied from the Nix store); the build and install succeed, and `chmod u+w` on that file silences it.

**Smoke test** (headless sway, private `HOME` and XDG dirs, `SONA_PRIVATE=1`, no real session touched):

```sh
devenv shell -- bash build/linux/flatpak/smoke.sh
```

It passes when `--version` prints `sona <VERSION>`, the log has `startup: first paint` and sway shows a window with `sandbox_app_id` `dev.coredev.Sona`, `org.mpris.MediaPlayer2.sona` and `dev.coredev.Sona` are owned on the session bus, a second `flatpak run` reaches the running instance (`sona status`), and the Widevine CDM has downloaded into `~/.var/app/dev.coredev.Sona/data/sona/widevine` (`SONA_FLATPAK_SMOKE_OFFLINE=1` skips that last check).

**What the run on NixOS showed** (flatpak 1.18.4, GNOME 51 runtime, headless sway, `MYGO_GPU=0`):
- **First paint** at 391 to 564 ms after process start; the window had `sandbox_app_id` `dev.coredev.Sona`.
- **CDM.** The setup dialog showed "Widevine: Ready, version 4.10.3112.0", downloaded after first paint into the Flatpak's data directory. The load check ran in the CDM host child through the fallback below.
- **CDM host sandbox.** `cdmhost: no namespaces for the playback host; starting it without` with `fork/exec /app/bin/sona: operation not permitted`, as [CDM host sandbox](#cdm-host-sandbox) describes. Playback still ran through the fallback.
- **Preview playback** (`SONA_PREVIEW=1 SONA_PRIVATE=1`, searching "come together beatles" and playing the first result from the keyboard): `sona status --json` reported `"state":"playing","preview":true`; the host used `pipewiresink` and `avdec_aac`; `pw-link -l` showed the stream "Sona" linked to a private null sink (`sona-flatpak-null`, targeted with `--env=PIPEWIRE_NODE=`), and `pw-record --target sona-flatpak-null -P '{ stream.capture.sink=true }'` captured 7.7 s of audio with peak 16385 and RMS 3088 (16-bit). The real PipeWire socket was reached by symlinking it into the headless run directory.
- **D-Bus.** `org.mpris.MediaPlayer2.sona` and `dev.coredev.Sona` owned; `flatpak run dev.coredev.Sona open music://…` reached the running instance.
- **Not Sona's: the test machine had no desktop portals.** The headless session has no `org.freedesktop.portal.Flatpak`, so the window icon's glycin loader (`flatpak-spawn --sandbox`) failed with a `window icon` warning, the document portal could not start (`F: Can't get document portal`), and the sleep inhibitor and Realtime portals were unavailable (`inhibit: acquire`, `cdmhost: audio threads keep normal priority`). Secret Service was a locked headless gnome-keyring, so Sona used its 0600 file fallback. On a real desktop the portals exist.

**Not run:** the tray against a real host, notifications, a protected (Widevine) track (needs a Music User Token), a live Wayland GPU session (the run used the CPU renderer), Secret Service on GNOME and KDE, `music://` from a browser.

**CI.** The dispatch-only `Flatpak` workflow (`.github/workflows/flatpak.yml`) builds the manifest in `ghcr.io/flathub-infra/flatpak-github-actions:gnome-51` and uploads `sona.flatpak`. It is not a required check and has no runtime test (the container has no desktop).

## What remains

1. **cdmhost:** add the `flatpak-spawn --sandbox --no-network --watch-bus --sandbox-flag=share-sound --sandbox-expose-path-ro=<cdm dir>` path when `/.flatpak-info` exists, keeping today's fallback. Report the child's real PID. Test a full playback session through it.
2. **Flathub:** a metainfo file, and a `git` source at a tag in place of the `dir` source.
3. **Real-desktop checks:** tray, notifications, PipeWire sink on a live session, a GPU Wayland session, Secret Service on GNOME and KDE, `org.freedesktop.portal.Secret`, `music://` from a browser, a protected track.
4. **Diagnostics wording for Flatpak:** "no network namespace (Flatpak)" instead of the raw `fork/exec` error.
