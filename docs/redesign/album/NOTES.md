# Album redesign: Gatefold

**Direction.** The album page is a gatefold: the sleeve is no longer a rounded panel floating in the page but a flat column of the cover's colour cut by the pane's top, left and bottom edges, and the listing is the facing page on the plain ground. The cover, title, artist, catalogue line, badges, actions, disc index and fine print all live on the sleeve; the listing holds only the songs (disc and work headings) and the notes. Shelves span the full width under both.

## Composition by region
- **Wide (>=860px of content).** Grid of a sleeve column (cover + 48px + two gutters: 368/432px) and the listing. The pane's scroll has no padding any more; the sleeve column stretches the grid's height, which is at least the pane's, so it reaches the bottom edge. Inside, the cover leads, then the head (title, artist, catalogue line, release lead when not out yet, actions, badges and note), a hairline, the disc index and the fine print. The contents are sticky at the pane's top when they fit the pane. The page gutter is the sleeve's inner margin, so sleeve text and the shelves share a left edge.
- **Narrow.** A full-bleed band across the top, no radius, no margin, cover (168) beside the head; listing, fine print and notes on the ground below.
- **Not out yet.** Same gatefold; the release lead stays the biggest line after the title, now on the sleeve.
- **Skeleton.** Same shape: a placeholder sleeve column (wide) or band (narrow), rows on the ground.

## States covered
Loading skeleton, error and signed out (padded block on the ground), released, library-only (backdrop wash), Not out yet (countdown, year only, done), 940px.

## Tests
- Goldens regenerated: album-light/dark, album-940x600-light, album-upcoming-light/dark/940x600/year-light/year-dark/done-light, library-album-not-out-light/dark. Added album-wide-light/dark (>=860px content had no golden).
- `TestArtistLinkTakesTheCoversAccent` now measures against the sleeve and expects `Sleeve.AccentInk` (the byline is on the sleeve in every layout).
- `TestSleeveStopsAtTheGridsEnd` uses a 900px pane: the sleeve now holds the head too and is taller than 620px.
- `TestCatalogueSections` and `TestSectionTilesOpenPagesAndPlay` scroll to the shelves: they begin under the gatefold, one pane down.

## Constraints
- `playOnSleeve` is now always true for `palette.ForAlbum`, since the Play disc sits on the sleeve in both layouts; `colours` no longer depends on the layout.
- No kit change: the full-bleed comes from removing the scroll's own padding in this package.

## New strings
None.
