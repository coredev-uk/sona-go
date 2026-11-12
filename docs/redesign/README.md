# Redesign pass: a different take on every page

Every page package under `internal/ui/features/*` gets a fresh composition in this branch. The house world (`docs/design.md`) stays: one grotesque at its six sizes, the faintly green-grey board, the cover as the only colour, amber only for what plays, no gradients, glass or live blur, the performance rules of `AGENTS.md`, the keyboard and assistive-technology contracts of `docs/accessibility.md`. What changes is how each page is set: its layout, hierarchy, rhythm and the shape of its parts. The old layout is evidence, not a template.

Each page has its own folder here with `before-*.png` and `after-*.png` captures (light and dark) and a `NOTES.md` that names the direction and what moved.

## How the captures are made

`bin/_shots` (throwaway tooling, not committed) starts a probe build of Sona per page and theme inside `headless` (private XDG dirs, `dbus-run-session`), on the synthetic fixture profile (signed out, 24 invented albums), with `SONA_PREVIEW=1 SONA_PRIVATE=1` and the fake engine (`SONA_E2E=1`, so "playing" plays nothing). Every HTTP request goes to a local server that answers the fixture's covers and passes the rest to Apple's public catalogue, so catalogue pages (Discover, Radio, an album, an artist) show real public data and no account data. Surfaces with no live trigger (the Widevine notice, the update notice, release reminders) are rendered through `ui.Tester` in the same `headless` session.

## Direction per page

| Page | Direction |
| --- | --- |
| Home | **Side A.** Top Picks as a numbered running order beside the lead; Recently Played as a strip of covers; ruled section heads. |
| Discover | **The issue.** A masthead, the lead at full width with the featured list as a strip under it, contents as a row of jumps, bands with their heading in the margin; Charts with big ranks. |
| Search | **Card catalogue.** A rail of result groups with counts, a best match set large, artists as a row of portraits. |
| Concerts | **Gig poster.** Ticket-stub dates, months counted, a strip of the coming days that jumps to a date. |
| Radio | **Programme guide.** Live stations as a timeline board with a now line, programme blocks and the time left. |
| Library | **The crate.** Letter rails, dated groups for Recently added, a timeline for History, a stronger artist detail. |
| Album | **Gatefold.** The sleeve as a full-bleed column in the cover's colour, the listing as the facing page. |
| Artist | **Press kit.** A reworked header and the discography as a career line by year. |
| Song | **Liner card.** Facts as a ledger beside the cover, credits grouped by role. |
| Music video | **Cinema.** The picture leads at full width; the title and actions sit under it as a caption row. |
| Playlist | **Mixtape J-card.** A ledger header and a running "starts at" column in the track list. |
| Player | **Transport deck.** Seek along the bar's edge, a numbered queue with now, next and later, a recomposed Now Playing. |
| Mini player | **Cassette.** Cover edge to edge, progress as a hairline on the foot edge. |
| Equaliser | **Graphic EQ.** Presets as a row of chips, the response curve drawn behind the faders on one scale. |
| Settings | **Marginalia.** Section icons in the index, group titles in a margin column beside their rows. |
| Account | **Two-panel sign-in.** What signing in adds beside the form. |
| Commands | **Launcher.** A larger palette with a key-hint foot; a shortcut sheet with keycaps in one column. |
| First-run setup | **Step rail.** The steps as a vertical rail beside the step. |
| Private listening | **Off the record.** A dashed chip while private. |
| Widevine | **Inline progress.** A small meter beside the download percentage. |
| Updates | **Release slip.** A compact card in the sidebar foot with its progress. |
| Reminders | **Dated reminder.** The menu entry names the day and shows the bell's state. |
