# Settings redesign: Marginalia

**Direction.** Settings reads as a document with marginalia. The section opens on a 1px ink rule under its name; each group's heading and its one muted line hang in a 176px left margin beside the group's rows, and the rows keep title/description left, control right, one hairline each. The index gains a 16px icon per section.

## Composition by region
- **Index**: icon + name + state line per section (active fill kept); the wrapped narrow list keeps the icons.
- **Section head**: h2 (24/650) over a full-width ink rule; before this, a bare h2.
- **Groups** (wider than 960): h3 and group line in the left margin, aligned to the first row's title; below 960 they stack above the rows as before. Untitled blocks (Language, Widevine, Appearance rows) sit in the rows column with an empty margin so the left edge is shared. Group spacing 40 -> 32.
- **Storage**: figures are now 24/650 numbers in a group "On this computer"; the rows are the group "Limit and cleanup".
- **Diagnostics**: verdict is the first margin note, the three actions the first row; check groups use the margin frame.
- **Shortcuts**: unchanged two-column ledger; its h3s stay above rows (the columns need the full width).
- Loading skeletons use the same margin frame (`marginal`), so they match the final layout.

## States covered
Loading, error (kit.ErrorBlock), empty Shortcuts search, unavailable rows, narrow window, light and dark.

## Tests
No behavioural or accessibility assertion changed. Goldens regenerated: settings-general/appearance light+dark; added settings-playback-light, settings-general-narrow-dark (707 px), settings-storage-light, settings-diagnostics-light.

## Constraints
- The theme has no sliders-horizontal, palette, plug, hard-drive or stethoscope icons (theme is out of bounds): the index uses sliders-vertical, play, moon, link-2, keyboard, folder-closed, badge-check.
- Find's group names are unchanged (the new Storage group titles are visual only).

## New strings
"On this computer", "Limit and cleanup".
