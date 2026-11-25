# Sona design direction

This is the visual direction for Sona's interface: type, colour, shape, motion and how each page is laid out. Read it before you change how anything looks. The tokens below live in `internal/ui/theme/tokens.json`; change them there and here together.

## Contents

- Foundations
  - [Thesis: the label's house style](#thesis-the-labels-house-style)
  - [Principles](#principles)
  - [Type](#type)
  - [Colour](#colour) (with [Themes](#themes))
  - [Space and density](#space-and-density)
  - [Shape and depth](#shape-and-depth)
  - [Icons](#icons)
  - [Artwork](#artwork)
  - [Backdrops (pre-blurred in Go)](#backdrops-pre-blurred-in-go)
  - [Window material (macOS, Windows 11)](#window-material-macos-windows-11)
  - [Motion](#motion)
  - [Reduce effects](#reduce-effects)
  - [Lists](#lists)
  - [Choosing and playing: the interaction table](#choosing-and-playing-the-interaction-table)
  - [Dates](#dates)
  - [Empty, loading and error states](#empty-loading-and-error-states)
  - [Selectable text](#selectable-text)
- Pages and surfaces
  - [Playlist page](#playlist-page)
  - [The song page: a liner card](#the-song-page-a-liner-card)
  - [People: pictures and initials](#people-pictures-and-initials)
  - [Home](#home)
  - [Shell](#shell) (sidebar, top bar, library pages, player bar, title bar, first-run setup)
  - [Discover](#discover)
  - [Charts](#charts)
  - [Search](#search)
  - [Album page](#album-page)
  - [Artist page](#artist-page)
  - [Library Artists](#library-artists)
  - [Now Playing](#now-playing)
  - [Lyrics](#lyrics)
  - [Radio](#radio)
  - [Link lists](#link-lists)
  - [Concerts](#concerts)
  - [New releases](#new-releases)
  - [Video view](#video-view)
  - [Settings](#settings)
  - [Command palette and shortcut sheet](#command-palette-and-shortcut-sheet)

## Thesis: the label's house style

A good record label sets every release through one fixed template: the same typeface at the same station, the same catalogue line, a quiet board, and the cover as the only picture. Sona works the same way. The chrome is a neutral grey board set in one grotesque. The colour on screen comes from the music: cover art, and backdrops pre-blurred from it in Go. The interface itself owns one spot colour, amber, and spends it on one job only: showing where the music is.

What it refuses: the streaming-app default of near-black with a brand-coloured accent on every control, and frosted glass over blurred artwork.

## Principles

1. **Fast before pretty.** Nothing visual may cost first paint or scroll smoothness (`docs/architecture.md` §8): the interface draws on the GPU, and layout, text shaping and image work stay out of every frame.
2. **The cover is the colour.** Chrome stays neutral so artwork reads true.
3. **The spot colour means "playing" (amber in Sona's own theme).** The now-playing row, the play control, the progress fill, the active queue item. Navigation selection, focus and primary buttons do not use it. Another theme may pick another spot colour, and it means the same thing (Themes, below). On an album page the cover's own accent does the spot colour's job for that record (Album page, below).
4. **Native to the desktop.** Light and dark follow the system; it works with the keyboard; it is dense enough to sit tiled at half width beside an editor.
5. **Honest states.** Signed out, syncing, offline, no CDM: each is said plainly where the missing data would be.

## Type

One family: **Schibsted Grotesk** (variable TTF, SIL OFL 1.1, embedded in `internal/ui/theme/fonts` and registered by `theme.RegisterFonts` before the first window opens, so there is no network fetch). It is a news grotesque, so it stays sharp at 12–14px in dense lists and has character at title sizes.

| Role             | Size / line       | Weight  | Use                                        |
| ---------------- | ----------------- | ------- | ------------------------------------------ |
| Collection title | 32 / 36, -0.02em  | 700     | Album, artist, playlist name in its header |
| Page title (h1)  | 24 / 30, -0.015em | 650     | For You, Songs, Settings                   |
| Section          | 16 / 22           | 600     | Shelf titles, settings section titles      |
| Body             | 14 / 20           | 400–500 | Nav, row titles, form labels               |
| Secondary        | 13 / 18           | 400     | Artist and album columns, descriptions     |
| Meta             | 12 / 16           | 450     | The catalogue line, durations, timestamps  |

- Six sizes, three weights in practice (400, 600, 700). No uppercase labels, no letter-spaced small caps, no monospace.
- **Wording and case.** Titles are in **Title Case** in Apple's style, and everything else is in sentence case.
  - **Title Case** for page titles (h1), section and shelf headings, sidebar group and section names, nav items, dialog and sheet titles, tab names, settings page and group names, column headers, and the window and tray titles ("Made for You", "Top Picks for You", "Recently Added", "Songs in Your Library", "Keyboard Shortcuts", "Sona Mini Player").
  - **Sentence case** for buttons and menu items ("Add to library", "Delete from library"), body text, descriptions, hints, toasts, error messages, empty-state titles, the catalogue line and other meta lines ("Couldn't load your recommendations", "On this computer and your other devices").
  - **Style:** a, an, the, and, but, or, for, of, on, in, to, by and at stay lower case unless first or last; every other word has a capital ("From", "With", "Your"). A name or a word that already has capitals (EPs, DJ, ListenBrainz) keeps them.
  - **Apple's names** (shows, stations, artists, albums, playlists, curators) are always shown exactly as Apple sends them, never re-cased.
  - `go test ./cmd/i18ntool` (`TestUITitlesAreTitleCase`) fails on a title that is not in Title Case. It knows titles by where they are put (`titleSinks` and `titleFields` in `cmd/i18ntool/titles.go`): add a new heading helper there.
- Every number that updates or aligns (durations, track numbers, counts, progress) uses tabular figures (`FontFeatures("tnum")`).
- **The catalogue line**: every collection header and the now-playing bar carry one meta line in the same form: `Album · 2008 · 10 songs, 42 min`. Middle dots, sentence case (a meta line, not a title), tabular figures. It is the label's catalogue number and appears in exactly this form everywhere.

## Colour

Strategy: restrained chrome, colour from artwork, one spot colour with one job. All text tiers reach 4.5:1 on the surface they sit on. Token names are written with a `--` prefix on this page; in code they are `theme.C("ink")` and the like, from `tokens.json`.

| Token            | Light     | Dark      | Role                                                 |
| ---------------- | --------- | --------- | ---------------------------------------------------- |
| `--ground`       | `#f5f5f5` | `#141414` | Content canvas                                       |
| `--surface`      | `#ededed` | `#191919` | Sidebar, top bar, now-playing bar                    |
| `--raised`       | `#ffffff` | `#212121` | Menus, dialogs, popovers, inputs                     |
| `--hover`        | `#e6e6e6` | `#262626` | Row and nav hover                                    |
| `--active`       | `#dedede` | `#2e2e2e` | Selected nav item, pressed                           |
| `--line`         | `#dbdbdb` | `#2a2a2a` | Structural 1px lines                                 |
| `--control-line` | `#878787` | `#707070` | Input and outline-button borders (≥ 3:1)             |
| `--ink`          | `#181818` | `#f0f0f0` | Primary text                                         |
| `--ink-muted`    | `#585858` | `#aeaeae` | Secondary text                                       |
| `--ink-faint`    | `#5e5e5e` | `#a5a5a5` | Meta, placeholders                                   |
| `--amber`        | `#ad7700` | `#f0b43c` | Spot fill: play button, progress, now-playing marker |
| `--amber-ink`    | `#845500` | `#f0b43c` | Amber as text or icon on the ground                  |
| `--on-amber`     | `#1a1405` | `#1a1405` | Glyphs on an amber fill                              |
| `--danger`       | `#b42318` | `#f0705f` | Errors and destructive actions only                  |
| `--positive`     | `#1c6943` | `#5cc48c` | Success states only                                  |

- Sona's own board is neutral grey (OKLCH chroma 0), so no tint competes with a cover. No cream, no navy, no violet: that is Sona's own themes' rule, not the community palettes' (Themes, below).
- The primary button is ink-filled (near-black on light, near-white on dark), not amber.
- Focus ring: 2px `--ink` at 60% (the `ring` token, MyGo's keyboard-focus ring) outside the control, everywhere (≥ 3:1 on every background tone). Selection is amber at 15% (the `selection` token); the caret is `--amber-ink`.
- Contrast floors (WCAG AA, `site/src/content/docs/contributing/accessibility.md`): text tokens ≥ 4.5:1 on every background up to `--active` (selected rows), `--control-line` and the focus ring ≥ 3:1. Slider thumbs are always shown: the thumb, not the fill colour, marks the value.
- The kit's role names (`background`, `primary`, `muted`, `ring`, …) are tokens in `tokens.json` too, mapped onto these, so MyGo's widgets and Sona's components read one palette without per-component overrides.

### Themes

The table above is Sona's own theme (`sona`): `tokens.json` is its whole document, and every other theme is a set of overrides of it (`internal/ui/theme/themes/*.json`, and the user's own files, `site/src/content/docs/reference/theme-format.md`). A token a theme leaves out comes from Sona's own, so a theme never shows a missing colour. Every light/dark difference is a token (`scrim`, `input-bg`, `switch-off`, `select-hover`, `badge-destructive`, `link-tile-border`, `dialog-border`…); the kit has no `theme.Dark()` branch for a colour.

| Theme | Looks | Spot colour |
| --- | --- | --- |
| `sona` | The default: the table above, light and dark. | amber |
| `graphite` | A cooler neutral board, light and dark. | sky blue |
| `paper` | A warm neutral board, grey and not cream, light and dark. | teal |
| `contrast` | White or black board, strong lines and focus ring, every text colour at 7:1, light and dark. | blue (light), yellow (dark) |
| `oled` | Dark only. A `#000000` ground, with surface and raised just above it to separate menus. | blue |
| `catppuccin`, `nord`, `gruvbox`, `solarized`, `rose-pine`, `tokyo-night` | The community palettes, each with a light and a dark variant (Latte/Mocha, Snow Storm/Polar Night, light/dark, light/dark, Dawn/main, Day/Night). | peach, frost, yellow, yellow, gold, orange |
| `dracula` | The community palette, dark only. | pink |

- **Sona's own themes keep the brief.** The chrome is a neutral board (no tone of it strays more than a few levels from grey; a test holds the five to it), the cover is the colour, and there is no cream, navy or violet. The community presets are their projects' palettes mapped onto Sona's tokens: their own hues win, violet and navy included, and each credits its project in `author`. Where an exact palette value fails the floors below (text tiers, the control line, the spot floors), that token is moved in lightness only, and the theme file's `$comment` lists each change.
- **An accent picks the spot colour for Sona's own themes.** Settings → Appearance has an Accent row under the gallery for `sona`, `graphite`, `paper`, `contrast` and `oled`.
  - The choices are "Theme default" and up to six named hues (Amber, Gold, Lime, Sky, Teal, Rose). Each is a designed light and dark value set stored in the theme file, offered only when that theme and accent pass `theme.Validate`.
  - The row is a radio group of swatches with their names and a check on the chosen one (colour is never the only sign). It is kept per theme (`accents`), so switching theme brings back its accent; the summary reads "OLED · Lime · Dark".
  - It changes `amber`, `amber-ink` and `on-amber` and nothing else. OLED's own colour is a clear blue.
- **A theme may change the spot colour** (`amber`, `amber-ink`, `on-amber`; `play`, `play-ink` and `on-play` follow). It still means "playing" and nothing else.
  - A theme whose `primary`, `ring` (the focus ring), `active` or `hover` (navigation selection and hover) or any other role of the kit resolves to the spot colour is refused.
  - The cover's accent on the album page replaces it there. `palette` reads its six colours (ground, hover, active, ink, spot, on-spot) from the current theme.
- **A theme that fails is refused, not adjusted.** `theme.Validate` is the one rule set, for the built-ins' tests and for a user's file at run time. The floors:
  - Text tiers, the spot colour as text and the status colours keep 4.5:1 on every tone up to `--active` and on a selected row (ink at 12%).
  - `--control-line` and the ring keep 3:1. Avatar tones keep 4.5:1. The backdrop scrim stays at 0.8 or more.
  - The spot fill keeps 3:1 against the ground, surface and hover in a light and a dark theme (WCAG 1.4.11, non-text contrast: the play disc is a graphical object; Sona's own light amber has 3.6, 3.3 and 3.1). `--on-amber` keeps 4.5:1 on it.
  - Ink and `--ink-muted` keep 4.5:1 on the selection (the spot colour at 15%) over the ground, surface and raised. 15% is the most that holds in every built-in theme and every accent: at the old 32% Sona dark's `--ink-muted` was 3.5 to 4.0:1 and 16 of the 22 built-in variants fell short. The selection stays distinct from hover and active, which are neutral greys.
- **A theme with one variant pins the appearance** to it: `oled` and `dracula` are dark, so Settings → Appearance disables the mode row and says so.
  - Settings → Appearance lists the themes as cards: a miniature window drawn from each theme's own tokens, its name, a line, and "Dark only" where it applies.
  - The cards are in two groups, "Sona's own" and "Community palettes", then the user's files.

## Space and density

| Token | Value | Use |
| --- | --- | --- |
| `space` | 4px base (`1` = 4, `2` = 8, `3` = 12 … `16` = 64) | Every gap and padding |
| `control` | 32px | Top-bar search, buttons |
| `control-sm` | 28px | Compact icon buttons |
| `sidebar` | 232px | Sidebar width |
| `top-bar` | 48px | Top bar height |
| `player-bar` | 72px | Now-playing bar, full width |
| `gutter` | 32px (`gutter-narrow` 24px under 960px wide) | Content gutter |
| `nav-row` / `nav-pitch` | 32px on a 34px pitch | Nav rows |
| `nav-icon` | 16px | Outline icons at 1.5 stroke (SF Symbols on macOS for the ones "Icons" names), 10px to the label |
| `track-row` / `track-art` | 44px / 32px | Track rows and their artwork |
| `tile-min` | 160px | Album and playlist cover tiles |
| `tile-gap-x` / `tile-gap-y` | 24px / 28px | Gaps between tiles |
| `window-min-width` / `window-min-height` | 940 × 600 | Minimum window; the layout holds at half of a 1920 screen |

- Track rows have no zebra striping and no row dividers: hover is a `--hover` fill at radius 6.
- Tiles carry the title at 14/500 and the artist at 13 muted under the cover.

## Shape and depth

| Radius token | Value | Use |
| --- | --- | --- |
| `sm` | 4px | Artwork |
| `md` | 6px | Controls, hover fills |
| `lg` | 8px | Menus, dialogs, panels |
| `full` | circle | Avatars and the round play button; nothing else is pill-shaped |

- Surfaces separate by a tone step, or by one 1px `--line`, never both.
- Shadows only on what floats (menus, dialogs, toasts): the `float` shadow, 8px down with a 24px blur, black at 12% in light and 40% in dark.
- No gradients, no glows, no glass. Two exceptions: the album page's band, which ends in a dissolve and a far-end gradient (Album page), and on macOS and Windows 11 the OS's window material behind the sidebar, never drawn by Sona (Window material, below).

## Icons

- Lucide outline icons (`theme.Icon`), at 16px in controls, 14px in dense rows and 12px beside meta text, in the colour of the text they sit with. Solid forms only for what is on or what plays: every play triangle (`kit.PlayGlyph`: on the amber disc, over a cover's wash, in a track's number cell, on a station row; a bare outlined triangle would read as a different button), a favourite star, a set thumb, a running sleep timer.
- **macOS** draws the transport and the common controls as SF Symbols, so the Mac's controls read as the Mac's: play and pause, previous and next (`backward.fill`, `forward.fill`), shuffle, repeat and repeat one, the speaker (`speaker.wave.1.fill`, `speaker.wave.2.fill`, `speaker.slash.fill`), the queue (`music.note.list`), search (`magnifyingglass`), close and clear (`xmark`), more (`ellipsis`), add, delete (`trash`), favourite (`star`, `star.fill` when on) and the four chevrons. The map is `theme`'s (`symbols.go`); every other icon, and Sona's own glyphs (`hat-glasses` for private listening, `disc-3`, the brand mark), stays Lucide there too.
- A symbol takes the box the Lucide icon had (16, 14 or 12) and is drawn at its natural scale, so a row of them balances as macOS balances them (a cross stays smaller than a speaker), over 1.2 times the box so that its ink is as large as the Lucide icons' beside it, at the Regular weight, which is the 1.5 stroke of the Lucide icons beside them. The transport and the speaker are solid, as macOS draws them; the rest are outlines like Lucide's.
- One colour per symbol, the token the icon had: no multicolour or hierarchical symbols, which would bring Apple's palette into Sona's. A symbol does not ease its colour with its button's face (it changes on the frame the face starts to), the one difference from Lucide.
- Linux and Windows keep Lucide everywhere, and the golden images are Linux's, so they show Lucide. The Play triangle is the exception to "the system's shape": `play.fill`'s mass sits left of the Play disc's middle by more than Lucide's does, so `theme.symbolTune` moves it right (4.4% of the glyph) and draws it 5.5% smaller, which puts its box and mass where Lucide's lie, and `TestPlayButtonsTriangleIsOpticallyCentred` pins that on every system. The sidebar shuffle's glyph reading stays Linux's: a symbol's shape is the system's (`docs/testing.md`, "Golden images").

## Artwork

- Square, 4px radius, a 1px line at 6% ink drawn just inside the edge (`ArtOpts.Edge`) so pale covers do not dissolve into the ground. Video frames are 16:9; hero cards are Apple's 1060:608; bricks are 2:1 and page banners keep their own 2.4:1 or 4:1 shape.
- Always requested from Go at the exact rendered box (width and height × the window's scale; grid tiles at their exact width, never rounded up, so the art is never resampled while painting) through `/art/<key>/<w>x<h>.jpg` paths, resolved in one place (`kit.ArtLoader`, `internal/ui/kit/artloader.go`) and decoded off the main thread, in parallel, covers on screen first. The box is fixed before the picture arrives, so rows never shift.
- **A play glyph over a cover** (a track row's cover, Home's rows, the sidebar's shuffle button) sits on a ground-coloured wash, `kit.ArtworkWash`, so it reads on any cover: 70% for a row's 40px cover, which still shows through, and 85% for a small cover (the sidebar's 20px), where the glyph covers most of the picture and a weaker wash lets the picture fight its strokes. Nothing else dims a cover.
- **A tile's overlays follow the cover's whole box.** A fluid tile's width is a fraction of a DIP at most window widths, so its cover's height is rounded to whole DIPs (`coverArtHeight`, as `ArtBox` rounds a cover's) and the play disc, the play mark and the release badge (8px from the cover's bottom-left corner) and a wide card's disc (12px) are placed from that height. The inset is then exact at 1x and 2x, whatever the window's width.
- Missing art: a `--hover` square with a 40%-ink outline music glyph centred, at the same size. People never get this glyph: see People, below.
- Playlists Apple rendered no cover for show a mosaic Go composes (`internal/artwork/mosaic.go`): the first four distinct album covers in track order as a 2×2 grid, edge to edge with no gutter, each tile the centred square of its cover, at the exact requested box like any cover. One distinct cover, or fewer than four, shows the first cover whole. Cropping each tile is what makes it a collage; the UI sees an ordinary `/art/` path, and the playlist's backdrop and colours come from the mosaic. A playlist with no track artwork keeps the missing-art square.
- **A playlist's cover is the user's to change** (*Change cover…* in the item menu of any library playlist, and *Cover* in the editor).
  - Apple's web API cannot store one (`docs/apple-music-reference.md` §5.8, "Playlist artwork"), so the choice is Sona's own, kept on this computer.
  - The picker says so before anything else: a notice at the top of the dialog (`kit.InfoLine`: an info icon and "Only in Sona: Apple Music can't keep covers set here." on the quiet `hover` wash, a `RoleStatus`, also the dialog's accessible description). It is not a line of grey small print under the title that nobody reads.
  - The notice stays up every time; no toast and no setting, because the dialog is where the choice is made.
  - The choices:
  - **Automatic** is Apple's cover: for a playlist with songs and no photo, the cover with its name Apple generates, in the design chosen in Apple's apps and four colours from the first song's artwork. The mosaic above shows only where Apple gave nothing.
  - **None** is the missing-art square, the same as a playlist with no artwork.
  - **Your image** is cropped to its centre square, scaled to at most 1024 px and stored as a JPEG. JPEG or PNG in, up to 20 MB and 40 megapixels, at least 300 px each way; the orientation of a photo is honoured and transparency goes onto white.
  - **Apple Music's own nine designs** of the generated cover, in Apple's order: Gradient, Disc, Split, Bars, Fold, Eclipse, Arcs, Dots and Haze (Sona's names; Apple shows them unnamed). Apple's CDN draws them, so they are Apple's look exactly, with the playlist's colours and its current name. They are offered only while Apple generates the playlist's cover (a playlist with songs and no photo of its own), and the picker says so when they are not there.
  - Every choice is an ordinary `/art/` path, so the sidebar, the library grid, search, pins and the page and its backdrop show it without knowing.
- **The picker** (`internal/ui/features/playlist/covers.go`): a dialog headed by its title and the notice above, then tiles in four columns (92 DIP covers with their names under them), a radio group, so one is chosen and a ring, a check and a heavier name say which (not colour alone). A choice applies at once, on every surface; there is no Save, only Done. Below the tiles: *Choose an image…* (the system file chooser; hidden with a sentence where there is none), *Remove my image* once one is kept, and the limits in view beside the button, not behind a failure. The grid scrolls in a short window.
- The cover is never cropped, tinted or overlaid with text, with one exception: an album not out yet carries its release date ("Out 19 Nov") in a small `--raised` chip at the cover's bottom-left corner, where the play control of a playable tile sits (it has none). Its page leads with the release ("Out in 18 days, 6 hours", the day in full), re-read every minute, never a ticking counter (Album page, Not out yet, below).
- Hero cards (the untitled "Featured" row) fill Apple's own card shape, 1060:608, with the 4:1 `subscriptionHero` art cropped to it by the CDN, as music.apple.com does: the art keeps its text and subject in the centre, so nothing is letterboxed or cut. A card without a banner shows its cover whole on its background colour. On Discover they are the front page's lead and list (Discover, below). Bricks (a category row such as Discover's "New for November") are cropped the same way, to a 2:1 frame: every banner fills it, the CDN cutting the centre of the art, so no card is a small picture between flat bands. Some art carries bands of its own (Best of Apple Music Club's 4:1 `subscriptionHero` is black either side of a 1.96:1 picture), which only a crop removes; music.apple.com crops bricks the same way (to 16:9). A square banner (a curator with only a square flavour) is cut to the frame too. No blur and no flat colour is involved, so there is no image work per frame.

## Backdrops (pre-blurred in Go)

The artwork service renders, per cover, a tiny pre-blurred JPEG (target ~32×32, heavy Gaussian blur, saturation kept). The UI never blurs anything itself.

- **Where**: behind the now-playing bar, the mini player, the now-playing view, full-screen lyrics, collection headers (playlist, artist), the album page's band when the cover's colour isn't known (Album page), and the top of Home (from its lead pick).
- **How**: `kit.Backdrop` (`internal/ui/kit/media_backdrop.go`) fills its box with the cover's dominant colour at once, then draws the tiny image over it at 110% (the GPU's bilinear upscaling of the tiny image is the softness). A solid scrim sits over it: the surface colour (bar, mini player) or the ground colour (views, headers) at `--backdrop-scrim`, 92% in light and 86% in dark. The result is a wash of the cover's colour, never a recognisable picture.
  - Text sits on the scrim, so contrast is the scrim's, not the image's. The scrim is the floor at which `--ink-muted` and `--ink-faint` stay ≥ 4.5:1 over any backdrop: a black one in light (the binding case: 91% on surface) and a white one in dark (85%).
  - It is one value per theme, never lowered per surface. A stronger cover colour would need a darker text token, not a thinner scrim.
- **Change**: when the track changes the new backdrop fades in over 300ms (the one authored motion moment). Images are decoded before the swap so the fade never shows an empty frame.
- **Banned**: any live blur (a blur effect at paint time) on anything larger than an icon, and any blur on scrolling content. The window material (below) is the system's, composited by the OS, not a blur Sona paints.
- **Reduce effects**: backdrops are not rendered at all; the surfaces are solid tokens. Full-Screen Lyrics then has no wash to drift.

## Window material (macOS, Windows 11)

On macOS and Windows 11 the sidebar lets the OS's window material through: on macOS Liquid Glass from macOS 26, the sidebar material on 14 and 15; on Windows Mica from Windows 11 22H2 (build 22621), the same backdrop as Settings' and File Explorer's navigation panes. The system draws it from the desktop (and on macOS the windows behind Sona); Sona draws nothing of it and nothing else of the window is translucent. Linux and older Windows have none.

- **The sidebar only.** The top bar, the page, the player bar, menus, dialogs and Now Playing stay solid tokens. The 1px `--line` still separates the sidebar from the page.
- **A tint keeps the text readable.** The sidebar lays its `--surface` over the material at the least opacity, in steps of 5%, at which `--ink`, `--ink-muted` and `--ink-faint` keep 4.5:1 over every measured sample of that material (`internal/ui/app/material.go`): over black, white, grey and saturated desktops, active and inactive, Liquid Glass and the sidebar material measured on Apple silicon for #317 (`glassSamples`), Mica on Windows 11 build 26200 (`micaSamples`). Sona's own theme gets 80% in light and 70% in dark over the Mac's materials, and none over Mica: Mica stays within a few steps of its light or dark grey whatever the wallpaper, so the sidebar is plain Mica, as Windows' own navigation panes are. A theme whose text needs more than 95% gets a solid sidebar and no material.
- **Light and dark as Sona is.** The app's appearance follows Sona's mode (or a one-variant theme), so the material, the window buttons and the system's menus are drawn light or dark as Sona's tokens are, whatever the desktop's mode.
- **Solid where asked.** Reduce effects and the desktop's Reduce Transparency (on Windows, Settings → Personalisation → Colours → Transparency effects) turn the material off and the sidebar is solid `--surface` again.
- **Inactive.** Windows draws Mica only in the active window; an inactive one shows its plain light or dark grey, which the samples include.
- **The mini player** has no material; on macOS its frameless window has rounded corners (20px), as a floating panel does.

## Motion

| Motion token | Duration | Use |
| --- | --- | --- |
| `press` | 100ms | Small fades and turns (chevrons, hover glyphs) |
| `feedback` | 120ms | Hover and press: background, colour |
| `route` | 150ms | Route content |
| `float` | 160ms | Menus and floating surfaces |
| `tooltip` | 400ms | A tooltip shows once the pointer has rested this long (MyGo's `Theme.TooltipDelay`); after one goes, the next shows at once |

- The durations live in `tokens.json`. The backdrop crossfade is 300ms (see Backdrops).
- Full-Screen Lyrics and the lyric sheet in Now Playing share two: `climb` (600ms, the `out` curve) is how each line glides to its place when the sung line changes, after a wait of 60ms for every line it trails the sung one by (4 lines at most), and `drift` (40s, `in-out`; full screen only) moves the wash a few percent and back. Both are poses (`AnimatePose`): transforms and fades that only paint, never layout.
- Easing is `cubic-bezier(0.16, 1, 0.3, 1)` (exponential out, the `out` curve). No bounce, no layout animation, no scroll-linked effects.
- A loop costs frames for as long as it runs: MyGo redraws the window on every frame something animates, however small the part that moves. So work that can run for long shows a still icon and progress that moves when the work does (the library sync: the bell's bar, and the row's bar and count in the notification centre). A spinner (`kit.ButtonOpts.Spin`) is for waits of a second or two.
- The desktop's reduced-motion preference, the Reduced motion setting and the reduce-effects mode all make the kit skip its transitions and animations (`theme.ReducedMotion()`).

## Reduce effects

A user setting (`Settings → Appearance`, persisted by Go as `reduceEffects`; the UI reads it as `theme.ReduceEffects()`). With it on:

- no backdrops, solid surfaces only (the album page's band is one flat colour that ends square, with no dissolve; on macOS the sidebar is solid and the window has no material);
- transitions and animations off;
- the same layout and colours otherwise, so the mode is never a degraded look.

It is the recommended setting on software rendering.

## Lists

- A list that can exceed ~200 rows (library songs, albums, artists, long playlists) builds only the rows in view and a few beyond (`kit.Pane`, or the list's own `ui.Scroll`). The content pane is the scroll element, so the shell never scrolls.
- Fixed row heights (44px, or the grid's computed row height) so virtualisation never measures.
- **A cover's play control fades in** on hover or keyboard focus, over the `press` duration (`kit.Fade`; at once under reduced motion): a track row's cover wash and a wide card's play disc, as a tile's disc does. It stays in the accessibility tree while hidden.
- **The playing track's row is marked in place** wherever it appears: its index is replaced by an amber equaliser glyph (`kit.EqualiserBars`) and its title takes `--amber-ink`. It keeps that mark while the list scrolls or re-sorts.
  - While a song plays the glyph's bars each rise and fall between a quarter of the height and all of it on a period of their own (0.8 to 1.1 s, none a multiple of another, so the three drift in and out of step), quick through the middle of a stroke and slow at the turns, with no randomness.
  - A bar's top edge moves by fractions of a pixel: the bar is squeezed from its foot, not resized, since a resized rectangle's edges snap to whole device pixels and step at 1x.
  - The glyph is painted at the display's rate by painting the glyph alone again, so a playing song costs no build or layout of the page around it.
  - The bars hold still, at 60, 100 and 40%, while paused, while the Now Playing view or full-screen lyrics cover the list, and under reduced motion.
- **An uploaded song is marked where it sits.** A song the user uploaded (in the library with no catalogue copy: `services.Song.IsUpload`) gets an **Uploaded** badge after its title, beside the explicit mark: `kit.UploadedBadge`, the house `Badge` in its outline variant with the upload glyph, 20px high, so it stays quiet beside the title and never competes with the playing row's amber.
  - It shows in every list of songs: the track list (album, playlist, artist, Songs, the other library pages), a track shelf's rows, search (results, the best match and the suggestions), and the queue (the playing row included). The row's accessible name says it after the title ("Demo, uploaded"), so a screen reader hears the same distinction; in the queue it follows the artist ("Demo, Mia Vale, uploaded").
  - It marks a song wherever it could be taken for a catalogue one, so a catalogue song and an upload of the same name are two rows that tell themselves apart. The one place it is left off (`TrackListOpts.HideUploadMark`) is a record whose songs are all uploads, where it would mark every row and tell none apart.
  - An upload has no catalogue page, so its menu has none of what needs one (no Copy link, View credits or Create station), and its row opens no song page: it plays in place.
- Queue rows have one menu, opened by right-click, Shift+F10 or the Menu key, or the ellipsis button that shares the hover and focus slot with the remove button (both replace the duration): Play, Play next (to the front of what plays next), Move to end (to the back of the user's songs, before AutoPlay's), then Remove. An item that would change nothing is left out (no Play next on the first song after the current one, no Move to end on the last of the user's), and the playing song has no menu. AutoPlay and station rows keep their place, so their menu is Play and Remove. A move is announced in a status region ("Airbag: moved to position 2 of 6") and a row with focus stays focused and in view.
- **A queue row's title and artist are links, as the bar's are**: the title opens the song in its album (a music video its page), the artist their page, and the playing row also links its album. They are pointer shortcuts, matched against the row's own click so that a row can still be dragged by its title or artist: the list stays one Tab stop, Enter and a double-click elsewhere on the row still play it, and a right-click on a link opens the row's menu. Without the page the text is plain; a song credited to several artists keeps its artist as plain text (the bar's menu of them is a button, which would take the press from the drag). "Playing from <station>" is a Tab stop.
- **The queue panel is a running order in three groups**, each under a 14/600 heading. Each group is a list of its own for assistive technology, so a row says its place in its group.
  - **Now playing**: the playing song as one 72px row on a `--hover` fill at radius 8 (cover 56px, the title at 14/600 in `--amber-ink`, the artist and the album under it, the equaliser at the end).
  - **Next**: the songs after it in play order, 52px rows numbered from 1 in `--ink-faint` tabular figures where the cover's index would be. The grip of a row that can be dragged takes the number's place while the pointer or the focus is on it. The heading's detail line says how many follow ("3 songs after this one", or the station it plays from, as a link).
  - **The station's or AutoPlay's part**: under its own heading, numbered on from Next.

### Several artists

A song or album with several artists keeps the one thing Apple writes, its artist string ("The Kid LAROI & Justin Bieber"), as the text, and the text decides nothing silently: the first artist is never taken for the rest, and a compilation under "Various Artists" is plain text, not a link to a page that does not exist.

- **One artist is a link** (underlined on hover). **Several are one control** that looks the same at rest and underlines on hover, and opens a small menu of the artists.
  - The menu is the kit's: `--raised`, radius 8, the `float` shadow, rows like the item menu's, no icons, the panel's start under the text's start, above it where there is no room below.
  - The row holding the pointer or the keys takes `--accent`; the keyboard's takes the ring as well.
  - No arrow is drawn on the text: a trailing chevron would either move the line when it appears or leave a gap in "Artist · Album".
- The same control is the artist in the player bar, Now Playing, the video view's caption, a track list's artist column and a track shelf's rows, the album, song and video headers and a station's songs. In a list it is not a Tab stop (the list is one), and Shift or Ctrl click on it selects the row like any other cell.
- The song menu's "Go to artist" is a sub-menu of the artists when there are several (a plain row for one).
- **"Add to playlist" is a sub-menu in every item menu** (a song's, an album's, a playlist's and a selection's), not a dialog that opens from it. Pointer hover or Right opens it beside the row; Left or Escape closes it. The pointer can cross the rows between on a diagonal toward an open sub-menu without closing it (it rests there for 300 ms or leaves the triangle to switch).
  - It lists *New playlist…* first (with a plus; it opens the new playlist form, and the playlist is made with the items in it), then a rule, then the playlists the user can edit: the ones Apple lists as recently used lead (at most five, as the picker's Recent), then the sidebar's order, folders before playlists. A recent playlist is not listed twice at the top level.
  - A folder is a row with a folder glyph and a chevron that opens a sub-menu of its own; a folder with nothing the user can edit is left out. Folders open two levels deep; a deeper one, and a level of more than fifteen rows, end with *More playlists…*, which opens the picker (search, groups by folder path) for the rest.
  - Choosing a playlist adds at once and says so with the toast and its *Open*; the rows are plain labels, with no cover or count, so the sub-menu stays as light as the menu it hangs from.
  - Until the playlists are read, and when there is none to list, the row is the plain one and opens the picker.

## Choosing and playing: the interaction table

One rule governs every list, shelf, row and tile that shows an item (a song, an album, a playlist, an artist, a station, a music video, a curator, an editorial card, a link):

- **Choosing an item opens it.** A click, or Enter on the focused item, goes to its page, a song's too. An item with no page of its own does what it is: a station with no page (a local broadcaster) plays, a web-only link opens the browser, and each says so in its cue.
- **Play plays.** A Play control (the disc on a cover, the triangle on a row, the amber disc beside a best match) starts the item, and does nothing else. Nothing else starts playback except a track row (below).
- **A track row plays.** A row of songs in a list (album, playlist, library, a shelf's rows, search's Songs) is the one place a double-click or Enter plays: it is a list of songs, not a list of pages. A track row: click selects, double-click and Enter play. Every list of songs follows it, the shelf's rows included (Shift or Ctrl+click and Ctrl+Space select more than one).
- **Space has one rule.** Space selects where the list selects (a track row; Ctrl+Space toggles), plays where the item is itself a Play control (a page-less station) or where the list's hint says "Space plays" (Home's strip and lead rows), and does nothing on a link (a tile, card or row that only opens). It never opens.
- **Choosing or playing from a result is remembered.** A tile's Play, a link tile, a station row and a track shelf's artist link call `OnOpen` as opening does, so search's "Recently searched" fills the same way however the item was chosen.
- **A modifier never does something the cue doesn't hint at.** Shift, Ctrl or Cmd with a click selects where the list selects and does exactly what the plain click does where it cannot, never both and never nothing; Shift+Enter plays only where the control shows it.

  | List | Selects on Shift/Ctrl/Cmd+click | A modified click on |
  |---|---|---|
  | Cover tiles with an Item (`CoverShelf`, `ItemGrid`, library and search tiles) | yes | the tile selects it |
  | Cover tiles with no Item (curator, room, web link) | no | the tile opens as a plain click does |
  | Track lists and track shelves, Home's rows | yes | a row, and its artist and album links, select the row |
  | Wide cards, link-list tiles, station rows | no | the card, tile or row opens or plays as a plain click does |
- **Artists and curators open only.** No player is registered for them, so no tile, card, row, best match or lead pick draws a Play control for them, and no surface draws a disabled one. Chosen over an artist player because "play an artist" has no single meaning (top songs? shuffle?) and the artist page already offers Play top songs, Shuffle and Play library songs. A Play control exists exactly where `ItemPlayers.CanPlay` is true; a withheld explicit item shows it disabled.
- **A page that is not routed is plain.** A tile keeps no underline; a wide card, link tile and the artist's latest-release slip are not built as a button at all (no tab stop, pointer, hover or navigation).
- **The cue says what the choice does.** A link (underline on hover, a chevron on the active row) opens; a play triangle or disc plays; an arrow out leaves Sona.
- **A More button and the menu agree.** Every surface that draws a "More for …" button opens the same menu on a right-click (on the row, or on a link in it, which passes the press through) and on Shift+F10 or the Menu key while its opener has the focus (the button is not a Tab stop). `TestInteraction…` pins it for every kit surface that has the button. Curators, rooms, groupings and link-list tiles have no registered action (nothing to add, play or pin), so they have no button and no menu: the two agree by both being absent, and a stub menu would only pretend.

The table is what the code does on `main`.

- It comes from the `TestInteraction…` tests: one or more per surface, in each package's `interaction_test.go`. They are `ui.Tester` tests that click, double-click and press the keys.
- A test that pins a departure from the rule has a `// DEPARTS:` comment at the assertion. When the departure is fixed, the test changes with it.
- Each departure is an issue, listed after the tables.
- Right-click menus are recorded by their presence only; their contents change in their own work.

Unless a cell says otherwise: Shift+Enter does what Enter does or nothing, Space follows its rule above (nothing on a link), a double-click is two clicks, and "menu" is opened by right-click, Shift+F10 or the Menu key.

### Tiles, cards, rows and lists that show items

| Surface (component) | Item | Click | Double-click | Enter | Shift+Enter | Space | Play | Menu | Cue |
|---|---|---|---|---|---|---|---|---|---|
| Cover tile: shelf, grid, library, search tiles (`kit` `CoverShelf`, `ItemGrid`) | song, album, playlist, video, station with a page | opens | opens (twice) | opens | none | none | disc, on hover and focus | yes | link, title underlines on hover |
|  | artist, curator, room, grouping | opens | opens | opens | none | none | none | artist yes, curator no | link |
|  | upcoming album | opens | opens | opens | none | none | none, release badge in its place | yes | link, "Out 19 Nov" |
|  | station with no page | plays | plays | plays | plays | plays | none: the tile is the control | yes | button "X Play" ("X Pause" while it plays and a press pauses), play (pause) mark on hover |
|  | web link | opens the browser | twice | same | none | none | none | no | link, arrow out, named "X Opens in your browser" |
|  | unrouted page | none, no underline | none | none | none | none | disc | yes | group |
|  | any with an Item, Shift/Ctrl+click | selects | | | | | | | |
| Wide editorial card (`WideRow`: Home heroes, Discover and editorial bricks) | any page kind | opens, modifier ignored (cannot select) | opens | opens | opens | none | disc, except artist and curator | More button and right-click, Shift+F10, Menu key | link |
|  | station with no page | plays, pauses while it plays | twice | same | same | same | none: the card is the control | More button and right-click, Shift+F10, Menu key | button "X Play" / "X Pause", play (pause) mark after the title on hover and focus |
|  | web link | opens the browser | twice | same | none | none | none | none | link, arrow out, named "X Opens in your browser" |
| Link list tile (`LinkList`: "More to Explore") | page link | opens, remembered (`OnOpen`) | opens | opens | opens | none | none: a link tile is a plain link, even for a station with a page (its cover tile, card and row have the Play control) | none | link |
|  | station with no page | plays, pauses while it plays | twice | same | same | same | none | none | button "X Play" / "X Pause", play (pause) mark after the title on hover and focus |
|  | web link | opens the browser | twice | same | same | none | none | none | link with the arrow out, named "X Opens in your browser" |
| Station list (`StationList`, Radio's "Live now") | station with a page | opens | opens | opens | opens | none | ghost play button; the equaliser replaces it while playing and is the pause and resume button, "Pause X" | More button and right-click, Shift+F10, Menu key (a station that cannot be played too: right-click on the row) | link |
|  | station with no page | plays, or toggles while playing | twice | same | same | same | none | More button and right-click, Shift+F10, Menu key (a station that cannot be played too: right-click on the row) | button "X Play" / "X Pause" |
| Track shelf rows (`TrackShelf`: Top songs on artist pages, Home's songs) | song | selects | plays from the row, the shelf is the queue | plays | none | selects | cover button | More button and right-click, Shift+F10, Menu key | list item, hover fill |
|  | artist link in the row | opens the artist | twice | n/a | n/a | n/a | n/a | n/a | underline on hover |
| Track list rows (`TrackList`: album, playlist, library, search Songs, charts) | song | selects | plays from the row | plays | none | selects | number-column button | yes, also on the artist and album links | row; link cells underline |
|  | artist or album cell | opens | twice | n/a | n/a | n/a | n/a | n/a | link |
|  | Shift/Ctrl+click | range / toggle | | | | | | | |

### Home, Discover and Radio

| Surface | Item | Click | Enter | Space | Play | Cue |
|---|---|---|---|---|---|---|
| Home shelves (tiles) | as the cover tile above | | | | | |
| Home strip rows (Recently played) and lead order | song, album, playlist, video, station with a page | opens | opens | plays | disc | link |
|  | artist | opens | opens | none | none | link |
|  | station with no page | plays, pauses while it plays | same | same | the name | button "X Play" / "X Pause", play (pause) mark over the cover on hover |
|  | web link | opens the browser | same | none | none | link, arrow out, named "X Opens in your browser" |
| Home lead pick, hero card | album, song, playlist, video, station | opens | opens | none | disc | link |
|  | artist, curator | opens | opens | none | none | link |
|  | station with no page | plays, pauses while it plays | same | same | the name, no disc | button "X Play" / "X Pause", play (pause) mark after the name on hover and focus |
| Home local shelves (Recently added, Your playlists) | album, playlist | opens | opens | none | disc, on hover and focus; More menu | link |
| Home "See all" | | opens the shelf's page | | | | link |
| Discover lead card, queue tiles | album, playlist, station, video | opens | opens | none | disc | link |
|  | artist, curator | opens | opens | none | none | link |
|  | station with no page | plays, pauses while it plays | same | same | the name, no disc | button "X Play" / "X Pause", play (pause) mark after the title on hover and focus |
| Discover "Show in lead" (on a queue tile) | any | makes it the lead, the keyboard's focus going with it | same | same | | button |
| Discover bands and room tiles | as the cover tile | | | | | |
| Discover Top songs, Charts | song | selects | plays from the row | none | number-column button | row; "Play Top songs" queues from the first, "See all" opens `/charts/songs` |
| Radio board lane | station with a page | opens | opens | none | disc | link |
|  | station with no page | plays | plays | plays | the name, play mark on hover | button "X Play" |
|  | tuned station with no page | toggles | toggles | toggles | equaliser | button "X Pause" |
|  | unplayable | none | not focusable | | none | disabled, reason in text |
| Radio programme box | | opens the station page | not a Tab stop | | | link |

### Library pages

| Surface | Item | Click | Double-click | Enter | Play | Menu |
|---|---|---|---|---|---|---|
| Songs, Favourites, History | song | selects | plays from the row | plays | number-column button | yes |
| Albums, Playlists, Recently added grids | album, playlist | opens | opens | opens | disc, on hover and focus | yes |
| Videos grid | music video | opens | opens | opens | disc where the platform plays videos | yes where the platform plays videos (none otherwise) |
| Artists master list | artist | chooses it: the detail beside it changes and `?artist=` follows | same | same, Enter or Shift+Enter moves the focus to the detail | none on the row; the detail has Play and Shuffle | n/a |
| Songs page head | | "Play your songs" plays from the first song, "Shuffle" shuffles | | | | |

### Search

| Surface | Item | Click | Enter | Shift+Enter | Play | Cue |
|---|---|---|---|---|---|---|
| Suggestion list: typed completion | term | searches | searches | searches | n/a | search icon, chevron when active |
| Suggestion list: result | song with a page, library song with a catalogue copy | opens `/song/…` | opens | opens | none | chevron |
|  | song with no page (a library upload) | plays | plays | plays | none | play triangle when active |
|  | album, artist, playlist, curator, video, station with a page | opens | opens | opens | none | chevron |
|  | station with no page | plays | plays | plays | none | play triangle when active |
|  | web link | opens the browser | same | same | none | arrow-up-right when active |
| Suggestion list: recently searched | term | searches | searches | searches | none | clock icon, chevron when active |
|  | album, artist, song with a page | opens | opens | opens | none | chevron |
|  | station with no page | plays | plays | plays | none | play triangle when active |
| Best match panel | song, album, playlist, station with a page, video | opens | opens | none | amber disc beside it | link |
|  | artist, curator | opens | opens | none | none | link |
|  | song with no page, station with no page | plays | plays | none | the disc is drawn into the panel | button "X, Play" |
|  | web link | opens the browser | same | none | none | link, arrow-up-right at its end |
| Compact rows beside it | song | opens | opens | none | quiet Play button on hover and focus | link |
|  | album, playlist, station with a page, video | opens | opens | none | quiet Play button on hover and focus | link |
|  | song or station with no page | plays | plays | none | play glyph on hover | button |
|  | web link | opens the browser | same | none | none | link, arrow-up-right at its end |
| Group shelves ("Albums", See all grid, library scope) | as the cover tile | | | | | |
| Songs group, See all, library songs | song | selects | plays | none | number-column button | row |
| Recent searches shelf | term, album, artist, song with a page | opens | opens | none | none | link; the X forgets it |
|  | station with no page | plays | plays | none | none | button "X, Play"; play glyph on hover |
|  | library song, the other scope's entries | not listed (in the field's list either) | | | | |
| Command palette | artist, album, playlist | opens | opens | opens | none | cover and "Kind · subtitle"; the active row ends in a chevron |
|  | song | opens its page | opens | opens, Shift+Enter plays (the foot names "Shift+Enter Play" while it is active) | none | the same look, chevron |
|  | song with no page here (an upload) | plays | plays | plays | none | the same look, play triangle on the active row |
|  | "Search Apple Music for …" | searches | searches (the first entry is active until the user moves, so Enter right after typing runs what the list shows first, a library hit when there is one) | searches | none | search icon, chevron on the active row |

### The shell: sidebar, queue, player bar, mini player

| Surface | Item | Click | Double-click | Enter | Shift+Enter | Space | Play | Menu | Cue |
|---|---|---|---|---|---|---|---|---|---|
| Sidebar nav links | page | opens | opens once | opens | none | none | none | none | link, active fill |
| Sidebar playlist rows | playlist | opens | opens | opens | shuffles | none | the shuffle glyph over the art shuffles | More button and right-click, Shift+F10, Menu key | link; shuffle glyph on hover |
| Sidebar pins | playlist, album, song, set to Open | opens (a song opens its album) | once | opens | playlist, album: shuffles; song: none | none | playlist, album: the shuffle glyph over the art | More button and right-click, Shift+F10, Menu key | link; shuffle glyph on hover (playlist, album) |
|  | playlist, album, song, set to Play or Shuffle | plays or shuffles | twice | same | same | same | same | More button and right-click, Shift+F10, Menu key | button; the play or shuffle glyph at the row's end |
|  | artist, any action | opens | | opens | none | none | none | More button and right-click, Shift+F10, Menu key; no Play or Shuffle choice | link |
|  | any, set to Open with no page | nothing | | nothing | | | | same as its kind; Open checked and off | plain row, disabled to assistive technology |
| Sidebar folders | folder | folds or unfolds | net nothing | same | none | same | none | More button and right-click, Shift+F10, Menu key | disclosure |
| Queue rows | song, AutoPlay, music video | focuses the row | plays from the row | plays | none | none | none | yes | hover fill and grip |
|  | the playing song | none | restarts it | restarts it | none | none | none | the song's menu, as the bar's | amber title |
|  | song title, music video title | opens its album with the song marked, its page | | | none | none | none | the row's menu | link (a pointer shortcut, not a Tab stop) |
|  | artist | opens the artist (several credited: plain text) | | | none | none | none | the row's menu | link (a pointer shortcut, not a Tab stop) |
|  | "Playing from <station>" | opens the station | | opens | | | | | link, a Tab stop |
| Player bar | cover | opens Now Playing (a video: the video view) | | opens | none | opens | n/a | yes | button |
|  | title | song: its album with the song marked; station: its page; video: its page | | opens | none | none | n/a | yes | link |
|  | artist, album | opens | | opens | none | none | n/a | yes | link |
|  | play / pause | toggles | twice | toggles | none | toggles | n/a | none | button |
| Mini player | cover, title, artist, album line | none | none | no Tab stop | | | n/a | yes, on every line of text | plain text |

### Page headers and pages

| Surface | Control | What it does |
|---|---|---|
| Album and playlist head | Play | plays from the first song |
|  | Shuffle | plays from a random song with shuffle on (not on an album not out yet, whose Play says "Play the N songs out now") |
| Artist head | Play top songs, Shuffle | plays the eight top songs, or shuffles them |
| Artist latest-release slip | the slip | opens the album; plain (no button, tab stop or underline) when the page is not routed |
|  | disc | plays the album |
| Artist release rows, discography tiles | row or tile | opens (Cmd or Shift selects); disc over the cover, on hover and focus, plays the album |
| Artist library page | Play | plays the artist's library songs |
| Song page | Play | plays the one song (queue of one); no Shuffle |
| Music video page | Play, "Play the song", "Watch in Apple Music" | plays the video, plays the song, hands the link to the browser |
| Album, playlist, song, video pages | artist, album, curator links | open (a song's album link carries `?song=`) |

Argued departures that stay (not issues):

- **A pin set to Play or Shuffle plays on click.** The user chose it in the pin's menu ("When chosen"), and the pin's name says so to assistive technology; its glyph at the row's end (Frame and sidebar) says so to everyone else.
- **The Now playing queue row restarts the song** on Enter or double-click: it is a row of songs, and `Jump` plays its own index from 0.
- **The Artists master list chooses instead of opening**: it is a master-detail list, and the detail is the page.
- **A tile's Shift/Ctrl+click selects** (a list of tiles can be dragged into a playlist and acted on as a group) where the tile has an Item; the open/play cue is the one owed.
- **Playing one tile plays that tile alone** even with three selected: Play is a control of the tile, not of the selection.
- **The mini player is a remote: its cover, title, artist and album line are plain text.** It is a small frameless window that a press drags, so they take no click and are no Tab stop; the pages they could open (the album, the artist) live in the main window, which the player bar's links and Open Sona reach. Every line of the text block, and the cover, opens the playing item's menu on a right-click, Shift+F10 or the Menu key.

### Departures

| Issue | What |
|---|---|
| [#436] | Discover's lead card has no cue for a web link (the other surfaces draw the arrow out) |
| [#443] | the sidebar's play glyph shuffles; a Play/Shuffle pin looks like a link |

[#436]: https://github.com/coredev-uk/sona-go/issues/436
[#443]: https://github.com/coredev-uk/sona-go/issues/443

## Dates

- **The user's own things are dated in calendar words** (`i18n.RelativeDay`): a playlist's "Updated 3 hr. ago", "Updated yesterday", "Updated on Tuesday", "Updated last week", "Updated earlier this month", "Updated in March", "Updated last year", "Updated in 2023". Days are calendar days in the computer's zone, never 24-hour spans, so a change at 23:50 is "yesterday" ten minutes after midnight, and a page left open relabels itself at midnight.
- **The words never stand alone**: the exact date and time ("Tuesday, November 10, 2026 at 2:30 PM", nothing more) is the label's tooltip and its description, and the label is a tab stop on the catalogue line, so the keyboard reaches the tooltip and a screen reader reads the date after the words.
- **Freshness stays terse**: "Synced 4 min. ago", Concerts' and Discover's "Updated 12 min. ago" and Home's "Picked for you …" say how old a copy is, in minutes and hours (`i18n.RelativeTime`).

## Empty, loading and error states

- An empty route keeps its frame: the h1, any column header row it will have, then one dashed-border block (1px `--line`, radius 8) holding a 16/600 sentence-case title, one sentence in the product's own words, and at most one action.
- Loading: the final layout with flat `--hover` bars at the real text sizes; no shimmer, no layout shift when data lands.
- Errors live inside the block that failed: what failed, when data was last good, and a Retry text button. `--danger` only on a small icon.

## Selectable text

Nothing is selectable by default: a page of rows, tiles and links would fight its own clicks, drags and double-click to play. Text is selectable (MyGo's `Element.Selectable()`) only where people copy it, case by case. One rule decides how:

- **Values, not names.** The value text (`--ink`) is selectable, and so is the muted text that belongs to it: the ℗ and © line, a song's credit roles, the note under a Diagnostics warning and an error's reason. The muted name beside a value, a heading and a picture's initials are `Unselectable()`. MyGo paints the selection (`--selection`, the spot colour at 15%) under the text and leaves the text's colour alone; `theme.Validate` keeps both `--ink` and `--ink-muted` at 4.5:1 on it in every theme, and refuses a user theme that fails.
- **A container for a block of values**, so a drag runs across rows and Ctrl/Cmd+A copies the block, joined by newlines. A control inside keeps its own behaviour (MyGo does not select text inside a button or a link), so a selectable block holds no link or row that opens something.
- **An identifier also gets a Copy button** (`kit.CopyButton`: a 24px ghost icon button, named "Copy ISRC", with a toast "ISRC copied"): it is one Tab stop beside its value, for someone at the keyboard.
- **Roles and names are unchanged**: `Selectable()` adds no role, so a screen reader reads the same tree. Selecting never clicks the parent: a selectable block is not in a clickable parent.

| Candidate | Decision | Why |
| --- | --- | --- |
| Song page, Details: ISRC | Selectable, with a Copy button | The identifier people paste into a form or a report; the button serves the keyboard |
| Song page, Details: released, duration, genre, track, disc, written by | Selectable (values; the names are not) | Plain facts people quote; no clickable parent. The song has no label or ℗ line in Apple's data |
| Song page, Credits: people's names | Selectable (one selection across all groups) | Credits get pasted into notes and forms; names are `--ink` |
| Song page, Credits: roles | Selectable (in the same selection as the names) | Muted type, which the selection carries at 4.5:1 (issue #411) |
| Song page, Credits: group titles, initials | No | Headings and decoration |
| Album page, fine print: the label, release dates, genres, writers | Selectable (values; the names are not) | Plain facts people quote, in a column no row or link surrounds |
| Album page, fine print: the ℗ and © line | Selectable | A plain fact people quote; `--ink-muted` keeps 4.5:1 on the selection (issue #411) |
| Album page, fine print: UPC | Not shown | Sona does not fetch the UPC; there is nothing to select |
| Lyrics (song page, Now Playing, the full-screen sheet) | No | Apple's Developer Program License Agreement (3.3.6 D) allows MusicKit content only as the MusicKit APIs render it and says music-related text may not be used apart from playback; two of Sona's three lyrics sources (Musixmatch, LRCLIB) are licensed by others; and the Now Playing sheet's lines seek on a click. Unclear, so left unselectable |
| Settings, Diagnostics: values (versions, paths, ids, sizes, pid) | Selectable, one selection per group | Single values are what someone pastes into a bug report; "Copy diagnostics" still copies the redacted whole |
| Settings, Diagnostics: the notes under a warning | Selectable (same selection as the values) | What to do about it, worth pasting into a report |
| Settings, Diagnostics: labels | No | Names, not values |
| Error block: the reason (second line) | Selectable | The sentence people paste into a report; the title is not. Go's codes never reach the screen, "Copy diagnostics" carries them |
| Notice text (toasts) | No | A toast leaves by itself |
| About: the version and build | Selectable | Settings, Diagnostics, "Sona" group (the version, Go, platform); Sona has no About page of its own and the OS's About panel is the OS's |
| Concerts: venue address | No | Sona's data has the venue's name and city, not an address; its rows are links and ticket buttons |
| Track-list rows, tiles, sidebar, the player bar's title, buttons, links | No | They click, drag, double-click to play or open a menu |

## Playlist page

The playlist is set like a tape's J-card: the cover is the front panel and the facts are its spine.

- **Head.** On the cover's wash (`kit.Backdrop`), a 224px cover (144 under 960 wide, 288 from 1600 wide) beside a text column as tall as the cover: the title at 32/36, the byline and the description at its top, and the actions (Play, Shuffle, the library and rating buttons, Edit, the item menu) in one row on the cover's bottom edge. In edit mode the row holds Done, "Name and description" and Cover.
- **The ledger strip** runs under both, in one row: the kind of playlist in `--ink` at weight 600, then the number of songs, the running time, "Created 3 March 2024" (or "Added 8 September 2024") and "Updated 3 hr. ago". Each is a 13/18 `--ink-muted` cell with a 1px `--control-line` rule between. It is the catalogue line taken apart into cells.
  - The date is **"Created …" only for what the user made**: their own editable library playlists and the collaborative playlists they host (the collaboration's viewer is its owner). It is **"Added …" for everything else in the library**: a joined collaborative playlist, or one added from Apple Music or a curator. `dateAdded` is when the playlist came to *this* library, so it is the creation date only for the user's own; Apple exposes no creation date for the others (`docs/apple-music-reference.md`, "Playlist dates"), and a catalogue playlist outside the library has no date at all. Both are a calendar date, "today" or "yesterday" when recent. "Updated …" is in calendar words. Each keeps its tab stop and its tooltip with the exact date and time.
  - **Apple's personal mixes** (`pl.pm-…`) show only "Updated …": Apple makes them and rewrites them, and the day one was added says nothing. They use the sidebar's personal-mix test.
  - The date and the update on the same day show one cell ("Created today", "Added today"). **"Updated" never reads earlier than the date beside it**: an update before that date (Apple's `lastModifiedDate` of a playlist joined after its last change) leaves its cell out, and the date's tooltip keeps the exact time. An unknown date leaves its cell out.
  - "Find in playlist" sits at the strip's right end, with "N of M songs" to its left while it holds a query; there is no field above the table.
  - **Play and Shuffle play what the table lists.** While a Find query is active they queue the songs found, and say so: their accessible names read "Play the N songs found" and "Shuffle the N songs found" (the visible "Shuffle" and the Play disc are unchanged). Without a query they are "Play {playlist}" and "Shuffle".
- **Edit mode** keeps the table's columns: a header row (reorder, #, Title, Album, Starts, duration) over the rows, so the order being edited reads as a list of times.
- **Suggested songs** are rows of the table's own height and columns (art under the numbers' edge, title, artist, Add under the album column, length), the playlist's next rows rather than a grid of tiles. The shelf starts **collapsed**: its heading carries a Show/Hide suggestions disclosure (chevron), and nothing is fetched until it is opened (not remembered beyond the page). Open, the head also has **Refresh**, which draws another set (skeleton rows meanwhile, focus kept on the button, the result announced; songs of the replaced set and songs already in the playlist are left out). **Adding a song removes its row** at once, with the announcement and the focus moving to the next row's play button (the previous one, or Refresh when the list is empty); a failed add puts the row back in place with a toast. An empty or failed list shows a short note under the head, and Refresh stays in the head. **Featured artists** is a row of small round portraits (100px at least); more-from-curator keeps its covers (144px at least).
- **States.** The loading skeleton is the final layout (cover, title, strip, rows). An empty playlist or an empty edit mode puts its title and sentence flush left under the strip, with the suggestions below as rows.

## The song page: a liner card

The song page is a record's liner note for one track: who it is on the left, what is known about it on the right. The line is the one the album's band draws (cover, then the facts), so a song reads as a smaller sibling of its record.

- **Two columns from 860px of content.** The left is a spine in the cover's wash (`kit.Backdrop`, the same wash the other headers use) that bleeds off the top and left of the column.
  - The spine holds the cover in one of three sizes by the page's width (240px, 280px from 1300px and 320px from 1600px; the spine follows the cover), so Go serves it at its exact box and only three of them.
  - Under the cover: the title at 32/36 (three lines at most), the artist at 16/500 (a link, or a menu of the artists where there are several: "Several artists", above), the album as a link at 13 muted (it opens the album with this song marked), and Apple's badges.
  - Then the actions: the Play disc (amber, as every Play disc), Add, Favourite, Suggest Less and the menu.
  - The spine is as tall as its own content and ends there; the right column carries on beside and below it. Videos and More by the artist are shelves under both, full width.
- **The right column reads as a ledger**, top down: what the song is, who made it, what it says.
  - **Details** is a name and value per row: muted names in a 112px column, values in ink, all at 13/18 with a 1px `--line` under the heading. A fact Apple does not send has no row (Released in full, Duration, Genre, Track, Disc from the second disc, Written by, ISRC); the DTO has no label or ℗ line, so there is none. The album is not repeated there, so each link has one tab stop.
  - **Credits** are one block per group under their own 1px rule, the group's name at 13/600 muted (an h3 inside a group). The people sit in a grid of two, three or four to a row by the room the column has (at least 210px a person, 24px between).
  - The body column keeps its 760px reading width with up to three to a row and grows to 960px when four fit. The Details rows and the lyrics keep their own measure (760px, 540px) and the rules run the column's width.
  - A person is the 40px avatar, their name at 14 and their roles at 13 muted; the roles wrap.
- **Lyrics** come last: Apple's excerpt at 16/28, a verse break kept, with View full lyrics at the shelf's end, and the whole lyrics on request.
- **Under 860px** (down to the 940px window, 707px of content) the spine is a band across the top, the 136px cover beside the title and actions on the same wash, and the body follows under it in one column. A song with no credits and no lyrics says so under Details.
- **Loading** is the same frame built by the same code, with flat bars where the text goes: the cover's square, the title, the artist, the action discs, Details' five rows and two rows of credits. Nothing moves when the song arrives. An error or a signed-out account takes the column's top in the usual way (a way back, Try again, Sign in).

## People: pictures and initials

A credited person (the song page's Credits, in a grid of two or three to a row) is a 40px circle beside their name: their picture where Apple has one, their initials where it has not.

- **The picture** is the `artwork` Apple sends with the credit (`credit-artists`), cut to the centre square by Apple's CDN and requested from Go at the exact size like any cover (`kit.PersonAvatar` over `kit.Artwork`, round). Apple's own stand-in for a person it has no picture of, an initials image under `/image/thumb/gen/`, is not a picture: Go drops it (`applemusic.Artwork.Generated`), so the same initials are not drawn twice in two styles. The credits arrive with the song page, so pictures cost no request of their own; a picture is requested only when its row is built.
- **The initials** (`kit.Initials`) are the first letter of the first and of the last word, upper-cased: "Thom Yorke" is TY, "Madonna" M, "Ludwig van Beethoven" LB. Punctuation before a letter, a bare "&" and a leading "The" are skipped. A letter is a whole user-perceived character in any script (米, 봉, ف, 𠮷); a name in a script without spaces gives its first character. They are set 38% of the avatar's size in weight 600, centred.
- **The tint** is one of eight quiet tones (`avatar-<n>-bg` and `-fg` in `tokens.json`, per theme; `kit.AvatarTone`) picked by a hash of the lower-cased name, so a person has the same tint on every page and session. Hues spread around the wheel at low chroma (light: L 0.90, C 0.035 under L 0.32 text; dark: L 0.36, C 0.04 under L 0.92 text) so a column of them reads as a set, not as brand colour. Text on every tint is at least 4.5:1 in both themes (a test reads the values from `tokens.json`).
- **A picture that fails** to load falls back to the initials while Go's retry runs, never the broken-image glyph or the grey artist glyph.
- **Decorative**: the name is always written beside the avatar, so the avatar is hidden from assistive technology; the credit reads as name, then roles.

## Home

Home ("For You") answers one question first: what shall I play now. It is set like the A-side of a record: the lead pick is large, Apple's other picks are the running order beside it (unnumbered: Apple's order is not information a reader needs), and the shelves under it are ruled sections. It sets Apple's personal shelves (`internal/ui/features/home/plan.go`) instead of stacking them as Apple sends them, and never copies Apple's own Home.

```text
┌───────────────────────────────────────────────────────────┐
│ For You  Picked for you 3 min. ago              ⟳ Refresh │  one line; wash of the lead's
│ Top Picks for You ───────────────────────── Show all 12 ⌄ │  cover, full bleed
│ [ cover 280 ]     [40] Apple Music Live       New Release │
│                   ──────────────────────────────────────  │  the running order:
│                   [40] …                      …           │  one list, hairlines
│ Your Station      [40] …                      …           │
│ Station · …       …                                       │
│ (▶) (⋯)           [40] …                      …           │
├───────────────────────────────────────────────────────────┤
│ Recently Played ───────────────────────────── See all ›   │
│ [112] [112] [112] [112] [112] [112] [112] [112]            │  strip of covers
│ Name   Name …                                             │
│ Made for You ──────────────────────────────── See all ›   │
│ [160] …                                                   │
│ More for you ─────────────────────────────────────────    │
│ ›  Country        3 playlists · 9 albums     [32][32][32] │  the ledger
│ ──────────────────────────────────────────────────────    │
└───────────────────────────────────────────────────────────┘
```

- **The page's head** is one line of the lead band: "For You" at 24/650, when Home was picked (13 muted, on the title's baseline) and the ghost Refresh at the far end. Under it the first section's ruled head.
- **Ruled heads**: every section of Home (`rule.go`) is its heading at 16/600, a 1px hairline (ink at 14%, so it reads on the ground and on a cover's wash) that runs to the right edge, and the section's controls on the line's end: "See all" to the section's own page, or "Show all N" / "Show fewer" in place. Sections sit 48px apart, so the page keeps a rhythm of rule, content, space.
- **The lead**: the first "feature" shelf (Top Picks).
  - **Its first pick is large**: the cover at 280px as the left column (360px from 1400px of lead width; a 144px cover beside the name when the page is under 900px wide), then the name at 24/650, the artist or curator at 16/500, the catalogue line (`Station · Made for You`: what it is, then Apple's reason) and the amber play disc with the item's menu.
  - **The other picks are the running order** beside it (under it when narrow): one column of 56px rows in Apple's order, none numbered (the order is not information the reader needs, and a rank is decoration). Each row is its 40px art, the name at 14/500 and Apple's reason, side by side in two equal columns from 480px of row, under the name below it. A hairline closes every row.
  - **What a pick says** is what Apple's own pick says: Apple's reason, the name, and one line. For a station or episode Apple recommends, the name is Apple's short editorial name and the line its one-line description (`plainEditorialNotes`), never the tagline label or a promo sentence. A line that only repeats the name is left out (`kit.Repeats`), and a clipped name or line has the full text as its tooltip.
  - Eight rows show; "Show all 12" in the head opens the rest.
  - The page header and the lead sit on the lead cover's pre-blurred backdrop, ground scrim, full bleed. Reduce effects leaves the plain ground.
- **Rows** (`internal/ui/features/home/rows.go`): 56px rows with 40px art, the title at 14/500 and Apple's reason with the artist or curator at 13 muted. Every shelf of stations only (Stations for You, moods, favourite artists) is rows two deep, in as many columns as fit from 264px: you play them at once, so a name and a play control matter more than a large cover. The art sits on the page's left edge; the hover plate reaches 8px past it. On a backdrop the hover is ink at 6%, not the opaque `--hover`.
- **Recently Played** is a strip: one row of covers as wide as their columns fit (112px up to about 130, the name at 14/500 and the line under it at 13 muted), so a whole evening's listening reads at once. It is the same list as the rows (one Tab stop; arrows, Home and End move; Enter opens; Space plays; Shift+F10 or the Menu key opens the menu): hover or focus shows a play disc on the cover and the more button at its corner. "Show all N" opens further rows of covers in place.
- **Cover shelves**: the first three plain shelves Apple ranks highest keep their cover tiles (160px; feature shelves 176px) under a ruled head. Features, heroes and track shelves always stay in full.
- **The index** ("More for you"): every later plain shelf (genres, "more like", fans also like) is one 52px line of a ledger: a chevron, the title at 16/600, what it holds at 13 muted in a column of its own ("3 playlists · 9 albums"; under the title below 640px of line), and a strip of its covers at 32px at the far end, a hairline between lines. The line is a disclosure button; open, the shelf's cover tiles appear under it and the strip goes. Twelve shelves take one screen instead of four, and their full-size covers load only when asked for.
- **A lone hero** (Apple's untitled featured card) is a square band across the row, washed in its colour: the art's pre-blurred backdrop under the ground scrim, as the lead band is, and the plain ground under Reduce effects. Inside it, 32px in, are its square art (200px, 128 under 560px of row, with the `float` shadow) and, 24px beside it and centred against it, the name at 24/650, the catalogue line and the play disc. The square art is the cover, which for Apple's stations is the station's own tile (the centre square of its key art, "Pop" with the Apple Music mark). A card with no cover shows the centre square of its banner. The wide banner is not shown: for Apple's stations it is a gradient with the name in a corner, so shown large it is mostly empty colour.
- **Captions** every item of a shelf shares are dropped; said once in the heading, they are noise.
- **States**: signed out, the lead's place holds one ledger band (a rule above and below, "Recommendations need Apple Music" at 24/650 with one sentence under it, an ink Sign in at the far end), then the library's shelves. Offline with no copy of Home, the same band says there are no recommendations while offline and that Sona fetches them when it reconnects: no Retry, and no Refresh in the header while offline. Apple's Recently Played failing falls back to "Played on this computer", with its own empty line. Loading draws the final layout: the head, the 01 cover with the name's lines and the play circle, and the running order's eight ranked rows.
- **Keyboard**: the lead's title, play and menu are Tab stops; each row list (the running order, station rows, the Recently Played strip) is one Tab stop (the arrow keys, Home and End move; Enter opens, Space plays, Shift+F10 or the Menu key opens the row's menu; the row focused last is where Tab returns). Index lines are disclosure buttons that tell assistive technology whether they are expanded.

## Shell

```text
┌──────────┬───────────────────────────────────────────────┐
│ ● Sona   │ ‹ ›  [ Search Apple Music and your library ]  ◯ ✕ │  48px surface, the title bar
│          ├───────────────────────────────────────────────┤
│ Listen…  │                                               │
│ Search   │  h1 + optional one-line subline               │
│          │                                               │
│ Library  │  content (scrolls; virtualised lists)         │
│ Songs    │                                               │
│ Albums   │                                               │
│ Artists  │                                               │
│ Playlists│                                               │
│          │                                               │
│ Settings │                                               │
├──────────┴───────────────────────────────────────────────┤
│ ━━━━━━━━━━o───────────────────────────────────────────── │  the seek bar: the bar's top edge
│ [art] Title ☆    0:42 ⇄ ⏮ (⏵) ⏭ ⟳ 3:58      ☾ 🎤 ≡ 🔈──    │  72px surface + backdrop
└──────────────────────────────────────────────────────────┘
```

### Frame and sidebar

- Sidebar and top bar share `--surface`; the canvas is `--ground`, divided by one 1px `--line`.
- The mark is a small amber disc with a spindle hole (a record label seen from above), next to the word "Sona" at 16/700.
- Active nav item: `--active` fill at radius 6, label and icon in ink. Inactive items are `--ink-muted`.
- Group label "Library" in 12px muted Title Case, 20px above its first item. Settings sits at the sidebar's foot.
- **Playlists in the sidebar.** Below the library links the sidebar lists playlists in three sections. Each is under a 12px Title Case heading at weight 600 in `--ink-muted`, with how many it holds beside it in `--ink-faint` (tabular), 22px above and 4 below.
  - **Pinned**, **Apple Music** (the playlists Apple provides, in one list: first its personal mixes, the library copies whose catalog ID starts `pl.pm-`, in Apple's order, then the playlists the user added that they cannot edit, from Apple Music or another curator, Replay and others, by name) and **Playlists** (the user's own, editable playlists and their folders, and every collaborative playlist, whether the user hosts it or joined it; a joined one sits in its folder like the rest, and offers no edit, move or drag-in because Apple says it is not editable).
  - A pinned playlist shows under Pinned only, whichever section it belongs to. A section with nothing left in it is not drawn.
  - Apple Music is a flat list with no "New playlist" button, no folders, no drops and no rename. The mixes and the added playlists share one list with no divider or sub-label: the mixes are named for what they are and the order says the rest. Its heading is a button that folds the list shut (state kept across restarts, `config.Sidebar`), with a chevron that turns, as a folder's does. Apple's Favourite Songs playlist (tagged `favorited`) is not in it, nor in Library → Playlists, Recently added, Home or search: those songs are one page, Library → Favourite songs. A pin Apple already holds for it stays under Pinned and opens that page. Playlists keeps the "+" menu and the drop row.
  - Over the cover of a playlist row, and of a playlist or album pin, a small **shuffle** glyph (the design system's `shuffle`; the play triangle is for what plays in order) fades in with the row's hover (`kit.Fade`, at once under reduced motion) on a `--ground` wash at 85%, so it reads on any cover. The glyph is `--ink`, never amber (amber marks what plays).
  - Clicking the glyph shuffles that playlist or album and does not open it; Shift+Enter on the focused row does the same, so the glyph is the cue for the key, and a row without the glyph (a song or artist pin, a folder) does nothing on Shift+Enter. A playlist with no songs, or while the account needs signing in again, shows it in `--ink-faint` with the reason as its tooltip. Folders have no cover and no button.
  - **Pins.** A pin set to Open with no page of its own (an album or song whose page is not ported) does nothing when chosen, never plays. The pin's menu offers what its kind does: Play, Shuffle and Open for a playlist, album or song (Open is off, and checked when that is the setting, while there is no page), and no "When chosen" group for an artist, which only opens (there is no artist player yet).
  - A pin set to Open is a link, as a library row is. A pin set to Play or Shuffle is a button and draws its action at the row's end: the play triangle or the shuffle glyph, 12px in `--ink-faint`, which gives way to the More button while the row is hovered or focused.

### Top bar and account

- Top bar: back and forward, then the search field (max 520px, `/` or Ctrl+F focuses it, never narrower than 240px), an empty stretch of at least 64px, the notification bell and, 8px after it, the account avatar.
- **The avatar** is the account menu's button.
  - A `--hover` circle signed out; `--active` with the glyph in ink signed in, and while its menu is open (it reads as held). A small `--danger` dot appears when the account needs the user (Apple stopped taking the sign-in, the keyring didn't hand it over, Apple refuses Sona's requests).
  - **The menu** (`--raised`, 288px, end-aligned) opens with "Apple Music" and then each fact about the account on a meta line of its own behind a 14px glyph: where it stands (a check, a clock while it is being checked, an alert in `--danger` when it needs the user), offline, restricted content, an unencrypted token in `--danger`.
  - Then come the one action that fits (Sign in or Account settings, Check now, Try again), private listening as a checkbox item with its note, and Sign out in danger red last.
  - Anything that needs room (the sign-in form, the account's details) opens the account dialog, never a page.
- **The account dialog** is two panels, 680px, bare (no padding of its own, corners clipped to the 8px radius).
  - The leading panel (280px, `--surface`) holds the Sona mark with its name at the top. At the bottom it holds the one heading (24/30, "Your Apple Music, on your desktop") and three single lines on what signing in adds (your library in sync, recommendations, whole songs rather than previews), each behind a 16px glyph.
  - A 1px `--line` divides it from the account's panel, which holds the sign-in flow or the signed-in view in a 36px inset. Signed in, the panel titles the view "Account". The first lines of both panels sit on one centre line.
  - Below 640px of panel width the panels stack, the lead above the account with a hairline between. The whole panel scrolls as one in a short window, so the lead's tone runs as long as the form.
  - Close, Escape and a click on the scrim close it as every dialog, and focus returns to the opener.

### Notification centre

What happened without the person asking, and what is going on that they may need to act on, lives behind one bell in the top bar. It replaces desktop notifications for releases (they stay an opt-in), the toast that said an add-on was switched off, and the sidebar's sync line and update slip. Toasts stay what they were: quick feedback on something the person just did ("Link copied"). Track notifications are Playback's and stay desktop notifications.

```text
                                   ( 🔔• )  ( ◯ )      the bell, 8px before the avatar
        ┌──────────────────────────────────────┐
        │ Notifications              Clear all │     16/600, Title Case
        │ ┌──────────────────────────────────┐ │
        │ │ ⟳ Syncing songs     812 of 2,040 │ │     live rows: --hover, radius 6
        │ │   ━━━━━━━━━━──────────────────── │ │
        │ └──────────────────────────────────┘ │
        │ [▣] In Rainbows is out       2h ago •│     entries, newest first
        │     Radiohead                        │
        │ (⧉) Webhook was switched off  Monday │
        │     It stopped several times in a …  │
        │ ──────────────────────────────────── │
        │ • Synced 2 minutes ago      Sync now │     the foot: meta, --ink-muted
        └──────────────────────────────────────┘
```

- **The bell** is a 32px ghost button (radius 6) with the Lucide `bell` at 16 in `--ink-muted`; hover is `--hover` with the glyph in ink, and while the panel is open it is `--active` (held, as the avatar is). It is never amber: nothing here is playing.
  - **A dot** at its top end (8px, a 2px `--surface` ring, as the avatar's) says there is something to see: `--ink` for news (unread entries, an update waiting, an add-on waiting for approval), `--danger` when something failed (a sync, the Widevine download, an add-on that can't run, an update that didn't install). Colour is never the only sign: the bell's name says how many are new, and the panel says what failed.
  - **Work under way** (a sync, the first Widevine download, an update downloading) draws a 2px bar, 14px wide, at the bell's foot: an `--ink-faint` track with the fill in the glyph's colour. It moves only when the work reports progress (no loop, "Motion"); a size not known yet is the empty track. The bell's name adds what runs ("Notifications; Syncing songs, 812 of 2,040").
- **The panel** hangs from the bell, end-aligned: 360px, `--raised`, the float shadow, radius 8, 8px padding, Escape or a press outside closes it and the focus returns to the bell.
  - **Head**: "Notifications" at 16/600 and, when there are entries, a ghost "Clear all" at the end.
  - **Live rows** come first: what is going on now, read from each module's own state, never stored (`module.Activity`). Each is a `--hover` block at radius 6 (`kit.ActivityRow`): a 16px glyph in `--ink-muted` (`--danger` when it failed), the title at 14/500, a tabular count at its end in `--ink-muted`, a detail line at 13 in `--ink-muted`, a 3px bar while work runs, and its action as a small ink button (a quieter ghost one after it). In module order: the library sync (running, or failed with Try again), add-ons waiting for approval or unable to run (Review opens Settings → Add-ons), a new version (Install and restart where Sona updates itself, View elsewhere), and the first Widevine download (or why there is none, with Try again where it can help).
  - **Entries** follow, newest first, in a list that scrolls past 392px: a release you asked to be reminded of came out ("In Rainbows is out", the artist under it, the cover), new releases from your artists (the release and "New single from …", or "3 new releases from your artists"), an add-on switched off after crashing. Each is a row with a 32px picture (the cover, with its 1px edge, or the kind's glyph on a 32px `--hover` tile), the title at 14 and the detail at 13 muted, each up to two lines, and when it happened at 12 in `--ink-faint` at the trailing end. Choosing the row opens what it is about and closes the panel. Under the pointer or the focus the row takes `--hover` and the time gives way to a Remove button (a ghost `x`), faded in over `press`.
  - **New** entries have their title at 600 (500 once read) and a 6px `--ink` dot after the time. Opening the panel marks them read; the ones that were new keep their mark until it closes.
  - **Nothing** live and no entries: "Nothing new" at 14/500 over one muted line, centred, instead of an empty list.
  - **The foot** is the library's line while no sync runs or failed: a hairline, then a 6px `--positive` dot, "Synced 2 minutes ago" in meta `--ink-muted`, and a ghost "Sync now".
- **Entries are kept** in SQLite for 30 days (the newest 100). They hold names, never sentences, so they read in the interface's language of the day. The ones about the account go at sign-out.

### Library pages

- **Library pages share one header** (`features/library/header.go`): the title at 24/650 with the size of the list on its baseline in 13 muted tabular figures (`24 albums`), the page's controls at the right edge on the same line, then one 1px `--line` that starts the list. The count takes no height of its own, so nothing moves when it arrives. History puts its one meta line under the title instead of a count.
- **A–Z letter rail.** Albums (ordered by Title or Artist), Songs (Title, Artist or Album) and Artists (unfiltered) carry a static column of letters at the page's right edge, outside the scroll container, 24px wide.
  - The rail is A to Z and `#`: the letters that start something are buttons in `--ink-muted` 12/600, the others are in `--control-line`. A letter jumps to the first item of that letter, scrolled to the top of the list.
  - It is one toolbar ("Jump to letter") and one Tab stop, Up and Down moving among the letters that can be jumped to. Each button is named "Jump to A".
  - It shows only for an ascending order, since the library sorts bytewise and a descending list has no useful first item. The page keeps the rail's place (the list is 8px narrower than it would be) whether or not it shows, so controls and tiles never move when the sort changes.
  - A list longer than one page of 200 finds its letters by bisection over single-row reads on a worker (a few hundred, once per sort until a sync), and shows every letter faint until they are known.
- **List controls** sit on the header's right. The album list has a segmented control for what it shows (All, Albums, Singles & EPs; `Segmented`, the same control as the search scope, 32px tall in a `--hover` track, the chosen one raised) before the Sort select.
  - The choice lives in the URL (`?type=`) beside the sort and keeps its lifetime: Back and Forward return to it, and a reload keeps it.
  - A type with no albums keeps the page and says so, with Show all albums as its one action, rather than the empty-library text.

### Player bar

- **The now-playing bar is a transport deck.**
  - The seek bar runs along its whole top edge: a 3px track (5px under the pointer or the keyboard) with the amber fill up to an always-shown 10px ink thumb, in a 12px-tall hit area across the window's width. A live station has a plain 1px `--line` there.
  - **The song** stands at the left: cover 48px, title and the line under it, Favourite. The line's artist and album are each a link in `--ink-muted` that underlines on hover when its page exists, the album opening `/album/{id}` plain since the title already opens it with the song marked. A live station's marker and what is on air take that line.
  - **The transport** sits at the centre of the bar, not of what is left of it: the elapsed time (`--ink-muted`), shuffle, previous, the 44px amber play disc with a solid glyph, next, repeat, and the length (`--ink-faint`), the times in tabular figures at 12.
  - **At the right**: the sleep timer, lyrics, queue, mute and the 96px volume slider.
- **Scrolling over the volume** (the mute button and the slider, and the gap between them, in the bar and in the video view) changes the volume, as dragging the slider does.
  - A notch of a mouse wheel is 2% (50 notches from silent to full; Ctrl+↑ and Ctrl+↓ keep 5%). Scrolling that arrives in fractions is proportional at the same rate (1% for 25 DIPs of travel).
  - There is one rate because the system does not say which device scrolls: on Linux a high-resolution wheel, or a compositor's scroll factor, sends a mouse wheel as fractional deltas that look like a trackpad's, and two rates made the same wheel jump 10 to 14% a notch, whichever of vertical and horizontal moves further.
  - The direction is the one the system reports, so "natural" scrolling follows the fingers: scrolling up, or fingers moving down or to the right, raise it (the other way right to left, where the slider fills from the right).
  - It stops at 0 and 100%. From 0 (muted) it raises the volume, as the slider does, and the icon follows. The momentum of a swipe after the fingers lift changes nothing.
  - The page never scrolls under it, and Ctrl with the wheel is left to zoom. The mini player has no volume.

### Title bar

The sidebar header and the top bar are the window's title bar. Who draws it comes from Go (`Capabilities.WindowControls`, resolved by `internal/windowchrome`; `internal/ui/app/shell.go` creates the window from it), never from a default of the toolkit:

- **macOS**: the system's own traffic lights, inset into Sona's 48px band (hidden title bar, full-size content, centred on the band's height). They keep full screen, tiling, the inactive dimming and VoiceOver. The toolkit reports the room they take (`Context.TitleBar`) and the band keeps it clear at the leading edge (in the sidebar header, or the top bar while the sidebar is hidden).
- **Windows and the stacking Linux desktops** (GNOME, KDE Plasma, Cinnamon, Xfce, MATE, Budgie, Pantheon, …): the title bar is hidden under Sona's band and the window buttons are MyGo's, laid out as the desktop lays out its own (on Linux GTK's `gtk-decoration-layout`: GNOME's default is Close alone; on Windows the caption buttons at the trailing edge), so they read, look and behave as the desktop's own. Leading buttons are kept clear in the sidebar header (or the top bar while the sidebar is hidden); trailing ones in the top bar after the account.
- **Tiling compositors** (sway, Hyprland, i3, niri, river, bspwm, …) and desktops Sona doesn't recognise: the desktop's own title bar or none; Sona draws no buttons and the band is not a drag region.
- Settings → General → **Title bar** (Windows, Linux): Automatic (the hint names the desktop and what Automatic picks there), Sona's with window buttons, Sona's without window buttons, The desktop's. It applies from the next start (the window is created with or without its title bar).
- Where Sona's top bar is the title bar, the band's empty parts drag the window (`DragWindow`) and a double click maximises or restores; every control in the band stays a control. On macOS a double click does what the system's title bar setting says.
- The band keeps the buttons' room clear. Close does what closing the window does (Settings → General: keep playing in the background, or quit).
- The mini player keeps its own Open Sona and Close buttons, at the trailing end of the title line, or before the title where the platform puts Close first (macOS and leading desktop layouts), Close outermost, 4px apart. It is a cassette: the cover fills the window's height at the leading edge with no margin, the text and transport sit beside it, and progress is a 2px hairline along the whole foot (4px with a thumb under the pointer or focus), amber on a 12% ink track; the time left is meta text at the foot, the transport at the trailing side.

### First-run setup

The setup is a modal over the live app, not a page of its own: the sidebar, top bar and player bar stay drawn behind the dialog scrim (taking no input), so the first thing anyone sees is the app they are setting up, and each Appearance choice shows at once on the window behind. The card is as tall as its step, but never shorter than its step rail, so there is no empty column to fill.

```text
┌────────┬─────────────────────────────────────────────────┐
│ sidebar│ top bar          (scrim over all of it)         │
│        │   ┌───────────────────────────────────────┐     │
│        │   │ ● Set up Sona  3 of 7      Skip setup │     │  head, 56px
│        │   ├─────────────┬─────────────────────────┤     │
│        │   │ ✓ Welcome   │ Playback                │     │  rail: one Tab stop
│        │   │ ✓ Apple Mu… │ What plays your music…  │     │
│        │   │▌3 Playback  │ [cards] / rows          │     │  step scrolls inside
│        │   │  4 Connect… │                         │     │  when short
│        │   │  5 Appear…  │                         │     │
│        │   │  6 Desktop  │                         │     │
│        │   │  7 Done     │                         │     │
│        │   ├─────────────┴─────────────────────────┤     │
│        │   │ ‹ Back                       Continue │     │  64px foot
│        │   └───────────────────────────────────────┘     │
├────────┴─────────────────────────────────────────────────┤
│ player bar                                               │
└──────────────────────────────────────────────────────────┘
```

- **The card** is 800px wide (less 24px a side on a narrow window) and holds two panes under a head that spans it: the step rail at the leading side (192px, on `--surface`, a tone step against the `--raised` card and no line) and the step beside it (28px padding, scrolling on its own).
  - The card is `--raised` with the float shadow and, in dark, a 1px white/10 border. The dialog scrim is black/50 in light and black/70 in dark, where a lighter one left the card barely apart from the app behind; no blur.
  - Its top edge is fixed at 10% of the window's height (at least 24px), so the head, the rail and the foot don't move from step to step. It grows downwards to 24px above the window's foot, and past that the step scrolls while the head, the rail and the foot stay.
- Below 744px of window (a card under 720px) or when the rail would not fit between head and foot, the card is the narrow one, 640px wide: the head grows a strip under it, one segment per step filled up to the current one with each name under its segment, and the rail is gone. The strip is the same list with the same keys.
- The head names the setup and the place in it ("3 of 7"), with Skip setup at its trailing end. The rail lists the seven steps down the card, one 36px row each: its number (tabular figures), or a check once the step is behind (muted ink, not green and not amber: a step passed may have been skipped), and its name. The current step is `--ink` in semibold with a 3px ink bar at the row's leading edge; the rest are muted and take `--hover` under the pointer. The rail is one Tab stop: ↑/↓ and ←/→ move between steps, Home and End go to the ends, Enter goes there. It is a list named "Setup steps" of one button per step; the current step's button is the current step.
- The welcome step is the first picture: the Sona mark at 56px (the amber label with two grooves and the spindle hole, the same mark as the head's, enlarged) beside "Welcome to Sona" at the 32/36 display size and one sentence, then what the setup covers and what whole songs need (Apple Music and Widevine, each with where it stands). The language row joins them once there is another language to choose.
- Each step is a title, one line, and its own parts, not Settings' rows: choice cards for a choice between a few (audio quality, theme, what closing the window does), short switch rows for on/off, a compact Widevine status, and a row per connection. They save through the same queries as Settings, so a choice made here reads the same there.
- Theme is the one picture: three windows in miniature drawn from each palette's tokens (System is both, halved), as choice cards (Sona's own theme in the chosen mode); one muted line under them points to Settings → Appearance for the other themes (OLED and the community palettes).
- Where the platform can't do something (no tray, no autostart, no notifications) Desktop says so in one line at its foot instead of leaving the reader to look for a missing row.
- One primary button, at the foot's trailing end ("Get started", Continue, Done). On Apple Music, before signing in, the password form's Sign in is that primary, in the foot beside a ghost "Not now", so it stays in view however far the form scrolls (at 940×600 the form runs below the fold); the form's own heading is left out, the step's title names it; Back is a ghost button at the leading end. No amber but the mark and the previews' play controls.
- Every step can be skipped. "Skip setup" ends it at once; Escape asks first ("Skip the rest of the setup?"), except on the last step, where it is Done; a click outside does nothing. The last step lists only what is still to do, each with the way back to its step.

## Discover

Discover (`/new`) sets Apple's editorial front page as Sona's own issue: a masthead, the front page (the featured cards as one large lead with a queue of them beside it), the contents, then the shelves grouped by what they hold, each band with its name in a margin. It is the one long page in the app (twenty-odd shelves), so it gets a lead to start from and a way to reach any band without scrolling or tabbing through the rest. Charts and the room, curator, multiroom and grouping pages share its masthead.

```text
┌──────────────────────────────────────────────────────────┐
│ Discover  Updated 12 min ago                  (controls) │
│ ┌────────────────────────────────┐  ┌────────┐ ┌────────┐ │
│ │                                │  │ (lead) │ │        │ │
│ │   lead: 60% of the column      │  └────────┘ └────────┘ │
│ │                                │  Title      Title      │
│ └────────────────────────────────┘  Byline     Byline     │
│ ━━━━━━━━ ━━━━──── ──────── ────────  ┌────────┐ ┌────────┐ │
│ Lead title              ⏸  ‹ 2 of 6 › │        │ │        │ │
│ Apple's one line                     └────────┘ └────────┘ │
│ Badge · curator                      Title      Title      │
│                                                          │
│ On this page                                             │
│ [Songs and albums 3]  [Playlists 2]  [Radio 1]           │
│   Best New Songs        Updated Playlists  Live Radio    │
│ ──────────────────────────────────────────────────────── │
│ Songs and      Best New Songs                  See all > │
│ albums         ▭ ▭ ▭ ▭                                   │
│ (24/650)       New This Week                             │
└──────────────────────────────────────────────────────────┘
```

- **Masthead.** The page title at 32/36 (the display size) and, on its baseline 16px after it, the issue line at 13 muted (when the data was fetched, or the stale-copy sentence); the page's controls (Charts' Genre, a room's Order and Play) sit at the row's end, centred on it. In a narrow window the line wraps under the title rather than being cut. Loading: a bar holds the line's place.
- **Front page.** One featured card is the lead, the first to start with.
  - **The spread.** From 900px of content the lead and the queue share the row: the queue takes 40% of the column as two rows of two tiles, 16px apart across and 20px down, 24px from the lead, and the lead takes the rest. The lead's picture is as tall as makes the lead (picture, progress and caption) end with the queue's second row, kept between the banner's own 4:1 and the hero card's shape. Narrower, the lead takes the whole column (the hero card's shape until 540px or 45% of the visible height, wider from there, never past 4:1) and the queue is one row of four under it, 12px apart, two from 560px down.
  - **The lead** shows Apple's picture centre-cropped to its box (requested at that exact box), with a play button on hover and focus and the item's menu.
  - **The progress** sits 12px under the picture: a 3px segment a card across the lead's width, 4px apart. The segments before the lead are `--ink`, those after it `--line`; the lead's own fills with `--ink` over its `--line` as its interval runs, so the wait between slides is in view. While the rotation holds or is paused, and under reduced motion, the lead's segment is a whole `--ink-muted` mark of where it is. It moves in frames that only paint it (no build and no layout), each asked for when its fill has a device pixel more to show (about 21 a second for a 150px segment), and it is drawn, not read: the pager's count says the same.
  - **The caption** sits 12px under the progress as a row: the title at 24/30, Apple's one line about the card at 14/20 in ink below it, the byline at 13 muted under that, and at the row's end the pager: the pause control, then the previous chevron, "2 of 6" and the next chevron, 28px each.
    - What it says is what Apple's hero card says: Apple's badge, the name, the artist or curator, and its one line (on music.apple.com the line sits over the picture). The line is the card's own tagline on a card with its own art (320), else the content's short editorial note. The badge is folded into the byline ("Updated playlist · Apple Music Pop"), never set as a label over the title.
    - The line's row is kept for a card without one, so nothing under the lead moves as it rotates. Clipped, it shows whole as a tooltip, and the lead's opener is described by the line and the byline.
    - The chevrons bring the card before or after into the lead, wrapping round, so a focused chevron is never disabled under the keyboard.
  - **The queue** is a still set of four featured cards in Apple's order, the lead among them. A tile is the card's picture in the hero card's shape (Apple's centre crop, as the lead shows it, in miniature, or the cover whole on the item's colour for a card with no banner), radius 4, then 8px down the title at 14/500 (underlined on hover) and the byline at 13 muted.
    - **The tiles do not move as the lead rotates.** The lead's tile is marked where it stands: an `--active` fill reaching 6px past it at radius 6 (the mark of what is selected, never amber; described to assistive technology as the current one), with the lead's progress as a 2px line along its bottom. As the lead moves on, the mark moves from tile to tile (its fill fades over 400ms). Only when the lead leaves the four does the set move on, to start at the lead (wrapping round), as one 400ms cross-fade of the whole queue.
    - A tile opens its card, as the lead does and as every other tile does ([#442]). On hover and focus it shows a 32px play disc at the picture's bottom left and, but on the lead's tile, a "Show in lead" button at its top right ("Show in lead: {name}" to assistive technology); the button makes the card the lead, and when the keyboard pressed it the focus goes on to the lead's title, since the button goes.
  - Play controls appear on hover and focus as on every tile; a right-click, Shift+F10 or the Menu key opens the item's menu.
  - What the front page shows is left out of the cover shelves below it (a shelf it would empty stays whole), so one album isn't met four times on the way down.
- **The lead rotates.** Every 7 s the lead gives way to the next featured card, wrapping round.
  - The new card fades in sliding 1rem from the right over 400ms while the old one fades out the other way (ease in-out, so the two pictures cross rather than one vanishing first). Nothing blurs or filters, and a one-off 400ms transition plus one deadline every 7 s (no per-frame work) is all the page does.
  - The next card's picture is fetched and decoded while the lead is read, so it fades in as a picture. The interval restarts whenever the lead changes, however it did, so a chosen card is read for as long as any.
  - **It holds** (and starts a whole interval after) while the pointer is over the hero or the keyboard has put focus anywhere in it, while the window is hidden or unfocused, and while the hero is scrolled off screen or covered (by Now Playing, lyrics or a modal). Focus a click left on a control (the pause button or the pager) does not hold: the pointer has gone and nothing would end the hold.
  - A tick that finds a menu open, or a video playing in the lead (motion artwork is left to play), waits a whole interval more.
  - Under Reduce effects or the desktop's reduced motion it does not run: the lead is still, the pause control is not shown, and the queue's "Show in lead" and the chevrons are the way to change the lead.
- **Pause and play** (WCAG 2.2.2): a 28px ghost button at the start of the pager, with the pause glyph while the lead rotates and the play glyph while it is paused, named "Pause featured rotation" or "Play featured rotation". It is in the Tab order after the lead's own controls and before the queue, and reflects only the user's choice, not the temporary holds. Pressing it, and the chevrons, are said once through one polite announcement ("Rotation paused", "2 of 6"); the lead's own changes are never announced, and the lead's outgoing card is an inert copy outside the accessibility tree while it fades.
- **Contents** ("On this page"). Under its name (16/600) a row of columns, wrapping when the width runs out, one 216px column for each band: its 32px button on `--hover` at radius 6 (`--active` on hover), the band's name at 14/500 in ink and the number of its titled shelves at 13 muted (described as "3 shelves"), over a button for each titled shelf at 13 muted (ink and underlined while hovered). Every button jumps to its place (no smooth scroll) and moves focus there, so the next Tab is that band's or shelf's first control: any shelf is one jump away. A page of one band has no contents.
- **Bands.** The shelves are grouped by what they hold, in this order: Songs and albums, Playlists, Radio, Videos, Browse (category bricks and links).
  - Apple's order holds inside each band. A shelf of mixed items goes by most of them (the first item breaks a tie), and a paragraph stays with the shelf after it.
  - Each band opens with one 1px `--line` across the column, 56px below the band before.
  - From a page 1100px wide the band's name stands in a 200px margin column at the left, 24px under the line, at 24/30 (it wraps in the margin). The shelves fill the column beside it, 32px away, their first title set 6px lower so the two baselines meet. Narrower, the name stands above the shelves, 24px under the line.
  - There is no numbering: the order of the bands is not information. Inside, shelves keep their usual 40px pitch.
- **Loading.** The masthead's line holds a bar; the front page's own frame (the lead's block, its progress and caption bars, the queue's pictures and lines at their tile size), the contents' buttons and the first band's line and heading (in the margin at the wide size) stand where they will land, then bars for the shelves.

## Charts

Charts (`/charts`, `/charts/{chart}`) takes Discover's masthead (Genre at its end). Top songs is the kit's track list, its first column the chart rank as big numerals (the title size, tabular figures, right-aligned in a 56 DIP column; Play replaces the numeral on a hovered row), and the other charts are shelves with "See all". Loading: the masthead's line holds a bar, then the Top songs heading and six rows of covers and bars, then a shelf of tiles.

## Search

The term is the page title (32/36, in curly quotes) with the scope switch (Apple Music, Library) on its baseline at the end. With no query the page is the recent searches and the browse categories.

Results are a **card catalogue**: a 200px rail on the left lists the groups found, each as a button with its name and its count (tabular figures); a click scrolls that group to the top and moves the focus to it, and the group in view sits on the `--hover` tone. The rail is outside the scroll container, so it stays while the results scroll. Under 960px it is a wrapping row of the same buttons (6px radius, a `--line` border) above the results. The results column follows Apple's order, each group under a 1px `--line` and its 16/600 heading:

- **Top results** are a best match and its neighbours: the first result in a `--surface` panel (radius 8, `--line` border) beside up to four compact rows (40px cover), below it under 640px.
  - The panel has a 120px cover, its name at 24/650 and its catalogue line under it (the kind first, then the subtitle: `Artist · Rock`, `Album · Blue Band · 2019`; no label above the name), and the amber Play disc where it plays.
  - Choosing a result opens its page, a song's too: the panel and the rows are links, and the amber Play disc beside the panel is the separate control that plays it.
  - Every playable compact row (song, album, playlist, station with a page, video) has a quiet Play button at its end, shown on hover and focus with the menu button. The menu keeps Play and Play next.
  - Only a result with no page here (an upload, a station that plays when opened) plays as a whole, and then the disc is drawn into the panel and its row carries the play glyph.
- **Artists** are one row of round portraits (104px at least, the tile grows to fill the row), the other item groups cover shelves with 136px tiles, so five or six fit where the other pages fit four.
- **Songs** are the track list (number, cover, title, artist, and album from 720px of results width), eight rows, "See all" beside the heading. They behave as on the album and playlist pages: activating a row plays, the artist and album cells open their pages.

Loading is the same layout in `--accent` blocks (the rail, the panel, its rows, one shelf of portraits). Nothing found is a title at 24/650, one sentence and one ink button that searches the other scope; a failed search is the dashed error block with Try again; signed out, the sign-in sentence and button take the same place.

The suggestion list under the top-bar field:

- **Runs** are named by an icon and a 12/16 600 heading: Suggestions, Top results, Recently searched, In your library.
- **An item row** ends in its kind's icon.
- **The active row** (the keyboard's or the pointer's) takes the `--active` fill, a ring and a glyph where the kind's icon was. The glyph says what Enter does:
  - a **chevron** opens a page. A song's suggestion opens its song page. Shift+Enter is Enter here: no cue hints at a second action and a recent entry keeps too little of a song to play it, so the list has none;
  - a **play triangle** plays: a song with no page of its own, an upload in the library, and a station with no page (the kit's `PlaysWhenOpened`);
  - an **arrow-up-right** leaves Sona for the browser (a web-only link, as the link list draws it).

## Album page

The album page opens on the record: a full-bleed band in the cover's own colour with the cover as its hero and the head beside it, which dissolves into the ground below it, so the colour belongs to the top of the page and has no edge to scroll into. Under it the listing is on the plain ground: the songs, with the disc index and the fine print in a margin beside them.

```text
┌ band (full bleed, cover's colour, flush top/left/right) ──────────┐
│ ┌─────────────┐                                                    │
│ │             │  Title (32/36 → 48/52 with the cover, wraps)       │
│ │             │  Artist (in the cover's accent)                    │
│ │    cover    │  Album · 1995 · 40 songs, 2 hrs…  [Lossless]       │
│ │ 232/288/352 │  Apple's note                                      │
│ └─────────────┘  (▶) Shuffle  + Add  ☆  ⊘  ⋯      ← head on the cover's bottom edge
│ ░░░░░░░░░░░░░░ dissolve into the ground (112–128px) ░░░░░░░░░░░░░░░│
   #  Title                                  ◷       Disc 1 · 12 songs…
   Disc 1                                             Disc 2 · …
   Goldberg Variations (1981 Recording)  ← a work     ─────────────────
   1  Aria                                  3:22      Released · Genres
   2  Variatio 1                            2:08      Written by · Label
   …                                                  ℗ line
   Notes (58ch)
Other Versions · Music Videos · More to Hear · More by · Featured in Playlists · You Might Also Like
```

- **The band is printed in the cover's colour**: a flat, square surface (no radius, no outer margin: the pane's edges cut it) in the colour Apple gives for the cover's background (`ArtworkColors.background`; Go's first dominant colour when Apple gives none).
  - Nothing is blended at paint time but the two gradients below (`internal/ui/theme/palette`; built once per cover and theme, never per frame).
  - It is the page's one coloured surface. The listing sits on the plain ground under it, so the colour stays at the top with the record.
  - The page's own gutter is the band's inner margin, so the band's text, the songs and the shelves share one left edge, and the margin's right edge is the shelves'.
  - A cover with no colour known keeps the backdrop wash: Go's backdrop under the ground scrim with a 4% ink layer.
  - Three rules hold the colour to the board:
  - one tone step off the ground: a cover the colour of the ground (a white one in light, a black one in dark) moves 0.05 in OKLCH lightness away from it, the way the theme's own tones go;
  - in the dark theme no lighter than 0.36 OKLCH lightness, hue kept, so a white or pale cover is a dark tint of itself rather than the brightest slab in the window; in the light theme a dark cover stays dark;
  - never so near a mid grey that black or white loses 5.8:1 on it and its pressed tone, so the text and a muted step below it can both reach 4.5:1.
- **The dissolve**: the band ends in a stretch with nothing in it where the ground's own colour comes over it as a gradient of opacity.
  - The stretch is 112px from 860px of content, 128px from 1600px, 64px on a short pane and 72px in the narrow band.
  - The gradient is a smootherstep sampled in 24 stops: no slope at either end, so the band neither begins to fade abruptly nor lands on the ground. No step is more than 8 of 255 in any channel, which a test reads down the page's edge.
  - Both gradients are the ground colour or the text's opposite laid over the band, built once per theme or cover, never per frame, never a blur.
  - The pre-blurred backdrop is not used on the band: its 32px picture, scaled to a band's size, shows its steps wherever it is not under the 92% scrim, and no scrim keeps text 4.5:1 over any picture.
- **The far end deepens** (wide only): from a third of the way across, the band takes up to 30% of the colour its text is not (white under dark ink, black under light) as a left-to-right gradient, so a wide band is not a flat slab. Only ever away from the text's colour, so the text's contrast, which the surface is chosen for, can only improve where the gradient lies (a test checks ink and muted ink on the far end against the surface for extreme covers in both themes).
- **Reduce effects** prints the band solid and ends it square (no dissolve, no deepening); the cover's shadow stays, being no wash.
- **Text on the band** takes Apple's own text colours for the cover (`text1` for ink, `text2` or `text3` for muted) when they read at 4.5:1 on the band and on its hover and pressed tones.
  - Otherwise ink is black or white with a trace of the band, and muted is ink mixed 35% towards the band, moved in lightness only as far as 4.5:1 needs.
  - The band redeclares its own `ink`, `ink-muted`, `hover`, `active`, `line`, `control-line` and the roles built on them (`theme.Scope`), so the badges and outline buttons inside it (the whole head and the actions) follow it. Focus rings there are the band's ink.
  - The palette reads its colours from the theme in use, so the band works on every theme: the ground it dissolves into, and the colours it must read against, are the theme's.
- **The cover's accent takes amber's job on the page.** The most colourful of Apple's colours and Go's dominant ones (OKLCH chroma ≥ 0.06) fills the Play disc, marks the playing row and its equaliser, and sets the artist's name, the record's one link out.
  - Two fills, each fitted in OKLCH lightness (hue and chroma kept) to at least 3:1 in both themes: the Play disc against the surface it sits on (the sleeve in the narrow layout, else the ground; in dark the ground too), and the equaliser, playing-row mark and selection against the ground their rows sit on. They are no longer one colour on a light page. A cover whose hue no lightness serves keeps Sona's amber for that role. The disc's glyph keeps 4.5:1, and the accent as text keeps 4.5:1 on every row tone, a selected row included.
  - A greyscale cover keeps amber.
  - The accent is redeclared for the whole page body, Apple's shelves below included: the mark as `amber` and `on-amber`, the disc as `play` and `on-play`, and `amber-ink` and `play-ink` for text.
  - Only the album page takes colour from its cover this way: the song, playlist and artist headers keep the backdrop wash of the cover's first dominant colour.
  - Every pair is checked by unit tests over extreme covers (white, black, mid grey, the sRGB primaries, a 6-level grid in both themes).
- **The cover is the hero**, at a fixed size so Go serves it at its exact box (never a size that follows the window, which would ask for a new picture on every frame of a resize).
  - Sizes: 232px from 860px of content, 288px from 1240px, 352px from 1600px (a 1920 window), 168px on a short pane (under 560px high) and in the narrow band.
  - It casts the `float` shadow (offset 8, blur 24), because a cover is often the band's own colour at its edge.
  - The title grows with it (32/36, 40/44 from 288, 48/52 from 352), and wraps instead of clamping.
  - Wide, the whole head (title, artist, catalogue line, badges, note, actions) sits on the cover's bottom edge, as a record's header does, with its ordinary spacing. A head taller than the cover (a long note, a title on several lines, an album not out yet) starts at the cover's top. In the narrow band the head starts at the cover's top.
  - Motion artwork plays there at the cover's size, while it is on screen, under the same rules as every header.
- **A short pane compacts the band** (under 560px of height, a 1280×600 window): the 168px cover, 20px above it, the 56px dissolve, the actions 14px under the badges and Apple's one-line note left to the notes below, so Play and the first three songs are on the first screen.
- **The margin holds while the songs scroll** (from 860px of content): the disc index and the fine print, one left edge, are a 264px column 48px right of the songs, held 16px under the pane's top (`Element.Sticky`) while the songs go by and released when they end, whenever the pane shows all of the margin (measured from its height in the frame before, so a one-disc album holds in a shorter window than a three-disc one; a margin taller than the pane scrolls with the page). It is first in the tree, so Tab and the reading order meet the disc index before the songs. An album with neither discs nor facts has no margin and its songs take the width.
- **The fine print** is the record's facts in meta type, each fact a muted name over its entry: released (the day in full), first released (the year of its oldest recording, when that is earlier: a reissue, a compilation), genres (all of Apple's, without the root "Music"), written by (every songwriter in Apple's composer credits, most credited first, eight named and the rest counted) or, on a classical album, composer, then the label unless the ℗ line names it, and the ℗ line last. In the margin from 860px of content; in the narrow layout after the songs.
- **The disc index**: each disc in the catalogue line's form ("Disc 2 · 14 songs, 1 hr, 3 mins"); choosing one brings its heading to the top and focuses its first song. A list long enough to be virtualised (over 200 songs) has no disc headings, so no index either.
- **A song with no disc number is on no disc.**
  - An upload's file often has none (`DiscNumber` 0, which Apple's order by disc puts before disc 1), and "Disc 0" is never shown.
  - A record whose songs all lack a disc is one disc with no index.
  - On a record with discs, the songs without one follow the last disc (each group in its own order). They sit under no heading of their own and out of the disc index, so a disc's count and running time are its own songs'.
  - They are told from the disc's songs by their **Uploaded** badge, not by a sub-heading. The album page is an operating surface: a "Your uploads" band would be a second heading level for what one pill already says, and would need wording for songs without a disc that are not uploads.
- **Uploads among the songs** are listed in the record, badged (Lists, above): a catalogue song and an upload of the same name are two rows, the second with its badge. A record of uploads alone, with nothing to tell apart, carries no badges.
- **Classical albums read by work**: where Apple names the work a track belongs to, each work gets a heading (14/500 ink, under the disc's) and its rows show the movement alone ("Aria", not "Goldberg Variations, BWV 988 (1955 Recording): Aria"); the row's play control and menu still name the song in full, and the queue and now playing show its full name. An album of several composers leads each work with its composer and names the composer under a piece outside any work.
- **Rows explain themselves**: the line under a title names a credit other than the album artist's (an Artist column only for a real compilation, where most credited songs are someone else's; a song with no credit yet counts for neither) and why a song doesn't play: "Unavailable" or the account's explicit restriction (on an album not out yet its group's heading says when: Not out yet, below). An unpublished song's stand-in artist ("unknown") is dropped (by the services, so no list names it, here or anywhere), and "Popular" only marks a song that plays.
- **The catalogue line describes the record**: a library album holding some of its songs counts the whole album and says "2 of 13 songs in your library · Show complete album" in the status slot; an album not out yet leaves out a running time its unpublished songs can't add to.
- **The catalogue line counts what plays**: the uploads among the songs are in its song count and running time (a partial library album adds them to the record's own count), though the "2 of 13 songs in your library" status counts only the record's songs.
- **Songs not out yet are the album's alone**: a pre-added album's unpublished songs ("Track 3", credited to nothing) are listed on its page, as songs to come under the group heading with the day, in the muted ink of a song that doesn't play, with no Play control and none of the menu's actions that need a released song (favourite, suggest less, add to playlist, pin, go to the artist); Songs and its count and letters, an artist's songs, the favourites and the library search leave them out until the day they are out, and from then on list them with their real names after the next sync.
- **Not out yet** (an album Apple marks as a prerelease, from the catalogue or pre-added to the library): the page answers "when, and what can I do about it", in that order, and is the released layout with its head reordered and its songs in two groups, not a second page. In the band the head reads title, artist, catalogue line, **the release**, the actions, then Apple's badges and note (taller than the cover, so it starts at the top).

  ```text
  Salt Flats                                  title (32/36)
  Mia Vale
  Album · 2026 · 5 songs · Indie Pop
  Out in 18 days, 6 hours                     the lead (24/30, 650)
  Sunday, 22 November 2026 · Pre-added        the day in full, then what you've done (13/18, muted)
  (▶) [+ Pre-add] [🔔 Remind me]  ☆ ⊘ ⋯        Play the 2 songs out now, then the account's answers
  [Explicit] [Lossless]  Apple's note
  #  Title                                  ◷
  Out now                                    group heading (14/500)
  1  Salt Flats                           3:20
  2  Low Tide                             3:20
  Out 22 Nov                                 group heading
  3  Glass Houses
  ```

  - **What leads: the release.** The countdown is the biggest line after the title, in `title` type and ink (never amber, which only means playing), with the day in full under it. It is one `Release` group for assistive technology (the time left, then the day), not a live region: it is re-read every minute and must not be announced every minute. The songs already out are an offer (the Play disc and the "Out now" group), not the headline.
  - **Primary action.** One control leads, and only without a Play disc is it ink-filled: the Play disc when any song is out ("Play the 2 songs out now", in the cover's accent like every Play disc); else **Pre-add** for a signed-in account; else **Remind me** where the platform has notifications and the album has a full day. Pre-add comes before Remind me, as the one that changes the library. The rest are outline buttons, so the row never has two primaries.
  - **Done states are said twice, never by colour alone.** A pre-added album's button reads **Pre-added** with a check (the outline "In library" button: it asks before deleting, named "Pre-added. Delete “…” from your library"); a set reminder is Remind me pressed, with the ringing bell. The day line adds "· Pre-added" and "· Reminder set". With both done and no song out there is no ink-filled button at all. A library album pre-added before release is the same page, already Pre-added.
  - **Each song says for itself whether it is out; the album's date never decides for it.** A song with `playParams` is out (it plays and sits under "Out now"), even when Apple gives it no `releaseDate` of its own ("Flower In A Hurricane" on the GTA VI album: out six days before the album's date); a song with a `releaseDate` ahead, or with neither, is coming. The same rule holds in the library's copy of the album, the library's song lists, search, an artist's top songs, and playlists, and Play queues only the songs that are out (`kit.PlayableRefs`, and the service refuses the rest with `not_released`). The album's own date still heads the page. One song is out and still does not play: a library-only song (no catalogue copy) with no `playParams` and a `releaseDate` of its own, on an album that is out. Apple answers its library ID with "no longer available" (checked live, 2026-11-16, all four in a real library), so `mapper.libSong` marks it not playable and its row is greyed and says "Unavailable". A library-only song with no date and no `playParams` is left to its album's date: it is the song of an album that has just come out, and waits for the next sync.
  - **Songs out now and songs coming are two groups, not greyed rows.** The songs that play come first under an **Out now** heading, then the rest under the day they are out ("Out 22 Nov"), each group in the record's order with its own track numbers; no row repeats the day and no duration column fills with dashes (`TrackListOpts.BlankDuration`). A record with every song to come has the one heading and nothing to separate; one with every song out has none. Two groups have no disc headings or disc index, because discs would repeat across them. The coming rows are the list's ordinary disabled rows (`--ink-faint`, 4.5:1), now explained by their heading rather than by a line each.
  - **A year and no day** ("Out 2027") has no countdown and no Remind me (a reminder needs a day); the lead is "Out 2027" and the line under it says "The day hasn't been announced yet". The songs sit under one "Out 2027" heading.
  - **No song out** has no Play disc: Pre-add (or Remind me) is the page's one ink-filled button, and the list is a preview of the record with its title and numbers only.
  - **The moment it comes out while the page is open.** When the release time passes, the countdown becomes **Out now** with "Updating the songs" in the line under it. The page announces "{album} is out now" once (a polite announcement, `c.Announce`) and fetches the album again, every minute.
    - The service says an album is out from its date alone, whether or not Apple has published its songs (they have no play parameters until it does). So the page keeps the state while songs that were waiting are still unplayable, up to 30 tries (about half an hour); after that the songs show as they are ("Unavailable").
    - Meanwhile the head keeps its shape (the stand-in credits stay dropped, the catalogue line stays without a running time, no dashes), and the songs aren't marked "Unavailable". When they play the page is the released one.
  - **Long and classical lists.** Headings need a list the table builds whole, so an album of more than 200 songs (a virtual list) keeps its order and says "Out 22 Nov" under each song that isn't out. On a classical album a group's heading carries the work of its first row ("Out now · Ludwig van Beethoven · Symphony No. 5"), the work named again even where it runs on from the other group.
- **Below 860px of content** (a 940px window) the band is the same band, 168px cover with the title beside it and the same dissolve (72px), so the page keeps its character across the breakpoint; the actions run under the cover and the title block when the text beside the cover is under 400px wide (the queue open). The songs follow at full width, then the fine print and the notes on the ground; the disc headings in the list stand in for the index. There is no margin.
- Then Notes at a reading measure (58ch, about 75 characters at the body size), and Apple's shelves in Apple's order, with the radio episodes about the record ("More to hear") after its music videos.

## Artist page

An artist page is the act's press kit: the sheet that says who they are and what is newest, what to play first, the career laid out year by year, then the company they keep. It answers "who is this" on the first screen and never repeats a release to fill space.

```text
┌ header (backdrop wash) ──────────────────────────────────────────────┐
│ Radiohead (32/36)                      ┌───────┐              ╭────╮  │
│ Artist · Alternative · Abingdon, …     │ 152   │              │ 200│  │
│ the biography's first paragraph, 2 lines│ cover │              │    │  │
│ Read more                              │ (▶)   │              ╰────╯  │
│ (▶) Shuffle  ☆  Suggest less  ⋯        └───────┘                      │
│                                        Title · Latest release · date  │
└──────────────────────────────────────────────────────────────────────┘
Top songs (6 numbered rows)             See all │ ┌ Essential albums ──┐
#  Title                    Album          ◷    │ │ ▭ In Rainbows 2007 │
…                                               │ └────────────────────┘
Discography (24/30) ──────────────────────────────────────────────────
2021 ● ▭ KID A MNESIA · Compilation
     │
2016 ● ▭ Daydreaming · Single      ▭ Hail to the … · Live
     │
2004 ● ▭ Com Lag: 2+2=5 · EP
     Singles and EPs featuring Radiohead ›
Music videos: one row of 16:9 tiles
Related ──────────────────────────────────────────────────────────────
Appears on (a list) · Playlists featuring · Similar artists
About Radiohead ──────────────────────────────────────────────────────
From · Born or formed · Genres (200px) │ the whole biography (32rem)
```

- **The header** is the act's press sheet, one row on the wash: the text column at the leading edge, the latest release in a second column, the round portrait (people stay round) at the trailing edge (200px; 256px from 1500px of content; 176px under 1040px; 112px under 720px).
  - **The text column** holds the name at 32/36, the catalogue line in its usual form with the kind first (`Artist · Alternative · Abingdon, Oxfordshire, England`; `Artist · 1 release` when the page holds every release of a small act), the biography's first paragraph at 13 muted clamped to two lines at a 32rem measure, "Read more" (which scrolls to About and moves focus there), and the actions on one row.
  - **The latest release** is a 152px column from 1040px of content (192px from 1500px): the cover with the amber Play disc over its corner, the title at 16/600, and `Latest release · 13 Aug 2025` and `Album · 12 songs` in meta type, with Apple's " - Single" / " - EP" suffix moved into that line.
  - Narrower, the latest release is a row under the text, after a hairline: an 88px cover (64px under 720px), the lines and the Play disc at the end.
  - The disc plays the album through the item players and is left out where the album cannot play (not out yet). Title and cover open the album. An act with one release has no slip (it is in the discography).
- **The lead**: top songs (six rows, numbered in Apple's popularity order; the rest behind See all) with the essential albums beside them from 800px of content, set in a 340px `surface` panel at radius 8 as a release list. Under 800px the panel follows the songs, its albums in two columns.
- **Release lists**: the form of essential albums and Appears on. Rows of 52px: a 40px cover, the title at 14/500 and a 13 muted line (the year, or the artist and year for Appears on), a `--hover` fill at radius 6 that reaches 8px past the covers so they line up with the heading. Rows open, select, drag and open their menu (right-click, Shift+F10, the Menu key) like cover tiles; ↑ and ↓ move between them.
- **Bands**: each is its heading at 24/30 with a hairline running on to the column's edge.
  - **Discography** is a career line: the releases the page holds (albums, singles and EPs, live albums, compilations), once each, newest first, the newest 18, grouped by year. The year (16/600 tabular, faint) is a 64px left gutter (52px under 720px), and a dot and a 1px rail link the years.
  - A year's releases are release rows to the right (two columns from 840px of content when a year has four or more), each saying its kind (`Album`, `Single`, `EP`, `Live`, `Compilation`, or `Out 19 Nov` for an upcoming one).
  - Under the career line, a ghost link names each shelf's view the timeline does not show whole (`Singles and EPs featuring Radiohead ›`), so every full view stays one click away. Music videos follow as a row of 16:9 tiles.
  - **Related** holds Appears on (a release list), playlists featuring the artist and similar artists.
  - **About** is a spec column (From, Born or formed, Genre at 200px) ahead of the whole biography at a 32rem measure. Narrower, the facts run in a row above the text.
  - Empty shelves and bands are left out.
- **Loading** holds a major act's final frame: the name, line, two biography lines, Read more, actions, slip and the portrait at the trailing edge, then top songs and the essential albums panel as loading rows of the real lists. For such an act nothing moves when the data lands, but the scrollbar arriving with it narrows the column by its width. A small act with no biography or slip has a shorter header, so under 800px of content its songs move up when they load.
- A library artist keeps its library shelves (albums, singles and EPs, songs in your library) between the header and the lead; its catalogue line counts the library (`Artist · Alternative · 3 albums, 20 songs in your library`).

## Library Artists

Library → Artists is an overview, not a launcher: every artist in the library on one side, and whichever one is open on the other, with their library albums. Moving from one artist to the next never leaves the page.

```text
┌ list 320 ────────────┬ detail ───────────────────────────────────────────┐
│ Artists (24/30)      │ ( portrait 96 )  ABBA (32/36)                     │
│ 90 artists           │                  Artist · 12 albums, 130 songs    │
│ [ Filter artists   ] │                  (▶) Shuffle  Go to artist  ⋯     │
│ A                    │                                                   │
│ ◉ ABBA               │ Albums (16/600)                                   │
│   12 albums, 130 s…  │ ▭ Voyage         ▭ More ABBA Gold   ▭ Oro   ▭ Gold │
│ ◉ AC/DC              │   2021             1993               1993    1992 │
│ …                    │ Singles and EPs                                   │
│ B                    │ ▭ …                                               │
└──────────────────────┴───────────────────────────────────────────────────┘
```

- **Two columns, two scrolls.** The list is a column as tall as the window with its own scroll box, with a 1px `--line` to the detail, which scrolls with the page. So the albums' grid virtualises against the detail's pane like every other cover grid, and the list keeps its place while they scroll. The page bleeds to the content edges to let the column run from the top bar to the player bar.
- **The list**: the page title and count on one baseline (24/650 and 13 muted; `1 of 12 artists` while filtering), then the filter (32px, the Shortcuts filter's field).
  - Under them the artists run A to Z under a letter heading each (13/600 ink with a 1px `--line` running out to the right, the letter itself, not a label), `#` last for digits, symbols and other scripts. The letter rail sits at the list's right edge while the filter is empty.
  - Rows are 44px with a 36px round portrait (the artist's own, else their newest album's cover, else the placeholder), the name at 14/500 and what the library holds of them at 12 muted (`12 albums, 130 songs`, tabular figures). The open artist is a `--active` fill at radius 6; hover is `--hover`.
  - Heights are fixed, so the list, which builds only the rows in view and a few beyond in its own scroll box, never measures: thousands of artists scroll like ninety.
- **The filter** matches as you type, ignoring case and accents (`bjork` finds Björk, `edith` Édith Piaf, `sorensen` Sørensen), best first: names starting with it, then names with a word starting with it, then any substring, then the name without its spaces and punctuation (`acdc` finds AC/DC). Filtering drops the letter headings, since the order is the match's. The artists come from one query (`LibraryService.ArtistIndex`: name, portrait, album and song counts, from SQLite only; a collaboration counts and lists under each artist it is credited to, `docs/architecture.md`, "Artists of a song, stored"), so filtering never calls Go and never waits.
- **The detail** is the open artist's library, not the catalogue.
  - **Head**: the portrait (144px, 192px from 1500px of page width, bottom-aligned with the actions), the name at 32/36, the catalogue line (`Artist · 12 albums, 130 songs`), the amber play disc, Shuffle, **Full artist page** (the artist's own page, with the catalogue: biography, top songs, the whole discography) and the artist's menu. A 1px `--line` follows.
  - **Albums** is a cover grid, newest release first. **Singles and EPs** sit under their own heading (each heading carries its count in tabular figures and a 1px rule out to the right).
  - Apple's " - Single" / " - EP" title suffix, or the album's single flag, decides which group an album is in. The suffix moves to the tile's line: `2020 · Single`.
  - Play and Shuffle read the artist's songs from SQLite when pressed. An artist with songs but no albums shows their songs (Songs in your library, the track table) where the albums would be.
- **In the URL**: the open artist is `?artist=<library id>`, so reload, Back and Forward keep it. With none (or one not in the library) the first match is open beside the list, without writing the URL. Clicking an artist adds a history entry; moving with the arrow keys replaces the entry, so a walk down the list is one step back, not fifty.
- **Keyboard**: the filter (a combobox) and the list (a listbox, the second Tab stop) share their keys. ↑/↓ move the open artist (PageUp/PageDown by ten, Home/End on the list), Enter or → (from the end of the filter's text) go into the detail: focus lands on its first album, or on its first control while the albums are still being read. Escape clears the filter, and from the detail returns to it. Typing on the list types into the filter. The open artist is the active descendant of both while its row is built; rows tell assistive technology their position and the size of the set. The count of matches is announced.
- **Narrow windows** (the page under 880px wide, which is a 940px window with the sidebar open): the list and the detail take turns. The list fills the page; choosing an artist (a click or Enter, which opens the filter's top match when nothing is marked) shows the detail with a ghost **‹ Artists** button above it; the artist's name is then the page's h1. The arrow keys move a cursor in the list without opening anything. The list stays mounted while hidden, so its scroll and filter survive. Back (the button or the browser's) returns to it with focus in the filter and the artist you left still marked; Escape in the detail moves focus to the button.
- **States**: signed out, syncing, empty and error are the library gate's block in place of both columns (`Artists` title above it). Loading draws fourteen list rows. An artist's albums loading draw six cover tiles; failing, an inline error with Retry. No match: "No artist matches “xyz”" under the filter, and no detail open.

## Now Playing

The record held open: the sleeve on one side (the cover and its label copy), the insert on the other, one at a time: the lyric sheet (Lyrics, below) or the running order still to play (Up next). It covers the page column, so the sidebar, the top bar and the player bar with its transport stay where they are, over a wash of the cover (Backdrops). A fold, one 1px `--line` from the top of the view to its foot, divides the two.

```text
┌ page column, the cover's wash ───────────────────────────────────────┐
│                         │ Lyrics   Up next                        ✕  │  tabs at 16/600, Close
│                         │ ────────────────────────────────────────── │  the header's rule
│    ┌───────────────┐    │                                            │
│    │  cover 400    │    │    the pane: the lyric sheet,              │
│    │  (320, 240)   │    │    or the queue's rows                     │
│    └───────────────┘    │                                            │
│    SKINNY (32/36)       │                                            │
│    Billie Eilish        │ ────────────────────────────────────────── │  the foot's rule
│    HIT ME HARD AND SOFT │ Translation …    − Lyrics on time + ⤢ Full screen │  the pane's own strip
│    [E]  ☆  ⊘  ⓘ  ⋯     │                                            │
└──────────────────────────────────────────────────────────────────────┘
```

- **The sleeve.** The cover is at a fixed size so Go serves it at its exact box: the smaller of what the column's width and its height allow.
  - Sizes: 560px from 1800px of column and 1050px of height, 480px from 1500 and 900, 400px from 1000 and 720, 320px from 880 and 640, 240px in a shorter window. On a big screen the sleeve fills its column instead of floating in it.
  - The view uses the whole page column up to 1760px (the largest sleeve and an insert of about 1100px), centred beyond that.
  - The cover and the label copy under it are one block, held at the top of the sleeve column, 24px down, on the same line as the insert's tabs at any height. The column is the cover's width and 48px at each side; a block taller than the column is cut at the foot. The copy is set from the cover's left edge.
  - **The label copy**: the title at 32/36 (24/30 under a 320px cover), wrapping to three lines; the artist at 16/500 and the album at 13 muted, both links out that close the view; the marks the bar also shows (Explicit, Preview, and Spatial while the song plays as its Spatial Audio rendition: the same quiet `Tag`, no colour, in the bar beside the title where it is the first thing to go in a narrow window); then, where the Spatial tag would be, the quiet line that says why a song Apple lists with a Spatial Audio mix plays stereo while Spatial Audio is on ("Stereo · Apple keeps this song's Spatial Audio mix for its own apps": 12/16, muted, no colour, up to two lines; the bar has nothing for it); then the song's own actions: Favourite, Suggest Less and View credits (the song page).
  - **A station** shows its name, a Live line with the dot (amber while it plays) and what is on air, as the stream or the schedule names it.
  - **A local broadcaster** is headed by the song its stream names on air (that song's cover, its artist under it), with the station, its genre and place, its description and an Open website button below.
  - **A music video** shows its 16:9 frame at the cover's width and a Show video button.
- **The insert.** Its header is one row, the tabs named in words (Lyrics, Up next) and Close, 47px tall over a 1px `--line` rule: the shown tab is in ink with a 2px ink rule resting on that line (not amber, which stays with what plays). A station or a song outside the catalogue has no lyrics, so its row holds only the heading Up next. The view reopens on the pane it showed last; the bar's Lyrics opens it on the lyrics and its Queue (or Q) on Up next, and each closes the view when its pane is already showing. The queue beside the page steps out while the view is open and comes back when it closes, so the queue is never drawn twice or hidden under the view.
- **Each pane's controls sit at its foot**, as full screen keeps its own, in a 52px strip under a rule of the header's length: the lyric sheet's Translation and Pronunciation at the start, the timing nudge and Full screen at the end; Up next's line ("9 songs after this one", or the station it plays from) at the start, Clear and AutoPlay at the end. The header holds only the tabs and Close, so the column opens on the words.
- **One text edge.** The tabs, both rules, the lyric lines, the queue's numbers and the foot's line all start 24px from the fold (12px of the insert's side, 12px of inset). Up next is the queue's own list (rows, keys, drag and drop; the groups of the queue panel, Lists); on the wash its hover and focus fill is ink at 6%, not the opaque `--hover`, the playing row's fill is ink at 5%, and its equaliser holds still under the view like every other (Lists). Beside the page the Next heading's detail line is the panel's summary; here the foot says it, so the heading has none.
- **The sheet's reading position** is measured from the sheet's own height, not the window's: the sung line stands a third of the way down, so it sits there however much the header above takes. The first line starts there too, so nothing moves until the second begins.
- **Narrow** (under 880px of column, a 940px window with the sidebar): the sleeve becomes a header, the cover at 168px beside the label copy and Close at its end, over the tabs (the insert's header, with its rule) and the pane at full width; there is no fold.
- **No lyrics**: the usual dashed block ("No lyrics for this song", the reason) with one link, Lyrics sources, to Settings → Playback.
- **Cost.** Nothing new moves, and nothing beneath it is drawn: the page under the view is hidden (it stays built, so it keeps its scroll, but is not drawn and takes no input), so a motion cover never repaints the album page's own header under it. The layout follows the view's own box as last drawn and changes only when its size class does. The pane not shown is not built, so on Up next the lyrics' timers are gone. Motion artwork plays on the cover from one rendition (asked for at 560 DIPs, the largest cover class, so Apple's 1120 px picture at 2x, never narrower than the cover) at every size, so a resize never reloads it.
- **Focus.** Opening the view focuses the shown tab (Close when there is one pane); leaving full screen returns to Full screen; Escape closes the view, from anywhere in it (a tooltip showing on the focused button takes the first Escape, as everywhere).

## Lyrics

A lyric sheet, the insert in a record's sleeve: the words set in verses down one column in the house type, with amber marking only where the voice is. The now-playing view's lyrics column is the sheet (`internal/ui/features/player/lyrics_sheet.go`, with the timing logic in `internal/ui/lyricsync`); Full-Screen Lyrics keeps its rules and lays the same lines out differently; the mini player shows the line being sung (`internal/ui/features/miniplayer/sungline.go`).

- **One sung line.** The sung line is the only line in `--ink` (timed by line only, it is `--amber-ink` itself); earlier lines are `--ink-faint`, later ones `--ink-muted`, plain lyrics `--ink`. Word-timed lyrics light each word `--amber-ink` as the voice reaches it. Short words light whole as they begin; held notes (600 ms or longer) fill in the reading direction, stepped about 12 times a second behind a soft 0.5em edge. Lines change colour without a transition.
- **Words move, letter by letter.** As the colour sweeps a word of word-timed lyrics, its letters lift with it (the motion Apple Music has and the players modelled on it share; the numbers are Apple Music-like Lyrics', `internal/ui/kit/media_wordmotion.go`). A letter starts to rise the moment the colour reaches it and is up 0.05em (0.10em in background vocals) 180 ms later, whatever the word's length, eased expo-out (most of the move at once, then a soft landing): a wave across the letters that keeps up with the voice, never creeping up behind a slow word's fill. The letters stay up while the line is sung and unsung words stand still. When the line stops being sung (it ends and the next begins, or the voice rests) they drop back from where they were, from the left, each over 220 ms with the same ease and starting 8 ms after the one before (120 ms at most in all). While a letter moves it is drawn at the fraction of a pixel it has reached, so the motion is smooth (a little soft, as text in motion is); at rest it is as sharp as ever. A word held for 1 s or more and of two to seven characters (CJK of any length) also swells, letter by letter, the letters starting in turn over the first 40% of the word's time: each swells to about 106% (at most 112%) about its own centre, drifts from the word's middle and up a little over its rise, and glows in `--amber-ink`, the last word of a line more strongly. Right-to-left words (their letters join) rise as one unit, and do not swell, and words that wrap do not swell.
- **Verses.** A pause of 3 s or more between one line's end and the next, or a change of the song part the lyrics name (Apple's "Verse", "Chorus"), starts a verse: a 32px gap in the column, 0.9em in Full-Screen Lyrics. While the voice rests in a break (from 0.5 s after the line's end) nothing is lit; in a break of 5 s or more the dots below stand in for the sung line.
- **Instrumental gaps.** Where no line is sung for 5 s or more (from one line's end to the next one's begin, or from the start of the song to a first line that begins 5 s or more in), three dots stand where a line would, in a line's height at the reading edge. They are sized from the font of the line they lead to, as a line of the lyrics is: 0.6em across and 0.25em apart (14px and 6px in the sheet, 43px and 18px at 1080px in Full-Screen Lyrics). Only line- and word-timed lyrics have them; plain lyrics have no timing. The dots are the sung line while the gap runs, from the end of the line before to the begin of the next on the lyrics clock. An unlit dot is the ink of a line still to come in that view (`--ink-muted`, the sheet's and Full-Screen Lyrics' alike), never brighter than the lines around it, and each fills with `--amber-ink` from the reading side over its own third of the gap (the first over the first third, the second over the second, the third over the last) as a word lights, stepped about 12 times a second like a held note, with no soft edge. Under reduced motion each dot lights whole as its third begins. While the gap runs the dots ride a slow wave in the reading direction, so the break reads as the music going on rather than a stall: each rises and falls 0.22 of its width on a 2.4 s period, a sixth of a period behind the dot before it, the wave growing in over its first 600 ms. In the last 520 ms before the next line the wave settles while the dots swell by 15% (180 ms, the `out` curve), then they gather into the middle dot's place and shrink away, gathering speed (eased in), gone the moment the line begins. They move by transforms, painted from the lyrics clock in frames that paint only the dots (as the playing row's equaliser does), and only while a song plays; paused they hold still, and a gap still to come stands still. Under reduced motion they stand still throughout and clear when the line begins. Once passed they are not drawn, but keep their place in the column so nothing moves. The sheet follows them as it does a line, and in Full-Screen Lyrics they take the sung place.
- **The sheet follows the voice line by line.** The sung line stays a third of the way down the sheet (`readAt`), and the lines glide past it as each one is sung, in the same cascade as full screen: the sung line and the lines before it first, the lines after it 60 ms later each (at most 240 ms), 600 ms each, exponential out, no bounce. A seek back glides them down, the lines above the sung one trailing. A line that begins while the lines are still gliding sets them on from where they are. A line without a gap leading it stands at the reading position; one with an instrumental gap before it leads with the dots there until it begins. A seek or a click that moves the sung line more than 6 lines (or more than a sheet's height), the first line, a song change and reduced motion place the lines without the cascade. The user's own scroll (wheel, touch or key) is left alone for 4 s, with nothing cascading; the next line after it glides the lines once to the reading position. Clicking a line (or Enter on it) seeks to it and takes the sheet back.
- **Edges.** The sheet fades out over its top and bottom edges (3rem), so a line leaving the view never shows clipped glyphs. It draws only while the lines glide.
- **Under each line**, smaller and in `--ink-muted`, each on its own line: the background vocals (600: they are sung, and they light on their own timing, so they can still be filling under the next line), then, when switched on in the view's bar, the romanisation (400, lit like the line when it is word-timed) and the translation (400) in the language chosen in Settings → Playback → Lyrics.
- **Duets**, as a lyric sheet sets them, from one reading edge: the second voice is indented 1.5em and set in italic; lines every voice sings stay at the margin, upright, the first of each such passage headed "Together" in meta type, `--ink-muted`. Never voices on opposite sides of the screen.
- **Full screen**: Full-Screen Lyrics, below.
- **The mini player** puts the sung line in the album's place, under the artist: up to two lines at 13/18 in `--ink` after the 12px lyrics glyph, its words lit whole in `--amber-ink` (no fill: a glance needs no frames). Before the first line, in breaks and for songs without synced lyrics it shows the album in `--ink-faint`. It keeps the main window's per-song timing.
- **Keyboard.** In Now Playing's column the lines are one Tab stop (the sung line, or the one focused last): ↑/↓ move, Home/End go to the ends, Enter or Space seeks there. Full-Screen Lyrics has its own keys, which go to it only while it is open (Full-Screen Lyrics, below).
- **Motion and cost.** Words and lines move in frames that only paint: the lines glide as poses and the words are painted from the clock (`kit.SweepText`; `docs/ui-architecture.md`, "Now Playing and lyrics (`features/player`, `lyricsync`)"), which asks for a frame only while a word fills, lifts, swells or settles, and sleeps on a timer for the next change once they have all come to rest. Under reduced motion or reduce effects every word lights whole as it begins, nothing lifts, swells or glows, and the lines jump. Full-Screen Lyrics moves by poses (below).

### Full-Screen Lyrics

The song's phrasing is the layout. It replaces the old full-screen sheet and keeps its entry points: Shift+L, and Full screen in Now Playing's lyrics tools. It is called Full-Screen Lyrics in menus, the palette and tooltips (`internal/ui/features/player/lyrics_stage.go`, `nowplaying_room.go`).

- **A column on the cover's wash.** A left column starts at 8% of the window's width and is 70% of it wide at most; the right third holds only the wash. The wash is `kit.Backdrop` and nothing over it: no box, card or glass. The view is dark in a light theme too (`theme.Scope` over the dark tokens, and the dark theme's scrim): it is read across a room and left on a TV, and a lit white screen is wrong there. Light and dark windows draw the same picture.
- **One size.** The sung line and the lines around it are all set at one size, 72 at 1080px of window height, scaling with the height (22 at the least): what tells the lines apart is the ink and the distance below, not their size. Weight is 700 (Schibsted Grotesk), lines 1.05 tall. A line wraps to two lines at most before its size steps down, through 58, 46 and 40 at 1080px; plain lyrics, which have no timing, are all 64.
- **Where lines stand.** The sung line's middle is at 45% of the height. One or two earlier lines stand above it in `--ink-faint`, the next two below in `--ink-muted`, a little smaller and fainter by their distance; beyond those nothing shows. In a short break nothing is lit, and the coming line waits in the middle in coming ink. In an instrumental gap the dots are in the middle: the line coming is the next place under them, the dots of a gap still to come take a place of their own, and the dots of a gap that has passed shrink and fade away as the column climbs and take none. Verse breaks and line gaps follow the sheet's rules (0.9em at a break); the second voice of a duet is indented 1.5em and italic, "Together" heads the first line sung by every voice; background vocals sit under their line at 0.42em, 600, muted, and the romanisation and translation, when switched on, under those. Words light `--amber-ink` as the voice reaches them and held notes fill, as in the sheet (the same `kit.SweepText`, painted from the clock).
- **Motion.** When the sung line advances every line glides to its new place on its own, in a cascade: the sung line and the lines before it set off together, the lines after it follow 60 ms later each (at most 4 steps, 240 ms), so the column unfolds as a wave and the gap between two lines only ever opens. Each glide is 600 ms, exponential out (`climb`), settling without a bounce, and carries the line's scale and fade into its new rank with it. On a seek back of a few lines the same happens downward: the lines below the sung one go with it, the ones above trail. Each line, and the dots of an instrumental gap, is its own pose, so frames that move them only paint. Text is shaped once per line and size, and nothing is laid out per frame. The view is built when the sung line changes, not once a frame. A seek of more than a few lines cuts instead of gliding. The wash drifts, a pose of a few percent of scale and slide over 40 s (`drift`) and back, which costs frames for as long as the view is open and the window shows. The drift stops under reduced motion and Reduce effects (which draw no wash), on battery and while the window is hidden (`MotionPolicy.Ambient`). Under reduced motion or Reduce effects lines jump into place, with no delay. At a song change the column fades out, the wash crossfades (the backdrop's 300 ms) and the new song's lines rise in.
- **Words.** The words lift and swell as in the sheet (Words move, above), at the stage's size: 0.05em is 3.6px at 1080px, and a swollen letter's glow reaches up to 0.24em.
- **The strip.** Hidden when the view opens. The pointer moving, or any key, brings it; it goes after 3 s without either while the song plays (and the pointer goes with it), and stays while paused, loading or failed. It is the verse block's foot: a 1px `--line` rule across the column's margins, then three parts on one line, the sides sharing what the middle leaves so the middle stays at the strip's centre at any width (1280 to 2560 and a narrow window alike).
  - **At the left, the song:** the 56px cover, the title (20/600) and artists (17, `--ink-muted`).
  - **In the middle, one control cluster:** the transport (previous, the amber play or pause disc and next: the player bar's own buttons, with its commands and its rules, so what a live station or a station of songs can't skip is as unavailable here as there), and under it the scrubber with the time elapsed at its start and the time left at its end (15, `--ink-muted`, tabular figures, a fixed width so the bar doesn't shift as they tick). The cluster is 44% of the window wide, 640px at most (scaled with the view). The transport and the thing it moves read as one control, never a transport floating apart from the bar.
  - **At the right,** the timing nudge and Exit full screen.
  - **The scrubber is a real seek slider**, the player bar's (`seekPad.slider` in `bar.go`, one implementation for both, each with its own drag state): drag to seek (the thumb and times show where it will land, and the song seeks once when the pointer lets go), click to jump, and ←/→ when it has the focus. The track is 3px, 5px under the pointer or the focus, with an amber fill (`--amber-ink` on the stage) and an ink thumb. A live station's is disabled, and so is a song that can't seek, with times shown as `–:––`.
  - It fades in and out. Focus is on the view itself, so its keys work; Tab goes into the strip once it shows, in that order (previous, play or pause, next, the scrubber, the timing nudge, Exit), and a strip that hides with the focus on it hands the focus back to the view.
- **States.** No lyrics (or an instrumental): the wash, the cover at 42% of the window's height in the column's place with the title and artist beside it, and "No lyrics for this song" in `--ink-muted`. Loading: the layout with flat bars where the lines go. A failed load is the usual error block with Retry.
- **Keys.** Escape and Shift+L leave; Space plays and pauses (the player's); ↑ and ↓ seek to the previous or next line (plain lyrics move the reading line instead); ← and → seek 5 s; Home goes to the start of the song; `[`, `]` and `\` nudge the timing. They are shortcuts in their own context (`lyrics-room`), so they show in the shortcut list and can be rebound. Media keys are the system's and work as they do everywhere.
- **A TV left on.** While the view is open and the music plays the screen is kept on (`Env.Window.KeepScreenOn`: MyGo's `Power.KeepAwake` with the display), beside the suspend inhibitor the player already holds. Nothing static and bright stays long: the strip hides and the wash drifts.

## Radio

Radio (`/radio`) answers what radio alone can: what is on air right now, and what follows. Apple's live stations open the page as a programme guide, the way a broadcaster prints its listings: a time ruler across the top, one lane per station, each programme a box set by its real start and end, and one line down every lane at now. The local broadcasters follow, as they are live too; then Apple's radio page in bands, with the stations to tune to before the shows to listen back to. The page hangs its headings in a rail at the left, as wide as the guide's station column, so the guide, the broadcasters and every band start their content on one vertical edge.

```text
┌──────────────────────────────────────────────────────────────┐
│ Radio                                                        │
│                     ┌10:15 AM                                │
│ Live now      10:00 AM   10:30 AM   11:00 AM   11:30 AM      │
│ ▣ Apple Music 1  ▶ │ [The Zane Lowe Show ][ tagline ][ Matt…]│
│ ▣ Hits           ▶ │ [Songs you know. 1 hr 45 min left][Club]│
│ ▣ Country        ▶ │ [ Country tagline                      ]│
│ ──────────────────────────────────────────────────────────── │
│ Local Broadcasters   ▣ KISS         ▣ Capital     (the shared│
│  See all ›                                         station list)
│ ──────────────────────────────────────────────────────────── │
│ Stations             Your stations · Top · by genre          │
│ Shows and episodes   episodes, interviews, shows, hosts      │
│ Browse               more to explore                         │
└──────────────────────────────────────────────────────────────┘
```

- **Live now: the guide.** The section's heading is at the bands' size and sits over the station column, level with the ruler. The local broadcasters' list follows inside the section. The board reaches 8px past the content on the left so the art lines up with the page's edge.
  - Each row is 72px: the **station column** (232px: the station's own art at 56px, its name at 14/500 over two lines at most, the play control at its end) and the **track** beside it.
  - The track shows a window of time that starts 30 minutes before now and is as long as the width allows at about 3.4px a minute, in half hours, between 90 minutes and five hours (120 at Sona's narrowest, 940 wide; 240 at 1440).
  - The ruler names every half hour in meta type with tabular figures (hours in `--ink-muted`, half hours `--ink-faint`), with a short tick under each. A flag in `--ink` with `--ground` text at now carries the clock time. A hairline in `--line` at every hour runs down behind the boxes.
  - **One line, 2px in `--ink`, runs from the flag down every lane over the boxes.** The window starts on the minute and the lines move only when the board reads the clock, every 30 seconds, never per frame.
- **Programme boxes.** A programme is a box 56px high at radius 6, placed by its start and end and clipped to the window. One that began before it is cut at the left edge and shows "‹ 09:15" there; one that goes on past the right edge shows a chevron. Back-to-back slots with one title are one box (a station's own hours, a show's consecutive hours).
  - **On air**: `--active`, the title at 600 and, when there is an end to count down to, "45 min left · 10:00 AM–11:00 AM" in meta (the slot only if it fits), with a 2px bar along its foot up to the now line. Its words go right of the line (left of it, when the show is nearly over and that side is wider) so the line never crosses them.
  - **A named show to come**: `--hover`, with its title at 14/500 and its slot.
  - **The station's own hours, and a programme that has ended inside the window**: a 1px `--line` outline with the tagline (or the description) at 13 muted. A slot named after the station shows its description, or the tagline when that is all it says.
  - The station's own hours run to the next named show and no hour goes unaccounted for. When they run to the end of the schedule there is nothing to count down to, and the box shows the tagline alone, with no bar.
  - A box too narrow for words keeps its name for assistive technology ("The Zane Lowe Show, 10:00–11:00").
  - Each box is a link to the station's page (not a Tab stop: the station column's link is the one stop). What comes next is the box after it, or, past the window, on the station's page. The schedules are the station pages' own queries, so opening a station is instant.
- **Play and state.**
  - The play control is always shown at the end of the station column (a 32px ghost circle): picking a station is the page's job, so it is not left to hover. The column takes `--hover` at radius 6 while the pointer is on it or a control in the row has focus.
  - The station playing takes amber where the guide shows it: its name in `--amber-ink`, the equaliser in place of the play control, and the bar on its programme in amber while it plays. Nothing else on the board is amber.
  - A station Sona can't play has no play control. Its lane is one outlined box that says why, with the `CircleSlash` icon, in place of its programmes. A live station with no schedule shows its tagline the same way.
  - Below 940 the guide keeps its columns and shows a shorter stretch of time (90 to 120 minutes); the bands stack their headings over their content.
- **Stations with nothing to open.** A station opens a page only when the page has something to show: a live station's schedule, an episode's show, the songs a track station plays first.
  - A local broadcaster has no schedule, so it has no page. Its row (the shared station list) is a button, "Play {name}", the name stretched over the row, and the menu is beside it (Play, Copy link). The play mark is a hover cue, not a second control.
  - While it plays, the name button pauses it ("Pause {name}"), the equaliser stands in the mark's place, and the second line is the song on air ("Artist – Title", from its stream) instead of the tagline.
  - A live station whose schedule turns out empty is treated the same, in the guide too: its whole lane is the play button.
  - Cover tiles do likewise (the tile is the button, named "{name} Play"). A link and a button are never mixed in one tile.
  - **One shape everywhere** (`kit.StationLabel`, `StationCue`, `OpenWithoutPage`):
    - One control, the name, named "{name} Play"; "{name} Pause" while the station plays.
    - A press pauses a playing station and resumes a paused one; it never starts it over.
    - The mark is a 16px play triangle (pause bars while it plays) in `--ink-muted`, shown on hover and focus, after the title on a wide card, a link tile and the lead; over the cover on a tile or row.
    - No second disc: the wide card, Home's lead pick and hero card, Home's rows and the Radio board lane have none for a station with no page.
    - Discover's lead and strip cards are left as they were ([#442]).
- **What is on air.** For a local broadcaster the player bar's live line, the bar's cover, Now Playing and MPRIS show the song its stream names, with the catalog's cover when Apple has that exact song, else the station's. Now Playing puts the song first (title, artist), then "On air · {station} · {genre} · {place}", TuneIn's description when it has one and an Open website button. A song change is not announced to screen readers (the bar's line is not a live region); the line is read where it is.
- **Controls that don't apply to a station.** Go says which controls apply (`controls` in the playback state) and every surface follows it. A live station is a stream; a station of songs (Hits Station, a song or artist station) is endless but made of real tracks. AutoPlay's own station is the user's queue and changes nothing.

  | Control | Song or album | Station of songs | Live station |
  | --- | --- | --- | --- |
  | Play / pause, volume | yes | yes | yes |
  | Next | at the end of the queue only with repeat all, a station or AutoPlay | yes (no limit, unlike Apple's) | no |
  | Previous | restarts after 3 s, else the song before | restarts the song only; nothing in its first 3 s | no |
  | Seek bar | yes | yes | no: the live line (dot, "Live", what is on air) |
  | Shuffle, Repeat | yes | no: "Not available while listening to a station" | no: "Not available while listening to a live station" |
  | AutoPlay toggle | yes | no (as above) | no (as above) |
  | Play next / Play last | yes | no (as above) | no (as above) |

  Each surface keeps its layout and says why, so nothing disappears quietly:

  - **The player bar's controls** stay in place at 40%, disabled in look but still focusable, with the reason as the tooltip's second line and the accessible description. A press does nothing. Shuffle and Repeat read off whatever they were set to.
  - **The mini player's buttons and the queue's AutoPlay toggle** do the same (the reason as title and description).
  - **Menus** keep Play next and Add to queue, greyed, with the reason under the label in `--ink-faint`.
  - **Keys** (Ctrl+←/→, Shift+←/→, S, R) take the chord, do nothing and say the reason once in the notifications region (a toast, not repeated while the key is held). The command palette leaves those commands out.
  - **MPRIS** reports `Shuffle` false, `LoopStatus` None and refuses writes to either. On a live station `CanGoNext`, `CanGoPrevious` and `CanSeek` are false too.
  - **Now Playing** has no transport of its own, so it has nothing to switch off.
- **Bands.** After the live part the page's shelves go by what they hold: Stations (the user's own first, Apple's top and genre stations), Shows and episodes (episodes, interviews, sessions, DJ mixes, shows and hosts), Browse (the links). Apple's order holds inside each; the band line and heading are Discover's. Apple's own rows of the live stations are left out while the board lists them, and kept when Sona couldn't list them.
- **Signed out.** The Stations band opens with a "Your stations" block saying what signing in adds, with Sign in.
- **Loading and errors.** The guide's frame (the ruler's heading, six lanes at their height, the art and the programmes as bars) and the broadcasters' and first band's rule, rail heading and shelves stand where they will land. A block that fails says so in place, beside its rail heading; when nothing loads, the page says it once, with one Retry.
- **The station page** (`/station/{id}`) puts the station's identity in a column of its own that stays in view as the lists scroll.
  - The column holds the cover in two size classes by the content's width (264px, 320px from 1500px of content, so Go serves exact boxes of two sizes, not one per window width), the name at 32/36, the byline (the show, the host, the tagline, or what a track station was made from), the catalogue line, the notes, the Play disc and the menu.
  - Beside it the lists take all the width left, up to 1100px. From 1500px of content the two columns (320 + 48 + 1100) always fit; past 1100px they stay together at the page's left edge.
  - The lists are the note of a station Sona can't play, a broadcaster's "On air", the schedule, and the songs a track station plays first, each under a ruled head (a rule across the column above all but the first).
  - Under 800px of content the identity stacks over them with a 112px cover beside its lines.
  - **The schedule is a vertical timeline.** Each programme that has not ended is a row: a start time (14/13 tabular) beside a 28px rail and its box, in the guide's look (the programme on air `--active`, a named show `--hover`, the station's own hours outlined). The box holds the art at 48 (requested at exactly 48), the title, the description at two lines and, on air, the time left.
  - The rail is a 1px `--line` with a hollow dot at each start. For the programme on air it is inked from its start to how far through it is, ending in a larger dot (amber while Sona plays this station), and reads as its progress.
  - Back-to-back slots with one title are one row, so a station's own hours are not a list of identical rows. The schedule is read every 30 seconds like the guide.

## Link lists

Apple ends its catalogue pages with lists of links, "More to Explore": hubs (Browse by Genre, Decades, Charts), then every genre. They are the page's way on, so they are controls, not a paragraph of text (`internal/ui/kit/collections_items_linklist.go`, one component for every page that shows them: Discover and Radio today, Home and the editorial pages whenever Apple sends one).

```text
More to Explore (16/600)
┌──────────────┐ ┌──────────────┐ ┌──────────────┐ ┌──────────────┐
│ Concerts   ↗ │ │ Decades      │ │ Worldwide    │ │ Music Videos │   the hubs: eight, two rows of four
│ Browse by G… │ │ Moods and A… │ │ Charts       │ │ Spatial Audio│
└──────────────┘ └──────────────┘ └──────────────┘ └──────────────┘
  (24px)
┌──────────────┐ ┌──────────────┐ ┌──────────────┐ ┌──────────────┐ ┌──────────────┐
│ African Music│ │ Classic Rock │ │ Indie        │ │ Música Tropi…│ │ Soul/Funk    │   the genres: Apple's
│ Americana    │ │ Country      │ │ Jazz         │ │ Oldies       │ │ Urbano Latino│   untitled continuation
└──────────────┘ └──────────────┘ └──────────────┘ └──────────────┘ └──────────────┘
```

- **A button each.**
  - 44px at least, a `--hover` fill at radius 6 (`--active` on hover and press: the same fill every row of Sona takes), the name at 14/500 in ink, two lines before it is cut.
  - The one glyph is the 16px arrow out in `--ink-muted` at the end of a link that opens in the browser (its name says "Opens in your browser"). A page in Sona needs none, being what a button is.
  - Nothing else is added: Apple sends a link's label and URL and nothing more, so a colour or an icon per tile would be Sona's invention, and amber means playing.
  - In light the tile also has a 1px `--line` border, the one place a fill and a line go together: `--hover` is only about 1.1:1 against the ground and the line is what makes the tile read as a control. In dark the fill is clear alone and the border is transparent (it keeps the box the same size).
  - Focus is the usual 2px ring outside the tile.
- **Columns.** As many as hold a button of 184px, 8px apart, filled top to bottom in Apple's order (`itemsLinkGrid`): the genres are alphabetical, so they read down a column, and Tab follows the same path. A short list keeps the long list's tracks, so a button is one width on the whole page and the lists line up. The columns fill so the last holds at least half of one: 21 genres at five columns set 6, 6, 6 and 3, never 5, 5, 5, 5 and a lone button.
- **Continuation.** Apple splits a long list in two; the second has no title and sits 24px under the first, a clear step from the 8px between buttons. It is its own region, "More links". Every link is a Tab stop, in reading order: they are navigation, not a list to arrow through.

## Concerts

Concerts (`/concerts`) answers one question: what is on near me, and when. A gig is a date, a place and a ticket, so the page is set like a sheet of tickets: each day is a stub (its weekday over a big number), with that day's shows beside it. It is read down by date, set by the venue's own calendar, never a grid of covers.

```text
┌─────────────────────────────────────────────────────────────┐
│ Concerts (⌖ Set location)                        32/700      │
│ Near London, the main city of your… · Updated 5 min ago      │
│ [Any date ▾] [All genres ▾] ◯ Your artists (3)               │
│ Fri  Sat  Sun  Mon  Tue  Wed  Thu  Fri  …  14 days           │
│  9   10   11   12   13  [14]  15   16                        │
│                         ••                                   │
│ April 2027 · 3 shows                                24/650   │
│ Wed │ 6:00 PM (●) Example Band with Opener   [Get tickets ↗] │
│ 14  │           Example Hall · London  6 mi away              │
│     │ 8:30 PM (●) Sample Singer              [Get tickets ▾] │
│ Wed │ 8:00 PM (●) Late Show                  [Get tickets ↗] │
│ 21  │                                                        │
└─────────────────────────────────────────────────────────────┘
```

- **Header.** The title at 32/700 with the location button beside it (an outline button: the control sits with what it sets, not at the far edge), and under them one line at 13 muted saying where the list is near and how old it is. Filters follow in one row.
- **The coming days.** Under the filters, the next 14 days (from today, or from the first day of a date range that starts later, to its last day) as one row of day cells.
  - A cell is the weekday at 12 over the day number at 16/600, tabular, the cells sharing the width at 72 at most.
  - A day with shows is a button (a `--hover` fill on hover, the number in ink at weight 600) named "Wed, Apr 14, 2 shows", with one to three 4px ink dots under it for how many (not amber). A day without is `--ink-faint` at weight 400 and takes no click.
  - A button scrolls the agenda so that the day is at the top and focuses the day's first control. The cells are one Tab stop with the arrows moving along them.
  - The strip is left out when no day of it has a show or when the range is under a week (a weekend), and it counts only the shows the filters leave.
- **The agenda.** Months under a 24/650 heading with their count in 13 muted (`April 2027 · 2 shows`), then days. The last month's count is left out while Apple has more shows than the list asked for, as it may be short.
  - A day is a stub: the weekday at 12 muted over the day number at 32/700, tabular, in a 64px column, a 1px `--line` rule the height of the day, and that day's shows as rows to its right, so the rule's length says how busy the day is. Days are 8px apart, months 48px.
  - The day, month and clock are read in the venue's own time zone, in the interface's language. A show whose clock is not this computer's carries its zone's name ("8:00 pm EDT"), so a gig in another country is never read as local.
  - Shows are Apple's, soonest first; there is no popularity ordering to explain.
- **A row** is 72px and reads left to right as a timetable.
  - The start at the venue's clock at 14/500 (the first edge of every row), the headliner's portrait (56px, round like every artist), the line-up at 16/600 (a link to the artist) over the venue and city at 13 muted, with the straight-line distance from the chosen city at 12 faint beside it (`6 mi away`, miles where the locale uses them), and the one action at the row's end. A show by an artist the user follows says so on a third line.
  - **Get tickets** is an outline button (the same `--control-line` as the filters). Apple lists the sellers: one seller opens its https page in the browser (a trailing arrow), several are a menu of them (a chevron), and a show with none shows no button.
  - The row's `--hover` fill is at radius 6, as track rows.
- **Where.** The header's outline button names the chosen city ("Manchester"), or says "Set location" while the page is on the default, and the line under the title says what the default is (the storefront's main city).
  - It opens one small dialog: a search field and Sona's own list of cities (the storefront's first, then well-known ones). Each row has the city at 14/500 at the left and its country at 13 muted at the right, with a check in a slot kept on every row for the chosen one.
  - A sentence says the choice stays on this computer and Apple Music is told only the area. "Use my storefront's main city" undoes a choice.
  - Keys: ↓ moves from the field into the list, Enter takes the first match, Escape closes and returns focus to the button.
- **Filters** are one row of 32px controls: dates (any date, this week, this weekend, next 30 days, or two date fields), genre, and a switch "Your artists (n)" that appears only when the signed-in account has shows in this area. They live in the address (`when`, `from`, `to`, `genre`, `yours`), so Back returns to them. A change keeps the list on screen until the new one arrives.
- **Long lists.** Asked 100 at a time (Apple has no paging, so "Show more concerts" asks for 100 more, up to 500). The agenda is one Tab stop: ↑/↓ move between rows, ←/→ between a row's link and its tickets, Home/End jump, Tab leaves. What the page builds from a listing (the groups, the strip, each row's words) is made once per listing, not per frame.
- **States.** Loading draws the agenda's own frame: a month heading and three stubs with their rules and rows at their height (the strip waits for the data, as it depends on it); no concerts near the city (some countries have none) says so with Apple's "No concerts" and "Set location"; filters that match nothing say that with "Clear filters"; a failed load is the usual error block with Retry, and a stale copy says when it is from.
- **Elsewhere.** An artist's page carries "Upcoming concerts" between its lead and its discography: three rows with a compact date column and a 1px rule first (weekday, day number at 24/650, month; the artist's portrait and name are left out, the venue leads), the ones near the chosen city first, "Show all n", and a link to the agenda. It is left out when the artist has none. Home carries "Concerts in {city}" where Apple puts its concerts shelf: six such rows in two columns, with portraits and ghost ticket buttons, and "See all". Their start and distance sit under the venue, as the columns are too narrow for the agenda's timetable. Apple's own concerts links open this page.
- No amber: nothing here is playing.

## New releases

New releases (`/new-releases`) is the artist tracker's list (`docs/architecture.md`, "Artist tracker"): what the artists the user has favourited have put out, newest first. It answers one question, what is new from my artists, so it is a plain list of releases read down, each a cover and two lines, with nothing of Apple's shelves in it. It is Operate, not Persuade: no hero, no tiles, no colour beyond the covers.

```text
┌──────────────────────────────────────────────────────────────┐
│ New releases  [✓ Mark all as read] [🗑 Clear list]   32/700   │
│ From the artists you've favourited · Checked 3 hr. ago        │
│ Coming soon 1                                      16/600     │
│ ●  ▢  Winter Rooms                              Out Nov 27   │
│       Hollow Pines · Album                                    │
│ This week 2                                                   │
│ ●  ▢  Night Signals                                 Nov 14   │
│       Mia Vale · EP                                           │
│    ▢  Paper Boats                                   Nov 13   │
│       Hollow Pines · Single                                   │
│ Earlier 2                                                     │
└──────────────────────────────────────────────────────────────┘
```

- **Header.** The title at 32/700 with two outline buttons beside it (the control sits with what it acts on, as Concerts' location button does): *Mark all as read* (greyed with nothing unread) and *Clear list*. Under them one line at 13 muted: what the list is and how old the last check is ("Checked 3 hr. ago", `i18n.RelativeTime`), or "Not checked yet". The buttons are left out of an empty list.
- **Groups.** *Coming soon* (releases not out yet, soonest first), *This week* (out in the last seven days) and *Earlier*, each under a 16/600 heading with its count in `--ink-faint`, 32px apart; a group with nothing in it is not drawn. Each group is a table for assistive technology, named by its heading.
- **A row** is 64px: a 12px slot for the unread dot, the 48px cover (radius 4 with the 6% edge line), the title at 14 and the artist and kind ("Mia Vale · EP", "Hollow Pines · Album", "… · Single") at 13 muted under it, and the date right-aligned in a 104px column at 13 muted tabular ("Out Nov 27" while to come, "Nov 14" once out, the year alone when Apple gives no day). The " - EP" and " - Single" suffixes come off the title, the kind says it.
  - **Unread is said twice.** An 8px `--ink` dot in the slot and the title at weight 600 against 400 once read. The dot is ink, never amber: nothing is playing, so the spot colour stays out. The row's description starts "Unread" for assistive technology.
  - The hover fill is `--hover` at radius 6 as other lists; a selection takes ink at 12%. The title underlines on hover.
  - The row's end has two 24px ghost buttons: *More* (the item menu, shown while the row is hovered or holds focus) and *Remove*, a cross shown under the pointer only. Under the keyboard a row is one Tab stop; its remove is the menu's *Remove from New releases* and the Delete key.
  - **Opening a release marks it read** and goes to the album. A selection of rows, drag and the item menu (right-click, Shift+F10, the Menu key) are the albums' own. The menu adds *Mark as read* (unread rows only) and *Remove from New releases* after the library's actions, only in this list; an upcoming release carries the album's *Remind me*.
- **Sidebar.** The link sits after Concerts in the main section, with a calendar icon. It is there only while the tracker is on (Settings → General); with it off the sidebar has no link, and the page, reached from a notification, shows the off state. Its unread count is a figure at the link's end at 12/600 tabular in `--ink`: no badge shape, no colour, no animation, and nothing at 0. The count is part of the link's name ("New releases, 3 new").
- **States.** Off (the tracker is a setting, off by default): the empty frame's title *Release tracking is off*, one sentence naming the setting and *Open Settings*. On with nothing yet: *Nothing new yet* and one sentence saying it looks about once a day. Loading draws the heading and three rows' frame in flat bars. A failed load is the usual error block with Retry. A list found before the tracker was turned off stays readable under "Release tracking is off".
- No amber on the page.

## Video view

A music video has a page (`/music-video/{id}`, `internal/ui/features/video/page.go`) and a full-window view (`view.go`).

### The page

The page is set like a screening.

- **The stage** spans the whole content column at every width. The 16:9 picture fills it while it is no taller than 60% of the window's visible height (clamped to 360 to 640). Past that the stage keeps full width at that height, with the whole picture centred on the `surface` token like a cinema screen (never cropped). The picture is requested from Go at exactly its whole-DIP box. The cover is the colour, so there is no wash behind it.
- **The caption row** sits directly under the stage, at the column's width.
  - At the left: the title at 32/36 with the artist links, the catalogue line (`Music video · 2019 · 3:35 · Pop`) and the Explicit, 4K and HDR tags on one line.
  - At the right: Play (the amber disc), Watch in Apple Music, Play the song and the item menu. They stack under the title block when the column is under 720 wide.
- **Below the caption**: the note on standard definition and the song line in secondary type, then Notes, then "More by {artist}" and "More like this" as rows of 16:9 tiles from 296 wide (two or three across, "See all" only with an artist page), and the release date.
- **Loading** draws the picture, the caption bars and a row of tiles at their real sizes.

### The full-window view

A music video takes the whole window, like full-screen lyrics: the picture is the page (`view.go`). It opens when a video starts, and again from the bar's cover or "Show video", Now Playing's "Show video" or V. Closing it leaves the video playing in the bar.

- **Strips.**
  - A 48px button strip at the top holds only Full screen and Close at the right. It keeps the window's buttons clear and drags the window.
  - The foot has the seek bar across the full width (elapsed time flush left, time left flush right), then one row.
  - In that row the caption is at the left on the stage's ground, never over the picture. It is the video's name at heading size (a link to its page, with the SD badge), the artist under it in `--ink-muted` (a link), and the status line in meta type: "Loading the video…" before the first picture, "Buffering…" when pictures stop.
  - Previous, the 44px amber Play/Pause disc and Next sit in the exact middle, and the bar's volume control at the right. The caption and the volume share the leftover width, so the transport stays centred whatever the title's length.
  - Tab follows reading order: the strip's buttons, the seek bar, the title and artist, the transport, the volume.
- **The volume** at the right of the foot scrolls like the bar's (the mute button and the slider, wheel or swipe; see the player bar above), and the wheel brings the faded controls back.
- **The picture** keeps its own shape inside the stage, black behind the picture only, never behind the strips; the stage is the theme's `--ground`, light or dark. Before the first picture the video's artwork stands in at 16:9.
- **Fading.** While a picture plays, the strips fade after 3 s with no pointer or key (and the pointer hides); any movement or key brings them back. They stay while paused, loading, failed, or while a control has keyboard focus. Reduced motion makes the fade instant.
- **SD badge.** The house `Tag` reading "SD", its title explaining that Apple allows HD only on players with hardware protection. Shown while the video plays, hidden in an error state.
- **Error states** replace the picture with the usual dashed error block: what failed ("Couldn't play this video", or "Music videos don't play here" where Sona can't), the reason in Sona's words, "Watch in Apple Music", and "Try again" unless trying again can't help (Apple offers the video only in HD, or refused the licence). The bar and the mini player say "Couldn't play this video." with the reason on hover.
- **Keys:** Space or K play and pause, ← and → seek 5 s, F toggles the window's full screen, M mutes, Escape leaves full screen then closes, V closes.

## Settings

Settings is a document with marginalia: the left column says what each section is set to before you open it, and the section is a ledger of rows on the plain board whose group headings hang in a left margin beside their rows, not a stack of boxes.

```text
Settings (h1)                  Playback (h2 at 24/650)
[ Find a setting       ]       ═════════════════════════════════════ (1px ink)
⚙ General                                    ─────────────────────────────────
  Opens on For You · …                       Widevine                ✓ 4.10…
▶ Playback        ← --active   Sound         ─────────────────────────────────
  High quality · …             (16/600)      Audio quality          [High ▾]
☾ Appearance                                 AAC at 256 kbps, or 64 kbps…
  Light theme                                ─────────────────────────────────
…                              Listening     Private listening          (o )
```

- **The index** (240px) is held in view while a long section scrolls, where the window is at least 700px tall.
  - It holds the h1, Find a setting, then the sections. Each section is a 16px Lucide icon (the icon takes the name's colour) beside its name at 14/500, over its state in one catalogue line at 12 muted (`High quality · Crossfade 6 s`, `Nothing connected`, `2 changed`), at most two lines.
  - Only cheap data feeds the lines (settings, capabilities, the scrobblers' status). Diagnostics, whose checks cost a run, says what it holds. The current section takes `--active`.
  - Below 960px the h1 and the field share a row, the sections wrap under them as icon and name alone, and the h2 is visually hidden.

  | Section | Icon (Lucide) |
  | --- | --- |
  | General | `SlidersVertical` |
  | Playback | `Play` |
  | Appearance | `Moon` |
  | Connections | `Link2` |
  | Add-ons | `SquareTerminal` |
  | Shortcuts | `Keyboard` |
  | Storage | `FolderClosed` |
  | Diagnostics | `BadgeCheck` |

- **Find a setting** searches every row's title, its group, its section and a few other words for it (`dark` finds Theme), and every shortcut. While it holds a query the section list gives way to the results (title at 14 in ink, `Section · Group` in meta); choosing one opens the section and puts focus on the row's control, washing the row in `--hover` for two seconds. ↓ goes into the results, ↑/↓ and Home/End move, Enter in the field takes the first, Escape clears; the count is announced.
- **The ledger, with marginalia**: the section's h2 at 24/650 over one 1px `--ink` rule, then groups 32px apart.
  - Wider than 960px each group's h3 (16/600) and its one muted line (13, in the 176px column) sit in a left margin, level with the first row's title. The rows run in the column to its right, 32px on. A block with no heading of its own (Language, Widevine, Appearance) keeps to that same column with its margin empty, so every row of a section shares one left edge. At 960px and below the h3 stacks above its rows.
  - Every row sits under one 1px `--line` (no box, no tone step): the title at 14/500, what it does at 13 muted (60ch), the control at the right, 40px clear of the text. A row's state that needs attention keeps its own line under the description.
  - The content column is up to 960px.
- **New releases** (General) holds two switches under their own group heading: *Track new releases from favourite artists* and *Also show releases as desktop notifications*. Releases (a reminder that came due, what the tracker found) are in the notification centre either way; the second adds desktop notifications for them, with or without the tracker, and, like every row the platform can't back, keeps its place with the reason where the platform has no notifications.
- **What the platform can't do** keeps its row: the title in `--ink-muted` and, where the control would be described, a `CircleSlash` and why ("Not available on macOS yet.", or the specific reason: no tray to keep playing from, a player that reports plays itself). Nothing disappears quietly. Without any playback, Playback is that one row and the lyrics.
- **Theme** is three segments (System, Light, Dark) in one 32px outlined control, radio buttons underneath: every choice in view, ←/→ to move.
- **Add-ons** sits after Connections. It is a roster rather than a document: one ledger row per add-on, in two groups, so ten add-ons read as ten rows.
  - **Sona's Own** (Last.fm, ListenBrainz, Discord) comes first, as status only: no switch here, and a muted *Connections ›* link at the right, since their accounts and switches are there. **Installed** follows in the order of the add-ons' names, its margin line saying that an add-on runs in a sandbox with only what you approve. Its last row is *Add-ons folder* with *Open add-ons folder* and *Look again*. With none installed, the group holds the dashed empty block.
  - **A row** is a disclosure:
    - a 14 DIP chevron that turns to point down while open, and the add-on's name at 14/500;
    - under it, a 7 DIP state dot and the state in words at 13 muted, with how it runs after a middle dot ("Running · Sandboxed", "Off · WebAssembly", "Needs your approval · Sandboxed · Internet", "Running · Not isolated" for one of Sona's own that runs without the sandbox);
    - when the words don't say what to do, the reason under them at 13 muted.

    The dot is `--positive` while it runs or waits for work, `--danger` when it needs you (can't run, keeps stopping, switched off, waiting for an approval) and `--ink-faint` when off. Colour is never the only sign, because the word is always there. The installed add-on's switch, or Sona's own link, sits at the right, level with the name. A refused add-on whose manifest gave nothing to show is plain text, not a disclosure: its row already says all there is. A change of state is announced (polite) as the add-on's name and its word.
  - **Open, one at a time**, the row and its details are one block on `--raised` at radius 8. The block reaches 12 DIP past the text column on both sides, as a row's hover wash does, so the wash and the details share their edges. Opening a row scrolls it into view, since closing the one above can move it. Each part is named at 13/600 in a 104 DIP column, and the parts are set apart by space alone (the block is already a tone step, so no rules):
    - Isolation: one of Sona's own running without the sandbox, and why;
    - About: the version and description;
    - Permissions: each name at 14/500 over what it allows, then *Not approved* or a ghost *Revoke* (Sona's own have none);
    - Settings: the add-on's own rows (a title works its control, as a ledger row's does);
    - Log.
  - **Turning one on never saves anything by itself.** The switch opens a panel under the row on `--raised` with a 1px `--control-line` border: each permission, the warning, *Turn on* (the one filled button) and *Cancel*. The panel is announced as a group; Escape cancels.
  - A secret field is a password field that shows *Saved* as its placeholder and never the value. A refused add-on has no switch: its row says *Won't run* and the reason.
  - Every text the add-on supplies is plain, cut and never in ink-on-ink emphasis: the add-on's name is the row's title, never a badge or a picture. The log is selectable text in a scrolling `--raised` box.
  - The section's line in the index counts the installed add-ons and how many need you ("3 installed · 1 needs you"), or how many are on.
- **Storage** opens with a group, *On this computer*, whose row holds what Sona keeps as three figures at 24/650 with tabular figures: the artwork cache against its limit with a 4px meter in `--ink-muted`, the catalogue cache, the database. The group *Limit and cleanup* keeps only what each does and its action, so no figure is shown twice.
- **Shortcuts** set their groups in two columns where the section is at least 46rem wide, each group whole in one column; rows are the ledger's at 44px. The two columns use the section's full width, so their h3s stay above their rows instead of in a margin.
- **Diagnostics** leads with its verdict at 16/600 beside its icon ("Everything Sona needs is working", or the count of problems), the warnings counted under it, as the first margin note; its three actions are the first row beside it. The checks follow as groups in the margin frame, ledger rows of a 200px name column and the value. Variable names break after an underscore. The last group, Developer Tools, holds two ordinary setting rows: the **Developer tools** switch (off by default; it adds Inspect to every item's menu) and Open the inspector.
- No amber anywhere on the page: it has nothing playing to show.

## Item inspector

An Operate surface (Settings → Diagnostics → Developer tools, then Inspect in an item's menu): dense, precise, in the house style, with no amber. A bare dialog 960 wide on the ground: a header (56 artwork, name 18/600, "Kind · subtitle", the IDs as 24px bordered chips), a line-variant tab strip, one fixed-height body, a foot, each divided by a 1px `--line`.

- **No monospace.** The grotesque stays; numbers use tabular figures, and structure is shown by indentation guides (1px `--line` columns, 16 DIPs a level) and arrows.
- **JSON colours** reuse existing text tokens and add none: key `--ink-muted`, string `--ink`, number `--positive`, boolean `--danger`, null `--ink-faint`. They are plain hues here, not statuses; the quotes and the word "null" carry the type as well.
- The ledger is label (13, `--ink-muted`, 200px) and value (`--ink`, selectable) rows; a change of group is a wider gap, not a rule or a card.
- A tab with no data is dimmed with a tooltip that says why. Loading is the same layout in flat `--hover` bars; an error sits in the tab with Retry.

## Command palette and shortcut sheet

Both are launcher surfaces: raised panels over the scrim, one field at the top, one list, a foot that names the keys. Nothing in them is amber.

- **The palette follows the rule: choosing opens, Play plays.** A library song opens its song page and Shift+Enter plays it; an upload, which has no page, plays. The active row ends in the glyph that says which (a chevron opens, a play triangle plays; a command shows its keys), and the foot names Shift+Enter only while the active entry is a song with a page.
- **The palette's rows** are 36px at least with a 32px tile at the left: the command's icon at 16 in `--ink-muted` on `--surface`, radius 6, the same width as a library item's cover so a result list keeps one leading column. The label is 14/400 in ink and the keycaps end the row. A ranked command carries its group ("Playback") as a 13px muted second line, since no heading says it. With nothing typed the places to go (Pages) come first, then Playback, View, Navigation and General, so Enter on opening goes somewhere rather than pausing the music.
- **The sheet** is 820px wide, 10% from the top. The title at 16/600 and a 268px filter field share the head; a hairline, then the groups in two columns. A group is its title at 14/600 over a 1px `--line`, then rows of 32px: the label in ink, and at the right the keycaps (or "Not set" at 12 in `--ink-faint`) in a column whose width is the group's widest chord, so the caps of a group start on one edge. The foot, under another hairline, carries "Change any of them in Settings." at the left and ↓ Browse and Esc Close at the right, at 12 in `--ink-faint`.
- **Filtering** narrows by words in the label, keywords, group or keys; empty groups leave with their rows, and a filter that leaves nothing says "No shortcuts match" with one line of advice. The panel keeps its place and a minimum height while the list shortens.
