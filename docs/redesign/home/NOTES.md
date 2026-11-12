# Home redesign: Side A

**Direction.** Home reads like the A-side of a record. The lead pick is 01 with a large cover; Apple's other Top Picks follow as a numbered running order (02 to 09, Apple's own order, so the figures carry information) in one hairline-ruled list beside it. Everything under the lead band is a ruled section: heading, a 1px line running to the edge, and its "See all" / "Show all" on the line. The page's rhythm is rule, content, 48px of air. The old look (two loose columns of picks, a 232px lead, two-deep rows of Recently Played, an accordion with 44px thumbnails) was the anti-reference.

## Composition by region

- **Header**: title, freshness and Refresh are one line of the band (was a stacked title/subline block with the action far below).
- **Lead band** (backdrop wash kept, full bleed): ruled head "Top Picks for You" with the Show all toggle on the line; left column, a 280px cover with "01 Name / artist / catalogue line / play disc and menu" hanging under it; right, one single-column running order (rank, art, name and Apple's reason in two columns, hairline per row, eight rows, "Show all 12" for the rest). Narrow (<900px): the lead becomes a row (144px cover, then 01 and the name), the order runs full width under it.
- **Recently Played**: a strip of covers in one row (112px to about 130, name and line under), still one roving Tab stop with Space plays / Enter opens / menus (the `itemRows` machinery with a `Tile` mode, not `kit.CoverRow`, which has a Tab stop per tile).
- **Shelves**: ruled heads replace `kit.Shelf` (local `rule.go`); the cover shelves are `kit.CoverRow` under that head (the logic of `kit.ItemShelf`, rebuilt locally); station shelves stay as two-deep rows, now aligned to the page edge.
- **Hero**: text and play disc left, banner (45%) right, centred.
- **More for you**: a ledger of 52px lines, a column for "3 albums · 1 playlist", 32px thumbnails at the far end, hairline between lines; disclosure behaviour unchanged.
- **States**: signed out and offline are a ledger band (rules above and below, headline and sentence left, Sign in at the far end) instead of the dashed block; loading draws the new layout (head, 01 cover with name lines, eight ranked rows).

## Tests

No existing test was changed or deleted: the a11y, rows, lead, index and local tests all pass on the new composition. Added: `TestStripTilesAreOneTabStopWithTheRowsKeys`, `TestRankedRowsNumberFromTheFirstRank`, `TestLeadIsOneOfTheRunningOrderAndTheOthersFollowByRank`. Goldens regenerated: `home-light`, `home-dark`, `home-light-940`.

## Constraints hit

- `kit.Shelf` has no head hook, so every section Home owns uses a local ruled head. Sections built by kit (`kit.EditorialSections` for a hero shelf of several items) and the concerts shelf another module hands over keep the kit head.
- `kit.ItemShelf` pre-decodes a row of covers ahead through the unexported `itemAheadTile`; the local copy does not, so opening "Show all" decodes then.
- The running order shows Apple's reason as `Item.Subtitle`; there is no per-row duration or artist field in the DTO to add a ledger column.
- Rank figures are plain numerals (not translated).

## New strings

None. Existing msgids are reused ("For you", "Top picks", "Show all {count}", "Show fewer", "See all", "More for you", "Recommendations need Apple Music" and the rest).
