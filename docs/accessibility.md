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
  invisible to a screen reader. Only group, list, list box, option, scroll
  area, dialog, popup, status, tab list, table, grid, row, cell, column and
  row header, tree, list item, toolbar, radio group, alert dialog, menu bar,
  menu, heading, navigation, region, search, article, log, timer and alert
  hold other elements. Every control is a sibling of the thing it
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
  The playing song's menu opens the same way from the bar's cover and from
  the mini player; Now Playing also has a "More for <title>" menu button.
- Links are `RoleLink` (artist and album cells, "See all", sidebar
  entries), headings carry their level (`RoleHeading` and `Level`; the page
  title is level 1, dialog titles level 2), lists and tables are named by
  their heading, and lists hold only their items.
- Every dialog is a named `dialog`, or an `alertdialog` when it asks
  (`kit.Confirm`); what is behind a modal is not on the tree while it is
  open.
- Inline links are underlined, never marked by colour alone.
- A `kit.Select`'s list is a list box (MyGo's `SelectBase`) of options,
  each `Checked` while chosen (aria-selected); its trigger is a combo box.
- A choice made among pictures (the playlist cover picker) is a `radio group` of `radio` tiles, each named by the label drawn under it (2.5.3), described in a line of its own ("No picture"), `Checked` while chosen, with arrow keys over the grid (`kit.GridNav`). The chosen tile says so with a ring, a check mark and a heavier name, never by colour alone. A choice that applies at once announces its result ("Cover: Bars"), and a failure stays beside the control as an alert, with the limits it broke in view.
- A text with links in it is not itself on the tree (`kit.ArtistLinks` is a
  `RoleNone` rich text, so its links sit beside its separators): Orca reads
  the enclosing text before a focused link, so a link inside an artist's
  text was read twice ("Ines Calder, Ines Calder, link").
- A tooltip whose first line is the control's name (the account button's)
  describes the control by the rest (`kit.TooltipDescribed`): by itself it
  is the description, and the name is read twice.
- What takes the focus without being a control is a group named by its text
  (the setup step's title around its heading), not the heading: Orca reads a
  focused heading's role and level ("heading 2") and takes its words from
  the text, which MyGo gives to labels.

### States MyGo has, and what stands in for the rest

| ARIA | In the native app |
| --- | --- |
| `aria-checked`, `aria-pressed` | `Checked` on a switch, check box, toggle button, tab or menu item (`kit.ButtonOpts.Pressed`) |
| `aria-expanded`, `aria-haspopup` | `Expanded` on the opener, with `RoleMenuButton` or `RolePopUpButton` for a menu or a list; `HasPopup(ui.PopupDialog)` on a button that opens a dialog (Set location, a playlist's Name and description and Cover, the collaborators byline) |
| `aria-invalid` and its message | `kit.InputOpts.Error` / `kit.SelectOpts.Error`: the invalid state, the message read with the value |
| `aria-valuetext` | the slider's description (`kit.SliderOpts.ValueText`), only where it says more than the value and the percentage a screen reader reads anyway: Seek's "0:42 of 3:00", not Volume's "80%" (Orca read "80, 80 percent, 80%") |
| `aria-disabled` (a control that keeps focus and says why) | `kit.ButtonOpts.Reason`: `AccessDisabled(true)`, so it is read as unavailable and keeps the focus; drawn at half strength and the press ignored by the caller (`AccessDisabled` only tells assistive technology), the reason as its description and the tooltip's second line. A track list's withheld (explicit) row is the same, described by "Explicit content is restricted on this account" |
| `disabled` (really unavailable) | `Disabled`: no focus, no press |
| `aria-current` (current page, step or item) | `Current`: `CurrentPage` on the sidebar link of the open page (the playlist sidebar's links and Settings' section links too), `CurrentStep` on the setup strip's current step, `CurrentItem` on a chosen city, the lead of a Discover list, the playing queue row and the sung lyric line. A description that held more keeps it (Settings' section summary) |
| `aria-busy`, `role="status"` loading | `Busy(true)` on the region whose content is loading or changing (the concerts agenda while the next filters load, the sign-in form while a step runs), and a `RoleStatus` named "Loading" for a placeholder standing in for the content (skeletons; the setup and account placeholders are both). A busy sign-in step is still announced once as it starts: `Busy` only tells a screen reader to wait, and the button's new label is held back with the rest of the form |
| `role="alert"` | `RoleAlert` on the message: an `ErrorBlock`'s title and reason, a sign-in or token error, a refused scrobbler token, the warning that scrobbler sign-ins are stored unencrypted, a cover that could not be set, a video's problem. MyGo reads it out when it appears and when it changes, so there is no `c.Announce` beside it, and the buttons beside the message are not inside the alert. A wait (Last.fm's authorisation) or a problem the account section finds on arrival (the keyring notice) is a `RoleStatus`, announced once where it is news |
| `aria-live` | `c.Announce(text)` (read once, politely), and toasts, which announce their title and description |
| `aria-controls` | `Controls(target)` on a disclosure that shows or hides a region it owns: `kit.Notes`' "Read more", a read-only sidebar section's fold, a Home index line |

These are MyGo v0.8.1's `Current`, `AccessDisabled`, `Busy`, `RoleAlert`,
`Controls` and `HasPopup`. A disclosure whose region another component
builds sets no `Controls` (a shelf's "Show all", the player bar's Queue toggle
and the panel the shell builds beside the page): MyGo's vet refuses an element
kept past its build pass, and a handle cannot stand in for it.

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
  announced; a failure is an alert with "Watch in Apple Music" and, where it
  can help, "Try again" beside it.
- The Now Playing view's panes are tabs (Lyrics, Up next): one Tab stop,
  ←/→ and Home/End move and show.
- Escape on a control whose tooltip shows (Now Playing's Close, reached by
  keyboard) hides the tooltip first, as WCAG 1.4.13 asks, and closes the
  view or confirmation on the second press.
- A slider that moves by itself (Seek, as the song plays) keeps, for
  assistive technology, the value it had when the focus came and after each
  key: a screen reader reads every change of the focused slider, and the
  position ticks four times a second. The time and the bar go on moving.

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
  announced as they appear or change (errors as alerts, the rest through
  `c.Announce`); what is on screen when a page opens is read with the page, not
  announced again.
- Lyrics are not a live region. Each synced line is one button named by
  its text; the sung line is the current item.

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
| `e2e` `TestOrcaWalk`, `TestOrcaSetup` | CI "Orca walk" job, dispatch only (`only` = `orca` or `all`), and `orca-walk` | A real Orca walked through the app by keyboard, and what it said, from its debug log: see "Listening with Orca" |
| Contrast | `TestContrastFloors` | The text tokens' ratios, computed; the scrim, the focus ring and selected rows are checked as values |

The `ui.Tester` checks are `go test`s, so they run wherever CI runs the Go
tests; the e2e check runs on Linux only (headless sway, AT-SPI).

### Listening with Orca

`TestOrcaWalk` and `TestOrcaSetup` (`e2e/orca_walk_test.go`) run a real Orca
beside the app in the e2e harness's headless session, on the fixture profile,
and press keys with `wtype`, as a keyboard does: Tab through the sidebar, the
account button, menu and dialog, a library page (Albums: the filter, Sort, the
tiles, arrows, Shift+F10), an album (artist link, buttons, song list, Enter),
the queue (Q, a move, a removal, Escape), Now Playing (its tabs, Escape), the
player bar's Volume and Seek sliders and a track change, and then the
first-run setup. Orca writes every utterance it generates to its debug file
(`--debug-file`) whether or not a speech server is there, so nothing is
spoken and no audio is involved: the words checked are the words Orca would
say. The run also writes `orca-walk.md` and `orca-setup.md` to `E2E_OUT` (each
key, what Orca said for it, and where it put the focus), and Orca's log.

```sh
devenv shell -- orca-walk                  # Orca from the pinned nixpkgs; what it said: bin/orca-out
E2E_ORCA=1 e2e/headless.sh -- go test -tags e2e -count=1 -v -run '^TestOrca' ./e2e/   # `orca` on the PATH, or E2E_ORCA_BIN
```

Without `E2E_ORCA=1` the tests skip. CI runs them only when the CI workflow is
dispatched with `only` = `orca` or `all` (never on a pull request or a push,
and not a required check: the words and the timing are Orca's, and a new Orca
can move them). Orca's own English (`LC_ALL=C.UTF-8`) is the reference.

The walk waits for Orca by its log, not by fixed pauses: after a key it waits
for Orca to have been told the key was released, for its event queue to be
empty, and for 0.6 s in which Orca said and queued nothing (a slider's value
ticking as the song plays does not count). On the CI runner no answer to a key
took more than 0.38 s from the release to its last word, and the longest
silence inside one was 0.20 s. Orca's log is read as it grows, each entry
parsed once (`e2e/orca_log_test.go` checks that against a parse of the whole
file). The CI job runs in the CI image with Orca installed on top.

On every key the test fails for a name or text said twice in a row (a link
read with the text it is in) and for a flood of more than 16 utterances (a
value read as often as it changes). Per surface it checks the roles, names and
states of the table below, that a track change and a queue move are read
once, that closing a menu, dialog or the queue gives the focus back to what
opened it, and that the focused Seek slider is silent while the song plays.
What it heard, with Orca 50.2:

| Surface | Orca said |
| --- | --- |
| Sidebar, first Tab | "Main", "For you", "link."; then one link per Tab, the open page "Albums, link." with "Current page" (its current-page state), "New playlist or folder, push button menu. opens popup", "Playlists, List with 3 items", the pins as links |
| Account button | "Account, button. Not signed in." (the name once); Enter: "Account, menu, Sign in."; Escape: the button again |
| Account dialog | "Account, dialog, Apple Account, entry. The email address or phone number you use with Apple.", "Password, password text.", the browser and token buttons, "Close, button."; Escape returns to "Account, button." |
| Albums page | "Albums, panel, Album type, panel, All, toggle button pressed.", "Sort, combo box.", a tile "Fieldnotes, link. Kofi Bellamy."; the arrows move between tiles; Shift+F10 "More for …, menu, Create station."; Escape returns to the tile |
| An album | "Album" on arrival; "Ines Calder, link." (once); "Play …, button. Play."; "Favourite …, toggle button not pressed."; the song list one stop, "Dust on the Dial." per arrow; Enter: "Now playing: … by …" once |
| The queue | "Queue 2 songs after this one, panel, Clear, button."; the list one stop "Queue, List with 3 items"; Alt+↓ "Dust on the Dial: moved to position 2 of 2"; Delete moves the focus to the next row; Escape: "Queue, toggle button not pressed." |
| Now Playing | "Lyrics, page tab.", ←/→ "Up next, page tab.", "Close now playing (Escape), button." |
| Player bar | "Volume, horizontal slider, 80, 80 percent."; "Seek, horizontal slider, 42, 23 percent. 0:42 of 3:00." and then nothing as the song plays; Ctrl+→ "Now playing: … by …" |
| Setup | "Welcome to Sona, dialog, …, Language, combo box."; Enter on Get started: "Apple Music, Signing in brings your library…" (the title, then its line); Escape: "alert, Skip the rest of the setup?, …, Skip setup, button." |

The same walk found, and the changes of the same pull request fixed, an
account button read as "Account, button. Account Not signed in.", an artist
link read twice, a focused setup title read as "heading 2" with no words, a
Volume slider read as "80, 80 percent, 80%", and a focused Seek slider read
four times a second as the song played on.

### What a person checks (phase 5)

Nothing here replaces listening. With each release candidate, in a desktop
session with audio:

- **Orca (Linux)**: the walk above covers the keys, the words and the focus
  headless. In a desktop session, with a synthesiser: sign in; walk Home, a
  library page, an album, the queue, Settings and the setup with Tab and the
  arrows; listen for speech that is cut off or runs over the next key, that a
  track change and a queue edit are read once, that "Not available while
  listening to a live station" is read once per key hold, and that closing a
  dialog or menu puts the focus back.
- **VoiceOver (macOS)**: the same walk; the rotor lists headings, links and
  controls, a cover tile's play and more buttons are reachable beside its
  link, a sortable column header reads its direction.
- **Narrator or NVDA (Windows)**: the same walk, and the announcements.
- Both themes, with reduce-motion on and off.

## Known gaps

- A confirmation opened over the first-run setup returns the focus to the
  setup's first control, not to what opened it.
- Orca 50 has been listened to on Linux, through its log on the fixture profile
  (the walk above): not with a speech synthesiser in a desktop session, nor
  signed in, and nobody has yet listened to VoiceOver, Narrator or NVDA
  speak Sona's announcements (phase 5).
- Now Playing opened by a key (N, L) has no opener to return to: Escape
  leaves the focus on nothing, Orca says nothing and the next Tab starts at the
  sidebar. MyGo has no way to read which element has the focus, to remember it.
- A shortcut that shows a panel says nothing: Q opens the queue silently (the
  Queue toggle says "pressed" when it is pressed).
- Opening a page reads the route's title, which is set before the data
  arrives: "Album", not the album's name.
- Orca reads a list item's children with it: each arrow in the queue says
  the song, its artist again, the More and Remove buttons and the line of keys.
- Leaving the Seek slider says its time once ("0:30 of 3:00"), as the held
  value catches up. The player bar's group is read with the text beside the
  song ("Player · Paper Moons, panel") on entering it.
- Of MyGo v0.8.1's states, only the sidebar's current page has been heard
  (the Orca walk above: "Current page", once). Orca's words for the
  disabled-but-focusable, busy, alert, controls and popup states in Sona,
  and VoiceOver's and Narrator's for any of them, are unheard: the table
  above says what the tree holds. The setup strip's one Tab stop is the
  segment last focused, not the current step, because MyGo's focus group
  enters on a checked element only.
- The video title is not a heading: it holds a link, which a heading could
  not before MyGo v0.8.1.
- The hidden buttons of a row cannot be pressed by a test the way a screen
  reader presses them: `access.Node.Can(access.Press)` is what the tests
  check.
