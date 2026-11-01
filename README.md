# Sona

Sona is an Apple Music client for the desktop, built first for Linux. It is a
native GTK4 / WebKitGTK 6 app (Go + [Wails v3](https://v3.wails.io), React
frontend) that starts from a local cache, plays full tracks through Google's
Widevine CDM (downloaded by Sona from Google, see below), and integrates with
the desktop: MPRIS media controls, `music://` links, Secret Service
credentials, PipeWire/PulseAudio output.

You need an Apple Music subscription. Sona is not affiliated with Apple or
Google.

## Platforms

| Platform             | Status                                                                                                                                                                                                                                              |
| -------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Linux, x86_64        | Full: playback, media controls, notifications, tray, `sona` commands. Release packages: `.deb`, `.rpm`, Arch, AppImage. A Flatpak manifest builds from the release `.deb` for local use; it is not on Flathub ([docs/flatpak.md](docs/flatpak.md)). |
| macOS, Apple Silicon | Development only: built from source, unsigned, no `.app` bundle or package. Full playback through the downloaded CDM and Core Audio; no media controls, notifications, autostart or `sona` commands yet. Intel Macs are not supported.              |
| Windows              | The UI builds; no playback yet.                                                                                                                                                                                                                     |

Where Google publishes no Widevine CDM (Linux on arm64), Sona has no
playback and says so (`unsupported_platform`).

## Install (Linux)

From the [releases page](https://github.com/coredev-uk/sona-go/releases)
(each release lists SHA256SUMS):

| Distribution                    | File                                  | Install                                                   |
| ------------------------------- | ------------------------------------- | --------------------------------------------------------- |
| Ubuntu 24.04+ and Debian family | `sona_<version>_amd64.deb`            | `sudo apt install ./sona_<version>_amd64.deb`             |
| Fedora (tested on 45)           | `sona-<version>-1.x86_64.rpm`         | `sudo dnf install ./sona-<version>-1.x86_64.rpm`          |
| Arch Linux                      | `sona-<version>-1-x86_64.pkg.tar.zst` | `sudo pacman -U ./sona-<version>-1-x86_64.pkg.tar.zst`    |
| Anything else                   | `sona-<version>-x86_64.AppImage`      | `chmod +x` and run; AppImageUpdate can update it in place |

Each release is smoke-tested on clean Ubuntu 24.04, Fedora 45 and Arch Linux
containers. Release assets also carry the AUR `sona-bin` PKGBUILD.

Start Sona from your launcher and sign in with your Apple Account. It
downloads the Widevine CDM by itself.

Requirements, which the packages declare (the AppImage bundles all but the
audio server):

- A Wayland or X11 session.
- GTK 4.14+, WebKitGTK 6.0, GStreamer 1.x with the base, good, bad (WebKit's
  media players) and libav (AAC decoding) plugin sets, and PipeWire or
  PulseAudio.
- Optional: a Secret Service provider (GNOME Keyring, KWallet) for the Apple
  Music token. Without one Sona falls back to a 0600 file in
  `~/.config/sona/` and says so.

## The Widevine CDM

Apple Music's full tracks are protected with Widevine, so Sona plays them
through Google's Widevine Content Decryption Module (CDM), the same library
Chrome and Firefox use. Sona does not include it. Instead:

- After the window first appears, Sona downloads the CDM from Google's
  servers, with no prompt. The version and its SHA-512 come from Mozilla's
  Firefox manifest (the one Firefox uses to fetch the same CDM), and Sona
  verifies the download against that hash and Google's CRX3 signatures
  before installing it in its data directory.
- It checks for updates daily, and loads a new version only after a test
  load in a sandboxed process succeeds; a broken update rolls back.
- It loads the CDM in a separate, sandboxed process with no network access.
  Decrypted audio never leaves that process and is never written to disk.
- Until the CDM is ready, browsing works and playback waits; Settings →
  Playback shows the download and has a Retry button.
- Only the copy Sona downloaded itself is ever used: not a browser's, not a
  system copy, not a file you supply. There is no override.

For users this means the first start downloads about 22 MB from Google's
servers, and the CDM lives under Sona's data directory (`widevine/`); deleting
it makes Sona fetch it again.

For packagers: never bundle, mirror or patch the CDM. It is Google's
proprietary software; Sona's packages do not contain it and must not.
Sandboxed formats need network access for the download and a writable data
directory. Where Google publishes no CDM for a platform, Sona runs without
playback.

## Private listening

Settings → Playback → Private listening (also the player bar, the tray, the
command palette and `sona private on|off|toggle|status`) keeps plays out of
your Apple Music listening history and recommendations (they are reported to
Apple marked private, as Apple's web player does). While it is on, Sona also
sends no Last.fm or ListenBrainz scrobbles, shows no Discord status and adds
nothing to its History page. `sona private …` acts on the running Sona and
prints the state; with no Sona running it changes nothing.

Development builds start in private mode, and `SONA_PRIVATE=1` forces it in
any build ([CONTRIBUTING.md](CONTRIBUTING.md#private-mode)).

## Build from source

The whole toolchain comes from [devenv](https://devenv.sh) 2.4 or newer, on
[Nix](https://nixos.org/download) with flakes: Go 1.26, Node 24 + pnpm,
GStreamer, and on Linux the GTK4/WebKitGTK development files. Nothing else
needs installing. Without a devenv install, write
`nix run github:NixOS/nixpkgs/nixpkgs-unstable#devenv --` where the commands
below say `devenv` (your registry's `nixpkgs#devenv` may be older than 2.4,
which the project refuses).

### Linux

```sh
git clone https://github.com/coredev-uk/sona-go.git
cd sona-go
devenv shell -- go tool wails3 build   # frontend + bindings + bin/sona
devenv shell -- bin/sona --version
devenv shell -- bin/sona
```

The first `devenv shell` downloads the toolchain; `wails3 build` installs
the frontend dependencies, builds the frontend, regenerates the bindings and
writes `bin/sona` (a production build, so it follows your saved private
listening setting; `SONA_PRIVATE=1 bin/sona` forces private mode). To try it
without touching an existing Sona's settings and library, give it its own
directories and session bus:

```sh
try=$(mktemp -d)
devenv shell -- env SONA_PRIVATE=1 \
  XDG_CONFIG_HOME="$try/config" XDG_DATA_HOME="$try/data" \
  XDG_STATE_HOME="$try/state" XDG_CACHE_HOME="$try/cache" \
  dbus-run-session -- bin/sona
```

Keep `env` before `dbus-run-session`, so the session's own services (a
Secret Service such as GNOME Keyring, the portals) use the same directories
and never reach your real keyring. Sona then stores any credentials in a
0600 file in that directory and logs that it did.

`devenv shell` on its own gives an interactive shell with everything on
`PATH`; inside it, `build`, `dev` (Vite hot reload), `tests`, `check` and
`fmt` are shortcuts ([CONTRIBUTING.md](CONTRIBUTING.md#devenv-scripts) lists
them all).

If Nix fails to fetch devenv's inputs with an ssh host-key error, your git
config rewrites https URLs to ssh. Run devenv once with an empty config
directory, `XDG_CONFIG_HOME=$(mktemp -d) devenv shell -- true`, to lock the
inputs, then drop the prefix (kept, it would also move Sona's settings).

A binary built in devenv links against `/nix/store` and runs on the build
machine only. Distributable packages are built on Ubuntu 24.04 in a
container:

```sh
devenv shell -- release   # build/linux/release/release.sh → bin/release
```

This needs podman or docker (it falls back to `nix run nixpkgs#podman`),
builds the .deb, .rpm, Arch package and AppImage, and smoke-tests each on a
clean Ubuntu, Fedora and Arch container.

### macOS (Apple Silicon, development)

Install Nix (the Determinate or the upstream installer) and devenv 2.4 or
newer, then:

```sh
git clone https://github.com/coredev-uk/sona-go.git && cd sona-go
devenv shell                          # the first run downloads the toolchain
# inside the shell:
pnpm -C frontend install --frozen-lockfile
SONA_PRIVATE=1 dev                    # the app with Vite hot reload
# or without hot reload:
build DEV=true && SONA_PRIVATE=1 ./bin/sona
```

The build is unsigned and runs from the checkout. Its data lives under
`~/Library/Application Support/Sona` and `~/Library/Caches/Sona`.
[CONTRIBUTING.md](CONTRIBUTING.md#macos) covers what works on macOS and how
to check the CDM and the sandbox.

### Optional build settings

`SONA_LASTFM_KEY` and `SONA_LASTFM_SECRET` (a Last.fm API account) and
`SONA_DISCORD_CLIENT_ID` (a Discord application ID, for the opt-in Discord
status) are compiled into production builds when set; development builds
(`DEV=true`, as `dev` uses) and builds without them read the same variables
at runtime. Without them, those integrations are off.

## Where Sona keeps things

Linux: settings `~/.config/sona/settings.json`, library database, artwork
and the CDM under `~/.local/share/sona` and `~/.cache/sona`, log
`~/.local/state/sona/sona.log` (XDG variables are honoured). macOS: see
above. Settings → Diagnostics shows the version, CDM and GStreamer details.

## Documentation

- [docs/architecture.md](docs/architecture.md): how Sona is put together.
- [CONTRIBUTING.md](CONTRIBUTING.md): environment, checks, pull requests and
  the rules every change follows.
- [SECURITY.md](SECURITY.md): reporting a vulnerability.
- [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).

## Licence

Sona is licensed under the GNU Affero General Public License v3.0 only
([LICENSE](LICENSE)). Third-party code in this repository and its licences
are listed in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).

The Widevine CDM is Google's proprietary software. It is not part of Sona,
not in this repository, not in the binary and not in any Sona package. Sona
downloads it from Google's servers onto your computer, verifies it, and never
modifies or redistributes it.
