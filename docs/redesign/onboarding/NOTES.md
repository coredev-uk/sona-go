# First-run setup: "Step rail"

## Direction

The old card was one column with a segmented strip under its head: where you were read as a progress bar, and the card was a form with a title. The redesign makes the setup a small two-pane window. The steps are a rail down the card's leading side (the contents of the setup, always in view), the step is beside it, and the head and the foot span both. The rail turns the strip's idea ("filled up to the current one") into a list you read: a number, or a check once a step is behind, and a name; the current step is heavier with an ink bar. The welcome becomes a first impression: the Sona mark at 56px with its grooves, the display-size title and one sentence.

## Composition by region

- **Card**: 800px wide (was 640), `--raised`, float shadow, same fixed top edge (10% of the window, at least 24px) and same growth/scroll rules. Under 720px of card width (a window under 768px) or when the rail would not fit between head and foot (a body shorter than 296px) it is the old 640px card with the strip.
- **Head** (wide): one 56px row, mark, "Set up Sona", "n of 7" (tabular figures), Skip setup. The strip no longer lives here at wide widths.
- **Rail**: 192px, `--surface` (a tone step against the card, no line), 36px rows 2px apart. Number (tabular, ink-faint; ink and semibold on the current step) or a check (muted ink, not green: a step passed may have been skipped), then the name at 14px. Current: ink, weight 600, 3px ink bar at the row's leading edge. Hover: `--hover` fill and ink name. One Tab stop, ↑/↓ and ←/→ move, Home/End, Enter goes (a focus group of both orientations). The rail gives the card a minimum height (head 56 + rail 296 + foot 64), so the short steps (Welcome, Done) are not a stub.
- **Step pane**: the step's own scroll, 28px padding, title and line as before. The foot is unchanged (64px, Back / primary).
- **Welcome**: large mark (amber disc, two 1px grooves at 20% of `--on-amber`, spindle hole) beside "Welcome to Sona" at the 32/36 display size and "Sona plays Apple Music on this computer."; then a sentence on what the setup covers and that each step can be skipped; then the language row (when there is one) and "What whole songs need" as before.
- **Appearance**: the theme cards stay the one picture; the picture is now top-aligned in each card (it was vertically centred, which only showed once a card's hint wrapped: "Follows your desktop" wraps at this width).

## States covered

All of them kept: Widevine loading/downloading/ready/failed/unsupported, signed in/out, the sign-in form with its foot actions, the Done step's "Still to do" and "Ready to play", the unavailable-platform lines, the narrow strip card, and the short window (the strip card when the rail does not fit).

## Tests

- Changed: `onboarding_test.go` clicked at the card's right edge using the old 640px `panelWidth`; it now uses `cardWidth` (`rig.cardRight`, `rig.panePad`). No assertion weakened.
- Added: `TestStepRailIsMovedByTheVerticalArrows` (↓/↑/wrap/Enter on the rail), `TestNarrowWindowKeepsTheStepsAsAStrip` (700px: strip, ←/→/End/Enter and the same names).
- The existing strip keyboard test and the a11y list test (named list "Setup steps", one item per step holding one button, the current one `aria-current=step`) now run against the rail (the rig is 900px wide) and pass unchanged.
- Goldens regenerated: `welcome-light/dark`, `appearance-light/dark` (940×700, wide card).

## Constraints hit

- A kit-less rail: built in `progress.go` from `ui.ButtonBase` and a focus group; no kit change.
- "Done" steps are those before the current one (the setup does not track completion per step), hence a neutral check.

## New strings

- `Sona plays Apple Music on this computer.` (the welcome's line)
- `A few short steps sign you in and set how it plays, looks and starts. Each can be skipped and changed later in Settings.` (the old welcome sentence, split in two)

The old msgid `Sona plays Apple Music on this computer. A few short steps sign you in and set how it plays, looks and starts. Each can be skipped and changed later in Settings.` is gone; the catalogues are regenerated centrally.
