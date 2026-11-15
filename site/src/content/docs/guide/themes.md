---
title: Themes
description: Change Sona's colours with a built-in theme, an accent, or a theme file of your own.
---

A theme sets Sona's colours. Twelve come with Sona, and you can add your own as a JSON file.

## Choose a theme

1. Open Settings → Appearance.
2. Pick a theme in the gallery.

The light, dark or system mode setting still decides which variant a theme shows. A theme with one variant stays in that look, and Settings → Appearance tells you so.

## Built-in themes

| Id | Name | Looks | Spot colour |
| --- | --- | --- | --- |
| `sona` | Sona | Sona's own board, light and dark (the default) | amber |
| `graphite` | Graphite | A cooler neutral board, light and dark | sky blue |
| `paper` | Paper | A warm neutral board, grey and not cream, light and dark | teal |
| `contrast` | High contrast | Strong lines, every text colour at 7:1, light and dark | blue (light), yellow (dark) |
| `oled` | OLED | True black `#000000` ground, dark only | blue |
| `catppuccin` | Catppuccin | Latte (light) and Mocha (dark) | peach |
| `nord` | Nord | Snow Storm (light) and Polar Night (dark) | frost |
| `gruvbox` | Gruvbox | Light and dark | yellow |
| `solarized` | Solarized | Light and dark | yellow |
| `dracula` | Dracula | Dark only | pink |
| `rose-pine` | Rosé Pine | Dawn (light) and the main palette (dark) | gold |
| `tokyo-night` | Tokyo Night | Day (light) and Night (dark) | orange |

Sona's five own themes use a neutral board, so album covers show true colour. The others map each project's palette onto Sona's colours and credit the project in `author`. Where an exact value fails the [contrast rules](/reference/theme-format/#the-rules), one colour is adjusted in lightness only.

## Change the accent

Sona, Graphite, Paper, High contrast and OLED have an Accent row in Settings → Appearance: amber, gold, lime, sky, teal or rose. "Theme default" keeps the theme's own colour. The accent marks what is playing. Sona doesn't offer an accent that fails the contrast rules in either variant.

## Add your own theme

1. Open Settings → Appearance → "Open themes folder". Sona creates the folder if it is missing.
2. Copy a theme from [`internal/ui/theme/themes/`](https://github.com/coredev-uk/sona-go/tree/main/internal/ui/theme/themes) into it and give it a new `id`.
3. Change the colours, starting with the board, then the accent colour (`amber`).
4. Save. The theme appears in the gallery, and every later save shows at once, with no restart.

You write only the colours that change; Sona supplies the rest. The [theme file format](/reference/theme-format/) lists every colour and rule.

If a theme breaks a rule, Sona doesn't load it. It stays in the gallery as a disabled card that lists what failed, and a theme that worked before keeps its last good version. Fix the file and save.

The themes folder is:

| System | Folder |
| --- | --- |
| Linux | `~/.config/sona/themes` (or `$XDG_CONFIG_HOME/sona/themes`) |
| macOS | `~/Library/Application Support/Sona/themes` |
| Windows | `%AppData%\Sona\themes` |

Sona reads at most 100 files of up to 256 KiB each. Your settings export includes the chosen theme but not the files, so copy those yourself.

## Tips

- Keep `hover` and `active` a step apart. Text must stay readable on both.
- Check the accent colour in the light and dark variants. A colour that works on a dark board often fails the 3:1 floor on a light one. For light themes, make `amber-ink` darker than `amber`; in dark themes they are usually equal.
- Sona stores the theme's `id` in your settings. If the file is deleted or refused when Sona starts, Sona shows its own theme and says so in Settings. The choice comes back with the file.
