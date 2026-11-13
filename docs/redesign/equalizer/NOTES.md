# Equaliser: Graphic EQ

## Direction

The old page was a settings screen: a tall list of sixteen presets on the left, a small panel on the right with a thin graph floating above a separate row of faders. The new page is the instrument itself. The presets become a row of chips over the top, and the bands panel takes the full column: ten tall faders with the response curve drawn behind them on one dB scale, so the curve visibly runs through the thumbs and what you move is what you see change. The preamp stands apart on the left behind a rule, like a master fader; the dB scale sits at the right edge of the rules.

## Composition by region

- **Header**: unchanged in content (title, the chosen curve as the catalogue line, On/Off switch), now inside a column capped at 1120 so the switch lines up with the panel's right edge at any width.
- **Presets**: a wrapping row of 32px chips (radius 6, `surface` fill, hover tone on hover) in place of the 176px list. "Your curve" is set 8px apart from the built-in curves. The chosen chip is ink-filled while the equaliser is on; off, it keeps the `active` tone and gets a 1px ink outline, so the fill always means "this shapes the sound" and selection is never colour alone. Still one `RadioGroup` (one Tab stop, arrows move and select in reading order, wrapping).
- **Bands panel**: full width, `surface`, radius 8, padding 24. Left to right: preamp fader (64 wide), a 1px rule with 20px either side, ten band faders, the dB scale. Each column is value above, a 224px fader (was 160), name below. The curve and three rules (+12, 0, −12) are drawn by one `Draw` callback in a box that is exactly one fader tall and as wide as the ten columns; the rules are the faders' travel ends (inset 6, half the thumb) and the faders lie over the box as an absolute row, so nothing under the pointer is taken by the drawing. The dB labels are at the rules' heights right of the bands, the "Hz" unit at the names' line. The level-change note stays at the panel's foot.
- **Fader fill** is `ink-muted` (the cap stays ink) so the curve, not the fills, carries the line.

## States

- **Loaded, on / off**: off cross-fades the curve to its faint tone and draws the faders at 55% as before, the chosen chip changes as above.
- **Loading**: header with its subline's place kept (`PageHeaderCounted`), 16 chip-sized bars wrapping like the chips and a panel of the real panel's height with the three rules and the preamp rule.
- **Error**: header, then the existing `kit.ErrorBlock` with Try again, under the same header (no jump).
- **Unavailable**: header with "Not available on this platform yet", and a panel of the bands panel's size with the sliders glyph, one sentence and a pointer to Settings → Playback.

## Performance

No new animation or frame loop. The curve's gains→points computation is cached as before; the two `ui.Path`s are now kept in the cache and refilled with `Reset` instead of allocated per draw. The rules are four `Line` calls.

## Tests

- No behavioural or a11y test changed: the tab walk (switch, chosen preset, preamp, bands), arrow navigation with wrapping, previews/saves, retry, unavailable and error states all pass as they were.
- `newRig` gained `newRigAt` (a window width), used by the new narrow golden.
- Goldens regenerated: `equalizer-light`, `equalizer-dark`, `equalizer-off-light`. Added: `equalizer-narrow-light` (707 wide, three chip rows, faders share the rest) and `equalizer-unavailable-dark`.

## Constraints

- `OriginSlider` fixes the thumb at 12 and the track's inset at 6, so the graph box and the labels use that constant (`thumbInset`); if the kit changes it, they must follow.
- The curve is a real filter response, not a line through the thumbs: at the shelves and between bands it deviates from them. That is the point of drawing it.
- The preamp's travel is 0 to −12 dB, not the bands' ±12, so it has no share in the rules.

## New strings

- `Hz` (the unit under the faders' names)
- `Playback here has no equaliser, so there is nothing to adjust.`
- `The other sound settings are in Settings, under Playback.`
