# Sona design direction

This is the working visual direction. The tokens below live in `frontend/src/index.css`; change them there and here together.

## Thesis: the label's house style

A good record label sets every release through one fixed template: the same typeface at the same station, the same catalogue line, a quiet board, and the cover as the only picture. Sona works the same way. The chrome is a neutral, faintly green-grey board set in one grotesque. The colour on screen comes from the music: cover art, and backdrops pre-blurred from it in Go. The interface itself owns one spot colour, amber, and spends it on one job only: showing where the music is.

What it refuses: the streaming-app default of near-black with a brand-coloured accent on every control, and CSS glass over blurred artwork.

## Principles

1. **Fast before pretty.** Nothing visual may cost first paint or scroll smoothness on WebKitGTK.
2. **The cover is the colour.** Chrome stays neutral so artwork reads true.
3. **Amber means "playing".** The now-playing row, the play control, the progress fill, the active queue item. Navigation selection, focus and primary buttons do not use amber.
4. **Native to the desktop.** Light and dark follow the system; it works with the keyboard; it is dense enough to sit tiled at half width beside an editor.
5. **Honest states.** Signed out, syncing, offline, no CDM: each is said plainly where the missing data would be.

## Type

One family: **Schibsted Grotesk** (variable, self-hosted via `@fontsource-variable/schibsted-grotesk`, no network fetch). It is a news grotesque, so it stays sharp at 12–14px in dense lists and has character at title sizes. Fallback: `system-ui, sans-serif`.

| Role | Size / line | Weight | Use |
| --- | --- | --- | --- |
| Collection title | 32 / 36, -0.02em | 700 | Album, artist, playlist name in its header |
| Page title (h1) | 24 / 30, -0.015em | 650 | Listen Now, Songs, Settings |
| Section | 16 / 22 | 600 | Shelf titles, settings section titles |
| Body | 14 / 20 | 400–500 | Nav, row titles, form labels |
| Secondary | 13 / 18 | 400 | Artist and album columns, descriptions |
| Meta | 12 / 16 | 450 | The catalogue line, durations, timestamps |

- Six sizes, three weights in practice (400, 600, 700). No uppercase labels, no letter-spaced small caps, no monospace.
- Every number that updates or aligns (durations, track numbers, counts, progress) uses `tabular-nums` (utility `.num`).
- **The catalogue line**: every collection header and the now-playing bar carry one meta line in the same form: `Album · 2008 · 10 songs, 42 min`. Middle dots, sentence case, tabular figures. It is the label's catalogue number and appears in exactly this form everywhere.

## Colour

Strategy: restrained chrome, colour from artwork, one spot colour with one job. All text tiers reach 4.5:1 on the surface they sit on.

| Token | Light | Dark | Role |
| --- | --- | --- | --- |
| `--ground` | `#f4f5f3` | `#121513` | Content canvas |
| `--surface` | `#eceeeb` | `#171a18` | Sidebar, top bar, now-playing bar |
| `--raised` | `#ffffff` | `#1e2220` | Menus, dialogs, popovers, inputs |
| `--hover` | `#e4e7e3` | `#222724` | Row and nav hover |
| `--active` | `#dbdfda` | `#2a302c` | Selected nav item, pressed |
| `--line` | `#dadcd7` | `#262b28` | Structural 1px lines |
| `--control-line` | `#838984` | `#6b736d` | Input and outline-button borders (≥ 3:1) |
| `--ink` | `#161917` | `#eef1ee` | Primary text |
| `--ink-muted` | `#545a55` | `#a9b0aa` | Secondary text |
| `--ink-faint` | `#5d635e` | `#959c96` | Meta, placeholders |
| `--amber` | `#e5a31b` | `#f0b43c` | Spot fill: play button, progress, now-playing marker |
| `--amber-ink` | `#845500` | `#f0b43c` | Amber as text or icon on the ground |
| `--on-amber` | `#1a1405` | `#1a1405` | Glyphs on an amber fill |
| `--danger` | `#b42318` | `#f0705f` | Errors and destructive actions only |
| `--positive` | `#1f6e47` | `#5cc48c` | Success states only |

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

- Square, 4px radius, a 1px inset line at 6% ink so pale covers do not dissolve into the ground.
- Always requested from Go at the exact rendered size (×2 for HiDPI) through `/art/...` URLs, `loading="lazy"`, `decoding="async"`, explicit `width`/`height` so rows never shift.
- Missing art: a `--hover` square with a 40%-ink outline music glyph centred, at the same size.
- The cover is never cropped, tinted or overlaid with text.

## Backdrops (pre-blurred in Go)

The artwork service renders, per cover, a tiny pre-blurred JPEG (target ~32×32, heavy Gaussian blur, saturation kept). The frontend never blurs anything itself.

- **Where**: behind the now-playing bar, the mini player, the now-playing view, full-screen lyrics, and collection headers (album, playlist, artist).
- **How**: an `<img>` (component `Backdrop`) absolutely positioned, `object-fit: cover`, scaled up by the browser (bilinear upscaling of the tiny image is the softness), under a solid scrim of the surface colour (bar, mini player) or the ground colour (views, headers) at `--backdrop-scrim`: 92% in light, 86% in dark. The result is a wash of the cover's colour, never a recognisable picture. Text sits on the scrim, so contrast is the scrim's, not the image's: the scrim is the floor at which `--ink-muted` and `--ink-faint` stay ≥ 4.5:1 over any backdrop, a black one in light (the binding case: 91% on surface) and a white one in dark (85%). It is one value per theme, never lowered per surface; a stronger cover colour would need a darker text token, not a thinner scrim.
- **Change**: when the track changes the new backdrop fades in over 320ms (the one authored motion moment). Images are decoded before the swap so the fade never shows an empty frame.
- **Banned**: `backdrop-filter`, `filter: blur()` on anything larger than an icon, and any blur on scrolling content.
- **Reduce effects**: backdrops are not rendered at all; the surfaces are solid tokens.

## Motion

- Feedback: 120ms on hover and press (background, colour); menus and dialogs 160ms fade + 4px rise (tw-animate defaults trimmed); route content 150ms opacity only.
- Easing `cubic-bezier(0.16, 1, 0.3, 1)` (exponential out). No bounce, no layout animation, no scroll-linked effects.
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

### Title bar

The sidebar header and the top bar are the window's title bar. Who draws it comes from Go (`Capabilities.windowControls`, `app/shell/window-chrome.ts`), never from the user agent:

- **macOS**: the system's own traffic lights, inset into Sona's band (hidden title bar, full-size content). They keep full screen, tiling, the inactive dimming and VoiceOver. The band is 52px there (AppKit's unified toolbar height, so the lights centre on Sona's controls) and keeps 84px free at the leading edge (in the sidebar header, or the top bar while the sidebar is hidden); in full screen the lights hide and the band returns to 48px with no room.
- **Windows and the stacking Linux desktops** (GNOME, KDE Plasma, Cinnamon, Xfce, MATE, Budgie, Pantheon, …): the window is frameless and Sona draws the buttons, in the desktop's own set and order (GTK's `gtk-decoration-layout`, GNOME's `button-layout`, KWin's `ButtonsOnLeft`/`ButtonsOnRight`): GNOME's default is Close alone. Leading buttons open the sidebar header (or the top bar while the sidebar is hidden), 8px from the edge, 12px before the mark; trailing ones close the top bar after the account.
  - Windows: Windows 11 caption buttons, 46px wide and the band's full height, flush to the top and right edges; 10px glyphs drawn at 1px (minimise, maximise, restore, close); hover `--hover`, pressed `--active`; Close hovers in Windows' red `#c42b1c` with a white glyph. This red is the platform's meaning of Close, not an error, so it is the one exception to "danger only".
  - Linux: Sona's own compact buttons, not Adwaita's or Breeze's: 28px, radius 6, the top bar's 16px outline glyphs at 1.75 stroke (`Minus`, `Square`, `Copy` for Restore, `X`) in `--ink-muted`, 4px apart, `--hover` on hover, `--active` pressed, 8px from the window edge. Close stays neutral.
- **Tiling compositors** (sway, Hyprland, i3, niri, river, bspwm, …) and desktops Sona doesn't recognise: the desktop's own title bar or none; Sona draws no buttons and the band is not a drag region.
- Settings → Desktop → **Title bar** (Windows, Linux): Automatic (the hint names the desktop and what Automatic picks there), Sona's with window buttons, Sona's without window buttons, The desktop's. It applies from the next start (GTK doesn't give a window that ran without its title bar its own back).
- Where Sona draws it, the band's empty parts drag the window, a double click maximises or restores, and a right click opens the desktop's window menu (Linux); every control in the band stays a control. On macOS a double click does what the system's title bar setting says.
- The buttons are real buttons in a group named "Window", labelled Minimise, Maximise or Restore (by the window's state) and Close, and reachable with Tab. Close does what closing the window does (Settings → Desktop: keep playing in the background, or quit).
- While the window is inactive, the wordmark and the button glyphs drop to `--ink-faint`.
- The mini player keeps its own Open Sona and Close buttons, on the side where the platform puts Close (leading on macOS and leading desktop layouts), Close outermost, 4px apart.

## Empty, loading and error states

- An empty route keeps its frame: the h1, any column header row it will have, then one dashed-border block (1px `--line`, radius 8) holding a 16/600 sentence-case title, one sentence in the product's own words, and at most one action.
- Loading: the final layout with flat `--hover` bars at the real text sizes; no shimmer, no layout shift when data lands.
- Errors live inside the block that failed: what failed, when data was last good, and a Retry text button. `--danger` only on a small icon.
