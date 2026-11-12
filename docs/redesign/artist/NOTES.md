# Artist page redesign: "Press kit"

## Direction

The header becomes the act's press sheet: the name leads at the left with its catalogue line, two-line bio, Read more and one row of actions; the round portrait moves to the trailing edge; the latest release is a prominent second column (a 152px cover with the amber Play disc, its title and lines) from 1040px of content. The back catalogue stops being a stack of shelves and becomes a career line: releases merged across the albums, singles/EPs, live and compilation shelves, grouped by year, newest first, the year in a left gutter linked by a rail. Bands share one heading form (heading plus a hairline running to the edge), the essential albums become a panel beside the numbered top songs, and About becomes a spec column plus the text.

## Composition by region

- Header: before, portrait leading, text, slip as a 300px right column with a 112px cover. Now text leading, slip column (1040+) or a hairline-ruled row below (under 1040), portrait trailing (200 / 176 / 112). Play disc on the cover (beside) or at the row's end (below).
- Lead: top songs as before (the track list already numbers rows by Apple's popularity order); essential albums in a 340px `surface` panel from 800px (was 900, 300px, no fill).
- Discography: was an Albums cover row plus three side-by-side lists. Now one timeline of the newest 18 loaded releases, kind in each line; ghost links for each shelf whose view is not shown whole (See all paths kept, routes unchanged); music videos stay a tile row.
- Related: unchanged content, new band heading.
- About: facts (200px) lead, biography follows; narrow, facts in a wrapping row above.
- Loading frame matches the new header, panel and widths.
- Narrow (707px, the 940 window): portrait 112 at the trailing edge, slip row under a hairline with a 64px cover, essentials panel under the songs in two columns, year gutter 52px, one release column.

## States

Loading skeleton (final layout), error with Retry, not found, signed out, upcoming release (badge in the slip and the kind line), restricted explicit releases, small act with no biography or slip, library artist: all unchanged in behaviour, built in the new composition. A shelf's rows keep selection, drag, menus and keyboard.

## Tests

- Changed: layout test (`layoutFor` now also reports `slipBeside`, thresholds 800/1040/720), the slip placement test, the page-order test (timeline kinds instead of per-shelf titles and `2016 · Single` lines), See all count (one; the singles view is now a link), Tab walk order (timeline order), About facts test (facts column now ahead of the biography), tile-link test (timeline rows).
- Deleted: `TestCatalogueListsShareARowWhenTheyFitAndStackWhenTheyDoNot` (the catalogue lists no longer exist on the page).
- Added: timeline grouping/cap/dedup unit tests, career-line placement test, latest-release Play test.
- Goldens: `artist_light`, `artist_dark` (now 1132 wide: slip beside), `artist_900_light` (new, middle tier), `artist_940x600_light` (now the narrow composition, taller).

## Constraints

- The page data is per shelf and capped by Apple's page size, so the timeline is built from what is loaded and the views stay reachable by link; years older than the loaded releases are not shown.
- The kit TrackList draws the rank numerals; they are not restyled.
- Rows in different years are separate release lists (separate arrow-key groups).

## New strings

`Compilation`, `Undated`.
