# Accessibility

Sona targets WCAG 2.2 AA in both themes. This page holds the rules every
change keeps, what each check covers, and what is left to a person with a
screen reader.

MyGo's native UI reaches assistive technology through the platform's own
API: AT-SPI on Linux (Orca), NSAccessibility on macOS (VoiceOver) and UI
Automation on Windows (Narrator, NVDA). MyGo describes a frame only once
assistive technology asks, and describes it as a tree of elements with a
role, a name, a value, states and actions
(`docs/ui/accessibility.md` in MyGo). What that tree can and cannot say
shapes the rules below.

## Rules

### Names, roles and structure

- Every control has an accessible name that contains its visible text
  (2.5.3 Label in Name): a song row is named by its text, a tile's play
  button "Play {name}" (an icon, so no visible text to contain). A control
  with no text takes an `A11yLabel` (`kit.Button`, `kit.Input`, `kit.Select`)
  or `Label`.
- **Controls are never nested in an opener.** MyGo reads a button, link,
  tab, column header, text and every other widget role as one piece: the
  elements inside it are not on the tree, although the pointer and Tab still
  reach them, so a "more" button drawn inside a row that is a button is
  invisible to a screen reader. Only group, list, scroll area, dialog, popup,
  status, tab list, table, row, cell, tree, list item, toolbar, radio group, alert dialog, menu bar
  and menu hold other elements. Every control is a sibling of the thing it
  sits beside:
  - `kit.DragItem` builds the item's source (the opener) and its `After`
    controls as siblings in one wrapper (cover tiles, Home rows, artist
    releases); wide cards and queue rows build the same shape in a wrapper or
    a list item of their own;
  - a row's controls are cells of the row, which is a row, not a button;
  - a sortable column header is its own sort button (the header cell has no
    role), named by the title with the direction as its value.
- A control that shows only while its row is hovered is built at rest too,
  `Opacity(0)` and `IgnoreInput(true)` (the pointer and Tab ignore it,
  assistive technology finds and presses it): the play button of a track
  row, the more and remove buttons of a queue row.
- A list is one Tab stop with arrow keys inside (a focusable row, the
  others reached by the arrows); its controls are on the tree but are not
  stops. Menus are reached from the row with Shift+F10 or the Menu key.
- Links are `RoleLink` (artist and album cells, "See all", sidebar
  entries), headings carry their level (`RoleHeading` and `Level`; the page
  title is level 1, dialog titles level 2), lists and tables are named by
  their heading, and lists hold only their items.
- Every dialog is a named `dialog`, or an `alertdialog` when it asks
  (`kit.Confirm`); what is behind a modal is not on the tree while it is
  open.
- Inline links are underlined, never marked by colour alone.

### States MyGo has, and what stands in for the rest

| ARIA | In the native app |
| --- | --- |
| `aria-checked`, `aria-pressed` | `Checked` on a switch, check box, toggle button, tab or menu item (`kit.ButtonOpts.Pressed`) |
| `aria-expanded`, `aria-haspopup` | `Expanded` on the opener, with `RoleMenuButton` or `RolePopUpButton` for the popup |
| `aria-invalid` and its message | `kit.InputOpts.Error` / `kit.SelectOpts.Error`: the invalid state, the message read with the value |
| `aria-valuetext` | the slider's description (`kit.SliderOpts.ValueText`) |
| `aria-disabled` (a control that keeps focus and says why) | `kit.ButtonOpts.Reason`: enabled, drawn at half strength, the press ignored by the caller, the reason as its description and the tooltip's second line |
| `disabled` (really unavailable) | `Disabled`: no focus, no press |
| `aria-current` (current page, step or item) | no state; the control's description is "Current page" (navigation), "Current step" (the setup strip) or "Current" (a chosen city, the lead of a Discover list) |
| `aria-busy`, `role="status"` loading | a `RoleStatus` named "Loading" (skeletons), and `c.Announce` for the result |
| `role="alert"` | a `RoleStatus` carrying the text, and `c.Announce` as it appears |
| `aria-live` | `c.Announce(text)` (read once, politely), and toasts, which announce their title and description |
| `aria-controls` | none |

MyGo has no aria-current, aria-disabled, aria-busy, alert, live-region or
aria-controls equivalents; the table is what the features use meanwhile.
Their gaps are tracked as upstream asks, and each stand-in is marked in the
code by the state it replaces.

### Contrast

- Text tokens are ≥ 4.5:1 on every background up to `--active` and on
  selected rows (ink at 12%); `--control-line` and the focus ring are
  ≥ 3:1 on ground, surface and raised. The values live in the theme tokens
  (`internal/ui/theme`); `docs/design.md` lists them.
- Slider thumbs are always shown: the thumb, not the fill colour, marks
  the value.
- Text over a cover backdrop gets its contrast from the scrim
  (`docs/design.md`, Backdrops). `--danger` falls below 4.5:1 there in dark,
  so no danger text sits on a backdrop: icons only (≥ 3:1), with the message
  in ink.

### Focus

- Every focus stop draws a visible indicator (MyGo's focus ring in the ring
  token) and is on screen when focused.
- Focus returns where it came from: closing a dialog, menu, popover or
  confirmation returns it to what opened it. MyGo notes the opener after the
  overlay's content is built, so a control that takes the focus in the
  overlay's first frame (`AutoFocus`) would be taken for it: use
  `kit.FocusWhenShown`, which focuses one frame later. Removing a row moves
  focus to the row that takes its place. Focus never drops to nowhere.
- Modal overlays hold Tab inside and take the page behind them off the
  tree.
- Menus opened from the keyboard start on their first item; row menu
  buttons are not Tab stops (a list is one stop): they open from a screen reader's press, and the row's menu from Shift+F10 or the Menu key.
- Long lists are one Tab stop with arrow keys inside. The queue: ↑/↓ one
  row, Home/End the current and the last song, Alt+↑/↓ moves a song, Delete
  removes it, Enter plays it. Escape in the queue closes it and returns the
  focus to the player bar's queue toggle; in the Now Playing view's Up next
  Escape closes the view.
- The video view is a group named "Music video", described by a line
  listing its keys. It takes focus when it opens and keeps Tab inside.
  Keys: Space or K play and pause, ← and → seek 5 s, F toggles full
  screen, M mutes, Escape leaves full screen and then closes, V closes. Its
  controls never fade while one has keyboard focus. Loading and buffering are
  announced; a failure is a status with "Watch in Apple Music" and, where it
  can help, "Try again".
- The Now Playing view's panes are tabs (Lyrics, Up next): one Tab stop,
  ←/→ and Home/End move and show.

### Live announcements

MyGo announces through `c.Announce` (VoiceOver and Orca announcements, a
live region on Windows) and every toast.

- Track changes are announced politely ("Now playing: {song} by {artist}",
  or "Now playing a preview: …", `kit/playerqueue_announce.go`) once the
  engine reports the track as playing. The track restored paused at launch,
  play/pause, seeking and repeating the same track stay quiet.
- Moving a queue song says its new place; Remove says nothing, and Play next
  and Play last from an item's menu raise a toast. A playlist editor's
  Alt+↑/↓ says the new place too.
- A control that does not apply to what plays (Previous, Next, Shuffle,
  Repeat and the AutoPlay toggle on a live station; Shuffle, Repeat and
  AutoPlay on a station of songs) uses `ButtonOpts.Reason`; a shortcut for
  one does nothing and says the same reason once in a toast
  (`kit.Unavailable`): the toast stays about 4 s and the reason is not raised
  again while it shows, so a key held longer is read again about every 4 s.
- Errors, warnings, update and diagnostics verdicts, the scrobbler account
  line, busy steps of sign-in and setup, and filter results with no match are
  announced as they appear or change; what is on screen when a page opens is
  read with the page, not announced again.
- Lyrics are not a live region. Each synced line is one button named by
  its text; nothing marks the sung line to assistive technology.

### Keyboard alternatives for drag and drop

| Drag | Keyboard |
| --- | --- |
| Songs, albums, playlists onto a sidebar playlist | "Add to playlist" in every item's action and context menu |
| Onto the queue | "Play next" / "Play last" in the same menus |
| Reorder the queue | Alt+↑ / Alt+↓ on a focused queue row (Delete removes, Enter plays), or "Play next" and "Move to end" in the row's menu (Shift+F10 or the Menu key); each move is announced with the song's new place. Only the user's songs move, and they stay before the station's or AutoPlay's songs |
| Reorder a playlist in the editor | Alt+↑ / Alt+↓ on a focused row, announced |
| Reorder pins in the sidebar | "Move up" in the pin's menu |

### Reduced motion

The platform's reduce-motion setting and Sona's reduce-effects mode set
every transition and animation to none (`theme.ReducedMotion`): state changes
still happen, without movement. Word-timed lyrics light each word whole as
it begins; motion artwork does not play (Settings → Diagnostics → Motion
artwork says which input stops it); covers show their still.

## What is checked, and by what

| Check | Where | What it covers |
| --- | --- | --- |
| `uitest/access` | `go test` (Linux on every pull request, macOS after merges; Windows builds and vets only) | The tree MyGo describes for assistive technology, read from a `ui.Tester`: roles, names, descriptions, states and actions of a page; `NoNestedControls` fails for every clickable or focusable element inside a button, link or other leaf role; `TabWalk` presses Tab round the window and fails for a stop that is not on the tree or a walk that never comes back (a trap) |
| `internal/ui/kit/a11y_test.go` | `go test` | Each list and item component (cover grid, shelf, item grid, track list, track shelf, stations, link list, wide row), form controls and their states, dialogs, confirmations, toasts, menus |
| `internal/ui/**/a11y_test.go` | `go test` | Per feature package: its pages' trees and Tab walk in reading order, Escape and focus return for the overlays it owns, announcements, the states table above |
| `e2e` `TestAccessibility`, `TestAccessibilityPages` | CI "E2E (headless sway)" job only | The real app's tree on the AT-SPI bus: the window's landmarks, sidebar links and each library page's heading, with the roles and names a screen reader gets (`headless -- go test -tags e2e ./e2e/...`) |
| Contrast | `TestContrastFloors` | The text tokens' ratios, computed; the scrim, the focus ring and selected rows are checked as values |

The `ui.Tester` checks are `go test`s, so they run wherever CI runs the Go
tests; the e2e check runs on Linux only (headless sway, AT-SPI).

### What a person checks (phase 5)

Nothing here replaces listening. With each release candidate, in a desktop
session with audio:

- **Orca (Linux)**: sign in; walk Home, a library page, an album, the queue,
  Settings and the setup with Tab and the arrows; check that each control is
  announced with its role, name and state, that a track change and a queue
  edit are read once, that "Not available while listening to a live station"
  is read once per key hold, and that closing a dialog or menu puts the focus
  back.
- **VoiceOver (macOS)**: the same walk; the rotor lists headings, links and
  controls, a cover tile's play and more buttons are reachable beside its
  link, a sortable column header reads its direction.
- **Narrator or NVDA (Windows)**: the same walk, and the announcements.
- Both themes, with reduce-motion on and off.

## Known gaps

- A confirmation opened over the first-run setup returns the focus to the
  setup's first control, not to what opened it.
- No one has yet listened to Orca, VoiceOver or Narrator speak Sona's
  announcements in a desktop session (phase 5).
- No current-page, disabled-but-focusable, busy or alert state exists in
  MyGo; the stand-ins above read well but are not those states. The setup
  strip's one Tab stop is the segment last focused, not the current step,
  because MyGo's focus group enters on a checked element only.
- A control that takes the focus without being a Tab stop (the web
  `tabindex=-1` pattern: the video view, the setup step title) is also a Tab
  stop.
- A heading cannot hold a link (the video title is not a heading).
- The hidden buttons of a row cannot be pressed by a test the way a screen
  reader presses them: `access.Node.Can(access.Press)` is what the tests
  check.
