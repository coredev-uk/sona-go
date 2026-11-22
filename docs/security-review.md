# Security review

This document is Sona's security design and its audit record. It gives the threat model and the controls for each component that handles untrusted input, then lists the weaknesses found in Sona's own code (F1–F28), the control that closed each one and the tests that keep it closed.

To report a vulnerability, follow [SECURITY.md](../SECURITY.md). The CI threat model (workflows, secrets, the release bot) is maintainer documentation: [docs/maintainers/ci-and-secrets.md](maintainers/ci-and-secrets.md). For how the pieces fit together, see [architecture.md](architecture.md) and [auth.md](auth.md).

## Contents

- [Security model at a glance](#security-model-at-a-glance)
- [Components](#components)
  - [Widevine CDM downloader](#widevine-cdm-downloader)
  - [Linux sandbox](#linux-sandbox)
  - [macOS sandbox](#macos-sandbox)
  - [Windows sandbox](#windows-sandbox)
  - [Native add-ons](#native-add-ons)
  - [Music videos](#music-videos)
  - [Apple's translations](#apples-translations)
  - [Lyrics from other services](#lyrics-from-other-services)
  - [TuneIn metadata](#tunein-metadata)
  - [Local broadcaster streams](#local-broadcaster-streams)
  - [WebAssembly add-ons](#webassembly-add-ons)
  - [The item inspector](#the-item-inspector)
  - [Sona's own add-ons](#sonas-own-add-ons)
  - [Secret scanning](#secret-scanning)
  - [Vulnerability scanning](#vulnerability-scanning)
- [Findings](#findings)
  - [Summary](#summary)
  - [F1. Settings import could set the CDM path](#f1-settings-import-could-set-the-cdm-path)
  - [F2. The CDM child sandbox blocked only inet sockets](#f2-the-cdm-child-sandbox-blocked-only-inet-sockets)
  - [F3. The sandbox failed open](#f3-the-sandbox-failed-open)
  - [F4. Discord IPC trusted any user's socket](#f4-discord-ipc-trusted-any-users-socket)
  - [F5. NaN or infinite MPRIS volume](#f5-nan-or-infinite-mpris-volume)
  - [F6. Motion artwork followed redirects anywhere](#f6-motion-artwork-followed-redirects-anywhere)
  - [F7. Another local user could complete the loopback sign-in](#f7-another-local-user-could-complete-the-loopback-sign-in)
  - [F8. A compromised child could exhaust the parent's memory](#f8-a-compromised-child-could-exhaust-the-parents-memory)
  - [F9. Artwork decoding accepted any registered image format](#f9-artwork-decoding-accepted-any-registered-image-format)
  - [F10. Development builds logged the build credentials](#f10-development-builds-logged-the-build-credentials)
  - [F11. The child could reach the user's terminals](#f11-the-child-could-reach-the-users-terminals)
  - [F12. On Ubuntu the CDM host child could reach the session bus](#f12-on-ubuntu-the-cdm-host-child-could-reach-the-session-bus)
  - [F13. Release builds ran third-party tools without a pin or a checksum](#f13-release-builds-ran-third-party-tools-without-a-pin-or-a-checksum)
  - [F14. CI and release images were trusted by a tag any branch could overwrite](#f14-ci-and-release-images-were-trusted-by-a-tag-any-branch-could-overwrite)
  - [F15. The release bot's App key was readable by branch runs](#f15-the-release-bots-app-key-was-readable-by-branch-runs)
  - [F16. A NUL byte or invalid UTF-8 in on-air metadata broke MPRIS](#f16-a-nul-byte-or-invalid-utf-8-in-on-air-metadata-broke-mpris)
  - [F17. The sona command printed other people's text to the terminal unfiltered](#f17-the-sona-command-printed-other-peoples-text-to-the-terminal-unfiltered)
  - [F18. The diagnostics bundle carried the person's city and device names](#f18-the-diagnostics-bundle-carried-the-persons-city-and-device-names)
  - [F19. The motion-artwork remuxer panicked on a huge tfhd base offset](#f19-the-motion-artwork-remuxer-panicked-on-a-huge-tfhd-base-offset)
  - [F20. The fMP4 parser panicked on crafted init segments and fragments](#f20-the-fmp4-parser-panicked-on-crafted-init-segments-and-fragments)
  - [F21. Server-declared counts and lengths allocated without limit](#f21-server-declared-counts-and-lengths-allocated-without-limit)
  - [F22. Artwork from the CDN was decoded without a pixel limit](#f22-artwork-from-the-cdn-was-decoded-without-a-pixel-limit)
  - [F23. Local broadcaster streams followed any URL](#f23-local-broadcaster-streams-followed-any-url)
  - [F24. CDM message events started unbounded renewal goroutines](#f24-cdm-message-events-started-unbounded-renewal-goroutines)
  - [F25. The Last.fm approval token was in the browser's command line](#f25-the-lastfm-approval-token-was-in-the-browsers-command-line)
  - [F26. The update manifest's version and platform were not signed](#f26-the-update-manifests-version-and-platform-were-not-signed)
  - [F27. The loopback sign-in could be completed with another account's token](#f27-the-loopback-sign-in-could-be-completed-with-another-accounts-token)
  - [F28. On macOS any program could read Sona's keychain items without a prompt](#f28-on-macos-any-program-could-read-sonas-keychain-items-without-a-prompt)
- [Areas reviewed without findings](#areas-reviewed-without-findings)
- [Run the tests](#run-the-tests)

## Security model at a glance

Each row is a trust boundary. The sections below give the detail.

| Boundary | Trusted with | Main control |
| --- | --- | --- |
| UI process | The user's session, settings and tokens; drawing text | Strings from Apple, lyrics services and TuneIn are drawn as plain text, never markup or code. Artwork decodes only as JPEG or PNG ([F9](#f9-artwork-decoding-accepted-any-registered-image-format)). Settings import cannot set local keys ([F1](#f1-settings-import-could-set-the-cdm-path)) |
| CDM host child, Linux | Running Google's closed CDM on Apple's encrypted media, nothing else | Own user, network and mount namespaces, Landlock, seccomp and a scrubbed environment ([Linux sandbox](#linux-sandbox)). The `.deb` and `.rpm` ship the AppArmor profile Ubuntu needs for the namespaces ([F12](#f12-on-ubuntu-the-cdm-host-child-could-reach-the-session-bus)). No sandbox, no CDM ([F3](#f3-the-sandbox-failed-open)) |
| CDM host child, macOS | The same | Seatbelt profile: no network, no writes outside temporary folders, Sona's folder and keychain out of reach. Fails open ([macOS sandbox](#macos-sandbox)) |
| CDM host child, Windows | The same | AppContainer with no capabilities inside a job object. No sandbox, no CDM ([Windows sandbox](#windows-sandbox)) |
| Add-ons, native (out of process) | Their own folder, a private data folder and what the user approved (`network` included); nothing else of the user's system | On every OS the process is confined by the system, and where that can't be set up it does not run: user, mount and PID namespaces, Landlock and seccomp on Linux, a Seatbelt profile on macOS, an AppContainer in a job object on Windows ([Native add-ons](#native-add-ons)) |
| Network fetches | Fixed `https` hosts for each feature | Host allow-lists checked on every redirect (at most 5), size caps, timeouts, no cookies or Apple credentials where none are needed |
| Local files and settings | The user's own data | Imported settings cannot name a library or a local key. The child cannot read or write Sona's folders. Files are written atomically |
| Secrets and tokens | The keyring, with a 0600 file fallback | Tokens never reach URLs, errors or logs. Build credentials are redacted from logs and diagnostics ([F10](#f10-development-builds-logged-the-build-credentials)). Secret scanning guards the repository ([Secret scanning](#secret-scanning)) |
| Loopback sign-in server | Apple's sign-in redirect on `127.0.0.1` | The owner uid of each connection, single-use start links, a cookie ([F7](#f7-another-local-user-could-complete-the-loopback-sign-in)) and a per-attempt state in the callback path ([F27](#f27-the-loopback-sign-in-could-be-completed-with-another-accounts-token)) |
| Add-ons, WebAssembly runtime | Running a third party's module inside Sona's process, with the user's grant | No ambient authority: no file but its data folder (links refused), no network, no process; memory and CPU capped. The module is checked and read once; the compiled cache is private ([WebAssembly add-ons](#webassembly-add-ons)). The exposure is the runtime itself: a bug in wazero is a bug in Sona's process |
| Item inspector (Settings → Diagnostics → Developer tools) | The user's own item data, shown to the user | Off by default and only offered when the person turns it on (always in a development build). Reads Go's caches through one service method, never Apple; every text passes `logging.Redact`; no request header, token or credential is ever held. Nothing leaves the machine ([The item inspector](#the-item-inspector)) |
| Add-ons, Sona's own | Sona's own signed code, run as `sona --addon <id>`, holding the Last.fm and ListenBrainz sign-ins | Registered in code, never from a folder. Sona keeps the private filter and the queue and sends only filtered listens; `scrobble.*` requests go to bundled add-ons only ([Sona's own add-ons](#sonas-own-add-ons)) |
| Other local users | Nothing | Discord IPC socket and named pipe ownership checks ([F4](#f4-discord-ipc-trusted-any-users-socket)); the loopback checks above |
| Third-party services | Apple, Google, Mozilla, Musixmatch, LRCLIB, NetEase, TuneIn | Trusted for what their text says, not for code. The Widevine package needs two Google signatures. Lyrics sources can be switched off in Settings → Playback → Lyrics |

## Components

Each component lists what it does, the threats, the controls, the tests and the residual risks.

### Widevine CDM downloader

**What it does.** Sona downloads the Widevine CDM itself (`internal/playback/cdm`), with no prompt, once the network opens after the first paint. It checks for updates daily.

- Browser and system copies are never used.
- There is no other source: no setting, environment variable or flag names a library.
- The CDM is never bundled in a package or the binary, never redistributed and never patched.

**Threats.** A network or mirror attacker gets Sona to install and `dlopen` a library that is not Google's genuine Widevine CDM.

**Controls.**

- **Manifest trust and pinning** (`manifest.go`):
  - The version, size, SHA-512 and download URLs come from Mozilla's Firefox GMP manifest for Widevine (`toolkit/content/gmp-sources/widevinecdm.json`), the file Firefox's own GMP updater falls back to.
  - Sona fetches it pinned to two URLs, in order: the GitHub mirror (`raw.githubusercontent.com/mozilla-firefox/firefox/refs/heads/main/…`), then `hg.mozilla.org/mozilla-central/raw-file/tip/…`.
  - Only `raw.githubusercontent.com`, `hg.mozilla.org` and `hg-edge.mozilla.org` are allowed, redirects included. The document is capped at 1 MiB.
  - `hashFunction` must be `sha512` and every hash 64 bytes; anything else is `manifest_invalid`.
  - The version must be numeric (`^[0-9]{1,6}(\.[0-9]{1,6}){1,3}$`), because it names an install directory.
  - A platform entry's `alias` is followed for at most three hops. A platform with no entry (Linux arm64 today) is `unsupported_platform`.
  - The declared size must be between 1 byte and 64 MiB.
- **Package hosts** (`manifest.go`, `download.go`):
  - The file URL and its mirrors are used only when they are `https`, without user info, on the default port and on `edgedl.me.gvt1.com`, `www.google.com` or `dl.google.com`.
  - Other URLs are dropped, and an entry with none left is `untrusted_url`.
  - Every redirect is checked against the same list (at most 5). Each URL is tried in turn.
- **Download limits** (`download.go`):
  - The package streams to a temp file in the install directory, reading at most the manifest's size plus one byte. A larger `Content-Length` is refused up front as `too_large`.
  - A transfer that receives nothing for 60 s is cancelled.
  - The size and SHA-512 must match the manifest (`hash_mismatch`).
- **CRX3 verification** (`crx3.go`), as Chromium's `crx_verifier.cc` does it:
  - The header is parsed by hand (magic `Cr24`, version 3, header at most 256 KiB, at most 16 proofs), not by a protobuf library.
  - A header containing a ZIP end-of-central-directory token (`PK\x05\x06`, `PK\x06\x07`, `PK\x06\x06`) is refused, so the archive cannot parse differently from what was signed.
  - The signed header's CRX ID must be `oimompecagnajdejgnnjijobebaeigek`, the Widevine component's ID.
  - There must be a proof by the Widevine component key and one by Google's publisher key.
  - Every RSA and ECDSA proof in the header must verify (SHA-256 over `CRX3 SignedData\x00`, the signed header's length and bytes, and the archive). Any failure is `signature_invalid`.
- **Extraction** (`extract.go`, `binfmt.go`):
  - Only three entries are written, each under a fixed name: the platform's library, its `.sig` file (macOS and Windows) and the root `manifest.json`. All else is ignored.
  - These are refused: unsafe names (empty, absolute, `..` elements, backslashes, NUL, drive letters) anywhere in the archive, a wanted entry appearing twice, a non-regular entry and more than 64 entries.
  - An entry over its limit is refused too: 64 MiB for the library, 64 KiB for the others, enforced on the bytes read whatever the header claims. The CRC is checked.
  - The component manifest must name `WidevineCdm` at the manifest's version.
  - The library's format must match the platform: a 64-bit ELF shared object for the right machine, a Mach-O dylib (or universal slice) for the right CPU, or a PE DLL.
- **Install** (`install.go`, `store.go`, `lock_*.go`):
  - Versions live in `<data>/widevine/<version>/<platform>/`. A staging directory is renamed into place, and `current.json` (current, previous, rejected versions, consecutive failures, last check) is replaced by an atomic rename. There are no symlinks.
  - A cross-process lock (`flock` on `.lock`, `LockFileEx` on Windows) serialises installs between Sona, `cdmfetch`, `playspike` and tests.
  - A version is never downgraded: a manifest version not newer than the current one changes nothing, so a stale or replayed manifest can only point at older genuine packages, which are not installed.
  - Old versions and leftovers are pruned. A new version applies to the next playback host, never a running one.
- **Load check before a version becomes current:**
  - The new version must load in a sandboxed probe child that loads the CDM and nothing else (`cdmhost.LoadCheck`, 1 min; no GStreamer).
  - If the CDM fails to load or initialise, or the child dies loading it (`cdm.ErrNotLoadable`), the version is deleted. When a working version exists, it is also recorded as rejected and never fetched again.
  - A check that cannot finish (out of time, or the child could not start) proves nothing either way. The verified files stay as `pending` in `current.json` with their library's SHA-256.
  - The next check of the same version loads them again if they still match; otherwise they are removed and downloaded again.
  - A pending version is validated like current and previous (a version pattern, so it cannot name a path outside the directory).
- **Repair** (`store.repair`):
  - `current.json` records the SHA-256 of each kept version's library, taken at install.
  - `store.repair` drops a kept version whose library is missing or does not match. Its directory is removed, a damaged current version falls back to an intact previous one, else to none, and the same version is downloaded again (not rejected, since the package was fine).
  - It runs when the manager starts (after first paint), on every update check, in `Ensure` (`playspike`, `cdmtest`, `cdmfetch`), and at once when a playback host fails to load the CDM. Such a failure then does not count towards rollback and the download starts immediately.
- **Rollback:** after 3 consecutive failed host starts with the current version, the previous version becomes current again and the failed one is removed and rejected. Hosts using the override are not counted.
- **Sandbox** (`cdmhost/host.go`, `sandbox_fs_linux.go`):
  - The parent passes the CDM library's directory to the child (`SONA_CDMHOST_CDM_DIR`).
  - The child keeps it in its filesystem view and bind-mounts it read-only wherever it lies, also under the private `/tmp`, where Landlock would otherwise allow writes.
  - Code in the CDM cannot replace the installed library or `current.json`.

**Tests.**

- `internal/playback/cdm`: `manifest_test.go`, `crx3_test.go`, `extract_test.go`, `install_test.go` and `manager_test.go` run against fixture packages signed with test keys. `live_test.go` runs against the real manifest and package.
- `internal/playback/cdmhost` `TestLoadCheck*`: the check never starts GStreamer, a broken library is `ErrNotLoadable`, running out of time is not.

**Residual risks.**

- Sona trusts Mozilla's manifest (its repository and the GitHub mirror) for which version to fetch, and Google's Widevine component key and publisher key for what is genuine.
- A compromised manifest could withhold updates, point at an unsupported or older platform entry (refused if older than the current one, but accepted on a first install), or make every check fail.
- It cannot make Sona install anything not signed by both Google keys and served from Google's hosts, since the CRX3 proofs, the CRX ID and the component manifest are checked independently of it.
- A genuine but older Google build is acceptable on a first install. It is still Google's signed CDM.
- When the playback sandbox is unavailable, the load check is skipped (logged) and the CDM host refuses to load the CDM anyway ([F3](#f3-the-sandbox-failed-open)).
- The CDM itself remains Google's closed code running in the sandboxed child (the [Linux sandbox](#linux-sandbox) residual risks apply).

### Linux sandbox

**What it does.** The CDM host child runs Google's closed CDM, so the parent starts it confined (`internal/playback/cdmhost/sandbox_linux.go`, `sandbox_fs_linux.go`, `host.go`). It is the layered successor of a sandbox that blocked only inet sockets ([F2](#f2-the-cdm-child-sandbox-blocked-only-inet-sockets)).

**Threats.** Code running in the CDM leaves the sandbox or steals data through:

- the environment (including `DBUS_SESSION_BUS_ADDRESS`)
- path Unix sockets (session and system bus, resolved, agents, X11), for example D-Bus with systemd's `StartTransientUnit`
- the home directory
- the parent process (`/proc/<pid>/mem`, `environ`, `ptrace`)

**Controls.**

- **Environment.**
  - `childEnv` drops `DBUS_*`, `DISPLAY`, `WAYLAND_*`, `XAUTHORITY`, `SSH_*`, `GPG_*`, `GNOME_KEYRING*`, `AT_SPI_*`, `KRB5*` and `SESSION_MANAGER`.
  - It also drops anything named like a credential (`TOKEN`, `SECRET`, `PASSW`, `CREDENTIAL`, `COOKIE`, `API_KEY`, `PRIVATE_KEY`, `AUTH`).
  - The child starts in `/`.
- **Session.** The child starts in a session of its own (`Setsid`) on every spawn path, so it has no controlling terminal: `open("/dev/tty")` fails with `ENXIO` and it cannot push keystrokes into Sona's terminal ([F11](#f11-the-child-could-reach-the-users-terminals)).
- **Namespaces and filesystem.** The child starts in a new user, network and mount namespace. Its first stage (holding `CAP_SYS_ADMIN` in its own user namespace only, as an ambient capability) changes the view as follows:
  - `/run`, `/tmp`, `/var/tmp` and `$XDG_RUNTIME_DIR` become empty tmpfs. `/run`'s top-level symlinks are recreated. `/dev/pts` becomes an empty read-only tmpfs, so the user's terminals are not there at all.
  - Only these are bound back: the PipeWire socket (or PulseAudio's when there is no PipeWire), and the program's own files when they live under a covered directory (`APPDIR`, the executable's directory, `LD_LIBRARY_PATH`, the GStreamer plugin paths and scanner, the CDM's directory).
  - The home directory becomes recursively read-only (`mount_setattr`).
  - Sona's config and state directories, `~/.ssh`, `~/.gnupg`, `~/.password-store` and the keyrings are hidden under empty read-only tmpfs.
- **Landlock** (applied with or without namespaces, at whatever ABI the kernel offers):
  - No file writes outside `/dev/null`, `/dev/shm` and `/dev/snd`, plus the private `/tmp` when there is one.
  - No device ioctls outside those paths (`LANDLOCK_ACCESS_FS_IOCTL_DEV`, ABI 5), which includes `TIOCSTI` on any terminal.
  - No TCP or UDP.
  - `connect()` to path Unix sockets only for the audio socket (ABI 9).
  - Abstract Unix sockets and signals are scoped to the sandbox (ABI 6).
  - Landlock's ptrace rule also denies `/proc/<pid>/{mem,environ}` of every process outside the sandbox.
- **Two-stage start.**
  - The first stage restricts its own thread and then re-executes `/proc/self/exe`.
  - So the second stage starts with no capabilities, `no_new_privs` set, and every thread inside the Landlock domain.
  - It refreshes GStreamer's registry first, while the home directory is still writable.
- **Seccomp** (second stage, TSYNC):
  - `socket` and `socketpair` are allowed only for `AF_UNIX`.
  - These fail with EPERM: `ptrace`, `process_vm_readv/writev`, `pidfd_getfd`, `kcmp`, `io_uring_*`, `bpf`, `perf_event_open`, `userfaultfd`, the keyring calls, `unshare`, `setns`, the mount calls and `clone` with namespace flags.
  - `clone3` fails with ENOSYS, and glibc then uses `clone`.
  - The child checks that `AF_INET`, `AF_INET6` and `AF_NETLINK` are refused.
- **AppArmor profile** (the `.deb` and `.rpm`; `build/linux/apparmor/sona`, installed as `/etc/apparmor.d/sona`):
  - Ubuntu 23.10 and later set `kernel.apparmor_restrict_unprivileged_userns=1`: an unconfined program that creates a user namespace gets one without capabilities, so the child's first stage cannot build its filesystem view.
  - The profile (`profile sona /usr/bin/sona flags=(unconfined)` with `userns,`) lets the installed binary create user namespaces with capabilities. `flags=(unconfined)` means AppArmor mediates nothing else of Sona. It is the profile Chrome and Discord use on Ubuntu 24.04.
  - `postinstall.sh` loads it when `apparmor_parser` and AppArmor are present; `preremove.sh` unloads it on removal (not on upgrade). It is a conffile: local changes survive upgrades, and `/etc/apparmor.d/local/sona` is included.
  - The Arch package does not ship it (no Arch kernel carries the restriction). An AppImage cannot: it runs from a changing mount point, and a profile that matched that path would let any file there create namespaces.
- **Without unprivileged user namespaces** (Flatpak, user namespaces disabled) **or under Ubuntu's AppArmor restriction without the profile** (an AppImage, or a binary installed outside `/usr/bin/sona`):
  - The child runs with Landlock and seccomp only. Under the restriction it keeps its network namespace but gets no private filesystem view ("make mounts private: permission denied", seen on GitHub's ubuntu-24.04 runners).
  - Below Landlock ABI 9 (every Ubuntu 24.04 kernel) nothing then refuses `connect()` to path Unix sockets, so the child can reach the session bus ([F12](#f12-on-ubuntu-the-cdm-host-child-could-reach-the-session-bus)). Seccomp cannot close this: it sees the address's pointer, not the path.
  - Diagnostics says so (`Sandbox.ReachesSockets`, gap `sockets`: "It can connect to your session bus and other Unix sockets", with the way to fix it). The CDM still loads ([F3](#f3-the-sandbox-failed-open) needs seccomp and the network refusal only), so playback keeps working.
- **`PR_SET_DUMPABLE=0` is not set in the parent.**
  - Measured: a non-dumpable process gets `Portal operation not allowed: Unable to open /proc/<pid>/root` from every xdg-desktop-portal call (Settings, and with it the file chooser for settings import/export).
  - Crash capture (`debug.SetCrashOutput`) would not have been affected.
  - The child cannot trace the parent anyway: seccomp denies `ptrace` and `process_vm_*`, and Landlock denies its `/proc` memory and environment.
- **Realtime scheduling from outside:**
  - PipeWire's module-rt needs D-Bus to ask RTKit for realtime scheduling. So the child reports its audio threads over the pipe, and the parent asks RTKit (or the Realtime portal) for them. See [architecture.md](architecture.md) §4.2, "Realtime priority".
  - The seccomp filter is unchanged: RTKit changes the threads' scheduling from outside, and the child needs only `sched_getscheduler` to find them.

**Tests.**

- `TestSandboxConfinesChild` runs a probe child that enters the real sandbox and tries each escape. The `namespaces` subtest uses the namespace path, `landlock only` the fallback. It checks that:
  - The child cannot connect to the session bus, the system bus, resolved's socket (where present), or test sockets in `$XDG_RUNTIME_DIR` and `/tmp`, nor to an abstract socket.
  - It cannot write under `$HOME`.
  - It cannot open `AF_INET`/`AF_INET6`/`AF_NETLINK`.
  - It cannot read the parent's `/proc/<pid>/environ` or call `process_vm_readv` on it.
  - It does reach the PipeWire socket.
- `TestSandboxGapsReportReachableSockets` (a child without a mount namespace reports `ReachesSockets` and the `sockets` gap below Landlock ABI 9 and not from it; with the view it never does). In `landlock only`, the assertion on path sockets states its condition: refused from ABI 9, reachable below it, and the reported gap agrees with what the probe observed.
- `TestChildEnvDropsSessionAndSecrets`, `TestChildCannotOpenInetSockets` (also requires the mount namespace), `TestSandboxFallsBackWithoutNamespaces`.
- **When tests skip.** `TestChildCannotOpenInetSockets` and the `namespaces` subtest skip, with the reason, only where the system refuses the namespaces:
  - EPERM or EACCES when a probe child of the test's own creates user, network and mount namespaces (Flatpak, user namespaces disabled), or changes its mounts in them (Ubuntu 24.04's AppArmor restriction creates the namespaces but denies their capabilities).
  - Any other failure, including a sandbox that misses a layer the system allows, fails them.
  - CI lifts Ubuntu's restriction (`kernel.apparmor_restrict_unprivileged_userns=0`) and sets `SONA_CDMHOST_REQUIRE_NAMESPACES=1`, so there they must run.
- The DRM path regression (`go run ./cmd/playspike -seek 90`) prints the sandbox it got (its `sandbox` line: kind, namespaces, Landlock ABI, seccomp, `inet_denied`, reason) and plays a full track inside it.
- Realtime scheduling:
  - `TestThreadReportRaisesChildThreads`: a child marks a thread the way libpipewire does and reports it with PID 1's thread. The parent asks only for the child's own, lowers its `RLIMIT_RTTIME`, and shows the grant in `Info`.
  - `TestAudioThreadsBounded` (at most 8 threads, each once).
  - `TestRaise` (the decisions: priority cap, nice fallback, refusals, no broker, foreign threads).
  - `TestBusBroker` (RTKit's and the portal's D-Bus interface on a private bus).

**Residual risks.**

- The PipeWire (or PulseAudio) socket stays reachable, because output needs it. The audio server is trusted. A PulseAudio server would also accept `load-module` requests.
- Files in the home directory stay readable, except the hidden directories.
  - Read data has no way out to the network: the child has no network, no bus and no writes outside `/dev/null`, `/dev/shm` (shared) and `/dev/snd`.
  - Its only other channel is the pipe to the parent, whose CDM messages go to Apple's licence server.
- A compromised child can get up to 8 of its own threads realtime (`SCHED_RR` at RTKit's `MaxRealtimePriority`, 20 by default, which the PipeWire server's own threads match or exceed).
  - It reports thread IDs only; the parent asks only for threads under `/proc/<child>/task`, each once.
  - Before asking, the parent lowers the child's `RLIMIT_RTTIME` (soft and hard) to RTKit's `RTTimeUSecMax` (200 ms by default). The child has no `CAP_SYS_RESOURCE` to raise it again, so a realtime thread that runs that long without blocking is killed.
  - RTKit's watchdog also demotes every realtime thread when it sees the system starve.
  - Where neither RTKit nor the Realtime portal answers, or they refuse, the audio threads run at normal priority (Diagnostics → Audio priority says why).
- On kernels without Landlock:
  - The fallback has seccomp only, so path sockets and home writes are reachable.
  - In both paths, `/proc/<pid>/mem` and `environ` of other same-user processes are guarded only by Yama and ordinary ptrace rules.
- Without a private filesystem view, below Landlock ABI 9, the child can reach path Unix sockets, the session bus among them, and so the impact of [F2](#f2-the-cdm-child-sandbox-blocked-only-inet-sockets) (systemd's `StartTransientUnit`) returns. This is the state of:
  - an AppImage on Ubuntu 23.10 or later, and any install where the AppArmor profile is not loaded or does not match the binary's path;
  - Flatpak, whose own bus proxy still limits what the session bus offers;
  - any system without unprivileged user namespaces and with a kernel below ABI 9.
  - Sona does not refuse playback there: that would end full-song playback for every Ubuntu AppImage user. Diagnostics reports it, and the packages, the kernel (Landlock ABI 9) or a namespace policy that allows Sona's binary close it. Using the `.deb` is the safe choice on Ubuntu.

### macOS sandbox

**What it does.** macOS has none of the Linux layers (namespaces, Landlock, seccomp, `prctl`). The CDM host child there runs under Seatbelt, macOS's own sandbox, through `/usr/bin/sandbox-exec -p <profile> <sona>`.

- `internal/playback/cdmhost/seatbelt.go` writes the profile and `sandbox_darwin.go` applies it.
- The profile starts from `(allow default)` and takes away what the child never needs. Later rules win, so each exception follows its rule.

**Controls.** This table compares the Linux layers with the Seatbelt profile.

| Area | Linux | macOS (Seatbelt) |
| --- | --- | --- |
| Network | Own network namespace; seccomp refuses `AF_INET`/`AF_INET6`/`AF_NETLINK` sockets; Landlock: no TCP/UDP | `(deny network*)`: no TCP, UDP or Unix-socket connect, bind or listen. Core Audio talks Mach IPC, not sockets |
| Session services | Session and system bus sockets gone (fresh `/run`, `$XDG_RUNTIME_DIR`); Landlock allows only the audio socket | No bus to hide; the keychain's Mach services are denied (`com.apple.SecurityServer`, `com.apple.securityd`, `com.apple.securityd.xpc`, `com.apple.security.agent`, `com.apple.security.authhost`): the child is Sona's own executable, which Sona's keychain items trust ([F28](#f28-on-macos-any-program-could-read-sonas-keychain-items-without-a-prompt)). Other Mach services stay reachable (Core Audio needs `coreaudiod`) |
| File writes | Only `/dev/null`, `/dev/shm`, `/dev/snd` and the private `/tmp` (Landlock); home read-only | Only `/dev` (except the terminals: `tty`, `ttys*`, `pty*`, `console`, no write or ioctl), `/private/tmp`, `/private/var/tmp`, `/private/var/folders` (`$TMPDIR`) and GStreamer's registry cache (`$XDG_CACHE_HOME`/`~/.cache/gstreamer-1.0`, or `$GST_REGISTRY_1_0`'s folder) |
| Hidden files | Sona's config and state, `~/.ssh`, `~/.gnupg`, `~/.password-store`, the keyrings; the downloaded CDM's folder is bound back read-only | Sona's folder (`~/Library/Application Support/Sona`: settings, `credentials.json`, the database, the log), `~/Library/Keychains`, `~/.ssh`, `~/.gnupg`, `~/.password-store`: neither read nor written. The one exception is the downloaded CDM's folder (`<data>/widevine/<version>/mac_arm64`, from `SONA_CDMHOST_CDM_DIR`: the dylib and its `.sig`), readable but not writable, plus the metadata of the folders above it so the path resolves |
| Terminals | New session (no controlling terminal); `/dev/pts` covered by an empty tmpfs; Landlock refuses writes and device ioctls outside `/dev/null`, `/dev/shm`, `/dev/snd` | New session (`Setsid`); the profile denies write and ioctl on `/dev/tty`, `/dev/ttys*`, `/dev/pty*`, `/dev/console` [INFERENCE: not run on a Mac] |
| Other processes | seccomp denies `ptrace`/`process_vm_*`; Landlock's ptrace scope | Not restricted by the profile; macOS's own rules apply (no `task_for_pid` on another process without entitlements) [INFERENCE] |
| Environment | `childEnv` drops session addresses and credential-like names | The same `childEnv` |
| Self-check | Child tries `socket(AF_INET…)`; refused = `InetDenied` | Child tries a UDP `connect()` to `127.0.0.1:9` (sends nothing) and listing `~/Library/Keychains`; both must fail with `EPERM`/`EACCES` (`InetDenied`, `SecretsHidden`) |
| No sandbox | The CDM is not loaded (`ErrSandbox`, [F3](#f3-the-sandbox-failed-open)) | The child runs unconfined and **still loads the CDM**; Diagnostics' Host sandbox row says "Not sandboxed" with the reason |

Whether the sandbox is used is decided once per process, on the first host start (`seatbeltChoice`). It is not used when:

- `SONA_SEATBELT=0` is set (a development switch, for telling a sandbox problem from a playback one)
- `/usr/bin/sandbox-exec` is missing
- it refuses the profile (a probe runs `/usr/bin/true` under it)

`playspike` prints the outcome (`sandbox kind=seatbelt seatbelt=true inet_denied=true secrets_hidden=true reason=""`), and Diagnostics shows it ("Sandboxed by macOS", or "Not sandboxed" and why).

**Why macOS fails open.**

- macOS releases exist (ad-hoc-signed, `build/darwin/bundle.sh`, no hardened runtime), but Seatbelt is still fail-open.
- `sandbox-exec` is marked deprecated by Apple, though it still ships (Chromium's macOS sandbox uses the same Seatbelt engine through libsandbox's `sandbox_init_with_parameters`). A user must not lose playback if a macOS update removes it.
- A fail-closed sandbox, as on Linux, is still to do.

**Tests, and what is not covered yet (verify on a Mac).**

- The macOS build is type-checked from Linux (`go run ./cmd/typecheck -os darwin -arch arm64 ./...`). The profile text and the decision are unit-tested there (`TestSeatbeltProfile`, `TestSeatbeltChoice`).
- `TestSeatbeltConfinesChild` (macOS only) starts a real child and a probe under the profile: no TCP connect, no write to the home folder, no listing of the keychain folder, but temporary files.
- `ci.yml`'s "macOS build and test" job runs it. It runs when dispatched, and on pushes and pull requests once the repository is public.
- A hand run on a real Mac belongs to the first native release's verification. Open questions for that run:
  - that Seatbelt's "later rule wins" holds for the write exceptions and for the CDM folder inside Sona's hidden folder
  - that Core Audio, GStreamer's registry and the CDM work under the profile
  - that the CDM needs none of the denied Mach services

**Residual risks.**

- Files in the home folder outside the hidden ones stay readable (as on Linux).
- Mach services other than the keychain's stay reachable.
- The CDM is Google's downloaded and verified library (see [Widevine CDM downloader](#widevine-cdm-downloader)), loaded unmodified. macOS's library validation does not apply to an unsigned host (none is hardened) [INFERENCE].

### Windows sandbox

**What it does.** Windows has none of the Linux layers and no `sandbox-exec`. The CDM host child there runs in an **AppContainer**, the OS's own sandbox for processes that must touch neither the user's files nor the network (Chromium's renderers use one too), inside a **job object**.

- `internal/appcontainer` makes the profile, grants its SID the folders and starts the process in it; `internal/playback/cdmhost/appcontainer_windows.go` says which folders the CDM host reads and `sandbox_windows.go` holds the decision and the child's self-checks.
- The child is the same `sona.exe`, started with `CreateProcess` and `PROC_THREAD_ATTRIBUTE_SECURITY_CAPABILITIES`.
- Go's `os/exec` cannot pass that attribute, so the parent creates the process itself and keeps its own handle for the kill (Go's handle from `os.FindProcess` cannot terminate it).

**Controls.**

- **Identity and network.**
  - The profile `sona.cdmhost` (`CreateAppContainerProfile`, created on the first spawn and kept in the user's registry) holds no capabilities: not `internetClient`, not `privateNetworkClientServer`.
  - Windows checks every access of the child twice, against the user and against the profile's SID (and ALL APPLICATION PACKAGES), and the network stack refuses all the child's traffic.
  - Observed on windows-2025: a TCP `connect()` to an address off the machine fails at once with `WSAEACCES`, and a connection to a listener of the parent's on the loopback gets no answer.
  - Two things the AppContainer does *not* stop: the child can connect to a socket of its own on the loopback, and a UDP `connect()` succeeds (it only sets the peer address). So the self-check uses neither.
- **Files, reads.**
  - The child can read what ALL APPLICATION PACKAGES can (the Windows folder, Program Files).
  - It can also read what Sona grants its SID: read and execute on the program's folder (the same executable, the shim DLL, a bundled GStreamer), GStreamer's `bin`, `lib` and `libexec` (found from where `libgstreamer-1.0-0.dll` is loaded; `GST_PLUGIN_*` folders the environment names), the CDM's folder (`SONA_CDMHOST_CDM_DIR`, inside Sona's data folder) and the shim DLL's folder (`SONA_CDM_SHIM`, development builds).
  - Everything else is out of reach: Sona's config, data and state (settings, `credentials.json`, the database, the log), the user's profile and the other users'.
- **Files, writes.** The child can write only to its package folder (`%LOCALAPPDATA%\Packages\sona.cdmhost\AC\sona-host`: its temporary files and GStreamer's plugin registry).
- **Files, grants.**
  - The parent adds one access-control entry naming only the profile's SID (read and execute, inherited) to each folder the SID cannot read yet. It never removes a thing, replaces a list or grants a drive root (`grantAccess` refuses one).
  - The grant is Sona's, at run time, before every child starts: the installer grants nothing, and the uninstaller only takes the entries away.
- **Files, after an update.**
  - An update replaces the install's files by renaming them in from a folder beside it (MyGo's `swapFiles`). A rename on the same volume keeps the access list a file was made with, so the new files do not have the folder's entry (observed on windows-2025, and setting the folder's list again does not hand it down).
  - The program is therefore checked as well (`grantNeeded`). When it lacks the entry the tree inherits the folder's list again (`TreeSetNamedSecurityInfo`, `TREE_SEC_INFO_RESET_KEEP_EXPLICIT`: explicit entries and the folder's own protection stay, nothing is added that the folder's list does not hold), and the log says `cdmhost: granted the sandbox read access to a folder`.
- **Job object.**
  - Killed when the parent's last handle on it closes, so the child cannot outlive the parent even if the parent crashes.
  - One active process; no desktop, clipboard, global atom, display-setting or system-parameter access; no breakaway.
- **Process.**
  - No child processes (`PROCESS_CREATION_CHILD_PROCESS_RESTRICTED`). GStreamer scans plugins in the process (`GST_REGISTRY_FORK=no`) into a registry in the package folder that later children read. `DETACHED_PROCESS` is set, because a console process would start `conhost.exe`, which the restriction refuses.
  - Heap corruption ends the process; bottom-up ASLR; no legacy extension points or fonts from outside.
  - No images from remote shares or from files with a low integrity label (a file the child wrote cannot be loaded, plugin registry entries included).
  - The only handles it inherits are its three pipes (an explicit handle list). It is suspended until it is in the job.
- **Environment.**
  - `childEnv` drops credential-like names as everywhere. The parent sets `TEMP` and `TMP` to the package folder.
  - It also sets `GST_REGISTRY_1_0` to a registry file there, one per `GST_REGISTRY_1_0` setting of the parent and never the parent's own file. The UI process loads GStreamer plugins from its registry (the output device list), so a child that could write that file could name a plugin for the parent to load.
  - Windows points `%LOCALAPPDATA%` and `%APPDATA%` of an AppContainer process into its package folder, so the parent names the folders to hide (`SONA_CDMHOST_HIDDEN_DIRS`).
- **Self-check.**
  - The child checks its own token (`TokenIsAppContainer`, integrity level low), that the outbound `connect()` to 192.0.2.1:9 (TEST-NET-1) fails with `WSAEACCES`, and that it cannot list Sona's folders or the user's `AppData\Local` and `AppData\Roaming` (at least one must exist and none may be listable).
  - It reports `Sandbox.AppContainer`, `NetworkDenied`, `FilesHidden`. Diagnostics shows "Sandboxed by Windows" only when all three hold.
- **No sandbox, no CDM.**
  - Where the profile, the folder grants or the process creation fail (an old Windows, a policy that refuses AppContainers, a program folder the user cannot change and ALL APPLICATION PACKAGES cannot read), the parent starts the child unconfined and says why (`parentSandbox.reason`, gap `no_appcontainer`).
  - The child **does not load the CDM** unless the AppContainer and the network refusal are in place (`Sandbox.loadsCDM`, `ErrSandbox`, as on Linux and unlike macOS). Diagnostics then reads "Blocked" and "Not sandboxed", and only previews and library uploads play.
  - A test binary alone honours `SONA_CDMHOST_ALLOW_UNSANDBOXED=1` (`testing.Testing()`), for a machine that refuses AppContainers; no release does.
- **Removal** (`cdmhost.RemoveSandbox`, `sona --remove-sandbox`):
  - The Windows uninstaller runs `sona.exe --remove-sandbox` before it deletes the files. The portable `.zip` has no uninstaller, so its user runs the same command (from the unpacked folder, before deleting it).
  - It removes the explicit entries of the profile's SID from the access lists of the folders the parent grants (the program's, GStreamer's and the `GST_PLUGIN_*` folders, found as at a start; the entries the files under them inherited go with them).
  - It also removes them from every folder under the Widevine folder (`<data>\widevine`: the CDM itself, which is the user's data, stays, only the entries on its folders go).
  - Then it calls `DeleteAppContainerProfile` and removes the package folder under `%LOCALAPPDATA%\Packages` if Windows left it.
  - It does the same for every add-on's AppContainer (`sandbox.RemoveProfiles`, profiles `sona.addon.<id>`): the entries of its SID on the add-on's folder (`<config>\addons\<id>`), its data folder (`<state>\addons\<id>\data`, which stays) and, for Sona's own add-ons, the program's folders, then the profile and its package folder. The IDs come from disk, with no list kept: Sona's own, the folders of the installed add-ons, and the folders of `<state>\addons` (each add-on that ever ran has one, and Sona never removes them, so an add-on uninstalled earlier is found). `ProfileIDs` accepts only names shaped like an add-on ID. Only the names are listed; the data is never read.
  - A step that fails does not stop the others; the exit status is non-zero and the message names each failure. It finds nothing wrong with what is already gone (an add-on that never ran has no profile), so it can run again.

**Tests** (`sandbox_windows_test.go`).

- They run on windows-2025 in `ci.yml`'s "Windows build", step "Sandbox tests", with `SONA_CDMHOST_REQUIRE_SANDBOX=1` so a refusal fails them and a skip fails the step. The whole host and native suites then run through the sandbox, since the child cannot load the CDM otherwise.
- `TestAppContainerConfinesChild` starts a probe child through `startSandboxed` and checks that:
  - its outbound `connect()` and its connection to a loopback listener of the test fail (the listener sees nothing)
  - it reads nothing in a folder that was not granted, writes nothing under the profile, lists no app data folder and starts no process
  - it reads and loads the real CDM, writes its temp and registry folders, and registers a thread with MMCSS's "Pro Audio" task as wasapi2sink's render thread does (the capability-less token is enough)
- `TestAppContainerKill`; `TestSandboxedHostReports` (a real host reports `Active()` and the `appcontainer` kind); `TestSandboxedHostReportsMMCSS` (a sandboxed host playing through `wasapi2sink`, muted, reports its render thread at MMCSS's priority; skipped without an audio endpoint, so on the runners); `TestGrantAccess`, `TestReadFolders`, `TestSandboxLoadsCDM`, `TestSandboxGaps`, `TestEnvHelpers`.
- `TestRemoveSandbox` creates a profile of its own (never `sona.cdmhost`, which the other tests use), grants it a program folder and a CDM folder with a file, and runs the removal. It checks that:
  - the profile's registry mapping and package folder, and every entry of its SID (including the file's inherited one), are gone
  - the CDM file stays
  - a second run succeeds
- `build/windows/smoke.sh`, in `release.yml`, installs and silently uninstalls the installer and checks that the profile is gone.
- Add-ons' profiles are tested by `TestRemoveProfiles` (Native add-ons, below).

**Residual risks.**

- **Audio output from the AppContainer is not verified on hardware.**
  - The runners have no audio endpoint, so CI plays to `fakesink`.
  - WASAPI works for AppContainer processes through the audio service without a capability (what UWP apps do) [INFERENCE].
  - If a Windows build refuses it, the symptom is no sound with a working CDM, and the fix is a capability in the profile, which this section would then list. Verify on a real PC with the first Windows release.
- **The access entries stay while Sona is installed.**
  - The entries for the profile's SID on the program, GStreamer and CDM folders are not removed when the child exits (removing them would rewrite thousands of entries at every start). They go with the folders, or with `sona --remove-sandbox` (the installer's uninstaller runs it).
  - The profile stays in the user's registry (`HKCU`) and its package folder under `%LOCALAPPDATA%\Packages` until then. The same holds for each add-on's profile.
  - A portable `.zip` deleted without running the command leaves them behind: the profiles (the host's and the add-ons'), their package folders, and the entries on the Widevine folder in the user's data. They grant read and execute to an identity only Sona's child holds, and the profile is only a name for that identity.
- **Same-user processes.** An AppContainer protects the user from the child, not the child from the user's other processes. A process of the user at medium integrity can open the child (the parent does) and read the decrypted audio from its memory, as with any player; the child cannot open them (low integrity and its own SID).
- **Kernel and win32k.** The child shares the kernel with everything; `win32k` is reachable and the mitigations stop short of arbitrary-code guard and code-integrity guard, which the CDM and GStreamer's plugins would not survive. A kernel or `win32k` exploit in the CDM or a GStreamer plugin is not contained.
- **Read access to the system.** The child reads the Windows folder and Program Files, and the program's, GStreamer's and the CDM's folders. The folders it can write hold nothing it can load (the low-integrity image mitigation).
- **In-process plugin scan.** With no scanner process, a plugin that crashes while GStreamer scans takes the child down the first time (the registry then excludes it, or the child fails the same way each time). The scan is slower than the forking scanner's first run.
- **Motion artwork decodes in the UI process.**
  - The UI process (not the sandboxed child) plays the looping covers: with Media Foundation (DXVA on Direct3D 11) on Windows, AVFoundation (VideoToolbox) on macOS and libav through GStreamer on Linux. The files come from the motion cache, which fetched them from Apple's CDNs only ([F6](#f6-motion-artwork-followed-redirects-anywhere)).
  - The exposure is the same on all three, with the user's rights and no sandbox.
  - On Windows and macOS the decoders are the system's own, serviced with the OS. Sona packages and links no video codec there, and the package holds no video plugin, so the CDM host child's plugin scan has none to load.
  - The files are compressed video from Apple's servers; the CDM host's decrypted media never reaches it.
- **Self-check limits.**
  - `NetworkDenied` is one refused connect, `FilesHidden` four folder listings. They catch a child that is not in the AppContainer, not every hole in it.
  - Offline, with no route at all, the refused `connect()` may fail with another error than `WSAEACCES`. The check then says the network is not refused and the CDM is not loaded until the next start online [INFERENCE: not observed].
- **Windows 10 and later only**, the AppContainer APIs and the mitigation policies it uses; `windows-2025` is the only version tested.

### Native add-ons

A native add-on (docs/addons.md) is a program somebody else wrote that Sona runs beside itself: the one component that runs code the user chose rather than code Sona was released with. It speaks protocol v2 over its standard input and output, and #304 puts the OS around the process.

**What it does.** It hears Sona's events and sends calls, each behind a permission the user approved; it contributes menu entries, palette commands and settings rows that Sona draws; it may keep files in a data folder of its own. Bundled add-ons (#302) are Sona's own executable started with `--addon <id>`; they run under the same sandbox.

**Threats.** An add-on that is malicious from the start, or turned so by an update or a compromised dependency, wants to:

- read what the user's account can read: documents, `~/.ssh`, browser profiles, the keyring and Sona's own settings and tokens;
- reach local services of the user's session: the session and system D-Bus (and through it the Secret Service), the agent sockets, X11 and Wayland, the audio server;
- send what it reads to a server, or scan the user's network;
- push keystrokes into the user's terminal (`TIOCSTI`) or read it, which is a shell;
- signal, trace or read the memory of the user's other processes, Sona's among them;
- leave a process behind that outlives Sona;
- use kernel attack surface that no add-on needs (namespaces, `io_uring`, `bpf`, `ptrace`) to get out of its sandbox;
- read or write another add-on's data.

**What the sandbox gives an add-on.**

| | Linux | macOS | Windows |
| --- | --- | --- | --- |
| Its own folder | Read-only and runnable, a bind mount in its own root | Read | Read and execute (an access-list entry for its AppContainer) |
| Its data folder (`<state>/addons/<id>/data`, `SONA_ADDON_DATA`) | Read and write, never run (`noexec`, and no Landlock execute right) | Read and write | Read and write (an entry for its AppContainer) |
| The rest of the user's files | Not there: the add-on's root is a new tmpfs that holds only what is listed here | Home, `/Users`, `/Volumes`, the temporary folders and the keychain folder are denied to read and write | Not granted: an AppContainer reaches only what its SID was granted, and the system folders `ALL APPLICATION PACKAGES` may read |
| System programs and libraries | `/usr`, `/bin`, `/sbin`, `/lib*`, the Nix and Guix stores, read-only; a short list of `/etc` files (certificates, resolver, `passwd`, `nsswitch.conf`, the loader cache, the time zone) | Everything that is not denied above stays readable | The Windows folder and Program Files |
| Without `network` | A network namespace with no interface (not even the loopback is up); `socket()` refused for every family but an anonymous `AF_UNIX` pair; Landlock denies TCP (and UDP where its ABI has it) | `(deny network*)` | No capability: the network stack refuses every connection |
| With `network` | The host's network namespace; `socket()` for `AF_INET` and `AF_INET6` only (no `AF_UNIX`, `AF_NETLINK`, `AF_PACKET`) | The network is not denied | The `internetClient` capability, which is the internet only: not the loopback, not the private network |
| Unix sockets | None exist in its root; `socket(AF_UNIX)` is refused unless code names a socket (the Discord add-on's, [the one exception](#the-discord-add-ons-socket-exception)) | Denied with the network (a named socket is allowed back) | None reachable |
| Other processes | Its own PID namespace: it sees only itself and its children, and can't signal, trace or read Sona or anything else | Not hidden; same-user signals are possible | Not reachable from an AppContainer; the job limits it to 64 processes |
| The terminal | New session with no controlling terminal; the standard descriptors are pipes, checked by the sandbox's init; no `/dev/tty`, `/dev/pts` or any terminal in its root | New session; the profile denies the terminal devices, to read, write and `ioctl` | Created detached, with no console |
| Its processes end with Sona | `Pdeathsig` on the init, and the init is PID 1: when it dies the kernel kills everything in the namespace, children that left the process group included | Process group; an add-on must exit when its input ends | Job object with kill-on-close |

**Linux, step by step.** Sona starts a copy of its own executable (`/proc/self/exe`, with a request in the environment) in new user, mount, PID, IPC and UTS namespaces, and a network namespace unless the add-on has `network`; the user namespace maps the user's uid onto itself and gives the copy `CAP_SYS_ADMIN` in it alone. That copy is the add-on's init (`internal/addons/sandbox`: `linux_helper.go`, `linux_view.go`, `linux_confine.go`):

1. Closes every descriptor it doesn't need and refuses to go on if one of the standard three is a terminal.
2. Builds the new root: a tmpfs with read-only bind mounts of the system's files, the add-on's folder and the extra read folders, a `noexec` bind of the data folder, a `/dev` of its own with `null`, `zero`, `full`, `random` and `urandom`, and a `/proc` of its own; `pivot_root`s into it, drops the old root and remounts the new root read-only. Every source is opened before the first mount, so one under `/tmp` (an AppImage) still binds.
3. Sets `no_new_privs` and applies Landlock (the same places, read and run, read and write, plus the scopes of abstract Unix sockets and signals where the ABI has them) when the kernel has it.
4. Clears its ambient capabilities, installs the seccomp filter (the CDM host's list: `ptrace`, `process_vm_*`, `io_uring`, `bpf`, `perf_event_open`, `userfaultfd`, keyrings, `unshare`, `setns`, the mount calls and `clone` with namespace flags fail with `EPERM`; `clone3` and foreign ABIs with `ENOSYS`; `socket` only for the families above).
5. Starts the add-on from that thread (so it inherits Landlock and the filter), reports to Sona on a pipe that the add-on's own descriptors don't include, and stays as init: it passes `SIGTERM` and the other termination signals on, reaps orphans and exits with the add-on's status.

Sona never opens, lists or follows anything inside the data folder (it only makes the folder and a `tmp` folder in it; a link in `tmp`'s place is removed, never followed), so a hostile add-on can't point Sona at a file.

**When the sandbox can't be set up.** Third-party add-ons fail closed. `addons.Start` returns a `*sandbox.Error`; the supervisor does not start the add-on, does not restart it (that is not a crash), logs the reason in the add-on's log, and Settings → Add-ons says it in plain words (`AddonNoSandbox`). There is no setting, environment variable or flag that runs such an add-on without the sandbox: only a Go test binary can (`testing.Testing()` and `SONA_ADDON_UNSANDBOXED=1`, the pattern of `SONA_CDMHOST_ALLOW_UNSANDBOXED`), and the services' tests use it to test the supervisor on any machine.

| Reason | Cause | Way out |
| --- | --- | --- |
| `userns_restricted` | Ubuntu 23.10 and later: `kernel.apparmor_restrict_unprivileged_userns=1` creates the user namespace without its capabilities unless an AppArmor profile allows the program that asks | An AppArmor profile for Sona's program that allows `userns`, or `sudo sysctl -w kernel.apparmor_restrict_unprivileged_userns=0` |
| `no_userns` | The kernel (`user.max_user_namespaces=0`, Debian's `kernel.unprivileged_userns_clone=0`), a container or the Flatpak sandbox refuses user namespaces | None in Sona: allow them, or run the `.deb`, RPM or AppImage |
| `no_seccomp` | The syscall filter can't be built (an architecture other than amd64 and arm64) or installed | None |
| `no_seatbelt` | `/usr/bin/sandbox-exec` is missing, refused the profile, or the profile did not hide the user's home when probed | None |
| `no_appcontainer` | Windows could not create the profile, grant the folders or start the process in the AppContainer | Run the add-on from a folder the account owns |
| `unsupported` | An operating system with no sandbox for add-ons | None |
| `setup_failed` | Anything else (a mount refused, `pivot_root` failing in a container whose root is not a mount, an init that ends before it reports, which says its exit status, 127 being the dynamic loader failing); the add-on's log has the detail | |

**The exception: Sona's own add-ons (`lastfm`, `listenbrainz`, `discord`).** Where `addons.Start` fails with a `*sandbox.Error`, the supervisor starts a code-registered bundled add-on again with `Spec.Unsandboxed`: a plain child process with Sona's rights (still in its own process group or job object, with the environment allowlist, its data folder, credentials only in `hello`, and the protocol and permission checks). The decision is made on the registry (`Entry.Bundled`, from `BundledEntries`), never on the ID string, so a user folder claiming a reserved ID is still refused by the scanner; `Spec.Unsandboxed` is set only in that one place. The log gets a `sona:` note with the reason code and `AddonInfo.Unsandboxed` carries it to Settings → Add-ons ("Running without isolation: …").
- *Rationale:* before #302 the same code, shipped inside the signed release, ran in Sona's process with Sona's rights, so this is no wider than what Sona did then. Without it Last.fm and Discord stopped working in Sona's Flatpak and on stock Ubuntu 23.10 and later.
- *Cost:* on those hosts these three add-ons have no isolation: a vulnerability in their code (parsing a Last.fm or ListenBrainz reply, Discord's IPC) is a vulnerability in a process with the user's rights, with the network and the keyring item `addon.<id>`, and the Discord add-on can reach any socket it chooses, though it dials only the sockets Sona checked (`SONA_DISCORD_IPC`).
- *Not changed:* third-party native add-ons and `addons dev` runs; WebAssembly add-ons have no sandbox to lose.

**What Sona's own add-ons need to start: the loader's path, no more binds.** A bundled add-on is Sona's own executable, so before it can say anything the dynamic loader must find the libraries that executable links. The published Nix package's binary has no rpath: its wrapper sets `LD_LIBRARY_PATH` (`build/linux/nix/release.nix`, `runtime.nix`), and the supervisor's environment allowlist left it out, so on NixOS the sandbox's init (the same binary) and then the plain child both ended at the loader with status 127 (`error while loading shared libraries: libgstapp-1.0.so.0`), five times in a row, and the crash rule saved all of them off. The fix is in the allowlist and widens nothing else:

- *`LD_LIBRARY_PATH` is inherited by Sona's own add-ons only* (`bundledEnv`; `TestEnvironmentGivesBundledAddonsTheLoaderPath`). `LD_PRELOAD`, `LD_AUDIT`, `GST_*` and every other variable of Sona's environment stay out, as for any add-on, and an add-on of the user's never has `LD_LIBRARY_PATH`. The folders it names are Sona's own install's: the same ones that already bound read-only into the sandbox (`sandbox.ProgramReads`: the executable's folder, `APPDIR`, the folders of `LD_LIBRARY_PATH`; `TestLoaderPathAndItsFolderReachTheAddon`), so the path adds no place to read and the add-on can't write to any of them.
- *No new read-only bind.* On NixOS the executable, its loader and its libraries are in `/nix/store`, which every add-on's root already binds read-only (`systemPaths`). Binding the executable's own closure instead was considered: on Nix it is `/nix/store` again, and elsewhere the folders `LD_LIBRARY_PATH` names are `sandbox.ProgramReads` already.
- *A bundled add-on is never switched off by the crash rule.* The rule exists so that a third-party add-on that keeps crashing stops costing CPU and log; it saved `enabled: false` for Sona's own too, which turned Last.fm and Discord off for good while Settings → Connections still said they were on. Now the supervisor starts them again every 30 seconds for as long as Sona runs, saves nothing, and ignores any saved `enabled` of theirs (a value rc.3 left is written back as `true` at the next start). Their on/off is Settings → Connections' alone. `AddonInfo.Reason` says why one isn't up (`ended_at_start`, `no_hello`, `stopped`, `start_failed`), so a failure is on the page and not only in the log.

Sona doesn't ship a privileged helper to get around a restricted user namespace, and a user-visible "run without the sandbox" switch is a deliberate non-feature: the sandbox is what makes approving an add-on a bounded decision, and the add-ons that run without it are the ones in Sona's own code.

**Residual risks and where a platform makes it weaker.**

- **`network` is any address.** An add-on with it can reach every host and port the user's computer can, the user's LAN and the loopback services (a database on `127.0.0.1`) included, on Linux and macOS. The label says "Reach the internet"; its sentence says "any address on the network". Sona doesn't broker the traffic (that is only enforceable under the WebAssembly host, #303). On Windows the `internetClient` capability is narrower: the internet, not the private network or the loopback, so an add-on that talks to a device at home needs the other platforms for now.
- **No resource limits.** The sandbox sets no CPU, memory, disk, process or file limits on Linux and macOS (Windows' job limits the process count): an add-on can fill its data folder or the machine's memory. A cgroup is not available to an unprivileged process.
- **Kernel attack surface.** The seccomp filter denies a list, not everything but a list: an add-on can still call every other system call, and a kernel bug in one is out of the sandbox's reach. Landlock, the namespaces and `no_new_privs` are second and third layers.
- **Linux without Landlock** (a kernel before 5.13, or one that doesn't enable it): the root, the namespaces and the filter confine the add-on and the second check of files is missing. The add-on's log says so on each start.
- **macOS is a deny-list.** The profile is `(allow default)` with the user's data denied, as the CDM host's. What it doesn't name stays reachable: the rest of the system's files (`/etc`, `/Library`, `/Applications`), the Mach services other than the keychain's (the clipboard, distributed notifications, Apple Events), and other processes of the user's. An add-on with `network` can also connect to Unix sockets. `sandbox-exec` is deprecated; its absence fails closed. Nothing of the macOS sandbox was run in the pull request that added it: the profile's text is tested on Linux, the profile is probed on each start (it must accept a program and hide the home directory, or the add-on doesn't run), and the rest is compiled for macOS.
- **Windows.** An AppContainer reaches the system folders `ALL APPLICATION PACKAGES` may read, so an add-on's runtime must be in the Windows folder, Program Files or the add-on's own folder (a Python in the user's profile is not readable). The SID of each add-on's AppContainer keeps its access-list entries and its profile until `sona --remove-sandbox` (the installer's uninstaller runs it) removes them: see the removal below. An add-on that `sona addons dev` ran from a folder outside the add-ons folder leaves the entry on that folder, which only the developer's own command and the profile's SID know; the profile itself goes.
- **Bundled add-ons** are Sona's own executable and are confined as any other; they read Sona's install folder (to run) and not the user's config, state or keyring. The Discord add-on is the one that is given Unix sockets (Discord's IPC ones, named in code, no more), and F4's checks apply to them: [The Discord add-on's socket exception](#the-discord-add-ons-socket-exception).
- **The protocol is the channel.** Everything an add-on may do to the rest of Sona goes through the permissions of protocol v2, checked on every call; the sandbox does not widen it and the permissions do not widen the sandbox, except `network`, which has no call.

**Tests.**

- `internal/addons/sandbox` (Linux, run by CI's playback job with the AppArmor restriction lifted and `SONA_ADDON_REQUIRE_SANDBOX=1`, which turns a skip into a failure):
  - an add-on without `network` can't connect to a TCP listener, open a netlink socket or a Unix socket, and one with it can connect and still can't open a Unix or netlink socket;
  - it can't read a file in the user's home outside its folders, list `/home`, `/root`, `/run`, `/sys`, `/mnt` or `/var`, write to its own folder, `/` or `/tmp`, or run a program from its data folder, and it can write to its data folder and read its own;
  - it has no controlling terminal, can't open `/dev/tty` or list `/dev/pts`, and none of its descriptors is a terminal; it holds no descriptor but its channel's;
  - it sees at most itself in `/proc` and can't signal Sona; the request is not in its environment;
  - a Unix socket is reachable only when the policy names it, and the one named in a folder leaves its neighbours out of reach;
  - `Kill` ends a process that left the add-on's session;
  - the loader's `LD_LIBRARY_PATH` reaches the add-on and the folder it names is readable there (`TestLoaderPathAndItsFolderReachTheAddon`); an init that dies before it reports says how it ended (`TestAnInitThatEndsWithoutAReportSaysHowItEnded`).
- `internal/addons` and `internal/services`: the environment allowlist and the data folder (`SONA_ADDON_DATA`, `HOME`, `TMPDIR`), a data folder never removed, a sandbox that can't start leaves the add-on refused with the reason and not restarted, and revoking `network` stops the add-on. For Sona's own add-ons: `LD_LIBRARY_PATH` in both the sandboxed and the plain start (`TestBundledAddonsGetTheLoaderPathInBothStarts`), no switch-off by the crash rule and a `Reason` meanwhile (`TestAddonsBundledAreNeverSwitchedOffByTheCrashRule`), and a stuck saved `enabled: false` healed at start (`TestAddonsBundledStuckSwitchesHealAtStart`).
- `internal/addons/sandbox/seatbelt_test.go`: the profile's rules and their order, in plain Go on Linux.
- `internal/ui/features/settings`: every reason has words and the page shows them.
- `internal/addons/sandbox/profiles_test.go`: which add-on IDs have a profile to remove (Sona's own, the installed folders, and the data folders left by add-ons that ran), in plain Go on Linux. `TestRemoveProfiles` (`windows_test.go`) makes two add-ons' profiles, grants them as `Start` does and removes them, on windows-2025 in `ci.yml`'s "Sandbox tests" step.
- Not run: the macOS sandbox and the Windows AppContainer path for add-ons were type-checked and, for macOS, its profile text tested; they were not run on those systems in the pull request that added them. Removing the Windows profiles (`TestRemoveProfiles`) is run by the Windows CI job.

### Music videos

**What it does.** Music videos play through the same CDM host child as songs. The CDM decrypts and decodes the video rendition with its own video decoder (`internal/playback/widevine`, `cdmhost`), and the child paces the pictures against the audio clock ([architecture.md](architecture.md) §4.2).

**Threats.** Protected content leaving the child in a form that could be saved, and a compromised child abusing the picture path.

**Controls.**

- **What leaves the child:**
  - Decoded pictures only, as I420 planes, one at a time when each is due.
  - The clear bitstream and the keys never go from the child to the parent (licence requests go out as they do for audio). The parent only sends the child the encrypted segments it fetched.
- **Checks in the parent** (`internal/playback/cdmhost/video.go`, `host.go`):
  - A picture is dropped before it reaches `OnVideoFrame` unless both sides are positive and at most 1920 (`MaxVideoSize`) and its data is exactly the I420 size for them (`I420Size`).
  - The general frame limit ([F8](#f8-a-compromised-child-could-exhaust-the-parents-memory), 16 MiB) still bounds the message itself.
- **Memory only** (`internal/playback/video`): the surface keeps only the latest picture, never a backlog, and never writes one to disk, a cache or a log. When the current item stops being a video, the engine clears it.
- **Serving:** there is none.
  - The UI draws in the same process: its video feed (`internal/ui/media`, `kit.VideoFeed`) waits on the surface in memory and converts the latest picture to RGBA for one bitmap.
  - No HTTP handler, loopback or network listener serves pictures.
- **Quality:** SD only. Apple licenses HD and 4K only to hardware-backed CDMs (HDCP), and Sona never works around that ([apple-music-reference.md](apple-music-reference.md) §4.5).

**Tests.**

- `TestVideoFrameValid` and `TestI420Size` (`cdmhost/videotime_test.go`: wrong sizes, short and long data, over the limit).
- The surface tests (`internal/playback/video/surface_test.go`).
- The engine test that only the loaded video's pictures reach the surface (`internal/playback/native/video_test.go`).

### Apple's translations

**What it does.** Sona's UI text in other languages comes first from Apple Music's own web strings. Go fetches them from music.apple.com at run time and caches them (`internal/i18n/applestrings`; [ui-architecture.md](ui-architecture.md) "Strings and locales").

**Threats.** A network attacker (or Apple serving something unexpected) gets text into the UI that misleads the user, breaks rendering, or runs as code.

**Controls.**

- **Requests** (`bundle.go`, `store.go`):
  - Plain `GET`s of the `/us/home` page, the index script it names and one locale's bundle, with a browser User-Agent and no credentials or cookies, through the start-up gate (after first paint).
  - Redirects must stay on `https://music.apple.com` (at most 5).
  - Responses are capped (page 4 MiB, index script 16 MiB, bundle 4 MiB, read up to the cap plus one byte).
  - Cached URLs are trusted only under `https://music.apple.com/assets/`. Cache file names come only from Apple's locale tags matching `^[a-z0-9]+(-[a-z0-9]+){0,3}$`.
- **Parsing** (`bundle.go`): the bundle is a JS module, read as data and never run. It holds string constants and one object literal of string keys and string values, with JS string escapes decoded. Calls, template substitutions, numbers, arrays or anything else fail the whole bundle.
- **Conversion and checks** (`mapping.go`, `icu.go`):
  - Only the keys `apple-keys.json` maps are read; everything else in the bundle is ignored.
  - Each becomes an ICU message for its Sona message: Apple's `@@name@@` placeholders are renamed through the reviewed mapping, and ICU syntax characters in Apple's text are quoted, so Apple's text cannot add arguments, selects or plurals.
  - A message is dropped when an Apple placeholder has no Sona argument, a Sona argument is missing from Apple's text (the count in a plural's `other` form), the plural shape differs, or it is over 4 KiB.
- **Cache** (`store.go`):
  - `<cache>/strings/index.json` and `<apple-tag>.json`, written atomically.
  - A file over 1 MiB, with unknown fields, another format version or an untrusted URL is ignored and fetched again. Cached messages are kept only for IDs the current mapping has.
- **Rendering:**
  - `internal/ui/i18n` parses a message with MyGo's `i18n.ParseMessage`. One that does not parse is left out, logged once, and the English text shows.
  - It formats the message into a string that MyGo's widgets draw as text; nothing interprets it as markup or code.
  - A locale is shown only when its layers translate 90% of Sona's messages.

**Tests.**

- `internal/i18n/applestrings`: `TestParseBundleModuleRejectsCode`, `TestToICURefuses*`, `TestConvertKeepsOnlyMappedCheckedMessages`, `TestStoreRefusesOversizedBundle`, `TestStoreStaysOnApplesOrigin`, `TestStoreIgnoresEnglishAndDamagedCache`.
- `internal/ui/i18n`: `TestLayersAppleThenCommunityThenEnglish`.

**Residual risks.**

- Sona trusts music.apple.com over TLS for what the strings say. Apple (or anyone able to serve that origin) can put wrong or misleading wording in place of a mapped message, within its placeholders and the size cap.

### Lyrics from other services

**What it does.** Besides Apple's, Sona shows lyrics from Musixmatch, LRCLIB and NetEase Cloud Music (`internal/lyrics`; [architecture.md](architecture.md) §3 "Lyrics").

**Threats.**

- A service (or a network attacker) sends text that misleads the user, breaks rendering or runs as code.
- A service learns more about the user than the song.
- Musixmatch's user token leaks.

**Controls.**

- **Requests** (`provider.go`, `services/lyrics.go`):
  - Only from Go, after first paint, through the start-up gate, to fixed `https` hosts (`apic-appmobile.musixmatch.com`, `lrclib.net`, `music.163.com`). Redirects must stay on `https` (at most 5). The UI never talks to these hosts.
  - Each request carries the song's title, artist, album, length and ISRC, a `Sona/<version>` User-Agent and no cookies or Apple credentials.
  - A body is read up to 8 MiB; a larger one fails to decode.
  - Each provider has a 10 s timeout, its own pace and a backoff, so a slow or hostile service delays only its own answer.
- **Musixmatch's user token** (`musixmatch.go`):
  - It is anonymous, from Musixmatch's `token.get` (no account). It is kept in the keyring with the usual 0600 file fallback (`store.OpenSecretWith`, key `musixmatch`), never in SQLite or the settings.
  - It is added to the query only when a request is sent, and a transport error is rewrapped without its URL, so the token never reaches an error message or the log.
  - A token Musixmatch no longer accepts is replaced once; a captcha answer is a rate limit (15 minutes' rest).
- **Parsing** (`lrc.go`, `musixmatch.go`, `netease.go`):
  - Bodies are JSON decoded into fixed structs. LRC, Musixmatch's richsync and subtitle bodies are parsed as data (timestamps and text), never run or interpreted as markup.
  - A result whose song does not match (title, artist, length within 3 s, or the ISRC) is dropped.
- **Cache** (`cache.go`): the parsed result (`lyrics.Lyrics` JSON) per provider, song and translation language in `lyrics_cache`. An entry that doesn't decode is ignored and fetched again.
- **Rendering:** lines, words and translations reach the UI as plain strings in the lyrics DTO and are drawn as text by MyGo's widgets; nothing interprets them as markup.

**Tests.** `internal/lyrics`: `TestMusixmatchKeepsTokenOutOfErrors`, `TestMusixmatchCaptchaAndTokenRenewal`, `TestMusixmatchMatchesByISRC`, `TestLRCLIB`, `TestNetEase`, `TestRateLimitBacksOff`, `TestLimiter`, `TestCachePositiveAndNegative`.

**Residual risks.**

- Each service sees which songs the user looks up lyrics for (title, artist, album, ISRC) and the user's IP address. Sources can be switched off in Settings → Playback → Lyrics; NetEase is off by default.
- The services decide what the lyrics say: a wrong or offensive text is shown as sent. A wrong match is limited by the title, artist and length checks.
- Musixmatch's endpoint and token flow are the ones its own apps use, not a documented API. Musixmatch can change or refuse them, and Sona then shows other sources' lyrics.

### TuneIn metadata

**What it does.** While a local broadcaster plays, Sona shows the song its stream names. For the stations Apple serves through TuneIn, it also shows TuneIn's profile of the station (`internal/tunein`, `internal/playback/native/broadcast.go` and `onair.go`; [architecture.md](architecture.md) §4.2).

**Threats.**

- A service (or a network attacker) sends text that misleads the user or breaks rendering.
- A request can be steered to another host.
- Listening data leaves Sona.

**Controls.**

- **Requests:**
  - From the parent only (the CDM host child has no network and the UI never contacts these hosts), to one fixed `https` host, `opml.radiotime.com` (`Describe.ashx?id=s…&render=json`).
  - The station ID is not free text. `tunein.StationID` takes it from the `ti_ad_params` of Apple's own play URL, only when that URL is on `tunein.com`, and only when it matches `^s[0-9]{1,12}$`. `Describe` refuses anything else before a request is built, so a station name or a hostile URL cannot add a path or parameter.
  - No search by name is made, so a station Apple does not serve through TuneIn is never looked up there.
  - A response is read up to 256 KiB.
- **Polling** is bounded and ends with the track:
  - 30 s while the station lists songs and the stream names none, 10 min otherwise.
  - Exponential backoff from 30 s to 10 min on failure.
  - None for a station TuneIn does not know, and none once the stream names the song itself.
- **The song's cover** is a catalog search at Apple (`amp-api.music.apple.com`, the usual client). Its artwork goes through the existing artwork cache, so no new image host is allowed.
- **Parsing:**
  - TuneIn's JSON is decoded into a fixed struct. Every field reaches the UI as plain text drawn by MyGo's widgets (no markup).
  - The website TuneIn lists is kept only if it is an `http` or `https` URL without user info. It opens in the system browser on a click (`OpenURL` of `internal/services`, called from Now Playing), never inside the app.
  - A stream's ICY text is split at " - " and never interpreted.
  - The MPRIS server (`internal/mpris/server.go`, `wireString`) makes every Metadata string one D-Bus can carry: NUL bytes are dropped and invalid UTF-8 becomes U+FFFD ([F16](#f16-a-nul-byte-or-invalid-utf-8-in-on-air-metadata-broke-mpris)).
- **Not a listen:** the queue item is the station; the song on air is display and MPRIS metadata. It is never reported as Apple play activity, never recorded in the history, and live radio is never scrobbled (`internal/scrobble/track.go`).

**Tests.**

- `internal/tunein`: `TestStationID`, `TestParseDescribe`, `TestDescribe`, `TestWatch…`.
- `internal/playback/native`: `TestBroadcastTuneIn`, `TestBroadcastOnAirWhenHeard`, `TestMatchArt`.
- `internal/playback/broadcast`: `TestOnAirOfTitle`.
- `internal/mpris`: `TestStreamMetadataTheBusCannotCarry`.

**Residual risks.**

- TuneIn sees the user's IP address and which of its stations they play (as it already does when Apple's playlist makes Sona fetch TuneIn's PLS). The catalog search tells Apple which song is on air, with the account's token.
- Private mode does not stop either (it keeps plays out of Apple's history and the scrobblers; these are reads).
- `Describe.ashx` is TuneIn's long-public OPML endpoint, not a documented API for third parties. It can change or be withdrawn, in which case the profile and TuneIn's song disappear and the stream's own metadata remains.
- The cover can be wrong only if the catalog holds a different song with the same title and artist.

### Local broadcaster streams

**What it does.** A local broadcaster (Capital, KISS, TuneIn's stations) plays from one clear URL Apple's `/v1/play/assets` names, often plain `http`: an HLS playlist, a PLS or M3U playlist, or a Shoutcast/Icecast stream. The parent fetches it (`internal/playback/broadcast`, `internal/playback/native/broadcast.go`; [architecture.md](architecture.md) §4.2). The URL, the playlists it leads to and the redirects it answers are all chosen by the broadcaster, or by anyone on the network path of a plain `http` request.

**Threats.**

- A hostile playlist, redirect or HLS segment list makes Sona send GETs to `127.0.0.1`, a router, another machine on the LAN or `169.254.169.254` (blind: no response data leaves Sona, but a browser-like request reaches services a web page could not, past a browser's Private Network Access).
- A small playlist names tens of thousands of entries, so one play sends that many requests.
- A name resolves to a public address when checked and a private one when dialled (DNS rebinding).

**Controls** (`internal/playback/broadcast/guard.go`, `broadcast.go`; `native.Engine.streamHTTP`):

- **One client for everything.** `broadcast.NewClient` is the engine's stream client. Playlists, nested playlists, HLS playlists and segments, the first request and every reconnect use it (`session.hc`).
- **The dial is checked, not the URL.** The transport's `net.Dialer.Control` runs on the IP about to be connected to, after DNS and for every redirect and rebinding answer, and refuses:
  - loopback (`127.0.0.0/8`, `::1`), private (RFC 1918, unique local `fc00::/7`), link-local (`169.254.0.0/16`, which holds cloud metadata, and `fe80::/10`);
  - unspecified (`0.0.0.0/8`, `::`), multicast, reserved (`240.0.0.0/4`, broadcast), carrier-grade NAT (`100.64.0.0/10`), `192.0.0.0/24`, benchmarking `198.18.0.0/15`;
  - an IPv4 address inside an IPv6 one (`::ffff:a.b.c.d`, the NAT64 prefix `64:ff9b::/96`) is judged as the IPv4 address.
- **Redirects:** at most 5; only to `http` or `https`; never from `https` to `http`.
- **Playlists:** only the first 16 entries of a playlist are tried, and the nth waits `(n-1) × 100 ms` (up to 1 s) after the previous one. Nesting stays at 3 levels, so a play makes at most a few thousand requests, not tens of thousands, and slowly.
- **No environment proxy.** A proxy would be the host dialled (usually a private address, which this check would refuse) and would fetch the target itself, past the check. The stream client dials directly, so `HTTP_PROXY` and `HTTPS_PROXY` do not apply to broadcaster streams (Apple's own requests are unchanged).
- Requests carry no Apple credential; TuneIn's profile fetch is a separate fixed-host client (see TuneIn metadata).

**Tests.**

- `internal/playback/broadcast`: `TestDialControl` (each range, IPv4-mapped and NAT64 forms, names), `TestClientRefusesLoopback`, `TestBroadcasterCannotAimRequestsAtPrivateAddresses` (a PLS, an M3U, an HLS playlist and a redirect from one server naming another: the target gets no GET), `TestRedirectCap`, `TestCheckRedirect`, `TestPlaylistEntryCap`, `TestPlaylistEntryBackoff`.
- `internal/playback/native`: `TestEngineStreamClientRefusesLoopback`.

**Residual risks.**

- A broadcaster can still make Sona fetch from any public address, and an `http` stream can be read or altered on the path. Only the audio is played; nothing is executed.
- A user who can reach the internet only through a proxy cannot play broadcaster streams until the proxy is not needed. The guard is not relaxed for them.
- A station hosted on a private address (a LAN radio) is refused by design.

### WebAssembly add-ons

**What it does.** An add-on whose manifest says `runtime: "wasm"` (`docs/addons.md`) is a WebAssembly module that Sona runs **inside its own process**, on [wazero](https://github.com/tetratelabs/wazero) (v1.12.0, Apache-2.0, pure Go, no cgo), in `internal/addons` (`wasm.go`, `wasmfs.go`, `wasmcache.go`, `module.go`). It speaks protocol v2 over WASI standard input and output. The supervisor (`services/addons.go`) starts it after the first paint, on a goroutine of its own, never on the UI thread.

**Why it needs no OS sandbox.** The OS sandbox for native add-ons takes rights from a program the OS runs with the user's. A module has none to take: the only way out of it is a host function Sona defines. Sona gives it WASI preview 1 over its standard streams, the clock, random bytes, its arguments and a small environment, and one folder. WASI preview 1 has no function for a socket, a process or the host's environment, so there is nothing to deny.

**Threats and controls.**

| Threat | Control | Test |
| --- | --- | --- |
| The module reads or writes a file of the user's | No folder is preopened except the add-on's own data folder. WASI's `..` and absolute paths are refused by the runtime | `TestWasmReachesOnlyItsDataFolder` |
| The module escapes its data folder through a link | wazero's directory mount follows a link wherever it points and lets the module create one (`path_symlink`). Sona mounts `confinedFS` instead: it refuses any path through a link and every `symlink`, so neither a link the module makes nor one the user left reaches outside | `TestWasmReachesOnlyItsDataFolder` (a link to a file and one to a folder, both pre-existing, and one it tries to make) |
| The module reaches the network | WASI preview 1 has no sockets. A manifest that asks `wasm` and `network` is refused (`bad_permission`), so the approval panel never promises it | `TestManifestRuntime`, `TestWasmAddonRefusals` |
| The module reads the host's environment or paths | The environment is the allowlist without the variables that name host paths (`HOME`, `PATH`, `TMPDIR`); tokens, keys and bus addresses never reach it | `TestWasmEnvironmentIsTheAllowlist` |
| The module spins | A CPU budget of a quarter of a core with a burst of five seconds, counted from the module's running time between WASI calls (waiting for input or sleeping costs nothing). The runtime stops a module at its next loop or call (`WithCloseOnContextDone`) | `TestWasmRunawayCPUIsStopped`, `TestWasmAddonRunawayIsStoppedAndLogged` |
| The module takes Sona's memory | 64 MiB of linear memory per module; growth past it fails | `TestWasmMemoryIsCapped` |
| The module does not leave when asked | `shutdown`, end of input, one second, then a kill that stops it at its next loop or call and frees a read or write it waits in | `TestWasmStopKillsADeafModule`, `TestWasmAddonStopsAtQuit` |
| The file is swapped after it was checked | The module is read once from the open file (a regular file of the user's, not writable by others, inside the folder, at most 32 MiB) and those bytes are what is compiled. `sona addons dev` does the same on every system | `TestDevRunsAWasmAddon`, `TestDevWasmRunsWhereNativeCannot` |
| The compiled-code cache is poisoned | It is machine code Sona loads, and wazero checks only a CRC. The folder is Sona's own (cache directory, mode 0700), outside every add-on's reach. An entry or folder that isn't the user's, or that others can write, drops the whole cache; it is kept to 256 MiB | `TestWasmCache` |
| The module floods Sona with frames | The session's limits, as for a native add-on: 1 MiB a frame, 256 frames unread, call rates | `docs/addons.md`, "Limits" |

**Residual risks.**

- **It runs in Sona's address space.** A bug in wazero's compiler or in Go's runtime that a module can trigger is code execution in a process that holds the user's tokens. That is the price of running inside Sona, against a native add-on's OS sandbox. The reasons to accept it: wazero is pure Go (memory safe apart from the machine code it emits), is fuzzed by its authors, and is pinned and updated through Renovate; a module comes only from the user's own folder after the user approves it; and a module has no network, so it can't send what it learns anywhere but into Sona's own answers.
- **Resources are shared.** The caps are per module. There is no total across add-ons, and compiling a large module takes up to two cores for a few seconds, once.
- **The CPU budget is wall time between host calls.** It is not a CPU-time counter: a machine paused or overloaded can charge a module for time it didn't run (a long pause is clamped to a few ticks).
- **Same trust as native for the user.** A module still answers the user's invokes and may show a link, which opens only after a click.

### The item inspector

**What it does.** **Inspect** in an item's menu (`internal/ui/features/inspect`, `services/inspect.go`) shows what Sona mapped for a song, album, playlist, artist, station or music video, the response Go's catalogue cache holds for it, its library row and what an add-on is sent for it ("What an add-on gets": the `invoke` target and, for a song, the `now-playing` track). It began as a development-build tool (#373). A release build now has it too, for add-on developers, behind **Settings → Diagnostics → Developer tools**, off by default and saved as `developerTools` in `settings.json`. A development build offers it regardless of the setting. MyGo's own element inspector (F12) stays out of release builds.

**Threat model.** What it exposes is the user's own item data: names, IDs, artwork URLs and cached catalogue responses they could read in the app anyway. It runs only when the person turns it on. Nothing new leaves the machine: the panel makes no request, and its copy buttons write to the user's clipboard on a click.

**Threats and controls.**

| Threat | Control | Test |
| --- | --- | --- |
| A token or credential appears in the panel | Every text passes `logging.Redact` (`redactedJSON`, the cache key). The cache holds response bodies only, never a request, so there is no header to show | `TestInspectShowsIDsArtworkAndTheCachedAppleResource` (a JWT-shaped value in a cached body, and no `Authorization`, `Bearer` or user-token header name anywhere in the report or either copy) |
| The UI reaches Apple through it | Raw data comes from `catalog.Catalog.Raw`, a cache read that never asks Apple; the UI holds no token and calls no Apple host | `internal/catalog` `Raw` reads `GetCache` only |
| A release build shows it without being asked | The action is registered only while `DeveloperTools` is on (or in a development build); turning it off unregisters it at once | `TestReleaseBuildOffersInspectOnlyWhileTheSettingIsOn`, `TestDevelopmentBuildOffersInspectWhateverTheSettingSays` |
| A settings import turns it on | An export carries the setting, like any preference. An import does not run code; the worst it does is add a menu entry that reads the user's own data. A file from someone else is still checked as before ([F1](#f1-settings-import-could-set-the-cdm-path)) | `TestDeveloperToolsIsOffByDefaultAndTravelsInAnExport` |
| "What an add-on gets" misleads about what add-ons see | It is built by the protocol's own code (`AddonTargetOf`, `invokeTarget`, `track`, `cli.AddonEventData`), and the section says add-ons never see private listens or previews while this inspection shows the item regardless | `TestInspectShowsTheTargetAnAddonReceivesAndTheNowPlayingTrack` (the target equals what a running add-on is sent), `TestWhatAnAddonGetsIsTheProtocolsJSONForTheItem` |

**Residual risks.**

- Someone with the user's session can turn the setting on and read cached item data. They already have the same session, the settings file and the database.
- The `InspectService.Inspect` method itself does not check the setting: the UI is its only caller, and it offers the action only when the setting is on. No add-on, command line or other process can reach it.

### Sona's own add-ons

**What it does.** Sona's Last.fm, ListenBrainz and Discord integrations are bundled add-ons (#302, `docs/addons.md`, "Bundled add-ons"): `sona --addon <id>`, Sona's own signed executable run by the add-on supervisor over protocol v2. They are registered in code and never read from `<config>`. Last.fm and ListenBrainz do their HTTP in their add-ons, including Last.fm's browser approval, whose token and session handling live in the add-on; Sona keeps the pipeline (tracker, private filter, persistent queue, backoff) and the stored sign-in, and sends the add-on only listens that passed the filter, as `scrobble.*` requests.

**Threats and controls.**

| Threat | Control | Test |
| --- | --- | --- |
| A private listen or a preview reaches a service | One host-side filter, `privatemode.Mode.Hidden`, decides before the queue: a hidden listen is never queued, so it is never a request to the add-on and never reaches the network | `TestScrobbleListenBrainzNeverSeesAPrivateListen`, `TestTrackerSkipsPrivateListens` |
| A third-party add-on is sent a `scrobble.*` request (and the credential in it) | `Session.Request` refuses any session the supervisor did not mark bundled, from the registry in code; the method must be one of four fixed names, there is no general call | `TestHostRequestsAreBundledOnly`, `TestAddonsRequestReachesABundledAddon` |
| A folder in the user's add-ons directory takes a bundled ID | The scanner refuses a folder that claims a reserved ID, and the registry is the only source of the manifest and the program | `TestAddonsUserFolderCannotClaimABundledID` |
| The add-on names a page for Sona to open that is not a web page | Sona opens only an `http` or `https` address without a user name; anything else fails the connection | `TestAddonScrobblerRefusesAnApprovalPageThatIsNotAWebAddress` |
| The sign-in leaks through the environment, a log or an error | It is in the keyring item `addon.<id>`; it crosses the channel only in the `credential` of the request that needs it; the environment is an allowlist (plus the proxy and certificate variables and the loader's `LD_LIBRARY_PATH`, for bundled add-ons only); an error frame carries a fixed message, never the cause's text | `TestEnvironmentIsAnAllowlist`, `TestEnvironmentGivesBundledAddonsTheLoaderPath`, `TestListenBrainzAddonAnswersInCodes` |
| A development override points the add-on at another host | The base-URL variables exist only in a `dev` build, are passed to bundled add-ons only, and are taken only for a loopback address | `endpoints_dev.go` |
| The approval token is read from the browser's command line by another local user | The browser is given a one-use loopback link, not Last.fm's page: the listener drops connections another user owns (Linux), the link works once and checks `Host`, and the redirect to Last.fm happens inside the user's browser ([F25](#f25-the-lastfm-approval-token-was-in-the-browsers-command-line)) | `TestLastFMApprovalTokenIsNotOnTheBrowsersCommandLine`, `TestApprovalLinkWorksOnce` |

**Residual risks.**

- **A bundled add-on holds the user's sign-in and the network, and has the user's rights until the OS sandbox (#304) confines it.** It is Sona's own signed code, not a third party's: the risk is a flaw in it, as in the rest of Sona.
- **A process of Sona's size runs per integration in use.** Each runs only while its account is connected (or a connect flow holds a lease on it), so a user with no accounts pays nothing.

### Secret scanning

**What it does.** A workflow scans the repository for leaked secrets (`.github/workflows/secret-scan.yml`, on pull requests and pushes to `main`, read-only token, no secrets).

**Threats.** A credential committed to a public repository, in a commit, a branch, a tag or a pull request's text.

**Controls.**

- **The guard:**
  - gitleaks 8.30.1 from its release, checked against the release's SHA-256.
  - It scans every commit of every branch and tag (a pull request's own commits included, since they stay public through it), then the pull request's title and body (read from the event payload, since a step's `env:` is printed in the log).
  - The public log shows each finding's rule, commit, file and line only, as an annotation: gitleaks' own output prints the rest of the line around a secret even with `--redact`.
- **Rules.** `.gitleaks.toml` adds these to gitleaks' defaults:

  | Rule | Catches |
  | --- | --- |
  | `jwt-segment` | A JWT's header segment and dot, so also a developer token split across string literals |
  | `apple-music-user-token` | A long token assigned to, or sent as, `Music-User-Token`, `media-user-token` or `MUSIC_USER_TOKEN` |
  | `long-base64-token` | A base64 run of 150+ characters with high entropy (a MUT without its name) |
  | `lastfm-api-credential` | 32 hex characters next to a Last.fm key or secret name |
  | `discord-client-id` | Replaces the default rule, which takes 18 digits only; Discord IDs have 17 to 20 (Sona's has 19) |
  | `secret-file` | `.env`, `.env.*`, `.p8`, `.p12`, `.pfx`, `.pem`, `.key`, keystores, SSH keys, by name |

- **Allowlist.** Each entry is commented and tied to its file:
  - the fake developer tokens in `internal/logging/logging_test.go` and `internal/diag/diag_test.go` (only their two exact fake payloads)
  - the fake Discord ID in `logging_test.go` (only that value)
  - the SRP vectors (`internal/auth/appleid/testdata/srp_vectors.json`, made from fixed test bytes)
  - Renovate's `<!--renovate-debug:…-->` comment in the pull request text (`pull-request.md` only, and only a base64 run that opens as Renovate's JSON does, `{"createdInVer"`: with a few labels it passes `long-base64-token`'s 150 characters)
- **After changing a rule,** check it against planted fakes on a branch you never push. These must all be found:
  - a one-line and a split developer token
  - a MUT in a header map
  - a Last.fm secret
  - a Discord ID
  - a `.p8` key
  - a `.env.production`
  - a fixture's fake payload copied to another file
  - a new token added to an allowlisted file

To run the scan locally:

```sh
# all local branches
nix run nixpkgs#gitleaks -- git --config .gitleaks.toml --redact .
# the working tree, uncommitted files included
nix run nixpkgs#gitleaks -- dir --config .gitleaks.toml --redact .
```

**Residual risks.**

- A secret pushed in any commit, even one later amended, force-pushed or squashed away, stays fetchable from GitHub by its SHA and through its pull request. If the scan fires, rotate the credential first; removing the commit comes second.
- Pull-request and issue text and Actions logs can't be rewritten with the history. The workflow scans titles and bodies; comments are not scanned.
- A pull request can change `.gitleaks.toml` itself; changes to it need the maintainer's review.
- Shapes the rules can't recognise (a token split into short pieces, encoded differently, or under an unrelated name) pass.

### Vulnerability scanning

**What it does.** CI runs `govulncheck` (pinned in `go.mod` as a `tool`) over every package with `-tags sqlite_fts5`, in the "Vet and type-check" job (`.github/workflows/ci.yml`), on pull requests, the nightly and releases.

**Threats.** A known vulnerability in the Go standard library or a required module that Sona's code can reach.

**Controls.**

- Only a reachable vulnerability (a symbol the code calls, directly or through another package) fails the job; `-show verbose` lists the imported but uncalled ones. The standard library counts at the release `go.mod` names.
- It does not run on a push to main, so a newly published advisory can't fail a merge that already passed; the next pull request, the nightly and the next release turn red until the fix lands.
- The fix is the release the finding names. There is no allowlist.
- The scan is the host build; Windows- and macOS-only files are type-checked, not scanned. Dependabot alerts cover the `site/` npm tree.

**Residual risks.** The database lists only known, reported vulnerabilities, and a reachable call through reflection or a plugin can be missed. The CI log is public and shows each finding with its call stacks, as the advisory itself already is.

## Findings

Each finding has an issue, its impact, the fix and the test that keeps it fixed. Where a fix is a component's control, the finding links to that component instead of repeating it.

### Summary

| ID | Title | Severity | Status | Fix reference |
| --- | --- | --- | --- | --- |
| F1 | Settings import could set the CDM path | Medium | Fixed | `internal/config/transfer.go` |
| F2 | CDM child sandbox blocked only inet sockets | Medium | Fixed (residual risks listed) | `internal/playback/cdmhost/sandbox_linux.go`, `sandbox_fs_linux.go`, `host.go` |
| F3 | Sandbox failed open | Low | Fixed | `server.open`, `Sandbox.loadsCDM`, `cdmhost.ErrSandbox` |
| F4 | Discord IPC trusted any user's socket | Low | Fixed (Unix sockets and Windows named pipes) | `internal/discord/ipc_unix.go`, `ipc_windows.go` |
| F5 | NaN/Inf MPRIS volume | Low | Fixed | `playback.ClampVolume`, MPRIS `Set Volume`, `config.Settings.Normalized` |
| F6 | Motion artwork followed redirects anywhere | Low | Fixed | `internal/motion/motion.go` |
| F7 | Loopback sign-in could be completed by another local user | Low | Fixed (residual risks listed) | `internal/auth/mut/loopback.go`, `peer.go`, `peer_linux.go`, `internal/auth/manager.go` |
| F8 | Child could exhaust the parent's memory | Low | Fixed | `internal/playback/cdmhost/host.go` |
| F9 | Artwork decoding accepted any registered image format, TIFF included | Low | Fixed | `internal/artwork/image.go`, `artwork.go`, `color.go` |
| F10 | Development builds logged the build's Last.fm and Discord credentials, and the diagnostics bundle carried them | Low | Fixed | `internal/logging/redact.go`, `internal/diag/bundle.go`, `build/ldflags.sh` |
| F11 | CDM child could reach the user's terminals (`/dev/tty`, `/dev/pts`, `TIOCSTI`) | Medium | Fixed | `internal/playback/cdmhost/stdio_unix.go`, `sandbox_linux.go`, `sandbox_fs_linux.go`, `seatbelt.go` |
| F12 | On Ubuntu the CDM host child could reach the session bus | Medium | Fixed for the `.deb` and `.rpm` (residual risk for the AppImage listed) | `build/linux/apparmor/sona`, `build/linux/nfpm/`, `Sandbox.ReachesSockets` |
| F13 | Release builds ran third-party tools (linuxdeploy and its plugins, the Chocolatey packages `nsis` and `pkgconfiglite`, the Go toolchain) without a pin or a checksum, next to the artefacts they could change, and kept a Go cache that outlived the run | Medium | Fixed | `build/linux/appimage-tools.sha256`, `build/windows/tools.sha256`, `build/windows/install-tool.sh`, `build/windows/package.sh`, `build/linux/release/Containerfile`, `build/ci/go.sha256`, `.github/workflows/release.yml` |
| F14 | CI and release images were trusted by a tag any branch could overwrite, and the images they build on were named by tag | Medium | Fixed (residual risks listed) | `.github/workflows/images.yml`, `build/ci/find-image.sh`, `build/ci/Containerfile`, `build/linux/release/Containerfile`, `build/linux/release/release.sh`, `.github/workflows/flatpak.yml`, `.renovaterc.json5` |
| F15 | The bot-cecil App key was a repository-level secret, readable by any `pull_request` run from a branch of this repository | Medium | Fixed (key rotation pending) | Repository settings: environments `bot-cecil`, `release`, `golden-update`, `nix-vendor-hash`; ruleset "release tags"; `docs/maintainers/ci-and-secrets.md` |
| F16 | A NUL byte or invalid UTF-8 in on-air metadata broke MPRIS | Low | Fixed | `internal/mpris/server.go` |
| F17 | The `sona` command printed other people's text to the terminal unfiltered | Low | Fixed | `internal/cli/plain.go`, `internal/cli/run.go`, `internal/cli/lyrics.go` |
| F18 | The diagnostics bundle carried the person's city and device names | Low | Fixed | `internal/config/diagnostics.go`, `internal/services/system_bundle.go` |
| F19 | The motion-artwork remuxer panicked on a huge tfhd base offset | Low | Fixed | `internal/motion/remux.go` |
| F20 | The fMP4 parser panicked on crafted init segments and fragments | Low | Fixed | `internal/playback/fmp4/init.go`, `fragment.go` |
| F21 | Server-declared counts and lengths allocated without limit in the parent | Low | Fixed | `internal/playback/fmp4/fragment.go`, `internal/playback/native/media.go`, `upload.go` |
| F22 | Artwork from the CDN was decoded without a pixel limit | Low | Fixed | `internal/artwork/image.go`, `artwork.go` |
| F23 | Local broadcaster streams followed any URL, so a playlist or redirect could aim requests at loopback, the LAN or cloud metadata | Low | Fixed | `internal/playback/broadcast/guard.go`, `broadcast.go`, `internal/playback/native/engine.go` |
| F24 | CDM message events started unbounded renewal goroutines (F8 incomplete) | Low | Fixed | `internal/playback/cdmhost/host.go`, `internal/playback/native/engine.go`, `renewals.go`, `media.go` |
| F25 | The Last.fm approval token was in the browser's command line | Low | Fixed (residual risks listed) | `internal/services/scrobble_link.go`, `scrobble.go`, `internal/auth/mut/peer.go` |
| F26 | The update manifest's version and platform were not signed (downgrade, cross-platform archive) | Low | Fixed (residual risks listed) | `cmd/releasesign/main.go`, `internal/update/statement.go`, `manifest.go`, `source.go`, `internal/services/updates.go` |
| F27 | The loopback sign-in could be completed with another account's token (login CSRF) | Low | Fixed | `internal/auth/mut/loopback.go` |
| F28 | On macOS any program of the user's could read Sona's keychain items (the MUT, the scrobbler and lyrics tokens) without a prompt | Medium | Fixed (residual risks listed) | `internal/auth/store/keyring_darwin.go` |

### F1. Settings import could set the CDM path

Severity: Medium. Status: fixed.

**Issue.** `ParseExport` filtered local keys by their exact name, but `encoding/json` matches field names case-insensitively. So `"CDMPath"` or `"cdmpath"` in a shared settings file still landed in `Settings.CDMPath`.

**Impact.** The CDM host then `dlopen`ed whatever that path named.

**Fix** (`internal/config/transfer.go`):

- `Import.Apply` decodes the document over a copy of the current settings and then puts back every local field from the current settings (`keepLocal`). No raw-key filtering is involved.
- `localKeys` is only used to leave the local keys out of an export.
- The `CDMPath` setting no longer exists. Sona loads only the CDM it downloaded itself (see [Widevine CDM downloader](#widevine-cdm-downloader)), so neither a settings file nor the environment can name a library.

**Test.** `TestImportIgnoresLocalKeysInAnySpelling` imports a fully customised document with every key, local ones included, spelled as exported, in lower case and in upper case.

### F2. The CDM child sandbox blocked only inet sockets

Severity: Medium. Status: fixed (residual risks listed).

**Issue.** The child had the whole environment (including `DBUS_SESSION_BUS_ADDRESS`), every path Unix socket (session and system bus, resolved, agents, X11), and full home access.

**Impact.** Code running in the CDM could leave the sandbox through D-Bus, for example with systemd's `StartTransientUnit`.

**Fix** (`internal/playback/cdmhost/sandbox_linux.go`, `sandbox_fs_linux.go`, `host.go`): the layered controls under [Linux sandbox](#linux-sandbox):

- a scrubbed environment
- new user, network and mount namespaces with a private filesystem view
- Landlock
- a two-stage start
- seccomp

The residual risks are listed there too.

**Test.** `TestSandboxConfinesChild`, `TestChildEnvDropsSessionAndSecrets`, `TestChildCannotOpenInetSockets`, `TestSandboxFallsBackWithoutNamespaces`, the DRM path regression and the realtime scheduling tests, all described under [Linux sandbox](#linux-sandbox) → Tests.

### F3. The sandbox failed open

Severity: Low. Status: fixed.

**Issue.** When the seccomp filter could not be installed (an architecture other than amd64/arm64, or a kernel without seccomp), the child logged a warning and loaded the CDM anyway.

**Impact.** The CDM ran without its sandbox.

**Fix:**

- `server.open` refuses to load the CDM unless `Sandbox.Seccomp && Sandbox.InetDenied` (`Sandbox.loadsCDM` on Linux).
- Windows refuses it unless the child is in its AppContainer with the network refused (see [Windows sandbox](#windows-sandbox)).
- macOS builds load it unsandboxed where Seatbelt is unavailable, and say so (see [macOS sandbox](#macos-sandbox)).
- The refusal is `cdmhost.ErrSandbox`, sent with the reply code `sandbox` and mapped back by `replyErr`.
- Diagnostics sets `DiagPlayback.CDMBlocked`. The Widevine CDM row then reads "Blocked", and Host sandbox reads "Not sandboxed" with the child's reason.
- Library uploads (no CDM) still play.

**Test.** `TestCDMRefusedWithoutSeccomp`.

### F4. Discord IPC trusted any user's socket

Severity: Low. Status: fixed.

**Issue.** The Discord IPC client trusted whatever socket it found at the usual paths.

**Impact.** Another local user could create `/tmp/discord-ipc-0` and receive the now-playing track.

**Fix on Unix** (`internal/discord/ipc_unix.go`):

- Candidates are `Lstat`ed and must be sockets (not symlinks) owned by `getuid()`.
- The parent directory must be owned by the user, or be a root-owned directory that is not group/world-writable or is sticky (`/tmp`).
- The temp directories are probed only when `XDG_RUNTIME_DIR` is unset.

**Fix on Windows** (`internal/discord/ipc_windows.go`). Discord listens on `\\.\pipe\discord-ipc-0` to `-9`, and pipe names are global to the computer, so any user's process may create one first:

- Each pipe is opened with `SECURITY_SQOS_PRESENT | SECURITY_IDENTIFICATION`, so its server can identify Sona but never impersonate it.
- Before anything is written, `GetNamedPipeServerProcessId` gives the server's process, and its token's user SID must equal Sona's. A server process Sona may not query counts as another user's. A pipe that fails is closed and the next one tried.

**Tests.**

- Unix: `TestIgnoresUntrustedSockets` covers a regular file, a symlink, another uid's socket, temp sockets while a runtime directory exists, and the fallback when it does not.
- Windows (the Windows job's "Discord pipe tests"): `TestRefusesAnotherUsersPipe` checks a pipe whose server is another user (LocalSystem) is refused before a byte reaches it; `TestSetActivityOverOwnPipe` and `TestProcessUserIsTheCurrentUser` cover the user's own pipe.
- The add-on (#302): see [the Discord add-on's socket exception](#the-discord-add-ons-socket-exception) below.

#### The Discord add-on's socket exception

Discord's presence is the bundled `discord` add-on (`internal/addons/own/discord.go`) on Linux and macOS. It is the one add-on given a socket, and F4's checks apply at both ends:

- **What is named.** Sona (`discord.Sockets`, with the Unix checks above) lists the sockets that are there when it starts the add-on: `discord-ipc-0` to `-9` in `$XDG_RUNTIME_DIR` (in `$TMPDIR`, `$TMP`, `$TEMP` or `/tmp` only when it is unset, which is macOS' case) and in the Flatpak and Snap folders Discord uses under it. Each must be a socket (not a link) of the user's, in a folder only the user can replace it in. These, and no other path, are the add-on's `Spec.Sockets` (`internal/addons/sandbox`), which only code can set: a manifest has no field for it, and only `discord` gets any (`addonRunner.shareDiscord`). The add-on is told the list in `SONA_DISCORD_IPC` and dials only those (`discord.NewAt`), after the same ownership checks.
- **What that allows.** Linux binds each named socket into the add-on's new root at its own path (so the session bus, the audio server, Wayland and every other socket in the same folder are not there) and lets `socket(AF_UNIX)` through the seccomp filter, which it refuses otherwise; with no Discord running, none is named and the add-on has no Unix socket at all. macOS adds `(allow network-outbound (literal "<socket>"))` for each, over the profile's `(deny network*)`. The add-on has no `network` permission and no network: no TCP, UDP, netlink or loopback.
- **What the add-on could do with it.** Talk to the user's Discord as any program of the user's may over its IPC with the application ID it was given: set or clear an activity, and read what Discord answers. It holds no Discord token and no other secret, and what it knows of the music is what any add-on is told, nothing of a private listen or a preview.
- **A socket Discord makes later.** A bind mount shows the add-on the socket that existed when it started; a Discord that started or restarted since has made another that the add-on can't reach. The add-on reports each failed attempt (`connection`), and Sona starts it again only when the sockets it now finds differ from those it named at start. It never widens the list to a folder to see a socket that isn't there yet.
- **Windows.** Discord's IPC there is the named pipe `\\.\pipe\discord-ipc-N`. An AppContainer reaches a named pipe only where the pipe's own access list names the container (or `ALL APPLICATION PACKAGES`), which is Discord's to set; Sona's sandbox grants an AppContainer files and folders (`internal/appcontainer`), and neither widening the container (a capability or `ALL APPLICATION PACKAGES` for every pipe) nor changing the access list of another program's pipe stays narrow or is Sona's to do. So on Windows Discord's presence stays in Sona's process: `services.Platform.DiscordInProcess` is set by `cmd/sona/integrations_windows.go`, the add-on is not listed or started there, and F4's pipe checks above are what protect it. This was not tried against a real AppContainer: it is the design's reason, not a measurement.

**Tests of the exception.**

- `TestOnlyTheNamedSocketNotItsNeighbours` (`internal/addons/sandbox`, Linux, run in the real sandbox): the add-on reaches the one socket it is given and none beside it in the same folder (another `discord-ipc-N`, `bus`, `wayland-0`) or under it (the Flatpak's), and with the exception it still has no TCP and no netlink socket. `TestUnixSocketOnlyWhenNamed` and `TestNoNetworkWithoutThePermission` cover the rest.
- `TestDiscordAddonIsGivenDiscordsSocketsOnly` (`internal/services`): what `Spec.Sockets` holds is exactly Discord's sockets (runtime folder, Flatpak, Snap; not `discord-ipc-10`, a `.lock`, another application's folder, a file or a link named like one, nor the bus, Wayland or PipeWire), no network, and no other bundled add-on has a socket or is told the list.
- `TestDiscordAddonRestartsWhenDiscordsSocketsChange`, `TestDiscordAddonIsAbsentWhereItsPresenceStaysInSona` and `TestOnlyTheDiscordAddonReportsAConnection` (`internal/services`): the restart rule, Windows' seam and that only `discord` has the `connection` request.
- `TestDiscordAddon…` (`internal/addons/own`): over the real protocol and a fake Discord socket, the application ID comes in `hello`, a public listen is `SET_ACTIVITY`, an idle status (what a private listen or a preview is) clears it, the add-on reports its connection and clears the presence when it is stopped; the plan's tests are the old ones of `internal/services` for what is shown.

### F5. NaN or infinite MPRIS volume

Severity: Low. Status: fixed.

**Issue.** Go's `min`/`max` pass NaN through, so `Volume = nan` set over MPRIS was not rejected.

**Impact.** The NaN reached the snapshots, the settings and GStreamer's `volume`.

**Fix:**

- MPRIS `Set Volume` rejects NaN and ±Inf with `InvalidArgs`, and its `clamp01` maps NaN to 0.
- `playback.ClampVolume` (NaN → 0) is used by `VolumeGain` and `Coordinator.SetVolume`.
- `config.Settings.Normalized` clamps the same way.

**Test.** `TestMethodsReachPlayer` (NaN, +Inf, −Inf → InvalidArgs), `TestSetVolumeNaN`, `TestVolumeGainClamps`.

### F6. Motion artwork followed redirects anywhere

Severity: Low. Status: fixed.

**Issue.** The motion artwork fetch checked its host allow-list on the first request only, so a redirect could lead anywhere.

**Impact.** An allowed host could redirect the fetch to another https host or to plain http.

**Fix** (`internal/motion/motion.go`):

- `New` copies the HTTP client and sets `CheckRedirect` to re-run the allow-list on every hop, with at most 5 redirects.
- `appleMedia` rejects userinfo and any port but 443.

**Test.**

- `TestRedirectLeavingAllowList`: an allowed host redirects to another https host and to plain http. Nothing is served, and the targets get no request.
- `TestAppleMedia`.

### F7. Another local user could complete the loopback sign-in

Severity: Low. Status: fixed (residual risks listed).

**Issue.** `internal/auth/mut/loopback.go` used a nonce as its only secret, and that nonce is in the URL passed to `xdg-open`. That puts it in the argv of `xdg-open` and the browser, which every user can read in `/proc/<pid>/cmdline` unless `/proc` is mounted with `hidepid`.

**Impact.** Another user could POST their own token to `/auth/<nonce>/complete` before the real redirect arrived, and Sona would sign in to their account.

**Fix** (`internal/auth/mut/loopback.go`, `peer.go`, `peer_linux.go`; `internal/auth/manager.go`):

- **Socket owner (Linux).**
  - The loopback listener looks up each connection's client socket in `/proc/net/tcp` and `/proc/net/tcp6` (the row whose local address is the client's and remote address is the server's).
  - It drops the connection, unanswered, unless Sona's own uid owns it. A connection whose socket is not found is dropped too. Reading the URL from argv no longer gets another user anything.
  - This is what closes the finding. A start-page cookie alone would not, because a process that reads the URL can load `/start` and post `/complete` within milliseconds, before `xdg-open` has started the browser.
- **Single-use start links bound by a cookie** (every platform):
  - Each start link carries its own random ticket (`/auth/<nonce>/start?t=<ticket>`) and works once.
  - Loading it sets the cookie `sona_signin`: 256 random bits that never appear in a URL, host-only (no `Domain`), `Path=/auth/<nonce>/`, `HttpOnly`, `SameSite=Strict`, `Max-Age` = the rest of the attempt.
  - `/complete` refuses (403) without that cookie, compared in constant time.
  - A ticket loaded a second time ends the attempt (`mut.ErrStartReused`). Sona's pages never load one twice (the start page replaces itself with Apple's), so a repeat means the link reached someone else.
  - The UI's "Open the page again" and "Copy link" ask Go for a new link (`AuthService.SignInLink`) instead of reusing the opened one.
- **The attempt ends** on success, failure, timeout, cancel or a replayed link. After that every start link and `/complete` answer 410, cookie or not, and the server shuts down. A successful `/complete` deletes the cookie.
- **Why `SameSite=Strict` works here:**
  - Apple's authorize page returns the browser to `/callback` with a cross-site top-level navigation, so the cookie is not sent there, and does not need to be. `/callback` only serves the page that reads the fragment, and the token never reaches the server on that request.
  - The cookie is needed on the callback page's `fetch` POST to `/complete`, a same-origin request from a top-level `127.0.0.1` document, where `Strict` cookies are sent.
  - Measured in headless Chromium 155 against a stand-in for `authorize.music.apple.com` (served over TLS through `--host-resolver-rules`): `/start` (`Sec-Fetch-Site: none`) sets the cookie, `/callback` arrives `cross-site` with no cookie, `/complete` arrives `same-origin` with it and succeeds.
  - There is no `Secure` attribute or `__Host-` prefix: both need https.

**Test.**

- `internal/auth/mut`:
  - `TestCompleteRequiresTheStartCookie` (no cookie, a wrong value and a truncated value get 403 and leave the attempt pending; the right cookie completes, even beside a stray one).
  - `TestCompleteBeforeStartIsRefused`.
  - `TestStartLinkWorksOnceAndAReplayEndsTheAttempt` (the replay gets 410 with no cookie and no authorize URL, `Wait` returns `ErrStartReused`, the first cookie can no longer complete).
  - `TestEachLinkWorksOnce` (a second link works; a missing or guessed ticket is 404 and harmless).
  - `TestTimeoutEndsTheAttempt` (after the deadline the cookie no longer completes, `/start` sets nothing, the server is closed).
  - `TestHappyPathWithCookieJar` (real HTTP with a cookie jar; a racing client without it gets 403; the cookie is deleted after).
  - `TestStartCookieBindsTheAttempt` (the attributes above).
  - `TestCompleteDeliversOnceAndRejectsReplays`.
  - `TestSocketUIDFindsTheClientRow` and `TestParseProcAddrRejectsMalformed` cover the `/proc/net/tcp*` parser (the client's row, not the server end's; IPv4-mapped rows in `tcp6`).
  - `TestOwnerListenerDropsOtherUsers` (Linux) runs the real lookup: a listener expecting another uid closes this process's connection unanswered, one expecting its own uid serves it.
- `internal/auth`: `TestReplayedStartLinkEndsSignIn`, `TestOpenFailureKeepsWaiting` (a new link from `SignInLink` completes the attempt), `TestLoopbackSignIn` (the manager's browser loads the start link with a cookie jar before completing).

**Residual risks.**

- On macOS the socket-owner check is not implemented. There other users cannot read a process's arguments (the kernel refuses `KERN_PROCARGS2` across users) and `open` hands the URL to the browser through Launch Services, so the URL does not leak that way. The cookie binding still applies.
- Windows has neither check, and its loopback sign-in has not been reviewed.
- Cookies are not isolated by port. A same-user process can always read Sona's state, so this matters only for another user, whom the Linux check already keeps off Sona's port. Another user's server on a different `127.0.0.1` port would see the cookie only if the browser visited it with a same-site navigation, which `SameSite=Strict` rules out for links from other sites.
- Another user who reads the URL can still load a start link to end the attempt (Linux drops that connection, so only on platforms without the check). That is a denial of service only: the user starts again, and the native sign-in is unaffected.
- If a browser runs in a different network namespace from Sona, it cannot reach `127.0.0.1:<port>` at all. Flatpak and Snap browsers share the host's network namespace, and their sockets carry the user's uid.

### F8. A compromised child could exhaust the parent's memory

Severity: Low. Status: fixed.

**Issue.** The parent read child frames and queued child events without a bound.

**Impact.** A compromised child could exhaust the parent's memory.

**Fix** (`internal/playback/cdmhost/host.go`):

- **Frame size.**
  - The parent reads child frames through `gobFrames`. It parses gob's per-message length prefix and fails a message over 16 MiB (`maxChildFrame`) before gob allocates for it.
  - The largest legitimate child→parent frame is a decoded music video picture: about 630 KB for an SD picture (864×486 I420), at most about 5.5 MB at the 1920×1920 limit. Licence requests and renewals are a few KiB.
- **Event queue.** At most 1024 events (`maxQueuedEvents`) wait for the receiver. Beyond that, position samples are dropped and any other event ends the child.
- **On a violation,** the parent kills the child, and `Err` wraps the reason (`errMisbehaved`).

**Test.**

- `TestOversizedFrameKillsChild`: a child announces a 512 MiB frame.
- `TestEventFloodIsBounded`: 3072 position samples, then 2048 other events.
- `TestGobFramesPassThrough`: real streams pass through at any read size, and a frame over the limit fails.

### F9. Artwork decoding accepted any registered image format

Severity: Low. Status: fixed.

**Issue.**

- `github.com/disintegration/imaging` crashes on crafted TIFF files and has no patched version. Sona used it to resize and blur artwork, and it also registers TIFF and BMP decoders with the `image` package.
- Artwork was decoded with `image.Decode`, which picks a decoder by sniffing the bytes. The same went for the WebP decoder Sona imported itself.

**Impact.** A TIFF served for an artwork URL would have reached the vulnerable decoder.

**Fix** (`internal/artwork/image.go`, `artwork.go`, `color.go`):

- Artwork bytes are recognised by their magic numbers and decoded with `image/jpeg` or `image/png` directly, never through the `image` format registry.
  - Anything else fails at fetch with `artwork.ErrFormat` and is not cached. The UI reads artwork through the cache in process, through `internal/ui/kit`'s loader.
  - Apple's CDN serves JPEG or PNG for every template Sona fills.
- `imaging` and the WebP decoder are gone. Resampling (Lanczos-3 for variants and backdrops, box for colour extraction), the Gaussian blur and the saturation and brightness adjustments are Sona's own code in `image.go`. They match imaging's output: variants are pixel-identical within 1 level, backdrops differ by a mean of 0.25 levels.
- The binary no longer links `golang.org/x/image/tiff`, `golang.org/x/image/bmp`, `golang.org/x/image/webp` or `imaging`.

**Test.**

- `TestDecodeAcceptsOnlyJPEGAndPNG` (JPEG and PNG decode; TIFF in both byte orders, GIF, BMP, WebP, truncated magic and empty input fail with `ErrFormat`).
- `TestTIFFSourceRejected` (a TIFF from the CDN fails `Get`, `Backdrop` and `DominantColors`, is not cached, gets a 502, and a TIFF decoder registered with the `image` package is never called).
- `TestPNGSource`.

### F10. Development builds logged the build credentials

Severity: Low. Status: fixed.

**Issue.**

- The build compiles Sona's Last.fm API key and secret and its Discord application ID in with `-ldflags "-X …scrobble.lastfmKey=… -X …scrobble.lastfmSecret=… -X …discord.clientID=…"` (from `SONA_LASTFM_KEY`, `SONA_LASTFM_SECRET`, `SONA_DISCORD_CLIENT_ID`). Without `-trimpath`, Go records `-ldflags` in the binary's build info (`debug.ReadBuildInfo`).
- Sona's previous shell, Wails (removed with the move to the native UI; the finding was made against it), logged every build-info setting at startup as `"Build Info:"` in builds without the `production` tag.
- The record went through Sona's log handler unchanged into `sona.log` and the dev stderr, and from the log tail into the diagnostics bundle (Settings → Diagnostics → Copy diagnostics, meant for pasting into issues).

**Impact.** The credentials were in a log file on the developer's own machine and in any bundle copied from it.

- Severity Low: the values are compiled into every distributed binary, where anyone can read them, so they are not true secrets.
- Production builds were not affected: `-trimpath` keeps `-ldflags` out of the build info, and the `production` tag compiled the startup log out.
- The native app has no such logger, but the controls below stay as defence in depth, and the bundle still has to scrub logs an older build wrote.
- Other paths checked: Sona itself never calls `debug.ReadBuildInfo`; crash files hold the Go runtime's panic output and stacks, which carry no build settings; the Diagnostics report lists versions and a fixed set of environment variables, none of them these.

**Fix:**

- **Log handler** (`internal/logging/redact.go`):
  - An attribute named `-ldflags` (any case, `-`/`_` ignored) is replaced whole with `[REDACTED]`.
  - The whole value goes rather than only credential-named `-X` symbols, because a name list misses the next credential someone injects, and nothing else in the flags is worth a log line (the version is logged by `startup: process start`).
  - In free text, `Redact` replaces a quoted or bare `-ldflags` value, and the value of every linker `-X name=value` assignment whatever the symbol is called.
- **Diagnostics bundle** (`internal/diag/bundle.go`): `Scrub` runs `logging.Redact` over the log tail as read from disk, so a log written by an older build still loses the values.
- **Build** (`build/ldflags.sh`, used by `build/linux/Taskfile.yml`, `build/darwin/Taskfile.yml`, `bundle.sh`, `package.sh`, devenv's `build` and CI's build steps; `cmd/devloop`):
  - Development builds (what `dev` builds through `cmd/devloop`, whose `-ldflags` are only `-w`) do not compile the credentials in. Sona reads the same variables from the environment at runtime (`internal/scrobble/lastfm.go`, `internal/discord/discord.go`; devenv loads `.env`).
  - The Taskfiles' production and release builds keep the injection, with `-trimpath`, so `go version -m bin/sona` shows no `-ldflags`.
  - `ldflags.sh` reads the credentials from the environment, so no command line holds them. Devenv's `build` and CI's builds (no `-trimpath`) pass `--no-credentials`.

**Test.**

- `TestNewKeepsBuildCredentialsOutOfBuildInfoRecord` (a record shaped like the removed shell's Build Info, through `logging.New`'s JSON file and dev text handlers).
- `TestRedactBuildFlagsInText` (JSON and text records, bare, quoted and `-X=` forms, a command line; redacting twice changes nothing).
- `TestBundleScrubsBuildCredentialsFromOldLogs` (JSON and text Build Info lines in `sona.log` and `sona.log.1`).

### F11. The child could reach the user's terminals

Severity: Medium. Status: fixed.

**Issue.**

- The child started in Sona's session, so it kept Sona's controlling terminal, and `/dev` was the host's and writable under Landlock, with no ioctl rule and no seccomp filter on `ioctl`.
- A compromised child could `open("/dev/tty")` and `ioctl(TIOCSTI)` to push keystrokes into the terminal's input queue (run by the user's shell outside every sandbox layer; needs Sona started from a terminal and `dev.tty.legacy_tiocsti=1`). Independently it could open the user's other `/dev/pts/N` to write escape sequences (OSC 52 clipboard) or read typed input.
- macOS: the Seatbelt profile allowed writes under `/dev`, and the child kept its session [INFERENCE: not run on a Mac].

**Impact.** Arbitrary commands as the user (when `TIOCSTI` is enabled), otherwise input capture and escape-sequence or clipboard injection into the user's terminals. Same class as F2.

**Fix:**

- Every spawn path starts the child with `Setsid` (`newSession`, and in `namespaceAttr`): no controlling terminal, `/dev/tty` gives `ENXIO`. Windows needs nothing: the AppContainer child is detached from the console (`DETACHED_PROCESS`).
- Landlock writes under `/dev` are limited to `/dev/null`, `/dev/shm` and `/dev/snd`; ABI 5 and later also handle `LANDLOCK_ACCESS_FS_IOCTL_DEV` and grant it only there.
- In the mount namespace, `/dev/pts` is an empty read-only tmpfs.
- macOS: the profile denies write and ioctl on the terminal devices under `/dev` (`tty`, `ttys*`, `pty*`, `console`), after the `/dev` write allowance.

**Test.** `TestSandboxConfinesChild` (both subtests) opens `/dev/tty` and the slave of a pty the test created from inside the child: both must fail, while `/dev/null` still opens for writing. `TestSeatbeltProfile` keeps the terminal deny after the write allowance.

### F12. On Ubuntu the CDM host child could reach the session bus

Severity: Medium. Status: fixed for the `.deb` and `.rpm`; the AppImage keeps a residual risk (below).

**Issue.** On stock Ubuntu 23.10 and later (24.04 included) the child had no private filesystem view and nothing else kept it off the session bus:

- `kernel.apparmor_restrict_unprivileged_userns=1` creates an unconfined program's user namespace without capabilities. The child's first stage then could not make its mounts private, so it had no mount namespace and kept the real `$XDG_RUNTIME_DIR` ([Linux sandbox](#linux-sandbox)).
- Landlock refuses `connect()` to path Unix sockets (`LANDLOCK_ACCESS_FS_RESOLVE_UNIX`) only from ABI 9. Ubuntu 24.04's kernels, the HWE ones included, are below it, and seccomp allows `AF_UNIX`.
- The child's uid maps onto the real one, so the bus's `EXTERNAL` authentication accepted its connection to `/run/user/$UID/bus`.
- The CDM still loaded, because the load gate needs only seccomp and the network refusal ([F3](#f3-the-sandbox-failed-open)).
- The tests did not cover it: CI lifts the restriction, and `landlock only` asserted the refusal of path sockets only for ABI 9 and above, silently. The docs listed the case as an accepted residual risk without its impact.

**Impact.** A compromised CDM host (Google's closed code, exploited) could call `org.freedesktop.systemd1.Manager.StartTransientUnit` on the session bus and run code outside the sandbox: the impact of [F2](#f2-the-cdm-child-sandbox-blocked-only-inet-sockets). It could also drive `dev.coredev.Sona.Instance.Run` (including `private off` and `like`) and MPRIS. Found by code review of 0.5.0; the whole chain was not exercised on a live system.

**Fix:**

- **AppArmor profile** (`build/linux/apparmor/sona`): `profile sona /usr/bin/sona flags=(unconfined) { userns, … }`, the profile Chrome and Discord use on Ubuntu 24.04. The `.deb` and `.rpm` install it as `/etc/apparmor.d/sona` (a conffile), `postinstall.sh` loads it when `apparmor_parser` and AppArmor are there, and `preremove.sh` unloads it when Sona is removed (a separate `postremove.sh` refreshes the desktop caches only, so a profile file that dpkg keeps after a plain `remove` is not loaded again). The Arch package does not ship it: no Arch kernel carries the restriction. `flags=(unconfined)` leaves everything but user namespace creation unmediated.
- **Diagnostics** (`Sandbox.ReachesSockets`, `playback.SandboxGapSockets`): a child with no mount namespace and Landlock below ABI 9 now reports the gap `sockets` ("It can connect to your session bus and other Unix sockets"), and Settings → Diagnostics → Host sandbox says "Not fully sandboxed" with the way to fix it: install the `.deb` or `.rpm`, or run a kernel with Landlock ABI 9.
- **Not done on purpose:** refusing the CDM there, as F3 does without seccomp. It would stop full-song playback for every Ubuntu AppImage user, who cannot ship a profile. That trade-off is the maintainer's; the residual risk is documented ([Linux sandbox](#linux-sandbox)) and shown in Diagnostics.

**Residual risk.** The AppImage on Ubuntu 23.10 and later (and any install whose binary is not `/usr/bin/sona`, or where AppArmor does not load the profile) still has the child on the session bus below Landlock ABI 9. A profile for an AppImage would have to match its changing mount point, which lets any file there create user namespaces.

**Test.**

- `TestSandboxGapsReportReachableSockets` (`ReachesSockets` and the `sockets` gap for each combination of mount namespace and Landlock ABI 0, 4, 8 and 9).
- `TestSandboxConfinesChild`: `landlock only` spells out the condition (path sockets refused from ABI 9, reachable below it) and checks that the reported gap matches what the probe child observed; `namespaces` checks that nothing is reachable and no gap is reported.
- `TestSandboxRows` (the Diagnostics sentence and its hint).

### F13. Release builds ran third-party tools without a pin or a checksum

Severity: Medium (GHSA-f67w-fwwq-f3jj). Status: fixed.

**Issue.**

- Linux: `build/linux/appimage.sh` downloaded `linuxdeploy`, its AppImage plugin (the moving `continuous` release) and the GTK plugin (the head of `master`), and ran them as root in the release container with the finished `.deb`, `.rpm` and Arch packages in `/src/bin`, with `/out` and the Go caches mounted. A change reaching one of those upstream projects changed what ran with no commit of Sona's to show for it.
- Windows: `choco install pkgconfiglite` and `choco install nsis` named no version. `makensis` ran after the update archive was written into the output folder, and the archive is what the update key signs.
- The Go toolchain of the Linux release image and of the CI image was downloaded from go.dev without a checksum.
- The release job restored a Go cache that earlier runs on `main` (nightlies and releases) had saved, and saved a new one, so code a build wrote into it ran in the next signed build.

**Impact.** Code running in the release build could replace a package, the AppImage or the Windows update archive before it was checksummed, attested and (for the archive) signed. The result carried valid provenance and a valid update signature, and reached everyone who installed or self-updated. The cache let a one-off compromise persist.

**Fix.**

- **Pins.** linuxdeploy, its AppImage plugin and the GTK plugin come from fixed release tags and one commit, each checked against a SHA-256 in `build/linux/appimage-tools.sha256` before it is made executable. NSIS and pkg-config-lite install at a pinned version, and `build/windows/install-tool.sh` checks the installed files against a tree hash in `build/windows/tools.sha256`. The Go toolchain archive is checked against its published SHA-256 before it is unpacked (`ARG GO_SHA256` in the release Containerfile, `build/ci/go.sha256`, which is part of the CI image's tag).
- **Order.** `build/windows/package.sh` runs `makensis` first, on the package folder, which must hash the same afterwards, with the installer written to a scratch folder; the zip and the update archive are written after it, by 7z and tar. This narrows what a compromised NSIS could change; it still runs in the job, and the pin is what keeps it out.
- **Cache.** A release, nightly or prerelease run restores and saves no Go cache. A dry run keeps one under a key no publishing run reads (`release.yml`).

**Residual.** The pins are bumped by hand (Renovate cannot compute a hash); the GTK plugin has no releases, so it is pinned by commit. The macOS job's `brew install pkgconf` (only when the runner image lacks it) and the distribution packages of the build images, which come from signed apt archives, are not hash-pinned.

**Test.** Workflow and shell changes; no Go test. The release dry run (Actions → Release → Run workflow, no prerelease) builds, smoke-tests and exercises them.

### F14. CI and release images were trusted by a tag any branch could overwrite

Severity: Medium (GHSA-q2xm-pq49-7mxq). Status: fixed.

**Issue.**

- `images.yml` ran on `pull_request` with `packages: write`, and its pull request runs pushed to the same tag the main-scope runs pulled (`ghcr.io/coredev-uk/sona-ci:<tag>`; the release image was published too). Any workflow of a branch of the repository can push any tag of the packages linked to it. The release (`release.yml`, `release.sh`) pulled the image by tag, with no digest and no provenance check, and ran the build in it with the Last.fm and Discord values.
- `images.yml` did not rebuild an image that existed (`build=false`), so one a branch pushed stayed.

**Impact.** A backdoored image ran the release build, outside the protection of the `release` environment, and the packages it produced were attested and published with valid provenance.

**Fix.**

- **Who writes what.** A pull request's build goes to `pr-<tag>`; only a run on `main` writes `<tag>` and attests it. The release job does not pull an image: `release.sh` builds the Linux release image from the checked-out Containerfile, and `images.yml` publishes no release image.
- **What main trusts.** Main, the nightly and the release run the CI image only when `build/ci/find-image.sh` verifies the build provenance of `images.yml` on `refs/heads/main` (`gh attestation verify --signer-workflow … --source-ref refs/heads/main`), and use the image by the digest it verified (`image@sha256:…`). A branch's run can't produce that attestation; an image without one is rebuilt by the next run on main.
- **Base images by digest.** The `FROM` lines of both Containerfiles and the Flatpak workflow's `container:` image name a digest. `release.sh` pulls the digest the release Containerfile names and refuses a line without one. Renovate refreshes the digests weekly and never moves a tag; the release image's digest pull request is labelled `verdict/review`.

**Residual.** GHCR still lets any workflow of the repository write its packages, so a tag a branch overwrote exists; it is ignored, not prevented. The `sona-release` package left from before the release image was built in the job can be deleted by the maintainer, and `sona-ci`'s "Manage Actions access" is worth a look; both are settings this change does not touch. The smoke tests' Fedora and Arch images (and the Ubuntu one for the `.deb`) are pulled by tag on purpose: they get the packages read-only and no secret.

**Test.** Workflow and shell changes; no Go test. A pull request that changes `build/ci` builds `pr-<tag>` and its CI waits for it (the "CI image" job); the release dry run (Actions → Release → Run workflow, no prerelease) builds the release image from the pinned digest.

### F15. The release bot's App key was readable by branch runs

Severity: Medium. Status: fixed; since #608 the key is not in GitHub at all, and the exposed key has been rotated.

**Issue.**

- The bot-cecil GitHub App's client ID and private key (`RENOVATE_APP_CLIENT_ID`, `RENOVATE_APP_PRIVATE_KEY`) were repository-level secrets. Any workflow run on a branch of this repository could read them, including a `pull_request` run, whose workflow files come from the pull request's merge commit, so the pusher chooses the jobs. A fork's run gets no secrets; a branch push needs write access (a collaborator, or a compromised bot or agent credential).
- The workflows already named environments for every job that used the key, but the environments `bot-cecil`, `golden-update` and `nix-vendor-hash` held no secrets and had no branch policy or reviewers, so the repository-level copies were what the jobs read.
- The App can write contents, workflows, pull requests, issues, checks and statuses. Its key outlives the access of whoever read it, and its tokens could push a `v*` tag, which the `release` environment's policy at the time let in, reaching the update signing key.

**Impact.** A long-lived key that mints write tokens for the repository, in reach of anyone who could push a branch. No misuse was seen; the key was not rotated before this fix, so it has to be assumed read.

**Fix** (repository settings, 2026-11-21):

- The App's credentials are environment secrets only: `bot-cecil` (deploys from `main` and `v*` tags), `release` (from `main`), and `golden-update` and `nix-vendor-hash` (required reviewers, because their jobs run on a pull request's ref, which no branch policy admits). The repository-level copies are deleted. The master copy is the 1Password item "bot-cecil"; `docs/maintainers/ci-and-secrets.md` has the rotation steps.
- The "release tags" ruleset now also restricts creating `v*` tags (it already blocked updating and deleting them). Only repository admins and the bot-cecil App bypass it, so a write collaborator can no longer make a tag that satisfies the `bot-cecil` environment's policy.
- To rotate: generate a new private key in the App's settings, update the 1Password item, set it in the four environments, and delete the old key from the App.

**Test.**

- After the move, `nix-branch.yml` dispatched on `main` (run 44931021094) minted the bot-cecil token from the `bot-cecil` environment and pushed: the environment path works without the repository-level copies.
- With the ruleset change, a token minted for the App created and deleted a throwaway `v0.0.0-ruleset-check` tag (201, 204): the App's bypass keeps release-please and the nightly prereleases working.
- `gh secret list` shows no repository-level secret; each of the four environments lists both App secrets.
- Not tested: a push of a `v*` tag by a non-admin collaborator being refused (no such account was at hand); GitHub enforces the ruleset for every actor without a bypass.

**Since.** #608 took the key out of GitHub: octo-sts holds it, and each job exchanges its OIDC token for an installation token under a trust policy in `.github/chainguard/` that names the job's environment ([ci-and-secrets.md](maintainers/ci-and-secrets.md)). None of the four environments holds the App's secrets any more. The maintainer rotated the key on 2026-11-21, so the one the repository-level copies exposed no longer mints tokens.

### F16. A NUL byte or invalid UTF-8 in on-air metadata broke MPRIS

Severity: Low. Status: fixed.

**Issue.** Metadata strings from a station (an ICY `StreamTitle`, HLS `EXTINF` attributes, TuneIn's Describe JSON) reached `mpris.metadata` unchanged. godbus refuses to encode a string holding a NUL byte or invalid UTF-8, and none of the three sources removed either (`streamTitle` kept NUL, `extinfOnAir` did not check UTF-8, and JSON `\u0000` decodes to NUL).

**Impact.** While that title was current, every `PropertiesChanged` that carried `Metadata` was dropped, with the `PlaybackStatus` and `Volume` changes in the same signal, and `GetAll` and `Get(Metadata)` failed. Media widgets and `playerctl` lost Sona until the title changed. Availability of a desktop integration only, hence Low. A plain-http stream let an on-path attacker do the same.

**Fix** (`internal/mpris/server.go`): `metadata` passes every string through `wireString` at the D-Bus boundary: NUL bytes are removed and invalid UTF-8 becomes U+FFFD. A value that is only NUL is left out of the Metadata map, like an empty one.

**Test.** `TestStreamMetadataTheBusCannotCarry` (a title with NUL, an artist with invalid bytes and an album of only NUL, through `Update` and `GetAll` on a private bus; without the fix the signal never arrives).

### F17. The sona command printed other people's text to the terminal unfiltered

Severity: Low. Status: fixed.

**Issue.** `sona status`, `lyrics` (plain, `--lrc` and `--follow`), `queue`, `search`, `library` and the error lines printed titles, lyrics and service messages as they came. The lyrics come from LRCLIB or Musixmatch community lyrics, the titles from Apple, a station or a library upload. The LRC parser only trims whitespace, and the text formatters replaced only CR and LF (`oneLiner`); only `--json` escaped.

**Impact.** The source of a lyric or a title controlled what the user's terminal did: the window title, the screen's content, OSC 8 links, an OSC 52 clipboard write (kitty and WezTerm allow it by default), which enables paste-jacking. It needed the user to run the command in a terminal, hence Low.

**Fix** (`internal/cli/plain.go`, `run.go`, `lyrics.go`):

- `plain` returns a deep copy of a result with every string passed through `TerminalSafe` (the filter `addons dev` already uses: escape sequences in their 7-bit and 8-bit forms are removed whole, other control, format and bidirectional characters and line breaks become a space, invalid UTF-8 becomes U+FFFD). It walks the result's structs, slices, maps and pointers by reflection, so a field added later is covered.
- It is applied where a result becomes text: `decodeText` for every command's reply, `writeStatus` for `status --follow` and the status line, `writeLyricsNow` for `lyrics --follow` and its templates, and `Env.write` for every error line.
- Values reach `--format` templates already cleaned. The template text is the user's and is printed as written, so a template may still colour its own output.
- `--json` is the data as received: JSON escapes control characters.

**Test.**

- `TestTextOutputHasNoTerminalControls` (every text command with OSC 52, OSC 0, CSI, 8-bit CSI/OSC, BEL, NUL and a bidi override in each field, and in an error from the service).
- `TestFollowedLyricsLineHasNoTerminalControls`.
- `TestJSONOutputIsNotChanged`.
- `TestPlainCopiesAndLeavesTheOriginal`.

### F18. The diagnostics bundle carried the person's city and device names

Severity: Low. Status: fixed.

**Issue.** The bundle's Settings section printed every setting except the credential and identity keys `diag.dropKey` names (and `lastRoute`). The Concerts city (`gb-manchester`), the sidebar's open folders and the audio output device (a PipeWire node name, which can carry a USB serial number) were in it. Settings export already treats these as belonging to one person and one computer (`config.localKeys`).

**Impact.** The bundle is meant to be pasted into a public GitHub issue, and the site guide promised only tokens and the e-mail address were removed. The reporter's city and device identifiers were published.

**Fix** (`internal/config/diagnostics.go`, `internal/services/system_bundle.go`):

- The bundle's Settings section is `config.ForDiagnostics`, not the raw settings. It leaves out `concerts.city`, `sidebar.openFolders`, `audio.outputDevice` and every add-on's `values` (text the person typed into an add-on's settings).
- It lists what it dropped under `_withheld`, so a maintainer reading a bundle knows a key is missing on purpose and not lost.
- It keeps what a bug report needs: theme, quality, equaliser, crossfade, shortcuts, integrations, which add-ons are enabled and the permissions granted, and the rest.
- The user-facing guide says what the report leaves out.

**Test.**

- `TestForDiagnosticsWithholdsWhereThePersonIs` (the city, open folders, device name and an add-on value are absent; the theme, equaliser and grants stay; `_withheld` lists the paths).
- `TestForDiagnosticsLeavesTheSettingsAlone`.
- `TestDiagnosticsBundleSettingsLeaveOutLocation` (the rendered bundle).

### F19. The motion-artwork remuxer panicked on a huge tfhd base offset

Severity: Low. Status: fixed.

**Issue.**

- `flatten` (`internal/motion/remux.go`) read a `tfhd` base-data-offset as `int64(u64)` and only rejected negative values, so any value up to `MaxInt64` passed.
- The sample bounds check added the sample size to the position (`pos+int64(size)`). With a base near `MaxInt64` the sum overflowed negative, passed the check and sliced the source out of range.
- `flatten` runs inside `singleflight.DoChan`, which re-panics in a new goroutine, so the panic could not be recovered. Nothing was cached, so every view of the album downloaded the rendition again and crashed again.

**Impact.** A rendition from a compromised CDN or a broken TLS session crashed the UI process on every view of the album. Availability only; the allow-list and redirect checks of [F6](#f6-motion-artwork-followed-redirects-anywhere) still apply.

**Fix** (`internal/motion/remux.go`): a `tfhd` base beyond the source is a bad `tfhd`, and the sample check compares the size with the remaining length (`int64(size) > len(src)-pos`), so nothing overflows.

**Test.** `TestFlattenHugeBaseDataOffset` (base `MaxInt64`, `MaxInt64-1` and one past the file, default sample size 1).

### F20. The fMP4 parser panicked on crafted init segments and fragments

Severity: Low. Status: fixed.

**Issue.**

- `ParseInit` checked that the `stsd` entry count was not zero but not that the entry list held any entry, so a count of 1 with an empty body made `entries[0]` panic.
- `ParseFragment` computed the `saio` aux-info position as `base + offset` and tested `pos + size` against the fragment length. An offset of 2^64-1 and a size of 1 wrap that sum to 0, pass the test and slice `b[2^64-1:0]`.
- The parser runs in the parent (`native` streams, prefetch goroutine) and in the CDM host child; none of those goroutines recover.

**Impact.** A crafted init segment or fragment from a compromised CDN or a broken TLS session crashed Sona when the item played or was prepared (in the child only playback died). Availability only.

**Fix** (`internal/playback/fmp4/init.go`, `fragment.go`; no parsing moved across the CDM boundary):

- `ParseInit` returns `fmp4: empty stsd` when the entry list is empty.
- `parseAux` rejects an offset or base beyond the fragment and compares every entry size against the remaining length by subtraction, so nothing wraps.

**Test.** `TestParseInitEmptyStsdEntryList`, `TestParseFragmentWrappingSaioOffset`.

### F21. Server-declared counts and lengths allocated without limit

Severity: Low. Status: fixed.

**Issue.**

- `fmp4.ParseFragment` trusted a `trun` sample count. Without per-sample fields the length check cannot fail, so a count up to 2^32-1 with a default sample size of 0 appended a `Sample` (~88 bytes) per entry, hundreds of GB. The parser runs in the parent (`native` streams) and in the CDM host child.
- `native.fetchOnce` allocated `make([]byte, size)` from an `#EXT-X-BYTERANGE` length (`hls.parseRange` only requires a positive number) and preallocated from `Content-Length`; `downloadOnce` allocated `make([]byte, resp.ContentLength)` for an upload. Lengths above 2^48 panic, in a goroutine nothing recovers.

**Impact.** One crafted response crashed or exhausted the memory of the UI process. Availability only; it needs control of an Apple media response over TLS.

**Fix:**

- `internal/playback/fmp4/fragment.go`: at most 65536 samples per fragment (`maxSamples`; Apple's segments hold a few hundred) and a zero-size sample is an error. No parsing moved across the CDM boundary.
- `internal/playback/native/media.go`: one fetched resource is at most 64 MiB (`maxFetchBytes`). A larger byte range fails before any request, a larger `Content-Length` before the body is read, and a body without a length is read through a limit of the cap plus one byte. These fail with `errTooLarge`, which is not retried.
- `internal/playback/native/upload.go`: an uploaded file held in memory is at most 2 GiB (`maxUploadBytes`), for a declared length and for a body without one.

**Test.** `TestParseFragmentBoundsSampleCount`, `TestParseFragmentRejectsEmptySamples`, `TestFetchRefusesHugeDeclaredLengths`, `TestDownloadRefusesHugeContentLength`.

### F22. Artwork from the CDN was decoded without a pixel limit

Severity: Low. Status: fixed.

**Issue.**

- `Cache.fetch` capped the download at 16 MiB and read the image header, but never looked at the declared width and height. A ~1.5 MB PNG can declare 40000×40000 8-bit grey.
- `Cache.source` wrote that file to the disk cache before anything decoded it, and `decode` ran `png.Decode` or `jpeg.Decode` on the whole picture (1.6 GB), after which resampling made an RGBA copy (6.4 GB).
- The upload path was bounded (`MaxCoverPixels`); the CDN path was not.

**Impact.** Go cannot recover from running out of memory, so the UI process died, and the cached file crashed Sona again on every view until the cache was cleared. Availability only; it needs a hostile artwork response from Apple's CDN (or a broken TLS session).

**Fix** (`internal/artwork/image.go`, `artwork.go`):

- `checkPixels` refuses a header with no area or more than `MaxSourcePixels` (the cover bound, 40 million) with `ErrTooManyPixels`.
- `fetch` runs it right after reading the header, so a rejected image is never stored.
- `decodeImage` runs it before any decoder allocates, so a bomb cached by an older build fails on every surface (`Get`, `Backdrop`, `DominantColors`, mosaics, uploads) with an error instead of an out-of-memory crash.
- Artwork is still decoded once at the size it is shown (resampled by `fit` and `cover`); the limit only rejects sources far past any real cover.

**Test.** `TestSourcePixelLimit` (a 40000×40000 PNG header served by the CDN fails `Get` and `Backdrop` and leaves the cache empty; `decodeImage` and `decode` of a cached bomb, and edge cases of the limit, fail with `ErrTooManyPixels`).

### F23. Local broadcaster streams followed any URL

Severity: Low. Status: fixed.

**Issue.** The stream client for a local broadcaster's URL was the engine's default HTTP client: any host, up to 10 redirects, https to http allowed. A sniffed playlist was resolved and every entry played with no address or scheme check and no delay, nested to 3 levels, and HLS segments went through the same client.

**Impact.** A hostile or on-path-modified broadcaster URL (often plain `http`) made Sona send blind GETs to `127.0.0.1`, RFC 1918 addresses and `169.254.169.254`, with a browser User-Agent and `Icy-MetaData: 1`, past a browser's Private Network Access. A 1 MiB playlist with tens of thousands of entries per level amplified that. No response data leaves Sona. Severity Low (about CVSS 3.4, AV:A/AC:H/PR:N/UI:R/S:C/I:L).

**Fix** (`internal/playback/broadcast/guard.go`, `broadcast.go`, `internal/playback/native/engine.go`):

- `broadcast.NewClient` is the engine's `streamHTTP`: a transport whose `net.Dialer.Control` refuses loopback, private, link-local, carrier-grade NAT, unspecified, multicast and reserved addresses on the IP dialled (so DNS rebinding and redirects are covered; IPv4-mapped IPv6 and NAT64 are judged as IPv4). The environment's proxy is not used (see the component section).
- Redirects: at most 5, `http` or `https` only, no https to http.
- A playlist's first 16 entries are tried, with a growing pause (100 ms steps, up to 1 s).
- Playlists, HLS playlists and segments and reconnects all use the session's one client.
- Tests serve from `httptest` on loopback by giving the engine a client without the guard (`Engine.streamHTTP`) or the broadcast tests a plain client; production never does.

**Test.**

- `TestDialControl`, `TestClientRefusesLoopback`, `TestBroadcasterCannotAimRequestsAtPrivateAddresses` (PLS, M3U, HLS, redirect: the target gets no request), `TestRedirectCap`, `TestCheckRedirect`, `TestPlaylistEntryCap`, `TestPlaylistEntryBackoff`.
- `TestEngineStreamClientRefusesLoopback`.

### F24. CDM message events started unbounded renewal goroutines

Severity: Low. Status: fixed. Completes [F8](#f8-a-compromised-child-could-exhaust-the-parents-memory).

**Issue.** F8 bounded the frames and the queue of undelivered events, but the engine drained that queue at once and ran `go e.renew(...)` for every `EventMessage`.

- Each goroutine kept the message's `Data` (up to the 16 MiB frame limit) and waited on the media's licence lock; the ones that got through sent a licence request to Apple with the user's token.
- `Session` and `Err` had no limit but the frame's.

**Impact.** A compromised child that knows a session ID could exhaust the parent's memory (the thing F8 was meant to rule out) and send back-to-back licence requests to Apple with the account's credentials.

- Severity Low, as F8: it needs a compromised child, and it affects availability only.

**Fix:**

- **Payload limits** (`internal/playback/cdmhost/host.go`, `checkEvent`). A child event whose payload no legitimate one has ends the child like any F8 violation (`errMisbehaved`):
  - an `EventMessage` with more than 64 KiB of `Data` (`maxMessageData`; licence requests and renewals are a few KiB);
  - a `Session` over 256 bytes (`maxEventSession`; session IDs are a few bytes);
  - an `Err` over 16 KiB (`maxEventErr`; one error message).
- **One renewal at a time** (`internal/playback/native/renewals.go`, `engine.go` `watch`):
  - Each host has one renewal goroutine, not one per message.
  - A message for a session the engine does not own is dropped without work (`queueRenewal`).
  - A message for a session that already has one waiting replaces it, so there is at most one pending message per session. Memory is bounded by the number of sessions times 64 KiB, and Apple sees at most one renewal request at a time.
  - When the host exits, the pending messages are dropped.
- The CDM host child stays free of network access: it only sends the messages. The DRM rules are unchanged.

**Test.**

- `TestOversizedEventPayloadKillsChild` (`cdmhost/frames_test.go`): a child sends an oversized message, session ID and error, one at a time.
- `TestRenewalFloodIsBounded` (`native/renewal_test.go`): 5,000 maximum-size messages for a live session and 5,000 for unknown ones while Apple's licence endpoint is held. The goroutine count stays flat, one renewal is in flight and one coalesced, and no other request is made.

### F25. The Last.fm approval token was in the browser's command line

Severity: Low. Status: fixed (residual risks listed).

**Issue.** Connecting Last.fm opened `https://www.last.fm/api/auth/?api_key=…&token=…` with the system's URL opener, so the approval token was in the browser's (or launcher's) argv, which every user can read in `/proc/<pid>/cmdline` unless `/proc` is mounted with `hidepid` (the channel of [F7](#f7-another-local-user-could-complete-the-loopback-sign-in)). Sona then polls `auth.getSession` every 3 s with that token. The request is signed with the API secret, which is compiled into every release ([F10](#f10-development-builds-logged-the-build-credentials)), so it is no secret from another user.

**Impact.** Another local user could poll faster and get a Last.fm session key for the victim (scrobble, now playing, love, tags) until it is revoked, or approve the token with their own account first, so that Sona connects to the attacker's account and sends the victim's listening history there. Whether Last.fm tokens are single-use is unconfirmed. Linux only, another local user, no `hidepid`, hence Low.

**Fix** (`internal/services/scrobble_link.go`, `scrobble.go`; `internal/auth/mut/peer.go`):

- `approveLastFM` no longer opens the page the add-on names. It starts `approvalLink`, a loopback HTTP server on a random `127.0.0.1` port that serves one path (`/lastfm/<130 random bits>`), and gives the system that address. The page, with its token, is only the `Location` of the link's redirect, which the user's browser follows: the token is never in an argv.
- **Socket owner (Linux).** The listener is `mut.OwnerOnly`, the owner check of [F7](#f7-another-local-user-could-complete-the-loopback-sign-in): it drops a connection whose client socket another user owns, so reading the link from `/proc` gets another user nothing.
- **Single use.** The first load redirects; a second gets 410. If someone else loaded it first (on a platform without the owner check) the user's browser is refused, so the user connects again instead of Sona silently connecting to another account.
- **Narrow.** Only `GET` of the exact path with `Host` equal to the listener's address is answered (a rebound name or another method is 404). Answers are `no-store` with `Referrer-Policy: no-referrer`.
- **It ends with the approval:** the server is closed when the connect succeeds, fails, times out (10 minutes) or is cancelled.
- The Last.fm page the user approves on is unchanged, and so is polling. If the loopback server cannot start, the connect fails with the browser-open error; it never falls back to putting the token on the command line.

**Test.**

- `TestLastFMApprovalTokenIsNotOnTheBrowsersCommandLine` (the address Sona hands the system holds no token, key or Last.fm host; a browser that follows it ends on the approval page with the token).
- `TestApprovalLinkWorksOnce` (a wrong path, the root, POST, HEAD and a rebound Host are 404 and do not use the link; the first load is 302 to the page, the second 410 with no page, and Close stops the listener).
- `TestApprovalLinkIsClosedWhenTheApprovalEnds`.
- The owner check itself is `TestOwnerListenerDropsOtherUsers` (`internal/auth/mut`).

**Residual risks.**

- On macOS and Windows the owner check is not implemented. macOS refuses another user's `KERN_PROCARGS2`. Windows does not show another user's command line to a standard user [INFERENCE: not tested]. So the link should not leak there either; if it did, the one-use rule would turn the race into a refused browser instead of a stolen token.
- The token still travels in the browser's own address after the redirect (history, a synced browser). That is the user's browser, not another user's view.
- A browser that cannot reach `127.0.0.1` on Sona's host (a different network namespace) cannot open the link. Flatpak and Snap browsers share the host's, and their sockets carry the user's uid ([F7](#f7-another-local-user-could-complete-the-loopback-sign-in)).
- Last.fm's own page still shows the token in its address bar and is subject to Last.fm's handling of tokens.

### F26. The update manifest's version and platform were not signed

Severity: Low. Status: fixed (residual risks listed).

**Issue.** `cmd/releasesign` signed only the archive's SHA-256, the signature MyGo's updater checks. The manifest's `version` was not covered, and neither was the platform it was published for. `Feed.Manifest` took any well-formed version, `UpdatesService` offered it when `Newer(version, current)`, and `Source.Open` checked the digest signature alone, whatever version or target the manifest said. MyGo's own `checkUpdate` compares the same unsigned version, and nothing noticed that a lower version was running afterwards.

**Impact.** Whoever could publish or replace release assets without the signing key (a `contents: write` token, or a compromise of GitHub or its TLS) could put an old genuine archive (known bugs, or a release candidate) under the version `99.0.0`, or another platform's archive under this platform's manifest name. Installs would be offered the update and, once the user chose Install, move to the older build, be held back or looped, or get the wrong platform's files. No arbitrary code: the digest signature still held. CVSS AV:N/AC:H/PR:H/UI:R/C:N/I:L–H/A:L.

**Fix** (`cmd/releasesign/main.go`; `internal/update/statement.go`, `manifest.go`, `source.go`; `internal/services/updates.go`):

- `releasesign` also writes `sha256` (the digest, hex) and `statement_signature`: an Ed25519 signature, by the same key, of `sona-update-v1 NUL target NUL version NUL channel NUL hex(sha256)`. The prefix names and versions the format, the target is `darwin-arm64` or `windows-amd64`, and the channel is `prerelease` for a version with a pre-release part, else `stable`. No field can hold a NUL, so two different statements are never the same bytes.
- `Feed.Manifest` verifies it with the build's public key (`VerifyStatement`) before the manifest is used: another version, platform or digest than was signed together, or no statement, is `ErrSignature` (`update_signature`). It also refuses a pre-release on the stable feed and, on the test-version feed, a manifest whose version is not the newest tag's.
- `Source.Open` (before MyGo sees anything) checks the statement again, refuses a version not newer than the running one (`ErrNotNewer`; a build with no release version never updates), downloads, and requires the bytes' SHA-256 to be the statement's before the digest signature. MyGo's own digest signature is unchanged and still checked.
- Installs that predate the statement keep working: MyGo reads the manifest ignoring unknown fields, and checks the same digest signature.

**Test.**

- `internal/update`:
  - `TestReplayedOlderManifestIsNotInstalled` (an older genuine manifest, an older pre-release, the running version and a build with no version: `ErrNotNewer`, and the archive is never requested).
  - `TestCrossPlatformManifestIsRefused` (another platform's archive with valid signatures for that platform: refused by `Feed.Manifest` and `Source.Open`, nothing downloaded).
  - `TestTamperedManifestIsRefused` (a changed version, digest, digest format or statement, a statement by another key, and a manifest without one).
  - `TestSignedManifestIsAccepted` (the valid manifest passes the feed and the source and the archive is served).
  - `TestSourceHoldsTheDownloadToTheStatementsDigest`, `TestFeedNeedsTheBuildsKey`, `TestFeedRefusesTheWrongChannelOrRelease`.
  - `TestStatementFormat` (the signed bytes and a signature over fixed inputs), `TestChannelOf`.
- `cmd/releasesign`: `TestReleasesignWritesVerifiableManifest` (the statement verifies for its platform and version and for no other).
- `internal/services`: `TestSelfUpdateRefusesAVersionTheKeyDidNotSign` (the advisory's reproduction: a manifest claiming `99.0.0` is refused at the check with `update_signature`, nothing is offered and the app is untouched).

**Residual risks.**

- A genuine older manifest replayed under the current one's name is authentic and never newer, so it is not installed; it can hide a newer release until the real manifest is restored. The statement carries no time or counter.
- A release signed before this change has no statement and is refused by builds that check it. Run `releasesign` again over its archives to publish one that is offered.
- Whoever holds `SONA_UPDATE_KEY` can sign any statement. The key's protection is `docs/maintainers/ci-and-secrets.md`.

### F27. The loopback sign-in could be completed with another account's token

Severity: Low. Status: fixed.

**Issue.** [F7](#f7-another-local-user-could-complete-the-loopback-sign-in) bound a sign-in to the browser Sona opened with the `sona_signin` cookie, but nothing bound the fragment to Apple's redirect. The nonce and port are in the start link's argv. A page in the victim's browser during the 10-minute window could navigate it to `http://127.0.0.1:<port>/auth/<nonce>/callback#<btoa({"musicUserToken":"<attacker's token>"})>`. The connection is the victim's browser, so the owner check passes; the callback page posts the fragment to `/complete`, and the `SameSite=Strict` cookie is sent on that same-origin `fetch` (measured in F7). Sona stored whichever token it was handed first, once `/v1/me/storefront` accepted it.

**Impact.** Sona used the attacker's account: the victim's plays and library edits went to it and the attacker saw what the victim listened to. Needs another local user (Linux without `hidepid`) and attacker content in the victim's browser at the right time, injected before Apple's redirect. CVSS 3.3 (AV:L/AC:H/PR:L/UI:R/C:L/I:L).

**Fix** (`internal/auth/mut/loopback.go`):

- Each attempt draws a 256-bit `state` (`crypto/rand`, base64url) with its nonce and cookie. It is a path segment of the callback URL Sona gives Apple as `referrer`: `/auth/<nonce>/callback/<state>`. It is in no start link, no cookie and no argv.
- `GET /callback/<state>` serves the page only for the right state (constant-time compare). `/callback`, a wrong state and extra segments are 404 and the body does not contain the state.
- The page carries the state, strips it from the address bar (`history.replaceState` to `/auth/<nonce>/callback`) and posts `{fragment, state}`.
- `POST /complete` requires the state as well as the cookie: constant-time compare, 403 otherwise. A refused request does not end the attempt, so the real redirect can still complete it.
- The callback page has `Referrer-Policy: no-referrer` and loads nothing, so the state goes nowhere but the page and `/complete`.
- A foreign fragment is now useless to an attacker: to post it they need the state, which only Apple's redirect (and the browser that follows it) holds.

**Test.**

- `internal/auth/mut`:
  - `TestCompleteRequiresTheCallbackState` (the foreign fragment with the victim's cookie and no state, a wrong, truncated or extended state, or the nonce gets 403 and leaves the attempt pending; the right state completes and `Wait` returns the token).
  - `TestCallbackPageNeedsTheState` (the bare callback path, a wrong state and an extra segment are 404 without the state; the right path serves a page with `replaceState` to the stateless path and `Referrer-Policy: no-referrer`).
  - `TestStateIsFreshAndOutsideTheStartLinkAndCookie`.
  - `TestHappyPathWithCookieJar`, `TestStartPageRedirectsToAppleWithCallbackReferrer` (the authorize URL's referrer carries the state).
- `internal/auth`: `TestLoopbackSignIn` (the manager's browser takes the state from the authorize URL's referrer, as Apple's redirect returns to it).

**Live check.** That Apple's authorize page redirects back to the full `referrer` path, not only its origin, was read from its script (`sessionStorage.ac` takes the whole `referrer` href) and then run with a real account on 2026-11-21 (Linux, private window): `go run ./cmd/authspike -open=false` received the token through `…/callback/<state>` and printed `signed in: storefront=…`. Safari is still to run ([auth.md](auth.md#live-check-of-the-callback-path)). If Apple ever dropped the path, the sign-in would fail closed (404 page, timeout), never open.

### F28. On macOS any program could read Sona's keychain items without a prompt

Severity: Medium (GHSA-49cj-524r-6gp3). Status: fixed (residual risks listed).

**Issue.** `internal/auth/store` kept its secrets with go-keyring v0.2.8, whose macOS backend runs `/usr/bin/security add-generic-password` and `find-generic-password`. The keychain trusts an item to the program that creates it, so these items let `/usr/bin/security` (requirement `identifier "com.apple.security" and anchor apple`) read the secret without asking, and no other program, Sona included. Any program can run `/usr/bin/security`. The comment on `seatbeltKeychainServices` and the [macOS sandbox](#macos-sandbox) table said the item trusted Sona's executable.

**Impact.** Any unsandboxed program of the same user could run `security find-generic-password -s sona -a mut -w` and get the Music User Token, and likewise the Last.fm session key and the ListenBrainz and Musixmatch tokens, with no prompt: the Apple Music library, history and playlists, not the Apple Account. Elsewhere macOS asks the user before one program reads another's keychain item, and Linux's Secret Service and Windows' Credential Manager have no such check, so only macOS lost a protection. CVSS 4.4 AV:L/AC:L/PR:L/UI:N/C:L/I:L.

**Fix** (`internal/auth/store/keyring_darwin.go`):

- On macOS `store.SystemKeyring` calls Security.framework itself (`SecItemAdd`, `SecItemCopyMatching`, `SecItemUpdate`, `SecItemDelete`, through purego). Sona creates its items, so only Sona's executable may read them without a prompt. go-keyring stays on Linux and Windows (`keyring_gokeyring.go`).
- Sona's items carry the creator code `Sona`. A keychain holds one generic password per service and account, so an item for `sona`/`<key>` without the code is go-keyring's. `Get` reads it through `/usr/bin/security`, its owner, deletes it the same way and stores the secret in an item Sona creates. `Set` and `Delete` remove it too. None of this asks the user.
- `Set` updates Sona's item in place, which keeps its access list. An earlier build's item that this build may read but not delete (the user chose Allow, not Always Allow, after an update) is overwritten with a tombstone by `Delete`, which any program may do, and a tombstone reads as no item, so signing out still removes the token.
- The comment on `seatbeltKeychainServices` and the sandbox table are now true: Sona's items trust Sona's executable, which is also the CDM host child's, so the child's profile still denies the keychain's Mach services.

**Test** (`internal/auth/store/keyring_darwin_test.go`, macOS only, each on a throwaway keychain with user interaction off, so a prompt fails the test instead of waiting):

- `TestKeychainItemTrustsOnlySona` (the advisory's check: after `Set` the only application the item lets read its secret is the test binary, not `/usr/bin/security`, per `security dump-keychain -a`).
- `TestKeychainMovesGoKeyringItems` (an item made as go-keyring made it, base64 and hex: `Get` returns the plain secret without a prompt and leaves an item only the test binary may read).
- `TestKeychainSetAndDeleteReplaceGoKeyringItems`, `TestKeychainDeleteTombstonesAnItemItMayNotDelete`, `TestKeychainRoundTrip`.

**Residual risks.**

- Sona is signed ad hoc, so the keychain trusts one build's code hash, and the first read after an update makes macOS ask "Sona wants to use your confidential information". Always Allow adds the new build. Deny leaves the items unreadable to it: Sona falls back to the 0600 file as when the keyring is unavailable, and the user signs in again. A stable signing identity would end the prompts.
- An item is still writable by any program (its `encrypt` authorisation is open to all, as for every item), so another program can replace a secret, though not read it.
- A secret is readable by any program until Sona first starts after the update, which moves it.

## Areas reviewed without findings

- native Apple ID input handling
- the artwork and motion artwork caches' file and path handling
- parent→child cdmhost framing
- single-instance `Activate` and launch URLs
- the other MPRIS methods
- notifications
- the portal file chooser
- the settings file
- secret stores
- the scrobble queue
- the diagnostics bundle
- autostart, and the update check's request (its manifest is [F26](#f26-the-update-manifests-version-and-platform-were-not-signed))

## Run the tests

```sh
go test -race ./internal/playback/cdm/... ./internal/playback/cdmhost/... ./internal/config/... ./internal/discord/... ./internal/motion/... ./internal/mpris/... ./internal/playback/ ./internal/playback/coordinator/... ./internal/logging/ ./internal/diag/ ./internal/auth/... ./internal/i18n/... ./internal/lyrics/... ./internal/addons/...
```
