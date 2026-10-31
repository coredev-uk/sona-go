# Agent Notes

## Repo

- Go module `github.com/coredev-uk/sona-go`: the Sona desktop app. Entry `main.go` (Wails v3), Wails services in `internal/services`, core packages under `internal/`, React frontend in `frontend/`.
- The retired headless Chrome engine (`internal/playback/chrome`, `internal/playback/enginepage`, `cmd/enginespike`; chromedp v0.21) is kept only as the documented fallback. It is not wired into the app; do not extend it.
- Generated code, never hand-edited: `internal/db/dbq` (sqlc, from `internal/db/queries`) and `frontend/bindings` (wails3). Regenerate with the devenv `gen` script.
- Migrations (`internal/db/migrations`) are numbered densely from `00001` and applied strictly in order (no goose out-of-order): a new one takes the next free number when it lands, and a released one is never renumbered or edited.
- `VERSION` is the single version source. The build injects it as `main.version` and into the packages.

## Sources of Truth

- `docs/auth.md`: authentication. `docs/apple-music-reference.md`: Apple endpoints and features (functionality only). `docs/design.md`: visual direction. `docs/frontend-architecture.md` and `internal/services/doc.go`: module contracts.
- External reference, never copied wholesale: [vibez](https://github.com/simonepelosi/vibez) (MIT), which hosts the Widevine CDM the same way.

## Environment and Commands

- Run everything through devenv: `nix run nixpkgs#devenv -- shell -- <cmd>` (or `devenv shell` when installed). It sets `CGO_ENABLED=1`, `GOFLAGS=-tags=sqlite_fts5`, the GStreamer plugin path, and `SONA_WIDEVINE_CDM`.
- If locking inputs fails with an ssh hostkey error (a git config that rewrites https to ssh breaks Nix's fetcher), run devenv once with `XDG_CONFIG_HOME` pointing at an empty directory: `XDG_CONFIG_HOME=$(mktemp -d) devenv shell -- true`. Do not keep that prefix when running the app: it moves Sona's settings into that directory.
- devenv scripts: `dev` (Vite hot reload), `build` (frontend + bindings + `bin/sona`), `gen` (sqlc + TS bindings), `fmt` (gofumpt + prettier), `check` (go vet, golangci-lint, tsc, eslint), `test` (go test + vitest), `play-spike`, `auth-spike`.
- Before handing off: `check`, `go test -race ./...`, and in `frontend/`: `pnpm exec prettier --check src` and `pnpm exec vitest run`.
- `wails3` is a Go tool dependency (`go tool wails3`). Plain `go build .` needs `frontend/dist` built first; production builds use `-tags production,sqlite_fts5`.
- DRM path regression, after CDM, playback, or Apple changes: `go run ./cmd/playspike -seek 90`.
- Release packages: `bash build/linux/release/release.sh` (podman, Ubuntu 24.04; output in `bin/release`).
- `.env` (ignored) holds `MUSIC_USER_TOKEN` for local testing. Never print, log, or commit it.

## Adding Features

- A new page or feature is a Go service (`internal/services/<feature>.go` + `<feature>_dto.go` + one `registry.go` line) and a frontend module (`frontend/src/features/<name>/index.ts` + one `frontend/src/app/modules.ts` line).
- Design every page fresh with `skill://impeccable` and `skill://design-taste-frontend`; never copy Apple's layouts.

## Stack

- Go 1.26+, Wails v3.0.0 (GTK4 + WebKitGTK 6.0 on Linux), React + TypeScript + Vite, ShadCN + Tailwind, TanStack Query and Router, `pnpm`.
- SQLite via `github.com/mattn/go-sqlite3` (FTS5 through the `sqlite_fts5` build tag) with a libSQL-compatible schema, goose + sqlc.
- Playback: Google's Widevine CDM (`libwidevinecdm.so`) hosted by Sona through a C++ shim, GStreamer `appsrc` for AAC decode and PipeWire output.

## Architecture Rules

- Go owns all Apple Music HTTP traffic, tokens, sync, caching, MPRIS, and artwork. The frontend never holds tokens and never calls `*.apple.com`.
- The developer token is the web player token (`devtoken.WebPlayer`). Every Apple request must send `Origin: https://music.apple.com`; go through `applemusic.Client`.
- MUT acquisition: the native in-app sign-in (`internal/auth/appleid`, `auth.Manager.SignInWithPassword`/`SubmitSecurityCode`) is primary; the loopback redirect flow (`internal/auth/mut`) and token paste are fallbacks. Never MusicKit popups or webview header rewriting. Passwords and 2FA codes are never stored or logged.
- Protected playback runs in the CDM host child process (`internal/playback/cdmhost`): it loads the user's installed Widevine CDM, decrypts through the CDM's own `Decrypt` API, and hands clear AAC only to GStreamer inside that process. Decrypted audio never leaves the child, is never written to disk, and never crosses the pipe to the parent.
- The CDM host process makes no network requests. The parent fetches playlists, certificates, licences and segments and passes bytes to the child.
- Never redistribute, download-and-bundle, or patch the CDM. Never extract keys or use reimplemented CDMs (`openwv`, extracted device keys).
- Do not try to make Widevine work in WebKitGTK.
- Go's MPRIS (`org.mpris.MediaPlayer2.sona`) is the only Sona player on D-Bus.

## Linux Performance Rules

- No `backdrop-filter` or large CSS blurs on scrolling content. Pre-blur backdrops in Go.
- Cold start reads SQLite only. The network and the playback host start after first paint.

## Secrets

- Never store tokens or DRM credentials in database tables, logs, or committed files. Use Secret Service (go-keyring) with an explicit, user-visible 0600 file fallback.

## Worktrees and Branches

- Prefer a separate git worktree and branch for each feature or fix, so independent work can run side by side: `git worktree add ../sona-go-<slug> -b <type>/<slug>` (`<type>` is the conventional-commit type, e.g. `feat/queue-autoplay`). A small, self-contained change can be made on a branch in the main checkout instead.
- Run independent items in parallel worktrees where they touch different files; serialise items that edit the same files (group them into waves).
- Generated files are never merged by hand: after rebasing onto `main`, regenerate `frontend/bindings`, the Lingui catalogues (`pnpm i18n:extract`) and `internal/db/dbq` (`gen`), then commit them.
- A new worktree has no `frontend/node_modules`, `frontend/dist` or `.env`. Run `pnpm install` in `frontend/` inside devenv, build the frontend before `go build .`, and symlink `.env` from the main checkout only when a live token is needed.
- Worktrees running at once must not share runtime state: a different `WAILS_VITE_PORT` per worktree, private XDG dirs and `dbus-run-session` for every run except one on the real desktop session, and a separately named PipeWire null sink.
- Commits and PR titles follow Conventional Commits (`feat:`, `fix:`, `perf:`, `refactor:`, `docs:`, `test:`, `ci:`, `build:`, `chore:`).
