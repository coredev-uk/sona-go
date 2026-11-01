# Accessibility

Sona targets WCAG 2.2 AA in both themes. This page holds the rules every
change keeps and how to check them.

## Rules

### Names, roles and structure

- Every control has an accessible name that contains its visible text
  (2.5.3 Label in Name): a song row is named by its text with "Play" after
  it, never "Play {song}" alone.
- Sliders: Radix puts `role="slider"` on the thumb, so `id`, `aria-label`,
  `aria-labelledby`, `aria-describedby` and `aria-valuetext` go to the thumb
  (`shared/ui/slider.tsx` forwards them). Settings rows label sliders with
  `aria-labelledby="<id>-label"` (`SettingRow` gives its label that id).
- Landmarks: the top bar is a `<header>` (banner), the page is `<main>`, the
  player bar is the "Player" region and the Now Playing view the "Now
  playing" one. Region names never repeat; the sidebar's Pinned and
  Playlists are plain sections under their headings inside the Main nav.
- A "Skip to content" link is the first Tab stop and focuses `<main>`
  (`tabIndex=-1`; the router owns the hash, so it does not follow
  `#content`).
- Lists and grids hold only their own items: `role=list` contains list
  items, a grid contains rows (disc headings are rows with one spanning
  cell in `TrackList.tsx`), and help text or live regions sit beside a grid,
  not inside it. Pickers that hold buttons and sections are a `group`.
- Each window has one `<h1>` (the mini player's is the song name).
- Inline links are underlined, never marked by colour alone.
- Tooltips on tags (`Tag`, `shared/components/Badges.tsx`): WebKitGTK drops
  the `title` of a flex box from AT-SPI, so the box keeps `title` for the
  hover tooltip and the text inside carries `aria-describedby` pointing at
  a `hidden` copy of it.

### Contrast

- Text tokens are ≥ 4.5:1 on every background up to `--active` and on
  selected rows (ink at 12%); `--control-line` and the focus ring are
  ≥ 3:1 on ground, surface and raised. The values live in
  `shared/styles/index.css`; `docs/design.md` lists them.
- Slider thumbs are always shown: the thumb, not the fill colour, marks
  the value.
- Text over a cover backdrop gets its contrast from the scrim
  (`--backdrop-scrim`, one value per theme; `docs/design.md`, Backdrops).
  `--danger` falls below 4.5:1 there in dark, so no danger text sits on a
  backdrop: icons only (≥ 3:1), with the message in ink.

### Focus

- Every focus stop draws a visible indicator (a 2 px outline or ring at
  `--ring`; menu and select items draw the ring inset as well as the tint)
  and is on screen when focused.
- Focus returns where it came from: closing the Now Playing view,
  full-screen lyrics or the queue returns it to whatever opened the panel
  (`shared/lib/useReturnFocus`); a dialog opened from a menu returns it to
  the last page element focused (`shared/lib/page-focus.ts`); removing a
  row moves it to the row that takes its place. Focus never drops to
  `<body>`.
- Modal overlays make the page behind them `inert`, not only `aria-hidden`
  (`shared/lib/modal-inert.ts`, which undoes only the `inert` it added).
- Menus opened from the keyboard start on their first item; row menu
  buttons answer Enter, Space and AT clicks
  (`shared/actions/ActionMenu.tsx`, `shared/selection/useItemList.tsx`).
- Long lists are one Tab stop with arrow keys inside (roving tabindex).
  The queue (`features/player/queue-focus.ts`): ↑/↓ one row, Home/End the
  current and the last song, the focused row remembered by entry so it
  follows a moved song, off-screen targets scrolled into the virtual range
  before focusing. Escape anywhere in the queue closes it and focuses the
  player bar's queue toggle.
- Tab wraps inside each window (`shared/lib/focus-wrap.ts`: Tab on the last
  element goes to the skip link, Shift+Tab on the first to the last). On
  Linux, GTK focus must never land on WebKitGTK's hidden key-binding text
  widget, which would swallow every later key: `internal/shell/contextkey_linux.c`
  sends focus straight back to the web view, and consumes Shift+F10 and the
  Menu key before WebKit sees them, re-sending them to the page as a
  `keydown` so Sona's own row and tile menus open.

### Live regions

- A live region's message goes in a new element (a keyed `<p>`/`<div>`),
  never a text node updated in place, and the region exists before the
  message: WebKitGTK reports a live-region change to AT-SPI only as an
  inserted child (`object:children-changed:add`), never as changed text or
  a region that appears with its text.
- Track changes are announced politely in both the main window and the mini
  player ("Now playing: {song} by {artist}", or "Now playing a preview: …";
  `features/player/announce.ts`, `TrackAnnouncer.tsx`), once the engine
  reports the track as playing. The track restored paused at launch,
  play/pause, seeking and repeating the same track stay quiet.
- Errors are `role=alert` with the field `aria-invalid`; filter results
  with no match are announced in a `role=status`.

### Keyboard alternatives for drag and drop

| Drag | Keyboard |
| --- | --- |
| Songs, albums, playlists onto a sidebar playlist | "Add to playlist…" in every item's action and context menu |
| Onto the queue | "Play next" / "Add to queue" in the same menus |
| Reorder the queue | Alt+↑ / Alt+↓ on a focused queue row (Delete removes, Enter plays). Only the user's songs move, and they stay before the station's or AutoPlay's songs, which keep their place (their rows say so in their keyboard help) |
| Reorder a playlist in the editor | Alt+↑ / Alt+↓ on a focused row, announced in a live region |
| Reorder pins in the sidebar | "Move up" in the pin's menu |

### Reduced motion

`prefers-reduced-motion: reduce` and the reduce-effects mode set every
transition and animation to 0 s and one iteration
(`shared/styles/index.css`): state changes still happen, without movement.
Full-screen lyrics jump instead of scrolling smoothly under reduced motion.

### Sessions without an accessibility bus

WebKitGTK's sandboxed web process prints `Can't connect to a11y bus` on a
desktop with no AT-SPI bus. Sona asks `org.a11y.Bus.GetAddress` itself at
startup, before the first web view; only when the session definitively has
none does it set `WEBKIT_A11Y_BUS_ADDRESS=` (empty), which WebKit treats as
"no bus", and log `startup: no accessibility bus; web content is not exposed
to AT-SPI` (`internal/app/a11ybus.go`). A user-set `WEBKIT_A11Y_BUS_ADDRESS`
or `AT_SPI_BUS_ADDRESS`, a running bus, or a lookup that times out leave
everything to WebKit; WebKit's sandbox stays on.

## How to check

Run the app in isolation (`dbus-run-session`, private XDG dirs, private
mode, audio on your own null sink: CONTRIBUTING.md), in both themes.

- **axe-core**: build with `-tags mcp,sqlite_fts5` and inject axe through
  the Wails MCP server's `js_eval` on each route you touched, light and dark
  (`.dark` on `<html>`). Serious and critical violations must be 0. Text
  over a cover backdrop is "incomplete" for axe; work it out from the
  tokens and the scrim instead.
- **Keyboard**: send real keys (`wtype` on wlroots compositors; in a
  headless seat keep one `wtype -s 86400000` alive or the seat loses
  keyboard focus) and log each Tab stop with a `focusin` listener: the
  indicator it draws, whether it is on screen, and that nothing is skipped
  or trapped. Check Escape and focus return for every overlay you touched.
- **AT-SPI**: read the tree with python3 `gi.repository.Atspi` on the a11y
  bus. WebKitGTK emits events only for listeners registered before the app
  starts, and only after an AT has read the tree once, so register an
  `Atspi.EventListener` first when checking live regions.
- **Orca**: speech needs a desktop session with audio output; headless runs
  can only check what Orca consumes (names, roles, live-region events).

## Known gaps

- No one has yet listened to Orca speak Sona's announcements in a desktop
  session.
- Behind a modal, Radix keeps live regions exposed, so `<main>` (which holds
  the shortcut recorders' live notes) stays out of `aria-hidden`/`inert`,
  and axe reports `scrollable-region-focusable` on it while the Settings →
  Shortcuts "Reset all" dialog is open. Nothing needs to scroll it then.
