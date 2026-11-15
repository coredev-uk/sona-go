# Commands: the palette and the shortcut sheet (direction "Launcher")

## Direction

Both surfaces become launchers that teach their keys. The palette's rows gain a quiet icon tile and, when ranked, their group as context; it opens on the places to go. The shortcut sheet stops being a bare definition list and becomes a sheet you can search: a filter in the head, groups as ruled columns, each group's keycaps starting on one edge, and a foot that names the keys that move around.

## Composition by region

**Palette** (frame `kit.Palette`, unchanged)
- Entry: the icon sat bare in a 16 DIP cell; now it is in a 32 DIP tile (`surface`, radius 6) the width of a library cover, so command rows and library rows share one leading column. Commands with no icon (a sidebar link, a shell action) show the `command` glyph instead of a hole. The "Apple Music" row is a tile too.
- Ranked results: the group (Playback, View…) is the second line of each command, because no heading says it any more.
- Empty-query order: Pages, Playback, View, Navigation, General (was Playback first). Enter right after Ctrl+K now goes somewhere rather than pausing the music.
- Foot hints (↑↓ Move, Enter Run, Esc Close) already exist in the frame; nothing to add.

**Help sheet** (new composition)
- Head: the title at the left, a `kit.SearchField` ("Filter shortcuts") at the right; "Change any of them in Settings." moved to the foot.
- Body: two columns of groups, each a title over a hairline and 32 px rows. Keycaps are in a key column per group, sized to its widest chord (estimated from the chord text in `keycapsWidth`, no text measuring), left-aligned, so they share an edge. "Not set" sits in the same column in `ink-faint`.
- Foot: Settings line, then ↓ Browse and Esc Close.
- 820 wide (was 760), placed 10 % from the top so the field does not move as the filter shortens the list; the list has a minimum height for the same reason.

## Behaviour

- Focus starts in the filter (was: the list). Down moves into the "Shortcuts" group, which scrolls as before; Up at its top comes back. Tab: filter, Settings, Close.
- A shortcut stays when every typed word (folded for case and accents) is in its label, keywords, group title, or its chords (as the caps spell them and as spoken). The count is announced; a filter that leaves nothing says so.
- Escape clears a filled filter first (`kit.SearchField`), then closes.
- "?" in a focused field is text to MyGo, so the sheet's own chord could not close it from the filter, where focus starts. The dialog now observes the press of Shift and the slash key itself (by position, or with its Key) and closes the sheet while the filter holds nothing, or only the typed "?"; after other text a "?" stays text. From the list or the Settings link the chord works as before.

## States covered

Filled (light, dark goldens), filtered, filter with no match, narrow (one column, as before, no new golden). The palette's empty, ranked, library and Apple Music rows. No loading or error state exists (the lists are in-memory; the library lookup keeps its previous answer).

## Tests

- `TestQuestionMarkOpensTheHelpListingGroups`: the pinned "list takes the focus" became "filter takes the focus, Down enters the list, Up returns" (the focus contract moved, deliberately).
- `TestHelpsOwnChordClosesItAndSettingsLinkOpensSettings`: the chord is now tested both ways (typed into the empty filter; pressed from the list).
- a11y help: Tab stops are Settings, Close, then round to the filter (focus starts there).
- New: `TestHelpFilterNarrowsShortcutsAsYouType` (label, keys, keywords, group, no match, count announced, Escape clears then closes), `TestHelpQuestionMarkAfterTextIsText`.
- Goldens regenerated: `palette-light`, `palette-dark`, `help-light`, `help-dark`.

## Constraints

- The palette frame (field, rows' layout, section headings, hint foot, 600 DIP width) is `kit.Palette` and outside this package's boundary; only the entries, section order and leading content could change. A bolder frame (an active tile, wider panel, group headings with counts) needs a kit change.
- Shortcuts carry no per-command description in the DTO, so a row's second line is its group; there is no longer line of help text.

## New strings

`Filter shortcuts`, `Browse`, `No shortcuts match “{term}”`, `Try part of a command's name, or a key.`, `{count, plural, one {# shortcut} other {# shortcuts}}`. The catalogues are regenerated centrally.
