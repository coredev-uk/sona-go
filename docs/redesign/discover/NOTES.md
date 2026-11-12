# Discover and Charts: "The issue"

## Direction

Discover is set as a magazine issue. A masthead (the page title large, the freshness line as the issue line on its baseline) opens it; the front page is one large lead in Apple's hero shape at the full column width with a caption row under it (title, line, and the rotation pager at the row's end) and a strip of the other featured cards beneath; a row of band buttons is the contents; and each band's name moves into a left margin column beside its shelves, so the long page reads as a ledger of sections instead of a stack of same-looking headings. Charts shares the masthead and ranks its songs with large numerals, because the order is the information there.

## Composition change by region

- **Masthead** (`page.go` `masthead`): `kit.PageHeader` replaced on Discover, Charts, a chart in full, a room, and the curator/multiroom/grouping pages without a cover. 32/36 title, 13 muted line on its baseline, controls at the row's end.
- **Front page** (`front.go`, `layout.go`): the lead was two thirds of the width with a vertical list beside it; it is now the column's width (to 1080) with the caption under it. The pager moved from the list's foot to the caption's end (and the card's more button sits beside it). The list became a strip of four cards (one row; two rows of two under 880px of content), the lead's card on `--active`. Rotation, holds, pause, announcements, play and menu behaviour are unchanged.
- **Contents** (`newpage.go`): one wrapping row of buttons, one per band with its shelf count, replacing a line per band with a button per shelf.
- **Bands** (`newpage.go`): from a 1100px page, the band heading (24/30) stands in a 200px margin column; narrower, above the shelves. One 1px rule per band, no numbering.
- **Charts** (`rank.go`, `charts.go`, `room.go`): Top songs on the overview and a songs chart in full are ranked rows with a 24/650 faint tabular numeral leading each row.

## States

Loading skeletons follow the new layout (lead, caption bars, strip cards, contents buttons, first band with its margin; the charts skeleton has ranked placeholder rows). Error, empty, stale line and signed-out states are unchanged in content and sit under the new masthead.

## Tests

Changed because they pinned the old layout: lead art size (784×450 → 1080×619), pages of 6 → 4 ("1 of 3" → "1 of 4"), `layoutFor`'s table, the row-finding helper `rowCentre`, the thumbnail test (the poster is on the strip's second page), the Tab walk (lead, its play button, the pager, then the strip), the contents tests (one button per band with "1 shelf"/"2 shelves" in its description; jumping to a band, not a shelf). Deleted: the contents' per-shelf list assertions (the per-shelf buttons are gone by design). Added: the ranked rows (rank numerals, one tab stop, arrows, Enter plays from the row) and a narrow golden.

## Constraints hit

- `kit.TrackList` has no hook for its number column, so chart songs use a local `rankList` built from `kit.GridNav`, `kit.ActionMenus`, `kit.Artwork`, `kit.EqualiserBars`. It does not select several songs, drag, or sort as TrackList does, and is built whole (charts are a few hundred rows at most).
- Non-song charts (albums, playlists, videos) are kit shelves and grids; rank cannot be drawn on them without kit changes.
- `kit.ItemShelf` draws its own title, so Charts keeps stacked shelf headings; the margin-column composition is on Discover's bands only.
- The lead keeps the hero shape, so at the widest column its height is 620: on a short window the strip is below the fold. Capped at 1080 wide.

## New strings

`{0, plural, one {# shelf} other {# shelves}}` (the band buttons' description). Reused: Discover, Charts, On this page, Featured, the pager and rotation strings, band names.
