# Radio and Station: the programme guide

## Direction

Radio is the one page whose subject is time, so it is drawn as a broadcaster's listings: a time ruler, a lane per station, programmes as boxes set by their real start and end, and one ink line down every lane at now. Where the old page was a grid of identical 96px cards, each with its own progress bar and a "Next at" sentence, the guide shows what is on, what just ended and what follows in one picture, and every station's "now" lines up. The same idea hangs the rest of the page on a rail: headings at the left as wide as the guide's station column, content starting where the guide's track starts. The station page drops the wash and the hero for an identity column beside a vertical timeline of the schedule.

## What moved, by region

- **Live now** (was: a grid of 420px cards, art 80, name, programme, bar, "Next at"): a ruler (flag at now with the clock time, labels and ticks every half hour, hour hairlines behind the lanes) over lanes of 72px. Station column: art 56, name, play control or equaliser. Track: programme boxes placed by time, the one on air `--active` with its time left and a bar to the now line, named shows `--hover`, the station's own hours and ended programmes outlined. A box that began before the window is cut at the left edge with "‹ start"; one that goes on past the right shows a chevron. The window is 30 minutes back and 90 to 300 minutes long by width (about 3.4 DIPs a minute), so 120 at 940 and 240 at 1440. The words of the programme on air go right of the now line (left of it when the show is nearly over).
- **Unplayable / no schedule**: the lane is one outlined box with the reason (`CircleSlash`), the tagline, or "No schedule" when there is neither; stations with no page keep the whole lane as their play button and also show the play control (the context menu is on the row).
- **Local broadcasters and the bands**: ruled heads with the heading in the rail (sticky), "See all" under the broadcasters' heading, the content beside it. Below 880px of content the heading stands over the content.
- **Station page**: identity column (cover 264, 320 from 1500px of content; name 32/36, byline, catalogue line, notes, Play disc and menu; sticky) beside the lists, which take the rest of the width up to 1100px and sit under ruled heads (at 1920 they filled 720 of 1600 and left half the page empty; from 1500px of content the two columns, 320 + 48 + 1100, always fit and stay at the page's left edge). Below 800px it stacks, with a 112px cover beside the lines. Programme art is requested at exactly 48 and the guide's lane art at 56. The schedule is a vertical timeline: start time, a rail with a dot per start, and boxes in the guide's look. The rail of the programme on air is inked to how far through it is and is its progress indicator.
- Back-to-back slots with one title are joined on both pages (a station's own hours were five identical rows).

## States

Loading: the ruler's heading and six lanes with art and programme bars at their real height; the broadcasters and first band as rule, rail bar and shelves; the station page as identity bars plus three timeline rows. Error: in place beside the rail heading, with Retry; whole-page failure still says it once. Empty: "No live stations" in the rail layout. Signed out: the Stations band's block, unchanged. Station Sona can't play: the guide's lane and the station page's dashed note. Tuned: amber name, equaliser, bar (guide) and rail dot (timeline), nothing else.

## Tests

- Changed: `onair_test.go` no longer pins "Next at / Later at" sentences or the two-column grid (the guide has neither): the checks are now that the next show is a box with its slot, own hours join into one box and count down to the next show, the board never ticks between reads, and the narrow board holds a shorter stretch. The pixel test reads the bar 12 DIPs in, past the box's rounded corner. `station_test.go` checks start times and "45 min left" instead of the old "10:00–11:00" slot string.
- Deleted: `TestOnAirRowArtIsTheProgrammesElseTheStations` and `artOf`: lanes show the station's own art (a stable column of logos), not the programme's.
- Added: `timeline_test.go` (joining, window length and start, ticks, clipping, label placement, the cells of a lane, lanes that say something instead), a programme box is a link named for its slot and not a Tab stop, a box opens the station's page, the flag shows the clock time, the rail heading stays in view while scrolling. Goldens: `radio-narrow` and `station-narrow` added (light); the dark Radio golden has a station tuned (paused, so the equaliser holds still).
- Unchanged: every keyboard, a11y, playback and state assertion.

## Constraints hit

- `kit.StationList` (the local broadcasters) and the shelves are the kit's: only their heading and place could change, so the heading moved into the rail and "See all" is rebuilt locally; the rows keep their 64px look.
- The services give a schedule per station (`StationDetail.Schedule`), no programme art beyond the slot's own; the guide uses the station's art and keeps programme art for the station page.
- A programme inside a link is not on the accessibility tree, so the progress indicator sits beside its box, not inside.
- The next show past the window is not named on the board (the old "Next at" sentence is gone); it is a box when it falls inside the window, else on the station's page.

## New strings

`{programme}, {slot}` (a programme box's link name), `No schedule`. Reused: Live now, Time through this programme, `{left} left`, On air, Schedule, See all, Sona can't play this station. No longer used: `Next at <0>{nextAt}</0>: {nextTitle}`, `Later at <0>{nextAt}</0>: {nextTitle}` (the catalogues are regenerated centrally).
