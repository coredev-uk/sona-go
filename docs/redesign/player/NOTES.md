# Player: the bar, the queue, Now Playing, full-screen lyrics

Package `internal/ui/features/player`. Branch `feat/rd-player`.

## Direction: the transport deck

The player stops being a strip of controls with a seek bar squeezed under them and becomes a deck. The seek bar is the bar's top edge, across the whole window, with the transport at the centre of the bar and its times either side. The queue is a numbered running order in three named groups. Now Playing is a record held open on a fold: the sleeve (cover and label copy, one block centred on the column's height) on one side of a hairline, the insert on the other, with a header, a body and a foot that each have their own rule. Full-screen lyrics keep the sheet exactly as it was and only gain a footer rule.

## What moved, by region

### The bar (72px)

- **Seek bar**: from a 4px slider between two time labels inside the middle column to a 3px track along the bar's whole top edge (5px under the pointer or the keyboard), amber fill up to an always-shown 10px ink thumb, a 12px-tall hit area edge to edge (`seekStrip`, `drawSeek`). The 1px top line of the bar is gone; the track is the edge. A live station has no seek bar, so it gets the 1px `line` there.
- **Times**: elapsed (`--ink-muted`) to the left of the transport, length (`--ink-faint`) to the right of it, tabular figures; none for a live station.
- **Transport**: one row, shuffle / previous / the play disc / next / repeat, centred on the bar (it was a 560px column); the amber disc is 44px (was 36) with a solid play and pause glyph.
- **Song block**: unchanged except that a live station's marker and what is on air move into the line under its name (they were under the transport).
- **Right group**: unchanged controls, now out of the flow at the right edge over a spacer of its measured width (see Constraints).

### The queue panel (Aside, 320px)

- Header (heading, Clear, AutoPlay toggle, Close) unchanged.
- The old summary line and flat list become three groups, each a heading and a list of its own: **Now playing** (one 72px row on a `--hover` fill at radius 8, 56px cover, the title at 14/600 in `--amber-ink`, artist, album, equaliser), **Next** (52px rows numbered from 1 in tabular `--ink-faint` figures where the index would be; the drag grip takes the number's place on hover or focus; the heading's detail line is the summary, "3 songs after this one", "Playing from Chill"), then AutoPlay's or the station's part under its own heading, numbered on.
- Where AutoPlay or a station follows the playing song directly, its own heading is the group heading and there is no Next.
- The queue code's line model changed (`queueLines`): headings are lines with their own heights, every line has an offset (`line.y`), and the virtualised window is found by binary search on offsets instead of dividing by 52. Drag and drop, roving focus, keyboard, menus and announcements are unchanged in behaviour.

### Now Playing (Overlay)

- **Sleeve**: cover and label copy are one block, centred vertically in a column the cover's width plus 48px at each side; the copy is left-aligned to the cover's edge. Cover sizes (400/320/240/168) and their rules are unchanged.
- **Fold**: a 1px `--line` from the top of the view to its foot divides sleeve and insert (wide layout only).
- **Insert**: the tabs and Close are one 47px header row over a 1px rule that starts and ends at the text edge, the shown tab's 2px ink rule resting on it. Each pane's foot (lyric tools, Up next's summary and tools) is a 52px strip under a rule of the same length. Body text edge: 24px from the fold in the wide layout (the tabs, both rules, the lyric lines, the queue's numbers, the foot's line), with the dashed empty and the error block on it too.
- **Up next** is the queue panel's list (same groups). Beside the page the Next heading carries the summary; here the pane's foot already says it, so the heading has no detail. The playing row's fill is ink at 5% on the wash.
- **Narrow** (<880): unchanged shape (song as a header, cover 168 beside the copy and Close), with the tabs as the insert header, with its rule.

### Full-screen lyrics (Layer)

The sheet, its colours, timing, fades and keyboard are untouched. The footer strip sits under a rule spanning the side room (7% of the width), the cover is 48px and the title 14/600.

## States covered

Playing, paused, idle ("Not playing", disabled strip and disabled disc), live station (marker and on-air line under the name; seek bar replaced by the line), notices in the song block (sign in, no CDM, error) unchanged; queue: empty (unchanged dashed block), only the playing song ("Nothing after this song" under Next), user songs, AutoPlay-only, user songs then AutoPlay, AutoPlay on its way (the note), a station with and without its own heading; Now Playing: lyrics, loading skeleton, empty, error with Retry, Up next, station or video (Up next only), wide, narrow, light, dark.

## Tests changed

- `queue_logic_test.go`: `TestQueueLines` expects the Now and Next lines; `TestQueueGeometry` rewritten for per-line offsets, `lineAt`, `groupOf`, and the drop slot over the new headings (it pinned `queueRowH` arithmetic, which no longer holds).
- `a11y_test.go`: the playing song and what follows are now separate lists with level-3 headings (Now playing, Next), so `list "Queue"` became `list "Next"` and the places a row reads changed (the playing song is 1 of 1; Song 2 is 1 of 3 in Next). The assertions that matter are unchanged: every child of a list is a list item, the current item is marked, the rows' buttons are in the tree and are not Tab stops, the order of Tab stops, the keyboard help.
- `queue_panel_test.go`: the equaliser check reads the tree (the group heading is now also called "Now playing").
- New: `TestSeekBarRunsAlongTheBarsTopEdge` (the bar's seek slider is the bar's top edge, edge to edge, the ends reachable; a live station has none and its marker is under its name) and `TestTheQueueIsARunningOrderInGroups`.
- Nothing deleted. No behavioural, keyboard or accessibility assertion was weakened.

## Goldens regenerated

`nowplaying-light.png`, `nowplaying-dark.png`, `player-queue-light.png`, `player-queue-dark.png`. `internal/ui/app` goldens are the integrator's.

## Constraints hit

- **Tab order**: MyGo orders out-of-flow (`Absolute`) elements after the flow ones inside one parent, and Tab follows that order. The Seek slider must be out of the flow (it overlays the bar's top edge) yet come after Repeat and before the sleep timer (the order the existing test pins and v0.2.0 had). So the right group is out of the flow too (placed with `Right(16)`, over a spacer `Box` of its measured width) and both are built in that order: Repeat, Seek, then the right group.
- **Hit testing is clipped to the parent's box**: an out-of-flow slider can only live in a parent that spans the window's width, which is why it is a child of the bar's row, not of the transport.
- The lyric sheet's top and bottom fades use the wash colour at the sheet's edges, so the insert stays on the wash; no filled panel for the insert.
- No new data was needed from the services.

## New strings

- `Next`, with the context "the queue's heading over the songs that play after the one playing" (the queue's group heading and the name of its list; the bar's Next button keeps its own, context-free msgid).

Reused: `Now playing` (the equaliser's label, now also the group heading), `Queue`, `AutoPlay`, `Live`, `Seek`, and the rest unchanged.
