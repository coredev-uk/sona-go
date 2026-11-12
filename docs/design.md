# Sona design direction

This is the working visual direction. The tokens below live in `internal/ui/theme/tokens.json`; change them there and here together.

## Thesis: the label's house style

A good record label sets every release through one fixed template: the same typeface at the same station, the same catalogue line, a quiet board, and the cover as the only picture. Sona works the same way. The chrome is a neutral, faintly green-grey board set in one grotesque. The colour on screen comes from the music: cover art, and backdrops pre-blurred from it in Go. The interface itself owns one spot colour, amber, and spends it on one job only: showing where the music is.

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
| Page title (h1)  | 24 / 30, -0.015em | 650     | For you, Songs, Settings                   |
| Section          | 16 / 22           | 600     | Shelf titles, settings section titles      |
| Body             | 14 / 20           | 400–500 | Nav, row titles, form labels               |
| Secondary        | 13 / 18           | 400     | Artist and album columns, descriptions     |
| Meta             | 12 / 16           | 450     | The catalogue line, durations, timestamps  |

- Six sizes, three weights in practice (400, 600, 700). No uppercase labels, no letter-spaced small caps, no monospace.
- Every number that updates or aligns (durations, track numbers, counts, progress) uses tabular figures (`FontFeatures("tnum")`).
- **The catalogue line**: every collection header and the now-playing bar carry one meta line in the same form: `Album · 2008 · 10 songs, 42 min`. Middle dots, sentence case, tabular figures. It is the label's catalogue number and appears in exactly this form everywhere.

## Colour

Strategy: restrained chrome, colour from artwork, one spot colour with one job. All text tiers reach 4.5:1 on the surface they sit on. Token names are written with a `--` prefix on this page; in code they are `theme.C("ink")` and the like, from `tokens.json`.

| Token            | Light     | Dark      | Role                                                 |
| ---------------- | --------- | --------- | ---------------------------------------------------- |
| `--ground`       | `#f4f5f3` | `#121513` | Content canvas                                       |
| `--surface`      | `#eceeeb` | `#171a18` | Sidebar, top bar, now-playing bar                    |
| `--raised`       | `#ffffff` | `#1e2220` | Menus, dialogs, popovers, inputs                     |
| `--hover`        | `#e4e7e3` | `#222724` | Row and nav hover                                    |
| `--active`       | `#dbdfda` | `#2a302c` | Selected nav item, pressed                           |
| `--line`         | `#dadcd7` | `#262b28` | Structural 1px lines                                 |
| `--control-line` | `#838984` | `#6b736d` | Input and outline-button borders (≥ 3:1)             |
| `--ink`          | `#161917` | `#eef1ee` | Primary text                                         |
| `--ink-muted`    | `#545a55` | `#a9b0aa` | Secondary text                                       |
| `--ink-faint`    | `#5d635e` | `#959c96` | Meta, placeholders                                   |
| `--amber`        | `#e5a31b` | `#f0b43c` | Spot fill: play button, progress, now-playing marker |
| `--amber-ink`    | `#845500` | `#f0b43c` | Amber as text or icon on the ground                  |
| `--on-amber`     | `#1a1405` | `#1a1405` | Glyphs on an amber fill                              |
| `--danger`       | `#b42318` | `#f0705f` | Errors and destructive actions only                  |
| `--positive`     | `#1f6e47` | `#5cc48c` | Success states only                                  |

- The faint green-grey tint (hue ~150) keeps the board from reading as dead grey and sits well next to any cover. No cream, no navy, no violet: that is Sona's own themes' rule, not the community palettes' (Themes, below).
- The primary button is ink-filled (near-black on light, near-white on dark), not amber.
- Focus ring: 2px `--ink` at 60% (the `ring` token, MyGo's keyboard-focus ring) outside the control, everywhere (≥ 3:1 on every background tone). Selection is amber at 32% (the `selection` token); the caret is `--amber-ink`.
- Contrast floors (WCAG AA, `docs/accessibility.md`): text tokens ≥ 4.5:1 on every background up to `--active` (selected rows), `--control-line` and the focus ring ≥ 3:1. Slider thumbs are always shown: the thumb, not the fill colour, marks the value.
- The kit's role names (`background`, `primary`, `muted`, `ring`, …) are tokens in `tokens.json` too, mapped onto these, so MyGo's widgets and Sona's components read one palette without per-component overrides.

### Themes

The table above is Sona's own theme (`sona`): `tokens.json` is its whole document, and every other theme is a set of overrides of it (`internal/ui/theme/themes/*.json`, and the user's own files, `docs/themes.md`). A token a theme leaves out comes from Sona's own, so a theme never shows a missing colour. Every light/dark difference is a token (`scrim`, `input-bg`, `switch-off`, `select-hover`, `badge-destructive`, `link-tile-border`, `dialog-border`…); the kit has no `theme.Dark()` branch for a colour.

| Theme | Looks | Spot colour |
| --- | --- | --- |
| `sona` | The default: the table above, light and dark. | amber |
| `graphite` | A cooler neutral board, light and dark. | sky blue |
| `paper` | A warm neutral board, grey and not cream, light and dark. | teal |
| `contrast` | White or black board, strong lines and focus ring, every text colour at 7:1, light and dark. | blue (light), yellow (dark) |
| `oled` | Dark only. A `#000000` ground, with surface and raised just above it to separate menus. | lime |
| `catppuccin`, `nord`, `gruvbox`, `solarized`, `rose-pine`, `tokyo-night` | The community palettes, each with a light and a dark variant (Latte/Mocha, Snow Storm/Polar Night, light/dark, light/dark, Dawn/main, Day/Night). | peach, frost, yellow, yellow, gold, orange |
| `dracula` | The community palette, dark only. | pink |

- **Sona's own themes keep the brief.** The chrome is a neutral board (no tone of it strays more than a few levels from grey; a test holds the five to it), the cover is the colour, and there is no cream, navy or violet. The community presets are their projects' palettes mapped onto Sona's tokens: their own hues win, violet and navy included, and each credits its project in `author`. Where an exact palette value fails the floors below (text tiers, the control line, the spot floors), that token is moved in lightness only, and the theme file's `$comment` lists each change.
- **A theme may change the spot colour** (`amber`, `amber-ink`, `on-amber`; `play`, `play-ink` and `on-play` follow). It still means "playing" and nothing else: a theme whose `primary`, `ring` (the focus ring), `active` or `hover` (navigation selection and hover) or any other role of the kit resolves to it is refused. The cover's accent on the album page replaces it as before; `palette` reads its six colours (ground, hover, active, ink, spot, on-spot) from the current theme.
- **A theme that fails is refused, not adjusted.** `theme.Validate` is the one rule set, for the built-ins' tests and for a user's file at run time: text tiers, the spot colour as text and the status colours keep 4.5:1 on every tone up to `--active` and on a selected row (ink at 12%); `--control-line` and the ring keep 3:1; avatar tones 4.5:1; the backdrop scrim stays at 0.8 or more; the spot fill keeps 2:1 against the ground and 1.75:1 against surface and hover in a light theme (amber's own 2.0, 1.88 and 1.76), 3:1 in a dark one; `--on-amber` keeps 4.5:1 on it; ink keeps 4.5:1 on the selection (the spot colour at 32%).
- **A theme with one variant pins the appearance** to it: `oled` and `dracula` are dark, so Settings → Appearance disables the mode row and says so. Settings → Appearance lists the themes as cards (a miniature window drawn from each theme's own tokens, its name, a line, "Dark only" where it applies), in two groups, "Sona's own" and "Community palettes", then the user's files.

## Space and density

- 4px base. Control height 32px (top-bar search 32, buttons 32, compact icon buttons 28).
- Shell: sidebar 232px; top bar 48px; now-playing bar 72px spanning the full width; content gutter 32px (24px under 960px wide).
- Nav rows 32px on a 34px pitch, 16px outline icons at 1.5 stroke (SF Symbols on macOS for the ones "Icons" names), 10px to the label.
- Track rows 44px with 32px artwork (a later "compact" density setting moves rows to 32px, artwork hidden, type unchanged). No zebra striping and no row dividers: hover is a `--hover` fill at radius 6.
- Album and playlist grids: cover tiles from 160px, gap 24px horizontal, 28px vertical, title 14/500 and artist 13 muted under the cover.
- Minimum window 940×600; the layout holds at half of a 1920 screen.

## Shape and depth

- Tight radius family: artwork 4px, controls 6px, menus and dialogs 8px. Avatars and the round play button are circles. Nothing else is pill-shaped.
- Surfaces separate by a tone step, or by one 1px `--line`, never both. Shadows only on what floats (menus, dialogs, toasts): the `float` shadow, 8px down with a 24px blur, black at 12% in light and 40% in dark.
- No gradients, no glows, no glass.

## Icons

- Lucide outline icons (`theme.Icon`), at 16px in controls, 14px in dense rows and 12px beside meta text, in the colour of the text they sit with. Solid forms only for what is on or what plays: the play triangle of the amber disc, a favourite star, a set thumb, a running sleep timer.
- **macOS** draws the transport and the common controls as SF Symbols, so the Mac's controls read as the Mac's: play and pause, previous and next (`backward.fill`, `forward.fill`), shuffle, repeat and repeat one, the speaker (`speaker.wave.1.fill`, `speaker.wave.2.fill`, `speaker.slash.fill`), the queue (`music.note.list`), search (`magnifyingglass`), close and clear (`xmark`), more (`ellipsis`), add, delete (`trash`), favourite (`star`, `star.fill` when on) and the four chevrons. The map is `theme`'s (`symbols.go`); every other icon, and Sona's own glyphs (`hat-glasses` for private listening, `disc-3`, the brand mark), stays Lucide there too.
- A symbol takes the box the Lucide icon had (16, 14 or 12) and is drawn at its natural scale, so a row of them balances as macOS balances them (a cross stays smaller than a speaker), over 1.2 times the box so that its ink is as large as the Lucide icons' beside it, at the Regular weight, which is the 1.5 stroke of the Lucide icons beside them. The transport and the speaker are solid, as macOS draws them; the rest are outlines like Lucide's.
- One colour per symbol, the token the icon had: no multicolour or hierarchical symbols, which would bring Apple's palette into Sona's. A symbol does not ease its colour with its button's face (it changes on the frame the face starts to), the one difference from Lucide.
- Linux and Windows keep Lucide everywhere, and the golden images are Linux's, so they show Lucide.

## Artwork

- Square, 4px radius, a 1px line at 6% ink drawn just inside the edge (`ArtOpts.Edge`) so pale covers do not dissolve into the ground. Video frames are 16:9; hero cards are Apple's 1060:608; bricks and page banners keep their own 2.4:1 or 4:1 shape.
- Always requested from Go at the exact rendered box (width and height × the window's scale; grid tiles at their exact width, never rounded up, so the art is never resampled while painting) through `/art/<key>/<w>x<h>.jpg` paths, resolved in one place (`kit.ArtLoader`, `internal/ui/kit/artloader.go`) and decoded off the main thread, in parallel, covers on screen first. The box is fixed before the picture arrives, so rows never shift.
- Missing art: a `--hover` square with a 40%-ink outline music glyph centred, at the same size. People never get this glyph: see People, below.
- Playlists Apple rendered no cover for show a mosaic Go composes (`internal/artwork/mosaic.go`): the first four distinct album covers in track order as a 2×2 grid, edge to edge with no gutter, each tile the centred square of its cover, at the exact requested box like any cover. One distinct cover, or fewer than four, shows the first cover whole. Cropping each tile is what makes it a collage; the UI sees an ordinary `/art/` path, and the playlist's backdrop and colours come from the mosaic. A playlist with no track artwork keeps the missing-art square.
- **A playlist's cover is the user's to change** (*Change cover…* in the item menu of any library playlist, and *Cover* in the editor), because Apple's web API cannot store one (`docs/apple-music-reference.md`, "Playlist artwork"), so the choice is Sona's own, kept on this computer, and the picker says so. The choices: **Automatic** (Apple's cover: for a playlist with songs and no photo, the cover with its name Apple generates, in the design chosen in Apple's apps and four colours from the first song's artwork; the mosaic above only where Apple gave nothing), **None** (the missing-art square, the same as a playlist with no artwork), **Your image**, and Apple Music's own nine designs of that generated cover, in Apple's order: Gradient, Disc, Split, Bars, Fold, Eclipse, Arcs, Dots and Haze (Sona's names; Apple shows them unnamed). Apple's CDN draws them, so they are Apple's look exactly, with the playlist's colours and its current name; they are offered only while Apple generates the playlist's cover (a playlist with songs and no photo of its own), and the picker says so when they are not there. A chosen image is cropped to its centre square, scaled to at most 1024 px and stored as a JPEG (JPEG or PNG in, up to 20 MB and 40 megapixels, at least 300 px each way; the orientation of a photo is honoured and transparency goes onto white). Every one is an ordinary `/art/` path, so the sidebar, the library grid, search, pins and the page and its backdrop show it without knowing.
- **The picker** (`internal/ui/features/playlist/covers.go`): a dialog of tiles in four columns (92 DIP covers with their names under them), a radio group, so one is chosen and a ring, a check and a heavier name say which (not colour alone). A choice applies at once, on every surface; there is no Save, only Done. Below the tiles: *Choose an image…* (the system file chooser; hidden with a sentence where there is none), *Remove my image* once one is kept, and the limits in view beside the button, not behind a failure. The grid scrolls in a short window.
- The cover is never cropped, tinted or overlaid with text, with one exception: an album not out yet carries its release date ("Out 19 Nov") in a small `--raised` chip at the cover's bottom-left corner, where the play control of a playable tile sits (it has none). Its page leads with the release ("Out in 18 days, 6 hours", the day in full), re-read every minute, never a ticking counter (Album page, Not out yet, below).
- Hero cards (the untitled "Featured" row) fill Apple's own card shape, 1060:608, with the 4:1 `subscriptionHero` art cropped to it by the CDN, as music.apple.com does: the art keeps its text and subject in the centre, so nothing is letterboxed or cut. A card without a banner shows its cover whole on its background colour. On Discover they are the front page's lead and list (Discover, below). Bricks sit whole on their own background colour: a row takes the shape of its tallest banner (at most 2:1), so only a wider banner in a mixed row is letterboxed.

### The song page: a liner card

The song page is a record's liner note for one track: who it is on the left, what is known about it on the right. The line is the one the album's sleeve draws (cover, then fine print), so a song reads as a smaller sibling of its record.

- **Two columns from 860px of content.** The left is a spine in the cover's wash (`kit.Backdrop`, the same wash the other headers use) that bleeds off the top and left of the column: the cover in one of three sizes by the page's width, 240px, 280px from 1300px and 320px from 1600px (the spine follows the cover), so Go serves it at its exact box and only three of them, the title at 32/36 (three lines at most), the artists as links at 16/500, the album under them as a link at 13 muted (it opens the album with this song marked), Apple's badges, then the actions: the Play disc (amber, as every Play disc), Add, Favourite, Suggest Less and the menu. The spine is as tall as its own content and ends there; the right column carries on beside and below it. Videos and More by the artist are shelves under both, full width.
- **The right column reads as a ledger**, top down: what the song is, who made it, what it says. **Details** is a name and value per row: muted names in a 112px column, values in ink, all at 13/18 with a 1px `--line` under the heading. A fact Apple does not send has no row (Released in full, Duration, Genre, Track, Disc from the second disc, Written by, ISRC); the DTO has no label or ℗ line, so there is none. The album is not repeated there, so each link has one tab stop.
- **Credits** are one block per group under their own 1px rule, the group's name at 13/600 muted (an h3 inside a group), then the people in a grid of two, three or four to a row by the room the column has (at least 210px a person, 24px between). The body column keeps its 760px reading width with up to three to a row and grows to 960px when four fit; the Details rows and the lyrics keep their own measure (760px, 540px) and the rules run the column's width. A person is the 40px avatar, their name at 14 and their roles at 13 muted; the roles wrap.
- **Lyrics** come last: Apple's excerpt at 16/28, a verse break kept, with View full lyrics at the shelf's end, and the whole lyrics on request, as before.
- **Under 860px** (down to the 940px window, 707px of content) the spine is a band across the top, the 136px cover beside the title and actions on the same wash, and the body follows under it in one column. A song with no credits and no lyrics says so under Details.
- **Loading** is the same frame built by the same code, with flat bars where the text goes: the cover's square, the title, the artist, the action discs, Details' five rows and two rows of credits. Nothing moves when the song arrives. An error or a signed-out account takes the column's top in the usual way (a way back, Try again, Sign in).

### People: pictures and initials

A credited person (the song page's Credits, in a grid of two or three to a row) is a 40px circle beside their name: their picture where Apple has one, their initials where it has not.

- **The picture** is the `artwork` Apple sends with the credit (`credit-artists`), cut to the centre square by Apple's CDN and requested from Go at the exact size like any cover (`kit.PersonAvatar` over `kit.Artwork`, round). Apple's own stand-in for a person it has no picture of, an initials image under `/image/thumb/gen/`, is not a picture: Go drops it (`applemusic.Artwork.Generated`), so the same initials are not drawn twice in two styles. The credits arrive with the song page, so pictures cost no request of their own; a picture is requested only when its row is built.
- **The initials** (`kit.Initials`) are the first letter of the first and of the last word, upper-cased: "Thom Yorke" is TY, "Madonna" M, "Ludwig van Beethoven" LB. Punctuation before a letter, a bare "&" and a leading "The" are skipped. A letter is a whole user-perceived character in any script (米, 봉, ف, 𠮷); a name in a script without spaces gives its first character. They are set 38% of the avatar's size in weight 600, centred.
- **The tint** is one of eight quiet tones (`avatar-<n>-bg` and `-fg` in `tokens.json`, per theme; `kit.AvatarTone`) picked by a hash of the lower-cased name, so a person has the same tint on every page and session. Hues spread around the wheel at low chroma (light: L 0.90, C 0.035 under L 0.32 text; dark: L 0.36, C 0.04 under L 0.92 text) so a column of them reads as a set, not as brand colour. Text on every tint is at least 4.5:1 in both themes (a test reads the values from `tokens.json`).
- **A picture that fails** to load falls back to the initials while Go's retry runs, never the broken-image glyph or the grey artist glyph.
- **Decorative**: the name is always written beside the avatar, so the avatar is hidden from assistive technology; the credit reads as name, then roles.

## Backdrops (pre-blurred in Go)

The artwork service renders, per cover, a tiny pre-blurred JPEG (target ~32×32, heavy Gaussian blur, saturation kept). The UI never blurs anything itself.

- **Where**: behind the now-playing bar, the mini player, the now-playing view, full-screen lyrics, collection headers (playlist, artist), the album page's sleeve when the cover's colour isn't known (Album page), and the top of Home (from its lead pick).
- **How**: `kit.Backdrop` (`internal/ui/kit/media_backdrop.go`) fills its box with the cover's dominant colour at once, then draws the tiny image over it at 110% (the GPU's bilinear upscaling of the tiny image is the softness), under a solid scrim of the surface colour (bar, mini player) or the ground colour (views, headers) at `--backdrop-scrim`: 92% in light, 86% in dark. The result is a wash of the cover's colour, never a recognisable picture. Text sits on the scrim, so contrast is the scrim's, not the image's: the scrim is the floor at which `--ink-muted` and `--ink-faint` stay ≥ 4.5:1 over any backdrop, a black one in light (the binding case: 91% on surface) and a white one in dark (85%). It is one value per theme, never lowered per surface; a stronger cover colour would need a darker text token, not a thinner scrim.
- **Change**: when the track changes the new backdrop fades in over 300ms (the one authored motion moment). Images are decoded before the swap so the fade never shows an empty frame.
- **Banned**: any live blur (a blur effect at paint time) on anything larger than an icon, and any blur on scrolling content.
- **Reduce effects**: backdrops are not rendered at all; the surfaces are solid tokens.

## Motion

- Feedback: 120ms on hover and press (background, colour; the `feedback` motion token); menus and floating surfaces 160ms (`float`); route content 150ms (`route`); a tooltip shows once the pointer has rested 400ms (`tooltip`, MyGo's `Theme.TooltipDelay`; after one goes, the next shows at once). The durations live in `tokens.json`.
- Easing `cubic-bezier(0.16, 1, 0.3, 1)` (exponential out, the `out` curve). No bounce, no layout animation, no scroll-linked effects.
- A loop costs frames for as long as it runs: MyGo redraws the window on every frame something animates, however small the part that moves. So work that can run for long shows a still icon and progress that moves when the work does (the sidebar's sync line: its bar and count); a spinner (`kit.ButtonOpts.Spin`) is for waits of a second or two.
- The desktop's reduced-motion preference, the Reduced motion setting and the reduce-effects mode all make the kit skip its transitions and animations (`theme.ReducedMotion()`).

## Reduce effects

A user setting (`Settings → Appearance`, persisted by Go as `reduceEffects`; the UI reads it as `theme.ReduceEffects()`). With it on:

- no backdrops, solid surfaces only;
- transitions and animations off;
- the same layout and colours otherwise, so the mode is never a degraded look.

It is the recommended setting on software rendering.

## Lists

- A list that can exceed ~200 rows (library songs, albums, artists, long playlists) builds only the rows in view and a few beyond (`kit.Pane`, or the list's own `ui.Scroll`). The content pane is the scroll element, so the shell never scrolls.
- Fixed row heights (44px, or the grid's computed row height) so virtualisation never measures.
- The playing track's row is marked in place wherever it appears: its index is replaced by an amber equaliser glyph (`kit.EqualiserBars`) and its title takes `--amber-ink`. It keeps that mark while the list scrolls or re-sorts. The glyph's bars move in three steps (a smooth loop redrew the window every frame for as long as a song played), and hold still while paused and while the Now Playing view or full-screen lyrics cover the list.
- Queue rows have one menu, opened by right-click, Shift+F10 or the Menu key, or the ellipsis button that shares the hover and focus slot with the remove button (both replace the duration): Play, Play next (to the front of what plays next), Move to end (to the back of the user's songs, before AutoPlay's), then Remove. An item that would change nothing is left out (no Play next on the first song after the current one, no Move to end on the last of the user's), and the playing song has no menu. AutoPlay and station rows keep their place, so their menu is Play and Remove. A move is announced in a status region ("Airbag: moved to position 2 of 6") and a row with focus stays focused and in view.
- The queue panel is a running order in three groups, each under a 14/600 heading: **Now playing**, the playing song as one 72px row on a `--hover` fill at radius 8 (cover 56px, the title at 14/600 in `--amber-ink`, the artist and the album under it, the equaliser at the end); **Next**, the songs after it in play order, 52px rows numbered from 1 in `--ink-faint` tabular figures where the cover's index would be (the grip of a row that can be dragged takes the number's place while the pointer or the focus is on it), the heading's detail line saying how many follow ("3 songs after this one", or the station it plays from, as a link); then the station's or AutoPlay's part under its own heading, numbered on from Next. Each group is a list of its own for assistive technology, so a row says its place in its group.

### Playlist page

The playlist is set like a tape's J-card: the cover is the front panel and the facts are its spine.

- **Head.** On the cover's wash (`kit.Backdrop`), a 224px cover (144 under 960 wide, 288 from 1600 wide) beside a text column as tall as the cover: the title at 32/36, the byline and the description at its top, and the actions (Play, Shuffle, the library and rating buttons, Edit, the item menu) in one row on the cover's bottom edge. In edit mode the row holds Done, "Name and description" and Cover.
- **The ledger strip** runs under both, in one row: the kind of playlist in `--ink` at weight 600, then the number of songs, the running time and "Updated 3 hr. ago", each a 13/18 `--ink-muted` cell with a 1px `--control-line` rule between. It is the catalogue line taken apart into cells; "Updated …" keeps its tab stop and its tooltip with the exact date. "Find in playlist" sits at the strip's right end, with "N of M songs" to its left while it holds a query; there is no field above the table.
- **Starts at.** While the songs are in the playlist's own order, each row's second line says when it begins if the playlist plays straight through ("Starts at 3:20": the sum of the lengths before it, from 0:00, `i18n.FormatDuration`). A sort or a filter hides it, since the numbers would no longer be true. In edit mode it is a column of its own beside the duration and follows each move.
- **Edit mode** keeps the table's columns: a header row (reorder, #, Title, Album, Starts, duration) over the rows, so the order being edited reads as a list of times.
- **Suggested songs** are rows of the table's own height and columns (art under the numbers' edge, title, artist, Add under the album column, length), the playlist's next rows rather than a grid of tiles. **Featured artists** is a row of small round portraits (100px at least); more-from-curator keeps its covers (144px at least).
- **States.** The loading skeleton is the final layout (cover, title, strip, rows). An empty playlist or an empty edit mode puts its title and sentence flush left under the strip, with the suggestions below as rows.

## Home

Home ("For you") answers one question first: what shall I play now. It is set like the A-side of a record: the lead pick is 01, Apple's other picks are the running order, and the shelves under it are ruled sections. It sets Apple's personal shelves (`internal/ui/features/home/plan.go`) instead of stacking them as Apple sends them, and never copies Apple's own Home.

```
┌───────────────────────────────────────────────────────────┐
│ For you  Picked for you 3 min. ago              ⟳ Refresh │  one line; wash of the lead's
│ Top Picks for You ───────────────────────── Show all 12 ⌄ │  cover, full bleed
│ [ cover 280 ]     02 [40] Apple Music Live    New Release │
│                   ──────────────────────────────────────  │  the running order:
│                   03 [40] …                   …           │  one list, hairlines
│ 01 Your Station   04 [40] …                   …           │
│    Station · …    …                                       │
│    (▶) (⋯)        09 [40] …                   …           │
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

- **The page's head** is one line of the lead band: "For you" at 24/650, when Home was picked (13 muted, on the title's baseline) and the ghost Refresh at the far end. Under it the first section's ruled head.
- **Ruled heads**: every section of Home (`rule.go`) is its heading at 16/600, a 1px hairline (ink at 14%, so it reads on the ground and on a cover's wash) that runs to the right edge, and the section's controls on the line's end: "See all" to the section's own page, or "Show all N" / "Show fewer" in place. Sections sit 48px apart, so the page keeps a rhythm of rule, content, space.
- **The lead**: the first "feature" shelf (Top Picks). Its first pick is 01: the cover at 280px as the left column (360px from 1400px of lead width; a 144px cover beside the name when the page is under 900px wide), then "01" at 24/650 in faint ink hanging beside the name at 24/650, the artist or curator at 16/500, the catalogue line (`Station · Made for You`: what it is, then Apple's reason) and the amber play disc with the item's menu. The other picks are the running order beside it (under it when narrow): one column of 56px rows, each with its rank in tabular faint figures (02, 03 …: Apple's order, so the number is information), the 40px art, the name at 14/500 and Apple's reason, side by side in two equal columns from 480px of row, under the name below it. A hairline closes every row. Eight rows show; "Show all 12" in the head opens the rest. The page header and the lead sit on the lead cover's pre-blurred backdrop, ground scrim, full bleed; reduce effects leaves the plain ground.
- **Rows** (`internal/ui/features/home/rows.go`): 56px rows with 40px art, the title at 14/500 and Apple's reason with the artist or curator at 13 muted. Every shelf of stations only (Stations for You, moods, favourite artists) is rows two deep, in as many columns as fit from 264px: you play them at once, so a name and a play control matter more than a large cover. The art sits on the page's left edge; the hover plate reaches 8px past it. On a backdrop the hover is ink at 6%, not the opaque `--hover`.
- **Recently Played** is a strip: one row of covers as wide as their columns fit (112px up to about 130, the name at 14/500 and the line under it at 13 muted), so a whole evening's listening reads at once. It is the same list as the rows (one Tab stop; arrows, Home and End move; Enter opens; Space plays; Shift+F10 or the Menu key opens the menu): hover or focus shows a play disc on the cover and the more button at its corner. "Show all N" opens further rows of covers in place.
- **Cover shelves**: the first three plain shelves Apple ranks highest keep their cover tiles (160px; feature shelves 176px) under a ruled head. Features, heroes and track shelves always stay in full.
- **The index** ("More for you"): every later plain shelf (genres, "more like", fans also like) is one 52px line of a ledger: a chevron, the title at 16/600, what it holds at 13 muted in a column of its own ("3 playlists · 9 albums"; under the title below 640px of line), and a strip of its covers at 32px at the far end, a hairline between lines. The line is a disclosure button; open, the shelf's cover tiles appear under it and the strip goes. Twelve shelves take one screen instead of four, and their full-size covers load only when asked for.
- **A lone hero** (Apple's untitled featured card): the name at 24/650, the catalogue line and the play disc on the text edge, its 1060:608 banner at 45% of the row at the right end, centred against it; never a half-width card with an empty half.
- **Captions** every item of a shelf shares are dropped; said once in the heading, they are noise.
- **States**: signed out, the lead's place holds one ledger band (a rule above and below, "Recommendations need Apple Music" at 24/650 with one sentence under it, an ink Sign in at the far end), then the library's shelves. Offline with no copy of Home, the same band says there are no recommendations while offline and that Sona fetches them when it reconnects: no Retry, and no Refresh in the header while offline. Apple's Recently Played failing falls back to "Played on this computer", with its own empty line. Loading draws the final layout: the head, the 01 cover with the name's lines and the play circle, and the running order's eight ranked rows.
- **Keyboard**: the lead's title, play and menu are Tab stops; each row list (the running order, station rows, the Recently Played strip) is one Tab stop (the arrow keys, Home and End move; Enter opens, Space plays, Shift+F10 or the Menu key opens the row's menu; the row focused last is where Tab returns). Index lines are disclosure buttons that tell assistive technology whether they are expanded.

## Shell

```
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

- Sidebar and top bar share `--surface`; the canvas is `--ground`, divided by one 1px `--line`.
- The mark is a small amber disc with a spindle hole (a record label seen from above), next to the word "Sona" at 16/700.
- Active nav item: `--active` fill at radius 6, label and icon in ink. Inactive items are `--ink-muted`.
- Group label "Library" in 12px muted sentence case, 20px above its first item. Settings sits at the sidebar's foot.
- Below the library links the sidebar lists playlists in four sections, each under a 12px sentence-case heading at weight 600 in `--ink-muted` with how many it holds beside it in `--ink-faint` (tabular), 22px above and 4 below: **Pinned**, **Made for you** (Apple's personal mixes: the library copies whose catalog ID starts `pl.pm-`), **Apple Music** (playlists the user added that they cannot edit, from Apple Music or another curator; Replay and others) and **Playlists** (the user's own, editable playlists and their folders). A pinned playlist shows under Pinned only, whichever section it belongs to, and a section with nothing left in it is not drawn. Made for you and Apple Music are flat lists with no "New playlist" button, no folders, no drops and no rename; their headings are buttons that fold the list shut (state kept across restarts, `config.Sidebar`), with a chevron that turns, as a folder's does. Playlists keeps the "+" menu and the drop row. Over a playlist row's 20px cover a small play glyph fades in with the row's hover (`kit.Fade`, at once under reduced motion) on a `--ground` wash at 85%, so it reads on any cover; the glyph is `--ink`, never amber (amber marks what plays). Clicking it shuffles that playlist and does not open it; a playlist with no songs, or while the account needs signing in again, shows it in `--ink-faint` with the reason as its tooltip. Folders have no cover and no button.
- Top bar: back and forward, then the search field (max 520px, `/` or Ctrl+F focuses it, never narrower than 240px), an empty stretch of at least 64px, and the account avatar.
- The avatar is the account menu's button: a `--hover` circle signed out, `--active` with the glyph in ink signed in, and a small `--danger` dot when the account needs the user (Apple stopped taking the sign-in, the keyring didn't hand it over, Apple refuses Sona's requests). The avatar reads as held (`--active`, glyph in ink) while its menu is open. The menu (`--raised`, 288px, end-aligned) opens with "Apple Music" and then each fact about the account on a meta line of its own behind a 14px glyph (where it stands: a check, a clock while it is being checked, an alert in `--danger` when it needs the user; offline; restricted content; an unencrypted token in `--danger`), then the one action that fits (Sign in or Account settings, Check now, Try again), private listening as a checkbox item with its note, and Sign out in danger red last. Anything that needs room (the sign-in form, the account's details) opens the account dialog, never a page.
- The account dialog is two panels, 680px, bare (no padding of its own, corners clipped to the 8px radius). The leading panel (280px, `--surface`) holds the Sona mark with its name at the top and, at the bottom, the one heading (24/30, "Your Apple Music, on your desktop") and three single lines on what signing in adds (your library in sync, recommendations, whole songs rather than previews), each behind a 16px glyph. A 1px `--line` divides it from the account's panel, which holds the sign-in flow or the signed-in view in a 36px inset; signed in, the panel titles the view "Account". The first lines of both panels sit on one centre line. Below 640px of panel width the panels stack, the lead above the account with a hairline between; the whole panel scrolls as one in a short window, so the lead's tone runs as long as the form. Close, Escape and a click on the scrim close it as every dialog, and focus returns to the opener.
- **Library pages share one header** (`features/library/header.go`): the title at 24/650 with the size of the list on its baseline in 13 muted tabular figures (`24 albums`), the page's controls at the right edge on the same line, then one 1px `--line` that starts the list. The count takes no height of its own, so nothing moves when it arrives. History puts its one meta line under the title instead of a count.
- **A–Z letter rail.** Albums (ordered by Title or Artist), Songs (Title, Artist or Album) and Artists (unfiltered) carry a static column of letters at the page's right edge, outside the scroll container, 24px wide: A to Z and `#`, the letters that start something in `--ink-muted` 12/600 as buttons, the others in `--control-line`. A letter jumps to the first item of that letter, scrolled to the top of the list. It is one toolbar ("Jump to letter") and one Tab stop, Up and Down moving among the letters that can be jumped to; each button is named "Jump to A". It shows only for an ascending order, since the library sorts bytewise and a descending list has no useful first item. The page keeps the rail's place (the list is 8px narrower than it would be) whether or not it shows, so controls and tiles never move when the sort changes. A list longer than one page of 200 finds its letters by bisection over single-row reads on a worker (a few hundred, once per sort until a sync), and shows every letter faint until they are known.
- The now-playing bar is a transport deck. The seek bar runs along its whole top edge, a 3px track (5px under the pointer or the keyboard) with the amber fill up to an always-shown 10px ink thumb, in a 12px-tall hit area across the window's width; a live station has a plain 1px `--line` there. The song stands at the left (cover 48px, title and the line under it, Favourite), and a live station's marker and what is on air take that line. The transport sits at the centre of the bar, not of what is left of it: the elapsed time (`--ink-muted`), shuffle, previous, the 44px amber play disc with a solid glyph, next, repeat, and the length (`--ink-faint`), the times in tabular figures at 12. The sleep timer, lyrics, queue, mute and the 96px volume slider stand at the right.
- Library lists put their controls on the header's right: the album list has a segmented control for what it shows (All, Albums, Singles & EPs; `Segmented`, the same control as the search scope, 32px tall in a `--hover` track, the chosen one raised) before the Sort select. The choice lives in the URL (`?type=`) beside the sort and keeps its lifetime: Back and Forward return to it, and a reload keeps it. A type with no albums keeps the page and says so, with Show all albums as its one action, rather than the empty-library text.

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

```
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

- The card is 800px wide (less 24px a side on a narrow window) and holds two panes under a head that spans it: the step rail at the leading side (192px, on `--surface`, a tone step against the `--raised` card and no line) and the step beside it (28px padding, scrolling on its own). `--raised` with the float shadow and, in dark, a 1px white/10 border. The dialog scrim is black/50 in light and black/70 in dark, where a lighter one left the card barely apart from the app behind; no blur. Its top edge is fixed at 10% of the window's height (at least 24px), so the head, the rail and the foot don't move from step to step; it grows downwards to 24px above the window's foot, and past that the step scrolls while the head, the rail and the foot stay.
- Below 744px of window (a card under 720px) or when the rail would not fit between head and foot, the card is the narrow one, 640px wide: the head grows a strip under it, one segment per step filled up to the current one with each name under its segment, and the rail is gone. The strip is the same list with the same keys.
- The head names the setup and the place in it ("3 of 7"), with Skip setup at its trailing end. The rail lists the seven steps down the card, one 36px row each: its number (tabular figures), or a check once the step is behind (muted ink, not green and not amber: a step passed may have been skipped), and its name. The current step is `--ink` in semibold with a 3px ink bar at the row's leading edge; the rest are muted and take `--hover` under the pointer. The rail is one Tab stop: ↑/↓ and ←/→ move between steps, Home and End go to the ends, Enter goes there. It is a list named "Setup steps" of one button per step; the current step's button is the current step.
- The welcome step is the first picture: the Sona mark at 56px (the amber label with two grooves and the spindle hole, the same mark as the head's, enlarged) beside "Welcome to Sona" at the 32/36 display size and one sentence, then what the setup covers and what whole songs need (Apple Music and Widevine, each with where it stands). The language row joins them once there is another language to choose.
- Each step is a title, one line, and its own parts, not Settings' rows: choice cards for a choice between a few (audio quality, theme, what closing the window does), short switch rows for on/off, a compact Widevine status, and a row per connection. They save through the same queries as Settings, so a choice made here reads the same there.
- Theme is the one picture: three windows in miniature drawn from each palette's tokens (System is both, halved), as choice cards.
- Where the platform can't do something (no tray, no autostart, no notifications) Desktop says so in one line at its foot instead of leaving the reader to look for a missing row.
- One primary button, at the foot's trailing end ("Get started", Continue, Done). On Apple Music, before signing in, the password form's Sign in is that primary, in the foot beside a ghost "Not now", so it stays in view however far the form scrolls (at 940×600 the form runs below the fold); the form's own heading is left out, the step's title names it; Back is a ghost button at the leading end. No amber but the mark and the previews' play controls.
- Every step can be skipped. "Skip setup" ends it at once; Escape asks first ("Skip the rest of the setup?"), except on the last step, where it is Done; a click outside does nothing. The last step lists only what is still to do, each with the way back to its step.

### Discover

Discover (`/new`) sets Apple's editorial front page as Sona's own issue: a masthead, the front page (the featured cards as one large lead with a strip of the rest), the contents, then the shelves grouped by what they hold, each band with its name in a margin. It is the one long page in the app (twenty-odd shelves), so it gets a lead to start from and a way to reach any band without scrolling or tabbing through the rest. Charts and the room, curator, multiroom and grouping pages share its masthead.

```
┌──────────────────────────────────────────────────────────┐
│ Discover  Updated 12 min ago                  (controls) │
│ ┌──────────────────────────────────────────────────────┐ │
│ │                                                      │ │
│ │        lead: the column's width, 1060:608            │ │
│ │                                                      │ │
│ └──────────────────────────────────────────────────────┘ │
│ Lead title                                  ⏸  1 of 4 ‹ › │
│ Caption · curator                                        │
│ ▭ Title    ▭ Title    ▭ Title    ▭ Title   (the strip)   │
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
- **Front page.** One featured card is the lead, the first to start with: the whole width of the content column, in the hero card shape until that would be taller than 540px; from there the box keeps 540px and widens, towards the banner's own 4:1 and never past it (Apple crops the centre of the 4:1 subscriptionHero art to the box, where its title and subject are), so the lead spans any window without running a screen tall; the picture is requested at that exact box, with Apple's picture, a play button on hover and focus and the item's menu. Its caption sits under it as a row: the title at 24/30, the line at 13 muted below, and at the row's end the pager: the pause control, "1 of 4" and 28px chevrons. Under the caption the featured cards are a strip, four to a page in one row (two rows of two under 880px of content), each a card 72px high with the item's 64px square cover (art that is not square is never cropped to fill the square: a wide picture, such as a station's 4:1 key art or the banner of a card that has no cover, is shown at the hero card's shape across the slot's width, Apple's centre crop of it as the lead shows it, in miniature, and a tall one fits the slot's whole height; the picture is decoded at the box it shows in, centred in the 64px slot), the title at 14/500 and the line at 13 muted, a `--hover` fill at radius 6 on hover, and the lead's card on `--active` (described to assistive technology as the current one), the mark of what is selected, never amber. The strip reaches 8px past the lead's edges so that the covers and the text inside the cards line up with the lead's. Choosing a card makes it the lead (the card's page opens from the lead's title); the strip turns to the page the lead is on whenever the lead changes. The pager turns the strip only (until the lead changes) and wraps round, so a focused chevron is never disabled under the keyboard. Play controls appear on hover and focus as on every tile; a right-click, Shift+F10 or the Menu key opens the item's menu. What the front page shows is left out of the cover shelves below it (a shelf it would empty stays whole), so one album isn't met four times on the way down.
- **The lead rotates.** Every 7 s the lead gives way to the next featured card, wrapping round: the new card fades in sliding 1rem from the right over 400ms while the old one fades out the other way (ease in-out, so the two pictures cross rather than one vanishing first). Nothing blurs or filters, and a one-off 400ms transition plus one deadline every 7 s (no per-frame work) is all the page does. The next card's picture is fetched and decoded while the lead is read, so it fades in as a picture. The interval restarts whenever the lead changes, however it did, so a chosen card is read for as long as any. It holds (and starts a whole interval after) while the pointer is over the hero or focus is anywhere in it, while the window is hidden or unfocused, while the hero is scrolled off screen or covered (by Now Playing, lyrics or a modal); a tick that finds a menu open, or a video playing in the lead (motion artwork is left to play), waits a whole interval more. Under Reduce effects or the desktop's reduced motion it does not run: the lead is still, the pause control is not shown, and the strip is the way to change the lead.
- **Pause and play** (WCAG 2.2.2): a 28px ghost button at the start of the pager, with the pause glyph while the lead rotates and the play glyph while it is paused, named "Pause featured rotation" or "Play featured rotation". It is in the Tab order after the lead's own controls and before the strip, and reflects only the user's choice, not the temporary holds. Pressing it, and turning the strip's page, are said once through one polite announcement ("Rotation paused", "2 of 4"); the lead's own changes are never announced, and the lead's outgoing card is an inert copy outside the accessibility tree while it fades.
- **Contents** ("On this page"). Under its name (16/600) a row of columns, wrapping when the width runs out, one 216px column for each band: its 32px button on `--hover` at radius 6 (`--active` on hover), the band's name at 14/500 in ink and the number of its titled shelves at 13 muted (described as "3 shelves"), over a button for each titled shelf at 13 muted (ink and underlined while hovered). Every button jumps to its place (no smooth scroll) and moves focus there, so the next Tab is that band's or shelf's first control: any shelf is one jump away. A page of one band has no contents.
- **Bands.** The shelves are grouped by what they hold, in this order: Songs and albums, Playlists, Radio, Videos, Browse (category bricks and links). Apple's order holds inside each band; a shelf of mixed items goes by most of them (the first item breaks a tie), and a paragraph stays with the shelf after it. Each band opens with one 1px `--line` across the column, 56px below the band before. From a page 1100px wide the band's name stands in a 200px margin column at the left, 24px under the line, at 24/30 (it wraps in the margin), and the shelves fill the column beside it, 32px away, their first title set 6px lower so the two baselines meet; narrower, the name stands above the shelves, 24px under the line. There is no numbering: the order of the bands is not information. Inside, shelves keep their usual 40px pitch.
- **Loading.** The masthead's line holds a bar; the front page's own frame (the lead's block and caption bars, the strip's covers and lines at their card height), the contents' buttons and the first band's line and heading (in the margin at the wide size) stand where they will land, then bars for the shelves.

### Charts

Charts (`/charts`, `/charts/{chart}`) takes the masthead (Genre at its end) and keeps the rest as it was: Top songs is the kit's track list, its first column the position, which is the chart rank, and the other charts are shelves with "See all". Loading: the masthead's line holds a bar, then the Top songs heading and six rows of covers and bars, then a shelf of tiles.

### Search

The term is the page title (32/36, in curly quotes) with the scope switch (Apple Music, Library) on its baseline at the end. With no query the page is the recent searches and the browse categories.

Results are a **card catalogue**: a 200px rail on the left lists the groups found, each as a button with its name and its count (tabular figures); a click scrolls that group to the top and moves the focus to it, and the group in view sits on the `--hover` tone. The rail is outside the scroll container, so it stays while the results scroll. Under 960px it is a wrapping row of the same buttons (6px radius, a `--line` border) above the results. The results column follows Apple's order, each group under a 1px `--line` and its 16/600 heading:

- **Top results** are a best match and its neighbours: the first result in a `--surface` panel (radius 8, `--line` border) with a 120px cover, its kind in 14px muted, its name at 24/650 and its catalogue line, and the amber Play disc where it plays; up to four compact rows (40px cover) beside it, below it under 640px. Opening the panel opens the result's page, or plays a song.
- **Artists** are one row of round portraits (104px at least, the tile grows to fill the row), the other item groups cover shelves with 136px tiles, so five or six fit where the other pages fit four.
- **Songs** are the track list (number, cover, title, artist, and album from 720px of results width), eight rows, "See all" beside the heading.

Loading is the same layout in `--accent` blocks (the rail, the panel, its rows, one shelf of portraits). Nothing found is a title at 24/650, one sentence and one ink button that searches the other scope; a failed search is the dashed error block with Try again; signed out, the sign-in sentence and button take the same place.

The suggestion list under the top-bar field names each run with an icon and a 12/16 600 heading (Suggestions, Top results, Recently searched, In your library); an item row ends in its kind's icon, and the active row (the keyboard's or the pointer's) takes the `--active` fill, a ring and a chevron where the kind's icon was.

### Album page

A record is two things in the hand: the sleeve, and the listing on its back. The album page is that gatefold: the sleeve is a full-height column of the cover's colour cut by the pane's own edges, and the listing is the facing page on the ground, with generous margins.

```
┌ sleeve (flush top, left, bottom) ┐
│ ┌─────────────────┐              │   #  Title                                   ◷
│ │                 │              │   Disc 1
│ │      cover      │              │   Goldberg Variations (1981 Recording)    ← a work
│ │   256 or 320    │              │   1  Aria                                  3:22
│ │                 │              │   2  Variatio 1                            2:08
│ └─────────────────┘              │   …
│ Title (32/36, wraps)             │
│ Artist (in the cover's accent)   │   Notes (58ch)
│ Album · 1995 · 40 songs, 2 hrs…  │
│ [Lossless]  Apple's note         │
│ (▶) Shuffle  + Add  ☆  ⊘  ⋯      │
│ ───────────────────────────────  │
│ Disc 1 · 12 songs…  Disc 2 · …   │
│ Released · Genres · Written by   │
│ Label · ℗ line                   │
└──────────────────────────────────┘
Other versions · Music videos · More to hear · More by · Featured in playlists · You might also like
```

- **The sleeve is printed in the cover's colour**: a flat, square surface (no radius, no outer margin: the pane's edges cut it) in the colour Apple gives for the cover's background (`ArtworkColors.background`; Go's first dominant colour when Apple gives none), flat, with nothing blended at paint time (`internal/ui/theme/palette`; built once per cover, theme and layout, never per frame). It is the page's one coloured surface; the listing sits on the plain ground beside it, so the colour stays with the record. The page's own gutter is the sleeve's inner margin, so the sleeve's text and the shelves below share one left edge. A cover with no colour known keeps the earlier wash: Go's backdrop under the ground scrim with a 4% ink layer. Three rules hold the colour to the board:
  - one tone step off the ground: a cover the colour of the ground (a white one in light, a black one in dark) moves 0.05 in OKLCH lightness away from it, the way the theme's own tones go;
  - in the dark theme no lighter than 0.36 OKLCH lightness, hue kept, so a white or pale cover is a dark tint of itself rather than the brightest slab in the window; in the light theme a dark cover stays dark;
  - never so near a mid grey that black or white loses 5.8:1 on it and its pressed tone, so the text and a muted step below it can both reach 4.5:1.
- **Text on the sleeve** takes Apple's own text colours for the cover (`text1` for ink, `text2` or `text3` for muted) when they read at 4.5:1 on the sleeve and on its hover and pressed tones; otherwise ink is black or white with a trace of the sleeve, and muted is ink mixed 35% towards the sleeve, moved in lightness only as far as 4.5:1 needs. The sleeve redeclares its own `ink`, `ink-muted`, `hover`, `active`, `line`, `control-line` and the roles built on them (`theme.Scope`), so the disc index, badges and outline buttons inside it (the whole head, the cover, the actions and the fine print) follow it; focus rings there are the sleeve's ink. Reduce effects changes nothing: the sleeve is a flat colour already.
- **The cover's accent takes amber's job on the page**: the most colourful of Apple's colours and Go's dominant ones (OKLCH chroma ≥ 0.06) fills the Play disc, marks the playing row and its equaliser, and sets the artist's name, the record's one link out. Its fill keeps at least amber's own distance from the ground (2:1 in light, 3:1 in dark) and from the sleeve when it sits on it, its glyph 4.5:1; as text it keeps 4.5:1 on every row tone, a selected row included. A greyscale cover keeps amber. (The accent is redeclared as `amber`, `on-amber`, `amber-ink` and the `play` roles for the whole page body, Apple's shelves below included.) Only the album page takes colour from its cover this way: the song, playlist and artist headers keep the backdrop wash of the cover's first dominant colour. Every pair is checked by unit tests over extreme covers (white, black, mid grey, the sRGB primaries, a 6-level grid in both themes).
- **One station**: the cover's top and the title's first line share a line on every album, whatever the title's length; the title wraps instead of clamping. Covers come in fixed sizes so Go serves them at the exact box: 256px, 320px from 1280px of content width, 384px from 1600px (a 1920 window).
- **Held while the songs scroll**: the sleeve's contents stay at the pane's top, inside the sleeve's column, whenever the pane shows all of them (measured, so a one-disc sleeve holds in a shorter window than a three-disc one), so a 40-song album keeps its cover, title, actions, disc index and fine print in view. The column itself is as tall as the pane at least, so it always reaches the pane's bottom edge, and the shelves begin under the gatefold. Motion artwork plays there while it is on screen, under the same rules as every header.
- **The back of the sleeve** is the record's fine print in meta type, each fact a muted name over its entry: released (the day in full), first released (the year of its oldest recording, when that is earlier: a reissue, a compilation), genres (all of Apple's, without the root "Music"), written by (every songwriter in Apple's composer credits, most credited first, eight named and the rest counted) or, on a classical album, composer, then the label unless the ℗ line names it, and the ℗ line last.
- **The sleeve column is the cover plus 48px of room, between two gutters** (368px at 256, 432px at 320, 496px at 384), so the action row keeps to one line; the listing takes the rest, 40px from the sleeve and a gutter from the scrollbar, and the notes follow the songs in it. At 1440 the title-to-duration scan is about 680px.
- **The disc index**: each disc in the catalogue line's form ("Disc 2 · 14 songs, 1 hr, 3 mins"); choosing one brings its heading to the top and focuses its first song. A list long enough to be virtualised (over 200 songs) has no disc headings, so no index either.
- **Classical albums read by work**: where Apple names the work a track belongs to, each work gets a heading (14/500 ink, under the disc's) and its rows show the movement alone ("Aria", not "Goldberg Variations, BWV 988 (1955 Recording): Aria"); the row's play control and menu still name the song in full, and the queue and now playing show its full name. An album of several composers leads each work with its composer and names the composer under a piece outside any work.
- **Rows explain themselves**: the line under a title names a credit other than the album artist's (an Artist column only for a real compilation, where most credited songs are someone else's; a song with no credit yet counts for neither) and why a song doesn't play: "Unavailable" or the account's explicit restriction (on an album not out yet its group's heading says when: Not out yet, below). An unpublished song's stand-in artist ("unknown") is dropped, and "Popular" only marks a song that plays.
- **The catalogue line describes the record**: a library album holding some of its songs counts the whole album and says "2 of 13 songs in your library · Show complete album" in the status slot; an album not out yet leaves out a running time its unpublished songs can't add to.
- **Not out yet** (an album Apple marks as a prerelease, from the catalogue or pre-added to the library): the page answers "when, and what can I do about it", in that order, and is the released layout with its head reordered and its songs in two groups, not a second page. Inside the sleeve the head reads title, artist, catalogue line, **the release**, the actions, then Apple's badges and note.

  ```
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
  - **Songs out now and songs coming are two groups, not greyed rows.** The songs that play come first under an **Out now** heading, then the rest under the day they are out ("Out 22 Nov"), each group in the record's order with its own track numbers; no row repeats the day and no duration column fills with dashes (`TrackListOpts.BlankDuration`). A record with every song to come has the one heading and nothing to separate; one with every song out has none. Two groups have no disc headings or disc index, because discs would repeat across them. The coming rows are the list's ordinary disabled rows (`--ink-faint`, 4.5:1), now explained by their heading rather than by a line each.
  - **A year and no day** ("Out 2027") has no countdown and no Remind me (a reminder needs a day); the lead is "Out 2027" and the line under it says "The day hasn't been announced yet". The songs sit under one "Out 2027" heading.
  - **No song out** has no Play disc: Pre-add (or Remind me) is the page's one ink-filled button, and the list is a preview of the record with its title and numbers only.
  - **The moment it comes out while the page is open.** When the release time passes, the countdown becomes **Out now** with "Updating the songs" in the line under it, the page announces "{album} is out now" once (a polite announcement, `c.Announce`), and the album is fetched again, every minute. The service says an album is out from its date alone, whether or not Apple has published its songs (they have no play parameters until it does), so the page keeps the state while songs that were waiting are still unplayable, up to 30 tries (about half an hour); after that the songs show as they are ("Unavailable"). Meanwhile the head keeps its shape (the stand-in credits stay dropped, the catalogue line stays without a running time, no dashes), and the songs aren't marked "Unavailable". When they play the page is the released one.
  - **Long and classical lists.** Headings need a list the table builds whole, so an album of more than 200 songs (a virtual list) keeps its order and says "Out 22 Nov" under each song that isn't out. On a classical album a group's heading carries the work of its first row ("Out now · Ludwig van Beethoven · Symphony No. 5"), the work named again even where it runs on from the other group.
- **Below 860px of content** (a 940px window) the sleeve is a full-bleed band across the top of the pane, edge to edge, flush to its top, with the title beside a 168px cover; the songs follow at full width, then the fine print on the ground; the disc headings in the list stand in for the index.
- Then Notes at a reading measure (58ch, about 75 characters at the body size), and Apple's shelves in Apple's order, with the radio episodes about the record ("More to hear") after its music videos.

### Artist page

An artist page is the act's press kit: the sheet that says who they are and what is newest, what to play first, the career laid out year by year, then the company they keep. It answers "who is this" on the first screen and never repeats a release to fill space.

```
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

- **The header** is the act's press sheet, one row on the wash: the text column at the leading edge, the latest release in a second column, the round portrait (people stay round) at the trailing edge (200px; 256px from 1500px of content; 176px under 1040px; 112px under 720px). The text column holds the name at 32/36, the catalogue line in its usual form with the kind first (`Artist · Alternative · Abingdon, Oxfordshire, England`; `Artist · 1 release` when the page holds every release of a small act), the biography's first paragraph at 13 muted, clamped to two lines at a 32rem measure, "Read more" (which scrolls to About and moves focus there), and the actions on one row. From 1040px of content the latest release is a 152px column (192px from 1500px): the cover with the amber Play disc over its corner, the title at 16/600, `Latest release · 13 Aug 2025` and `Album · 12 songs` in meta type, Apple's " - Single" / " - EP" suffix moved into that line. Narrower it is a row under the text, after a hairline: an 88px cover (64px under 720px), the lines and the Play disc at the end. The disc plays the album through the item players and is left out where the album cannot play (not out yet). Title and cover open the album. An act with one release has no slip (it is in the discography).
- **The lead**: top songs (six rows, numbered in Apple's popularity order; the rest behind See all) with the essential albums beside them from 800px of content, set in a 340px `surface` panel at radius 8 as a release list. Under 800px the panel follows the songs, its albums in two columns.
- **Release lists**: the form of essential albums and Appears on. Rows of 52px: a 40px cover, the title at 14/500 and a 13 muted line (the year, or the artist and year for Appears on), a `--hover` fill at radius 6 that reaches 8px past the covers so they line up with the heading. Rows open, select, drag and open their menu (right-click, Shift+F10, the Menu key) like cover tiles; ↑ and ↓ move between them.
- **Bands**: each is its heading at 24/30 with a hairline running on to the column's edge. Discography is a career line: the releases the page holds (albums, singles and EPs, live albums, compilations), once each, newest first, the newest 18, grouped by year. The year (16/600 tabular, faint) is a 64px left gutter (52px under 720px), a dot and a 1px rail link the years, and the year's releases are release rows to the right (two columns from 840px of content when a year has four or more), each saying its kind (`Album`, `Single`, `EP`, `Live`, `Compilation`, or `Out 19 Nov` for an upcoming one). Under the career line, a ghost link names each shelf's view the timeline does not show whole (`Singles and EPs featuring Radiohead ›`), so every full view stays one click away; music videos follow as a row of 16:9 tiles. Related holds Appears on (a release list), playlists featuring the artist and similar artists. About is a spec column (From, Born or formed, Genre at 200px) ahead of the whole biography at a 32rem measure; narrower the facts run in a row above the text. Empty shelves and bands are left out.
- **Loading** holds a major act's final frame: the name, line, two biography lines, Read more, actions, slip and the portrait at the trailing edge, then top songs and the essential albums panel as loading rows of the real lists. For such an act nothing moves when the data lands, but the scrollbar arriving with it narrows the column by its width. A small act with no biography or slip has a shorter header, so under 800px of content its songs move up when they load.
- A library artist keeps its library shelves (albums, singles and EPs, songs in your library) between the header and the lead; its catalogue line counts the library (`Artist · Alternative · 3 albums, 20 songs in your library`).

### Library Artists

Library → Artists is an overview, not a launcher: every artist in the library on one side, and whichever one is open on the other, with their library albums. Moving from one artist to the next never leaves the page.

```
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
- **The list**: the page title and count on one baseline (24/650 and 13 muted; `1 of 12 artists` while filtering), the filter (32px, the Shortcuts filter's field), then the artists A to Z under a letter heading each (13/600 ink with a 1px `--line` running out to the right, the letter itself, not a label), `#` last for digits, symbols and other scripts, and the letter rail at the list's right edge while the filter is empty. Rows are 44px with a 36px round portrait (the artist's own, else their newest album's cover, else the placeholder), the name at 14/500 and what the library holds of them at 12 muted (`12 albums, 130 songs`, tabular figures). The open artist is a `--active` fill at radius 6; hover is `--hover`. Heights are fixed, so the list, which builds only the rows in view and a few beyond in its own scroll box, never measures: thousands of artists scroll like ninety.
- **The filter** matches as you type, ignoring case and accents (`bjork` finds Björk, `edith` Édith Piaf, `sorensen` Sørensen), best first: names starting with it, then names with a word starting with it, then any substring, then the name without its spaces and punctuation (`acdc` finds AC/DC). Filtering drops the letter headings, since the order is the match's. The artists come from one query (`LibraryService.ArtistIndex`: name, portrait, album and song counts, from SQLite only), so filtering never calls Go and never waits.
- **The detail** is the open artist's library, not the catalogue: the portrait (144px, 192px from 1500px of page width, bottom-aligned with the actions), the name at 32/36, the catalogue line (`Artist · 12 albums, 130 songs`), the amber play disc, Shuffle, **Full artist page** (the artist's own page, with the catalogue: biography, top songs, the whole discography), the artist's menu, then a 1px `--line`, then **Albums** as a cover grid, newest release first, and **Singles and EPs** under their own heading (each heading carries its count in tabular figures and a 1px rule out to the right) (Apple's ` - Single` / ` - EP` title suffix, or the album's single flag, decides; the suffix moves to the tile's line: `2020 · Single`). Play and Shuffle read the artist's songs from SQLite when pressed. An artist with songs but no albums shows their songs (Songs in your library, the track table) where the albums would be.
- **In the URL**: the open artist is `?artist=<library id>`, so reload, Back and Forward keep it. With none (or one not in the library) the first match is open beside the list, without writing the URL. Clicking an artist adds a history entry; moving with the arrow keys replaces the entry, so a walk down the list is one step back, not fifty.
- **Keyboard**: the filter (a combobox) and the list (a listbox, the second Tab stop) share their keys. ↑/↓ move the open artist (PageUp/PageDown by ten, Home/End on the list), Enter or → (from the end of the filter's text) go into the detail: focus lands on its first album, or on its first control while the albums are still being read. Escape clears the filter, and from the detail returns to it. Typing on the list types into the filter. The open artist is the active descendant of both while its row is built; rows tell assistive technology their position and the size of the set. The count of matches is announced.
- **Narrow windows** (the page under 880px wide, which is a 940px window with the sidebar open): the list and the detail take turns. The list fills the page; choosing an artist (a click or Enter, which opens the filter's top match when nothing is marked) shows the detail with a ghost **‹ Artists** button above it; the artist's name is then the page's h1. The arrow keys move a cursor in the list without opening anything. The list stays mounted while hidden, so its scroll and filter survive. Back (the button or the browser's) returns to it with focus in the filter and the artist you left still marked; Escape in the detail moves focus to the button.
- **States**: signed out, syncing, empty and error are the library gate's block in place of both columns (`Artists` title above it). Loading draws fourteen list rows. An artist's albums loading draw six cover tiles; failing, an inline error with Retry. No match: "No artist matches “xyz”" under the filter, and no detail open.

## Now Playing

The record held open: the sleeve on one side (the cover and its label copy), the insert on the other, one at a time: the lyric sheet (Lyrics, below) or the running order still to play (Up next). It covers the page column, so the sidebar, the top bar and the player bar with its transport stay where they are, over a wash of the cover (Backdrops). A fold, one 1px `--line` from the top of the view to its foot, divides the two.

```
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

- **The sleeve.** The cover at a fixed size so Go serves it at its exact box: the smaller of what the column's width and its height allow, 560px from 1800px of column and 1050px of height, 480px from 1500 and 900, 400px from 1000 and 720, 320px from 880 and 640, 240px in a shorter window, so on a big screen the sleeve fills its column instead of floating in it. The view uses the whole page column up to 1760px (the largest sleeve and an insert of about 1100px), centred beyond that. The cover and the label copy under it are one block, held at the top of the sleeve column, 24px down, on the same line as the insert's tabs at any height (the column is the cover's width and 48px at each side; a block taller than the column is cut at the foot), with the copy set from the cover's left edge. The label copy: the title at 32/36 (24/30 under a 320px cover), wrapping to three lines; the artist at 16/500 and the album at 13 muted, both links out that close the view; the marks the bar also shows (Explicit, Preview); then the song's own actions: Favourite, Suggest Less and View credits (the song page). A station shows its name, a Live line with the dot (amber while it plays) and what is on air, as the stream or the schedule names it. A local broadcaster is headed by the song its stream names on air (that song's cover, its artist under it), with the station, its genre and place, its description and an Open website button below; a music video shows its 16:9 frame at the cover's width and a Show video button.
- **The insert.** Its header is one row, the tabs named in words (Lyrics, Up next) and Close, 47px tall over a 1px `--line` rule: the shown tab is in ink with a 2px ink rule resting on that line (not amber, which stays with what plays). A station or a song outside the catalogue has no lyrics, so its row holds only the heading Up next. The view reopens on the pane it showed last; the bar's Lyrics opens it on the lyrics and its Queue (or Q) on Up next, and each closes the view when its pane is already showing. The queue beside the page steps out while the view is open and comes back when it closes, so the queue is never drawn twice or hidden under the view.
- **Each pane's controls sit at its foot**, as full screen keeps its own, in a 52px strip under a rule of the header's length: the lyric sheet's Translation and Pronunciation at the start, the timing nudge and Full screen at the end; Up next's line ("9 songs after this one", or the station it plays from) at the start, Clear and AutoPlay at the end. The header holds only the tabs and Close, so the column opens on the words.
- **One text edge.** The tabs, both rules, the lyric lines, the queue's numbers and the foot's line all start 24px from the fold (12px of the insert's side, 12px of inset). Up next is the queue's own list (rows, keys, drag and drop; the groups of the queue panel, Lists); on the wash its hover and focus fill is ink at 6%, not the opaque `--hover`, the playing row's fill is ink at 5%, and its equaliser holds still under the view like every other (Lists). Beside the page the Next heading's detail line is the panel's summary; here the foot says it, so the heading has none.
- **The sheet's reading band** is measured from the sheet's own height, not the window's: the first line starts 14% down, so it sits at the band's top however much the header above takes.
- **Narrow** (under 880px of column, a 940px window with the sidebar): the sleeve becomes a header, the cover at 168px beside the label copy and Close at its end, over the tabs (the insert's header, with its rule) and the pane at full width; there is no fold.
- **No lyrics**: the usual dashed block ("No lyrics for this song", the reason) with one link, Lyrics sources, to Settings → Playback.
- **Cost.** Nothing new moves, and nothing beneath it is drawn: the page under the view is hidden (it stays built, so it keeps its scroll, but is not drawn and takes no input), so a motion cover never repaints the album page's own header under it. The layout follows the view's own box as last drawn and changes only when its size class does. The pane not shown is not built, so on Up next the lyrics' timers are gone. Motion artwork plays on the cover from one rendition (asked for at 560 DIPs, the largest cover class, so Apple's 1120 px picture at 2x, never narrower than the cover) at every size, so a resize never reloads it.
- **Focus.** Opening the view focuses the shown tab (Close when there is one pane); leaving full screen returns to Full screen; Escape closes the view, from anywhere in it (a tooltip showing on the focused button takes the first Escape, as everywhere).

## Lyrics

A lyric sheet, the insert in a record's sleeve: the words set in verses down one column in the house type, with amber marking only where the voice is. The now-playing view's lyrics column and full-screen lyrics share it (`internal/ui/features/player/lyrics_sheet.go`, with the timing logic in `internal/ui/lyricsync`); the mini player shows the line being sung (`internal/ui/features/miniplayer/sungline.go`).

- **One sung line.** The sung line is the only line in `--ink` (timed by line only, it is `--amber-ink` itself); earlier lines are `--ink-faint`, later ones `--ink-muted`, plain lyrics `--ink`. Word-timed lyrics light each word `--amber-ink` as the voice reaches it. Short words light whole as they begin; held notes (600 ms or longer) fill in the reading direction, stepped about 12 times a second behind a soft 0.5em edge. Lines change colour without a transition.
- **Verses.** A pause of 3 s or more between one line's end and the next, or a change of the song part the lyrics name (Apple's "Verse", "Chorus"), starts a verse: a 32px gap in the column, 0.9em full screen. While the voice rests in a break (from 0.5 s after the line's end) nothing is lit.
- **The sheet turns; it doesn't crawl.** While the sung line sits in the reading band (14–72% of the sheet's height) nothing moves. When it leaves, the sheet scrolls once so its verse starts at the band's top (the line itself when the verse is taller than the band); in a break it turns to the verse coming. Smooth, or a jump under reduced motion; a wheel, touch or key scroll by the user is left alone for 4 s.
- **Edges.** The sheet fades out over its top and bottom edges (3rem in the column, 12% of the window's height full screen), so a line leaving the view never shows clipped glyphs. It draws nothing while the sheet holds still.
- **Under each line**, smaller and in `--ink-muted`, each on its own line: the background vocals (600: they are sung, and they light on their own timing, so they can still be filling under the next line), then, when switched on in the view's bar, the romanisation (400, lit like the line when it is word-timed) and the translation (400) in the language chosen in Settings → Playback → Lyrics.
- **Duets**, as a lyric sheet sets them, from one reading edge: the second voice is indented 1.5em and set in italic; lines every voice sings stay at the margin, upright, the first of each such passage headed "Together" in meta type, `--ink-muted`. Never voices on opposite sides of the screen.
- **Full screen**: room type (4.4% of the window's width, between 36 and 84px, 700, -0.02em), the sheet a 24ch column centred on the screen, the song (48px cover, title at 14/600, artist at 13 muted) and the timing nudge in the quiet strip at the foot, under a 1px `--line` rule that spans the side room (7% of the width), as Now Playing's foot does. A failed load is the usual error block with Retry.
- **The mini player** puts the sung line in the album's place, under the artist: up to two lines at 13/18 in `--ink` after the 12px lyrics glyph, its words lit whole in `--amber-ink` (no fill: a glance needs no frames). Before the first line, in breaks and for songs without synced lyrics it shows the album in `--ink-faint`. It keeps the main window's per-song timing.
- **Keyboard.** In the column the lines are one Tab stop (the sung line, or the one focused last): ↑/↓ move, Home/End go to the ends, Enter or Space seeks there.
- **Motion and cost.** Nothing runs a frame loop: the words change on timers (`docs/ui-architecture.md`, "Now Playing and lyrics"). Under reduced motion or reduce effects every word lights whole as it begins.

### Radio

Radio (`/radio`) answers what radio alone can: what is on air right now, and what follows. Apple's live stations open the page as a programme guide, the way a broadcaster prints its listings: a time ruler across the top, one lane per station, each programme a box set by its real start and end, and one line down every lane at now. The local broadcasters follow, as they are live too; then Apple's radio page in bands, with the stations to tune to before the shows to listen back to. The page hangs its headings in a rail at the left, as wide as the guide's station column, so the guide, the broadcasters and every band start their content on one vertical edge.

```
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

- **Live now: the guide.** The section's heading is at the bands' size and sits over the station column, level with the ruler; the local broadcasters' list follows inside the section. The board reaches 8px past the content on the left so the art lines up with the page's edge. Each row is 72px: the **station column** (232px: the station's own art at 56px, its name at 14/500 over two lines at most, the play control at its end) and the **track** beside it. The track shows a window of time that starts 30 minutes before now and is as long as the width allows at about 3.4px a minute, in half hours, between 90 minutes and five hours (120 at Sona's narrowest, 940 wide; 240 at 1440). The ruler names every half hour in meta type with tabular figures (hours in `--ink-muted`, half hours `--ink-faint`), with a short tick under each, and a flag in `--ink` with `--ground` text at now carrying the clock time; a hairline in `--line` at every hour runs down behind the boxes. **One line, 2px in `--ink`, runs from the flag down every lane over the boxes.** The window starts on the minute and the lines move only when the board reads the clock, every 30 seconds, never per frame.
- **Programme boxes.** A programme is a box 56px high at radius 6, placed by its start and end and clipped to the window: one that began before it is cut at the left edge and shows "‹ 09:15" there, one that goes on past the right edge shows a chevron. Back-to-back slots with one title are one box (a station's own hours, a show's consecutive hours). The programme on air is `--active` with its title at 600 and, when there is an end to count down to, "45 min left · 10:00 AM–11:00 AM" in meta (the slot only if it fits), and a 2px bar along its foot up to the now line; its words go right of the line (left of it, when the show is nearly over and that side is wider) so the line never crosses them. A named show to come is `--hover`, with its title at 14/500 and its slot; the station's own hours, and a programme that has ended inside the window, are a 1px `--line` outline with the tagline (or the description) at 13 muted. A slot named after the station shows its description, or the tagline when that is all it says. The station's own hours run to the next named show and no hour goes unaccounted for; when they run to the end of the schedule there is nothing to count down to, and the box shows the tagline alone, with no bar. A box too narrow for words keeps its name for assistive technology ("The Zane Lowe Show, 10:00–11:00"). Each box is a link to the station's page (not a Tab stop: the station column's link is the one stop); what comes next is the box after it, or, past the window, on the station's page. The schedules are the station pages' own queries, so opening a station is instant.
- **Play and state.** The play control is always shown at the end of the station column (a 32px ghost circle): picking a station is the page's job, so it is not left to hover. The column takes `--hover` at radius 6 while the pointer is on it or a control in the row has focus. The station playing takes amber where the guide shows it: its name in `--amber-ink`, the equaliser in place of the play control, and the bar on its programme in amber while it plays; nothing else on the board is amber. A station Sona can't play has no play control; its lane is one outlined box that says why, with the `CircleSlash` icon, in place of its programmes. A live station with no schedule shows its tagline the same way. Below 940 the guide keeps its columns and shows a shorter stretch of time (90 to 120 minutes); the bands stack their headings over their content.
- **Stations with nothing to open.** A station opens a page only when the page has something to show: a live station's schedule, an episode's show, the songs a track station plays first. A local broadcaster has no schedule, so it has no page: its row (the shared station list) is a button, "Play {name}", the name stretched over the row, and the menu is beside it (Play, Copy link); the play mark is a hover cue, not a second control. While it plays the name button pauses it ("Pause {name}"), the equaliser stands in the mark's place, and the second line is the song on air ("Artist – Title", from its stream) instead of the tagline. A live station whose schedule turns out empty is treated the same, in the guide too: its whole lane is the play button. Cover tiles do likewise (the tile is the button, named "Play {name}"); a link and a button are never mixed in one tile.
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

  Each surface keeps its layout and says why, so nothing disappears quietly: the player bar's controls stay in place at 40%, disabled in look but still focusable, with the reason as the tooltip's second line and the accessible description, and a press does nothing; Shuffle and Repeat read off whatever they were set to. The mini player's buttons and the queue's AutoPlay toggle do the same (the reason as title and description). Menus keep Play next and Add to queue, greyed, with the reason under the label in `--ink-faint`. Keys (Ctrl+←/→, Shift+←/→, S, R) take the chord, do nothing and say the reason once in the notifications region (a toast, not repeated while the key is held); the command palette leaves those commands out. MPRIS reports `Shuffle` false, `LoopStatus` None and refuses writes to either; on a live station `CanGoNext`, `CanGoPrevious` and `CanSeek` are false too. Now Playing has no transport of its own, so it has nothing to switch off.
- **Bands.** After the live part the page's shelves go by what they hold: Stations (the user's own first, Apple's top and genre stations), Shows and episodes (episodes, interviews, sessions, DJ mixes, shows and hosts), Browse (the links). Apple's order holds inside each; the band line and heading are Discover's. Apple's own rows of the live stations are left out while the board lists them, and kept when Sona couldn't list them.
- **Signed out.** The Stations band opens with a "Your stations" block saying what signing in adds, with Sign in.
- **Loading and errors.** The guide's frame (the ruler's heading, six lanes at their height, the art and the programmes as bars) and the broadcasters' and first band's rule, rail heading and shelves stand where they will land. A block that fails says so in place, beside its rail heading; when nothing loads, the page says it once, with one Retry.
- **The station page** (`/station/{id}`) puts the station's identity in a column of its own that stays in view as the lists scroll: the cover in two size classes by the content's width (264px, 320px from 1500px of content, so Go serves exact boxes of two sizes, not one per window width), the name at 32/36, the byline (the show, the host, the tagline, or what a track station was made from), the catalogue line, the notes, the Play disc and the menu. Beside it the lists take all the width left, up to 1100px (from 1500px of content the two columns, 320 + 48 + 1100, always fit, and past 1100px they stay together at the page's left edge): the note of a station Sona can't play, a broadcaster's "On air", the schedule, and the songs a track station plays first, each under a ruled head (a rule across the column above all but the first). Under 800px of content the identity stacks over them with a 112px cover beside its lines. **The schedule is a vertical timeline**: each programme that has not ended is a row of a start time (14/13 tabular) beside a 28px rail and its box, in the guide's look (the programme on air `--active`, a named show `--hover`, the station's own hours outlined), with the art at 48 (requested at exactly 48), the title, the description at two lines and, on air, the time left. The rail is a 1px `--line` with a hollow dot at each start; for the programme on air it is inked from its start to how far through it is, ending in a larger dot (amber while Sona plays this station), and reads as its progress. Back-to-back slots with one title are one row, so a station's own hours are not a list of identical rows. It is read every 30 seconds like the guide.

## Link lists

Apple ends its catalogue pages with lists of links, "More to Explore": hubs (Browse by Genre, Decades, Charts), then every genre. They are the page's way on, so they are controls, not a paragraph of text (`internal/ui/kit/collections_items_linklist.go`, one component for every page that shows them: Discover and Radio today, Home and the editorial pages whenever Apple sends one).

```
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

- **A button each.** 44px at least, a `--hover` fill at radius 6 (`--active` on hover and press: the same fill every row of Sona takes), the name at 14/500 in ink, two lines before it is cut. The one glyph is the 16px arrow out in `--ink-muted` at the end of a link that opens in the browser (its name says "Opens in your browser"); a page in Sona needs none, being what a button is. Nothing else is added: Apple sends a link's label and URL and nothing more, so a colour or an icon per tile would be Sona's invention and amber means playing. In light the tile also has a 1px `--line` border, the one place a fill and a line go together: `--hover` is only about 1.1:1 against the ground and the line is what makes the tile read as a control. In dark the fill is clear alone and the border is transparent (it keeps the box the same size). Focus is the usual 2px ring outside the tile.
- **Columns.** As many as hold a button of 184px, 8px apart, filled top to bottom in Apple's order (`itemsLinkGrid`): the genres are alphabetical, so they read down a column, and Tab follows the same path. A short list keeps the long list's tracks, so a button is one width on the whole page and the lists line up. The columns fill so the last holds at least half of one: 21 genres at five columns set 6, 6, 6 and 3, never 5, 5, 5, 5 and a lone button.
- **Continuation.** Apple splits a long list in two; the second has no title and sits 24px under the first, a clear step from the 8px between buttons. It is its own region, "More links". Every link is a Tab stop, in reading order: they are navigation, not a list to arrow through.

### Concerts

Concerts (`/concerts`) answers one question: what is on near me, and when. A gig is a date, a place and a ticket, so the page is set like a sheet of tickets: each day is a stub (its weekday over a big number), with that day's shows beside it. It is read down by date, set by the venue's own calendar, never a grid of covers.

```
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
- **The coming days.** Under the filters, the next 14 days (from today, or from the first day of a date range that starts later, to its last day) as one row of day cells: the weekday at 12 over the day number at 16/600, tabular, the cells sharing the width at 72 at most. A day with shows is a button (a `--hover` fill on hover, the number in ink at weight 600) named "Wed, Apr 14, 2 shows", with one to three 4px ink dots under it for how many (not amber); a day without is `--ink-faint` at weight 400 and takes no click. A button scrolls the agenda so that the day is at the top and focuses the day's first control. The cells are one Tab stop with the arrows moving along them. The strip is left out when no day of it has a show or when the range is under a week (a weekend), and it counts only the shows the filters leave.
- **The agenda.** Months under a 24/650 heading with their count in 13 muted (`April 2027 · 2 shows`; the last month's count is left out while Apple has more shows than the list asked for, as it may be short), then days. A day is a stub: the weekday at 12 muted over the day number at 32/700, tabular, in a 64px column, a 1px `--line` rule the height of the day, and that day's shows as rows to its right, so the rule's length says how busy the day is. Days are 8px apart, months 48px. The day, month and clock are read in the venue's own time zone, in the interface's language; a show whose clock is not this computer's carries its zone's name ("8:00 pm EDT"), so a gig in another country is never read as local. Shows are Apple's, soonest first; there is no popularity ordering to explain.
- **A row** is 72px and reads left to right as a timetable: the start at the venue's clock at 14/500 (the first edge of every row), the headliner's portrait (56px, round like every artist), the line-up at 16/600 (a link to the artist) over the venue and city at 13 muted with the straight-line distance from the chosen city at 12 faint beside it (`6 mi away`, miles where the locale uses them), and the one action at the row's end. A show by an artist the user follows says so on a third line. **Get tickets** is an outline button (the same `--control-line` as the filters): Apple lists the sellers, and one seller opens its https page in the browser (a trailing arrow), several are a menu of them (a chevron); a show with none shows no button. The row's `--hover` fill is at radius 6, as track rows.
- **Where.** The header's outline button names the chosen city ("Manchester"), or says "Set location" while the page is on the default, and the line under the title says what the default is (the storefront's main city). It opens one small dialog: a search field and Sona's own list of cities (the storefront's first, then well-known ones), each a row with the city at 14/500 at the left and its country at 13 muted at the right, a check in a slot kept on every row for the chosen one, with a sentence saying the choice stays on this computer and Apple Music is told only the area. ↓ moves from the field into the list, Enter takes the first match, Escape closes and returns focus to the button; "Use my storefront's main city" undoes a choice.
- **Filters** are one row of 32px controls: dates (any date, this week, this weekend, next 30 days, or two date fields), genre, and a switch "Your artists (n)" that appears only when the signed-in account has shows in this area. They live in the address (`when`, `from`, `to`, `genre`, `yours`), so Back returns to them. A change keeps the list on screen until the new one arrives.
- **Long lists.** Asked 100 at a time (Apple has no paging, so "Show more concerts" asks for 100 more, up to 500). The agenda is one Tab stop: ↑/↓ move between rows, ←/→ between a row's link and its tickets, Home/End jump, Tab leaves. What the page builds from a listing (the groups, the strip, each row's words) is made once per listing, not per frame.
- **States.** Loading draws the agenda's own frame: a month heading and three stubs with their rules and rows at their height (the strip waits for the data, as it depends on it); no concerts near the city (some countries have none) says so with Apple's "No concerts" and "Set location"; filters that match nothing say that with "Clear filters"; a failed load is the usual error block with Retry, and a stale copy says when it is from.
- **Elsewhere.** An artist's page carries "Upcoming concerts" between its lead and its discography: three rows with a compact date column and a 1px rule first (weekday, day number at 24/650, month; the artist's portrait and name are left out, the venue leads), the ones near the chosen city first, "Show all n", and a link to the agenda. It is left out when the artist has none. Home carries "Concerts in {city}" where Apple puts its concerts shelf: six such rows in two columns, with portraits and ghost ticket buttons, and "See all". Their start and distance sit under the venue, as the columns are too narrow for the agenda's timetable. Apple's own concerts links open this page.
- No amber: nothing here is playing.

## Video view

A music video takes the whole window, like full-screen lyrics: the picture is the page (`internal/ui/features/video/view.go`). It opens when a video starts, and again from the bar's cover or "Show video", Now Playing's "Show video" or V; closing it leaves the video playing in the bar.

**The page** (`/music-video/{id}`, `page.go`) is set like a screening: the lead stage spans the whole content column at every width: the 16:9 picture fills it while it is no taller than 60% of the window's visible height (clamped to 360 to 640), and past that the stage keeps full width at that height with the whole picture centred on the `surface` token like a cinema screen (never cropped), requested from Go at exactly the picture's whole-DIP box; the cover is the colour, so there is no wash behind it. Directly under it, at the column's width, is the caption row: the title at 32/36 with the artist links, the catalogue line (`Music video · 2019 · 3:35 · Pop`) and the Explicit, 4K and HDR tags on one line at the left; Play (the amber disc), Watch in Apple Music, Play the song and the item menu at the right, stacked under the title block when the column is under 720 wide. The note on standard definition and the song line follow in secondary type, then Notes, then "More by {artist}" and "More like this" as rows of 16:9 tiles from 296 wide (two or three across, "See all" only with an artist page), and the release date. Loading draws the picture, the caption bars and a row of tiles at their real sizes.

- **Strips.** A 48px button strip at the top, holding only Full screen and Close at the right (it keeps the window's buttons clear and drags the window). The foot has the seek bar across the full width (elapsed time flush left, time left flush right), then one row: the caption at the left on the stage's ground, never over the picture (the video's name at heading size, a link to its page, with the SD badge; under it the artist in `--ink-muted`, a link, and the status line in meta type: "Loading the video…" before the first picture, "Buffering…" when pictures stop), Previous, the 44px amber Play/Pause disc and Next in the exact middle, and the bar's volume control at the right. The caption and the volume share the leftover width, so the transport stays centred whatever the title's length. Tab follows reading order: the strip's buttons, the seek bar, the title and artist, the transport, the volume.
- **The picture** keeps its own shape inside the stage, black behind the picture only, never behind the strips; the stage is the theme's `--ground`, light or dark. Before the first picture the video's artwork stands in at 16:9.
- **Fading.** While a picture plays, the strips fade after 3 s with no pointer or key (and the pointer hides); any movement or key brings them back. They stay while paused, loading, failed, or while a control has keyboard focus. Reduced motion makes the fade instant.
- **SD badge.** The house `Tag` reading "SD", its title explaining that Apple allows HD only on players with hardware protection. Shown while the video plays, hidden in an error state.
- **Error states** replace the picture with the usual dashed error block: what failed ("Couldn't play this video", or "Music videos don't play here" where Sona can't), the reason in Sona's words, "Watch in Apple Music", and "Try again" unless trying again can't help (Apple offers the video only in HD, or refused the licence). The bar and the mini player say "Couldn't play this video." with the reason on hover.
- **Keys:** Space or K play and pause, ← and → seek 5 s, F toggles the window's full screen, M mutes, Escape leaves full screen then closes, V closes.

## Settings

Settings is a document with marginalia: the left column says what each section is set to before you open it, and the section is a ledger of rows on the plain board whose group headings hang in a left margin beside their rows, not a stack of boxes.

```
Settings (h1)                  Playback (h2 at 24/650)
[ Find a setting       ]       ═════════════════════════════════════ (1px ink)
⚙ General                                    ─────────────────────────────────
  Opens on For you · …                       Widevine                ✓ 4.10…
▶ Playback        ← --active   Sound         ─────────────────────────────────
  High quality · …             (16/600)      Audio quality          [High ▾]
☾ Appearance                                 AAC at 256 kbps, or 64 kbps…
  Light theme                                ─────────────────────────────────
…                              Listening     Private listening          (o )
```


- **The index** (240px, held in view while a long section scrolls where the window is at least 700px tall): the h1, Find a setting, then the sections, each a 16px Lucide icon (`SlidersVertical`, `Play`, `Moon`, `Link2`, `Keyboard`, `FolderClosed`, `BadgeCheck`; the icon takes the name's colour) beside its name at 14/500 over its state in one catalogue line at 12 muted (`High quality · Crossfade 6 s`, `Nothing connected`, `2 changed`), at most two lines. Only cheap data feeds the lines (settings, capabilities, the scrobblers' status); Diagnostics, whose checks cost a run, says what it holds. The current section takes `--active`. Below 960px the h1 and the field share a row, the sections wrap under them as icon and name alone, and the h2 is visually hidden.
- **Find a setting** searches every row's title, its group, its section and a few other words for it (`dark` finds Theme), and every shortcut. While it holds a query the section list gives way to the results (title at 14 in ink, `Section · Group` in meta); choosing one opens the section and puts focus on the row's control, washing the row in `--hover` for two seconds. ↓ goes into the results, ↑/↓ and Home/End move, Enter in the field takes the first, Escape clears; the count is announced.
- **The ledger, with marginalia**: the section's h2 at 24/650 over one 1px `--ink` rule, then groups 32px apart. Wider than 960px each group's h3 (16/600) and its one muted line (13, in the 176px column) sit in a left margin, level with the first row's title, and the rows run in the column to its right, 32px on; a block with no heading of its own (Language, Widevine, Appearance) keeps to that same column, its margin empty, so every row of a section shares one left edge. At 960px and below the h3 stacks above its rows. Every row sits under one 1px `--line` (no box, no tone step): the title at 14/500, what it does at 13 muted (60ch), the control at the right, 40px clear of the text. A row's state that needs attention keeps its own line under the description. The content column is up to 960px.
- **What the platform can't do** keeps its row: the title in `--ink-muted` and, where the control would be described, a `CircleSlash` and why ("Not available on macOS yet.", or the specific reason: no tray to keep playing from, a player that reports plays itself). Nothing disappears quietly. Without any playback, Playback is that one row and the lyrics.
- **Theme** is three segments (System, Light, Dark) in one 32px outlined control, radio buttons underneath: every choice in view, ←/→ to move.
- **Storage** opens with a group, *On this computer*, whose row holds what Sona keeps as three figures at 24/650 with tabular figures: the artwork cache against its limit with a 4px meter in `--ink-muted`, the catalogue cache, the database. The group *Limit and cleanup* keeps only what each does and its action, so no figure is shown twice.
- **Shortcuts** set their groups in two columns where the section is at least 46rem wide, each group whole in one column; rows are the ledger's at 44px. The two columns use the section's full width, so their h3s stay above their rows instead of in a margin.
- **Diagnostics** leads with its verdict at 16/600 beside its icon ("Everything Sona needs is working", or the count of problems), the warnings counted under it, as the first margin note; its three actions are the first row beside it. The checks follow as groups in the margin frame, ledger rows of a 200px name column and the value. Variable names break after an underscore.
- No amber anywhere on the page: it has nothing playing to show.

## Command palette and shortcut sheet

Both are launcher surfaces: raised panels over the scrim, one field at the top, one list, a foot that names the keys. Nothing in them is amber.

- **The palette's rows** are 36px at least with a 32px tile at the left: the command's icon at 16 in `--ink-muted` on `--surface`, radius 6, the same width as a library item's cover so a result list keeps one leading column. The label is 14/400 in ink and the keycaps end the row. A ranked command carries its group ("Playback") as a 13px muted second line, since no heading says it. With nothing typed the places to go (Pages) come first, then Playback, View, Navigation and General, so Enter on opening goes somewhere rather than pausing the music.
- **The sheet** is 820px wide, 10% from the top. The title at 16/600 and a 268px filter field share the head; a hairline, then the groups in two columns. A group is its title at 14/600 over a 1px `--line`, then rows of 32px: the label in ink, and at the right the keycaps (or "Not set" at 12 in `--ink-faint`) in a column whose width is the group's widest chord, so the caps of a group start on one edge. The foot, under another hairline, carries "Change any of them in Settings." at the left and ↓ Browse and Esc Close at the right, at 12 in `--ink-faint`.
- **Filtering** narrows by words in the label, keywords, group or keys; empty groups leave with their rows, and a filter that leaves nothing says "No shortcuts match" with one line of advice. The panel keeps its place and a minimum height while the list shortens.

## Dates

- **The user's own things are dated in calendar words** (`i18n.RelativeDay`): a playlist's "Updated 3 hr. ago", "Updated yesterday", "Updated on Tuesday", "Updated last week", "Updated earlier this month", "Updated in March", "Updated last year", "Updated in 2023". Days are calendar days in the computer's zone, never 24-hour spans, so a change at 23:50 is "yesterday" ten minutes after midnight, and a page left open relabels itself at midnight.
- **The words never stand alone**: the exact date and time ("Tuesday, November 10, 2026 at 2:30 PM", nothing more) is the label's tooltip and its description, and the label is a tab stop on the catalogue line, so the keyboard reaches the tooltip and a screen reader reads the date after the words.
- **Freshness stays terse**: "Synced 4 min. ago", Concerts' and Discover's "Updated 12 min. ago" and Home's "Picked for you …" say how old a copy is, in minutes and hours (`i18n.RelativeTime`).

## Empty, loading and error states

- An empty route keeps its frame: the h1, any column header row it will have, then one dashed-border block (1px `--line`, radius 8) holding a 16/600 sentence-case title, one sentence in the product's own words, and at most one action.
- Loading: the final layout with flat `--hover` bars at the real text sizes; no shimmer, no layout shift when data lands.
- Errors live inside the block that failed: what failed, when data was last good, and a Retry text button. `--danger` only on a small icon.
