---
title: Theme File Format
description: The JSON format of a Sona theme file, every colour token, and the rules a theme must pass.
---

> A theme is one JSON file that overrides Sona's colours, scalars and shadows.

To install and choose themes, see [Themes](/guide/themes/).

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
      "amber": "#d65d28",
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

Each token is a `color` value: a colour, or the name of another token.

### The board

- `ground` - the page.
- `surface` - the sidebar, top bar and player bar.
- `raised` - menus, dialogs and inputs.
- `hover` - hovered rows.
- `active` - the selected navigation item and pressed controls.
- `line` - 1px structure.

### Text

- `ink`, `ink-muted`, `ink-faint` - text, from strongest to weakest.
- `danger`, `positive`, `white`

### Controls

- `control-line` - borders of inputs and outline buttons.
- `ring` - the focus ring.
- `edge` - the 1px line round artwork.
- `scrim` - behind dialogs.
- `selection`, `caret`, `scrollbar`, `scrollbar-hover`, `dialog-border`, `input-bg`, `invalid-ring`, `select-hover`, `badge-destructive`, `switch-off`, `switch-thumb`, `switch-thumb-on`, `tab-active-bg`, `tab-active-border`, `link-tile-border`, `danger-line`, `danger-wash`, `accent-soft`

### The spot colour

- `amber` - the fill.
- `amber-ink` - the colour as text or an icon.
- `on-amber` - a glyph on the fill.
- `play`, `play-ink`, `on-play` - roles that follow the three above.

### Avatars

- `avatar-0-bg` to `avatar-7-bg` - the background of each avatar colour.
- `avatar-0-fg` to `avatar-7-fg` - its text.

### Roles

Each role names one of the tokens above and follows when you change it.

- `background`, `foreground`, `card`, `card-foreground`, `popover`, `popover-foreground`
- `primary`, `primary-foreground`, `secondary`, `secondary-foreground`
- `muted`, `muted-foreground`, `accent`, `accent-foreground`
- `destructive`, `border`, `input`

## Accents

Only Sona's own theme files carry `accents`: a map from each accent (`amber`, `gold`, `lime`, `sky`, `teal`, `rose`) to the `amber`, `amber-ink` and `on-amber` of each variant. Settings → Appearance offers them as the Accent row; "Theme default" is the theme's own colour. An accent that fails the rules below in any variant isn't offered. A file of yours with `accents` is refused, because your theme sets its own `amber`.

## The rules

Sona checks a theme when it loads the file. A theme that breaks any rule is refused: it appears in the gallery as a disabled card that lists what failed, and a theme you were editing keeps its last good version. Contrast is the WCAG ratio; Sona first lays a translucent colour over the tone it sits on.

| Rule | Requirement |
| --- | --- |
| Text | `ink`, `ink-muted`, `ink-faint`, `positive`, `danger` and `amber-ink` reach 4.5:1 on `ground`, `surface`, `raised`, `hover`, `active` and a selected row (`ink` at 12% over `ground`) |
| Lines | `control-line` and `ring` reach 3:1 on `ground`, `surface` and `raised` |
| Avatars | Each `avatar-N-fg` reaches 4.5:1 on its `avatar-N-bg` |
| Backdrop | `backdrop-scrim` is 0.8 or more |
| Spot colour fill | `amber` reaches 3:1 on `ground`, `surface` and `hover`, in each variant |
| Spot colour text | `on-amber` reaches 4.5:1 on `amber`, and `ink` reaches 4.5:1 on `selection` |
| Play roles | `play`, `play-ink` and `on-play` stay the same colours as `amber`, `amber-ink` and `on-amber` |
| Spot colour only for what plays | `primary`, `primary-foreground`, `ring`, `active`, `hover`, `accent`, `secondary` and `muted` must not resolve to `amber` or `amber-ink`. The focus ring, the selected navigation row and the buttons are not what plays |
| Valid values | Colours resolve, token names exist, and shadows are in range |

The album page takes its accent from the cover, which replaces your spot colour there.
