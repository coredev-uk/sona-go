# Album redesign: Band

**Direction.** The album page opens on a band, not a column. The cover is the hero of a full-bleed band in its own colour, the head beside it, and the band dissolves into the ground under it, so the colour belongs to the top of the page and never has an edge to scroll into. The listing is on the plain ground: the songs (disc and work headings) and the notes, with the disc index and the fine print in a margin beside them. Shelves span the full width under both. Replaces the gatefold (a full-height sleeve column cut by the pane's edges), whose coloured box ended where the songs did and whose contents scrolled away on any record longer than the pane.

## Composition by region
- **Wide (>=860px of content).** Band: the cover (232, 288 from 1240, 352 from 1600) with its shadow, the head beside it, the whole block on the cover's bottom edge (title 32/36, 40/44, 48/52 with the cover; artist, catalogue line, badges, note, actions; a head taller than the cover starts at the top), then a 112/120/128px dissolve into the ground. Under it a grid of songs, 48px of room and a 264px margin (disc index, a hairline, the fine print), one left edge; the margin is held 16px under the pane's top while the songs scroll and released when they end, when it fits the pane. The far end of the band takes up to 30% of the colour its text is not, so a wide band is not a flat slab.
- **Short pane (<560px high).** 168px cover, 20px above it, 56px dissolve, compact head (Apple's note left to the notes, the actions closer): Play and three songs on the first screen of a 1280×600 window.
- **Narrow.** The same band: 168px cover beside the head (actions under both when the text is under 400px wide), a 72px dissolve; songs, fine print and notes below. No margin, no index.
- **Not out yet.** The same band; the release lead stays the biggest line after the title; the head is taller than the cover, so it starts at the top.
- **Skeleton.** The same shape: a placeholder band with its dissolve, rows on the ground.
- **Reduce effects.** The band is one flat colour and ends square; no dissolve, no deepening.

## Decisions
- **No backdrop image on the band.** A prototype laid Go's pre-blurred backdrop under a veil of the cover colour at the band's empty end; at 1280 to 1920 wide the 16 to 32px picture's bilinear steps showed as vertical streaks, and no veil that shows the image keeps text 4.5:1 over any picture. The band is flat colour plus two gradients of one colour each, only ever towards the text's opposite or the ground at the foot.
- **Dissolve** is a smootherstep sampled in 24 stops of the ground colour's opacity over the band's last stretch, never a blur; nothing sits in it. A test reads the page's left edge for any step over 8 of 255.
- **Cover sizes are fixed** (232/288/352/168), so Go serves exact boxes and a resize never asks for a new picture per frame.
- **The margin is sticky, not static**, in one scroll container with `Element.Sticky`; it is first in the tree so Tab meets the disc index before the songs (same order as on the sleeve), and a margin taller than the pane scrolls with the page.
- `palette.Sleeve` keeps its name (the theme package's type); the album package calls its surface the band.

## States covered
Loading skeleton, error and signed out (padded block on the ground), released, library-only (backdrop wash band), Not out yet (countdown, year only, done), short pane, 940px, beside the queue (580px), reduce effects, extreme covers in both themes (tests).

## Tests
- Goldens regenerated: album-light/dark, album-wide-light/dark, album-940x600-light, album-upcoming-*, library-album-not-out-*, album-queue-580-light, album-upcoming-queue-580-light. Added album-wide-short-light, album-wide-scrolled-light, album-wide-reduced-light.
- Band tests: the dissolve ends in the ground with no edge (wide, dark, short, narrow), reduce effects ends it square, the far end never costs the text its contrast (nine covers, both themes), a short pane shows Play and three songs, the margin holds and is released, the disc index and the fine print share a left edge.
- The sleeve's geometry tests (stays beside the songs, stops at the grid's end) are replaced by the margin's; the sleeve-pixel probes read the band's corner.

## New strings
None.
