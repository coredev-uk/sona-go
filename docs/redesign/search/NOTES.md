# Search redesign: "Card catalogue"

Branch `feat/rd-search`. Package `internal/ui/features/search` (results page, "see all" page, profile page, the top bar's suggestion list).

## Direction

The old page was a stack of equal shelves under a small title, with the top results as a three-column list and a single 192px artist circle. The new one reads like a card catalogue: the term is the page title, a rail on the left is the index of what was found (each group with its count), and the results column is a few drawers of different shapes: one big best-match card, a row of round portraits, dense cover shelves, a track list. The group you want is one click (or Tab and Enter) away, not a scroll through five shelves.

## Composition by region

- **Header.** The term at 32/36 (was 24/30) with the scope switch on its baseline.
- **Rail** (wide, 960px page or more): 200px column outside the scroll container (stays put). One jump button per group found: name, count in tabular figures. A jump scrolls the group to the top and moves the focus to it (named by the group's title while it holds it); the group in view is on the hover tone (the last anchor whose place is at the top). Narrow: a wrapping row of bordered chips above the results.
- **Top results.** The first is a best-match panel (surface fill, 8px radius, 120px cover, kind, name at 24/650, catalogue line, the amber Play disc where it plays: a disc drawn inside the panel when the panel itself plays, a `PlayButton` of its own when the panel opens a page and the result can also be played). Next four results are compact rows beside it (below under 640px of results width). The old three-column grid of nine rows is gone.
- **Artists.** One row of round portraits (104px minimum, the shelf grows them to fill) instead of a grid of 192px circles.
- **Albums, playlists, the rest.** Cover shelves with 136px minimum tiles (160 elsewhere), each group under a hairline.
- **Songs.** The kit `TrackList`, as before; the Album column drops below 720px of results width.
- **Suggestion popup.** Runs now have headings with an icon (Suggestions, Top results, Recently searched, In your library); item rows end in their kind's icon (song, album, artist, playlist, station, video); the active row swaps that icon for a chevron. Keyboard behaviour (`combobox.go`, active index, roving, Escape, Tab) is untouched.
- **"See all" page.** The title is 32/36 like the results page; the rest is unchanged.
- **Profile page.** Unchanged (its header was already display-sized).

## States

- Loading: the same layout in skeleton (rail bars, panel, rows, a shelf of portraits), labelled "Searching" as a status. The last term's results still stay on screen while the next load (the page is `Busy`).
- Nothing found (Apple Music or library): title at 24/650, one sentence, one ink button that searches the other scope (new).
- Signed out: same composition with the Sign in button.
- Error: the kit's `ErrorBlock` with Try again, unchanged.
- No query: recent searches and browse categories (no rail), library scope: the empty-state sentence.

## Tests

- Deleted/rewritten: `TestLibraryOpeningAnAlbumRemembersTheLibraryTermAndAlbum` clicked a point 110 DIPs under the "Albums" heading, which now finds the rail's button first; it jumps with the rail's "Albums, 2" button, Tabs to the first tile and presses Enter, which also exercises the jump's focus.
- Added: rail lists the groups and jumps (scroll, focus, rail stays), narrow chips sit above the results, nothing found offers the other scope (both ways).
- Goldens regenerated: `page_light`, `page_dark`, `field_light`, `field_dark` (now with the active row). New: `page_narrow_light` (760px), `page_empty_light`, `page_loading_light`.
- Tab order: the rail is the first stop of the results page (before the scope switch), as Discover's contents are.

## Constraints

- A group's count in the rail is the number of items Apple sent in that section (the services give no total).
- Top results beyond the best match and four rows are not shown (they were nine); they are the same items the groups below list.
- The "Show all" toggle of cover shelves, selection, menus and keyboard navigation are the kit's `ItemShelf`, so its heading style and spacing are kept; the hairlines and anchors are a wrapper around it.
- Baseline alignment of the switch uses MyGo's `AlignItems(Baseline)`.

## New strings

`Result groups`, `{title}, {count}`, `Search your library` (action; the title of the library scope's empty state already used it), `Search Apple Music`.
