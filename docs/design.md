# Sona design direction

This is the working visual direction. The tokens below live in `frontend/src/index.css`; change them there and here together.

## Thesis: the label's house style

A good record label sets every release through one fixed template: the same typeface at the same station, the same catalogue line, a quiet board, and the cover as the only picture. Sona works the same way. The chrome is a neutral, faintly green-grey board set in one grotesque. The colour on screen comes from the music: cover art, and backdrops pre-blurred from it in Go. The interface itself owns one spot colour, amber, and spends it on one job only: showing where the music is.

What it refuses: the streaming-app default of near-black with a brand-coloured accent on every control, and CSS glass over blurred artwork.

## Principles

1. **Fast before pretty.** Nothing visual may cost first paint or scroll smoothness on WebKitGTK (`docs/architecture.md` §8).
2. **The cover is the colour.** Chrome stays neutral so artwork reads true.
3. **Amber means "playing".** The now-playing row, the play control, the progress fill, the active queue item. Navigation selection, focus and primary buttons do not use amber.
4. **Native to the desktop.** Light and dark follow the system; it works with the keyboard; it is dense enough to sit tiled at half width beside an editor.
5. **Honest states.** Signed out, syncing, offline, no CDM: each is said plainly where the missing data would be.

## Type

One family: **Schibsted Grotesk** (variable, self-hosted via `@fontsource-variable/schibsted-grotesk`, no network fetch). It is a news grotesque, so it stays sharp at 12–14px in dense lists and has character at title sizes. Fallback: `system-ui, sans-serif`.

| Role             | Size / line       | Weight  | Use                                        |
| ---------------- | ----------------- | ------- | ------------------------------------------ |
| Collection title | 32 / 36, -0.02em  | 700     | Album, artist, playlist name in its header |
| Page title (h1)  | 24 / 30, -0.015em | 650     | For you, Songs, Settings                   |
| Section          | 16 / 22           | 600     | Shelf titles, settings section titles      |
| Body             | 14 / 20           | 400–500 | Nav, row titles, form labels               |
| Secondary        | 13 / 18           | 400     | Artist and album columns, descriptions     |
| Meta             | 12 / 16           | 450     | The catalogue line, durations, timestamps  |

- Six sizes, three weights in practice (400, 600, 700). No uppercase labels, no letter-spaced small caps, no monospace.
- Every number that updates or aligns (durations, track numbers, counts, progress) uses `tabular-nums` (utility `.num`).
- **The catalogue line**: every collection header and the now-playing bar carry one meta line in the same form: `Album · 2008 · 10 songs, 42 min`. Middle dots, sentence case, tabular figures. It is the label's catalogue number and appears in exactly this form everywhere.

## Colour

Strategy: restrained chrome, colour from artwork, one spot colour with one job. All text tiers reach 4.5:1 on the surface they sit on.

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
- Focus ring: 2px `--ink` at 60% outside the control, everywhere (≥ 3:1 on every background tone); ShadCN `ring-*` focus styles use it at full strength, never `/50`. Selection is amber at 30%; the caret is `--amber-ink`.
- Contrast floors (WCAG AA, `docs/accessibility.md`): text tokens ≥ 4.5:1 on every background up to `--active` (selected rows), `--control-line` and the focus ring ≥ 3:1. Slider thumbs are always shown: the thumb, not the fill colour, marks the value.
- ShadCN's variables (`--background`, `--primary`, `--muted`, …) are mapped onto these tokens in `index.css`, so stock components inherit the palette without per-component overrides.

## Space and density

- 4px base. Control height 32px (top-bar search 32, buttons 32, compact icon buttons 28).
- Shell: sidebar 232px; top bar 48px; now-playing bar 72px spanning the full width; content gutter 32px (24px under 960px wide).
- Nav rows 32px on a 34px pitch, 16px outline icons at 1.5 stroke, 10px to the label.
- Track rows 44px with 32px artwork (a later "compact" density setting moves rows to 32px, artwork hidden, type unchanged). No zebra striping and no row dividers: hover is a `--hover` fill at radius 6.
- Album and playlist grids: cover tiles from 160px, gap 24px horizontal, 28px vertical, title 14/500 and artist 13 muted under the cover.
- Minimum window 940×600; the layout holds at half of a 1920 screen.

## Shape and depth

- Tight radius family: artwork 4px, controls 6px, menus and dialogs 8px. Avatars and the round play button are circles. Nothing else is pill-shaped.
- Surfaces separate by a tone step, or by one 1px `--line`, never both. Shadows only on what floats (menus, dialogs, toasts): `0 8px 24px rgb(0 0 0 / 0.12)` light, `0 8px 24px rgb(0 0 0 / 0.4)` dark.
- No gradients, no glows, no glass.

## Artwork

- Square, 4px radius, a 1px line at 6% ink just inside the edge so pale covers do not dissolve into the ground: an outline on the image (`-outline-offset-1`), not a ring on a pseudo-element, because WebKitGTK repaints the visible page on every scroll step and the ring cost about a quarter of a frame's paint. Video frames are 16:9; hero cards are Apple's 1060:608; bricks and page banners keep their own 2.4:1 or 4:1 shape.
- Always requested from Go at the exact rendered box (width and height × the screen's density, at most 2; grid tiles at their exact width, never rounded up, so the art is never resampled while painting) through `/art/<key>/<w>x<h>.jpg` URLs, sized in one place (`shared/lib/art.ts`: `artUrl`), `loading="lazy"`, `decoding="async"`, explicit `width`/`height` so rows never shift.
- Missing art: a `--hover` square with a 40%-ink outline music glyph centred, at the same size.
- The cover is never cropped, tinted or overlaid with text, with one exception: an album not out yet carries its release date ("Out 19 Nov") in a small `--raised` chip at the cover's bottom-left corner, where the play control of a playable tile sits (it has none). Its page puts the countdown ("Out in 18 days, 6 hours", the day in full, the songs already out) above the actions, re-read every minute, never a ticking counter (Album page, below).
- Hero cards (the untitled "Featured" row) fill Apple's own card shape, 1060:608, with the 4:1 `subscriptionHero` art cropped to it by the CDN, as music.apple.com does: the art keeps its text and subject in the centre, so nothing is letterboxed or cut. A card without a banner shows its cover whole on its background colour. On Discover they are the front page's lead and list (Discover, below). Bricks sit whole on their own background colour: a row takes the shape of its tallest banner (at most 2:1), so only a wider banner in a mixed row is letterboxed.

## Backdrops (pre-blurred in Go)

The artwork service renders, per cover, a tiny pre-blurred JPEG (target ~32×32, heavy Gaussian blur, saturation kept). The frontend never blurs anything itself.

- **Where**: behind the now-playing bar, the mini player, the now-playing view, full-screen lyrics, collection headers (playlist, artist), the album page's sleeve, and the top of Home (from its lead pick).
- **How**: an `<img>` (component `Backdrop`) absolutely positioned, `object-fit: cover`, scaled up by the browser (bilinear upscaling of the tiny image is the softness), under a solid scrim of the surface colour (bar, mini player) or the ground colour (views, headers) at `--backdrop-scrim`: 92% in light, 86% in dark. The result is a wash of the cover's colour, never a recognisable picture. Text sits on the scrim, so contrast is the scrim's, not the image's: the scrim is the floor at which `--ink-muted` and `--ink-faint` stay ≥ 4.5:1 over any backdrop, a black one in light (the binding case: 91% on surface) and a white one in dark (85%). It is one value per theme, never lowered per surface; a stronger cover colour would need a darker text token, not a thinner scrim.
- **Change**: when the track changes the new backdrop fades in over 320ms (the one authored motion moment). Images are decoded before the swap so the fade never shows an empty frame.
- **Banned**: `backdrop-filter`, `filter: blur()` on anything larger than an icon, and any blur on scrolling content.
- **Reduce effects**: backdrops are not rendered at all; the surfaces are solid tokens.

## Motion

- Feedback: 120ms on hover and press (background, colour); menus and dialogs 160ms fade + 4px rise (tw-animate defaults trimmed); route content 150ms opacity only.
- Easing `cubic-bezier(0.16, 1, 0.3, 1)` (exponential out). No bounce, no layout animation, no scroll-linked effects.
- A loop costs frames for as long as it runs. In WebKitGTK every frame the page draws costs a composite of the whole window in the UI process, however small the part that moved: the sidebar's 12px sync icon spinning at 60 fps held 87% of a core in the UI process and 12% in the web process, a `steps(8)` rotation 15% and 3%, a still icon nothing (headless 2560×1440, `uiprobe -motion -scenarios idle`). So work that can run for long shows a still icon and progress that moves when the work does (the sidebar's sync line: its bar and count); a spinner is for waits of a second or two.
- `prefers-reduced-motion: reduce` and the reduce-effects mode both cut durations to zero.

## Reduce effects

A user setting (`Settings → Appearance`, persisted by Go as `reduceEffects`). It sets `.reduce-effects` on `<html>`:

- no backdrops, solid surfaces only;
- transitions and animations off;
- the same layout and colours otherwise, so the mode is never a degraded look.

It is the recommended setting on software rendering (`WEBKIT_DISABLE_COMPOSITING_MODE=1`).

## Lists

- TanStack Virtual for any list that can exceed ~200 rows (library songs, albums, artists, long playlists). The content pane is the scroll element, so the shell never scrolls.
- Fixed row heights (44px, or the grid's computed row height) so virtualisation never measures.
- The playing track's row is marked in place wherever it appears: its index is replaced by an amber equaliser glyph and its title takes `--amber-ink`. It keeps that mark while the list scrolls or re-sorts.

## Home

Home ("For you") answers one question first: what shall I play now. It sets Apple's personal shelves (`features/home/plan.ts`) instead of stacking them as Apple sends them, and never copies Apple's own Home.

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
- **Rows** (`ItemRows`): 56px rows with 40px art, the title at 14/500 and Apple's reason with the artist or curator at 13 muted, in as many columns as fit from 264px (248 beside the lead). Recently Played and every shelf of stations only (Stations for You, moods, favourite artists) are rows two deep: you go back to them or play them at once, so a name and a play control matter more than a large cover. On a backdrop the hover is ink at 6%, not the opaque `--hover`.
- **Cover shelves**: the first three plain shelves Apple ranks highest keep their cover tiles (160px; feature shelves 176px). Features, heroes and track shelves always stay in full.
- **The index** ("More for you"): every later plain shelf (genres, "more like", fans also like) is one 64px line: a chevron, the title at 16/600, what it holds at 12 muted ("3 playlists · 9 albums"), and a strip of its covers at 44px. The line is a disclosure button; open, the shelf's cover tiles appear under it and the strip goes. Twelve shelves take one screen instead of four, and their full-size covers load only when asked for.
- **A lone hero** (Apple's untitled featured card): its 1060:608 banner at 40% of the row beside its name at 24/650, the catalogue line and the play disc, never a half-width card with an empty half.
- **Captions** every item of a shelf shares are dropped; said once in the heading, they are noise.
- **States**: signed out, the lead's place holds one dashed block (Recommendations need Apple Music, one sentence, an ink Sign in), then the library's shelves. Offline with no copy of Home, it says there are no recommendations while offline and that Sona fetches them when it reconnects: no Retry, and no Refresh in the header while offline. Apple's Recently Played failing falls back to "Played on this computer", with its own empty line. Loading draws the lead's shape: the cover, the name's lines and eight rows.
- **Keyboard**: the lead's title, play and menu are Tab stops; each row list is one Tab stop (the arrow keys, Home and End move; Enter opens, Space plays, Shift+F10 or the Menu key opens the row's menu; the row focused last is where Tab returns). Index lines are buttons with `aria-expanded`.

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
- Top bar: back and forward, then the search field (max 520px, `/` or Ctrl+F focuses it, never narrower than 240px), an empty stretch of at least 64px, and the account avatar.
- The avatar is the account menu's button: a `--hover` circle signed out, `--active` with the glyph in ink signed in, and a small `--danger` dot when the account needs the user (Apple stopped taking the sign-in, the keyring didn't hand it over, Apple refuses Sona's requests). The menu (`--raised`, 288px, end-aligned) opens with where the account stands in meta text (storefront, restricted content, offline, an unencrypted token), then the one action that fits (Sign in or Account settings, Check now, Try again), private listening as a checkbox item with its note, and Sign out in danger red last. Anything that needs room (the sign-in form, the account's details) opens the account dialog, never a page.

### Title bar

The sidebar header and the top bar are the window's title bar. Who draws it comes from Go (`Capabilities.windowControls`, `app/shell/window-chrome.ts`), never from the user agent:

- **macOS**: the system's own traffic lights, inset into Sona's band (hidden title bar, full-size content). They keep full screen, tiling, the inactive dimming and VoiceOver. The band is 52px there (AppKit's unified toolbar height, so the lights centre on Sona's controls) and keeps 84px free at the leading edge (in the sidebar header, or the top bar while the sidebar is hidden); in full screen the lights hide and the band returns to 48px with no room.
- **Windows and the stacking Linux desktops** (GNOME, KDE Plasma, Cinnamon, Xfce, MATE, Budgie, Pantheon, …): the window is frameless and Sona draws the buttons, in the desktop's own set and order (GTK's `gtk-decoration-layout`, GNOME's `button-layout`, KWin's `ButtonsOnLeft`/`ButtonsOnRight`): GNOME's default is Close alone. Leading buttons open the sidebar header (or the top bar while the sidebar is hidden), 8px from the edge, 12px before the mark; trailing ones close the top bar after the account.
  - Windows: Windows 11 caption buttons, 46px wide and the band's full height, flush to the top and right edges; 10px glyphs drawn at 1px (minimise, maximise, restore, close); hover `--hover`, pressed `--active`; Close hovers in Windows' red `#c42b1c` with a white glyph. This red is the platform's meaning of Close, not an error, so it is the one exception to "danger only".
  - Linux: Sona's own compact buttons, not Adwaita's or Breeze's: 28px, radius 6, the top bar's 16px outline glyphs at 1.75 stroke (`Minus`, `Square`, `Copy` for Restore, `X`) in `--ink-muted`, 4px apart, `--hover` on hover, `--active` pressed, 8px from the window edge. Close stays neutral.
- **Tiling compositors** (sway, Hyprland, i3, niri, river, bspwm, …) and desktops Sona doesn't recognise: the desktop's own title bar or none; Sona draws no buttons and the band is not a drag region.
- Settings → General → **Title bar** (Windows, Linux): Automatic (the hint names the desktop and what Automatic picks there), Sona's with window buttons, Sona's without window buttons, The desktop's. It applies from the next start (GTK doesn't give a window that ran without its title bar its own back).
- Where Sona draws it, the band's empty parts drag the window, a double click maximises or restores, and a right click opens the desktop's window menu (Linux); every control in the band stays a control. On macOS a double click does what the system's title bar setting says.
- The buttons are real buttons in a group named "Window", labelled Minimise, Maximise or Restore (by the window's state) and Close, and reachable with Tab. Close does what closing the window does (Settings → General: keep playing in the background, or quit).
- While the window is inactive, the wordmark and the button glyphs drop to `--ink-faint`.
- The mini player keeps its own Open Sona and Close buttons, on the side where the platform puts Close (leading on macOS and leading desktop layouts), Close outermost, 4px apart.

### First-run setup

The setup (`/welcome`) takes the whole window: no sidebar, top bar or player bar, nothing to play yet and nothing to wander off to. What stays is the title bar's band, so the window still drags, maximises and closes as everywhere else, and the mark sits exactly where the sidebar's header will put it when the setup gives way to the app.

```
┌──────────┬───────────────────────────────────────────────┐
│ ● Sona   │                                        – □ ✕ │  the band only, no top bar
│          │                                               │
│ Set up…  │        h1 + one line                          │
│ Welcome  │                                               │
│ Apple M… │        groups and rows, as in Settings        │
│  Signed in                                               │
│ Playback │        (680px column, centred)                │
│  Widevine ready · High quality                           │
│ …        │                                               │
│          ├───────────────────────────────────────────────┤
│ Skip set…│        ‹ Back                       Continue  │  64px, stays while the step scrolls
└──────────┴───────────────────────────────────────────────┘
```

- The steps run down a 232px `--surface` column, the sidebar's width and tone. Each carries a live meta line in the catalogue line's form (`Widevine 42% · Data saver · Private`), so the list is also the summary: nothing on the last step repeats it.
- Every step is skippable (the list's links, "Not now", Continue) and "Skip setup" ends it; the last step lists only what is still to do (signing in, a download that hasn't finished), each with the way back to its step.
- The rows are Settings' own rows, so a choice reads and saves the same way in both places. Where the platform can't do something (no tray, no autostart, no notifications) the row is named with the reason instead of left out, because a first run is where people look for it.
- Theme is the one picture in the setup: three windows in miniature, drawn from the tokens in each palette (System is both, halved), chosen as radio buttons.
- One primary button, at the foot's trailing end; Back is a ghost button at its leading end. No amber anywhere but the mark and the previews' play controls.

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
│ Caption · curator                            1 of 3  ‹ › │
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

- **Front page.** The first featured card is the lead: two thirds of the content width, in the hero card shape, its title at 16/600 and its line at 13 muted below. The rest are a list beside it: rows with the item's 64px square cover (Apple's banners lose their text cropped that small), the title at 14/500 and the line at 13 muted, a `--hover` fill at radius 6 like track rows. The list holds as many rows as fit the lead's height, stretched so its pager ends level with the lead's caption. The pager ("1 of 3", 28px chevrons) turns the list only and wraps round, so a focused chevron is never disabled under the keyboard. Under 880px of content the list moves under the lead, in two columns of three. Play controls appear on hover and focus as on every tile; a right-click, Shift+F10 or the Menu key opens the item's menu. What the front page shows is left out of the cover shelves below it (a shelf it would empty stays whole), so one album isn't met four times on the way down.
- **Contents** ("On this page"). One line per band: its name at 14/500 in ink, then its titled shelves at 13 muted with middle dots between, each a button that jumps to its place (no smooth scroll) and moves focus there, so the next Tab is that shelf's first control.
- **Bands.** The shelves are grouped by what they hold, in this order: Songs and albums, Playlists, Radio, Videos, Browse (category bricks and links). Apple's order holds inside each band; a shelf of mixed items goes by most of them (the first item breaks a tie), and a paragraph stays with the shelf after it. Each band opens with one 1px `--line` across the column, 56px below the band before, and its heading 24px under the line at the page-title size, 24/30: the line is what sets a band heading apart from the page's h1, which has none. Inside, shelves keep their usual 40px pitch.
- **Loading.** The freshness line's place holds a bar; the front page's own frame (the lead's block and caption bars, the list's covers and lines at their row height), the contents' lines and the first band's line and heading stand where they will land, then bars for the shelves.

### Album page

A record is two things in the hand: the sleeve, and the listing on its back. The album page lays them side by side instead of stacking a banner over a table.

```
┌ sleeve ─────────────┐   Title (32/36, wraps; never moves the cover)
│ ┌─────────────────┐ │   Artist
│ │                 │ │   Album · 1995 · 40 songs, 2 hrs, 49 mins · Indie Rock
│ │      cover      │ │   [Lossless]
│ │   256 or 320    │ │   Apple's one-line note
│ │                 │ │   status: countdown, or "2 of 13 songs in your library"
│ └─────────────────┘ │   (▶) Shuffle  + Add  Favourite  Suggest less  ⋯
│ Disc 1 · 12 songs…  │   #  Title                                     ◷
│ Disc 2 · 14 songs…  │   Disc 1
│ Released 2 Oct 1995 │   1  Hello (Remastered)                      3:22
│ Label               │   …
│ ℗ line              │   Notes (68ch)
└─────────────────────┘
Other versions · Music videos · More by · Featured in playlists · You might also like
```

- **The sleeve** is the page's one coloured surface: a radius-8 panel washed in the cover's colour (the Go backdrop under the ground scrim, as headers) with a 4% ink layer over it, so it stays one tone step off the ground whatever the cover (a dark cover in the dark theme, a pale one in the light) and under reduce effects. The cover sits inset 16px, then the disc index of a multi-disc album and the record's fine print in meta type, one fact a line (released, label unless the ℗ line names it, ℗). Text on the sleeve is `--ink` or `--ink-muted` only: over the extreme backdrops the tone step leaves `--ink-muted` at 4.9:1 (a white cover in dark) but `--ink-faint` under 4.5. The listing sits on the plain ground beside it, so the colour stays with the record.
- **One station**: the cover's top and the title's first line share a line on every album, whatever the title's length; the title wraps instead of clamping. Covers come in fixed sizes so Go serves them at the exact box: 256px, 320px from 1280px of content width.
- **Held while the songs scroll**: the sleeve is sticky beside the listing whenever the pane shows all of it (measured, so a one-disc sleeve holds in a shorter window than a three-disc one), so a 40-song album keeps its cover and disc index in view. Motion artwork plays there while it is on screen, under the same rules as every header.
- **The listing is 2/3 of the width**, not the whole pane: at 1440 the title-to-duration scan drops from about 980px to 680px.
- **The disc index**: each disc in the catalogue line's form ("Disc 2 · 14 songs, 1 hr, 3 mins"); choosing one brings its heading to the top and focuses its first song. A list long enough to be virtualised (over 200 songs) has no disc headings, so no index either.
- **Rows explain themselves**: the line under a title names a credit other than the album artist's (an Artist column only for a real compilation, where most songs are someone else's) and why a song doesn't play: "Out 19 Nov" on an album not out yet, "Unavailable", or the account's explicit restriction. An unpublished song's stand-in artist ("unknown") is dropped.
- **The catalogue line describes the record**: a library album holding some of its songs counts the whole album and says "2 of 13 songs in your library · Show complete album" in the status slot; an album not out yet leaves out a running time its unpublished songs can't add to.
- **Not out yet**: the countdown takes the status slot; Play plays the songs already out (named "Play the 8 songs out now"), beside Remind me and Pre-add.
- **Below 860px of content** (a 940px window) the sleeve heads the page with the title beside a 168px cover, the songs follow at full width, then the fine print; the disc headings in the list stand in for the index.
- Then Notes at a reading measure, and Apple's shelves in Apple's order.

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

## Lyrics

The now-playing view's lyrics column and full-screen lyrics share one treatment (`features/player/LyricBody.tsx`).

- **The sung line is amber.** Line-timed lyrics light the whole line in `--amber-ink`. Word-timed lyrics keep the sung line in `--ink` and fill each word with `--amber-ink` as the voice reaches it, in the reading direction, with a soft 0.3em edge; sung words stay lit until the next line begins. Earlier lines are `--ink-muted` (`--ink-faint` full screen), later ones `--ink`.
- **Under each line**, smaller and in `--ink-muted`, each on its own line: the background vocals (600: they are sung, and they light on their own timing, so they can still be filling under the next line), then, when switched on in the view's bar, the romanisation (400, lit like the line when it is word-timed) and the translation (400) in the language chosen in Settings → Playback → Lyrics.
- **Duets.** When two or more voices take lines, the first to sing sits at the start and the others at the end, each kept 15% clear of the other's side (full screen: the 22ch column moves across); lines every voice sings together are centred. A song with one voice is laid out as before.
- **Motion.** The fill is the only thing that moves each frame, and only while a word is being sung (CPU: `docs/frontend-architecture.md`, "Word-timed lyrics"). Under `prefers-reduced-motion` or reduce effects each word lights whole as it begins, without a fill.

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

- **Live now.** The section's heading is at the bands' size (it is the first of them, with no line above it), and the local broadcasters' list sits inside it. One row per live station, 96px with an 8px inset, `--hover` fill at radius 6 like track rows (on hover and while a control in the row has focus), in a grid of 420px minimum columns (two at 1440, one at Sona's narrowest, three on a wide screen). The art is the programme's own square from the schedule (the station's when it has none) at 80px. Beside it, on fixed lines so every row's bar sits at the same height: the station's name at 14/500 (a link to its page over the whole row), what is on at 13 in ink, then the slot, a 2px progress bar and the time left ("1 hr, 18 mins left") in meta, and the next programme with a name of its own ("Next at 11:00: The Matt Wilkinson Show"). A slot named after the station shows its description, or the tagline when that is all it says. The station's own hours count as one stretch with the slots around them, so the slot and the time left run to the next named show and no hour goes unaccounted for; when those hours run to the end of the schedule there is nothing to count down to, and the row shows the tagline alone. A show's back-to-back slots are one stretch too. The board reads the clock every 30 seconds and moves on by itself when a slot ends; the schedules are the station pages' own queries, so opening a station is instant.
- **Play and state.** The play control is always shown at the row's end (a 32px ghost circle): picking a station is the page's job, so it is not left to hover. The station playing takes amber where the board shows it: its name in `--amber-ink`, the equaliser in place of the play control, and the progress fill in amber while it plays. A station Sona can't play has no play control; its row says why where the slot would be, with the `CircleSlash` icon.
- **Bands.** After the live part the page's shelves go by what they hold: Stations (the user's own first, Apple's top and genre stations), Shows and episodes (episodes, interviews, sessions, DJ mixes, shows and hosts), Browse (the links). Apple's order holds inside each; the band line and heading are Discover's. Apple's own rows of the live stations are left out while the board lists them, and kept when Sona couldn't list them.
- **Signed out.** The Stations band opens with a "Your stations" block saying what signing in adds, with Sign in.
- **Loading and errors.** The board's frame (rows at their height, the art and lines as bars) and the first band's line and heading stand where they will land. A block that fails says so in place; when nothing loads, the page says it once, with one Retry.

## Empty, loading and error states

- An empty route keeps its frame: the h1, any column header row it will have, then one dashed-border block (1px `--line`, radius 8) holding a 16/600 sentence-case title, one sentence in the product's own words, and at most one action.
- Loading: the final layout with flat `--hover` bars at the real text sizes; no shimmer, no layout shift when data lands.
- Errors live inside the block that failed: what failed, when data was last good, and a Retry text button. `--danger` only on a small icon.
