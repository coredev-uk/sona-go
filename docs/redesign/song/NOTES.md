# Song page redesign: the liner card

## Direction

The song page was the album header at 200px with two shelves under it. It is now a liner card: who the song is on the left (cover, title, artists, album, actions) and what is known about it on the right as a ledger (facts, credits, lyrics), so the page reads like the fine print of a record rather than a header over a list.

## Composition by region

- **Wide (content 860px and up):** a spine in the cover's wash bleeds off the top and left of the column: 240px cover (exact size), title 32/36, artists, album link, badges, actions. Beside it a body column (max 760px) holds Details, Credits and Lyrics. Videos and More by the artist stay full-width shelves below.
- **Narrow (707px of content, the 940px window):** the wash is a full-width band (136px cover beside title, artists, album, badges, actions); the body stacks under it.
- **Details:** new two-column name and value ledger at 13/18 (Released in full, Duration, Genre, Track, Disc from the second disc, Written by, ISRC). Replaces the three-line footer at the foot of the page and the meta line in the header.
- **Credits:** each group under a 1px rule with the group name as an h3; people in a 2 or 3 column grid of 40px avatars (by available room, min 210px a person), roles wrap.
- **Lyrics:** moved after the credits, same excerpt, toggle and states.

## States

Loading is the same frame (`frame`) with flat bars, at the same row pitch (golden `song-loading-light`). Error, not found and signed-out are unchanged (`kit.DetailError` at the top of the column). A song with neither credits nor lyrics says so under Details.

## Tests

- `TestLyricsAndCreditsSitSideBySideFrom1100` replaced by `TestLiningUpWideAndNarrow` (the breakpoint moved from the window's 1100 to 860 of content, and the order is Details, Credits, Lyrics).
- `TestCreditsHaveTwoColumnsFrom720` replaced by `TestCreditsGridHasMorePeopleToARowWhenThereIsRoom` (two to a row at 707, three at 1132).
- `TestSongPageShowsTheHeader` lost the meta line and the three footer lines (now ledger rows, asserted by the new `TestLedgerNamesAndValues`); `TestNeitherLyricsNorCreditsSaysSo` checks ledger rows instead of footer lines. The fixture song gained `TrackNumber: 1`.
- New golden `song-loading-light`. Regenerated: `song-light`, `song-dark`, `song-narrow-light`.

## Constraints

- `CatalogSong` carries no label, ℗ line or album track count, so the ledger has no Label or "Track 1 of 12". Nothing else of the Song DTO is unused.
- The album is a link only on the left (not repeated in the ledger), so each link has one tab stop.
- The spine is as tall as its content, not the body's (no sticky: the pane's scroll element is the page).

## New strings

Details, Released, Duration, Genre, Track, Disc, Written by, ISRC. Now unused: "Released {releaseDate}", "Composed by {composer}", "ISRC {isrc}" (the catalogue is regenerated centrally).
