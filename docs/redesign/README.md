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
| Reminders | **Dated reminder.** The menu entry names the day ("Remind me on 22 Nov"). |

## Result

Every page package has its redesign commit on `feat/ui-redesign-pass`. No page was reverted.

| Page | Commit | Captures |
| --- | --- | --- |
| Home | 1add20a6 | live, light and dark |
| Discover, Charts | 533cfda6, 63f7b124 | live |
| Search | 82534e02 | live |
| Concerts | 94213ed2 | live |
| Radio, Station | bf1c3c79 | live |
| Library (8 pages, sync footer) | 88b1d1bb | live, every page |
| Album | dcf5e35f | live, catalogue and library |
| Artist | 83dbb820 | live |
| Song | a45f38bb | live |
| Music video | 3cf165ee | live (page) |
| Playlist | 56665205 | live |
| Player (bar, queue, Now Playing) | 7e9b279a | live, playing on the fake engine |
| Mini player | b7b29a77 | live, its own window |
| Equaliser | f79aa7c4 | live |
| Settings | 6273d4aa | live, four sections |
| Account | d9fd163d | live |
| Commands | f9145ccf, dd6eee8c | live, palette and sheet |
| First-run setup | 316863b4 | live, welcome and appearance |
| Private listening | 78999843 | live bar and `ui.Tester` chip |
| Widevine notice | a7a84799 | `ui.Tester` |
| Update notice | 44e373f7 | `ui.Tester` |
| Reminders | d42b9c12 | `ui.Tester` (the album menu) |

- **Live captures:** the Widevine notice, the update notice and the reminder entry have no live trigger in a signed-out preview run, so their captures come from `ui.Tester` in the same `headless` session. The before captures were drawn by the same tests on b259a43.
- **Video view:** it needs the Widevine CDM to play, so it is shown by the package's goldens (`internal/ui/features/video/testdata/golden/video-view-*`), not by a live capture.
- **Integration commits:**
  - 39944597 regenerates the string catalogues and the shell's goldens.
  - 40c63855 gives full-window goldens more time to settle under load.
  - a7028a13 merges origin/main, whose four refactors moved past the branch's base. Every redesigned page now uses upstream's shared builders (`theme.Text`, `theme.StyleText`, `theme.Fade`, `kit.Fade`, `ctx.Current`). The after captures are from that commit.
- **Scroll performance:** the Albums scroll stays within its budget (`probe.md`).
