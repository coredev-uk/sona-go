# Third-party notices

Sona is licensed under the AGPL-3.0 ([LICENSE](LICENSE)). This repository
also contains code from other projects, under their own licences:

| Code                                                                                                                            | From                                                           | Licence                                                                   |
| ------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------- | ------------------------------------------------------------------------- |
| `internal/playback/widevine/content_decryption_module.h`, `content_decryption_module_export.h` (the CDM interface, unmodified) | Chromium, © The Chromium Authors                               | BSD-3-Clause, [internal/playback/widevine/LICENSE.chromium](internal/playback/widevine/LICENSE.chromium) |
| `internal/ui/theme/icons/*.svg` (the UI's icons, unmodified)                                                                      | [Lucide](https://lucide.dev) 1.66.0, © Lucide Contributors     | ISC (and MIT for Feather's), [internal/ui/theme/icons/LICENSE](internal/ui/theme/icons/LICENSE) |
| `internal/ui/theme/fonts/*.ttf` (the interface font, unmodified)                                                                 | [Schibsted Grotesk](https://github.com/google/fonts/tree/main/ofl/schibstedgrotesk), © 2023 The Schibsted-Grotesk Project Authors | SIL OFL 1.1, [internal/ui/theme/fonts/OFL.txt](internal/ui/theme/fonts/OFL.txt) |

The Linux release packages install `LICENSE` and `LICENSE.chromium` under
`/usr/share/doc/sona/`; the Windows ones carry them in `licenses\`. Go modules Sona depends on, MyGo (the native UI, MIT) among them, keep their
licences in their own sources (`go.mod`). At run time Sona loads GTK3 and
GStreamer (LGPL) from the system or, in the macOS app and the Windows
packages, from the bundled GStreamer (below).

The Widevine CDM is not third-party code in Sona: it is Google's proprietary
software, never in this repository, the binary or any package. Sona downloads
it from Google at runtime ([README](README.md#playback-and-the-widevine-cdm)).

## Lucide (ISC)

- Version: 1.66.0 (`lucide-static` npm package)
- Source: https://github.com/lucide-icons/lucide
- Used for: the UI's icons
- Embedded in: `internal/ui/theme/icons/*.svg`
- Licence text (ISC, plus the Feather MIT notice): [internal/ui/theme/icons/LICENSE](internal/ui/theme/icons/LICENSE)

## Schibsted Grotesk (SIL OFL 1.1)

- Version: google/fonts commit `8b0a1d0f5983c89bc2b93f1b5fb55f9e252744b5` (variable TTF, upright and italic)
- Source: https://github.com/google/fonts/tree/main/ofl/schibstedgrotesk
- Copyright 2023 The Schibsted-Grotesk Project Authors
- Used for: the UI's typeface
- Embedded in: `internal/ui/theme/fonts/SchibstedGrotesk-Variable.ttf`, `SchibstedGrotesk-Italic-Variable.ttf`
- Licence text: [internal/ui/theme/fonts/OFL.txt](internal/ui/theme/fonts/OFL.txt)

## GStreamer (LGPL-2.0-or-later), bundled in the Windows packages

- Version: 1.28.8, the official Windows MinGW x86_64 package
  (`gstreamer-1.0-mingw-x86_64-1.28.8.exe`, SHA-256 pinned in
  [build/windows/gstreamer.sha256](build/windows/gstreamer.sha256)), from
  https://gstreamer.freedesktop.org/data/pkg/windows/1.28.8/mingw/
- Source: https://gstreamer.freedesktop.org/src/ (the same release's tarballs,
  and the build recipes of its packages: https://gitlab.freedesktop.org/gstreamer/gstreamer)
- Used for: audio decoding and output
- Unmodified. `build/windows/package.sh` copies from the installation only the
  DLLs `sona.exe`, the plugins and the plugin scanner import (GStreamer's core
  libraries and their dependencies: GLib, libintl, libffi, PCRE2, zlib, ORC,
  the MinGW runtime's `libwinpthread` and `libgcc`, FFmpeg's libraries for the
  `libav` plugin and the like) and the plugins Sona uses (`coreelements`,
  `typefindfunctions`, `playback`, `app`, `audioconvert`, `audioresample`,
  `volume`, `equalizer`, `audioparsers`, `isomp4`, `libav`, `wasapi2`, and the
  optional `wasapi`, `directsound`, `autodetect`, `audiomixer`, `audiorate`,
  `gio`, `rawparse`, `tcp`, `id3demux`, `apetag`, `wavparse`, `aiff`). Everything
  is installed as separate DLLs beside `sona.exe` (`lib\gstreamer-1.0\`,
  `libexec\gstreamer-1.0\`): they are not linked statically, and a build of
  the same GStreamer minor can replace them.
- Sona's own code calls GStreamer through its public C API (cgo); the CDM
  host child and the app load it as a shared library.
- Licence texts: the packages carry them in `licenses\` (Sona's own,
  Chromium's, this file, and in `licenses\GStreamer\<package>\` the texts the
  official installation ships for the packages the bundled DLLs come from:
  GStreamer's own (LGPL 2.0 or later), GLib, FFmpeg (the libraries behind the
  `libav` plugin: LGPL 2.1 or later), the MinGW-w64 runtime and the small
  libraries). Each bundled library keeps its own licence; the sources are in
  the release above.
- The macOS app bundles GStreamer's framework of the same 1.28 minor in the
  same way (`build/darwin`, `build/darwin/gstreamer.sha256`).
