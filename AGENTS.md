# Agent Notes

## Current Repo State

- Go module `github.com/coredev-uk/sona-go`: the Sona desktop app (`main.go`, Wails v3) with Go services in `internal/services`, core packages under `internal/`, and the frontend in `frontend/`. The retired Chrome engine (`internal/playback/chrome`, `internal/playback/enginepage`, `cmd/enginespike`) is kept only as the documented fallback and is not wired into the app.
- Run everything through devenv: `nix run nixpkgs#devenv -- shell -- <cmd>` (or `devenv shell` when installed). It sets `CGO_ENABLED=1`, `GOFLAGS=-tags=sqlite_fts5`, the GStreamer plugin path, and `SONA_WIDEVINE_CDM`. If locking inputs fails with an ssh hostkey error (a git config that rewrites https to ssh breaks Nix's fetcher), run devenv once with `XDG_CONFIG_HOME` pointing at an empty directory: `XDG_CONFIG_HOME=$(mktemp -d) devenv shell -- true`. Do not keep that prefix when running the app: it moves Sona's settings into that directory.
- `wails3` is a Go tool dependency: `go tool wails3 build` (frontend + bindings + `bin/sona`), `go tool wails3 generate bindings -clean=true -ts`. `go build .` needs `frontend/dist` built first; production builds use `-tags production,sqlite_fts5`.
- Checks: `go vet ./... && go test -race ./...`; in `frontend/`: `pnpm exec tsc --noEmit`, `eslint src`, `prettier --check src`, `vitest run`. DRM path regression: `go run ./cmd/playspike -seek 90`. Release: `bash build/linux/release/release.sh` (podman, Ubuntu 24.04).
- `.env` (ignored) holds `MUSIC_USER_TOKEN` for local testing. Never print, log, or commit it.
- `docs/auth.md` is the source for authentication, `docs/apple-music-reference.md` for Apple endpoints and features (functionality only), `docs/design.md` for visual direction, `docs/frontend-architecture.md` and `internal/services/doc.go` for the module contracts.
- New pages/features: a Go service (`internal/services/<feature>.go` + `_dto.go` + one `registry.go` line) and a frontend module (`frontend/src/features/<name>/index.ts` + one `src/app/modules.ts` line). Design every page fresh with `skill://impeccable` and `skill://design-taste-frontend`; never copy Apple's layouts.
- External reference, never copied wholesale: [vibez](https://github.com/simonepelosi/vibez) (MIT), which hosts the Widevine CDM the same way.

## Stack

- Go 1.26+, Wails v3.0.0 (GTK4 + WebKitGTK 6.0 on Linux), React + TypeScript, ShadCN + Tailwind, `pnpm`.
- SQLite via `github.com/mattn/go-sqlite3` (FTS5 through the `sqlite_fts5` build tag) with a libSQL-compatible schema, goose + sqlc.
- Playback: Google's Widevine CDM (`libwidevinecdm.so`) hosted by Sona through a C++ shim, GStreamer `appsrc` for AAC decode and PipeWire output.
- chromedp v0.21 (only for the retired engine): generic/iterator API (`chromedp.Run`/`Do`, `chromedp.Call(ctx, cmd, params)`, `chromedp.Events`, `ExposeFunc`).

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
