# Security review

Sona's threat model and the controls behind it. The findings below (F1–F10) record each weakness found in Sona's own code, the control that closed it and the tests that keep it closed. The Widevine CDM downloader, Apple's runtime translations, lyrics from other services, the macOS and Windows sandboxes, the GitHub Actions workflows and secret scanning have their own sections. Each section lists the residual risks that remain.

Areas reviewed without findings: native Apple ID input handling, the artwork and motion artwork caches' file and path handling, parent→child cdmhost framing, single-instance `Activate` and launch URLs, the other MPRIS methods, notifications, the portal file chooser, the settings file, secret stores, the scrobble queue, the diagnostics bundle, the update check and autostart.

Run the tests with `go test -race ./internal/playback/cdm/... ./internal/playback/cdmhost/... ./internal/config/... ./internal/discord/... ./internal/motion/... ./internal/mpris/... ./internal/playback/ ./internal/playback/coordinator/... ./internal/logging/ ./internal/diag/ ./internal/auth/... ./internal/i18n/... ./internal/lyrics/...`.

| ID | Severity | Issue | Status |
| --- | --- | --- | --- |
| F1 | Medium | Settings import could set the CDM path | Fixed |
| F2 | Medium | CDM child sandbox blocked only inet sockets | Fixed (residual risks listed) |
| F3 | Low | Sandbox failed open | Fixed |
| F4 | Low | Discord IPC trusted any user's socket | Fixed |
| F5 | Low | NaN/Inf MPRIS volume | Fixed |
| F6 | Low | Motion artwork followed redirects anywhere | Fixed |
| F7 | Low | Loopback sign-in could be completed by another local user | Fixed (residual risks listed) |
| F8 | Low | Child could exhaust the parent's memory | Fixed |
| F9 | Low | Artwork decoding accepted any registered image format, TIFF included | Fixed |
| F10 | Low | Development builds logged the build's Last.fm and Discord credentials, and the diagnostics bundle carried them | Fixed |

## F1. Settings import could set the CDM path (Medium, fixed)

`ParseExport` filtered local keys by their exact name, but `encoding/json` matches field names case-insensitively. So `"CDMPath"` or `"cdmpath"` in a shared settings file still landed in `Settings.CDMPath`, and the CDM host then `dlopen`ed whatever that path named.

**Fix** (`internal/config/transfer.go`):
- `Import.Apply` decodes the document over a copy of the current settings and then puts back every local field from the current settings (`keepLocal`). No raw-key filtering is involved.
- `localKeys` is only used to leave the local keys out of an export.

The `CDMPath` setting no longer exists: Sona loads only the CDM it downloaded itself (see "Widevine CDM downloader"), so neither a settings file nor the environment can name a library.

**Test:** `TestImportIgnoresLocalKeysInAnySpelling` imports a fully customised document with every key, local ones included, spelled as exported, in lower case and in upper case.

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
- **Realtime scheduling from outside.** PipeWire's module-rt needs D-Bus to ask RTKit for realtime scheduling, so the child reports its audio threads over the pipe and the parent asks RTKit (or the Realtime portal) for them; see `docs/architecture.md` §4.2, "Realtime priority". The seccomp filter is unchanged: RTKit changes the threads' scheduling from outside, and the child needs only `sched_getscheduler` to find them.

**Tests:**
- `TestSandboxConfinesChild` runs a probe child that enters the real sandbox and tries each escape. The `namespaces` subtest uses the namespace path, `landlock only` the fallback. It checks:
  - The child cannot connect to the session bus, the system bus, resolved's socket (where present), or test sockets in `$XDG_RUNTIME_DIR` and `/tmp`, nor to an abstract socket.
  - It cannot write under `$HOME`.
  - It cannot open `AF_INET`/`AF_INET6`/`AF_NETLINK`.
  - It cannot read the parent's `/proc/<pid>/environ` or call `process_vm_readv` on it.
  - It does reach the PipeWire socket.
- `TestChildEnvDropsSessionAndSecrets`, `TestChildCannotOpenInetSockets` (also requires the mount namespace), `TestSandboxFallsBackWithoutNamespaces`.
- `TestChildCannotOpenInetSockets` and the `namespaces` subtest skip, with the reason, only where the system refuses the namespaces: EPERM or EACCES when a probe child of the test's own creates user, network and mount namespaces (Flatpak, user namespaces disabled) or changes its mounts in them (Ubuntu 24.04's AppArmor restriction creates the namespaces but denies their capabilities). Any other failure, including a sandbox that misses a layer the system allows, fails them. CI lifts Ubuntu's restriction (`kernel.apparmor_restrict_unprivileged_userns=0`) and sets `SONA_CDMHOST_REQUIRE_NAMESPACES=1`, so there they must run.
- The DRM path regression (`go run ./cmd/playspike -seek 90`) prints the sandbox it got (its `sandbox` line: kind, namespaces, Landlock ABI, seccomp, `inet_denied`, reason) and plays a full track inside it.
- `TestThreadReportRaisesChildThreads`: a child marks a thread the way libpipewire does and reports it with PID 1's thread; the parent asks only for the child's own, lowers its `RLIMIT_RTTIME`, and shows the grant in `Info`. `TestAudioThreadsBounded` (at most 8 threads, each once), `TestRaise` (the decisions: priority cap, nice fallback, refusals, no broker, foreign threads), `TestBusBroker` (RTKit's and the portal's D-Bus interface on a private bus).

**Residual risks:**
- The PipeWire (or PulseAudio) socket stays reachable, because output needs it. The audio server is trusted. A PulseAudio server would also accept `load-module` requests.
- Files in the home directory stay readable, except the hidden directories. Read data has no way out to the network: the child has no network, no bus and no writes outside `/dev` (which includes the shared `/dev/shm`). Its only other channel is the pipe to the parent, whose CDM messages go to Apple's licence server.
- A compromised child can get up to 8 of its own threads realtime (`SCHED_RR` at RTKit's `MaxRealtimePriority`, 20 by default, which the PipeWire server's own threads match or exceed). It reports thread IDs only; the parent asks only for threads under `/proc/<child>/task`, each once. Before asking, the parent lowers the child's `RLIMIT_RTTIME` (soft and hard) to RTKit's `RTTimeUSecMax` (200 ms by default), and the child has no `CAP_SYS_RESOURCE` to raise it again, so a realtime thread that runs that long without blocking is killed. RTKit's watchdog also demotes every realtime thread when it sees the system starve. Where neither RTKit nor the Realtime portal answers, or they refuse, the audio threads run at normal priority (Diagnostics → Audio priority says why).
- On kernels without Landlock:
  - The fallback has seccomp only, so path sockets and home writes are reachable.
  - In both paths, `/proc/<pid>/mem` and `environ` of other same-user processes are guarded only by Yama and ordinary ptrace rules.
- On kernels with Landlock below ABI 9, the fallback can still reach path Unix sockets.

## F3. The sandbox failed open (Low, fixed)

When the seccomp filter could not be installed (an architecture other than amd64/arm64, or a kernel without seccomp), the child logged a warning and loaded the CDM anyway.

**Fix:**
- `server.open` refuses to load the CDM unless `Sandbox.Seccomp && Sandbox.InetDenied` (`Sandbox.loadsCDM` on Linux). Windows refuses it unless the child is in its AppContainer with the network refused (see "Windows sandbox" below). macOS builds load it unsandboxed where Seatbelt is unavailable, and say so: see "macOS sandbox" below.
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

## F7. Another local user could complete the loopback sign-in with their own account (Low, fixed)

`internal/auth/mut/loopback.go` used a nonce as its only secret, and that nonce is in the URL passed to `xdg-open`. That puts it in the argv of `xdg-open` and the browser, which every user can read in `/proc/<pid>/cmdline` unless `/proc` is mounted with `hidepid`. Another user could then POST their own token to `/auth/<nonce>/complete` before the real redirect arrived, and Sona would sign in to their account.

**Fix** (`internal/auth/mut/loopback.go`, `peer.go`, `peer_linux.go`; `internal/auth/manager.go`):
- **Socket owner (Linux).** The loopback listener looks up each connection's client socket in `/proc/net/tcp` and `/proc/net/tcp6` (the row whose local address is the client's and remote address is the server's) and drops the connection, unanswered, unless Sona's own uid owns it. A connection whose socket is not found is dropped too. Reading the URL from argv no longer gets another user anything. This is what closes the finding: a start-page cookie alone would not, because a process that reads the URL can load `/start` and post `/complete` within milliseconds, before `xdg-open` has started the browser.
- **Single-use start links bound by a cookie** (every platform). Each start link carries its own random ticket (`/auth/<nonce>/start?t=<ticket>`) and works once. Loading it sets the cookie `sona_signin`: 256 random bits that never appear in a URL, host-only (no `Domain`), `Path=/auth/<nonce>/`, `HttpOnly`, `SameSite=Strict`, `Max-Age` = the rest of the attempt. `/complete` refuses (403) without that cookie, compared in constant time. A ticket loaded a second time ends the attempt (`mut.ErrStartReused`): Sona's pages never load one twice (the start page replaces itself with Apple's), so a repeat means the link reached someone else. The UI's "Open the page again" and "Copy link" ask Go for a new link (`AuthService.SignInLink`) instead of reusing the opened one.
- **The attempt ends** on success, failure, timeout, cancel or a replayed link. After that every start link and `/complete` answer 410, cookie or not, and the server shuts down. A successful `/complete` deletes the cookie.
- **Why `SameSite=Strict` works here.** Apple's authorize page returns the browser to `/callback` with a cross-site top-level navigation, so the cookie is not sent there, and does not need to be: `/callback` only serves the page that reads the fragment, and the token never reaches the server on that request. The cookie is needed on the callback page's `fetch` POST to `/complete`, a same-origin request from a top-level `127.0.0.1` document, where `Strict` cookies are sent. Measured in headless Chromium 155 against a stand-in for `authorize.music.apple.com` (served over TLS through `--host-resolver-rules`): `/start` (`Sec-Fetch-Site: none`) sets the cookie, `/callback` arrives `cross-site` with no cookie, `/complete` arrives `same-origin` with it and succeeds. There is no `Secure` attribute or `__Host-` prefix: both need https.

**Tests:**
- `internal/auth/mut`: `TestCompleteRequiresTheStartCookie` (no cookie, a wrong value and a truncated value get 403 and leave the attempt pending; the right cookie completes, even beside a stray one), `TestCompleteBeforeStartIsRefused`, `TestStartLinkWorksOnceAndAReplayEndsTheAttempt` (the replay gets 410 with no cookie and no authorize URL, `Wait` returns `ErrStartReused`, the first cookie can no longer complete), `TestEachLinkWorksOnce` (a second link works; a missing or guessed ticket is 404 and harmless), `TestTimeoutEndsTheAttempt` (after the deadline the cookie no longer completes, `/start` sets nothing, the server is closed), `TestHappyPathWithCookieJar` (real HTTP with a cookie jar; a racing client without it gets 403; the cookie is deleted after), `TestStartCookieBindsTheAttempt` (the attributes above), `TestCompleteDeliversOnceAndRejectsReplays`.
- `TestSocketUIDFindsTheClientRow` and `TestParseProcAddrRejectsMalformed` cover the `/proc/net/tcp*` parser (the client's row, not the server end's; IPv4-mapped rows in `tcp6`). `TestOwnerListenerDropsOtherUsers` (Linux) runs the real lookup: a listener expecting another uid closes this process's connection unanswered, one expecting its own uid serves it.
- `internal/auth`: `TestReplayedStartLinkEndsSignIn`, `TestOpenFailureKeepsWaiting` (a new link from `SignInLink` completes the attempt), `TestLoopbackSignIn` (the manager's browser loads the start link with a cookie jar before completing).

**Residual risks:**
- On macOS the socket-owner check is not implemented. There other users cannot read a process's arguments (the kernel refuses `KERN_PROCARGS2` across users) and `open` hands the URL to the browser through Launch Services, so the URL does not leak that way; the cookie binding still applies. Windows has neither check, and its loopback sign-in has not been reviewed.
- Cookies are not isolated by port. A same-user process can always read Sona's state, so this matters only for another user, whom the Linux check already keeps off Sona's port; another user's server on a different `127.0.0.1` port would see the cookie only if the browser visited it with a same-site navigation, which `SameSite=Strict` rules out for links from other sites.
- Another user who reads the URL can still load a start link to end the attempt (Linux drops that connection, so only on platforms without the check). That is a denial of service only: the user starts again, and the native sign-in is unaffected.
- If a browser runs in a different network namespace from Sona, it cannot reach `127.0.0.1:<port>` at all; Flatpak and Snap browsers share the host's network namespace, and their sockets carry the user's uid.

## F8. A compromised child could exhaust the parent's memory (Low, fixed)

**Fix** (`internal/playback/cdmhost/host.go`):
- **Frame size.** The parent reads child frames through `gobFrames`. It parses gob's per-message length prefix and fails a message over 16 MiB (`maxChildFrame`) before gob allocates for it. The largest legitimate child→parent frame is a decoded music video picture: about 630 KB for an SD picture (864×486 I420), at most about 5.5 MB at the 1920×1920 limit. Licence requests and renewals are a few KiB.
- **Event queue.** At most 1024 events (`maxQueuedEvents`) wait for the receiver. Beyond that, position samples are dropped and any other event ends the child.
- **On a violation**, the parent kills the child, and `Err` wraps the reason (`errMisbehaved`).

**Tests:**
- `TestOversizedFrameKillsChild`: a child announces a 512 MiB frame.
- `TestEventFloodIsBounded`: 3072 position samples, then 2048 other events.
- `TestGobFramesPassThrough`: real streams pass through at any read size, and a frame over the limit fails.

## F9. Artwork decoding accepted any registered image format (Low, fixed)

`github.com/disintegration/imaging` crashes on crafted TIFF files and has no patched version. Sona used it to resize and blur artwork, and it also registers TIFF and BMP decoders with the `image` package. Artwork was decoded with `image.Decode`, which picks a decoder by sniffing the bytes, so a TIFF served for an artwork URL would have reached the vulnerable decoder (the same went for the WebP decoder Sona imported itself).

**Fix** (`internal/artwork/image.go`, `artwork.go`, `color.go`):
- Artwork bytes are recognised by their magic numbers and decoded with `image/jpeg` or `image/png` directly, never through the `image` format registry. Anything else fails at fetch with `artwork.ErrFormat` and is not cached (the UI reads artwork through the cache in process, through `internal/ui/kit`'s loader). Apple's CDN serves JPEG or PNG for every template Sona fills.
- `imaging` and the WebP decoder are gone. Resampling (Lanczos-3 for variants and backdrops, box for colour extraction), the Gaussian blur and the saturation and brightness adjustments are Sona's own code in `image.go`, matching imaging's output: variants are pixel-identical within 1 level, backdrops differ by a mean of 0.25 levels.
- The binary no longer links `golang.org/x/image/tiff`, `golang.org/x/image/bmp`, `golang.org/x/image/webp` or `imaging`.

**Tests:** `TestDecodeAcceptsOnlyJPEGAndPNG` (JPEG and PNG decode; TIFF in both byte orders, GIF, BMP, WebP, truncated magic and empty input fail with `ErrFormat`), `TestTIFFSourceRejected` (a TIFF from the CDN fails `Get`, `Backdrop` and `DominantColors`, is not cached, gets a 502, and a TIFF decoder registered with the `image` package is never called), `TestPNGSource`.

## F10. Development builds logged the build credentials (Low, fixed)

The build compiles Sona's Last.fm API key and secret and its Discord application ID in with `-ldflags "-X …scrobble.lastfmKey=… -X …scrobble.lastfmSecret=… -X …discord.clientID=…"` (from `SONA_LASTFM_KEY`, `SONA_LASTFM_SECRET`, `SONA_DISCORD_CLIENT_ID`). Without `-trimpath`, Go records `-ldflags` in the binary's build info (`debug.ReadBuildInfo`). Sona's previous shell, Wails (removed with the move to the native UI; the finding was made against it), logged every build-info setting at startup as `"Build Info:"` in builds without the `production` tag. The record went through Sona's log handler unchanged into `sona.log` and the dev stderr, and from the log tail into the diagnostics bundle (Settings → Diagnostics → Copy diagnostics, meant for pasting into issues). Production builds were not affected: `-trimpath` keeps `-ldflags` out of the build info, and the `production` tag compiled the startup log out. The native app has no such logger, but the controls below stay as defence in depth, and the bundle still has to scrub logs an older build wrote.

Severity Low: the values are compiled into every distributed binary, where anyone can read them, so they are not true secrets. The exposure was a log file on the developer's own machine and any bundle copied from it.

Other paths checked: Sona itself never calls `debug.ReadBuildInfo`; crash files hold the Go runtime's panic output and stacks, which carry no build settings; the Diagnostics report lists versions and a fixed set of environment variables, none of them these.

**Fix:**
- **Log handler** (`internal/logging/redact.go`): an attribute named `-ldflags` (any case, `-`/`_` ignored) is replaced whole with `[REDACTED]`. The whole value goes rather than only credential-named `-X` symbols, because a name list misses the next credential someone injects, and nothing else in the flags is worth a log line (the version is logged by `startup: process start`). In free text, `Redact` replaces a quoted or bare `-ldflags` value, and the value of every linker `-X name=value` assignment whatever the symbol is called.
- **Diagnostics bundle** (`internal/diag/bundle.go`): `Scrub` runs `logging.Redact` over the log tail as read from disk, so a log written by an older build still loses the values.
- **Build** (`build/linux/Taskfile.yml`, `build/darwin/Taskfile.yml`, `cmd/devloop`): development builds (what `dev` builds through `cmd/devloop`, whose `-ldflags` are only `-w`) do not compile the credentials in; Sona reads the same variables from the environment at runtime (`internal/scrobble/lastfm.go`, `internal/discord/discord.go`; devenv loads `.env`). The Taskfiles' production and release builds keep the injection, with `-trimpath`, so `go version -m bin/sona` shows no `-ldflags`.

**Tests:** `TestNewKeepsBuildCredentialsOutOfBuildInfoRecord` (a record shaped like the removed shell's Build Info, through `logging.New`'s JSON file and dev text handlers), `TestRedactBuildFlagsInText` (JSON and text records, bare, quoted and `-X=` forms, a command line; redacting twice changes nothing), `TestBundleScrubsBuildCredentialsFromOldLogs` (JSON and text Build Info lines in `sona.log` and `sona.log.1`).

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

**Sandbox** (`cdmhost/host.go`, `sandbox_fs_linux.go`): the parent passes the CDM library's directory to the child (`SONA_CDMHOST_CDM_DIR`). The child keeps it in its filesystem view and bind-mounts it read-only wherever it lies, also under the private `/tmp`, where Landlock would otherwise allow writes. Code in the CDM cannot replace the installed library or `current.json`.

**Tests:** `internal/playback/cdm` (`manifest_test.go`, `crx3_test.go`, `extract_test.go`, `install_test.go`, `manager_test.go`, against fixture packages signed with test keys; `live_test.go` against the real manifest and package); `internal/playback/cdmhost` `TestLoadCheck*` (the check never starts GStreamer, a broken library is `ErrNotLoadable`, running out of time is not).

**Residual risks:**
- Sona trusts Mozilla's manifest (its repository and the GitHub mirror) for which version to fetch, and Google's Widevine component key and publisher key for what is genuine. A compromised manifest could withhold updates, point at an unsupported or older platform entry (refused if older than the current one, but accepted on a first install), or make every check fail. It cannot make Sona install anything not signed by both Google keys and served from Google's hosts, since the CRX3 proofs, the CRX ID and the component manifest are checked independently of it.
- A genuine but older Google build is acceptable on a first install. It is still Google's signed CDM.
- When the playback sandbox is unavailable, the load check is skipped (logged) and the CDM host refuses to load the CDM anyway (F3).
- The CDM itself remains Google's closed code running in the sandboxed child (F2's residual risks apply).

## Music videos

Music videos play through the same CDM host child as songs: the CDM decrypts and decodes the video rendition with its own video decoder (`internal/playback/widevine`, `cdmhost`), and the child paces the pictures against the audio clock (docs/architecture.md §4.2). The threats are protected content leaving the child in a form that could be saved, and a compromised child abusing the picture path.

**What leaves the child:** decoded pictures only, as I420 planes, one at a time when each is due. The clear bitstream and the keys never go from the child to the parent (licence requests go out as they do for audio); the parent only sends the child the encrypted segments it fetched.

**Checks in the parent** (`internal/playback/cdmhost/video.go`, `host.go`): a picture is dropped before it reaches `OnVideoFrame` unless both sides are positive and at most 1920 (`MaxVideoSize`) and its data is exactly the I420 size for them (`I420Size`). The general frame limit (F8, 16 MiB) still bounds the message itself.

**Memory only** (`internal/playback/video`): the surface keeps only the latest picture, never a backlog, and never writes one to disk, a cache or a log. When the current item stops being a video, the engine clears it.

**Serving:** there is none. The UI draws in the same process: its video feed (`internal/ui/media`, `kit.VideoFeed`) waits on the surface in memory and converts the latest picture to RGBA for one bitmap. No HTTP handler, loopback or network listener serves pictures.

**Quality:** SD only. Apple licenses HD and 4K only to hardware-backed CDMs (HDCP), and Sona never works around that (docs/apple-music-reference.md §4.5).

**Tests:** `TestVideoFrameValid` and `TestI420Size` (`cdmhost/videotime_test.go`: wrong sizes, short and long data, over the limit), the surface tests (`internal/playback/video/surface_test.go`), and the engine test that only the loaded video's pictures reach the surface (`internal/playback/native/video_test.go`).

## Apple's translations

Sona's UI text in other languages comes first from Apple Music's own web strings, which Go fetches from music.apple.com at run time and caches (`internal/i18n/applestrings`, docs/ui-architecture.md "Strings and locales"). The threat is a network attacker (or Apple serving something unexpected) getting text into the UI that misleads the user, breaks rendering, or runs as code.

**Requests** (`bundle.go`, `store.go`): plain `GET`s of the `/us/home` page, the index script it names and one locale's bundle, with a browser User-Agent and no credentials or cookies, through the start-up gate (after first paint). Redirects must stay on `https://music.apple.com` (at most 5). Responses are capped (page 4 MiB, index script 16 MiB, bundle 4 MiB, read up to the cap plus one byte). Cached URLs are trusted only under `https://music.apple.com/assets/`, and cache file names come only from Apple's locale tags matching `^[a-z0-9]+(-[a-z0-9]+){0,3}$`.

**Parsing** (`bundle.go`): the bundle is a JS module, read as data and never run: string constants and one object literal of string keys and string values, with JS string escapes decoded; calls, template substitutions, numbers, arrays or anything else fail the whole bundle.

**Conversion and checks** (`mapping.go`, `icu.go`): only the keys `apple-keys.json` maps are read; everything else in the bundle is ignored. Each becomes an ICU message for its Sona message: Apple's `@@name@@` placeholders are renamed through the reviewed mapping, ICU syntax characters in Apple's text are quoted, so Apple's text cannot add arguments, selects or plurals. A message is dropped when an Apple placeholder has no Sona argument, a Sona argument is missing from Apple's text (the count in a plural's `other` form), the plural shape differs, or it is over 4 KiB.

**Cache** (`store.go`): `<cache>/strings/index.json` and `<apple-tag>.json`, written atomically. A file over 1 MiB, with unknown fields, another format version or an untrusted URL is ignored and fetched again; cached messages are kept only for IDs the current mapping has.

**Rendering**: `internal/ui/i18n` parses a message with MyGo's `i18n.ParseMessage` (one that does not parse is left out, logged once, and the English text shows) and formats it into a string that MyGo's widgets draw as text; nothing interprets it as markup or code. A locale is shown only when its layers translate 90% of Sona's messages.

**Tests:** `internal/i18n/applestrings` (`TestParseBundleModuleRejectsCode`, `TestToICURefuses*`, `TestConvertKeepsOnlyMappedCheckedMessages`, `TestStoreRefusesOversizedBundle`, `TestStoreStaysOnApplesOrigin`, `TestStoreIgnoresEnglishAndDamagedCache`); `internal/ui/i18n` (`TestLayersAppleThenCommunityThenEnglish`).

**Residual risks:**
- Sona trusts music.apple.com over TLS for what the strings say: Apple (or anyone able to serve that origin) can put wrong or misleading wording in place of a mapped message, within its placeholders and the size cap.

## Lyrics from other services

Besides Apple's, Sona shows lyrics from Musixmatch, LRCLIB and NetEase Cloud Music (`internal/lyrics`, docs/architecture.md §3 "Lyrics"). The threats are a service (or a network attacker) sending text that misleads the user, breaks rendering or runs as code; a service learning more about the user than the song; and Musixmatch's user token leaking.

**Requests** (`provider.go`, `services/lyrics.go`): only from Go, after first paint, through the start-up gate, to fixed `https` hosts (`apic-appmobile.musixmatch.com`, `lrclib.net`, `music.163.com`); redirects must stay on `https` (at most 5). Each request carries the song's title, artist, album, length and ISRC, a `Sona/<version>` User-Agent and no cookies or Apple credentials; the UI never talks to these hosts. A body is read up to 8 MiB; a larger one fails to decode. Each provider has a 10 s timeout, its own pace and a backoff, so a slow or hostile service delays only its own answer.

**Musixmatch's user token** (`musixmatch.go`): anonymous, from Musixmatch's `token.get` (no account), kept in the keyring with the usual 0600 file fallback (`store.OpenSecret`, key `musixmatch`), never in SQLite or the settings. It is added to the query only when a request is sent, and a transport error is rewrapped without its URL, so the token never reaches an error message or the log. A token Musixmatch no longer accepts is replaced once; a captcha answer is a rate limit (15 minutes' rest).

**Parsing** (`lrc.go`, `musixmatch.go`, `netease.go`): bodies are JSON decoded into fixed structs; LRC, Musixmatch's richsync and subtitle bodies are parsed as data (timestamps and text), never run or interpreted as markup. A result whose song does not match (title, artist, length within 3 s, or the ISRC) is dropped.

**Cache** (`cache.go`): the parsed result (`lyrics.Lyrics` JSON) per provider, song and translation language in `lyrics_cache`; an entry that doesn't decode is ignored and fetched again.

**Rendering**: lines, words and translations reach the UI as plain strings in the lyrics DTO and are drawn as text by MyGo's widgets; nothing interprets them as markup.

**Tests:** `internal/lyrics` (`TestMusixmatchKeepsTokenOutOfErrors`, `TestMusixmatchCaptchaAndTokenRenewal`, `TestMusixmatchMatchesByISRC`, `TestLRCLIB`, `TestNetEase`, `TestRateLimitBacksOff`, `TestLimiter`, `TestCachePositiveAndNegative`).

**Residual risks:**
- Each service sees which songs the user looks up lyrics for (title, artist, album, ISRC) and the user's IP address. Sources can be switched off in Settings → Playback → Lyrics; NetEase is off by default.
- The services decide what the lyrics say: a wrong or offensive text is shown as sent. A wrong match is limited by the title, artist and length checks.
- Musixmatch's endpoint and token flow are the ones its own apps use, not a documented API; Musixmatch can change or refuse them, and Sona then shows other sources' lyrics.

## Local broadcasters' on-air metadata (TuneIn)

While a local broadcaster plays, Sona shows the song its stream names, and for the stations Apple serves through TuneIn, TuneIn's profile of the station (`internal/tunein`, `internal/playback/native/broadcast.go` and `onair.go`; docs/architecture.md §4.2). The threats are a service (or a network attacker) sending text that misleads the user or breaks rendering, a request that can be steered to another host, and listening data leaving Sona.

**Requests:** from the parent only (the CDM host child has no network and the UI never contacts these hosts), to one fixed `https` host, `opml.radiotime.com` (`Describe.ashx?id=s…&render=json`). The station ID is not free text: `tunein.StationID` takes it from the `ti_ad_params` of Apple's own play URL, only when that URL is on `tunein.com`, and only when it matches `^s[0-9]{1,12}$`; `Describe` refuses anything else before a request is built, so a station name or a hostile URL cannot add a path or parameter. No search by name is made, so a station Apple does not serve through TuneIn is never looked up there. A response is read up to 256 KiB. Polling is bounded (30 s while the station lists songs and the stream names none, 10 min otherwise, exponential backoff from 30 s to 10 min on failure, none for a station TuneIn does not know, and none once the stream names the song itself) and ends with the track. The song's cover is a catalog search at Apple (`amp-api.music.apple.com`, the usual client), whose artwork goes through the existing artwork cache, so no new image host is allowed.

**Parsing:** TuneIn's JSON is decoded into a fixed struct; every field reaches the UI as plain text drawn by MyGo's widgets (no markup). The website TuneIn lists is kept only if it is an `http` or `https` URL without user info, and opens in the system browser on a click (`OpenURL` of `internal/services`, called from Now Playing), never inside the app. A stream's ICY text is split at " - " and never interpreted.

**Not a listen:** the queue item is the station; the song on air is display and MPRIS metadata. It is never reported as Apple play activity, never recorded in the history, and live radio is never scrobbled (`internal/scrobble/track.go`).

**Tests:** `internal/tunein` (`TestStationID`, `TestParseDescribe`, `TestDescribe`, `TestWatch…`), `internal/playback/native` (`TestBroadcastTuneIn`, `TestBroadcastOnAirWhenHeard`, `TestMatchArt`), `internal/playback/broadcast` (`TestOnAirOfTitle`).

**Residual risks:**
- TuneIn sees the user's IP address and which of its stations they play (as it already does when Apple's playlist makes Sona fetch TuneIn's PLS); the catalog search tells Apple which song is on air, with the account's token. Private mode does not stop either (it keeps plays out of Apple's history and the scrobblers; these are reads).
- `Describe.ashx` is TuneIn's long-public OPML endpoint, not a documented API for third parties: it can change or be withdrawn, in which case the profile and TuneIn's song disappear and the stream's own metadata remains. The cover can be wrong only if the catalog holds a different song with the same title and artist.

## macOS sandbox

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

**Why macOS fails open.** macOS releases exist (ad-hoc-signed, `build/darwin/bundle.sh`, no hardened runtime), but Seatbelt is still fail-open. `sandbox-exec` is marked deprecated by Apple, though it still ships (Chromium's macOS sandbox uses the same Seatbelt engine through libsandbox's `sandbox_init_with_parameters`); a user must not lose playback if a macOS update removes it. A fail-closed sandbox, as on Linux, is still to do.

**Not covered yet (verify on a Mac).** The macOS build is type-checked from Linux (`go run ./cmd/typecheck -os darwin -arch arm64 ./...`), and the profile text and the decision are unit-tested there (`TestSeatbeltProfile`, `TestSeatbeltChoice`). `TestSeatbeltConfinesChild` (macOS only) starts a real child and a probe under the profile: no TCP connect, no write to the home folder, no listing of the keychain folder, but temporary files; `ci.yml`'s "macOS build and test" job runs it (it runs when dispatched, and on pushes and pull requests once the repository is public). A hand run on a real Mac belongs to the first native release's verification. Open questions for that run: that Seatbelt's "later rule wins" holds for the write exceptions and for the CDM folder inside Sona's hidden folder; that Core Audio, GStreamer's registry and the CDM work under the profile; and that the CDM needs none of the denied Mach services.

**Residual risks (macOS):** files in the home folder outside the hidden ones stay readable (as on Linux); Mach services other than the keychain's stay reachable; the CDM is Google's downloaded and verified library (see "Widevine CDM downloader"), loaded unmodified, but macOS's library validation does not apply to an unsigned host (none is hardened) [INFERENCE].

## Windows sandbox

Windows has none of the Linux layers and no `sandbox-exec`. The CDM host child there runs in an **AppContainer**, the OS's own sandbox for processes that must touch neither the user's files nor the network (Chromium's renderers use one too), inside a **job object** (`internal/playback/cdmhost/appcontainer_windows.go` starts it; `sandbox_windows.go` holds the decision and the child's self-checks). The child is the same `sona.exe`, started with `CreateProcess` and `PROC_THREAD_ATTRIBUTE_SECURITY_CAPABILITIES`; Go's `os/exec` cannot pass that attribute, so the parent creates the process itself and keeps its own handle for the kill (Go's handle from `os.FindProcess` cannot terminate it).

- **Identity and network.** The profile `sona.cdmhost` (`CreateAppContainerProfile`, created on the first spawn and kept in the user's registry) holds no capabilities: not `internetClient`, not `privateNetworkClientServer`. Windows checks every access of the child twice, against the user and against the profile's SID (and ALL APPLICATION PACKAGES), and the network stack refuses all the child's traffic. Observed on windows-2025: a TCP `connect()` to an address off the machine fails at once with `WSAEACCES`, and a connection to a listener of the parent's on the loopback gets no answer. Two things the AppContainer does *not* stop: the child can connect to a socket of its own on the loopback, and a UDP `connect()` succeeds (it only sets the peer address), so the self-check uses neither.
- **Files.** The child can read what ALL APPLICATION PACKAGES can (the Windows folder, Program Files) and what Sona grants its SID: read and execute on the program's folder (the same executable, the shim DLL, a bundled GStreamer), GStreamer's `bin`, `lib` and `libexec` (found from where `libgstreamer-1.0-0.dll` is loaded; `GST_PLUGIN_*` folders the environment names), the CDM's folder (`SONA_CDMHOST_CDM_DIR`, inside Sona's data folder) and the shim DLL's folder (`SONA_CDM_SHIM`, development builds). Everything else is out of reach: Sona's config, data and state (settings, `credentials.json`, the database, the log), the user's profile and the other users'. It can write only to its package folder (`%LOCALAPPDATA%\Packages\sona.cdmhost\AC\sona-host`: its temporary files and GStreamer's plugin registry). The parent adds one access-control entry naming only the profile's SID (read and execute, inherited) to each folder the SID cannot read yet; it never removes a thing, replaces a list or grants a drive root (`grantAccess` refuses one).
- **Job object.** Killed when the parent's last handle on it closes, so the child cannot outlive the parent even if the parent crashes; one active process; no desktop, clipboard, global atom, display-setting or system-parameter access; no breakaway.
- **Process.** No child processes (`PROCESS_CREATION_CHILD_PROCESS_RESTRICTED`: GStreamer scans plugins in the process, `GST_REGISTRY_FORK=no`, into a registry in the package folder that later children read; `DETACHED_PROCESS`, because a console process would start `conhost.exe`, which the restriction refuses); heap corruption ends the process; bottom-up ASLR; no legacy extension points or fonts from outside; no images from remote shares or from files with a low integrity label (a file the child wrote cannot be loaded, plugin registry entries included). The only handles it inherits are its three pipes (an explicit handle list). It is suspended until it is in the job.
- **Environment.** `childEnv` drops credential-like names as everywhere; the parent sets `TEMP` and `TMP` to the package folder, and `GST_REGISTRY_1_0` to a registry file there, one per `GST_REGISTRY_1_0` setting of the parent and never the parent's own file: the UI process loads GStreamer plugins from its registry (the output device list), so a child that could write that file could name a plugin for the parent to load. Windows points `%LOCALAPPDATA%` and `%APPDATA%` of an AppContainer process into its package folder, so the parent names the folders to hide (`SONA_CDMHOST_HIDDEN_DIRS`).
- **Self-check.** The child checks its own token (`TokenIsAppContainer`, integrity level low), that the outbound `connect()` to 192.0.2.1:9 (TEST-NET-1) fails with `WSAEACCES`, and that it cannot list Sona's folders or the user's `AppData\Local` and `AppData\Roaming` (at least one must exist and none may be listable). It reports `Sandbox.AppContainer`, `NetworkDenied`, `FilesHidden`; Diagnostics shows "Sandboxed by Windows" only when all three hold.
- **No sandbox, no CDM.** Where the profile, the folder grants or the process creation fail (an old Windows, a policy that refuses AppContainers, a program folder the user cannot change and ALL APPLICATION PACKAGES cannot read), the parent starts the child unconfined and says why (`parentSandbox.reason`, gap `no_appcontainer`), but the child **does not load the CDM** unless the AppContainer and the network refusal are in place (`Sandbox.loadsCDM`, `ErrSandbox`, as on Linux and unlike macOS): Diagnostics then reads "Blocked" and "Not sandboxed", and only previews and library uploads play. A test binary alone honours `SONA_CDMHOST_ALLOW_UNSANDBOXED=1` (`testing.Testing()`), for a machine that refuses AppContainers; no release does.

**Tests** (`sandbox_windows_test.go`; windows-2025 in `ci.yml`'s "Windows build", step "Sandbox tests", with `SONA_CDMHOST_REQUIRE_SANDBOX=1` so a refusal fails them and a skip fails the step; the whole host and native suites then run through the sandbox, since the child cannot load the CDM otherwise): `TestAppContainerConfinesChild` starts a probe child through `startSandboxed` and checks that its outbound `connect()` and its connection to a loopback listener of the test fail (the listener sees nothing), that it reads nothing in a folder that was not granted, writes nothing under the profile, lists no app data folder and starts no process, and that it reads and loads the real CDM and writes its temp and registry folders; `TestAppContainerKill`; `TestSandboxedHostReports` (a real host reports `Active()` and the `appcontainer` kind); `TestGrantAccess`, `TestReadFolders`, `TestSandboxLoadsCDM`, `TestSandboxGaps`, `TestEnvHelpers`; `TestRemoveSandbox` (below). `build/windows/smoke.sh`, in `release.yml`, installs and silently uninstalls the installer and checks that the profile is gone.

**Removal** (`cdmhost.RemoveSandbox`, `sona --remove-sandbox`). The Windows uninstaller runs `sona.exe --remove-sandbox` before it deletes the files; the portable `.zip` has no uninstaller, so its user runs the same command (from the unpacked folder, before deleting it). It removes the explicit entries of the profile's SID from the access lists of the folders the parent grants (the program's, GStreamer's and the `GST_PLUGIN_*` folders, found as at a start; the entries the files under them inherited go with them) and from every folder under the Widevine folder (`<data>\widevine`: the CDM itself, which is the user's data, stays, only the entries on its folders go), and then calls `DeleteAppContainerProfile` and removes the package folder under `%LOCALAPPDATA%\Packages` if Windows left it. A step that fails does not stop the others; the exit status is non-zero and the message names each failure. It finds nothing wrong with what is already gone, so it can run again. `TestRemoveSandbox` creates a profile of its own (never `sona.cdmhost`, which the other tests use), grants it a program folder and a CDM folder with a file, runs the removal and checks that the profile's registry mapping and package folder, and every entry of its SID, including the file's inherited one, are gone, that the CDM file stays, and that a second run succeeds.

**Residual risks (Windows):**

- **Audio output from the AppContainer is not verified on hardware.** The runners have no audio endpoint, so CI plays to `fakesink`. WASAPI works for AppContainer processes through the audio service without a capability (what UWP apps do) [INFERENCE]; if a Windows build refuses it, the symptom is no sound with a working CDM, and the fix is a capability in the profile, which this section would then list. Verify on a real PC with the first Windows release.
- **The access entries stay while Sona is installed.** The entries for the profile's SID on the program, GStreamer and CDM folders are not removed when the child exits (removing them would rewrite thousands of entries at every start); they go with the folders, or with `sona --remove-sandbox` (the installer's uninstaller runs it). The profile stays in the user's registry (`HKCU`) and its package folder under `%LOCALAPPDATA%\Packages` until then. A portable `.zip` deleted without running the command leaves them behind: the profile, its package folder, and the entries on the Widevine folder in the user's data. They grant read and execute to an identity only Sona's child holds, and the profile is only a name for that identity.
- **Same-user processes.** An AppContainer protects the user from the child, not the child from the user's other processes: a process of the user at medium integrity can open the child (the parent does) and read the decrypted audio from its memory, as with any player; the child cannot open them (low integrity and its own SID).
- **Kernel and win32k.** The child shares the kernel with everything; `win32k` is reachable and the mitigations stop short of arbitrary-code guard and code-integrity guard, which the CDM and GStreamer's plugins would not survive. A kernel or `win32k` exploit in the CDM or a GStreamer plugin is not contained.
- **Read access to the system.** The child reads the Windows folder and Program Files, and the program's, GStreamer's and the CDM's folders; the folders it can write hold nothing it can load (the low-integrity image mitigation).
- **In-process plugin scan.** With no scanner process, a plugin that crashes while GStreamer scans takes the child down the first time (the registry then excludes it, or the child fails the same way each time); the scan is slower than the forking scanner's first run.
- **Motion artwork decodes in the UI process.** The UI process (not the sandboxed child) plays the looping covers: with Media Foundation (DXVA on Direct3D 11) on Windows, AVFoundation (VideoToolbox) on macOS and libav through GStreamer on Linux, from files the motion cache fetched from Apple's CDNs only (F6): the same exposure on all three, with the user's rights and no sandbox. On Windows and macOS the decoders are the system's own (Sona packages and links no video codec there, and the package holds no video plugin, so the CDM host child's plugin scan has none to load), serviced with the OS. The files are compressed video from Apple's servers; the CDM host's decrypted media never reaches it.
- **Self-check limits.** `NetworkDenied` is one refused connect, `FilesHidden` four folder listings; they catch a child that is not in the AppContainer, not every hole in it. Offline, with no route at all, the refused `connect()` may fail with another error than `WSAEACCES`, so the check says the network is not refused and the CDM is not loaded until the next start online [INFERENCE: not observed].
- **Windows 10 and later only**, the AppContainer APIs and the mitigation policies it uses; `windows-2025` is the only version tested.

## Workflows

The repository's GitHub Actions run on GitHub-hosted runners only (a self-hosted runner reachable from a pull request was ruled out). Once the repository is public, anyone can open a pull request, so the workflows assume the pull request's code is hostile.

**What there is to protect.**

| Asset | Where it lives | Who can read it |
| --- | --- | --- |
| `MUSIC_USER_TOKEN` (a real account's Music User Token, about six months' access to its library and playback) | Secret of the `live` environment | Jobs that name `environment: live` and run from `main`: `live.yml` (weekly schedule, dispatch), `windows-playback.yml` (weekly schedule, dispatch) |
| Last.fm API key and secret, Discord application ID (compiled into every release binary, so readable by anyone who has one; the risk is someone else's builds or abuse posing as Sona before a release) | Secrets of the `release` environment | Jobs that name `environment: release` and run from `main` or a `v*` tag: `release.yml`'s build job (not a dispatched dry run, which has no environment), `release-secrets.yml` (dispatch, prints only whether each is set) |
| `OP_SERVICE_ACCOUNT_TOKEN` (Renovate's 1Password service account, which yields the Renovate app's key) | Repository secret, maintained with Renovate (`renovate.yaml`) | Any workflow run on a branch of this repository; never a fork's run |
| bot-cecil GitHub App credentials (`RENOVATE_APP_CLIENT_ID`, `RENOVATE_APP_PRIVATE_KEY`; the App's token is how release-please and the release publish step write, as Renovate does) | Repository secrets | Workflows run on a branch of this repository; `release-please.yml` and the `publish` job of `release.yml` mint a job-scoped token from them (contents, pull requests, issues write for release-please; contents write to publish) |
| `GITHUB_TOKEN` write scopes | Per job | `live` (`issues: write`, to report a failed scheduled run), `release.yml`'s build job (`id-token` and `attestations: write`, for the build provenance); every other job reads only; releases are written with the App token |
| Release assets and tags | GitHub releases | The `publish` job, after the CI checks and the build; the `release tags` ruleset keeps `v*` tags immutable |

**Threats and controls.**

- *A fork's pull request runs its own code in CI.* Every workflow a pull request triggers uses `pull_request`, never `pull_request_target` or `workflow_run`, so a fork's run gets a read-only `GITHUB_TOKEN` and no secrets; GitHub withholds both. None of those workflows (`ci.yml`, `pr-title.yml`) reads a secret other than `GITHUB_TOKEN`. Environment secrets stay out of reach even for a pull request from a branch of this repository: an environment deploys only to the refs in its policy (`release`: `main` and `v*` tags; `live`: `main`), and a pull request's ref is `refs/pull/<n>/merge`.
- *A token that outlives its job.* Every checkout sets `persist-credentials: false`, so the token isn't left in `.git/config` for later steps (tests, build scripts) to read. No workflow pushes with git.
- *Too much token scope.* Each workflow starts from `permissions: contents: read` (`pr-title.yml`: `pull-requests: read`; `release-secrets.yml`: none), and only the two jobs above widen it (`release.yml`'s build job for the provenance, `live`). The release PR, tag and release are written with a job-scoped bot-cecil App token instead. The repository's default workflow permission is read.
- *A third-party action moves its tag.* Every action is pinned to a full commit SHA, with the version in a comment; Renovate proposes updates to the pins, which are reviewed like any other pull request.
- *Script injection.* No `run:` step expands an attacker-controlled expression (pull request title, branch name, commit message); values reach scripts through `env:`.
- *A malicious dependency in CI.* Go modules are checked against `go.sum`, and Go is the only toolchain the build uses (no Node, no pnpm, no package-manager install scripts). A compromised dependency still runs its code in a pull request's job, which holds nothing worth taking (above).
- *Cache poisoning.* Caches written by a pull request's run are scoped to its ref; the default branch's caches, which every run can restore, are written only by runs on `main`. The cached Widevine CDM is reinstalled through Sona's verifying downloader (`cmd/cdmfetch`, "Widevine CDM downloader").
- *The live token in a test.* `live.yml` runs with `SONA_PRIVATE=1`, so nothing it does reaches the account's listening history, and plays nothing. `windows-playback.yml` plays one track through the CDM host on a Windows runner, also with `SONA_PRIVATE=1` and `playspike`, which reports nothing to Apple's play-activity service; it builds `main` only (never a branch the dispatcher names), so the secret only meets reviewed code, and prints the licence verdict, never the token or the licence.
- *Hosted-runner cost.* The macOS and Windows jobs in `ci.yml` run only when dispatched while the repository is private, and on every push and pull request once it is public.

**Residual risks.**

- Anyone with write access can push a `v*` tag, and with it a workflow that names `environment: release`. A tag ruleset that limits who may create `v*` tags closes that; once releases run from `main` alone, the `v*` rule can leave the environment's policy.
- `OP_SERVICE_ACCOUNT_TOKEN` is a repository secret, readable by a workflow on any branch of this repository (not a fork's), including the branches Renovate's app pushes. Moving it into an environment limited to `main` would confine it to `renovate.yaml`'s own runs.
- While the repository is private, forks' pull requests run no workflows at all ("Run workflows from fork pull requests" is off, and so are write tokens and secrets for them). Once it is public, a first-time contributor's pull request waits for a maintainer's approval before CI runs ("Require approval for outside contributors"); the controls above are what holds after that approval.

## Secret scanning

**The guard** (`.github/workflows/secret-scan.yml`, on pull requests and pushes to `main`, read-only token, no secrets): gitleaks 8.30.1 from its release, checked against the release's SHA-256, over every commit of every branch and tag (a pull request's own commits included, since they stay public through it), then over the pull request's title and body (read from the event payload, since a step's `env:` is printed in the log). The public log shows each finding's rule, commit, file and line only, as an annotation: gitleaks' own output prints the rest of the line around a secret even with `--redact`. `.gitleaks.toml` adds to gitleaks' defaults:

| Rule | Catches |
| --- | --- |
| `jwt-segment` | A JWT's header segment and dot, so also a developer token split across string literals |
| `apple-music-user-token` | A long token assigned to, or sent as, `Music-User-Token`, `media-user-token` or `MUSIC_USER_TOKEN` |
| `long-base64-token` | A base64 run of 150+ characters with high entropy (a MUT without its name) |
| `lastfm-api-credential` | 32 hex characters next to a Last.fm key or secret name |
| `discord-client-id` | Replaces the default rule, which takes 18 digits only; Discord IDs have 17 to 20 (Sona's has 19) |
| `secret-file` | `.env`, `.env.*`, `.p8`, `.p12`, `.pfx`, `.pem`, `.key`, keystores, SSH keys, by name |

Each allowlist entry is commented and tied to its file: the fake developer tokens in `internal/logging/logging_test.go` and `internal/diag/diag_test.go` (only their two exact fake payloads), the fake Discord ID in `logging_test.go` (only that value), the SRP vectors (`internal/auth/appleid/testdata/srp_vectors.json`, made from fixed test bytes) and Renovate's `<!--renovate-debug:…-->` comment in the pull request text (`pull-request.md` only, and only a base64 run that opens as Renovate's JSON does, `{"createdInVer"`: with a few labels it passes `long-base64-token`'s 150 characters). After changing a rule, check it against planted fakes on a branch you never push: a one-line and a split developer token, a MUT in a header map, a Last.fm secret, a Discord ID, a `.p8` key, a `.env.production`, a fixture's fake payload copied to another file and a new token added to an allowlisted file must all be found.

Locally: `nix run nixpkgs#gitleaks -- git --config .gitleaks.toml --redact .` (all local branches) or `… dir --config .gitleaks.toml --redact .` (the working tree, uncommitted files included).

**Residual risks:**
- A secret pushed in any commit, even one later amended, force-pushed or squashed away, stays fetchable from GitHub by its SHA and through its pull request. If the scan fires, rotate the credential first; removing the commit comes second.
- Pull-request and issue text and Actions logs can't be rewritten with the history. The workflow scans titles and bodies; comments are not scanned.
- A pull request can change `.gitleaks.toml` itself; changes to it need the maintainer's review.
- Shapes the rules can't recognise (a token split into short pieces, encoded differently, or under an unrelated name) pass.
