# Third-party notices

Sona is licensed under the AGPL-3.0 ([LICENSE](LICENSE)). This repository
also contains code from other projects, under their own licences:

| Code                                                                                                                            | From                                                           | Licence                                                                   |
| ------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------- | ------------------------------------------------------------------------- |
| `internal/playback/widevine/content_decryption_module.h`, `content_decryption_module_export.h` (the CDM interface, unmodified) | Chromium, © The Chromium Authors                               | BSD-3-Clause, [internal/playback/widevine/LICENSE.chromium](internal/playback/widevine/LICENSE.chromium) |
| `internal/ui/theme/icons/*.svg` (the UI's icons, unmodified)                                                                      | [Lucide](https://lucide.dev) 1.66.0, © Lucide Contributors     | ISC (and MIT for Feather's), [internal/ui/theme/icons/LICENSE](internal/ui/theme/icons/LICENSE) |
| `internal/ui/theme/fonts/*.ttf` (the interface font, unmodified)                                                                 | [Schibsted Grotesk](https://github.com/google/fonts/tree/main/ofl/schibstedgrotesk), © 2023 The Schibsted-Grotesk Project Authors | SIL OFL 1.1, [internal/ui/theme/fonts/OFL.txt](internal/ui/theme/fonts/OFL.txt) |

The release packages install `LICENSE` and `LICENSE.chromium` under
`/usr/share/doc/sona/`. Go modules Sona depends on, MyGo (the native UI, MIT) among them, keep their
licences in their own sources (`go.mod`). At run time Sona loads GTK3 and
GStreamer (LGPL) from the system or, in the macOS app, from the bundled
GStreamer framework.

The Widevine CDM is not third-party code in Sona: it is Google's proprietary
software, never in this repository, the binary or any package. Sona downloads
it from Google at runtime ([README](README.md#playback-and-the-widevine-cdm)).

## Lucide (ISC)

- Version: 1.66.0 (`lucide-static` npm package)
- Source: https://github.com/lucide-icons/lucide
- Used for: the UI's icons
- Embedded in: `internal/ui/theme/icons/*.svg`
- Licence text (ISC, plus the Feather MIT notice): [internal/ui/theme/icons/LICENSE](internal/ui/theme/icons/LICENSE)

## Go Bold font (BSD-3-Clause)

- Source: `golang.org/x/image/font/gofont/gobold`, © The Go Authors (Bigelow & Holmes' Go fonts)
- Used for: the playlist's name on the generated "Classic" cover (`internal/artwork/classic.go`)
- Embedded in: the binary, through the Go module; its licence is the module's own

## Schibsted Grotesk (SIL OFL 1.1)

- Version: google/fonts commit `8b0a1d0f5983c89bc2b93f1b5fb55f9e252744b5` (variable TTF, upright and italic)
- Source: https://github.com/google/fonts/tree/main/ofl/schibstedgrotesk
- Copyright 2023 The Schibsted-Grotesk Project Authors
- Used for: the UI's typeface
- Embedded in: `internal/ui/theme/fonts/SchibstedGrotesk-Variable.ttf`, `SchibstedGrotesk-Italic-Variable.ttf`
- Licence text: [internal/ui/theme/fonts/OFL.txt](internal/ui/theme/fonts/OFL.txt)
