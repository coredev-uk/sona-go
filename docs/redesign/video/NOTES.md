# Music video page and video view: "Cinema"

## Direction

The page is a screening, not a collection header. The 16:9 picture leads at the full content width (no wash, no side-by-side cover: the cover is the colour), and everything about the video is a caption under it: the title block at the left, the actions at the right, like a title card under a still. The view follows the same idea in reverse: the picture owns the stage, the top strip shrinks to the two window buttons, and the title leaves the top for a caption at the lower left of the foot, on the ground and never over the picture, with the transport centred beside it and the seek bar across the full width above.

## Composition by region

### Page (`/music-video/{id}`)

- **Before:** 356 DIP art beside a 32/36 title, byline, meta, badges and a wrapped actions row, on a Backdrop wash bleeding to the edges; shelves of 240+ tiles.
- **Lead picture:** `kit.Artwork` 16:9 at the content width, rounded down to a whole DIP and requested at exactly that box. Capped at 1120 wide and 360 to 630 high; the height cap follows the window's height (window less top bar, player bar, top padding and a 112 DIP caption), so the title is always above the fold, even at the 940x600 minimum. Left-aligned to the gutter; nothing sits to its right.
- **Caption row** at the picture's width: left, title (32/36, two lines at most, the page's h1), artist links (16/500), the catalogue line with the Explicit/4K/HDR tags on the same line (they wrap under it when narrow); right, the amber Play disc, Watch in Apple Music, Play the song, the item menu. Under 720 DIP of picture width the actions stack under the title block (the real minimum: a 940 window less the 232 sidebar).
- **Then** the note on standard definition with the song line (secondary type, 544 max), Notes, "More by {artist}" and "More like this" as rows of 16:9 tiles from 296 DIPs (two or three across at the usual widths), and the release date.
- **Loading** draws the picture box, the three caption bars and one row of tile boxes at the real sizes (a labelled "Loading" status), replacing `kit.DetailSkeleton`. Error, not-found and signed-out states are `kit.DetailError` as before, at the top of the page.

### View (Layer)

- **Top strip:** 48 DIP (or the window's title bar if taller), Full screen and Close at the right only. It still keeps clear of the window buttons and drags the window.
- **Stage:** unchanged: ground, picture letterboxed on black at its own shape, poster before the first picture, the dashed problem block in its place. The strip and foot together take about 160 DIPs, where the old 64 DIP title strip and the foot took about 176, so the picture is a little larger.
- **Foot:** the seek bar across the full width with the elapsed time flush to the left gutter and the time left flush to the right gutter (they were inset and mis-aligned with everything else), then one row in three cells: the caption (title as the h1 link with the SD badge; the artist link and the status line under it), the transport (Previous, 44 DIP amber disc, Next) in the exact middle, and the volume at the right. The caption and volume cells are equal-basis growers, so the transport stays centred whatever the title length.
- **Kept exactly:** fading after 3 s (strips and foot share one opacity), focus keeps controls visible, Space/K/arrows/F/M/Escape/V, announcements (status mounted with its first text, alert for errors), SD badge hidden in an error state, problem block and its actions.

## States covered

Page: loading skeleton in the final layout, loaded, no Notes / no song / no URL / no release date / no genre / no shelves, signed out and failed (kit), platform without video (note says Apple Music, no Play disc or menu), capabilities not yet known (no note, no Play). View: paused, playing, loading, buffering, problem (not supported / failed with or without Try again), faded.

## Tests changed

- `a11y_view_test.go` (`TestViewKeyboardWalkAndTree`): the expected Tab order is now the reading order of the new layout (Full screen, Close video, Seek, the title, the artist, then the transport). The old order had the title first because it was in the top strip; every control is still reached, with the same labels.
- `video_test.go` (`TestSpaceOnAFocusedButtonIsTheButtons`): the same order. The link half of the test now reaches the title's link with four Tabs; the button half starts on Full screen, the first stop. The assertion (Space on a link or button is not play/pause) is unchanged.
- `page_test.go` (`TestTilesOpenTheirPageAndPlayWhereVideosPlay`): the window is 1500 high, because the second shelf is now further down under the lead picture and a click needs it on screen.
- `golden_test.go`: page goldens render at 1132x1500 so both shelves show; added `video-page-707-light` (the stacked caption, the real narrowest page) and `video-view-940-light` (the view at the window minimum).

None deleted.

## Constraints

- No new service data: everything is from `CatalogVideo`.
- `kit.CollectionHeader` has a fixed art size and no 16:9 hero mode, so the page builds its own caption row from kit pieces (`Artwork`, `ArtistLinks`, `Badges`, `PlayButtons`, `Button`, `ActionMenus.Trigger`, `Shelf`, `CoverShelf`).
- The picture box tracks the scroll width (`pageW`, previous frame, as the gutters already did) and invalidates when it changes, so a resize asks for each new exact size; the loader cancels what is no longer asked for.

## New strings

None.
