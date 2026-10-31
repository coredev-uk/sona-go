# Sona

Sona is an Apple Music client for the Linux desktop. It is a native GTK4 /
WebKitGTK 6 app (Go + [Wails v3](https://v3.wails.io), React frontend) that
starts from a local cache, plays full tracks through Google's Widevine CDM
(downloaded by Sona from Google), and integrates with the desktop: MPRIS media
controls, `music://` links, Secret Service credentials, PipeWire/PulseAudio
output.

You need an Apple Music subscription. Sona is not affiliated with Apple.

## Requirements

- Linux (x86_64) with a Wayland or X11 session.
- GTK 4.14+, WebKitGTK 6.0, GStreamer 1.x with the base, good, bad (WebKit's
  media players) and libav (AAC decoding) plugin sets, and PipeWire or
  PulseAudio. The packages below declare these; the AppImage bundles all but
  the audio server.
- **Google's Widevine CDM, downloaded by Sona.** After the window first
  appears, Sona downloads the CDM from Google's servers (version and SHA-512
  from Mozilla's Firefox manifest, Google's signatures verified) into its
  data directory and keeps it up to date. It is never bundled in Sona's
  packages, and browser copies are not used. Until it is ready, browsing
  works but playback waits; Settings → Playback shows the download.
- Optional: a Secret Service provider (GNOME Keyring, KWallet) for the Apple
  Music token. Without one Sona falls back to a 0600 file in
  `~/.config/sona/` and says so.

## Install

From the [releases page](https://github.com/coredev-uk/sona-go/releases)
(each release lists SHA256SUMS):

| Distribution                    | File                                  | Install                                                  |
| ------------------------------- | ------------------------------------- | -------------------------------------------------------- |
| Ubuntu 24.04+ and Debian family | `sona_<version>_amd64.deb`            | `sudo apt install ./sona_<version>_amd64.deb`            |
| Fedora (tested on 45)           | `sona-<version>-1.x86_64.rpm`         | `sudo dnf install ./sona-<version>-1.x86_64.rpm`         |
| Arch Linux                      | `sona-<version>-1-x86_64.pkg.tar.zst` | `sudo pacman -U ./sona-<version>-1-x86_64.pkg.tar.zst`   |
| Anything else                   | `sona-<version>-x86_64.AppImage`      | `chmod +x` and run; AppImageUpdate can update it in place |

Each release is smoke-tested on clean Ubuntu 24.04, Fedora 45 and Arch Linux
containers. Release assets also carry the AUR `sona-bin` PKGBUILD.

Then start Sona from your launcher and sign in with your Apple Account; it
downloads the Widevine CDM by itself.

## Build from source

The toolchain comes from [devenv](https://devenv.sh) 2.4 or newer (Nix): Go
1.26, Node 24 + pnpm and the GTK4/WebKitGTK/GStreamer development files.
A development build downloads the CDM as a release does. You need [Nix](https://nixos.org/download) with flakes.
Without a devenv install, write
`nix run github:NixOS/nixpkgs/nixpkgs-unstable#devenv --` where the commands
below say `devenv` (your registry's `nixpkgs#devenv` may be older than 2.4,
which the project refuses).

```sh
git clone https://github.com/coredev-uk/sona-go.git
cd sona-go
devenv shell -- go tool wails3 build   # frontend + bindings + bin/sona
devenv shell -- bin/sona --version
devenv shell -- bin/sona
```

`devenv shell` gives you an interactive shell with everything on `PATH`;
inside it, `build`, `dev` (Vite hot reload), `tests`, `check` and `fmt` are
shortcuts (CONTRIBUTING.md lists them all). The first run downloads the
toolchain and Go/pnpm dependencies.

If Nix fails to fetch devenv's inputs with an ssh host-key error, your git
config rewrites https URLs to ssh; run devenv once as
`XDG_CONFIG_HOME=$(mktemp -d) devenv shell -- true` to lock
them, then drop the prefix (it would move Sona's settings into
that directory).

A binary built in devenv links against `/nix/store` and runs on the build
machine only. Distributable packages are built on Ubuntu 24.04 in a container:

```sh
devenv shell -- release   # build/linux/release/release.sh → bin/release
```

This needs podman or docker (it falls back to `nix run nixpkgs#podman`), builds
the .deb, .rpm, Arch package and AppImage, and smoke-tests each on a clean
Ubuntu, Fedora and Arch container.

Optional build-time settings: `SONA_LASTFM_KEY` and `SONA_LASTFM_SECRET` (a
Last.fm API account) and `SONA_DISCORD_CLIENT_ID` (a Discord application ID,
for the opt-in Discord status) are compiled into production builds when set;
development builds (`DEV=true`, as `dev` uses) and builds without them read
the same variables at runtime.

## Where Sona keeps things

Settings `~/.config/sona/settings.json`, library database and artwork under
`~/.local/share/sona` and `~/.cache/sona`, log `~/.local/state/sona/sona.log`
(XDG variables are honoured). Settings → Diagnostics shows the version, CDM
and GStreamer details.

## Private listening

Settings → Playback → Private listening (also the player bar, the tray, the
command palette and `sona private on|off|toggle|status`) keeps plays out of
your Apple Music listening history and recommendations (they are reported to
Apple marked private, as Apple's web player does). While it is on, Sona also
sends no Last.fm or ListenBrainz scrobbles, shows no Discord status and adds
nothing to its History page. `sona private …` acts on the running Sona and
prints the state; with no Sona running it changes nothing.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## Licence

Sona is licensed under the GNU Affero General Public License v3.0 only
([LICENSE](LICENSE)).

`internal/playback/widevine/content_decryption_module*.h` are vendored from
Chromium's CDM interface and are under Chromium's BSD-3-Clause licence
([internal/playback/widevine/LICENSE.chromium](internal/playback/widevine/LICENSE.chromium));
the packages ship both notices in `/usr/share/doc/sona/`.

The Widevine CDM is Google's proprietary software. It is not part of Sona,
not in this repository, not in the binary and not in any Sona package. Sona
downloads it from Google's servers onto your computer, verifies Google's
signatures, and never modifies or redistributes it.
