# Sona: agent instructions

Sona is a Go desktop Apple Music client with a native UI (MyGo). Go module `github.com/coredev-uk/sona-go`. These rules are for every agent and harness; they apply to people too.

## Workflow

The main checkout is read-only: it stays on `main` and you never edit, build or run in it. Every change gets its own worktree and branch. In order:

1. From the main checkout's root, create a worktree beside it: `git worktree add ../sona-go-<slug> -b <type>/<slug> origin/main` (`<type>` is the Conventional Commit type, e.g. `feat/queue-autoplay`).
2. `cd` into it, then run `pwd` and `git branch --show-current`: the path must be `…/sona-go-<slug>` and the branch must not be `main`.
3. Use absolute paths into the worktree for every file tool (read, edit, write). File tools resolve relative paths against the session's starting directory, which is often the main checkout.
4. Implement, with the docs and tests that describe the change in the same PR. A new worktree has no `.env`; symlink it from the main checkout only when a live token is needed.
5. Run `devenv shell -- check` and `devenv shell -- tests`. After rebasing onto `main`, also run `gen` and `go run ./cmd/i18ntool extract`, and commit the results.
6. Push and open the PR with a Conventional Commit title and the template (`.github/pull_request_template.md`): What, Why, Evidence (commands with their output; light and dark screenshots for UI, no personal account data), Tests, Docs, Left.
7. When CI is green and the work is done, comment `@bot-clarence merge`.
8. After the merge, remove the worktree: `git worktree remove ../sona-go-<slug>`.

## Boundaries

- Always: one change per branch and PR; run `check` and `tests` before a PR; update docs with the code; use `SONA_PRIVATE=1` for anything that plays music; read Clarence's card on your PR.
- Ask first: restarting WirePlumber, merging anything Clarence ruled `verdict/review`, `SONA_ALLOW_MAIN=1`, bumping a pinned dependency (MyGo, Go, nixpkgs), changing a DRM or security rule.
- Never: commit or push to `main` (`.githooks/` refuse both); merge with `gh pr merge` or the button; hand-edit generated files (`internal/db/dbq`, the English `.po` catalogues, `CHANGELOG.md`); print, log or commit `.env`, tokens or passwords; renumber or edit a released migration; bundle, patch or work around the CDM (Hard rules).

## Commands

Run everything as `devenv shell -- <cmd>` (devenv 2.4+; it sets `CGO_ENABLED`, `GOFLAGS=-tags=sqlite_fts5`, GStreamer paths, `.env`). Without it, `-tags` replaces `GOFLAGS`: name `sqlite_fts5` too.

- `check`: vet (also `dev` and `probe` tags), Windows and macOS type-checks, golangci-lint on new code, `mygo vet internal/ui`, i18n extract check, path case. `tests`: `go test -race` in `dbus-run-session`, `ui.Tester` and golden images included.
- `fmt` (gofumpt), `gen` (sqlc into `internal/db/dbq`), `go run ./cmd/i18ntool extract` (English `.po` catalogues; a test fails when stale).
- One test: `devenv shell -- go test -race ./internal/<pkg> -run '^TestName$'`.
- The rest (`dev`, `build`, `headless`, `e2e`, `probe`, `release`, spikes): `docs/development.md`, "devenv scripts". DRM regression after CDM, playback or Apple changes: `go run ./cmd/playspike -seek 90`.

## Repo map and where to look

- `cmd/sona` entry; `internal/services` Go services (contracts in `internal/services/doc.go`); `internal/ui` MyGo UI; `internal/db` SQLite, goose migrations (dense from `00001`, applied in order), sqlc. `VERSION` is the version source.
- `docs/architecture.md`: scope, process model, playback and DRM, layout (§6), data, performance, platforms, testing: the why behind every rule here.
- `docs/ui-architecture.md` UI modules; `docs/design.md` visual direction; `docs/accessibility.md` keyboard and assistive-technology contracts; `site/src/content/docs/reference/theme-format.md` theme files; `site/src/content/docs/reference/cli.md` the `sona` CLI; `site/` the public docs.
- `docs/auth.md` sign-in; `docs/apple-music-reference.md` Apple endpoints (functionality only); `docs/security-review.md` threat model; `docs/development.md` setup and scripts; `docs/testing.md` checks and tests; `docs/platforms.md`; `docs/translations.md` strings; `docs/maintainers/ci-workflows.md` CI; `CONTRIBUTING.md` and `site/src/content/docs/contributing/` the contributor pages.
- A new page is a service (`internal/services/<feature>.go`, `<feature>_dto.go`, one `registry.go` line) plus a UI package (`internal/ui/features/<name>`, one `internal/ui/app/modules.go` line).
- Design every page fresh with `skill://impeccable` and `skill://design-taste-frontend`; never copy Apple's layouts.
- Work comes from the project board (https://github.com/users/coredev-uk/projects/3) and its issues. Labels: `area/*` and `platform/*` say where; `decision` needs the maintainer's call, `needs-hardware` a real machine, `upstream` MyGo or Apple; `needs-triage` is a new report. `state/*`, `verdict/*`, `kind/*`, `type/*` and `clarence:*` are the bots'; leave them.
- Reference only: [vibez](https://github.com/simonepelosi/vibez) (MIT) hosts the CDM the same way; never copy it wholesale.

## Hard rules

Rationale: `docs/architecture.md`.

- Go owns all Apple HTTP traffic, tokens, sync, caching, MPRIS and artwork. The UI never holds tokens and never calls `*.apple.com`. Requests go through `applemusic.Client` with the web player developer token and `Origin: https://music.apple.com`.
- Sign-in: native in-app flow (`internal/auth/appleid`) first; loopback redirect (`internal/auth/mut`) and token paste as fallbacks. Never MusicKit popups or webview header rewriting; never store or log passwords or 2FA codes.
- DRM: protected playback runs only in the CDM host child (`internal/playback/cdmhost`). Decrypted audio never leaves it, never touches disk, never crosses the pipe; only CDM-decoded video pictures may leave, in memory, for display.
- The CDM host child makes no network requests. Never make Widevine work in the UI process.
- The CDM is never bundled, redistributed or patched; only Sona's own downloader installs it (Google's servers, Mozilla's pinned manifest, signatures verified). Never use browser, system or user-supplied copies, extracted keys or reimplemented CDMs (`openwv`).
- Secrets: never in database tables, logs or committed files; use the system keyring with the explicit 0600 file fallback.
- Go's MPRIS (`org.mpris.MediaPlayer2.sona`) is the only Sona player on D-Bus.
- Write a file Sona replaces in place with `fsutil.WriteFileAtomic`, never hand-rolled temp-and-rename.
- Performance: MyGo draws on the GPU, so no layout, text shaping or image work per frame; narrow invalidation; `Scroll` virtualises lists.
- Decode artwork at its shown size, off the main thread, cached. Pre-blur backdrops in Go, never live. Cold start reads SQLite only; network and playback host start after first paint.
- Platforms: OS-specific code goes behind the seams (`platform_<os>.go`, `services.Platform`, `docs/platforms.md`). Where a platform lacks a feature, report it (`errors.ErrUnsupported`, `PlatformService.Capabilities`) and hide or explain the control; never a stub that pretends.

## Process

- Private mode: every test, smoke or live check that plays runs with `SONA_PRIVATE=1`; never play against Apple without it. `SONA_PREVIEW=1` or `playspike -preview` play 30-second previews with no account.
- Live audio goes to your own PipeWire null sink, never the default sink or source; destroy only nodes you created. Recipe: `docs/development.md`, "Live playback and the session bus".
- Only one Sona may own the real session bus. Everything else runs under `dbus-run-session` with private XDG dirs, never the real `~/.config/sona` or `~/.local/share/sona`. Concurrent dev builds also set `SONA_DEV_INSTANCE=<name>`.
- Commits and PR titles are Conventional Commits (`feat`, `fix`, `perf`, `refactor`, `docs`, `test`, `ci`, `build`, `chore`, `revert`; scope optional); CI fails other PR titles and squash merges use the title.

## Clarence (merge bot)

- Clarence (`bot-clarence`) merges PRs. It posts one card (verdict, checks, timeline); read it before asking anyone to look.
- Merge by commenting `@bot-clarence merge`; never `gh pr merge`.
- `verdict/review` needs the maintainer (@coredev-uk); don't approve or merge on his behalf.
- Never remove `state/unhealthy` (it halts merging) or push to Renovate's (`bot-cecil`) branches.
- Commands, labels and reactions: `docs/maintainers/clarence.md`.
