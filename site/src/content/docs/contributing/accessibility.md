---
title: Accessibility Rules
description: The rules every change keeps for the keyboard, screen readers, contrast and reduced motion, how each is checked, and the known gaps.
---

Sona targets WCAG 2.2 AA in both themes. This page holds the rules every change keeps, what each check covers, and what is left to a person with a screen reader.

MyGo's native UI reaches assistive technology through the platform's own API: AT-SPI on Linux (Orca), NSAccessibility on macOS (VoiceOver) and UI Automation on Windows (Narrator, NVDA). MyGo describes a frame only once assistive technology asks, and describes it as a tree of elements with a role, a name, a value, states and actions (`docs/ui/accessibility.md` in MyGo). What that tree can and cannot say shapes the rules below.

For the user's view of the same ground, see the [accessibility page](/accessibility/). For the rest of the UI rules, see [UI conventions](/contributing/ui-conventions/). To report a problem, open an issue on [GitHub](https://github.com/coredev-uk/sona-go/issues/new?template=bug_report.yml).

## At a glance

Each table is one platform: the assistive technology, how it reaches Sona, and what has actually been heard and tested. Only Orca has been run against Sona, and only through its log.

### Linux

| Assistive technology | How it reaches Sona | Heard and tested |
| --- | --- | --- |
| Orca | AT-SPI, from MyGo's GTK backend | Orca 50.2 walked through the app by keyboard, read from its debug log |

- The walk runs in headless sway on the fixture profile (`TestOrcaWalk`, `TestOrcaSetup`).
- The `uitest/access` tests and the AT-SPI e2e tests (`TestAccessibility`, `TestAccessibilityPages`) run in CI.
- Not heard with a speech synthesiser in a desktop session, nor signed in.

### macOS

| Assistive technology | How it reaches Sona | Heard and tested |
| --- | --- | --- |
| VoiceOver | NSAccessibility, from MyGo's AppKit backend | The tree only; nobody has listened |

The `uitest/access` and per-feature tests run on macOS in the nightly and on dispatch. Nobody has listened to VoiceOver speak Sona.

### Windows

| Assistive technology | How it reaches Sona | Heard and tested |
| --- | --- | --- |
| Narrator | UI Automation, from MyGo's Windows backend | Nothing; nobody has listened |
| NVDA | UI Automation, as Narrator | Nothing; nobody has listened |

No accessibility test runs on Windows: its CI job builds and vets the UI, and runs the media, Discord, CDM and playback tests.

## Rules

Each rule area has the same shape: the rule, how to do it in Sona, and what checks it.

### Names, roles and structure

:::caution
Controls are never nested in an opener. MyGo reads a button, link, tab, column header, text and every other widget role as one piece, so a control inside one is not on the tree. `uitest/access` fails the build for it.
:::

**The rule**

- Every control has an accessible name that contains its visible text (2.5.3 Label in Name). A control with no visible text (an icon) is named by its purpose.
- Controls are never nested in an opener. The elements inside an opener are not on the tree, although the pointer and Tab still reach them, so a "more" button drawn inside a row that is a button is invisible to a screen reader.
- Only these roles hold other elements: group, list, list box, option, scroll area, dialog, popup, status, tab list, table, grid, row, cell, column and row header, tree, list item, toolbar, radio group, alert dialog, menu bar, menu, heading, navigation, region, search, article, log, timer and alert. Every control is a sibling of the thing it sits beside.
- A control that shows only while its row is hovered is built at rest too, so assistive technology finds and presses it.
- A list is one Tab stop with arrow keys inside: a focusable row, the others reached by the arrows. Its controls are on the tree but are not stops.
- Links are `RoleLink`. Headings carry their level (`RoleHeading` and `Level`): the page title is level 1, dialog titles level 2. Lists and tables are named by their heading, and lists hold only their items.
- Every dialog is a named `dialog`, or an `alertdialog` when it asks. What is behind a modal is not on the tree while it is open.
- Inline links are underlined, never marked by colour alone.
- Landmarks are few and named for what they hold. A landmark never sits inside another of the same name. A region's own heading or list may repeat its name: a heading and a list are not landmarks.
- What takes the focus without being a control is a group named by its text, not a heading. Orca reads a focused heading's role and level ("heading 2") and takes its words from the text, which MyGo gives to labels.
- A tooltip whose first line is the control's name describes the control by the rest. By itself it is the description, and the name is read twice.
- The artist of a song is one element, never a text with links in it. Orca read the enclosing text before a focused link, so a link inside an artist's text was read twice: "Ines Calder, Ines Calder, link".

**In Sona**

- Names:
  - a song row is named by its text; a tile's play button is "Play {name}" (an icon, so no visible text to contain);
  - a control with no text takes an `A11yLabel` (`kit.Button`, `kit.Input`, `kit.Select`) or `Label`.
- Siblings, not nesting:
  - `kit.DragItem` builds the item's source (the opener) and its `After` controls as siblings in one wrapper (cover tiles, Home rows, artist releases);
  - wide cards and queue rows build the same shape in a wrapper or a list item of their own;
  - a row's controls are cells of the row, which is a row, not a button;
  - a sortable column header is a column header holding its sort button: the button is named by the title, the header says the direction (see `aria-sort` in the states table).
- Hover-only controls are `Opacity(0)` and `IgnoreInput(true)` at rest: the pointer and Tab ignore them, assistive technology finds and presses them. They are the play button of a track row and the more and remove buttons of a queue row.
- Menus:
  - a menu is reached from the row with Shift+F10 or the Menu key;
  - wherever an item has a "More" button (a list row or tile, a page's header, a sidebar row) the same menu opens on a right-click at the pointer;
  - it also opens from Shift+F10 or the Menu key with the focus on the item or, for a page header, on any of its buttons;
  - only these keyboard ways start the menu on its first entry: the button and the right-click leave it unselected;
  - the playing song's menu opens the same way from the bar's cover and from the mini player, and Now Playing also has a "More for <title>" menu button.
- Links are the artist and album cells, "See all" and the sidebar entries.
- Dialogs: `kit.Confirm` builds an `alertdialog`.
- A `kit.Select`'s list is a list box (MyGo's `SelectBase`) of options, each `Checked` while chosen (aria-selected). Its trigger is a combo box.
- Suggestions:
  - the suggestions of a text field of Sona's own (the search field's, the artists filter's, the command palette's results) are a `RoleListBox` of `RoleOption`s;
  - the highlighted one is `Checked` (aria-selected) and pointed at by `ActiveDescendant` on the field, which keeps the keyboard focus;
  - the arrows move the option, as in MyGo's `ComboboxBase`, and the field reads as a combo box with its list shown (`Expanded`);
  - in the search field Enter or a click on a song suggestion opens its song page, like every other result (the chevron on the active row says so);
  - Shift+Enter is Enter: an option holds no control and no cue hints at a second action, so the field has no hidden way to play; the song's page and the results page have the Play buttons (the best match's disc, a song row's button) and the menu.
- Landmarks:
  - the shell's search field is a `RoleSearch` with no name: it is the one global search, and the field in it is already "Search";
  - the Albums page's type and sort are a `RoleSearch` named "Filter albums", and the Artists filter one named "Find artists" (not "Filter artists", which is the field's own name);
  - the player bar ("Player") and Now Playing ("Now playing") are `RoleRegion`s;
  - the queue panel stays a group named "Queue": as a region, or as a group described by its summary, Orca read nothing on entering it, where a group's text makes it say "Queue 2 songs after this one, panel".
- A choice made among pictures (the playlist cover picker) is a `radio group` of `radio` tiles:
  - each tile is named by the label drawn under it (2.5.3), described in a line of its own ("No picture"), and `Checked` while chosen, with arrow keys over the grid (`kit.GridNav`);
  - the chosen tile says so with a ring, a check mark and a heavier name, never by colour alone;
  - where the cover lives ("Only in Sona: Apple Music can't keep covers set here.") is a status at the top of the dialog (`kit.InfoLine`) and the dialog's description, read with its name as the focus enters and not announced again;
  - a choice that applies at once announces its result ("Cover: Bars"), and a failure stays beside the control as an alert, with the limits it broke in view.
- The artist of a song is `kit.ArtistLinks`: a link for one artist. For several it is a menu button named by Apple's artist string, with `haspopup` menu, `expanded` while open and the description "Choose an artist", opening a menu whose rows are `menuitem`s named by the artists.
- `kit.TooltipDescribed` takes the control's name as the first line of the tooltip and describes the control by the rest (the account button's).
- The setup step's title is a group around its heading, so the focus lands on a group that is named by its text.

**Checked by** `uitest/access` (`NoNestedControls`, `TabWalk`), [`internal/ui/kit/a11y_test.go`](https://github.com/coredev-uk/sona-go/blob/main/internal/ui/kit/a11y_test.go) and each feature's `a11y_test.go`. The e2e and Orca walks check the same names on the real app: see [Checks](#checks).

### States MyGo has, and what stands in for the rest

**The rule.** Where MyGo has the state, set it on the element. Where MyGo lacks it, use the stand-in named in the last column, and keep it until MyGo ships the state (see [Known gaps](#known-gaps)).

**In Sona.** The table says which MyGo state carries each ARIA state and what stands in where MyGo lacks it. The list under it says how Sona uses each.

| State (ARIA) | MyGo API | Stand-in where MyGo lacks it |
| --- | --- | --- |
| `aria-checked`, `aria-pressed` | `Checked` on a switch, check box, toggle button, tab or menu item | none needed |
| `aria-activedescendant`, `aria-selected` | `ActiveDescendant` on a field, pointing at the highlighted `RoleOption`, which is `Checked` (`Selected`) | none needed |
| `aria-expanded`, `aria-haspopup` | `Expanded` on the opener, with `RoleMenuButton` or `RolePopUpButton`; MyGo sets `HasPopup` itself only on its own menu components | A plain button's `Expanded(false)` reads as no state, so a closed one carries no `Expanded`: menu buttons say they open a menu with `HasPopup` |
| `aria-invalid` and its message | The invalid state, and the message read with the value | none needed |
| `aria-valuetext` | The slider's description | none needed |
| `aria-disabled` (keeps focus, says why) | `AccessDisabled(true)`: read as unavailable, keeps the focus | none needed |
| `disabled` (really unavailable) | `Disabled`: no focus, no press | none needed |
| `aria-current` | `Current`: `CurrentPage`, `CurrentStep`, `CurrentItem` | none needed |
| `aria-busy`, `role="status"` loading | `Busy(true)`, and a `RoleStatus` | `Busy` only tells a screen reader to wait, so a busy sign-in step is announced once as it starts |
| `role="alert"` | `RoleAlert`: read out when it appears and when it changes | none needed |
| `aria-posinset`, `aria-setsize` | `PositionInSet(pos, size)`: `pos` from 1 and `size` all of them, built or not | MyGo gives a grid's rows and cells `RowIndex` and `ColumnIndex`; a wall of covers is not a spreadsheet, so the cover table says no row or column count |
| `aria-live`, `role="status"` | A `RoleStatus` is read, politely and whole, when its text changes and not when it first appears | `kit.AnnounceMounted` for the first appearance; `c.Announce(text)` for what is not a status |
| `role="tabpanel"` | `RoleTabPanel` | A tab names the panel it owns (`Controls`) only if the panel is built before it in the same pass |
| `aria-controls` | `Controls(target)` | none where another component builds the region |
| `aria-sort` | `Sort(ui.SortAscending/SortDescending)`; `ui.SortNone` on a header that is not the sorted one | none needed |
| `aria-multiselectable`, `aria-rowcount`, `aria-rowindex`, `aria-colcount`, `aria-colindex` | `Multiselectable(true)`, `RowCount`, `ColumnCount`, `RowIndex`, `ColumnIndex` on a table, its rows and its cells | none needed |

How Sona uses each state:

- **`aria-checked`, `aria-pressed`**: `kit.ButtonOpts.Pressed`.
- **`aria-activedescendant`, `aria-selected`**: the search field, the artists filter and the command palette.
- **`aria-expanded`, `aria-haspopup`**:
  - `HasPopup(ui.PopupMenu)` on a menu button Sona builds itself: a tour's tickets, the playlist sidebar's More and New playlist or folder, an item's More, a queue row's More, the sleep timer, the account avatar;
  - `HasPopup(ui.PopupDialog)` on a button that opens a dialog: Set location, a playlist's Name and description and Cover, the collaborators byline.
- **`aria-invalid`**: `kit.InputOpts.Error` and `kit.SelectOpts.Error`.
- **`aria-valuetext`**: `kit.SliderOpts.ValueText`, only where it says more than the value and the percentage a screen reader reads anyway: Seek's "0:42 of 3:00", not Volume's "80%" (Orca read "80, 80 percent, 80%").
- **`aria-disabled`**:
  - `kit.ButtonOpts.Reason`: drawn at half strength and the press ignored by the caller (`AccessDisabled` only tells assistive technology), the reason as its description and the tooltip's second line;
  - a track list's withheld (explicit) row is the same, described by "Explicit content is restricted on this account".
- **`disabled`**: a control that is truly unavailable.
- **`aria-current`**:
  - `CurrentPage` on the sidebar link of the open page (the playlist sidebar's links and Settings' section links too);
  - `CurrentStep` on the setup strip's current step;
  - `CurrentItem` on a chosen city, the lead of a Discover list, the playing queue row and the sung lyric line;
  - a description that held more keeps it (Settings' section summary).
- **`aria-busy`, loading**:
  - `Busy(true)` on the region whose content is loading or changing (the concerts agenda while the next filters load, the sign-in form while a step runs);
  - a `RoleStatus` named "Loading" for a placeholder standing in for the content (skeletons; the setup and account placeholders are both);
  - the button's new label is held back with the rest of the form while the step is announced.
- **`role="alert"`**:
  - `RoleAlert` on the message: an `ErrorBlock`'s title and reason, a sign-in or token error, a refused scrobbler token, the warning that scrobbler sign-ins are stored unencrypted, a cover that could not be set, a video's problem;
  - there is no `c.Announce` beside it, and the buttons beside the message are not inside the alert;
  - a wait (Last.fm's authorisation) or a problem the account section finds on arrival (the keyring notice) is a `RoleStatus`, announced once where it is news.
- **`aria-posinset`, `aria-setsize`**: `PositionInSet` where the siblings do not say which of how many an item is.
  - A cover tile is a cell of a row that is only where the tiles wrap, in a grid that builds only the rows in view, so a tile at index 411 of 5,000 says 412 of 5,000. A shelf's tiles say their place among the tiles it shows, not among those "Show all" would add.
  - A queue row says its place among its list: the queue, or the station's or AutoPlay's part under its heading, which is a list of its own.
  - The sidebar's pins, playlists and folders say theirs among their own list: a folder's playlists are a list inside its item, and the dashed drop row shown while dragging is not counted.
- **`aria-live`, `role="status"`**:
  - a status that comes with news calls `kit.AnnounceMounted` once as it is first built (a note under a filter that finds nothing, in Settings and the shortcut sheet, a sign-in step's notice, the shortcut recorder's conflict);
  - what changes it afterwards is its own to read: never `c.Announce` as well, or it is said twice;
  - a status that stays as its words change is left to MyGo then (the sign-in keyring notice going from waiting to refused or unencrypted, which keeps its retry in place);
  - one shown for something new is keyed by it, so it appears anew and is announced once (the player bar's failure line, keyed by the item);
  - the status is the words, not the row they sit in (the updates row in Settings: a button's label inside a status is read with it);
  - a status with siblings before it is keyed, as its ID is how MyGo tells it from the last frame's;
  - what is not read as it changes is not a status: the notification centre's rows (`kit.ActivityRow`) are groups named by their title, so a count moving is not read out. What matters is said once from the module's `Activity.Status`, which runs whether or not the panel is open: `announceSync` says that a sync starts, ends or fails, and the update row's title is read as the update turns up and as it changes;
  - the notification bell is a button that opens a dialog (`HasPopup(ui.PopupDialog)`, `Expanded` while open) whose name carries what its dot and bar show: how many entries are new and the work under way ("Notifications, 2 new; Syncing songs, 812 of 2,040"). Escape or a press outside closes the panel and gives the focus back to the bell; each entry is a button named by its title and described by its detail, with its own Remove button;
  - anything else that should be said once is `c.Announce(text)` (read once, politely), and so are toasts, which announce their title and description.
- **`role="tabpanel"`**: `kit.TabPanel` is a `RoleTabPanel` named by its tab. Nothing builds one yet: Now Playing's panes are tabs with no panel, because the pane order does not build the panel before its tab.
- **`aria-controls`**:
  - `Controls(target)` on a disclosure that shows or hides a region it owns: `kit.Notes`' "Read more", a read-only sidebar section's fold, a Home index line;
  - a disclosure whose region another component builds sets no `Controls` (a shelf's "Show all", the player bar's Queue toggle and the panel the shell builds beside the page): MyGo's vet refuses an element kept past its build pass, and a handle cannot stand in for it.
- **`aria-sort`**: on the column header of the track list's sorted column, `ui.SortNone` on its other sortable headers. MyGo keeps a column header's children, so the sort button sits inside the header and carries no value of its own.
- **Table counts**: the track list's table is `Multiselectable(true)`, with `RowCount` (songs and the header row) and `ColumnCount`. Each row says `RowIndex` (its song's place, from 2 under the header at 1) and each cell its `ColumnIndex` among the columns shown, so a virtualised list of 2,000 builds only the rows in view and a screen reader still hears the true row and total.

**Checked by** the feature packages' `a11y_test.go` files, which assert each state on the tree, and by `uitest/access`. The tests turn the screen reader on (`tt.UseScreenReader()`) and count what it reads.

### Contrast

**The rule**

- Text tokens are ≥ 4.5:1 on every background up to `--active` and on selected rows (ink at 12%).
- `--control-line` and the focus ring are ≥ 3:1 on ground, surface and raised.
- The spot colour's fill (the play disc, WCAG 1.4.11 non-text contrast) is ≥ 3:1 on ground, surface and hover in light and dark.
- Slider thumbs are always shown: the thumb, not the fill colour, marks the value.
- Text over a cover backdrop gets its contrast from the scrim ([`docs/design.md`](https://github.com/coredev-uk/sona-go/blob/main/docs/design.md), Backdrops). `--danger` falls below 4.5:1 there in dark, so no danger text sits on a backdrop: icons only (≥ 3:1), with the message in ink.
- Selected text keeps `--ink` and `--ink-muted` at ≥ 4.5:1 on the selection (the spot colour at 15%), over ground, surface and raised, in every theme and accent; `theme.Validate` refuses a user theme that fails. So muted text is selectable too, where people copy it ([`docs/design.md`](https://github.com/coredev-uk/sona-go/blob/main/docs/design.md#selectable-text), "Selectable text"; issue #411).
- `Selectable()` adds no role, so a screen reader reads the same tree. An identifier that is selectable also has a Copy button (`kit.CopyButton`) for the keyboard.

**In Sona.** The values live in the theme tokens ([`internal/ui/theme`](https://github.com/coredev-uk/sona-go/tree/main/internal/ui/theme)); [`docs/design.md`](https://github.com/coredev-uk/sona-go/blob/main/docs/design.md) lists them.

**Checked by** `theme.Validate`, which holds every built-in theme to the floors above (and the scrim to ≥ 0.8), through `TestSonasOwnTokensValidate` and `TestBuiltinThemesAreEmbeddedAndValid` in `internal/ui/theme`. `TestHighContrastTextKeeps7To1` holds the High contrast theme to 7:1.

### Focus

**The rule**

- Every focus stop draws a visible indicator (MyGo's focus ring in the ring token) and is on screen when focused.
- Focus returns where it came from: closing a dialog, menu, popover or confirmation returns it to what opened it. Removing a row moves focus to the row that takes its place. Focus never drops to nowhere.
- Modal overlays hold Tab inside and take the page behind them off the tree.
- Menus opened from the keyboard start on their first item. Row menu buttons are not Tab stops (a list is one stop): they open from a screen reader's press, and the row's menu from Shift+F10 or the Menu key.
- Escape on a control whose tooltip shows (Now Playing's Close, reached by keyboard) hides the tooltip first, as WCAG 1.4.13 asks, and closes the view or confirmation on the second press.
- A slider that moves by itself (Seek, as the song plays) keeps, for assistive technology, the value it had when the focus came and after each key. A screen reader reads every change of the focused slider, and the position ticks four times a second. The time and the bar go on moving.

**In Sona**

- Focus return: MyGo notes the opener after the overlay's content is built, so a control that takes the focus in the overlay's first frame (`AutoFocus`) would be taken for it. Use `kit.FocusWhenShown`, which focuses one frame later.
- The queue is one Tab stop with arrow keys inside:
  - ↑/↓ one row, Home/End the current and the last song, Alt+↑/↓ moves a song, Delete removes it, Enter plays it;
  - Escape in the queue closes it and returns the focus to the player bar's queue toggle;
  - in the Now Playing view's Up next, Escape closes the view.
- The video view is a group named "Music video", described by a line listing its keys:
  - it takes focus when it opens and keeps Tab inside;
  - keys: Space or K play and pause, ← and → seek 5 s, F toggles full screen, M mutes, Escape leaves full screen and then closes, V closes;
  - its controls never fade while one has keyboard focus;
  - loading and buffering are read once, by the status in its header;
  - a failure is an alert with "Watch in Apple Music" and, where it can help, "Try again" beside it.
- The Now Playing view's panes are tabs (Lyrics, Up next): one Tab stop, ←/→ and Home/End move and show.
- The shortcut help:
  - `?` opens the shortcut help on every platform, and closes it again; the palette's "Keyboard shortcuts" opens it too;
  - on Linux (GTK) and macOS (AppKit) MyGo v0.9.0 gives Shift with a punctuation key no `Key`, only its position (`InputEvent.Code`), so Sona reads `?` there as Shift and the key where US and UK keyboards have `/` and `?` (`kit.ShiftSlash`, with `kit.SlashByPosition` for the label);
  - on another layout that is a key labelled otherwise: German types `?` with Shift+ß and has `-` in that place, so Shift+- opens the help there and Shift+ß does not;
  - the command's label says so on those platforms ("Keyboard shortcuts (? where US and UK keyboards have it)") while the help answers to `?`; Windows reads the key the layout types, and its label is plain;
  - bound to another key in Settings → Shortcuts, the help answers to that key alone, and any shortcut bound to `?` takes the press;
  - a `?` typed in a text field is text, and the key recorder records it as `?`.

**Checked by** the per-feature `a11y_test.go` files (Escape, focus return, Tab walk) and `uitest/access` `TabWalk`. The Orca walk checks that closing a menu, dialog or the queue gives the focus back, and that the focused Seek slider is silent while the song plays.

### Live announcements

**The rule**

- MyGo announces through `c.Announce` (VoiceOver and Orca announcements, a live region on Windows) and every toast.
- Each change is said once. What is on screen when a page opens is read with the page, not announced again.
- Lyrics are not a live region.

**In Sona**

- Track changes are announced politely ("Now playing: {song} by {artist}", or "Now playing a preview: …", [`kit/playerqueue_announce.go`](https://github.com/coredev-uk/sona-go/blob/main/internal/ui/kit/playerqueue_announce.go)) once the engine reports the track as playing. The track restored paused at launch, play/pause, seeking and repeating the same track stay quiet.
- Moving a queue song says its new place. Remove says nothing, and Play next and Play last from an item's menu raise a toast. A playlist editor's Alt+↑/↓ says the new place too.
- A control that does not apply to what plays uses `ButtonOpts.Reason`: Previous, Next, Shuffle, Repeat and the AutoPlay toggle on a live station, and Shuffle, Repeat and AutoPlay on a station of songs. A shortcut for one does nothing and says the same reason once in a toast (`kit.Unavailable`). The toast stays about 4 s and the reason is not raised again while it shows, so a key held longer is read again about every 4 s.
- Errors, warnings, update and diagnostics verdicts, the scrobbler account line, busy steps of sign-in and setup, and filter results with no match are announced as they appear or change:
  - errors are alerts;
  - the verdicts, the video's loading and buffering, the shortcut filter's and the playlist picker's empty notes and the sign-in notice are statuses that MyGo reads when their text changes (the first appearance through `kit.AnnounceMounted`);
  - the rest go through `c.Announce`.
- Lyrics: each synced line is one button named by its text, and the sung line is the current item. An instrumental gap's dots are an image named "Instrumental break" while the gap is to come or running, and nothing once it has passed.

**Checked by** the per-feature `a11y_test.go` files, which turn the screen reader on (`tt.UseScreenReader()`) and count what it reads, and by the Orca walk (a track change and a queue move are read once).

### Keyboard alternatives for drag and drop

**The rule.** Every drag has a keyboard alternative, and a move says the item's new place. The shortcuts are listed in [Keyboard Shortcuts](/guide/keyboard-shortcuts/).

**In Sona**

| Drag | Keyboard |
| --- | --- |
| Songs, albums, playlists onto a sidebar playlist | "Add to playlist" in every item's action and context menu: a sub-menu (Right opens it, Left closes it) of New playlist… and your playlists |
| Onto the queue | "Play next" / "Play last" in the same menus |
| Reorder the queue | Alt+↑ / Alt+↓ on a focused queue row, or "Play next" and "Move to end" in its menu |
| Reorder a playlist in the editor | Alt+↑ / Alt+↓ on a focused row, announced |
| Reorder pins in the sidebar | "Move up" in the pin's menu |

- On a queue row Delete removes and Enter plays. The row's menu opens with Shift+F10 or the Menu key.
- Each queue move is announced with the song's new place. Only the user's songs move, and they stay before the station's or AutoPlay's songs.
- Scrolling over the volume (a mouse wheel, a trackpad's swipe; `kit.VolumeWheel`) changes it too, as a convenience for the pointer only. The wrapper it sits on is not a stop and has no role.
- The volume slider keeps its arrows, Page Up and Page Down, Home and End, with Ctrl+↑ and Ctrl+↓ (5% each) and M (mute) anywhere.

**Checked by** the per-feature `a11y_test.go` files and the Orca walk (Alt+↓ in the queue).

### Reduced motion

**The rule.** The platform's reduce-motion setting and Sona's reduce-effects mode set every transition and animation to none. State changes still happen, without movement.

**In Sona**

- `theme.ReducedMotion` is the single switch.
- Word-timed lyrics light each word whole as it begins, and no word lifts, swells or glows (`SweepClock.Still`); an instrumental gap's three dots light whole, one as each third of the gap begins, in place of a fill in steps.
- Motion artwork does not play: Settings → Diagnostics → Motion artwork says which input stops it. Covers show their still.

**Checked by** `TestReducedMotionAndEffects` in `internal/ui/theme`.

## Checks

### The automated checks

| Check | What it covers | How to run it |
| --- | --- | --- |
| `uitest/access` | The tree MyGo describes, read from a `ui.Tester` | `devenv shell -- go test ./internal/ui/uitest/access/` |
| `internal/ui/kit/a11y_test.go` | The kit's list, item and form components | `devenv shell -- go test ./internal/ui/kit/` |
| `internal/ui/**/a11y_test.go` | Each feature package's pages | `devenv shell -- go test ./internal/ui/...` |
| `e2e` `TestAccessibility`, `TestAccessibilityPages` | The real app's tree on the AT-SPI bus | `devenv shell -- headless -- go test -tags e2e ./e2e/...` |
| `e2e` `TestOrcaWalk`, `TestOrcaSetup` | A real Orca walked through the app | `devenv shell -- orca-walk` |
| Contrast | The theme floors | `devenv shell -- go test ./internal/ui/theme/` |

What each covers:

- **`uitest/access`**:
  - roles, names, descriptions, states and actions of a page;
  - `NoNestedControls` fails for every clickable or focusable element inside a button, link or other leaf role;
  - `TabWalk` presses Tab round the window and fails for a stop that is not on the tree or a walk that never comes back (a trap);
  - `TestPinnedMyGo` pins the MyGo version the helper was checked against.
- **`internal/ui/kit/a11y_test.go`**: each list and item component (cover grid, shelf, item grid, track list, track shelf, stations, link list, wide row), their place in the set, form controls and their states, dialogs, confirmations, toasts, menus.
- **`internal/ui/**/a11y_test.go`**: per feature package, its pages' trees and Tab walk in reading order, Escape and focus return for the overlays it owns, announcements, the states table above.
- **`e2e` `TestAccessibility`, `TestAccessibilityPages`**: the window's landmarks, sidebar links and each library page's heading, with the roles and names a screen reader gets.
- **`e2e` `TestOrcaWalk`, `TestOrcaSetup`**: what Orca said, from its debug log: see [Listening with Orca](#listening-with-orca).
- **Contrast**: computed over every built-in theme: text 4.5:1, control line and focus ring 3:1, the spot fill 3:1, the scrim at 0.8 or more, High contrast text 7:1 (`TestSonasOwnTokensValidate`, `TestBuiltinThemesAreEmbeddedAndValid`, `TestHighContrastTextKeeps7To1`).

Where CI runs them:

- The `ui.Tester` checks are `go test`s, so they run wherever CI runs the Go tests: Linux on every pull request, macOS in the nightly and on dispatch. Windows builds and vets them but does not run them.
- The e2e check runs on Linux only (headless sway, AT-SPI), in the CI "E2E (headless sway)" job.
- The Orca walk runs in the CI "Orca walk (headless sway)" job, on dispatch only (`only` = `orca` or `all`), and with `orca-walk`.

### Listening with Orca

`TestOrcaWalk` and `TestOrcaSetup` ([`e2e/orca_walk_test.go`](https://github.com/coredev-uk/sona-go/blob/main/e2e/orca_walk_test.go)) run a real Orca beside the app in the e2e harness's headless session, on the fixture profile, and press keys with `wtype`, as a keyboard does. Orca writes every utterance it generates to its debug file (`--debug-file`) whether or not a speech server is there, so nothing is spoken and no audio is involved: the words checked are the words Orca would say.

1. Run the walk. It builds Orca from the pinned nixpkgs on first use:

   ```sh
   devenv shell -- orca-walk                  # Orca from the pinned nixpkgs; what it said: bin/orca-out
   E2E_ORCA=1 e2e/headless.sh -- go test -tags e2e -count=1 -v -run '^TestOrca' ./e2e/   # `orca` on the PATH, or E2E_ORCA_BIN
   ```

   Without `E2E_ORCA=1` the tests skip. Orca's own English (`LC_ALL=C.UTF-8`) is the reference.
2. The walk presses keys: Tab through the sidebar, the account button, menu and dialog, a library page (Albums: the filter, Sort, the tiles, arrows, Shift+F10), an album (artist link, buttons, song list, Enter), the queue (Q, a move, a removal, Escape), Now Playing (its tabs, Escape), the player bar's Volume and Seek sliders and a track change, and then the first-run setup.
3. After each key the walk waits for Orca by its log, not by fixed pauses: for Orca to have been told the key was released, for its event queue to be empty, and for 0.6 s in which Orca said and queued nothing (a slider's value ticking as the song plays does not count). On the CI runner no answer to a key took more than 0.38 s from the release to its last word, and the longest silence inside one was 0.22 s. Orca's log is read as it grows, each entry parsed once ([`e2e/orca_log_test.go`](https://github.com/coredev-uk/sona-go/blob/main/e2e/orca_log_test.go) checks that against a parse of the whole file).
4. On every key the test fails for a name or text said twice in a row (a link read with the text it is in) and for a flood of more than 16 utterances (a value read as often as it changes).
5. Per surface it checks the roles, names and states listed below, that a track change and a queue move are read once, that closing a menu, dialog or the queue gives the focus back to what opened it, and that the focused Seek slider is silent while the song plays.
6. Read the result. The run writes `orca-walk.md` and `orca-setup.md` to `E2E_OUT` (each key, what Orca said for it, and where it put the focus), and Orca's log.

CI runs the walk only when the CI workflow is dispatched with `only` = `orca` or `all`: never on a pull request or a push, and not a required check, because the words and the timing are Orca's and a new Orca can move them. The CI job runs in the CI image with Orca installed on top.

What it heard, with Orca 50.2:

- **Sidebar, first Tab**: "Main", "For You", "link."; then one link per Tab, the open page "Albums, link." with "Current page" (its current-page state), "New playlist or folder, push button menu. opens popup", "Playlists, List with 3 items", the pins as links.
- **Top bar**:
  - "search, Search, editable combo box.";
  - the next Tab "leaving search., Account, button. Not signed in. opens popup" (the name once; the popup is `HasPopup(ui.PopupMenu)`);
  - Enter: "Account, menu, Sign in."; Escape: the button again.
- **Account dialog**: "Account, dialog, Apple Account, entry. The email address or phone number you use with Apple.", "Password, password text.", the browser and token buttons, "Close, button."; Escape returns to "Account, button."
- **Albums page**:
  - "Albums, panel, search, Filter albums, Album type, panel, All, toggle button pressed." (and "leaving search." on the first tile), "Sort, combo box.", a tile "Fieldnotes, link. Kofi Bellamy.";
  - the arrows move between tiles;
  - Shift+F10 "More for …, menu, Create station."; Escape returns to the tile.
- **An album**: "Album" on arrival; "Ines Calder, link." (once); "Play …, button. Play."; "Favourite …, toggle button not pressed."; the song list one stop, "Dust on the Dial." per arrow; Enter: "Now playing: … by …" once.
- **The queue**:
  - "Queue 2 songs after this one, panel, Clear, button."; the list one stop "Queue, List with 3 items";
  - Alt+↓ "Dust on the Dial: moved to position 2 of 2";
  - Delete moves the focus to the next row; Escape: "Queue, toggle button not pressed."
- **Now Playing**: "Lyrics, page tab.", ←/→ "Up next, page tab.", "Close now playing (Escape), button."
- **Player bar**: "Volume, horizontal slider, 80, 80 percent."; "Sleep timer, push button menu. opens popup"; "Seek, horizontal slider, 42, 23 percent. 0:42 of 3:00." and then nothing as the song plays; Ctrl+→ "Now playing: … by …".
- **Setup**: "Welcome to Sona, dialog, …, Language, combo box."; Enter on Get started: "Apple Music, Signing in brings your library…" (the title, then its line); Escape: "alert, Skip the rest of the setup?, …, Skip setup, button."

The same walk found, and the changes of the same pull request fixed, an account button read as "Account, button. Account Not signed in.", an artist link read twice, a focused setup title read as "heading 2" with no words, a Volume slider read as "80, 80 percent, 80%", and a focused Seek slider read four times a second as the song played on.

### What a person checks

Nothing here replaces listening. With each release candidate, in a desktop session with audio, work through this list:

- [ ] **Orca (Linux)**: the walk above covers the keys, the words and the focus headless. In a desktop session, with a synthesiser, sign in and walk Home, a library page, an album, the queue, Settings and the setup with Tab and the arrows.
- [ ] Orca: listen for speech that is cut off or runs over the next key, and that a track change and a queue edit are read once.
- [ ] Orca: "Not available while listening to a live station" is read once per key hold, and closing a dialog or menu puts the focus back.
- [ ] **VoiceOver (macOS)**: the same walk. The rotor lists headings, links and controls, a cover tile's play and more buttons are reachable beside its link, and a sortable column header reads its direction.
- [ ] **Narrator or NVDA (Windows)**: the same walk, and the announcements.
- [ ] Both themes, with reduce-motion on and off.

## Known gaps

MyGo v0.9.0 closed two gaps. A `RoleStatus` is now read when its text changes, and the tab panel has a role (`RoleTabPanel`, adopted by `kit.TabPanel`). The gaps still open in v0.9.0 each keep the stand-in named in the table. The ones owned by MyGo are tracked in [#318](https://github.com/coredev-uk/sona-go/issues/318) under the number in the Owner column, and no issue for them is open in MyGo itself yet.

| Gap | Effect for the user | Workaround | Owner |
| --- | --- | --- | --- |
| No `Key` for Shift with a punctuation key (Linux, macOS) | `?` opens the help by its position, so on another layout it is another key | `kit.ShiftSlash` | MyGo upstream, #318 B1 |
| No `Key`s for F13 to F24 | A shortcut cannot be bound to them | `kit.ParseChord` has no names for them | MyGo upstream, #318 B2 |
| `Expanded(false)` on a plain button reads as no state | A closed plain button says nothing about being expandable | `HasPopup` on menu buttons | MyGo upstream, #318 B3 |
| No text input hints (`required`, `inputMode`, `spellcheck`) | A field cannot say it is required, numeric or spell-checked | None | MyGo upstream, #318 B6 |
| The focus of an overlay's first frame | `AutoFocus` in an overlay is taken for the opener | `kit.FocusWhenShown` | MyGo upstream, #318 B7 |
| No way to read the tree and press by it (tests only) | `uitest/access` reads the Tester by reflection | `Tester.AccessAll` only | MyGo upstream, #318 B8 |
| A tab cannot name its panel (`Controls`) | Now Playing's tabs do not say which panel they own | `kit.TabPanel` has the role | Sona |
| `?` needs the focus in the window (Linux, macOS) | `?` does nothing until something has the focus | Press Tab first, or use the palette (Ctrl+K) | MyGo upstream, not in #318 |
| A confirmation over the first-run setup returns the focus to the setup's first control | The focus is not where the user was | None | Sona |
| Now Playing opened by a key (N, L) has no opener to return to | Escape leaves the focus on nothing | Press Tab | MyGo upstream, not in #318 |
| A shortcut that shows a panel says nothing | Q opens the queue silently | The Queue toggle says "pressed" | Sona |
| Orca says nothing on entering a region (Linux) | The Player bar and Now playing are not announced | The queue panel is a group | Orca |
| Opening a page reads the route's generic title | "Album", not the album's name | None | Sona |
| Orca reads a list item's children with it (Linux) | Each arrow in the queue reads the whole row | None | Orca |
| Leaving the Seek slider says its time | "0:30 of 3:00" once | None | Sona |
| The setup strip's one Tab stop is the last segment focused | The strip is entered on the wrong step | None | MyGo upstream, not in #318 |
| Not listened to: a synthesiser, a signed-in account, VoiceOver, Narrator, NVDA | Announcements and states are read from Orca's log only | [What a person checks](#what-a-person-checks) | Sona |

Detail for the rows that need more:

- **Shift with a punctuation key**:
  - the command's label says where the key is on those platforms ("Keyboard shortcuts (? where US and UK keyboards have it)");
  - on a layout that puts another key there (German: Shift+-) the help answers to that key.
- **`?` needs the focus**: at the start press Tab first (or use the palette, Ctrl+K), as MyGo shows no key to the window while nothing has the focus.
- **Overlay first frame**: a control with `AutoFocus` in an overlay is taken for the opener, so closing the overlay has nowhere to return the focus; `kit.FocusWhenShown` focuses one frame later.
- **No tree read**:
  - only `Tester.AccessAll` exists (names and bounds, no node IDs, parents or actions), so `uitest/access` still reads the Tester by reflection;
  - `TestPinnedMyGo` fails on a bump of MyGo until the helper is re-checked;
  - the hidden buttons of a row cannot be pressed by a test the way a screen reader presses them: `access.Node.Can(access.Press)` is what the tests check.
- **A tab cannot name its panel**: Now Playing's panes are tabs with no panel, and a tab says which panel it owns (`Controls`) only when the panel is built before it in the same pass, which the pane order does not allow.
- **Confirmation over the setup**: probably the same cause as #318 B7, not confirmed.
- **Now Playing opened by a key**: Escape leaves the focus on nothing, Orca says nothing and the next Tab starts at the sidebar. MyGo has no way to read which element has the focus, to remember it.
- **A shortcut that shows a panel**: Q opens the queue silently (the Queue toggle says "pressed" when it is pressed).
- **Regions**:
  - the Player bar and Now playing are `RoleRegion`s, which a screen reader's landmark keys find, but Orca 50 announces only the search landmark ("search", "leaving search") as the focus enters and leaves it;
  - the queue panel is left a group for that reason (a region described by its summary was tried: still silent);
  - landmark navigation itself, and VoiceOver's and Narrator's words, are unheard.
- **Page title**: the route's title is set before the data arrives.
- **List items**: each arrow in the queue says the song, its artist again, the More and Remove buttons and the line of keys.
- **Seek slider**: leaving it says its time once ("0:30 of 3:00"), as the held value catches up.
- **Setup strip**: its one Tab stop is the segment last focused, not the current step, because MyGo's focus group enters on a checked element only.
- **Not listened to**:
  - Orca 50 has been listened to on Linux, through its log on the fixture profile (the walk above): not with a speech synthesiser in a desktop session, nor signed in;
  - nobody has yet listened to VoiceOver, Narrator or NVDA speak Sona's announcements;
  - of MyGo's states, only the sidebar's current page has been heard (the Orca walk above: "Current page", once);
  - Orca's words for the disabled-but-focusable, busy, alert, controls, popup, sort and row-count states in Sona, and VoiceOver's and Narrator's for any of them, are unheard: the states table above says what the tree holds.
