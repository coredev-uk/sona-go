# Discover and Charts: "The issue"

## Direction

Discover is set as a magazine issue. A masthead (the page title large, the freshness line as the issue line on its baseline) opens it; the front page is one large lead in Apple's hero shape at the full column width with a caption row under it (title, line, and the rotation pager at the row's end) and a strip of the other featured cards beneath; a row of band buttons is the contents; and each band's name moves into a left margin column beside its shelves, so the long page reads as a ledger of sections instead of a stack of same-looking headings. Charts shares the masthead.

## Composition change by region

- **Masthead** (`page.go` `masthead`): `kit.PageHeader` replaced on Discover, Charts, a chart in full, a room, and the curator/multiroom/grouping pages without a cover. 32/36 title, 13 muted line on its baseline, controls at the row's end.
- **Front page** (`front.go`, `layout.go`): the lead was two thirds of the width with a vertical list beside it; it is now the column's whole width with the caption under it (hero shape up to 540 tall, wider from there, to the banner's 4:1). The pager moved from the list's foot to the caption's end (and the card's more button sits beside it). The list became a strip of four cards (one row; two rows of two under 880px of content), the lead's card on `--active`. Rotation, holds, pause, announcements, play and menu behaviour are unchanged.
- **Contents** (`newpage.go`): side-by-side columns, wrapping, one per band: the band's button with its shelf count over its shelf buttons in 13 muted (every titled shelf stays one jump away), replacing a line per band.
- **Bands** (`newpage.go`): from a 1100px page, the band heading (24/30) stands in a 200px margin column; narrower, above the shelves. One 1px rule per band, no numbering.
- **Charts**: masthead only. A ranked-numeral row component was tried and dropped: `kit.TrackList` (kept) has no hook for its number column and the replacement lost multi-selection and drag. Its `#` column is the chart rank.

## States

Loading skeletons follow the new layout (lead, caption bars, strip cards, contents columns, first band with its margin; the charts skeleton has the Top songs rows). Error, empty, stale line and signed-out states are unchanged in content and sit under the new masthead.

## Tests

Changed because they pinned the old layout: lead art size (784×450 → 1080×619), pages of 6 → 4 ("1 of 3" → "1 of 4"), `layoutFor`'s table, the row-finding helper `rowCentre`, the thumbnail test (the poster is on the strip's second page), the Tab walk (lead, its play button, the pager, then the strip), the contents tests (a band's button now carries its shelf count in its description, and the per-shelf jumps are kept). Added: a narrow golden and an Enter-plays-from-the-row test for Top songs.

## Constraints hit

- `kit.TrackList` has no hook for its number column, so Charts' ranks are its small `#` column, not large numerals; `kit.ItemShelf` draws its own title, so Charts keeps stacked shelf headings and the margin-column composition is on Discover's bands only.
- The lead spans the column at every width. It keeps the hero shape up to 540 DIP tall; wider than that the box widens towards the banner's own 4:1 (Apple's centre crop), so a 1920 window shows it edge to edge without the strip falling below the fold. The strip's wide thumbnails are whole DIPs (64×37).

## New strings

`{0, plural, one {# shelf} other {# shelves}}` (the band buttons' description). Reused: Discover, Charts, On this page, Featured, the pager and rotation strings, band names.
