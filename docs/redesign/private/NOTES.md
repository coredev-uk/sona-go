# Private listening pill: "Off the record"

**Direction.** Private listening is the one switch that changes what Apple records, so on it should look like a margin note, not a selected button. Off keeps the muted glyph alone. On becomes a chip with a 1 DIP dashed `control-line` outline (the mark of something unrecorded), the hat-glasses glyph and "Private" in ink, no fill and never amber.

**Composition.** The selected-control `active` fill is gone; the dashed outline (MyGo's `BorderDashed`, drawn by the element, nothing local) replaces it. Hover still tints `accent-soft`. The word still shows from 1100 DIP and the development-build note from 1280; below 1100 the dashed glyph chip alone carries the state.

**States.** Off, on, on with word, forced by `SONA_PRIVATE=1` (focusable, a11y-disabled, tooltip says why), development build, unknown (dimmed 40%). All unchanged in behaviour.

**Tests.** None changed. Added `golden_test.go` (`pill-on-light`, `pill-on-dark`, 1200×48: the pill needs a 1100+ DIP window to show its word).

**Constraints.** None. No new strings.
