# Sona design direction

This is the working visual direction. The tokens below live in `internal/ui/theme/tokens.json`; change them there and here together.

## Thesis: the label's house style

A good record label sets every release through one fixed template: the same typeface at the same station, the same catalogue line, a quiet board, and the cover as the only picture. Sona works the same way. The chrome is a neutral, faintly green-grey board set in one grotesque. The colour on screen comes from the music: cover art, and backdrops pre-blurred from it in Go. The interface itself owns one spot colour, amber, and spends it on one job only: showing where the music is.

What it refuses: the streaming-app default of near-black with a brand-coloured accent on every control, and frosted glass over blurred artwork.

## Principles

1. **Fast before pretty.** Nothing visual may cost first paint or scroll smoothness (`docs/architecture.md` §8): the interface draws on the GPU, and layout, text shaping and image work stay out of every frame.
2. **The cover is the colour.** Chrome stays neutral so artwork reads true.
3. **Amber means "playing".** The now-playing row, the play control, the progress fill, the active queue item. Navigation selection, focus and primary buttons do not use amber. On an album page the cover's own accent does amber's job for that record (Album page, below).
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

- The faint green-grey tint (hue ~150) keeps the board from reading as dead grey and sits well next to any cover. No cream, no navy, no violet.
- The primary button is ink-filled (near-black on light, near-white on dark), not amber.
- Focus ring: 2px `--ink` at 60% (the `ring` token, MyGo's keyboard-focus ring) outside the control, everywhere (≥ 3:1 on every background tone). Selection is amber at 32% (the `selection` token); the caret is `--amber-ink`.
- Contrast floors (WCAG AA, `docs/accessibility.md`): text tokens ≥ 4.5:1 on every background up to `--active` (selected rows), `--control-line` and the focus ring ≥ 3:1. Slider thumbs are always shown: the thumb, not the fill colour, marks the value.
- The kit's role names (`background`, `primary`, `muted`, `ring`, …) are tokens in `tokens.json` too, mapped onto these, so MyGo's widgets and Sona's components read one palette without per-component overrides.

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
- The cover is never cropped, tinted or overlaid with text, with one exception: an album not out yet carries its release date ("Out 19 Nov") in a small `--raised` chip at the cover's bottom-left corner, where the play control of a playable tile sits (it has none). Its page puts the countdown ("Out in 18 days, 6 hours", the day in full, the songs already out) above the actions, re-read every minute, never a ticking counter (Album page, below).
- Hero cards (the untitled "Featured" row) fill Apple's own card shape, 1060:608, with the 4:1 `subscriptionHero` art cropped to it by the CDN, as music.apple.com does: the art keeps its text and subject in the centre, so nothing is letterboxed or cut. A card without a banner shows its cover whole on its background colour. On Discover they are the front page's lead and list (Discover, below). Bricks sit whole on their own background colour: a row takes the shape of its tallest banner (at most 2:1), so only a wider banner in a mixed row is letterboxed.

### People: pictures and initials

A credited person (the song page's Credits) is a 40px circle beside their name: their picture where Apple has one, their initials where it has not.

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

## Home

Home ("For you") answers one question first: what shall I play now. It sets Apple's personal shelves (`internal/ui/features/home/plan.go`) instead of stacking them as Apple sends them, and never copies Apple's own Home.

```
┌───────────────────────────────────────────────────────────┐
│ For you                                        ⟳ Refresh  │  wash of the lead's cover,
│ Picked for you 3 min. ago                                 │  full bleed like a
│ Top Picks for You                                         │  collection header
│ [cover 232]  Your Station        │ [40] Apple Music Live   [40] 1                │
│              Station · Made for… │      New Release · …         More from …      │
│              (▶) (⋯)             │ [40] …                  [40] …                │
├───────────────────────────────────────────────────────────┤
│ Recently Played                                  See all ›│  rows, 2 deep
│ [40] Your Station   [40] Zara Larsson   [40] …   [40] …    │
│ World of Oasis                               Show all 10  │  cover tiles
│ …                                                         │
│ More for you                                              │  the index
│ › Country           [44][44][44][44][44][44][44][44]…     │
│   3 playlists · 9 albums                                  │
└───────────────────────────────────────────────────────────┘
```

- **The lead**: the first "feature" shelf (Top Picks). Its first pick is set like a collection header in miniature: the cover (232px; 168px when the page is under 900px wide), the name at 32/700, the artist or curator at 16/500, then the catalogue line (`Station · Made for You`: what it is, then Apple's reason) and the amber play disc with the item's menu. Every other pick sits beside it as a row (two columns of four; under it when narrow), so the first screen holds all of Top Picks. The page header and the lead sit on the lead cover's pre-blurred backdrop, ground scrim, full bleed; reduce effects leaves the plain ground.
- **Rows** (`internal/ui/features/home/rows.go`): 56px rows with 40px art, the title at 14/500 and Apple's reason with the artist or curator at 13 muted, in as many columns as fit from 264px (248 beside the lead). Recently Played and every shelf of stations only (Stations for You, moods, favourite artists) are rows two deep: you go back to them or play them at once, so a name and a play control matter more than a large cover. On a backdrop the hover is ink at 6%, not the opaque `--hover`.
- **Cover shelves**: the first three plain shelves Apple ranks highest keep their cover tiles (160px; feature shelves 176px). Features, heroes and track shelves always stay in full.
- **The index** ("More for you"): every later plain shelf (genres, "more like", fans also like) is one 64px line: a chevron, the title at 16/600, what it holds at 12 muted ("3 playlists · 9 albums"), and a strip of its covers at 44px. The line is a disclosure button; open, the shelf's cover tiles appear under it and the strip goes. Twelve shelves take one screen instead of four, and their full-size covers load only when asked for.
- **A lone hero** (Apple's untitled featured card): its 1060:608 banner at 40% of the row beside its name at 24/650, the catalogue line and the play disc, never a half-width card with an empty half.
- **Captions** every item of a shelf shares are dropped; said once in the heading, they are noise.
- **States**: signed out, the lead's place holds one dashed block (Recommendations need Apple Music, one sentence, an ink Sign in), then the library's shelves. Offline with no copy of Home, it says there are no recommendations while offline and that Sona fetches them when it reconnects: no Retry, and no Refresh in the header while offline. Apple's Recently Played failing falls back to "Played on this computer", with its own empty line. Loading draws the lead's shape: the cover, the name's lines and eight rows.
- **Keyboard**: the lead's title, play and menu are Tab stops; each row list is one Tab stop (the arrow keys, Home and End move; Enter opens, Space plays, Shift+F10 or the Menu key opens the row's menu; the row focused last is where Tab returns). Index lines are disclosure buttons that tell assistive technology whether they are expanded.

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
│ [art] Title · Artist   ⏮ ⏯ ⏭  0:42 ━━━━━━━──── 3:58   🔈── │  72px surface + backdrop
└──────────────────────────────────────────────────────────┘
```

- Sidebar and top bar share `--surface`; the canvas is `--ground`, divided by one 1px `--line`.
- The mark is a small amber disc with a spindle hole (a record label seen from above), next to the word "Sona" at 16/700.
- Active nav item: `--active` fill at radius 6, label and icon in ink. Inactive items are `--ink-muted`.
- Group label "Library" in 12px muted sentence case, 20px above its first item. Settings sits at the sidebar's foot.
- Below the library links the sidebar lists playlists in four sections, each under a 12px muted sentence-case heading like "Library": **Pinned**, **Made for you** (Apple's personal mixes: the library copies whose catalog ID starts `pl.pm-`), **Apple Music** (playlists the user added that they cannot edit, from Apple Music or another curator; Replay and others) and **Playlists** (the user's own, editable playlists and their folders). A pinned playlist shows under Pinned only, whichever section it belongs to, and a section with nothing left in it is not drawn. Made for you and Apple Music are flat lists with no "New playlist" button, no folders, no drops and no rename; their headings are buttons that fold the list shut (state kept across restarts, `config.Sidebar`), with a chevron that turns, as a folder's does. Playlists keeps the "+" menu and the drop row.
- Top bar: back and forward, then the search field (max 520px, `/` or Ctrl+F focuses it, never narrower than 240px), an empty stretch of at least 64px, and the account avatar.
- The avatar is the account menu's button: a `--hover` circle signed out, `--active` with the glyph in ink signed in, and a small `--danger` dot when the account needs the user (Apple stopped taking the sign-in, the keyring didn't hand it over, Apple refuses Sona's requests). The menu (`--raised`, 288px, end-aligned) opens with where the account stands in meta text (storefront, restricted content, offline, an unencrypted token), then the one action that fits (Sign in or Account settings, Check now, Try again), private listening as a checkbox item with its note, and Sign out in danger red last. Anything that needs room (the sign-in form, the account's details) opens the account dialog, never a page.
- Library lists put their controls on the header's right: the album list has a segmented control for what it shows (All, Albums, Singles & EPs; `Segmented`, the same control as the search scope, 32px tall in a `--hover` track, the chosen one raised) before the Sort select. The choice lives in the URL (`?type=`) beside the sort and keeps its lifetime: Back and Forward return to it, and a reload keeps it. A type with no albums keeps the page and says so, with Show all albums as its one action, rather than the empty-library text.

### Title bar

The sidebar header and the top bar are the window's title bar. Who draws it comes from Go (`Capabilities.WindowControls`, resolved by `internal/windowchrome`; `internal/ui/app/shell.go` creates the window from it), never from a default of the toolkit:

- **macOS**: the system's own traffic lights, inset into Sona's 48px band (hidden title bar, full-size content, centred on the band's height). They keep full screen, tiling, the inactive dimming and VoiceOver. The toolkit reports the room they take (`Context.TitleBar`) and the band keeps it clear at the leading edge (in the sidebar header, or the top bar while the sidebar is hidden).
- **Windows and the stacking Linux desktops** (GNOME, KDE Plasma, Cinnamon, Xfce, MATE, Budgie, Pantheon, …): the title bar is hidden under Sona's band and the window buttons are MyGo's, laid out as the desktop lays out its own (on Linux GTK's `gtk-decoration-layout`: GNOME's default is Close alone; on Windows the caption buttons at the trailing edge), so they read, look and behave as the desktop's own. Leading buttons are kept clear in the sidebar header (or the top bar while the sidebar is hidden); trailing ones in the top bar after the account.
- **Tiling compositors** (sway, Hyprland, i3, niri, river, bspwm, …) and desktops Sona doesn't recognise: the desktop's own title bar or none; Sona draws no buttons and the band is not a drag region.
- Settings → General → **Title bar** (Windows, Linux): Automatic (the hint names the desktop and what Automatic picks there), Sona's with window buttons, Sona's without window buttons, The desktop's. It applies from the next start (the window is created with or without its title bar).
- Where Sona's top bar is the title bar, the band's empty parts drag the window (`DragWindow`) and a double click maximises or restores; every control in the band stays a control. On macOS a double click does what the system's title bar setting says.
- The band keeps the buttons' room clear. Close does what closing the window does (Settings → General: keep playing in the background, or quit).
- The mini player keeps its own Open Sona and Close buttons, on the side where the platform puts Close (leading on macOS and leading desktop layouts), Close outermost, 4px apart.

### First-run setup

The setup is a modal over the live app, not a page of its own: the sidebar, top bar and player bar stay drawn behind the dialog scrim (taking no input), so the first thing anyone sees is the app they are setting up, and each Appearance choice shows at once on the window behind. The card is as tall as its step, so there is no empty column to fill.

```
┌────────┬─────────────────────────────────────────────────┐
│ sidebar│ top bar          (scrim over all of it)         │
│        │      ┌──────────────────────────────────┐       │
│        │      │ ● Set up Sona  3 of 7  Skip setup│       │
│        │      │ ▬▬ ▬▬ ▬▬ ── ── ── ──             │  strip: one Tab stop
│        │      │ Playback                         │  title + one line
│        │      │ What plays your music…           │
│        │      │ [cards] / rows                   │  scrolls inside when short
│        │      ├──────────────────────────────────┤
│        │      │ ‹ Back                  Continue │  64px foot
│        │      └──────────────────────────────────┘       │
├────────┴─────────────────────────────────────────────────┤
│ player bar                                               │
└──────────────────────────────────────────────────────────┘
```

- The card is 640px wide (less 24px a side on a narrow window), `--raised` with the float shadow and, in dark, a 1px white/10 border. The dialog scrim is black/50 in light and black/70 in dark, where a lighter one left the card barely apart from the app behind; no blur. Its top edge is fixed at 10% of the window's height (at least 24px), so the strip and the foot don't move from step to step; it grows downwards to 24px above the window's foot, and past that its body scrolls while the head and foot stay.
- The head names the setup and the place in it ("3 of 7"); the strip under it has one segment per step, filled up to the current one, with each step's name under its segment (the current one in `--ink` and medium weight, the rest muted), so where you are and what comes next read without hovering. The strip is one Tab stop: ←/→ and Home/End move between steps, Enter goes there.
- Each step is a title, one line, and its own parts, not Settings' rows: choice cards for a choice between a few (audio quality, theme, what closing the window does), short switch rows for on/off, a compact Widevine status, and a row per connection. They save through the same queries as Settings, so a choice made here reads the same there.
- Theme is the one picture: three windows in miniature drawn from each palette's tokens (System is both, halved), as choice cards.
- Where the platform can't do something (no tray, no autostart, no notifications) Desktop says so in one line at its foot instead of leaving the reader to look for a missing row.
- One primary button, at the foot's trailing end ("Get started", Continue, Done). On Apple Music, before signing in, the password form's Sign in is that primary, in the foot beside a ghost "Not now", so it stays in view however far the form scrolls (at 940×600 the form runs below the fold); the form's own heading is left out, the step's title names it; Back is a ghost button at the leading end. No amber but the mark and the previews' play controls.
- Every step can be skipped. "Skip setup" ends it at once; Escape asks first ("Skip the rest of the setup?"), except on the last step, where it is Done; a click outside does nothing. The last step lists only what is still to do, each with the way back to its step.

### Discover

Discover (`/new`) sets Apple's editorial front page as Sona's own issue: the featured cards as a front page, then the page's contents, then its shelves grouped by what they hold. It is the one long page in the app (twenty-odd shelves), so it gets a lead to start from and a way to reach any shelf without scrolling or tabbing through the rest.

```
┌──────────────────────────────────────────────────────────┐
│ Discover                                                 │
│ Updated 12 min ago                                       │
│ ┌───────────────────────────────┐  ▭ Title               │
│ │                               │  ▭ Title               │
│ │  lead: 2/3 width, 1060:608    │  ▭ Title     the list, │
│ │                               │  ▭ Title     as many   │
│ └───────────────────────────────┘  ▭ Title     rows as   │
│ Lead title                         ▭ Title     fit       │
│ Caption · curator                         ⏸  1 of 3  ‹ › │
│                                                          │
│ On this page                                             │
│ Songs and albums   Best New Songs · New This Week · …    │
│ Playlists          Updated Playlists · Daily Top 100 · … │
│ Radio · Videos · Browse  (one line each)                 │
│ ──────────────────────────────────────────────────────── │
│ Songs and albums                     24/30, the band     │
│ Best New Songs                       16/600, the shelf   │
│ …                                                        │
└──────────────────────────────────────────────────────────┘
```

- **Front page.** One featured card is the lead, the first to start with: two thirds of the content width, in the hero card shape, its title at 16/600 and its line at 13 muted below. All of them, the lead's too, are a list beside it: rows with the item's 64px square cover (Apple's banners lose their text cropped that small, so art that is not square is never cropped to fill the square: a wide picture, such as a station's 4:1 key art or the banner of a card that has no cover, is shown at the hero card's shape across the slot's width, Apple's centre crop of it as the lead shows it, in miniature, and a tall one fits the slot's whole height; the picture is decoded at the box it shows in, centred in the 64px slot), the title at 14/500 and the line at 13 muted, a `--hover` fill at radius 6 like track rows, and the lead's row on `--active` (described to assistive technology as the current one) as the sidebar marks its page. Choosing a row makes its card the lead (the card's page opens from the lead's title); the list turns to the page the lead is on whenever the lead changes. The list holds as many rows as fit the lead's height, stretched so its pager ends level with the lead's caption. The pager ("1 of 3", 28px chevrons) turns the list only (until the lead changes) and wraps round, so a focused chevron is never disabled under the keyboard. Under 880px of content the list moves under the lead, in two columns of three. Play controls appear on hover and focus as on every tile; a right-click, Shift+F10 or the Menu key opens the item's menu. What the front page shows is left out of the cover shelves below it (a shelf it would empty stays whole), so one album isn't met four times on the way down.
- **The lead rotates.** Every 7 s the lead gives way to the next featured card, wrapping round: the new card fades in sliding 1rem from the right over 400ms while the old one fades out the other way (ease in-out, so the two pictures cross rather than one vanishing first). Nothing blurs or filters, and a one-off 400ms transition plus one deadline every 7 s (no per-frame work) is all the page does. The next card's picture is fetched and decoded while the lead is read, so it fades in as a picture. The interval restarts whenever the lead changes, however it did, so a chosen card is read for as long as any. It holds (and starts a whole interval after) while the pointer is over the hero or focus is anywhere in it, while the window is hidden or unfocused, while the hero is scrolled off screen or covered (by Now Playing, lyrics or a modal); a tick that finds a menu open, or a video playing in the lead (motion artwork is left to play), waits a whole interval more. Under Reduce effects or the desktop's reduced motion it does not run: the lead is still, the pause control is not shown, and the list is the way to change the lead.
- **Pause and play** (WCAG 2.2.2): a 28px ghost button in the pager row, left of the count, with the pause glyph while the lead rotates and the play glyph while it is paused, named "Pause featured rotation" or "Play featured rotation". It is in the Tab order after the list and reflects only the user's choice, not the temporary holds. Pressing it, and turning the list's page, are said once through one polite announcement ("Rotation paused", "2 of 3"); the lead's own changes are never announced, and the lead's outgoing card is an inert copy outside the accessibility tree while it fades.
- **Contents** ("On this page"). One line per band: its name at 14/500 in ink, then its titled shelves at 13 muted with middle dots between, each a button that jumps to its place (no smooth scroll) and moves focus there, so the next Tab is that shelf's first control.
- **Bands.** The shelves are grouped by what they hold, in this order: Songs and albums, Playlists, Radio, Videos, Browse (category bricks and links). Apple's order holds inside each band; a shelf of mixed items goes by most of them (the first item breaks a tie), and a paragraph stays with the shelf after it. Each band opens with one 1px `--line` across the column, 56px below the band before, and its heading 24px under the line at the page-title size, 24/30: the line is what sets a band heading apart from the page's h1, which has none. Inside, shelves keep their usual 40px pitch.
- **Loading.** The freshness line's place holds a bar; the front page's own frame (the lead's block and caption bars, the list's covers and lines at their row height), the contents' lines and the first band's line and heading stand where they will land, then bars for the shelves.

### Album page

A record is two things in the hand: the sleeve, and the listing on its back. The album page lays them side by side instead of stacking a banner over a table.

```
┌ sleeve ─────────────┐   Title (32/36, wraps; never moves the cover)
│ ┌─────────────────┐ │   Artist (in the cover's accent)
│ │                 │ │   Album · 1995 · 40 songs, 2 hrs, 49 mins · Indie Rock
│ │      cover      │ │   [Lossless]
│ │   256 or 320    │ │   Apple's one-line note
│ │                 │ │   status: countdown, or "2 of 13 songs in your library"
│ └─────────────────┘ │   (▶) Shuffle  + Add  Favourite  Suggest less  ⋯
│ Disc 1 · 12 songs…  │   #  Title                                     ◷
│ Disc 2 · 14 songs…  │   Disc 1
│ Released            │   Goldberg Variations (1981 Recording)    ← a work
│ 2 October 1995      │   1  Aria                                    3:22
│ Genres · Written by │   …
│ Label               │   Notes (58ch)
│ ℗ line              │
└─────────────────────┘
Other versions · Music videos · More to hear · More by · Featured in playlists · You might also like
```

- **The sleeve is printed in the cover's colour**: a radius-8 panel in the colour Apple gives for the cover's background (`ArtworkColors.background`; Go's first dominant colour when Apple gives none), flat, with nothing blended at paint time (`internal/ui/theme/palette`; built once per cover, theme and layout, never per frame). It is the page's one coloured surface; the listing sits on the plain ground beside it, so the colour stays with the record. A cover with no colour known keeps the earlier wash: Go's backdrop under the ground scrim with a 4% ink layer. Three rules hold the colour to the board:
  - one tone step off the ground: a cover the colour of the ground (a white one in light, a black one in dark) moves 0.05 in OKLCH lightness away from it, the way the theme's own tones go;
  - in the dark theme no lighter than 0.36 OKLCH lightness, hue kept, so a white or pale cover is a dark tint of itself rather than the brightest slab in the window; in the light theme a dark cover stays dark;
  - never so near a mid grey that black or white loses 5.8:1 on it and its pressed tone, so the text and a muted step below it can both reach 4.5:1.
- **Text on the sleeve** takes Apple's own text colours for the cover (`text1` for ink, `text2` or `text3` for muted) when they read at 4.5:1 on the sleeve and on its hover and pressed tones; otherwise ink is black or white with a trace of the sleeve, and muted is ink mixed 35% towards the sleeve, moved in lightness only as far as 4.5:1 needs. The sleeve redeclares its own `ink`, `ink-muted`, `hover`, `active`, `line`, `control-line` and the roles built on them (`theme.Scope`), so the disc index, badges and outline buttons inside it (the whole head, below 860px) follow it; focus rings there are the sleeve's ink. Reduce effects changes nothing: the sleeve is a flat colour already.
- **The cover's accent takes amber's job on the page**: the most colourful of Apple's colours and Go's dominant ones (OKLCH chroma ≥ 0.06) fills the Play disc, marks the playing row and its equaliser, and sets the artist's name, the record's one link out. Its fill keeps at least amber's own distance from the ground (2:1 in light, 3:1 in dark) and from the sleeve when it sits on it, its glyph 4.5:1; as text it keeps 4.5:1 on every row tone, a selected row included. A greyscale cover keeps amber. (The accent is redeclared as `amber`, `on-amber`, `amber-ink` and the `play` roles for the whole page body, Apple's shelves below included.) Only the album page takes colour from its cover this way: the song, playlist and artist headers keep the backdrop wash of the cover's first dominant colour. Every pair is checked by unit tests over extreme covers (white, black, mid grey, the sRGB primaries, a 6-level grid in both themes).
- **One station**: the cover's top and the title's first line share a line on every album, whatever the title's length; the title wraps instead of clamping. Covers come in fixed sizes so Go serves them at the exact box: 256px, 320px from 1280px of content width.
- **Held while the songs scroll**: the sleeve is sticky beside the listing whenever the pane shows all of it (measured, so a one-disc sleeve holds in a shorter window than a three-disc one), so a 40-song album keeps its cover, disc index and fine print in view. Motion artwork plays there while it is on screen, under the same rules as every header.
- **The back of the sleeve** is the record's fine print in meta type, each fact a muted name over its entry: released (the day in full), first released (the year of its oldest recording, when that is earlier: a reissue, a compilation), genres (all of Apple's, without the root "Music"), written by (every songwriter in Apple's composer credits, most credited first, eight named and the rest counted) or, on a classical album, composer, then the label unless the ℗ line names it, and the ℗ line last.
- **The listing is 2/3 of the width**, not the whole pane: at 1440 the title-to-duration scan drops from about 980px to 680px.
- **The disc index**: each disc in the catalogue line's form ("Disc 2 · 14 songs, 1 hr, 3 mins"); choosing one brings its heading to the top and focuses its first song. A list long enough to be virtualised (over 200 songs) has no disc headings, so no index either.
- **Classical albums read by work**: where Apple names the work a track belongs to, each work gets a heading (14/500 ink, under the disc's) and its rows show the movement alone ("Aria", not "Goldberg Variations, BWV 988 (1955 Recording): Aria"); the row's play control and menu still name the song in full, and the queue and now playing show its full name. An album of several composers leads each work with its composer and names the composer under a piece outside any work.
- **Rows explain themselves**: the line under a title names a credit other than the album artist's (an Artist column only for a real compilation, where most credited songs are someone else's; a song with no credit yet counts for neither) and why a song doesn't play: "Out 19 Nov" on an album not out yet, "Unavailable", or the account's explicit restriction. An unpublished song's stand-in artist ("unknown") is dropped, and "Popular" only marks a song that plays.
- **The catalogue line describes the record**: a library album holding some of its songs counts the whole album and says "2 of 13 songs in your library · Show complete album" in the status slot; an album not out yet leaves out a running time its unpublished songs can't add to.
- **Not out yet**: the countdown takes the status slot; Play plays the songs already out (named "Play the 8 songs out now"), beside Remind me and Pre-add.
- **Below 860px of content** (a 940px window) the sleeve heads the page with the title beside a 168px cover, the songs follow at full width, then the fine print on the ground; the disc headings in the list stand in for the index.
- Then Notes at a reading measure (58ch, about 75 characters at the body size), and Apple's shelves in Apple's order, with the radio episodes about the record ("More to hear") after its music videos.

### Artist page

An artist page is the label's file on the act: who they are, what to play first, then the back catalogue set out by format, then the company they keep. It answers "who is this" on the first screen and never repeats a release to fill space.

```
┌ header (backdrop wash) ──────────────────────────────────────────────┐
│ ( portrait )  Radiohead (32/36)                       ┌──────┐ Title  │
│               Artist · Alternative · Abingdon, …      │ 112  │ Latest release · 13 Aug 2025
│               the biography's first paragraph, 2 lines└──────┘ Album · 12 songs
│               Read more                                               │
│               (▶) Shuffle  ☆  Suggest less  ⋯                         │
└──────────────────────────────────────────────────────────────────────┘
Top songs (6 rows)                          See all │ Essential albums (300px)
#  Title                       Album            ◷   │ ▭ In Rainbows · 2007
…                                                   │ …
──────────────────────────────────────────────────────────────────────
Discography (24/30)
Albums: one row of cover tiles                              See all
Singles and EPs  See all │ Live albums        │ Compilations
▭ Daydreaming            │ ▭ Hail to the …    │ ▭ KID A MNESIA
  2016 · Single          │   2025             │   2021
Music videos: one row of 16:9 tiles
──────────────────────────────────────────────────────────────────────
Related: Appears on (a list) · Playlists featuring · Similar artists
──────────────────────────────────────────────────────────────────────
About Radiohead: the whole biography (32rem) │ From · Born or formed · Genres
```

- **The header** is the artist's file: the round portrait (200px, 136px under 720px of content), the name, the catalogue line in its usual form with the kind first (`Artist · Alternative · Abingdon, Oxfordshire, England`; `Artist · 1 release` when the page holds every release of a small act), the biography's first paragraph at 13 muted, clamped to two lines at a 32rem measure, and "Read more", which scrolls to About and moves focus there. From 900px of content the latest release stands in a 300px second column as a slip: its 112px cover, the title, `Latest release · 13 Aug 2025` and `Album · 12 songs` in meta type, Apple's " - Single" / " - EP" suffix moved into that line. Narrower, the slip sits under the portrait and text with a 56px cover. An act whose one release is the latest has no slip: the discography holds it.
- **The lead**: top songs (six rows; the rest behind See all) with the essential albums beside them in a 300px column, as a release list. Under 900px the essential albums follow the songs in two columns.
- **Release lists**: the back catalogue's form for singles, live albums, compilations, essential albums and Appears on. Rows of 52px: a 40px cover, the title at 14/500 and a 13 muted line (`2016 · Single`, or the artist and year for Appears on), a `--hover` fill at radius 6 that reaches 8px past the covers so they line up with the heading. Each list shows five releases (essential albums six) and See all for the rest. They sit side by side in up to three columns of at least 260px, 32px apart, the same columns in every band so the lists line up. Rows open, select, drag and open their menu (right-click, Shift+F10, the Menu key) like cover tiles; ↑ and ↓ move between them.
- **Bands**, as on Discover: Discography (albums as one row of cover tiles, the release lists, music videos), Related (Appears on, playlists featuring the artist, similar artists) and About (the whole biography at a 32rem measure, the facts beside it). Empty shelves and bands are left out.
- **Loading** holds a major act's final frame: the header's portrait, name, line, two biography lines, Read more, actions and slip, then top songs and the essential albums as loading rows of the real lists. For such an act nothing moves when the data lands, but the scrollbar arriving with it narrows the column by its width. A small act with no biography or slip has a shorter header, so under 900px of content its songs move up when they load.
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
- **The list**: the page title and count (24/30 and 13 muted; `5 of 90 artists` while filtering), the filter (32px, the Shortcuts filter's field), then the artists A to Z under a letter heading each (12/600 muted: the letter itself, not a label), `#` last for digits, symbols and other scripts. Rows are 44px with a 32px round portrait (the artist's own, else their newest album's cover, else the placeholder), the name at 14/500 and what the library holds of them at 12 muted (`12 albums, 130 songs`, tabular figures). The open artist is a `--active` fill at radius 6; hover is `--hover`. Heights are fixed, so the list, which builds only the rows in view and a few beyond in its own scroll box, never measures: thousands of artists scroll like ninety.
- **The filter** matches as you type, ignoring case and accents (`bjork` finds Björk, `edith` Édith Piaf, `sorensen` Sørensen), best first: names starting with it, then names with a word starting with it, then any substring, then the name without its spaces and punctuation (`acdc` finds AC/DC). Filtering drops the letter headings, since the order is the match's. The artists come from one query (`LibraryService.ArtistIndex`: name, portrait, album and song counts, from SQLite only), so filtering never calls Go and never waits.
- **The detail** is the open artist's library, not the catalogue: the portrait (96px), the name at 32/36, the catalogue line (`Artist · 12 albums, 130 songs`), the amber play disc, Shuffle, **Full artist page** (the artist's own page, with the catalogue: biography, top songs, the whole discography), the artist's menu, then **Albums** as a cover grid, newest release first, and **Singles and EPs** under their own heading (Apple's ` - Single` / ` - EP` title suffix, or the album's single flag, decides; the suffix moves to the tile's line: `2020 · Single`). Play and Shuffle read the artist's songs from SQLite when pressed. An artist with songs but no albums shows their songs (Songs in your library, the track table) where the albums would be.
- **In the URL**: the open artist is `?artist=<library id>`, so reload, Back and Forward keep it. With none (or one not in the library) the first match is open beside the list, without writing the URL. Clicking an artist adds a history entry; moving with the arrow keys replaces the entry, so a walk down the list is one step back, not fifty.
- **Keyboard**: the filter (a combobox) and the list (a listbox, the second Tab stop) share their keys. ↑/↓ move the open artist (PageUp/PageDown by ten, Home/End on the list), Enter or → (from the end of the filter's text) go into the detail: focus lands on its first album, or on its first control while the albums are still being read. Escape clears the filter, and from the detail returns to it. Typing on the list types into the filter. The open artist is the active descendant of both while its row is built; rows tell assistive technology their position and the size of the set. The count of matches is announced.
- **Narrow windows** (the page under 880px wide, which is a 940px window with the sidebar open): the list and the detail take turns. The list fills the page; choosing an artist (a click or Enter, which opens the filter's top match when nothing is marked) shows the detail with a ghost **‹ Artists** button above it; the artist's name is then the page's h1. The arrow keys move a cursor in the list without opening anything. The list stays mounted while hidden, so its scroll and filter survive. Back (the button or the browser's) returns to it with focus in the filter and the artist you left still marked; Escape in the detail moves focus to the button.
- **States**: signed out, syncing, empty and error are the library gate's block in place of both columns (`Artists` title above it). Loading draws fourteen list rows. An artist's albums loading draw six cover tiles; failing, an inline error with Retry. No match: "No artist matches “xyz”" under the filter, and no detail open.

## Now Playing

The record held open: the sleeve on one side (the cover and its label copy), the insert on the other, one at a time: the lyric sheet (Lyrics, below) or the running order still to play (Up next). It covers the page column, so the sidebar, the top bar and the player bar with its transport stay where they are, over a wash of the cover (Backdrops).

```
┌ page column, the cover's wash ───────────────────────────────────────┐
│ ┌──────────────────┐    Lyrics   Up next                         ✕  │  tabs at 16/600, Close
│ │                  │    ──────                                       │
│ │  cover 400       │                                                 │
│ │  (320, 240)      │    the pane: the lyric sheet,                   │
│ │                  │    or the queue's rows                          │
│ └──────────────────┘                                                 │
│ SKINNY (32/36)                                                       │
│ Billie Eilish (16/500)                                               │
│ HIT ME HARD AND SOFT (13)                                            │
│ [E] [Preview]                                                        │
│ ☆  ⊘  ⓘ View credits     Translation …   − Lyrics on time + ⤢ Full screen │  the pane's own strip
└──────────────────────────────────────────────────────────────────────┘
```

- **The sleeve.** The cover at a fixed size so Go serves it at its exact box: 400px from 1000px of column and 720px of height, 320px below either, 240px in a short window. Under it the label copy: the title at 32/36 (24/30 under a 320px cover), wrapping to three lines; the artist at 16/500 and the album at 13 muted, both links out that close the view; the marks the bar also shows (Explicit, Preview); then the song's own actions: Favourite, Suggest Less and View credits (the song page). A station shows its name, a Live line with the dot (amber while it plays) and what is on air, as the stream or the schedule names it. A local broadcaster is headed by the song its stream names on air (that song's cover, its artist under it), with the station, its genre and place, its description and an Open website button below; a music video shows its 16:9 frame at the cover's width and a Show video button.
- **The insert.** Tabs named in words (Lyrics, Up next), the shown one in ink over a 2px ink rule: not amber, which stays with what plays. A station or a song outside the catalogue has no lyrics, so its row holds only the heading Up next. The view reopens on the pane it showed last; the bar's Lyrics opens it on the lyrics and its Queue (or Q) on Up next, and each closes the view when its pane is already showing. The queue beside the page steps out while the view is open and comes back when it closes, so the queue is never drawn twice or hidden under the view.
- **Each pane's controls sit at its foot**, as full screen keeps its own: the lyric sheet's Translation and Pronunciation at the start, the timing nudge and Full screen at the end; Up next's line ("9 songs after this one", or the station it plays from) at the start, Clear and AutoPlay at the end. The tab row holds only the tabs and Close, so the column opens on the words.
- **One text edge.** The tabs, the lyric lines, the queue's covers and the foot's line all start 12px into the column. Up next is the queue's own list (rows, keys, drag and drop); on the wash its hover and focus fill is ink at 6%, not the opaque `--hover`, and the playing row's equaliser holds still under the view like every other (Lists).
- **The sheet's reading band** is measured from the sheet's own height, not the window's: the first line starts 14% down, so it sits at the band's top however much the header above takes.
- **Narrow** (under 880px of column, a 940px window with the sidebar): the sleeve becomes a header, the cover at 168px beside the label copy and Close at its end, over the tabs and the pane at full width.
- **No lyrics**: the usual dashed block ("No lyrics for this song", the reason) with one link, Lyrics sources, to Settings → Playback.
- **Cost.** Nothing new moves, and nothing beneath it is drawn: the page under the view is hidden (it stays built, so it keeps its scroll, but is not drawn and takes no input), so a motion cover never repaints the album page's own header under it. The layout follows the view's own box as last drawn and changes only when its size class does. The pane not shown is not built, so on Up next the lyrics' timers are gone. Motion artwork plays on the cover from one rendition (asked for at 420 DIPs, so Apple's 960 px picture at 2x, never narrower than the 400 DIP cover) at every size, so a resize never reloads it.
- **Focus.** Opening the view focuses the shown tab (Close when there is one pane); leaving full screen returns to Full screen; Escape closes the view, from anywhere in it (a tooltip showing on the focused button takes the first Escape, as everywhere).

## Lyrics

A lyric sheet, the insert in a record's sleeve: the words set in verses down one column in the house type, with amber marking only where the voice is. The now-playing view's lyrics column and full-screen lyrics share it (`internal/ui/features/player/lyrics_sheet.go`, with the timing logic in `internal/ui/lyricsync`); the mini player shows the line being sung (`internal/ui/features/miniplayer/sungline.go`).

- **One sung line.** The sung line is the only line in `--ink` (timed by line only, it is `--amber-ink` itself); earlier lines are `--ink-faint`, later ones `--ink-muted`, plain lyrics `--ink`. Word-timed lyrics light each word `--amber-ink` as the voice reaches it. Short words light whole as they begin; held notes (600 ms or longer) fill in the reading direction, stepped about 12 times a second behind a soft 0.5em edge. Lines change colour without a transition.
- **Verses.** A pause of 3 s or more between one line's end and the next, or a change of the song part the lyrics name (Apple's "Verse", "Chorus"), starts a verse: a 32px gap in the column, 0.9em full screen. While the voice rests in a break (from 0.5 s after the line's end) nothing is lit.
- **The sheet turns; it doesn't crawl.** While the sung line sits in the reading band (14–72% of the sheet's height) nothing moves. When it leaves, the sheet scrolls once so its verse starts at the band's top (the line itself when the verse is taller than the band); in a break it turns to the verse coming. Smooth, or a jump under reduced motion; a wheel, touch or key scroll by the user is left alone for 4 s.
- **Edges.** The sheet fades out over its top and bottom edges (3rem in the column, 12% of the window's height full screen), so a line leaving the view never shows clipped glyphs. It draws nothing while the sheet holds still.
- **Under each line**, smaller and in `--ink-muted`, each on its own line: the background vocals (600: they are sung, and they light on their own timing, so they can still be filling under the next line), then, when switched on in the view's bar, the romanisation (400, lit like the line when it is word-timed) and the translation (400) in the language chosen in Settings → Playback → Lyrics.
- **Duets**, as a lyric sheet sets them, from one reading edge: the second voice is indented 1.5em and set in italic; lines every voice sings stay at the margin, upright, the first of each such passage headed "Together" in meta type, `--ink-muted`. Never voices on opposite sides of the screen.
- **Full screen**: room type (4.4% of the window's width, between 36 and 84px, 700, -0.02em), the sheet a 24ch column centred on the screen, the song and the timing nudge in the quiet strip at the foot. A failed load is the usual error block with Retry.
- **The mini player** puts the sung line in the album's place: up to two lines at 13/18 in `--ink` after the 12px lyrics glyph, its words lit whole in `--amber-ink` (no fill: a glance needs no frames). Before the first line, in breaks and for songs without synced lyrics it shows the album in `--ink-faint`. It keeps the main window's per-song timing.
- **Keyboard.** In the column the lines are one Tab stop (the sung line, or the one focused last): ↑/↓ move, Home/End go to the ends, Enter or Space seeks there.
- **Motion and cost.** Nothing runs a frame loop: the words change on timers (`docs/ui-architecture.md`, "Now Playing and lyrics"). Under reduced motion or reduce effects every word lights whole as it begins.

### Radio

Radio (`/radio`) answers what radio alone can: what is on air right now, and for how long. Apple's live stations open the page as a now-and-next board drawn from their schedules; the local broadcasters follow, as they are live too; then Apple's radio page in bands, with the stations to tune to before the shows to listen back to.

```
┌──────────────────────────────────────────────────────────┐
│ Radio                                                    │
│ Live now                                                 │
│ ▣ Apple Music 1                ▶  ▣ Apple Music Hits   ▶ │
│   The Zane Lowe Show                Songs you know…      │
│   10:00–12:00 ━━━━──── 1 hr left    08:00–10:00 ━━── 42… │
│   Next at 12:00: Rebecca Judd                            │
│ (6 stations, two columns at 1440, one at 940)            │
│                                                          │
│ Local Broadcasters   (the shared station list)           │
│ ──────────────────────────────────────────────────────── │
│ Stations             Your stations · Top · by genre      │
│ Shows and episodes   episodes, interviews, shows, hosts  │
│ Browse               more to explore                     │
└──────────────────────────────────────────────────────────┘
```

- **Live now.** The section's heading is at the bands' size (it is the first of them, with no line above it), and the local broadcasters' list sits inside it. One row per live station, 96px with an 8px inset, `--hover` fill at radius 6 like track rows (on hover and while a control in the row has focus), in a grid of 420px minimum columns (two at 1440, one at Sona's narrowest, three on a wide screen). The art is the programme's own square from the schedule (the station's when it has none) at 80px. Beside it, on fixed lines so every row's bar sits at the same height: the station's name at 14/500 (a link to its page over the whole row), what is on at 13 in ink, then the slot, a 2px progress bar and the time left ("1 hr, 18 mins left") in meta, and the next programme with a name of its own ("Next at 11:00: The Matt Wilkinson Show", or "Later at 20:00: The Ebro Show" when the station's own hours come between). A slot named after the station shows its description, or the tagline when that is all it says. The station's own hours count as one stretch with the slots around them, so the slot and the time left run to the next named show and no hour goes unaccounted for; when those hours run to the end of the schedule there is nothing to count down to, and the row shows the tagline alone. A show's back-to-back slots are one stretch too. The board reads the clock every 30 seconds and moves on by itself when a slot ends; the schedules are the station pages' own queries, so opening a station is instant.
- **Play and state.** The play control is always shown at the row's end (a 32px ghost circle): picking a station is the page's job, so it is not left to hover. The station playing takes amber where the board shows it: its name in `--amber-ink`, the equaliser in place of the play control, and the progress fill in amber while it plays. A station Sona can't play has no play control; its row says why where the slot would be, with the `CircleSlash` icon.
- **Stations with nothing to open.** A station opens a page only when the page has something to show: a live station's schedule, an episode's show, the songs a track station plays first. A local broadcaster has no schedule, so it has no page: its row (the shared station list) is a button, "Play {name}", the name stretched over the row, and the menu is beside it (Play, Copy link); the play mark is a hover cue, not a second control. While it plays the name button pauses it ("Pause {name}"), the equaliser stands in the mark's place, and the second line is the song on air ("Artist – Title", from its stream) instead of the tagline. A live station whose schedule turns out empty is treated the same. Cover tiles do likewise (the tile is the button, named "Play {name}"); a link and a button are never mixed in one tile.
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
- **Loading and errors.** The board's frame (rows at their height, the art and lines as bars) and the first band's line and heading stand where they will land. A block that fails says so in place; when nothing loads, the page says it once, with one Retry.

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

Concerts (`/concerts`) answers one question: what is on near me, and when. A gig is a date, a place and a ticket, so the page is an agenda set by the venue's own calendar, not a grid of covers, and it is the one list in the app that is read down by date.

```
┌──────────────────────────────────────────────────────────┐
│ Concerts                                  (⌖ Set location)│
│ Near London, the main city of your… · Updated 5 min ago   │
│ [Any date ▾] [All genres ▾] ◯ Your artists (3)            │
│                                                          │
│ November 2026                                  16/600    │
│ Tue   (●) Stevie Wonder                  Get tickets ▾   │
│  3        The O2 · London                                │
│           18:30 · 6 mi away                              │
│ Thu   (●) Skepta                         Get tickets ↗   │
│  5        The Cause · London                             │
└──────────────────────────────────────────────────────────┘
```

- **The agenda.** Months under a 16/600 heading, then days: the date column (weekday at 12 muted over the day number at 24/650, tabular) once per day, with that day's shows as rows to its right. The day, month and clock are read in the venue's own time zone, in the interface's language; a show whose clock is not this computer's carries its zone's name ("8:00 pm EDT"), so a gig in another country is never read as local. Shows are Apple's, soonest first; there is no popularity ordering to explain.
- **A row** is 72px with the headliner's portrait (56px, round like every artist), the line-up at 14/500 (a link to the artist), the venue and city at 13 muted, and the start and the straight-line distance from the chosen city at 12 faint (`18:30 · 6 mi away`, miles where the locale uses them). A show by an artist the user follows says so in that line. Its one action is **Get tickets**, a ghost button: Apple lists the sellers, and one seller opens its https page in the browser (a trailing arrow), several are a menu of them (a chevron); a show with none shows no button. `--hover` fill at radius 6, as track rows.
- **Where.** The header's outline button names the chosen city ("Manchester"), or says "Set location" while the page is on the default, and the line under the title says what the default is (the storefront's main city). It opens one small dialog: a search field and Sona's own list of cities (the storefront's first, then well-known ones), with a sentence saying the choice stays on this computer and Apple Music is told only the area. ↓ moves from the field into the list, Enter takes the first match, Escape closes and returns focus to the button; "Use my storefront's main city" undoes a choice.
- **Filters** are one row of 32px controls: dates (any date, this week, this weekend, next 30 days, or two date fields), genre, and a switch "Your artists (n)" that appears only when the signed-in account has shows in this area. They live in the address (`when`, `from`, `to`, `genre`, `yours`), so Back returns to them. A change keeps the list on screen until the new one arrives.
- **Long lists.** Asked 100 at a time (Apple has no paging, so "Show more concerts" asks for 100 more, up to 500). The agenda is one Tab stop: ↑/↓ move between rows, ←/→ between a row's link and its tickets, Home/End jump, Tab leaves.
- **States.** Loading draws rows at their height; no concerts near the city (some countries have none) says so with Apple's "No concerts" and "Set location"; filters that match nothing say that with "Clear filters"; a failed load is the usual error block with Retry, and a stale copy says when it is from.
- **Elsewhere.** An artist's page carries "Upcoming concerts" between its lead and its discography: three rows with the date column first (the artist's portrait and name are left out, the venue leads), the ones near the chosen city first, "Show all n", and a link to the agenda. It is left out when the artist has none. Home carries "Concerts in {city}" where Apple puts its concerts shelf: six rows in two columns and "See all". Apple's own concerts links open this page.
- No amber: nothing here is playing.

## Video view

A music video takes the whole window, like full-screen lyrics: the picture is the page (`internal/ui/features/video/view.go`). It opens when a video starts, and again from the bar's cover or "Show video", Now Playing's "Show video" or V; closing it leaves the video playing in the bar.

- **Strips.** A 64px title strip: the video's name at heading size (a link to its page), the artist in `--ink-muted` (a link), the SD badge, a status line in meta type ("Loading the video…" before the first picture, "Buffering…" when pictures stop), then Full screen and Close. A foot strip: the seek bar with elapsed and remaining time, Previous, the 44px amber Play/Pause disc, Next, and the bar's volume control at the right.
- **The picture** keeps its own shape inside the stage, black behind the picture only, never behind the strips; the stage is the theme's `--ground`, light or dark. Before the first picture the video's artwork stands in at 16:9.
- **Fading.** While a picture plays, the strips fade after 3 s with no pointer or key (and the pointer hides); any movement or key brings them back. They stay while paused, loading, failed, or while a control has keyboard focus. Reduced motion makes the fade instant.
- **SD badge.** The house `Tag` reading "SD", its title explaining that Apple allows HD only on players with hardware protection. Shown while the video plays, hidden in an error state.
- **Error states** replace the picture with the usual dashed error block: what failed ("Couldn't play this video", or "Music videos don't play here" where Sona can't), the reason in Sona's words, "Watch in Apple Music", and "Try again" unless trying again can't help (Apple offers the video only in HD, or refused the licence). The bar and the mini player say "Couldn't play this video." with the reason on hover.
- **Keys:** Space or K play and pause, ← and → seek 5 s, F toggles the window's full screen, M mutes, Escape leaves full screen then closes, V closes.

## Settings

Settings is the label's index card: the left column says what each section is set to before you open it, and the section itself is a ledger of rows on the plain board, not a stack of boxes.

```
Settings (h1)                  Playback (h2, the same size, on the same line)
[ Find a setting       ]       ──────────────────────────────────────────────
General                        Widevine                         ✓ Version 4.10…
Opens on For you · Keeps…      ──────────────────────────────────────────────
Playback          ← --active   Sound (16/600)
High quality · Sound Check     ──────────────────────────────────────────────
Appearance                     Audio quality                    [High · 256 ▾]
Light theme                    AAC at 256 kbps, or 64 kbps…
…                              ──────────────────────────────────────────────
```

- **The index** (240px, held in view while a long section scrolls where the window is at least 700px tall): the h1, Find a setting, then the sections, each its name at 14/500 over its state in one catalogue line at 12 muted (`High quality · Crossfade 6 s`, `Nothing connected`, `2 changed`), at most two lines. Only cheap data feeds the lines (settings, capabilities, the scrobblers' status); Diagnostics, whose checks cost a run, says what it holds. The current section takes `--active`. Below 960px the h1 and the field share a row, the sections wrap under them as names alone, and the h2 is visually hidden.
- **Find a setting** searches every row's title, its group, its section and a few other words for it (`dark` finds Theme), and every shortcut. While it holds a query the section list gives way to the results (title at 14 in ink, `Section · Group` in meta); choosing one opens the section and puts focus on the row's control, washing the row in `--hover` for two seconds. ↓ goes into the results, ↑/↓ and Home/End move, Enter in the field takes the first, Escape clears; the count is announced.
- **The ledger**: the section's h2 at 24/650, then groups 40px apart, each an h3 at 16/600 (with one muted line about the group when it needs one) over its rows. Every row sits under one 1px `--line` (no box, no tone step): the title at 14/500, what it does at 13 muted (60ch), the control at the right, 40px clear of the text. A row's state that needs attention keeps its own line under the description.
- **What the platform can't do** keeps its row: the title in `--ink-muted` and, where the control would be described, a `CircleSlash` and why ("Not available on macOS yet.", or the specific reason: no tray to keep playing from, a player that reports plays itself). Nothing disappears quietly. Without any playback, Playback is that one row and the lyrics.
- **Theme** is three segments (System, Light, Dark) in one 32px outlined control, radio buttons underneath: every choice in view, ←/→ to move.
- **Storage** opens with what Sona keeps on this computer as figures: the artwork cache against its limit with a 4px meter in `--ink-muted`, the catalogue cache, the database. The rows under them keep only what each does and its action, so no figure is shown twice.
- **Shortcuts** set their groups in two columns where the section is at least 46rem wide, each group whole in one column; rows are the ledger's at 44px.
- **Diagnostics** leads with its verdict at 16/600 beside its icon ("Everything Sona needs is working", or the count of problems), the warnings counted under it, and its three actions at the right; the checks follow as ledger rows of a 200px name column and the value. Variable names break after an underscore.
- No amber anywhere on the page: it has nothing playing to show.

## Empty, loading and error states

- An empty route keeps its frame: the h1, any column header row it will have, then one dashed-border block (1px `--line`, radius 8) holding a 16/600 sentence-case title, one sentence in the product's own words, and at most one action.
- Loading: the final layout with flat `--hover` bars at the real text sizes; no shimmer, no layout shift when data lands.
- Errors live inside the block that failed: what failed, when data was last good, and a Retry text button. `--danger` only on a small icon.
