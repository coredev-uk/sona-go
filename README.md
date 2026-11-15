# Sona

An Apple Music client for the desktop, built first for Linux. Sona is a native
Go app whose interface is drawn on the GPU by [MyGo](https://github.com/egoist/mygo),
with no web view.

<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="site/src/assets/home-dark.webp">
    <img src="site/src/assets/home-light.webp" alt="Sona's Albums page: a sidebar, a grid of album covers and the player bar." width="720">
  </picture>
</p>
<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="site/src/assets/miniplayer-dark.webp">
    <img src="site/src/assets/miniplayer-light.webp" alt="Sona's mini player." width="360">
  </picture>
</p>

Sona is in early development, not a finished product. You need an Apple Music
subscription. Sona is not affiliated with Apple or Google.

## Install

Linux (x86_64), macOS (Apple silicon) and Windows (x64) builds are on the
[releases page](https://github.com/coredev-uk/sona-go/releases). The
[install guide](https://sona.coredev.uk/guide/install/) has the steps for each
system, including Nix.

## Features

- Your Apple Music library, with synced songs, albums, artists, playlists and history
- Word-by-word lyrics, with translations
- A queue with AutoPlay, crossfade, an equaliser and a sleep timer
- A mini player, media keys and Now Playing, tray and notifications
- Last.fm and ListenBrainz scrobbling, and Discord status
- Private listening
- Twelve themes, or write your own
- A command line for scripts and status bars

## Playback and the Widevine CDM

Apple Music's tracks are protected with Widevine. On first start Sona
downloads Google's Widevine CDM from Google's servers, verifies it, and runs
it in a sandboxed process. The CDM is Google's software: it isn't part of
this repository or any Sona build, and must never be bundled or modified.

## Links

- [Documentation](https://sona.coredev.uk)
- [Contributing](CONTRIBUTING.md)
- [Security policy](SECURITY.md)
- [Third-party notices](THIRD_PARTY_NOTICES.md)

Licensed under the GNU Affero General Public License v3.0 only
([LICENSE](LICENSE)).
