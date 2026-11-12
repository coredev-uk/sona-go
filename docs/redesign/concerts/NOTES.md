# Concerts redesign: "Gig poster"

## Direction

A gig is a date, a place and a ticket, so the page is a sheet of ticket stubs. Every day is a stub: its weekday over a 32/700 day number in a fixed column, a 1px rule the height of the day, and that day's shows beside it, so the rule's length says how busy the day is. Over the agenda sits a strip of the coming fortnight: weekday over day number, a day with shows marked with one to three ink dots and a button that jumps to it, the others dim and inert. Rows are recomposed as a timetable (start, portrait, line-up, venue and distance, Get tickets) instead of a list of stacked text blocks. No amber anywhere.

## What moved, by region

- **Header.** Title at 32/700 (was 24/650) with the location button beside it (was far right); the "Near … · Updated" line under both. Built locally (`kit.PageHeader` could not take the button beside the title).
- **Filters.** Unchanged controls and behaviour, bottom margin 16.
- **Coming days (new, `agenda.go`).** 14 cells from today (or from the first day of a later range, up to its last day), `Grow(1)` up to 72 wide, a Tab stop of its own (`FocusGroup(Horizontal)`) with tooltips. A day with shows is a button that sets `rov.want` to its first control (through `roving.firstStop`) and a one-frame `ScrollIntoView(ScrollStart)` on the day block, so the day lands at the top and its first control is focused. Hidden when no day of it has a show, or the range is under 7 days (a weekend).
- **Months.** 24/650 heading + `· 2 shows` (13 muted). The last month's count is dropped while Apple has more than the list asked for (its count could be short). 48px between months.
- **Days.** Stub column 64 (weekday 12 muted over 32/700 tabular), 1px `--line` rule, rows column. Was 56px date column with 24/650 number and no rule.
- **Rows.** Start (14/500, left edge of every row) → portrait 56 → line-up 16/600 over venue (13 muted) with distance (12 faint) beside it, "Your artist" on a third line → outlined Get tickets at the right edge. Was portrait first, three stacked lines, ghost ticket button.
- **Shelves (Home, artist page).** Same stub idea in compact form: date column + 1px rule first; start/distance stay under the venue because the columns are too narrow for the timetable. Ghost ticket button kept there. Separate shelf skeleton.
- **Location dialog.** City rows: name 14/500 left, country 13 muted at the right edge, check in a slot kept on every row (countries share one edge). Behaviour and keys unchanged.

## Cost

The groups, strip, per-show words (clock, distance, venue line) and per-day words are made once per listing in `viewOf` (key: filters, fetched-at, length, first/last ID, strip start/end, locale) and kept in `page.view`; the agenda only reads them. `shown` is no longer copied each frame unless "Your artists" filters it.

## States covered

Loading (month heading + three stubs, rules and rows at real height; the strip waits for the data), nothing near the city, filters match nothing (Clear filters), error with Try again, stale copy line, "Show more" busy, list kept while a filter loads (busy region), date-range filter (strip follows the range), narrow (708) and dark.

## Tests

- Changed: `page_test` (the row's start and distance are two texts now, month counts), `location_test` (country is its own text, no ", " prefix), `golden_test` (today moved to 9 April 2027 and a fourth show added so the strip has marks; a narrow golden added; `onDay` helper).
- Added: `TestComingDaysMarksTheDaysWithShowsAndKnowsWhereTheyAre` (model), `TestComingDaysStripJumpsToADay` (scroll + focus), `TestComingDaysStripIsOneTabStopAndAbsentWithoutShows`, `TestMonthHeadingCountsItsShowsExceptThePartialLast`.
- Goldens regenerated: `concerts-light`, `concerts-dark`, `concerts-location-light`; new: `concerts-narrow-light`.
- Nothing deleted; every keyboard, a11y and behaviour assertion is unchanged.

## Constraints hit

- `kit.PageHeader` fixes the action at the far end; the header is local.
- No kit dashed or perforated vertical rule: the stub rule is a solid 1px `--line` box.
- The skeleton pulses (`kit.Skeleton`), so loading cannot be a golden (it never settles).
- Distances and dates come from the existing DTOs; the strip is derived from the loaded shows only, so with "Show more" pending a later day could gain shows.

## New strings

- `Coming days` (the strip's group name).
- `{count, plural, one {# show} other {# shows}}` (month count and the strip's day names).
