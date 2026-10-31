# Contributing to Sona

Read this, then `AGENTS.md` (the binding rules) and the doc for the area you
touch.

## Environment

Everything runs through [devenv](https://devenv.sh), which pins Go 1.26,
Node 24 + pnpm, GTK4/WebKitGTK 6/GStreamer and a development Widevine CDM
(Google Chrome from nixpkgs, never redistributed):

```sh
nix run nixpkgs#devenv -- shell                     # interactive shell
nix run nixpkgs#devenv -- shell -- <command>        # one command
```

It sets `CGO_ENABLED=1`, `GOFLAGS=-tags=sqlite_fts5`, the GStreamer plugin
path and `SONA_WIDEVINE_CDM`. Shell shortcuts: `build`, `dev`, `gen`, `fmt`,
`check`, `test`, `package`, `play-spike`, `auth-spike`.

- `go tool wails3 build` builds the frontend, regenerates the TypeScript
  bindings and writes `bin/sona` (production; `DEV=true` for a debug build).
  `go tool wails3 dev` runs with Vite hot reload.
- `go tool wails3 generate bindings -clean=true -ts` regenerates
  `frontend/bindings` after changing an exported service method or DTO.
  Commit the result.
- `sqlc generate -f internal/db/sqlc.yaml` after changing queries; migrations
  live in `internal/db/migrations` (goose).
- `go build .` needs `frontend/dist` (build the frontend first).
- The version is the `VERSION` file; the build injects it with
  `-ldflags -X main.version` (`bin/sona --version`).

Local secrets: `.env` (ignored) may hold `MUSIC_USER_TOKEN` for
`cmd/playspike` and live tests. Never print, log or commit it.

## Checks

Run before sending a change (CI runs the same on every push and pull request,
`.github/workflows/ci.yml`):

```sh
go vet ./...
go test -race ./...
golangci-lint run --new-from-rev=origin/main ./...   # must be clean
cd frontend
node scripts/check-path-case.mjs   # no paths or module names differing only by case
pnpm exec tsc --noEmit
pnpm exec eslint src
pnpm exec prettier --check src     # --write to fix
pnpm i18n:check
pnpm exec vitest run
```

Real-CDM tests (`internal/playback/widevine`, `internal/playback/cdmhost`)
run only when `SONA_WIDEVINE_CDM` is set, as devenv does; CI skips them.
After touching playback or Apple request code, also run the DRM path
regression `go run ./cmd/playspike -seek 90` (needs `MUSIC_USER_TOKEN`).
Every playback run in tests, smokes and live checks uses private mode
(`SONA_PRIVATE=1`; development builds start private and `playspike` is
private by default), so nothing is reported to Apple or scrobbled.
Route long listening tests to your own PipeWire null sink, not your
speakers, and remove only the sinks you created.

When running the app for a check that doesn't need your desktop, start it
under `dbus-run-session` with private `XDG_CONFIG_HOME`, `XDG_DATA_HOME`,
`XDG_STATE_HOME` and `XDG_CACHE_HOME`, so it neither clashes with a running
Sona (single instance, MPRIS name) nor touches your real settings and library.

The CDM host sandbox tests (`TestChildCannotOpenInetSockets`,
`TestSandboxConfinesChild/namespaces`) need unprivileged user namespaces in
which the child may change its mounts. Where the system refuses them (EPERM
or EACCES: Flatpak, user namespaces disabled, or Ubuntu 24.04's AppArmor
restriction `kernel.apparmor_restrict_unprivileged_userns=1`) they skip and
say why; any other failure fails them. To run them on Ubuntu, lift the
restriction for the session with
`sudo sysctl -w kernel.apparmor_restrict_unprivileged_userns=0`. CI does, and
sets `SONA_CDMHOST_REQUIRE_NAMESPACES=1`, which turns a skip into a failure.

Packaging changes: `bash build/linux/release/release.sh` builds every package
on Ubuntu 24.04 and smoke-tests it on clean Ubuntu, Fedora and Arch
containers. Before a release, work through `docs/release-checklist.md`.

## Branches and pull requests

- One change per branch and pull request. Parallel work is easiest in
  separate worktrees (`git worktree add ../sona-go-<slug> -b <type>/<slug>`);
  a new worktree needs `pnpm install` in `frontend/` and a frontend build
  before `go build .`. Give each concurrently running worktree its own
  `WAILS_VITE_PORT`.
- Commit messages and PR titles follow
  [Conventional Commits](https://www.conventionalcommits.org): `feat`, `fix`,
  `perf`, `refactor`, `docs`, `test`, `ci`, `build`, `chore` or `revert`,
  with an optional scope (`fix(playback): …`). CI fails a PR with a
  non-conforming title; PRs are squash-merged under that title.
- Fill in the PR template: what changed, why, evidence (commands and their
  output; screenshots in light and dark themes for UI changes, without
  personal account data), tests, docs, and anything left.
- Update the docs describing what you change in the same PR.
- Generated files are never edited or merged by hand. After rebasing onto
  `main`, regenerate `frontend/bindings`, the Lingui catalogues
  (`pnpm i18n:extract` in `frontend/`) and `internal/db/dbq` (`gen`), then
  commit them.

## Module contract

A feature is a Go service plus a frontend module, each self-contained:

- Go: `internal/services/<feature>.go` (the service; exported methods are the
  bindings) + `<feature>_dto.go` (its DTOs and events) + one line in
  `internal/services/registry.go`. Features never call each other's
  services. Full contract: `internal/services/doc.go`.
- Frontend: `frontend/src/features/<name>/index.ts` exporting the module
  (routes, nav items, Go event handlers) + one line in
  `frontend/src/app/modules.ts`. Full contract:
  `docs/frontend-architecture.md`.
- Settings: one nested struct in `internal/config/settings.go` with its
  defaults and `normalized()`; apply changes live through
  `Settings.OnChange`.

## Architecture rules

- Go owns all Apple Music traffic, tokens, sync, caching, MPRIS and artwork.
  The frontend never holds tokens and never calls `*.apple.com`.
- Cold start reads SQLite only; the network and the playback host start
  after the first paint.
- Secrets go to Secret Service (or the explicit 0600 file fallback), never to
  the database, logs, settings or committed files.
- Linux performance: no `backdrop-filter` or large CSS blurs on scrolling
  content; backdrops are pre-blurred in Go.

## DRM rules (absolute)

- Protected playback runs in the CDM host child process
  (`internal/playback/cdmhost`). It loads the user's installed Widevine CDM
  and decrypts only through the CDM's own `Decrypt` API.
- Decrypted audio never leaves the child: it is never written to disk and
  never crosses the pipe to the parent. Audio processing happens in the
  child, after decryption.
- The child makes no network requests; the parent fetches everything and
  passes bytes in.
- Never redistribute, bundle, download or patch the CDM. Never extract keys or
  use reimplemented CDMs. Do not try to make Widevine work in WebKitGTK.

## Design rules

`docs/design.md` is the visual direction. Design every page fresh: Apple's
apps are functional references only, never layouts to copy. Fast before pretty;
light and dark follow the system; everything works from the keyboard; states
(signed out, syncing, offline, no CDM) are stated plainly.

## Licence

By contributing you agree your contribution is licensed under the AGPL-3.0
(`LICENSE`). Code copied from elsewhere must carry a compatible licence and
its notice; the vendored Chromium CDM headers keep theirs
(`internal/playback/widevine/LICENSE.chromium`, BSD-3-Clause).
