# Third-party notices

Sona is licensed under the AGPL-3.0 ([LICENSE](LICENSE)). This repository
also contains code from other projects, under their own licences:

| Code                                                                                                                            | From                                                           | Licence                                                                   |
| ------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------- | ------------------------------------------------------------------------- |
| `internal/playback/widevine/content_decryption_module.h`, `content_decryption_module_export.h` (the CDM interface, unmodified) | Chromium, © The Chromium Authors                               | BSD-3-Clause, [internal/playback/widevine/LICENSE.chromium](internal/playback/widevine/LICENSE.chromium) |
| `frontend/src/shared/ui/*` (components generated with the shadcn/ui CLI, then changed)                                          | [shadcn/ui](https://github.com/shadcn-ui/ui), © 2023 shadcn    | MIT (below)                                                               |
| `internal/ui/theme/icons/*.svg` (the UI's icons, unmodified)                                                                      | [Lucide](https://lucide.dev) 1.66.0, © Lucide Contributors     | ISC (and MIT for Feather's), [internal/ui/theme/icons/LICENSE](internal/ui/theme/icons/LICENSE) |
| `internal/ui/theme/fonts/*.ttf` (the interface font, unmodified)                                                                 | [Schibsted Grotesk](https://github.com/google/fonts/tree/main/ofl/schibstedgrotesk), © 2023 The Schibsted-Grotesk Project Authors | SIL OFL 1.1, [internal/ui/theme/fonts/OFL.txt](internal/ui/theme/fonts/OFL.txt) |

The release packages install `LICENSE` and `LICENSE.chromium` under
`/usr/share/doc/sona/`. Go modules and npm packages Sona depends on keep their
licences in their own sources (`go.mod`, `frontend/package.json`).

The Widevine CDM is not third-party code in Sona: it is Google's proprietary
software, never in this repository, the binary or any package. Sona downloads
it from Google at runtime ([README](README.md#playback-and-the-widevine-cdm)).

## shadcn/ui (MIT)

```text
MIT License

Copyright (c) 2023 shadcn

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

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
