# Proposal: add-ons, and the work left after #277

Status: draft for review. Nothing here is built yet.

Since #277, `main` has the redesign of every page, themes (#276) and shuffle from the sidebar. This document plans what is still open:

- **Add-ons** (plugins): a way to extend Sona from outside its own code.
- **Fonts in themes:** left out of #276 on purpose.
- **Debts from #277:** gaps the redesign worked around rather than fixed.

Each numbered item below is meant to land as its own branch and PR, in the order given. A PR that delivers an item ticks it here.

## Decisions already taken

- **Audiences.** Add-ons serve three:
  - you and power users, for automation and small integrations;
  - third-party developers, who publish add-ons;
  - Sona's own Last.fm, ListenBrainz and Discord integrations, which later move onto the same API.
- **Runtime.** Out-of-process first, over the transport the `sona` CLI already uses: D-Bus on Linux, a Unix socket in the state dir on macOS and Windows. An in-process WASM host (`wazero`) comes later and carries the same messages, so an add-on can move between the two.
- **Name.** "Add-ons". "Plugin", "module", "provider", "service", "integrations" and "extension" all mean something else in the code already.

## What an add-on may and may not do

- **May:**
  - receive events: playback state, now playing, queue and library changes, as public DTOs;
  - send commands: the CLI's set, each behind a permission;
  - contribute palette commands, item actions on songs, albums, artists and playlists, and a settings section of plain rows. Sona draws all of these, and choosing one sends an invocation to the add-on.
- **Never:**
  - see a token, an Apple URL, audio or video frames, or the database;
  - touch the keyring directly: Sona stores an add-on's secrets under `addon.<id>`;
  - receive an event for a private listen or a preview (`Private.Since`, `Preview`);
  - run in the frame loop;
  - take a shortcut or action ID outside `addon.<id>.*`.

## Add-ons

### 1. Protocol v2 and `docs/addons.md`
Extend `internal/cli`:
- **`hello`:** add-on ID, protocol version (semver, negotiated) and the permissions it asks for;
- **event subscription:** a generalised `status --follow` with a filter;
- **`invoke`:** calls for the add-on's contributions, with deadlines;
- **errors:** codes, not text.

`docs/addons.md` documents every message, with JSON schemas for the messages and the manifest, and covers the security rules above.

- **Acceptance:**
  - a test client written in another language (a shell script over `socat`, or Python) completes `hello`, subscribes, receives a now-playing event, and is refused a command it has no permission for;
  - a private listen produces no event;
  - the schema validates every example in the doc.

### 2. Manifests, permissions and the supervisor
- **Manifest:** `<config>/sona/addons/<id>/addon.json` holds id, name, version, the command to run (an executable inside the folder), permissions, contributions and strings per locale.
- **`internal/services/addons.go`, with `addons_dto.go` and one `registry.go` line**, which:
  - lists and validates manifests;
  - starts enabled add-ons after first paint (`OnNetwork`);
  - restarts with backoff, and disables an add-on after repeated crashes with a notice;
  - stops add-ons on quit;
  - enforces invocation timeouts;
  - keeps a log per add-on beside `sona.log`;
  - grants permissions on first enable and keeps them in settings.
- **Platforms:** a platform that can't run an add-on reports it through `PlatformService.Capabilities`.
- **Acceptance:**
  - service tests on temp dirs: listing, refused manifests, crash and backoff, the timeout, quit;
  - an add-on that hangs never blocks the UI;
  - the race tests pass.

### 3. UI contributions, Settings → Add-ons, `sona addons dev`
- **UI module `features/addons`:**
  - registers manifest contributions as `kit.ActionDef`s, palette entries and settings rows;
  - add-on strings come from the manifest (English fallback), outside the generated `.po` catalogues.
- **Settings → Add-ons:** each add-on with its permissions, an on/off switch, its log and its problems, plus "Open add-ons folder".
- **`sona addons dev <dir>`:** runs an add-on from a folder without installing it and streams its log.
- **Acceptance:**
  - `ui.Tester` tests for the page and its a11y tree;
  - an item action from a test add-on shows in the album menu, invokes the add-on and shows its answer as a toast;
  - goldens for Settings → Add-ons, light and dark.

### 4. Sample add-ons
These live in a separate repo, and `docs/addons.md` links to them:
- a "now playing" webhook;
- a Home Assistant bridge;
- a "copy song.link" item action;
- one sample in Python or shell.

- **Acceptance:** each runs against a release build under `sona addons dev`.

### 5. Sona's own integrations on the API
Move Last.fm, ListenBrainz and Discord presence onto the add-on API once it covers:
- private-mode-aware listen events (`scrobble.Provider`'s `NowPlaying` and `Submit`);
- secrets per add-on in the keyring;
- persistent retry of listens.

They ship bundled, enabled by default, and signed by Sona.

- **Acceptance:**
  - the scrobble queue tests still pass;
  - Settings → Connections still drives them;
  - no behaviour visible to the user changes.

### 6. WASM host
- Same protocol through `wazero` host functions; the manifest's `runtime: "wasm"` picks it.
- Capability-based: no ambient file or network access.
- The new dependency needs maintainer review and an update to the Flatpak `go-sources.json`.
- **Acceptance:** sample 4's item action, built for WASM, behaves exactly like its out-of-process build.

### 7. Sandboxing out-of-process add-ons
Reuse the CDM host's sandbox code (`docs/security-review.md`) per OS, and extend the threat model in `docs/security-review.md`.

### Open questions (add-ons)
- **Pages.** Can an add-on add a page, not only actions, commands and settings rows? That needs a declarative page schema. **Proposal:** not before item 6.
- **Distribution and trust.** How do third-party add-ons arrive, and is unsigned code allowed? Options:
  - folder copy only;
  - `sona addons install <url>` with a checksum;
  - a curated index.
- **Network.** Is an out-of-process add-on's network its own business, or does Sona broker it so the user sees the hosts it reaches? Brokering is only enforceable under the WASM host.

## Themes

### 8. Fonts in themes (decide, then build or close)
A theme could name a font file inside the themes folder. This needs:
- `FontFamily` as a token;
- `theme.RegisterFonts` at load, not only at start;
- validation of the file;
- a fallback to Schibsted Grotesk.

It is the costliest theme change, because type metrics feed layout: every golden would need a check under a second font. **Decide:** build it, or close it as not planned.

## Debts from #277

### 9. `kit.TrackList`: a leading rank column and an extra column
- **Charts:** Discover's charts wanted big rank numerals. They kept the small `#` column because `kit.TrackList` has no hook for it.
- **Playlist:** the "Starts at" time is the row's second line, not a column beside the duration.
- **Change:** add `TrackListOpts.Rank` (a leading numeral style) and a single extra column slot (`TrackColumns.Extra`, a title plus a func of the row), then use both.
- **Acceptance:** goldens for charts and playlist, light and dark; sorting, multi-select and drag keep working.

### 10. `kit.Artwork` rounds its boxes
`kit.Artwork` computes `h = size / Aspect` without rounding, so a caller passing a raw ratio gets a fractional box and the picture lands off whole pixels. Discover and Home round by hand today.
- **Change:** round inside the kit (and in `FluidArtwork`), then remove the callers' workarounds.
- **Acceptance:** an artwork test with a 1060:608 aspect gets a whole-pixel box, and existing goldens don't change.

### 11. Load-sensitive tests
`TestCollectionsGolden/grid` (kit) and the theme gallery's arrow-key test failed once in a full `-race` run while the machine was busy. Both pass alone and in quiet runs. The gallery test now waits for the view.
- **Change:** find what the grid golden depends on timing for (artwork decode or hover fade) and make it wait on state rather than time.
- **Acceptance:** `devenv shell -- tests` passes 5 runs in a row while another `go test -race ./...` runs in parallel.

### 12. Station page cover
On the station page, Apple Music 1's large artwork shows thin light bars at its sides in dark mode.
- **Change:** find out whether that comes from the source art or from the box Sona requests. Fix it if Sona's request is the cause; otherwise record it as Apple's art.

## Order

1. → 2. → 3. → 4. (the add-on v1)
2. 9, 10 and 11 can go in parallel with the add-ons at any time.
3. 8 needs a decision first.
4. 5 → 6 → 7 after the add-on v1 is in use.
