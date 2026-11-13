# Mini player redesign: "Cassette"

**Direction.** The window is a cassette: the cover is the whole leading side of the window, full height and flush to the edges, and everything else is a short text block beside it. The old version was a 108 DIP cover inset 12 DIP in a padded row with a seek slider and a time in the control row; the new one has no inset anywhere on the cover side and moves progress out of the controls to the window's foot, where it is a 2 DIP hairline.

## Composition by region (420x132)

- **Cover**: 132x132 at x=0, y=0 (was 108, inset 12). Radius is a hair above 0 so the cover meets the window's corners (the window rounds itself). Right-click still opens the playing item's menu.
- **Text column** (12 DIP from top, right and bottom; 14 from the cover): title now 16/22 600 (was 14/20 600), artist, then the third line (sung line, album, notice, reason), all unchanged in logic.
- **Window buttons** (Open Sona, Close): trailing end of the title line; before the title where the platform puts Close first (macOS inset, leading desktop layouts), Close outermost either way.
- **Foot row**: the time left (meta, `--ink-muted`, tabular; or the Live marker for a station) at the start, Previous / Play / Next at the trailing side (was: transport at the start, seek slider and time to the right).
- **Progress**: one slider along the window's whole foot, over the cover too: 2 DIP amber fill on a 12% ink track, hit area 12 DIP high, 4 DIP with a 10 DIP ink thumb under the pointer, while dragging and while keyboard-focused (focus ring around the thumb). Same label ("Seek"), value text, 1 s arrow steps, commit-once-on-release as the kit slider. A live station has no hairline (it has no position).

## States

Idle ("Not playing" / "Choose music in the main window", disabled transport, `–:––`, disabled slider), playing, loading (spinner play button), live station (marker in place of the time, no hairline), notices and reasons (third line, unchanged), error and sign-in lines (unchanged).

## Tests

- Added `golden_test.go`: one golden test, light and dark, `testdata/golden/miniplayer-light.png` and `miniplayer-dark.png` (generated cover via a local `coverSource`, a sung line, the seek hairline part-way).
- No existing test changed or deleted: all behavioural/a11y tests (seek by pointer and key, Close placement, Tab order) pass over the new layout.

## Constraints

- `kit.Slider` is a 16 DIP track with a permanent thumb and cannot be a hairline, so the seek line is a local `ui.SliderBase` with a custom draw (`kit` is off limits).
- `kit.ArtOpts.Radius` of 0 means the default 4, so the flush cover uses `Radius: 0.01`.
- On macOS (inset title bar) the native window buttons draw over the cover's top-left corner, as they did over the old inset cover; not exercised here.
- The hairline's amber over a very pale or amber cover can be hard to see on the cover's part; the text side's track stays readable.

## New strings

None.
