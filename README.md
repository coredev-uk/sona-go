# Sona

An Apple Music client for the desktop, built first for Linux: a native
Go app whose interface is drawn by [MyGo](https://github.com/egoist/mygo).

Sona is in early development, not a finished product. You need an Apple Music
subscription. Sona is not affiliated with Apple or Google.

| Platform             | Status                                                                  |
| -------------------- | ----------------------------------------------------------------------- |
| Linux, x86_64        | Plays; media controls, notifications, tray; .deb, .rpm, Arch, AppImage  |
| macOS, Apple Silicon | Plays; media keys and Now Playing; unsigned `.dmg`; updates itself      |
| Windows, x64         | Plays (WASAPI); media keys and Now Playing; unsigned installer and `.zip`; updates itself |

## Install

The native UI described below ships with the first native release (0.3); the
latest published release, 0.2.0, is the Linux-only web-based app. macOS and
Windows builds start with the native release.

Releases are on the [releases page](https://github.com/coredev-uk/sona-go/releases);
every file is listed in `SHA256SUMS`. Release notes give the first-run steps for
the unsigned macOS and Windows builds.

- **Linux:** the `.deb`, `.rpm`, Arch package or AppImage. The packages depend
  on GTK3, GStreamer and `libayatana-appindicator3` (the tray). Linux
  packages update through your package manager, the AppImage through
  AppImageUpdate.
- **Nix (Linux):** the repository is a flake. Follow its `nix` branch:
  `nix run github:coredev-uk/sona-go/nix`, `nix profile add
  github:coredev-uk/sona-go/nix`, or `inputs.sona.url =
  "github:coredev-uk/sona-go/nix"` with `overlays.default` (`pkgs.sona`) or
  `packages.<system>.default` in a NixOS or Home Manager configuration.
  CI moves that branch to every published release and nightly build by
  itself, so updating the flake input (`nix flake update sona`) is the whole
  update. On x86_64 `sona` is the latest release's Arch package, which has the
  Last.fm and Discord credentials, and `sona-nightly` the newest nightly rc of
  main (or the release, when that is newer). `sona-source` builds the checkout
  instead, without them (no Last.fm scrobbling or Discord presence), as does
  every package on aarch64, which has no release build, and `sona` on `main`,
  which has no pins. Outside NixOS the GPU driver needs
  [nixGL](https://github.com/nix-community/nixGL).
- **macOS 14 or later, Apple silicon:** open the `.dmg` and drag Sona to
  Applications. Settings → Updates installs new versions in place
  ("Install and restart").
- **Windows:** run the per-user installer or unpack the portable `.zip`.
  Updates install in place from Settings → Updates. The uninstaller removes
  Sona's playback sandbox too (your data stays); before deleting a portable
  folder, quit Sona and run `sona.exe --remove-sandbox` from it
  (`docs/cli.md`).

## Build and run

You need [Nix](https://nixos.org/download) and [devenv](https://devenv.sh)
2.4 or newer; devenv provides the rest of the toolchain.

```sh
git clone https://github.com/coredev-uk/sona-go.git && cd sona-go
devenv shell -- build    # builds bin/sona
devenv shell -- dev      # builds the development build and runs it
```

Development builds start in private listening, so plays aren't reported to
Apple or scrobbled.

## What the native UI changes

Sona's interface is Go, drawn on the GPU by MyGo over GTK3, AppKit or Win32,
with no web view. Compared with the web-based 0.2 releases:

- **Linux uses GTK3, not GTK4.** The window's decorations and window menu come
  from MyGo's GTK3 shell. On Wayland some compositors ignore window position
  and always-on-top, which the mini player uses.
- **Scrollbars float over the content** instead of taking a gutter.
- **Text is shaped by Pango, Core Text and DirectWrite**, not by WebKit, so
  hinting looks closer to each platform's own apps.
- **No web inspector.** Developer builds use MyGo's inspector and frame
  statistics; Settings → Diagnostics no longer has the WebKit switches
  (DMA-BUF, compositing).
- **Lighter and faster:** scrolling grids, motion artwork, memory use and
  start-up all cost less. Every lyric word sweeps, and line fades run at the
  display's rate.
- **New:** queue rows drop onto playlists. On macOS and Windows, media keys and
  Now Playing work.
- **The tray** needs `libayatana-appindicator3` on Linux; the packages depend
  on it.
- **Releases on every platform**, each with its own update path (see Install).

## Playback and the Widevine CDM

Apple Music's tracks are protected with Widevine. On first start Sona
downloads Google's Widevine CDM from Google's servers, verifies it, and runs
it in a sandboxed process. The CDM is Google's software: it isn't part of
this repository or any Sona build, and must never be bundled or modified.

## More

- [CONTRIBUTING.md](CONTRIBUTING.md): environment, checks and pull requests
- [docs/cli.md](docs/cli.md): `sona play`, `sona status --follow` and the
  other commands for the running Sona (scripts, Waybar, Polybar)
- [docs/architecture.md](docs/architecture.md): how Sona is put together
- [SECURITY.md](SECURITY.md): reporting a vulnerability

Licensed under the GNU Affero General Public License v3.0 only
([LICENSE](LICENSE)); third-party notices in
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
