# Third-party notices

Sona is licensed under the AGPL-3.0 ([LICENSE](LICENSE)). This repository
also contains code from other projects, under their own licences:

| Code                                                                                                                            | From                                                           | Licence                                                                   |
| ------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------- | ------------------------------------------------------------------------- |
| `internal/playback/widevine/content_decryption_module.h`, `content_decryption_module_export.h` (the CDM interface, unmodified) | Chromium, © The Chromium Authors                               | BSD-3-Clause, [internal/playback/widevine/LICENSE.chromium](internal/playback/widevine/LICENSE.chromium) |
| `frontend/src/shared/ui/*` (components generated with the shadcn/ui CLI, then changed)                                          | [shadcn/ui](https://github.com/shadcn-ui/ui), © 2023 shadcn    | MIT (below)                                                               |

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
