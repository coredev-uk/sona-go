# Security review

The review covered the auth loopback server, the `/art/` and `/motion/` handlers, cdmhost framing and sandbox, D-Bus `Activate` arguments, URL parsing and settings file handling. It found 8 issues: 2 Medium and 6 Low, none Critical or High. Seven are fixed. F7 belongs to the auth flow and is recorded below with its fix.

Areas reviewed without findings: native Apple ID input handling, `/art/`, the main.go middleware, `/motion/` path handling, parent→child cdmhost framing, single-instance `Activate` and launch URLs, the other MPRIS methods, notifications, the portal file chooser, the settings file, secret stores, the scrobble queue, the diagnostics bundle, the update check, autostart and the webview URL bindings.

Run the tests with `go test -race ./internal/playback/cdmhost/... ./internal/config/... ./internal/discord/... ./internal/motion/... ./internal/mpris/... ./internal/playback/ ./internal/playback/coordinator/...`.

| ID | Severity | Issue | Status |
| --- | --- | --- | --- |
| F1 | Medium | Settings import could set the CDM path | Fixed |
| F2 | Medium | CDM child sandbox blocked only inet sockets | Fixed (residual risks listed) |
| F3 | Low | Sandbox failed open | Fixed |
| F4 | Low | Discord IPC trusted any user's socket | Fixed |
| F5 | Low | NaN/Inf MPRIS volume | Fixed |
| F6 | Low | Motion artwork followed redirects anywhere | Fixed |
| F7 | Low | Loopback sign-in could be completed by another local user | Open: owned by the auth flow |
| F8 | Low | Child could exhaust the parent's memory | Fixed |

## F1. Settings import could set the CDM path (Medium, fixed)

`ParseExport` filtered local keys by their exact name, but `encoding/json` matches field names case-insensitively. So `"CDMPath"` or `"cdmpath"` in a shared settings file still landed in `Settings.CDMPath`, and the CDM host then `dlopen`ed whatever that path named.

**Fix** (`internal/config/transfer.go`, `settings.go`):
- `Import.Apply` decodes the document over a copy of the current settings and then puts back every local field from the current settings (`keepLocal`). No raw-key filtering is involved.
- `localKeys` is only used to leave the local keys out of an export.
- `Settings.Normalized` drops a `CDMPath` that is not a clean absolute path, or that lies under `/proc` or `/dev` (for example `/proc/self/cwd`, or `/dev/fd` → `/proc/self/fd`).

**Tests:**
- `TestImportIgnoresLocalKeysInAnySpelling` imports a fully customised document with every key, local ones included, spelled as exported, in lower case and in upper case. It also covers `"CDMPath"` and `"cdmpath"`.
- `TestNormalizedDropsUnsafeCDMPath`.

## F2. The CDM child sandbox blocked only inet sockets (Medium, fixed)

The child had the whole environment (including `DBUS_SESSION_BUS_ADDRESS`), every path Unix socket (session and system bus, resolved, agents, X11), and full home access. Code running in the CDM could leave the sandbox through D-Bus, for example with systemd's `StartTransientUnit`.

**Fix** (`internal/playback/cdmhost/sandbox.go`, `sandbox_fs.go`, `host.go`):

- **Environment.** `childEnv` drops `DBUS_*`, `DISPLAY`, `WAYLAND_*`, `XAUTHORITY`, `SSH_*`, `GPG_*`, `GNOME_KEYRING*`, `AT_SPI_*`, `KRB5*` and `SESSION_MANAGER`, plus anything named like a credential (`TOKEN`, `SECRET`, `PASSW`, `CREDENTIAL`, `COOKIE`, `API_KEY`, `PRIVATE_KEY`, `AUTH`). The child also starts in `/`.
- **Namespaces and filesystem.** The child starts in a new user, network and mount namespace. Its first stage (holding `CAP_SYS_ADMIN` in its own user namespace only, as an ambient capability) changes the view as follows:
  - `/run`, `/tmp`, `/var/tmp` and `$XDG_RUNTIME_DIR` become empty tmpfs. `/run`'s top-level symlinks are recreated.
  - Only these are bound back: the PipeWire socket (or PulseAudio's when there is no PipeWire), and the program's own files when they live under a covered directory (`APPDIR`, the executable's directory, `LD_LIBRARY_PATH`, the GStreamer plugin paths and scanner, `SONA_WIDEVINE_CDM`).
  - The home directory becomes recursively read-only (`mount_setattr`).
  - Sona's config and state directories, `~/.ssh`, `~/.gnupg`, `~/.password-store` and the keyrings are hidden under empty read-only tmpfs.
- **Landlock** (applied with or without namespaces, at whatever ABI the kernel offers):
  - No file writes outside `/dev`, plus the private `/tmp` when there is one.
  - No TCP or UDP.
  - `connect()` to path Unix sockets only for the audio socket (ABI 9).
  - Abstract Unix sockets and signals are scoped to the sandbox (ABI 6).
  - Landlock's ptrace rule also denies `/proc/<pid>/{mem,environ}` of every process outside the sandbox.
- **Two-stage start.** The first stage restricts its own thread and then re-executes `/proc/self/exe`. So the second stage starts with no capabilities, `no_new_privs` set, and every thread inside the Landlock domain. It refreshes GStreamer's registry first, while the home directory is still writable.
- **Seccomp** (second stage, TSYNC):
  - `socket` and `socketpair` are allowed only for `AF_UNIX`.
  - These fail with EPERM: `ptrace`, `process_vm_readv/writev`, `pidfd_getfd`, `kcmp`, `io_uring_*`, `bpf`, `perf_event_open`, `userfaultfd`, the keyring calls, `unshare`, `setns`, the mount calls and `clone` with namespace flags.
  - `clone3` fails with ENOSYS, and glibc then uses `clone`.
  - The child checks that `AF_INET`, `AF_INET6` and `AF_NETLINK` are refused.
- **Without unprivileged user namespaces** (Flatpak, user namespaces disabled), the child runs with Landlock and seccomp only. Under Ubuntu 24.04's AppArmor restriction the namespaces are created without capabilities: the child keeps its network namespace but gets no private filesystem view ("make mounts private: permission denied", seen on GitHub's ubuntu-24.04 runners). Diagnostics says why.
- **`PR_SET_DUMPABLE=0` is not set in the parent.** Measured: a non-dumpable process gets `Portal operation not allowed: Unable to open /proc/<pid>/root` from every xdg-desktop-portal call (Settings, and with it the CDM file chooser and settings import/export). Crash capture (`debug.SetCrashOutput`) would not have been affected. The child cannot trace the parent anyway: seccomp denies `ptrace`/`process_vm_*`, and Landlock denies its `/proc` memory and environment.

**Tests and evidence:**
- `TestSandboxConfinesChild` runs a probe child that enters the real sandbox and tries each escape. The `namespaces` subtest uses the namespace path, `landlock only` the fallback. It checks:
  - The child cannot connect to the session bus, the system bus, resolved's socket (where present; not on the test machine), or test sockets in `$XDG_RUNTIME_DIR` and `/tmp`, nor to an abstract socket.
  - It cannot write under `$HOME`.
  - It cannot open `AF_INET`/`AF_INET6`/`AF_NETLINK`.
  - It cannot read the parent's `/proc/<pid>/environ` or call `process_vm_readv` on it.
  - It does reach the PipeWire socket.

  Observed on Linux 7.2 (Landlock ABI 10): in the namespace path, the bus sockets give "no such file or directory" and a home write gives "read-only file system". In the fallback, the bus gives "permission denied" from Landlock.
- `TestChildEnvDropsSessionAndSecrets`, `TestChildCannotOpenInetSockets` (now also requires the mount namespace), `TestSandboxFallsBackWithoutNamespaces`.
- `TestChildCannotOpenInetSockets` and the `namespaces` subtest skip, with the reason, only where the system refuses the namespaces: EPERM or EACCES when a probe child of the test's own creates user, network and mount namespaces (Flatpak, user namespaces disabled) or changes its mounts in them (Ubuntu 24.04's AppArmor restriction creates the namespaces but denies their capabilities). Any other failure, including a sandbox that misses a layer the system allows, fails them. CI lifts Ubuntu's restriction (`kernel.apparmor_restrict_unprivileged_userns=0`) and sets `SONA_CDMHOST_REQUIRE_NAMESPACES=1`, so there they must run.
- Playback: `PIPEWIRE_NODE=<null sink> go run ./cmd/playspike -seek 90` played a full track with `sandbox netns=true mountns=true landlock=10 seccomp=true inet_denied=true reason=""`. The test track was "Weird Fishes / Arpeggi": first audio after 862 ms, the track advanced 12.1 s past the seek target, and the join into "Reckoner" measured 87 ms. `pw-record` of the null sink's monitor had signal in 22 of the 27 s recorded.

**Residual risks:**
- The PipeWire (or PulseAudio) socket stays reachable, because output needs it. The audio server is trusted. A PulseAudio server would also accept `load-module` requests.
- Files in the home directory stay readable, except the hidden directories. Read data has no way out to the network: the child has no network, no bus and no writes outside `/dev` (which includes the shared `/dev/shm`). Its only other channel is the pipe to the parent, whose CDM messages go to Apple's licence server.
- PipeWire's realtime module cannot reach RTKit (D-Bus) from the child, so the output thread runs at normal priority unless `RLIMIT_RTPRIO` allows it directly [INFERENCE]. Underruns under heavy CPU load have not been measured.
- On kernels without Landlock:
  - The fallback has seccomp only, so path sockets and home writes are reachable.
  - In both paths, `/proc/<pid>/mem` and `environ` of other same-user processes are guarded only by Yama and ordinary ptrace rules.
- On kernels with Landlock below ABI 9, the fallback can still reach path Unix sockets.

## F3. The sandbox failed open (Low, fixed)

When the seccomp filter could not be installed (an architecture other than amd64/arm64, or a kernel without seccomp), the child logged a warning and loaded the CDM anyway.

**Fix:**
- `server.open` refuses to load the CDM unless `Sandbox.Seccomp && Sandbox.InetDenied`.
- The refusal is `cdmhost.ErrSandbox`, sent with the reply code `sandbox` and mapped back by `replyErr`.
- Diagnostics sets `DiagPlayback.CDMBlocked`. The Widevine CDM row then reads "Blocked", and Host sandbox reads "Not sandboxed" with the child's reason.
- Library uploads (no CDM) still play.

**Test:** `TestCDMRefusedWithoutSeccomp`.

## F4. Discord IPC trusted any user's socket (Low, fixed)

Another local user could create `/tmp/discord-ipc-0` and receive the now-playing track.

**Fix** (`internal/discord/discord.go`):
- Candidates are `Lstat`ed and must be sockets (not symlinks) owned by `getuid()`.
- The parent directory must be owned by the user, or be a root-owned directory that is not group/world-writable or is sticky (`/tmp`).
- The temp directories are probed only when `XDG_RUNTIME_DIR` is unset.

**Test:** `TestIgnoresUntrustedSockets` covers a regular file, a symlink, another uid's socket, temp sockets while a runtime directory exists, and the fallback when it does not.

## F5. NaN/Inf MPRIS volume (Low, fixed)

Go's `min`/`max` pass NaN through, so `Volume = nan` reached the snapshots, the settings and GStreamer's `volume`.

**Fix:**
- MPRIS `Set Volume` rejects NaN and ±Inf with `InvalidArgs`, and its `clamp01` maps NaN to 0.
- `playback.ClampVolume` (NaN → 0) is used by `VolumeGain` and `Coordinator.SetVolume`.
- `config.Settings.Normalized` clamps the same way.

**Tests:** `TestMethodsReachPlayer` (NaN, +Inf, −Inf → InvalidArgs), `TestSetVolumeNaN`, `TestVolumeGainClamps`.

## F6. Motion artwork followed redirects anywhere (Low, fixed)

**Fix** (`internal/motion/motion.go`):
- `New` copies the HTTP client and sets `CheckRedirect` to re-run the allow-list on every hop, with at most 5 redirects.
- `appleMedia` rejects userinfo and any port but 443.

**Tests:**
- `TestRedirectLeavingAllowList`: an allowed host redirects to another https host and to plain http. Nothing is served, and the targets get no request.
- `TestAppleMedia`.

## F7. Another local user could complete the loopback sign-in with their own account (Low, open)

`internal/auth/mut/loopback.go` uses a nonce as its only secret, and that nonce is in the URL passed to `xdg-open`. That puts it in a browser's argv, which every user can read in `/proc/<pid>/cmdline` unless `/proc` is mounted with `hidepid`. Another user can then POST their own token to `/auth/<nonce>/complete` before the real redirect arrives.

This item is owned by the auth flow work (`internal/auth/**`) and was left to its owners.

**Suggested fix:** make `/start` single-use. It should set an `HttpOnly; SameSite=Strict; Path=/auth/<nonce>/` cookie holding a second random value that never appears in argv, and `/complete` should require that cookie.

## F8. A compromised child could exhaust the parent's memory (Low, fixed)

**Fix** (`internal/playback/cdmhost/host.go`):
- **Frame size.** The parent reads child frames through `gobFrames`. It parses gob's per-message length prefix and fails a message over 16 MiB (`maxChildFrame`) before gob allocates for it. The largest legitimate child→parent frame is a licence request or renewal of a few KiB.
- **Event queue.** At most 1024 events (`maxQueuedEvents`) wait for the receiver. Beyond that, position samples are dropped and any other event ends the child.
- **On a violation**, the parent kills the child, and `Err` wraps the reason (`errMisbehaved`).

**Tests:**
- `TestOversizedFrameKillsChild`: a child announces a 512 MiB frame.
- `TestEventFloodIsBounded`: 3072 position samples, then 2048 other events.
- `TestGobFramesPassThrough`: real streams pass through at any read size, and a frame over the limit fails.
