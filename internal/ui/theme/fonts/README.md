# Fonts

Schibsted Grotesk, variable (wght axis 400..900), SIL Open Font License 1.1
([OFL.txt](OFL.txt), copyright 2023 The Schibsted-Grotesk Project Authors).

Source: https://github.com/google/fonts/tree/main/ofl/schibstedgrotesk
at commit `8b0a1d0f5983c89bc2b93f1b5fb55f9e252744b5` (2026-03-12), fetched 2026-11-03.

| File                                    | Upstream file                      | Bytes  | sha256                                                             |
| --------------------------------------- | ---------------------------------- | ------ | ------------------------------------------------------------------ |
| `SchibstedGrotesk-Variable.ttf`         | `SchibstedGrotesk[wght].ttf`        | 176068 | `6ceeadf6be8e1fd7687011c7fa38ed0edd1abe967a0b73d97caec183552e823d` |
| `SchibstedGrotesk-Italic-Variable.ttf`  | `SchibstedGrotesk-Italic[wght].ttf` | 181440 | `b49fedb6f3a2ff9b43e13351888641505dc8e5f300941e597eecbc3f52ba357b` |
| `OFL.txt`                               | `OFL.txt`                          | 4406   | `3b4f3063b6ac7c1e403e2c4a5e8ef3a58190ff83ed7b15af66511858699139ce` |

Declared family name (name table ID 1) in both files: `Schibsted Grotesk`
(ID 2: `Regular` / `Italic`). ID 16 is not present. Register with
`ui.RegisterFont(data, "Schibsted Grotesk")`; the italic file is for lyrics.

## Refresh

```sh
B=https://raw.githubusercontent.com/google/fonts/<commit>/ofl/schibstedgrotesk
curl -fo SchibstedGrotesk-Variable.ttf "$B/SchibstedGrotesk%5Bwght%5D.ttf"
curl -fo SchibstedGrotesk-Italic-Variable.ttf "$B/SchibstedGrotesk-Italic%5Bwght%5D.ttf"
curl -fo OFL.txt "$B/OFL.txt"
sha256sum *
```

Then update the commit, date and hashes above.
