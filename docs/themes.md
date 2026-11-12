# Themes

A theme is a JSON file that changes Sona's colours. Sona ships twelve (Settings → Appearance), and you can add your own: put a `.json` file in the themes folder and it appears in the gallery, and an edit shows as you save it, with no restart.

The folder is `themes` inside Sona's config folder: `~/.config/sona/themes` on Linux (or `$XDG_CONFIG_HOME/sona/themes`), `~/Library/Application Support/Sona/themes` on macOS, `%AppData%\Sona\themes` on Windows. Settings → Appearance → "Open themes folder" opens it, creating it when it isn't there. Sona reads at most 100 files, each up to 256 KiB. The chosen theme is part of your settings export; the theme files are not, so copy them yourself.

## Built-in themes

| Id | Name | Looks | Spot colour |
| --- | --- | --- | --- |
| `sona` | Sona | Sona's own board, light and dark (the default) | amber |
| `graphite` | Graphite | A cooler neutral board, light and dark | sky blue |
| `paper` | Paper | A warm neutral board, grey and not cream, light and dark | teal |
| `contrast` | High contrast | Strong lines, every text colour at 7:1, light and dark | blue (light), yellow (dark) |
| `oled` | OLED | True black `#000000` ground, dark only | lime |
| `catppuccin` | Catppuccin | Latte (light) and Mocha (dark) | peach |
| `nord` | Nord | Snow Storm (light) and Polar Night (dark) | frost |
| `gruvbox` | Gruvbox | Light and dark | yellow |
| `solarized` | Solarized | Light and dark | yellow |
| `dracula` | Dracula | Dark only | pink |
| `rose-pine` | Rosé Pine | Dawn (light) and the main palette (dark) | gold |
| `tokyo-night` | Tokyo Night | Day (light) and Night (dark) | orange |

Sona's own five keep to the design brief: a neutral board, so that covers read true, and no cream, navy or violet. The community presets are the projects' palettes mapped onto Sona's tokens, so their own hues win, and each credits its project in `author`. Where an exact palette value fails the rules below, that one token is moved in lightness only; the file's `$comment` lists what moved. They are good starting points: copy one from the Sona source (`internal/ui/theme/themes/`) into your folder with a new `id`.

## The file

```json
{
  "format": 1,
  "id": "harbour",
  "name": "Harbour",
  "author": "Ada",
  "light": {
    "color": {
      "ground": "#f1f4f4",
      "surface": "#e9eeee",
      "raised": "#ffffff",
      "hover": "#e0e7e7",
      "active": "#d6dfdf",
      "line": "#d3dcdc",
      "control-line": "#7f8888",
      "ink": "#141a1a",
      "ink-muted": "#4f5a5a",
      "ink-faint": "#566161",
      "amber": "#d9602b",
      "amber-ink": "#9a3b0c",
      "on-amber": "#1b0a02"
    },
    "scalar": {
      "backdrop-scrim": 0.9
    },
    "shadow": {
      "float": { "x": 0, "y": 10, "blur": 28, "spread": 0, "color": "#000000/14" }
    }
  },
  "dark": {
    "color": {
      "ground": "#101515",
      "surface": "#151b1b",
      "raised": "#1c2323",
      "hover": "#212929",
      "active": "#293232",
      "line": "#252e2e",
      "control-line": "#6c7878",
      "ink": "#ecf1f1",
      "ink-muted": "#a8b3b3",
      "ink-faint": "#9fabab",
      "amber": "#f08a52",
      "amber-ink": "#f08a52",
      "on-amber": "#1b0a02"
    }
  }
}
```

- `format` is `1`. `id` is 1 to 40 of `a-z`, `0-9` and `-`, and unique: a built-in's or an earlier file's id (files are read in name order) is refused. `name` is 1 to 40 characters; `author` is optional. A field Sona doesn't know is an error, except `$comment` (anywhere), which Sona ignores.
- Give `light`, `dark` or both. Each holds `color`, `scalar` and `shadow`, all optional. **You write only what changes:** a token you leave out is Sona's own value for that appearance, so nothing is ever missing. A theme with one variant is that look only: Sona stays light (or dark) with it whatever the mode setting says, and Settings → Appearance says so. The variant you leave out is Sona's own, for the one place it shows (the pictures).
- **`color`** values are `"#rrggbb"`, `"#rrggbbaa"`, another colour token's name, or `"name/N"` (that token at N% opacity): `"ring": "ink/60"`. A token that refers to another follows it: change `ink` and `primary`, `foreground` and `ring` follow. A loop or an unknown name is an error.
- **`scalar`** holds `backdrop-scrim` (0.8 to 1: how opaque the wash over an album's backdrop is).
- **`shadow`** holds `sm` and `float`, each `{"x","y","blur","spread","color"}` (DIPs; offsets within ±128, blur 0 to 128, colour as above).
- Type, radius, spacing, metrics, motion and the font are not part of a theme.

## Colour tokens

| Group | Tokens |
| --- | --- |
| The board | `ground` (the page), `surface` (sidebar, top bar, player bar), `raised` (menus, dialogs, inputs), `hover`, `active` (selected nav item, pressed), `line` (1px structure) |
| Text | `ink`, `ink-muted`, `ink-faint`, `danger`, `positive`, `white` |
| Controls | `control-line` (borders of inputs and outline buttons), `ring` (the focus ring), `selection`, `caret`, `edge` (the 1px line round artwork), `scrollbar`, `scrollbar-hover`, `scrim` (behind dialogs), `dialog-border`, `input-bg`, `invalid-ring`, `select-hover`, `badge-destructive`, `switch-off`, `switch-thumb`, `switch-thumb-on`, `tab-active-bg`, `tab-active-border`, `link-tile-border`, `danger-line`, `danger-wash`, `accent-soft` |
| The spot colour | `amber` (the fill), `amber-ink` (as text or an icon), `on-amber` (a glyph on the fill) and the roles `play`, `play-ink`, `on-play`, which follow them |
| Avatars | `avatar-0-bg` … `avatar-7-bg` and `avatar-0-fg` … `avatar-7-fg` |
| Roles | `background`, `foreground`, `card`, `card-foreground`, `popover`, `popover-foreground`, `primary`, `primary-foreground`, `secondary`, `secondary-foreground`, `muted`, `muted-foreground`, `accent`, `accent-foreground`, `destructive`, `border`, `input` (each names one of the tokens above, and follows when you change it) |

## The rules

Sona checks a theme when it loads a file. A theme that fails any rule is **refused, not loaded**: it stays in the gallery as a disabled card that lists what failed, and while you edit a theme that was good, the last good version stays in use. Fix the file and save.

- **Text.** `ink`, `ink-muted`, `ink-faint`, `positive`, `danger` and `amber-ink` keep 4.5:1 on `ground`, `surface`, `raised`, `hover`, `active` and on a selected row (`ink` at 12% over `ground`).
- **Lines.** `control-line` and `ring` keep 3:1 on `ground`, `surface` and `raised`.
- **Avatars.** Each `avatar-N-fg` keeps 4.5:1 on its `avatar-N-bg`.
- **Backdrop.** `backdrop-scrim` is 0.8 or more.
- **The spot colour** marks what plays, and nothing else. It stays distinct: `amber` has at least 2:1 against `ground`, and 1.75:1 against `surface` and `hover`, in a light variant, and 3:1 against all three in a dark one (Sona's own amber has 2.0, 1.88 and 1.76 in light); `on-amber` keeps 4.5:1 on `amber`; `ink` keeps 4.5:1 on the selection (`selection`, the spot colour at 32%). `play`, `play-ink` and `on-play` must stay the same colours as `amber`, `amber-ink` and `on-amber`. And no other role may be the spot colour: `primary`, `primary-foreground`, `ring`, `active`, `hover`, `accent`, `secondary` and `muted` are refused if they resolve to `amber` or `amber-ink`, because the focus ring, the selected navigation row and the buttons are not what plays.
- Colours must resolve, token names must exist, and shadows must be in range.

Contrast is the WCAG ratio between the two colours; a translucent colour is first laid over the tone it sits on. The album page still takes its accent from the cover, which replaces your spot colour there.

## Tips

- Start from a copy of a built-in and change the board first. Keep `hover` and `active` a step apart; they are the pressed and selected tones text must still read on.
- Check the spot colour in both variants: a colour that looks fine on a dark ground often fails the 2:1 floor on a light one. Pick a darker `amber-ink` than the fill for light themes; in dark themes they are usually the same colour.
- The theme's `id` is what Sona keeps in your settings. If the file is deleted or refused when Sona starts, Sona shows its own theme and says so in Settings; the choice comes back with the file.
