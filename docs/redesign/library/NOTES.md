# Library redesign: "The crate"

## Direction

A record crate: dividers, letter tabs and dated sections rather than one undifferentiated shelf. Every library page now opens with the same header (title, count on its baseline, controls at the right, one rule), long name-ordered lists get an A–Z rail at the right edge, Recently added is filed under its ages, History is a timeline, and the Artists page and the sidebar sync footer are recomposed to match.

## Composition by region

- **Header (all pages)**: `pageHead`. Title 24/650 with the count as a 13 muted tabular figure on its baseline; controls on the same line at the right; a 1px `--line` below. Replaces `kit.PageHeader*` (title over a subline).
- **Albums / Songs**: letter rail (A–Z, `#`) outside the scroll container; letters present enabled, others `--control-line`; jumps to the first item of the letter. Shown for ascending Title/Artist (Songs also Album). One Tab stop (`FocusGroup`), Up/Down, "Jump to A" labels. Songs keeps its sort select and Play/Shuffle in the header.
- **Artists**: title and count share a baseline; letter headings are 13/600 ink with a rule; 36px portraits; letter rail at the list's edge; detail header with a 144px portrait, actions, a rule, and section headings with counts and rules.
- **Recently added**: groups Today / Past week / Past month / Earlier, each a heading with its count and a rule over its own cover grid.
- **History**: a left rail with the day heading and counts, a 1px line with a dot, the day's track list to the right.
- **Favourites, Playlists, Music videos**: the shared header; their bodies keep the track table and cover grids. Music videos adds a note that the list comes from Apple Music.
- **Sync footer**: idle is one line (green dot, "Synced 2 minutes ago", a Sync now button with a hover fill); running or failed is a `--hover` card with the domain left, the count right, and a full-width 3px bar.

## States

Loading skeletons, gate (signed out, syncing, empty), error with Retry and the filtered-none state all sit under the new header. Recently added's skeleton is a heading bar over the tile skeleton; History's is a rail with two bars over the row skeleton. The rail shows every letter faint until the letter index is known.

## Constraints hit

- `LibraryService` has no letter index. Lists of one page (≤ 200) read letters from the page; longer ones bisect with single-row reads on a worker (26 letters × log₂ n reads, once per sort until a sync). The library's order is SQLite `NOCASE` (ASCII only), so the rail files by the first byte lower-cased; names starting with a non-ASCII letter sort after Z and are not reachable from the rail (`#` jumps to the leading digits and symbols). The rail shows only for ascending order.
- Recently added's groups are exact when the first page (200 newest) reaches Earlier, which is the normal case. If 200 additions fit in the last group before Earlier, the rest of the list is labelled Earlier even if some of it is from the same period.
- The jump to a row uses the kit's constants (`kit.TrackRowHeight`, the table header 32 + 4 in `songsHeadHeight`, `kit.CoverRowHeight`): a change to the kit's geometry must change them.
- The count text stays one whole phrase ("24 albums") so tests and screen readers keep one msgid per plural.

## Tests changed

- `a11y_test.go`: Recently added's table is named "Recently added, Earlier" (one table per group).
- `recent_test.go`: the selection state is the Earlier group's.
- `history_test.go`: the day's counts and "from other devices" are two lines now, not one joined with a dot.
- Goldens regenerated: albums, songs, artists (light, dark). New: recent (light), history (light, dark).

## New strings

"Jump to letter", "Jump to {letter}", "Jump to numbers and symbols", "Past week", "Past month", "Earlier", "Recently added, {when}", "From Apple Music: needs a connection".
