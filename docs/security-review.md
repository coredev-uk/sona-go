# Security review

The review covered the auth loopback server, the `/art/` and `/motion/` handlers, cdmhost framing and sandbox, D-Bus `Activate` arguments, URL parsing and settings file handling. It found 8 issues: 2 Medium and 6 Low, none Critical or High. Seven are fixed. F7 belongs to the auth flow and is recorded below with its fix.

Areas reviewed without findings: native Apple ID input handling, `/art/`, the main.go middleware, `/motion/` path handling, parent→child cdmhost framing, single-instance `Activate` and launch URLs, the other MPRIS methods, notifications, the portal file chooser, the settings file, secret stores, the scrobble queue, the diagnostics bundle, the update check, autostart and the webview URL bindings.

The Widevine CDM downloader, added later, is covered in "Widevine CDM downloader" below; the GitHub Actions workflows in "Workflows". The repository's history was scanned for secrets before it went public; see "Secret scanning".

Run the tests with `go test -race ./internal/playback/cdm/... ./internal/playback/cdmhost/... ./internal/config/... ./internal/discord/... ./internal/motion/... ./internal/mpris/... ./internal/playback/ ./internal/playback/coordinator/... ./internal/logging/ ./internal/diag/`.

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
| F9 | Low | Artwork decoding accepted any registered image format, TIFF included (found after the review, Dependabot alert #1) | Fixed |
| F10 | Low | Development builds logged the build's Last.fm and Discord credentials, and the diagnostics bundle carried them (found after the review, in a macOS run log) | Fixed |

## F1. Settings import could set the CDM path (Medium, fixed)

`ParseExport` filtered local keys by their exact name, but `encoding/json` matches field names case-insensitively. So `"CDMPath"` or `"cdmpath"` in a shared settings file still landed in `Settings.CDMPath`, and the CDM host then `dlopen`ed whatever that path named.

**Fix** (`internal/config/transfer.go`, `settings.go`):
- `Import.Apply` decodes the document over a copy of the current settings and then puts back every local field from the current settings (`keepLocal`). No raw-key filtering is involved.
- `localKeys` is only used to leave the local keys out of an export.
- `Settings.Normalized` drops a `CDMPath` that is not a clean absolute path, or that lies under `/proc` or `/dev` (for example `/proc/self/cwd`, or `/dev/fd` → `/proc/self/fd`).

Since then the `CDMPath` setting has been removed altogether: Sona loads only the CDM it downloaded itself (see "Widevine CDM downloader"), so neither a settings file nor the environment can name a library.

**Tests:**
- `TestImportIgnoresLocalKeysInAnySpelling` imports a fully customised document with every key, local ones included, spelled as exported, in lower case and in upper case. It also covers `"CDMPath"` and `"cdmpath"`.
- `TestNormalizedDropsUnsafeCDMPath`.

## F2. The CDM child sandbox blocked only inet sockets (Medium, fixed)

The child had the whole environment (including `DBUS_SESSION_BUS_ADDRESS`), every path Unix socket (session and system bus, resolved, agents, X11), and full home access. Code running in the CDM could leave the sandbox through D-Bus, for example with systemd's `StartTransientUnit`.

**Fix** (`internal/playback/cdmhost/sandbox_linux.go`, `sandbox_fs_linux.go`, `host.go`):

- **Environment.** `childEnv` drops `DBUS_*`, `DISPLAY`, `WAYLAND_*`, `XAUTHORITY`, `SSH_*`, `GPG_*`, `GNOME_KEYRING*`, `AT_SPI_*`, `KRB5*` and `SESSION_MANAGER`, plus anything named like a credential (`TOKEN`, `SECRET`, `PASSW`, `CREDENTIAL`, `COOKIE`, `API_KEY`, `PRIVATE_KEY`, `AUTH`). The child also starts in `/`.
- **Namespaces and filesystem.** The child starts in a new user, network and mount namespace. Its first stage (holding `CAP_SYS_ADMIN` in its own user namespace only, as an ambient capability) changes the view as follows:
  - `/run`, `/tmp`, `/var/tmp` and `$XDG_RUNTIME_DIR` become empty tmpfs. `/run`'s top-level symlinks are recreated.
  - Only these are bound back: the PipeWire socket (or PulseAudio's when there is no PipeWire), and the program's own files when they live under a covered directory (`APPDIR`, the executable's directory, `LD_LIBRARY_PATH`, the GStreamer plugin paths and scanner, the CDM's directory).
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
- **`PR_SET_DUMPABLE=0` is not set in the parent.** Measured: a non-dumpable process gets `Portal operation not allowed: Unable to open /proc/<pid>/root` from every xdg-desktop-portal call (Settings, and with it the file chooser for settings import/export). Crash capture (`debug.SetCrashOutput`) would not have been affected. The child cannot trace the parent anyway: seccomp denies `ptrace`/`process_vm_*`, and Landlock denies its `/proc` memory and environment.

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
- `server.open` refuses to load the CDM unless `Sandbox.Seccomp && Sandbox.InetDenied` (`Sandbox.loadsCDM` on Linux). macOS development builds load it unsandboxed where Seatbelt is unavailable, and say so: see "macOS sandbox" below.
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

## F9. Artwork decoding accepted any registered image format (Low, fixed)

Found after the review, by Dependabot alert #1: `github.com/disintegration/imaging` crashes on crafted TIFF files and has no patched version. Sona used it to resize and blur artwork, and it also registers TIFF and BMP decoders with the `image` package. Artwork was decoded with `image.Decode`, which picks a decoder by sniffing the bytes, so a TIFF served for an artwork URL would have reached the vulnerable decoder (the same went for the WebP decoder Sona imported itself).

**Fix** (`internal/artwork/image.go`, `artwork.go`, `color.go`):
- Artwork bytes are recognised by their magic numbers and decoded with `image/jpeg` or `image/png` directly, never through the `image` format registry. Anything else fails at fetch with `artwork.ErrFormat`, is not cached, and the `/art/` handler answers 502. Apple's CDN serves JPEG or PNG for every template Sona fills.
- `imaging` and the WebP decoder are gone. Resampling (Lanczos-3 for variants and backdrops, box for colour extraction), the Gaussian blur and the saturation and brightness adjustments are Sona's own code in `image.go`, matching imaging's output: variants are pixel-identical within 1 level, backdrops differ by a mean of 0.25 levels.
- The binary no longer links `golang.org/x/image/tiff`, `golang.org/x/image/bmp`, `golang.org/x/image/webp` or `imaging`.

**Tests:** `TestDecodeAcceptsOnlyJPEGAndPNG` (JPEG and PNG decode; TIFF in both byte orders, GIF, BMP, WebP, truncated magic and empty input fail with `ErrFormat`), `TestTIFFSourceRejected` (a TIFF from the CDN fails `Get`, `Backdrop` and `DominantColors`, is not cached, gets a 502, and a TIFF decoder registered with the `image` package is never called), `TestPNGSource`.

## F10. Development builds logged the build credentials (Low, fixed)

Found after the review, in a macOS development run's log. The build compiles Sona's Last.fm API key and secret and its Discord application ID in with `-ldflags "-X …scrobble.lastfmKey=… -X …scrobble.lastfmSecret=… -X …discord.clientID=…"` (from `SONA_LASTFM_KEY`, `SONA_LASTFM_SECRET`, `SONA_DISCORD_CLIENT_ID`). Without `-trimpath`, Go records `-ldflags` in the binary's build info (`debug.ReadBuildInfo`), and in builds without the `production` tag Wails logs every build-info setting at startup as `"Build Info:"` (`pkg/application/application_debug.go`, `logStartup`). The record went through Sona's log handler unchanged into `sona.log` and the dev stderr, and from the log tail into the diagnostics bundle (Settings → Diagnostics → Copy diagnostics, meant for pasting into issues). Production builds were not affected: `-trimpath` keeps `-ldflags` out of the build info, and the `production` tag compiles `logStartup` out.

Severity Low: the values are compiled into every distributed binary, where anyone can read them, so they are not true secrets. The exposure was a log file on the developer's own machine and any bundle copied from it.

Other paths checked: Sona itself never calls `debug.ReadBuildInfo` or reads Wails' `BuildSettings`; crash files hold the Go runtime's panic output and stacks, which carry no build settings; the Diagnostics report lists versions and a fixed set of environment variables, none of them these.

**Fix:**
- **Log handler** (`internal/logging/redact.go`): an attribute named `-ldflags` (any case, `-`/`_` ignored) is replaced whole with `[REDACTED]`. The whole value goes rather than only credential-named `-X` symbols, because a name list misses the next credential someone injects, and nothing else in the flags is worth a log line (the version is logged by `startup: process start`). In free text, `Redact` replaces a quoted or bare `-ldflags` value, and the value of every linker `-X name=value` assignment whatever the symbol is called.
- **Diagnostics bundle** (`internal/diag/bundle.go`): `Scrub` runs `logging.Redact` over the log tail as read from disk, so a log written by an older build still loses the values.
- **Build** (`build/linux/Taskfile.yml`, `build/darwin/Taskfile.yml`): development builds (`DEV=true`, what `dev` builds) no longer compile the credentials in; Sona reads the same variables from the environment at runtime (devenv loads `.env`). Production and release builds keep the injection, with `-trimpath`, so `go version -m bin/sona` shows no `-ldflags`.

**Tests:** `TestNewKeepsBuildCredentialsOutOfWailsBuildInfo` (a record shaped like Wails' Build Info, through `logging.New`'s JSON file and dev text handlers), `TestRedactBuildFlagsInText` (JSON and text records, bare, quoted and `-X=` forms, a command line; redacting twice changes nothing), `TestBundleScrubsBuildCredentialsFromOldLogs` (JSON and text Build Info lines in `sona.log` and `sona.log.1`).

## Widevine CDM downloader

Sona downloads the Widevine CDM itself (`internal/playback/cdm`), with no prompt, once the network opens after the first paint, and checks for updates daily. Browser and system copies are never used. There is no other source: no setting, environment variable or flag names a library. The CDM is never bundled in a package or the binary, never redistributed and never patched. The threat is a network or mirror attacker getting Sona to install and `dlopen` a library that is not Google's genuine Widevine CDM.

**Manifest trust and pinning** (`manifest.go`):
- The version, size, SHA-512 and download URLs come from Mozilla's Firefox GMP manifest for Widevine (`toolkit/content/gmp-sources/widevinecdm.json`), the file Firefox's own GMP updater falls back to. Sona fetches it pinned to two URLs, in order: the GitHub mirror (`raw.githubusercontent.com/mozilla-firefox/firefox/refs/heads/main/…`), then `hg.mozilla.org/mozilla-central/raw-file/tip/…`. Only `raw.githubusercontent.com`, `hg.mozilla.org` and `hg-edge.mozilla.org` are allowed, redirects included. The document is capped at 1 MiB.
- `hashFunction` must be `sha512` and every hash 64 bytes; anything else is `manifest_invalid`.
- The version must be numeric (`^[0-9]{1,6}(\.[0-9]{1,6}){1,3}$`), because it names an install directory.
- A platform entry's `alias` is followed for at most three hops. A platform with no entry (Linux arm64 today) is `unsupported_platform`.
- The declared size must be between 1 byte and 64 MiB.

**Package hosts** (`manifest.go`, `download.go`): the file URL and its mirrors are used only when they are `https`, without user info, on the default port and on `edgedl.me.gvt1.com`, `www.google.com` or `dl.google.com`; others are dropped, and an entry with none left is `untrusted_url`. Every redirect is checked against the same list (at most 5). Each URL is tried in turn.

**Download limits** (`download.go`): the package streams to a temp file in the install directory, reading at most the manifest's size plus one byte (a larger `Content-Length` is refused up front as `too_large`), and a transfer that receives nothing for 60 s is cancelled. The size and SHA-512 must match the manifest (`hash_mismatch`).

**CRX3 verification** (`crx3.go`), as Chromium's `crx_verifier.cc` does it:
- The header is parsed by hand (magic `Cr24`, version 3, header at most 256 KiB, at most 16 proofs), not by a protobuf library.
- A header containing a ZIP end-of-central-directory token (`PK\x05\x06`, `PK\x06\x07`, `PK\x06\x06`) is refused, so the archive cannot parse differently from what was signed.
- The signed header's CRX ID must be `oimompecagnajdejgnnjijobebaeigek`, the Widevine component's ID.
- There must be a proof by the Widevine component key and one by Google's publisher key, and every RSA and ECDSA proof in the header must verify (SHA-256 over `CRX3 SignedData\x00`, the signed header's length and bytes, and the archive). Any failure is `signature_invalid`.

**Extraction** (`extract.go`, `binfmt.go`):
- Only three entries are written, each under a fixed name: the platform's library, its `.sig` file (macOS and Windows) and the root `manifest.json`. All else is ignored.
- Unsafe names (empty, absolute, `..` elements, backslashes, NUL, drive letters) anywhere in the archive, a wanted entry appearing twice, a non-regular entry, more than 64 entries, or an entry over its limit (64 MiB for the library, 64 KiB for the others, enforced on the bytes read whatever the header claims) are refused. The CRC is checked.
- The component manifest must name `WidevineCdm` at the manifest's version.
- The library's format must match the platform: a 64-bit ELF shared object for the right machine, a Mach-O dylib (or universal slice) for the right CPU, or a PE DLL.

**Install** (`install.go`, `store.go`, `lock_*.go`):
- Versions live in `<data>/widevine/<version>/<platform>/`. A staging directory is renamed into place, and `current.json` (current, previous, rejected versions, consecutive failures, last check) is replaced by an atomic rename. There are no symlinks. A cross-process lock (`flock` on `.lock`, `LockFileEx` on Windows) serialises installs between Sona, `cdmfetch`, `playspike` and tests.
- A version is never downgraded: a manifest version not newer than the current one changes nothing, so a stale or replayed manifest can only point at older genuine packages, which are not installed.
- Before a new version becomes current it must load in a sandboxed probe child that loads the CDM and nothing else (`cdmhost.LoadCheck`, 1 min; no GStreamer). If the CDM fails to load or initialise, or the child dies loading it (`cdm.ErrNotLoadable`), it is deleted and, when a working version exists, recorded as rejected and never fetched again. A check that cannot finish (out of time, or the child could not start) proves nothing either way: the verified files stay as `pending` in `current.json` with their library's SHA-256, and the next check of the same version loads them again if they still match (else they are removed and downloaded again). A pending version is validated like current and previous (a version pattern, so it cannot name a path outside the directory).
- `current.json` records the SHA-256 of each kept version's library, taken at install. `store.repair` drops a kept version whose library is missing or does not match: its directory is removed, a damaged current version falls back to an intact previous one, else to none, and the same version is downloaded again (not rejected, since the package was fine). It runs when the manager starts (after first paint), on every update check, in `Ensure` (`playspike`, `cdmtest`, `cdmfetch`), and at once when a playback host fails to load the CDM; such a failure then does not count towards rollback and the download starts immediately.
- After 3 consecutive failed host starts with the current version, the previous version becomes current again and the failed one is removed and rejected. Hosts using the override are not counted.
- Old versions and leftovers are pruned. A new version applies to the next playback host, never a running one.

**Sandbox** (`cdmhost/host.go`, `sandbox_fs.go`): the parent passes the CDM library's directory to the child (`SONA_CDMHOST_CDM_DIR`). The child keeps it in its filesystem view and bind-mounts it read-only wherever it lies, also under the private `/tmp`, where Landlock would otherwise allow writes. Code in the CDM cannot replace the installed library or `current.json`.

**Tests:** `internal/playback/cdm` (`manifest_test.go`, `crx3_test.go`, `extract_test.go`, `install_test.go`, `manager_test.go`, against fixture packages signed with test keys; `live_test.go` against the real manifest and package); `internal/playback/cdmhost` `TestLoadCheck*` (the check never starts GStreamer, a broken library is `ErrNotLoadable`, running out of time is not).

**Residual risks:**
- Sona trusts Mozilla's manifest (its repository and the GitHub mirror) for which version to fetch, and Google's Widevine component key and publisher key for what is genuine. A compromised manifest could withhold updates, point at an unsupported or older platform entry (refused if older than the current one, but accepted on a first install), or make every check fail. It cannot make Sona install anything not signed by both Google keys and served from Google's hosts, since the CRX3 proofs, the CRX ID and the component manifest are checked independently of it.
- A genuine but older Google build is acceptable on a first install. It is still Google's signed CDM.
- When the playback sandbox is unavailable, the load check is skipped (logged) and the CDM host refuses to load the CDM anyway (F3).
- The CDM itself remains Google's closed code running in the sandboxed child (F2's residual risks apply).
## macOS sandbox (development builds)

macOS has none of the Linux layers above (namespaces, Landlock, seccomp, `prctl`). The CDM host child there runs under Seatbelt, macOS's own sandbox, through `/usr/bin/sandbox-exec -p <profile> <sona>` (`internal/playback/cdmhost/seatbelt.go` writes the profile, `sandbox_darwin.go` applies it). The profile starts from `(allow default)` and takes away what the child never needs; later rules win, so each exception follows its rule:

| | Linux | macOS (Seatbelt) |
| --- | --- | --- |
| Network | Own network namespace; seccomp refuses `AF_INET`/`AF_INET6`/`AF_NETLINK` sockets; Landlock: no TCP/UDP | `(deny network*)`: no TCP, UDP or Unix-socket connect, bind or listen. Core Audio talks Mach IPC, not sockets |
| Session services | Session and system bus sockets gone (fresh `/run`, `$XDG_RUNTIME_DIR`); Landlock allows only the audio socket | No bus to hide; the keychain's Mach services are denied (`com.apple.SecurityServer`, `com.apple.securityd`, `com.apple.securityd.xpc`, `com.apple.security.agent`, `com.apple.security.authhost`): the child is Sona's own executable, which Sona's keychain item trusts. Other Mach services stay reachable (Core Audio needs `coreaudiod`) |
| File writes | Only `/dev` and the private `/tmp` (Landlock); home read-only | Only `/dev`, `/private/tmp`, `/private/var/tmp`, `/private/var/folders` (`$TMPDIR`) and GStreamer's registry cache (`$XDG_CACHE_HOME`/`~/.cache/gstreamer-1.0`, or `$GST_REGISTRY_1_0`'s folder) |
| Hidden files | Sona's config and state, `~/.ssh`, `~/.gnupg`, `~/.password-store`, the keyrings; the downloaded CDM's folder is bound back read-only | Sona's folder (`~/Library/Application Support/Sona`: settings, `credentials.json`, the database, the log), `~/Library/Keychains`, `~/.ssh`, `~/.gnupg`, `~/.password-store`: neither read nor written. The one exception is the downloaded CDM's folder (`<data>/widevine/<version>/mac_arm64`, from `SONA_CDMHOST_CDM_DIR`: the dylib and its `.sig`), readable but not writable, plus the metadata of the folders above it so the path resolves |
| Other processes | seccomp denies `ptrace`/`process_vm_*`; Landlock's ptrace scope | Not restricted by the profile; macOS's own rules apply (no `task_for_pid` on another process without entitlements) [INFERENCE] |
| Environment | `childEnv` drops session addresses and credential-like names | The same `childEnv` |
| Self-check | Child tries `socket(AF_INET…)`; refused = `InetDenied` | Child tries a UDP `connect()` to `127.0.0.1:9` (sends nothing) and listing `~/Library/Keychains`; both must fail with `EPERM`/`EACCES` (`InetDenied`, `SecretsHidden`) |
| No sandbox | The CDM is not loaded (`ErrSandbox`, F3) | The child runs unconfined and **still loads the CDM**; Diagnostics' Host sandbox row says "Not sandboxed" with the reason |

Whether the sandbox is used is decided once per process, on the first host start (`seatbeltChoice`): not when `SONA_SEATBELT=0` is set (a development switch, for telling a sandbox problem from a playback one), not when `/usr/bin/sandbox-exec` is missing, and not when it refuses the profile (a probe runs `/usr/bin/true` under it). `playspike` prints the outcome (`sandbox kind=seatbelt seatbelt=true inet_denied=true secrets_hidden=true reason=""`), and Diagnostics shows it ("Sandboxed by macOS", or "Not sandboxed" and why).

**Why macOS fails open.** macOS builds are development builds only: there is no macOS release until a signed one. `sandbox-exec` is marked deprecated by Apple, though it still ships (Chromium's macOS sandbox uses the same Seatbelt engine through libsandbox's `sandbox_init_with_parameters`); a development setup must not lose playback if a macOS update removes it. A macOS release would need a fail-closed sandbox, as on Linux.

**Not covered yet (verify on a Mac).** None of this has run on macOS: it is type-checked from Linux (`go run ./cmd/typecheck -os darwin -arch arm64 -cgo ./...`) and the profile text and the decision are unit-tested on Linux (`TestSeatbeltProfile`, `TestSeatbeltChoice`). `TestSeatbeltConfinesChild` (macOS only) starts a real child and a probe under the profile: no TCP connect, no write to the home folder, no listing of the keychain folder, but temporary files. Open questions for the Mac run: that Seatbelt's "later rule wins" holds for the write exceptions and for the CDM folder inside Sona's hidden folder; that Core Audio, GStreamer's registry and the CDM work under the profile; and that the CDM needs none of the denied Mach services.

**Residual risks (macOS):** files in the home folder outside the hidden ones stay readable (as on Linux); Mach services other than the keychain's stay reachable; the CDM is Google's downloaded and verified library (see "Widevine CDM downloader"), loaded unmodified, but macOS's library validation does not apply to an unsigned host (none is hardened) [INFERENCE].

## Workflows

The repository's GitHub Actions run on GitHub-hosted runners only (a self-hosted runner reachable from a pull request was ruled out). Once the repository is public, anyone can open a pull request, so the workflows assume the pull request's code is hostile.

**What there is to protect.**

| Asset | Where it lives | Who can read it |
| --- | --- | --- |
| `MUSIC_USER_TOKEN` (a real account's Music User Token, about six months' access to its library and playback) | Secret of the `live` environment | Jobs that name `environment: live` and run from `main`: `live.yml` (weekly schedule, dispatch) |
| Last.fm API key and secret, Discord application ID (compiled into every release binary, so readable by anyone who has one; the risk is someone else's builds or abuse posing as Sona before a release) | Secrets of the `release` environment | Jobs that name `environment: release` and run from `main` or a `v*` tag: `release.yml`'s release job, `release-secrets.yml` (dispatch, prints only whether each is set) |
| `OP_SERVICE_ACCOUNT_TOKEN` (Renovate's 1Password service account, which yields the Renovate app's key) | Repository secret, maintained with Renovate (`renovate.yaml`) | Any workflow run on a branch of this repository; never a fork's run |
| `GITHUB_TOKEN` write scopes | Per job | `release` (`contents: write`, to publish), `live` (`issues: write`, to report a failed scheduled run); every other job reads only |
| Release assets and tags | GitHub releases | The `release` job, after the CI checks |

**Threats and controls.**

- *A fork's pull request runs its own code in CI.* Every workflow a pull request triggers uses `pull_request`, never `pull_request_target` or `workflow_run`, so a fork's run gets a read-only `GITHUB_TOKEN` and no secrets; GitHub withholds both. None of those workflows (`ci.yml`, `pr-title.yml`) reads a secret other than `GITHUB_TOKEN`. Environment secrets stay out of reach even for a pull request from a branch of this repository: an environment deploys only to the refs in its policy (`release`: `main` and `v*` tags; `live`: `main`), and a pull request's ref is `refs/pull/<n>/merge`.
- *A token that outlives its job.* Every checkout sets `persist-credentials: false`, so the token isn't left in `.git/config` for later steps (tests, `pnpm install`, build scripts) to read. No workflow pushes with git.
- *Too much token scope.* Each workflow starts from `permissions: contents: read` (`pr-title.yml`: `pull-requests: read`; `release-secrets.yml`: none), and only the two jobs above widen it. The repository's default workflow permission is read.
- *A third-party action moves its tag.* Every action is pinned to a full commit SHA, with the version in a comment; Renovate proposes updates to the pins, which are reviewed like any other pull request.
- *Script injection.* No `run:` step expands an attacker-controlled expression (pull request title, branch name, commit message); values reach scripts through `env:`.
- *A malicious dependency in CI.* Go modules are checked against `go.sum`; `pnpm install --frozen-lockfile` installs the lockfile's versions and runs no dependency install scripts, since none is allowed. A compromised dependency still runs its code in a pull request's job, which holds nothing worth taking (above).
- *Cache poisoning.* Caches written by a pull request's run are scoped to its ref; the default branch's caches, which every run can restore, are written only by runs on `main`. The cached Widevine CDM is reinstalled through Sona's verifying downloader (`cmd/cdmfetch`, "Widevine CDM downloader").
- *The live token in a test.* `live.yml` runs with `SONA_PRIVATE=1`, so nothing it does reaches the account's listening history, and plays nothing.
- *Hosted-runner cost.* The macOS and Windows jobs in `ci.yml` run only when dispatched while the repository is private, and on every push and pull request once it is public.

**Residual risks.**

- Anyone with write access can push a `v*` tag, and with it a workflow that names `environment: release`. A tag ruleset that limits who may create `v*` tags closes that; once releases run from `main` alone, the `v*` rule can leave the environment's policy.
- `OP_SERVICE_ACCOUNT_TOKEN` is a repository secret, readable by a workflow on any branch of this repository (not a fork's), including the branches Renovate's app pushes. Moving it into an environment limited to `main` would confine it to `renovate.yaml`'s own runs.
- While the repository is private, forks' pull requests run no workflows at all ("Run workflows from fork pull requests" is off, and so are write tokens and secrets for them). Once it is public, a first-time contributor's pull request waits for a maintainer's approval before CI runs ("Require approval for outside contributors"); the controls above are what holds after that approval.

## Secret scanning

**History scan (2026-10-31), before the repository went public: clean.** No credential has ever been committed, and nothing has to be purged from history for secrets.

Scanned: every commit reachable from any branch, tag or pull-request ref on GitHub and in the local clone, plus the pull-request heads GitHub's timeline records as force-pushed away (fetched by SHA): 104 commits, 1,662 unique files. Also every pull request's and issue's title and body, every comment, and the logs of every finished Actions run.

- gitleaks 8.30.1, default rules and `.gitleaks.toml`, over `git log --all` and, separately, over every blob of every tree (which also covers merge commits): nothing beyond the fake test values listed below.
- The local `.env`'s `MUSIC_USER_TOKEN`, `SONA_LASTFM_KEY`, `SONA_LASTFM_SECRET` and `SONA_DISCORD_CLIENT_ID`, compared byte for byte (counts only): none of them in any blob, commit message, pull request, comment or Actions log.
- No `.env`, `.env.*`, `.p8`, `.pem`, key, keystore, keychain, log or database file was ever committed (the only file with a credential-like name, `frontend/.npmrc`, holds `minimum-release-age` alone). No PEM block, keyring dump or `Bearer` literal.
- JWT-shaped strings: only the fake developer tokens of the redaction tests. MUT-shaped strings (long high-entropy base64): only `internal/auth/appleid/testdata/srp_vectors.json`, whose SRP vectors are made from fixed test bytes for `someone@example.com`.
- Wails `Build Info:` and `-ldflags -X` lines appear only in docs, tests and build files, with placeholders, template variables (`{{.SONA_LASTFM_KEY}}`) or the tests' fake values; no log was ever committed (F10's leak stayed on the developer's machine).

**The guard** (`.github/workflows/secret-scan.yml`, on pull requests and pushes to `main`, read-only token, no secrets): gitleaks 8.30.1 from its release, checked against the release's SHA-256, over every commit of every branch and tag (a pull request's own commits included, since they stay public through it), then over the pull request's title and body (read from the event payload, since a step's `env:` is printed in the log). The public log shows each finding's rule, commit, file and line only, as an annotation: gitleaks' own output prints the rest of the line around a secret even with `--redact`. `.gitleaks.toml` adds to gitleaks' defaults:

| Rule | Catches |
| --- | --- |
| `jwt-segment` | A JWT's header segment and dot, so also a developer token split across string literals |
| `apple-music-user-token` | A long token assigned to, or sent as, `Music-User-Token`, `media-user-token` or `MUSIC_USER_TOKEN` |
| `long-base64-token` | A base64 run of 150+ characters with high entropy (a MUT without its name) |
| `lastfm-api-credential` | 32 hex characters next to a Last.fm key or secret name |
| `discord-client-id` | Replaces the default rule, which takes 18 digits only; Discord IDs have 17 to 20 (Sona's has 19) |
| `secret-file` | `.env`, `.env.*`, `.p8`, `.p12`, `.pfx`, `.pem`, `.key`, keystores, SSH keys, by name |

Each allowlist entry is commented and tied to its file: the fake developer tokens in `internal/logging/logging_test.go` and `internal/diag/diag_test.go` (only their two exact fake payloads), the fake Discord ID in `logging_test.go` (only that value) and the SRP vectors. The rules were checked against the real `.env` (all four values flagged) and against planted fakes on a scratch branch that was never pushed: a one-line and a split developer token, a MUT in a header map, a Last.fm secret, a Discord ID, a `.p8` key and a `.env.production` are all found, and so are the fixtures' fake payload copied to another file and a new token added to an allowlisted file.

Locally: `nix run nixpkgs#gitleaks -- git --config .gitleaks.toml --redact .` (all local branches) or `… dir --config .gitleaks.toml --redact .` (the working tree, uncommitted files included).

**Residual risks:**
- A secret pushed in any commit, even one later amended, force-pushed or squashed away, stays fetchable from GitHub by its SHA and through its pull request. If the scan fires, rotate the credential first; removing the commit comes second.
- Pull-request and issue text and Actions logs can't be rewritten with the history. The workflow scans titles and bodies; comments are not scanned.
- A pull request can change `.gitleaks.toml` itself; changes to it need the maintainer's review.
- Shapes the rules can't recognise (a token split into short pieces, encoded differently, or under an unrelated name) pass.
