# Playlist redesign: the J-card

## Direction

A playlist is a tape: the cover is the front panel of a J-card and the facts are its spine. The page was a collection header (a cover, a stack of meta text, a row of buttons) over a table with a search field floating above it. It is now one head and one strip: a larger cover beside a text column as tall as the cover (title, byline and description at the top, every action on the cover's bottom edge), then a ledger strip of the facts as cells with the song finder at its right end, then a table that says when each song would start if the tape played through.

## Composition by region

- **Head** (`header.go`, replaces `kit.CollectionHeader` for this page): cover 224 (144 narrow), text column stretched to the cover, actions on the cover's bottom edge instead of under the description. The wash stays (`kit.Backdrop`).
- **Ledger strip**: kind (ink, 600), songs, running time, "Updated …" as 13/18 cells with hairlines between. The catalogue line `Playlist · 6 songs, 20 min` is gone as one string. "Updated …" keeps its tab stop, tooltip and description (the exact date).
- **Finder**: moved from a field above the table into the strip's right end; "N of M songs" sits left of the field while a query is held (a reversed row, so the field is built first and the count reads the query it holds).
- **Track list**: "Starts at m:ss" on each row's second line while the list is in the playlist's own order; hidden once sorted or filtered. `startOffsets` is a pure function (`starts.go`); `page.starts` keeps the formatted labels per tracks (first song's address and count, the key `view` and `addedBy` use: edits make a new slice).
- **Edit mode**: a header row over the rows (the table had none), and a real "Starts" column beside the duration that follows each optimistic move.
- **Empty states**: flush-left title and sentence under the strip (`emptyBlock`), not a dashed box.
- **Suggestions**: rows of the table's height and columns (number column empty, art in the art column, title, artist in the artist column, Add in the album column, length), the playlist's next rows, not a 300px tile grid.
- **Featured artists**: 100px minimum round tiles (were 160+); more-from-curator 144px minimum.
- **Skeleton** (`pageSkeleton`): the new layout (cover, title bars, actions, strip, rows), no longer `kit.DetailSkeleton`.
- **Sidebar**: headings at 12/600 `--ink-muted` with a faint tabular count; playlist rows' 20px cover now leaves the name on the navigation's label edge (10 + 20 + 6 = 36); folder names at weight 500, and an open folder's contents hang from a hairline under the folder icon. Drag/drop, folding, menus and keyboard are untouched.
- **Dialogs and cover picker**: unchanged in structure. They already use `kit.Dialog`, the same type roles, radii and the 44px option rows as the track list; the picker's 4-column grid is exactly 3 rows with an image kept and the golden shows it tidy. Nothing was changed there on purpose rather than restyled for its own sake.

## States covered

Loading skeleton, not found / error with Retry (unchanged `loadError`), loaded (library, catalogue, collaborative), found and sorted (no "Starts at"), edit mode, empty and empty edit, suggestions loading (skeleton rows), signed-out is the shell's.

## Tests

- Changed: `TestPageMetaLineKindsAndUpdated` became `TestPageLedgerKinds` (the catalogue line is cells now); page and a11y tests that waited for `"Playlist · 3 songs, 10 min"` or `"Singer · 3:20"` wait for the cells instead; the Tab-order test pins "Play, then Updated, then the finder" (the note moved below the actions); the editor a11y test expects a header row of column headers before the three rows; the updated-tooltip golden tabs to the note.
- Added: `starts_test.go` (offsets, nil without lengths, cache keyed by tracks), goldens `playlist-page-narrow-light`, `playlist-edit-dark`, `playlist-empty-light`, `playlist-featured-dark`; the sidebar golden now opens the folder.
- Deleted: none.

## Constraints hit

- `kit.TrackList` has no extra-column option, so "starts at" is the row's second line (its `Subline` slot), not a column beside the duration. In edit mode, which the package draws itself, it is a true column.
- No kit edits: the header is rebuilt locally from `kit.Backdrop`, `kit.Artwork`, `kit.MotionCover`.
- The finder is not in the table's own header row (kit-owned); the strip right above it is its row.

## New strings

`Starts at {time}`, `Starts`, `Reorder`.
