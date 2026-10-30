# Sona

Sona is an Apple Music client for the Linux desktop. It is a native GTK4 /
WebKitGTK 6 app (Go + [Wails v3](https://v3.wails.io), React frontend) that
starts from a local cache, plays full tracks through the Widevine CDM of your
own Google Chrome install, and integrates with the desktop: MPRIS media
controls, `music://` links, Secret Service credentials, PipeWire/PulseAudio
output.

You need an Apple Music subscription. Sona is not affiliated with Apple.

## Requirements

- Linux (x86_64) with a Wayland or X11 session.
- GTK 4.14+, WebKitGTK 6.0, GStreamer 1.x with the base, good and libav
  plugin sets (AAC decoding), and PipeWire or PulseAudio. The packages below
  declare these; the AppImage bundles all but the audio server.
- **A Widevine CDM, from your own browser install.** Sona loads the
  `libwidevinecdm.so` that Google Chrome installs (`/opt/google/chrome`) or
  that Chrome/Chromium/Brave/Vivaldi/Edge download into your profile
  (`~/.config/<browser>/WidevineCdm`). Sona never bundles, downloads or
  modifies the CDM. Without one, browsing works but playback does not.
  To use a CDM elsewhere, set Settings → Playback → CDM path, or
  `SONA_WIDEVINE_CDM=/path/to/libwidevinecdm.so`.
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

Then install Google Chrome (or Chromium) once so a CDM is present, start Sona
from your launcher, and sign in with your Apple Account.

## Build from source

The toolchain comes from [devenv](https://devenv.sh) (Nix): Go 1.26, Node 24 +
pnpm, the GTK4/WebKitGTK/GStreamer development files, and Google Chrome as the
development CDM source. You need [Nix](https://nixos.org/download) with flakes;
devenv itself runs through `nix run`.

```sh
git clone https://github.com/coredev-uk/sona-go.git
cd sona-go
nix run nixpkgs#devenv -- shell -- go tool wails3 build   # frontend + bindings + bin/sona
nix run nixpkgs#devenv -- shell -- bin/sona --version
nix run nixpkgs#devenv -- shell -- bin/sona
```

`devenv shell` (or `nix run nixpkgs#devenv -- shell`) gives you an
interactive shell with everything on `PATH`; inside it, `build`, `dev`
(Vite hot reload), `test`, `check` and `fmt` are shortcuts. The first run
downloads the toolchain and Go/pnpm dependencies.

If Nix fails to fetch devenv's inputs with an ssh host-key error, your git
config rewrites https URLs to ssh; run devenv once as
`XDG_CONFIG_HOME=$(mktemp -d) nix run nixpkgs#devenv -- shell -- true` to lock
them, then drop the prefix (it would move Sona's settings into
that directory).

A binary built in devenv links against `/nix/store` and runs on the build
machine only. Distributable packages are built on Ubuntu 24.04 in a container:

```sh
nix run nixpkgs#devenv -- shell -- package   # build/linux/release/release.sh → bin/release
```

This needs podman or docker (it falls back to `nix run nixpkgs#podman`), builds
the .deb, .rpm, Arch package and AppImage, and smoke-tests each on a clean
Ubuntu, Fedora and Arch container.

Optional build-time settings: `SONA_LASTFM_KEY` and `SONA_LASTFM_SECRET` (a
Last.fm API account) are compiled in when set; without them Sona reads the
same variables at runtime.

## Where Sona keeps things

Settings `~/.config/sona/settings.json`, library database and artwork under
`~/.local/share/sona` and `~/.cache/sona`, log `~/.local/state/sona/sona.log`
(XDG variables are honoured). Settings → Diagnostics shows the version, CDM
and GStreamer details.

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
not in this repository and not in any Sona package; Sona uses the copy your
browser installed, under that browser's terms.
