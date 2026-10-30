# Accessibility

Audit of the app (2026-10-30), and the rules it left behind. Light and dark themes, WCAG 2.2 AA.

## How it was audited

- **Rendered app**: Sona built with `-tags mcp,sqlite_fts5` (dev mode against Vite) in a headless nested sway with a private D-Bus, a copy of the user's settings and library, and audio on a PipeWire null sink. axe-core 4.14 was injected through the Wails MCP `js_eval` and run on every route in both themes (`.dark` toggled on `<html>`); only `violations` are counted.
- **Keyboard**: real key events from `wtype` (virtual keyboard) with a `focusin` logger that records each Tab stop, whether it matches `:focus-visible` and which indicator it draws (outline, box-shadow, or a `:has()` parent outline), plus screenshots. Harness detail: a headless seat loses keyboard focus whenever the last virtual keyboard goes away, so a long-lived `wtype -s 86400000` must hold the seat.
- **Contrast**: axe's per-node checks, plus every token pair computed by hand (text tokens on ground/surface/raised/hover/active; focus ring and control borders against the same backgrounds).
- **Orca**: not run. Orca 50.2 and at-spi2-core are installable (`nixpkgs#orca`, `nixpkgs#at-spi2-core`), and the harness bus serves `org.a11y.Bus`. But the headless session has no audio output for speech-dispatcher and no way to hear or check what Orca says, so a meaningful Orca pass needs a desktop session. The ARIA that Orca relies on (names, roles, live regions) is covered by axe and the code review below.

## Results

axe serious + critical violations per route (nodes), light / dark:

| Route | Before | After |
| --- | --- | --- |
| Every page shell (player bar seek + volume sliders unnamed) | 2 / 2 | 0 / 0 |
| `/equalizer` (11 faders, preamp, player bar) | 13 / 13 | 0 / 0 |
| `/settings?section=playback` (volume slider) | 3 / 3 | 0 / 0 |
| `/search?q=…` (song rows named "Play X", hiding their visible text) | 5 / 5 | 0 / 0 |
| `/search/$group` (query link marked by colour only) | 3 / 3 | 0 / 0 |
| Now playing view (seek `aria-valuetext` on a non-slider, unnamed sliders) | 3 / 3 | 0 / 0 |
| Queue (`role=list` with non-item children) | 4 / 4 | 0 / 0 |
| All other routes: home, discover, charts, chart, radio, station, search, library ×9 (incl. History), album, artist, artist view, playlist, music video, curator, for-you, grouping, room, multiroom, profile, settings ×8 | 2 / 2 each | 0 / 0 each |
| `/song/$id` | not measured (first ID was a 404) | 0 / 0 |

After the fixes no route has any violation of any impact; the only remaining one is `page-has-heading-one` on a song page that failed to load (error state).

## Findings and fixes

### Shared components

- **Sliders unnamed** (`shared/ui/slider.tsx`): Radix puts `role="slider"` on the Thumb, but `aria-label`, `aria-valuetext` and `id` were passed to the Root, so every slider had no name and seek's value text sat on a span without a role (critical). The Slider now forwards `id`, `aria-label`, `aria-labelledby`, `aria-describedby` and `aria-valuetext` to the thumb. Settings rows label sliders with `aria-labelledby="<id>-label"` (`SettingRow` gives its label that id); EQ faders put the name on the Thumb.
- **Slider value shown by colour only**: the thumb only appeared on hover, so the position was the amber fill against the track (1.9:1 in light). The thumb is always shown (ink, 15:1).
- **Focus ring below 3:1**: `--ring` was ink at 40% (2.5:1 on light ground) and ShadCN controls drew it at `/50` (1.5:1). `--ring` is now ink at 60% (≥ 4.2:1 light, ≥ 5.3:1 dark, on every tone), and inputs, select, switch, tabs, badge, scroll area, slider and the playlist description field use `ring-2 ring-ring`. Search and recent-search rows' `:has()` outline uses the ring token instead of `ink/40`.
- **Control borders 1.5:1** (`--control-line`): inputs and selects are identified by their border, so it is now ≥ 3:1 on ground, surface and raised (`#838984` / `#6b736d`).
- **Text tokens on selected rows**: `--ink-faint` (4.1:1 on `--active` in light), `--amber-ink` (4.35:1) and `--positive` (3.9:1) failed where rows are selected or highlighted (track list, command palette, search suggestions). Darkened in light (`#5d635e`, `#845500`, `#1f6e47`) and `--ink-faint` lightened in dark (`#959c96`); all ≥ 4.5:1 on every tone.
- **Track list** (`shared/tracks/TrackList.tsx`): the actions column header was an empty `columnheader` with only `aria-label`; it now has visually hidden text.

### App shell

- **No bypass**: the sidebar (nav, pins, every playlist with its menu button) came before the page in Tab order. A "Skip to content" link is the first Tab stop and focuses `<main>` (`tabIndex=-1`; the router owns the hash, so it does not follow `#content`).
- **Top bar outside landmarks**: it is now a `<header>` (banner).
- **Duplicate landmarks**: the player bar and the Now Playing view were both "Now playing" regions; the bar is "Player". The sidebar's Pinned and Playlists sections were named regions that clashed with search's "Playlists" group; they are now plain sections under their headings inside the Main nav.

### Player

- **Track changes not announced**: the player bar holds a polite `role="status"` region that says "Now playing: {song} by {artist}" when a different track starts (`features/player/announce.ts`, tested). The track restored paused at launch, play/pause, seeking and repeating the same track stay quiet.
- **Focus lost when panels close**: closing the Now Playing view, full-screen lyrics or the queue (Escape, the close button, the bar toggles) dropped focus to `<body>`. `shared/lib/useReturnFocus` returns it to whatever opened the panel, only when focus was lost with it (tested). Radix dialogs and menus (command palette, shortcut help, playlist dialogs, action and context menus) already restore focus to their trigger or previous element.
- **Queue list structure**: the heading and note lines and an "Up next" span sat directly in `role="list"`. Heading/note lines are list items, "Up next" is part of the row's name, the list is named "Queue", and each movable row is described by its keyboard help ("Enter plays the song, Delete removes it, Alt+Up and Alt+Down move it, Escape closes the queue").
- **Full-screen lyrics scroller** had `outline-none` while focusable: it now shows the ring inset.
- **Inline links by colour only**: the "Playing from" station link (queue), the search query link (search group pages) and the song link on music-video pages are underlined.

### Search

- Song rows were named "Play {song}", which hid the visible text from the name (2.5.3 Label in Name). They are named by their visible text with "Play" after it.

## Keyboard walks

Tab from `<main>` through each page (45 stops) with real keys: every stop draws a visible indicator; nothing is skipped or trapped on home, discover, charts and album (logged); the rest of the walk was cut short by the harness (see below). Spot checks:

- Skip link → Enter → `<main>` → first page control.
- Player bar "Open now playing" → Enter: focus on the view's close button → Escape: focus back on "Open now playing".
- Full-screen lyrics (Shift+L): Tab cycles inside the view (scroller, timing buttons, exit).
- Track lists are one Tab stop with arrow keys inside; cover shelves are one stop per tile; the search field shows its focus as the ≥ 3:1 border and raised fill.

## Keyboard alternatives for drag and drop

| Drag | Keyboard |
| --- | --- |
| Songs, albums, playlists onto a sidebar playlist | "Add to playlist…" in every item's action and context menu |
| Onto the queue | "Play next" / "Add to queue" in the same menus |
| Reorder the queue | Alt+↑ / Alt+↓ on a focused queue row (Delete removes, Enter plays) |
| Reorder a playlist in the editor | Alt+↑ / Alt+↓ on a focused row, announced in a live region |
| Reorder pins in the sidebar | "Move up" in the pin's menu |

## Reduced motion

`prefers-reduced-motion: reduce` and the reduce-effects mode set every transition and animation to 0 s and one iteration (`shared/styles/index.css`): state changes still happen, without movement. Full-screen lyrics jump instead of scrolling smoothly under reduced motion.

## Second pass: overlays, Shift+F10, AT-SPI (2026-10-30)

Same harness, plus the AT-SPI registry started by hand on the a11y bus (`dbus-broker-launch --scope user` can only activate it through systemd), and a check on the real Hyprland session (private D-Bus, copied data, keys sent only to Sona's window with `hyprctl dispatch 'hl.dsp.send_shortcut({…, window="address:…"})'`).

### Shift+F10 / Menu key trapped the keyboard

A real Shift+F10 or Menu key reached the page (Sona's row menu opened) but WebKitGTK also handed it to the hidden GTK text widget it uses to translate key bindings, which opened GTK's own text context menu (AT-SPI: `menu "Context menu"` under a 0×0 `text` sibling of the web view; no DOM `contextmenu` event, so Wails' `--default-contextmenu` cannot hide it). Keyboard focus then stayed on that hidden widget: Escape, Tab and arrows never reached the page again. Reproduced on Hyprland 0.56 and in headless sway; this is also why the first pass's walk "stopped delivering keys".

Fix (`internal/shell/contextkey_linux.{go,c,h}`): a capture-phase key controller on each Sona window consumes Shift+F10 and the Menu key before WebKit sees them and re-sends them to the page as a `keydown` on the focused element, so the existing row and tile menu code runs. Verified with real keys on Hyprland and sway: Shift+F10 opens the row menu, ↓ moves through it, Escape returns to the row, and keys keep arriving afterwards; the same for the Menu key; Shift+F10 in the search field leaves typing working.

### Overlays

axe serious + critical nodes, light / dark. "Before" is the state at the start of this pass.

| Overlay | Before | After | Keyboard |
| --- | --- | --- | --- |
| Sleep timer menu (idle and running) | 6 / 6 (`aria-hidden-focus`); bar button 1 / 1 while running (`label-content-name-mismatch`) | 0 / 0 | Enter opens on the first item, arrows/Home/End move, Enter picks, Escape returns to the button |
| Context menu (Shift+F10, Menu key) | 6 / 6 + 1 light (`color-contrast` on a selected row) | 0 / 0 | opens on the first item; focus back on the row |
| Row action menu (⋯ in a row) | could not open without a pointer | 0 / 0 | Enter/Space or an AT click opens it on the first item; Escape returns to ⋯ |
| Header action menu, bulk selection menu (Shift+↓ ×2, Shift+F10) | same root cause as above | 0 / 0 | first item focused; Escape back to trigger / row; selection kept |
| Add to playlist dialog | 1 / 1 (`aria-required-children`: `role=list` around buttons and sections) | 0 / 0 | ↑/↓/Home/End over options, Tab cycles inside; Escape returned focus to `<body>`, now to the row |
| New playlist dialog | (modal root cause) | 0 / 0 | name → description → public switch → Cancel → Close, trapped; Escape back to "New playlist or folder" |
| Edit details / Delete playlist dialogs (sidebar playlist menu) | (modal root cause) | 0 / 0 | Escape returned focus to `<body>`, now to the playlist's ⋯; Delete is now an `alertdialog`, Cancel focused first |
| Command palette (Ctrl+K) | (modal root cause) | 0 / 0 | typing filters, ↑/↓ move the active option, Escape back to the row |
| Shortcut help (`?`) | (modal root cause) | 0 / 0 | Tab cycles Settings link → Close → list; Escape back |
| Playlist editor | 1 / 1 (`aria-required-children`: live region inside the grid); Edit and Done dropped focus to `<body>` | 0 / 0 | Edit focuses Done and Done focuses Edit; rows are one stop with arrows (Alt+↑/↓ and Delete not pressed: they would change the user's real playlist) |
| Album page (disc headings) | 1 / 1 (`aria-required-children`: `h3` inside the grid) | 0 / 0 | |

Fixes:

- **Modal overlays left the hidden page focusable** (every Radix dialog, menu and select): Radix only sets `aria-hidden`. `shared/lib/modal-inert.ts` makes whatever Radix marks `inert` too, and only undoes the `inert` it added (tested).
- **Menu and select items showed focus as a 1.05:1 (dark) / 1.25:1 (light) tint**: keyboard focus now also draws the ring inset (`focus-visible:outline-*` in `shared/ui/dropdown-menu.tsx`, `select.tsx`); the command palette's active option (focus stays in the field) carries the ring.
- **Focus lost after a dialog opened from a menu**: the menu item it would return to is gone. `shared/lib/page-focus.ts` remembers the last page element focused (a menu's trigger counts, a context menu's hidden anchor does not) and `DialogContent` returns there (tested).
- **Context menus opened by key started on the menu, not its first item**; the unarmed row ⋯ button ignored Enter, Space and AT clicks (`shared/actions/ActionMenu.tsx`, `shared/selection/useItemList.tsx`).
- **Sleep timer**: the button's name now includes the visible minutes ("Sleep timer: 15m left, pauses at …").
- **Selected rows** (ink at 12%): `--ink-faint` (`#5a605b` light, `#a0a7a1` dark) and `--positive` (`#1c6943` light) reach 4.5:1 on them on every ground.
- **Grids holding non-rows**: disc headings are rows with one spanning cell (`TrackList.tsx`); the editor's help text and live region moved beside its grid; the playlist pickers' `role=list` is a `group`.

### AT-SPI tree (WebKitGTK)

Read with python3 `gi.repository.Atspi` on the harness bus, home page: `application "dev.coredev.Sona"` → `frame "Sona"` → `document web "Sona"` → `link "Skip to content"`, `landmark "Main"` (the sidebar nav) holding `list`/`list item`/`link "For you"` … `link "Playlists"`, `heading "Library"`, `heading "Pinned"`, `heading "Playlists"`. Names and roles come through as written; WebKitGTK exposes buttons with `aria-haspopup="menu"` as `combo box "More for …"`.

## Third pass: keyboard walk, remaining overlays, live region (2026-10-30)

Same harness. The walk focuses `<main>` and presses real Tab until focus leaves the page for the player bar (cap 120–150 stops), logging each stop's indicator and whether it is on screen. AT-SPI events were read with an `Atspi.EventListener` registered before the app starts (`start.sh` `ATSPI_LISTEN`): WebKitGTK only emits events for listeners known when it starts, and only after an AT has read the tree once.

### Tab off the end of the page trapped the keyboard

Tab from the last control (the volume slider) or Shift+Tab from the first moved GTK focus out of the page onto the same hidden text widget as the Shift+F10 trap (AT-SPI: `text ""` focused beside the web view). No key reached the page again; `document.hasFocus()` stayed false. Every page hit this after one Tab past the player bar.

- `shared/lib/focus-wrap.ts` (main window and mini player): Tab on the last focusable element goes to the first (the skip link), Shift+Tab on the first goes to the last; Tab a dialog's focus trap already handled is left alone (tested).
- `internal/shell/contextkey_linux.c` (`guardKeyboard`, both windows): if GTK focus ever lands on a widget inside the web view other than the view itself, it goes straight back to the view. With the page-level wrap disabled, Tab off the end now leaves keys working and the next Tab reaches the skip link.

### Track-change announcement never reached AT-SPI

With a listener registered and the tree read, MPRIS `Next` on the private bus changed the player's `role=status` text but WebKitGTK sent no event for it (no `object:announcement`, no `text-changed`; nor for any text change in non-editable content, checked on a visible link too). It does send `object:children-changed:add` when a child element is inserted into a live region, which is what Orca's live-region handling reads. So live regions put each message in a new element:

- the player's announcer is a `div role=status` holding one `<p>` keyed by track (`NowPlayingBar.tsx`). After the change, MPRIS `Next` gives `children-changed:remove` + `children-changed:add` on `status bar` with the new `paragraph` (`container-live: polite`, `container-atomic: true`);
- the playlist editor's reorder announcements and the shortcut recorder's "Tab moves between controls" note are keyed the same way.

Rule: a live region's message goes in a new element (a keyed `<p>`/`<div>`), never a text node updated in place.

### Overlays

| Overlay | Before | After | Keyboard |
| --- | --- | --- | --- |
| Settings → Shortcuts recorder (recording) | 1 / 1 (`label-content-name-mismatch`: shows "Press keys…", named by the old keys) | 0 / 0 | Enter records, Tab refused with a note, Escape stops; a conflict focuses "Use it here", Cancel / Use / Escape returned focus to `<body>`, now to the recorder; Reset (row) and Reset all dropped focus as they hid or disabled, now stay on the recorder / the button (`aria-disabled`) |
| Shortcut filter with no match | no status message | 0 / 0 | "No shortcut matches …" is in a `role=status` |
| Reset all confirm dialog | 1 / 1 `scrollable-region-focusable` on `<main>` | unchanged | see below |
| Connections → ListenBrainz token form (open, and with a rejected token) | 0 / 0 | 0 / 0 | Connect focuses the field; Escape and Cancel returned focus to `<body>`, now to Connect (or the row's switch once connected); the error is a `role=alert` and the field `aria-invalid` |
| Equaliser | 0 / 0 | 0 / 0 | presets are one radio stop (arrows), preamp + 10 faders each a stop; ↑/↓ 0.5 dB, PgUp/PgDn, Home/End, `aria-valuetext` "+4 dB" |
| Charts genre Select | — | 0 / 0 serious | 2 px ring at `--ring` on the trigger; Enter opens on the selected option, ↓ moves, Escape returns to the trigger |
| Now playing view, its lyrics, full-screen lyrics, queue panel | — | 0 / 0 | see the walk below |

The Reset all dialog's remaining node: behind a modal, Radix's `hideOthers` keeps live regions exposed, so `<main>` (which holds the recorders' live notes) stays out of `aria-hidden`/`inert` while everything in it is hidden; axe then sees a scrollable `<main>` with nothing focusable. Nothing can or needs to scroll it while the modal is open, and it is restored when the dialog closes.

### Keyboard walk

Every stop on every route below draws a visible indicator (2 px outline or ring at `--ring`), is on screen when focused, and no route repeats or traps; each ends by reaching the player bar, then (after the fix above) the skip link.

- Library: recently added, artists, albums (cover grids, one stop per tile; 150+ stops), songs (sort, play, shuffle, three column sort buttons, the list as one stop), music videos, made for you, favourites, playlists, History (two lists, one stop each).
- Artist (58 stops), artist "See all", song (19), radio (tile grid, 120+), station (3), music video (16), search (98), search group pages `/search/song` and `/search/album`.
- Settings: all eight sections (section links, then each section's selects, sliders, switches and buttons).
- Now playing: Close → artist and album links → the player bar; the sidebar stays reachable (the view covers only the content area). Its lyrics view makes each line a stop (a button that seeks; 64 on the tested song). Full-screen lyrics: Tab cycles scroller → earlier → later → Exit; Escape (twice in a row) returned to the now playing view with focus on its close button, which is where `closeOverlay` sends it.
- Queue: opening it from the bar keeps focus on the toggle; the queue sits before the bar in Tab order (Shift+Tab from the bar), and the list is one stop (fourth pass, below).

### AT-SPI trees

- Album: `landmark ""` (banner: Back, Forward, `combo box "Search"`, `link "Account"`), the focusable `landmark ""` (`<main>`) with `header` → `heading` album name, artist `link`, `button "Play …"`, `button "Shuffle"`, `toggle button "Favourite …"`, `combo box "More for …"`, then `table "Songs on …"` with `column header`s (text through the Text interface, name empty, as WebKitGTK does for name-from-content), a `heading "Disc 1"` row, and focusable `table row`s; shelves are `landmark "More by …"`; then `landmark "Player"`.
- Now playing: `landmark "Now playing"` → `toggle button "Lyrics"`, `button "Close now playing (Escape)"` (focused), `image "… cover"`, `heading` song, artist and album links; the page behind is gone from the tree (inert); `landmark "Player"` holds `status bar` (the announcer), transport buttons, `slider "Seek"`, `combo box "Sleep timer"`, toggles, `slider "Volume"`.
- Settings (Playback): `heading "Settings"`, `landmark "Settings sections"` with the eight links, `landmark "Playback"` → `heading`, then each row's `combo box "Audio quality"`, `slider "Volume"`, `combo box "Stop the playback host when idle"`, `combo box "Output"` … named by their row labels.

## Fourth pass: queue keys, mini player, backdrop contrast (2026-10-30)

Queue and backdrop checks in the headless sway harness (real keys from `wtype`, a 1,075-song queue); the mini player and full-screen lyrics on the real Hyprland 0.56 session (private D-Bus, copied data, keys sent only to Sona's windows with `hl.dsp.send_shortcut`, audio on the null sink, app quit afterwards).

### Queue keys

- **Escape** anywhere in the queue panel (a row, Clear, AutoPlay, Close) closes it and puts focus on the player bar's queue toggle (`QUEUE_TOGGLE_ID`), whether the queue was opened from the toggle or with `Q`. Verified: focus on row 6, Escape → panel gone, focus on `button "Queue"` (`:focus-visible`, `aria-pressed=false`).
- **One Tab stop with roving tabindex** (`features/player/queue-focus.ts`, tested): ↑/↓ move one row, Home/End jump to the current song and the last song, without wrapping or reaching played songs. Off-screen targets are scrolled into the virtual range first (`scrollToIndex`), then focused. The focused row is remembered by its entry's uid, so it follows the song when Alt+↑/↓ moves it; while it is scrolled out of the virtual range the first rendered row is the Tab stop, so Tab always lands on a rendered row. Shift+Tab from the bar: `row0`, then Close queue, AutoPlay, Clear, the page (logged); Tab back in lands on the remembered row.
- **Removal keeps focus in the list**: Delete on a row, or the focused current song finishing, used to drop focus to `<body>`; focus now moves to the row that takes its place (a row the virtualiser unmounts while scrolling is not followed).
- Real keys, logged: ↓↓↓↑ → rows 1, 2, 3, 2; End → row 1074 (list scrolled to 55121 px, the row rendered and focused); ↑↑ → 1073, 1072; Home → row 0 (scroll 0). Alt+↓ ×2 and Alt+↑ moved the focused song 2 → 3 → 4 → 3 with focus on it; Delete removed the focused song and focused the next one; Enter played the focused song; Ctrl+→ while on the current song moved focus to the new current song. Every stop drew the 2 px ring at `--ring`. axe on the open queue with a focused row: 0 / 0 (no violation of any impact).
- AT-SPI (sway, a11y bus): `landmark "Queue"` → heading, Clear, `toggle button "AutoPlay"`, Close, then the list's `list item`s named "⦵, Coldplay", "Up next: ❍, Coldplay" (`focused`), …

### Mini player

| State | axe serious + critical, light / dark | Other findings |
| --- | --- | --- |
| Idle ("Not playing") | 0 / 0 | `page-has-heading-one` (moderate): the song name is now the `<h1>`; 0 violations of any impact after |
| Playing (lyric line, seek ticking) | 0 / 0 | `color-contrast` "incomplete" over the cover backdrop: worked out below |

Keyboard walk (real keys, Hyprland): Tab from the page start goes Open Sona → Close mini player → Previous → Play/Pause → Next → Seek → back to Open Sona (the Tab wrap); every stop is `:focus-visible` with a 2 px outline at `--ring` (the seek thumb its ring), on screen. Space and Enter toggle play/pause with focus kept on the button (its name follows: Pause / Play); Space on Next and Enter on Previous change the song; on Seek, ←/→ move 1 s and Home goes to 0:00; Enter on Open Sona brings the main window back and hides the mini player. Shift+Tab walks the same stops backwards (Seek → Next → … → Open Sona); `send_shortcut` delivers Shift+Tab with `key = "Unidentified"`, so the page-level wrap does not see it and one extra press passes through GTK's guard before reaching Seek (keys keep arriving). With a real keyboard WebKit reports `Tab` and the wrap applies (sway, third pass).

Fixes:

- **Track changes not announced in the mini player**: the main window's announcer is hidden with its window. The mini player holds the same announcer (`features/player/TrackAnnouncer.tsx`, shared). AT-SPI on sway (the mini player's tree exists there even though it renders blank): `frame "Sona mini player"` → `document web` → `landmark "Sona mini player"` → two `status bar`s, `heading "⦵"`, the artist paragraph, the five buttons, `slider "Seek"`; PlaybackService `Next` gave `children-changed:add` on the `status bar` (`container-live: polite`, `container-atomic: true`) with a `paragraph` reading "Now playing: ❍ by Coldplay".
- **Failed commands said in an inserted `role=status`**: the mini player's notice ("Couldn't skip: …") was a `<p role="status">` that appeared with its text, which WebKitGTK reports to no live region. A persistent `role=status` region now gets a new paragraph per failure (the visible line is `aria-hidden`), and the visible line is ink with a danger icon: `--danger` text on the backdrop drops to 3.9:1 in dark.
- **Heading**: the song name is the window's `<h1>`.

### Backdrop contrast (app-wide)

axe cannot measure text over the cover wash (`Backdrop`: Go's pre-blurred cover under a scrim of `--surface` or `--ground`), so it was worked out from the tokens: the image is at worst black or white (Go darkens by 15%, but a header's dominant colour shows unscaled while the image decodes). With the old scrims (78% surface: player bar, mini player; 70% ground: now playing, headers) `--ink-faint` / `--ink-muted` fell to 3.3 / 3.6 : 1 on the bar and 2.8 / 3.1 : 1 on headers in light over a dark cover, and 4.0 / 4.4 : 1 on the bar in dark over a light one. Pixels behind the text in screenshots with a dark cover (Coldplay, *Music of the Spheres*), light theme, worst of the two tokens: album header 2.83 → 4.96, player bar 3.30 → 4.63, now playing 2.87 → 4.97 (dark theme 7.4–7.8 before and after).

Fix: one scrim per theme, `--backdrop-scrim` (92% light, 86% dark), used by every backdrop (the per-use `scrim` prop is gone). Worst case after, any cover: light ≥ 4.63 (ink-faint), 5.07 (ink-muted), 4.60 (amber-ink), 4.78 (positive), 4.72 (danger); dark ≥ 4.68, 5.19, 6.19, 5.33, and `--danger` 3.94, which is why no danger text sits on a backdrop (icons only, ≥ 3:1). Before/after screenshots of the player bar, now playing, an album header and the mini player in both themes were reviewed; the wash is subtler, most of all in light. `docs/design.md` (Backdrops) records the rule.

### Full-screen lyrics after a Tab walk

On Hyprland, nine cycles of 12–60 real Tabs through the page, Shift+L, then Escape (one second later, or 0.3 s later): every Escape arrived, full-screen lyrics → now playing with focus on its close button → the page with focus back where it was; `document.hasFocus()` stayed true. The miss seen once in the headless session did not reproduce on a real compositor.

## Not done

- **Orca speech**: no human listened to Orca. What Orca consumes was checked on the a11y bus instead: names and roles in the AT-SPI trees above (main window, now playing, settings, queue, mini player), and the live regions' `object:children-changed:add` events with the inserted paragraph (`container-live: polite`) for track changes in the player bar (third pass) and the mini player (this pass). Whether Orca speaks them, and how, needs a person listening in a desktop session with speech output.
