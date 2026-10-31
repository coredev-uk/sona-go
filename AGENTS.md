# Agent Notes

## Repo

- Go module `github.com/coredev-uk/sona-go`: the Sona desktop app. Entry `main.go` (Wails v3), Wails services in `internal/services`, core packages under `internal/`, React frontend in `frontend/`. Layout: `docs/architecture.md` §6.
- Generated code, never hand-edited: `internal/db/dbq` (sqlc, from `internal/db/queries`) and `frontend/bindings` (wails3). Regenerate with the devenv `gen` script.
- Migrations (`internal/db/migrations`) are numbered densely from `00001` and applied strictly in order (no goose out-of-order): a new one takes the next free number when it lands, and a released one is never renumbered or edited.
- `VERSION` is the single version source. The build injects it as `main.version` and into the packages.

## Sources of Truth

- `docs/architecture.md`: product scope, architecture and the decisions behind it (process model, playback and DRM, module layout, data and sync, performance, platforms, testing).
- `docs/auth.md`: authentication. `docs/apple-music-reference.md`: Apple endpoints and features (functionality only). `docs/design.md`: visual direction. `docs/frontend-architecture.md` and `internal/services/doc.go`: module contracts.
- External reference, never copied wholesale: [vibez](https://github.com/simonepelosi/vibez) (MIT), which hosts the Widevine CDM the same way.

## Environment and Commands

- Run everything through devenv 2.4+: `devenv shell -- <cmd>` (without an install: `nix run github:NixOS/nixpkgs/nixpkgs-unstable#devenv -- shell -- <cmd>`; plain `nixpkgs#devenv` may be older and is refused). It sets `CGO_ENABLED=1`, `GOFLAGS=-tags=sqlite_fts5` and the GStreamer plugin path, and loads `.env`. nixpkgs is pinned in `devenv.yaml` (bumping: CONTRIBUTING.md).
- If locking inputs fails with an ssh hostkey error (a git config that rewrites https to ssh breaks Nix's fetcher), run devenv once with `XDG_CONFIG_HOME` pointing at an empty directory: `XDG_CONFIG_HOME=$(mktemp -d) devenv shell -- true`. Do not keep that prefix when running the app: it moves Sona's settings into that directory.
- devenv scripts (each described in CONTRIBUTING.md): `dev` (Vite hot reload, `-config ./build/config.yml`), `build` (frontend + bindings + `bin/sona`), `gen` (sqlc + TS bindings), `fmt` (gofumpt + prettier), `check` (frontend build, go vet, the Windows and macOS type-checks, golangci-lint on new code, path case, tsc, eslint, prettier, i18n), `tests` (frontend build, `go test -race` under `dbus-run-session`, vitest), `release` (release packages in a container), `play-spike`, `auth-spike`. `package` is Taskfile's only.
- Before handing off: `check` and `tests`.
- `wails3` is a Go tool dependency (`go tool wails3`). Plain `go build .` needs `frontend/dist` built first; production builds use `-tags production,sqlite_fts5`.
- Widevine CDM: devenv ships none. The app downloads it itself after first paint (into `<data>/widevine`; `mac_arm64` and `mac_x64` on macOS, with the `.sig` beside the dylib); `playspike` uses the same downloader (`-cdm-dir`). There is no override: Sona only ever loads the CDM its own downloader installed. Linux on arm64 has no CDM in the manifest, so playback there reports `unsupported_platform`. `go run ./cmd/cdmfetch` downloads and verifies it by hand (`-dir`, `-platform goos/goarch`, `-version` prints the manifest's version without downloading). Real-CDM tests find it through `internal/playback/cdm/cdmtest` (a download into `SONA_CDM_CACHE`, default `<user cache>/sona-tests/widevine`); they skip only when offline, and `SONA_CDM_REQUIRE=1` makes that skip a failure. macOS development builds play through the same CDM host under a `sandbox-exec` profile (CONTRIBUTING.md, "macOS").
- DRM path regression, after CDM, playback, or Apple changes: `go run ./cmd/playspike -seek 90`.
- Release packages: `release` (`bash build/linux/release/release.sh`: podman or docker, falling back to `nix run nixpkgs#podman`; Ubuntu 24.04; output in `bin/release`).
- `.env` (ignored) holds `MUSIC_USER_TOKEN` for local testing. Never print, log, or commit it.

## Adding Features

- A new page or feature is a Go service (`internal/services/<feature>.go` + `<feature>_dto.go` + one `registry.go` line) and a frontend module (`frontend/src/features/<name>/index.ts` + one `frontend/src/app/modules.ts` line).
- Design every page fresh with `skill://impeccable` and `skill://design-taste-frontend`; never copy Apple's layouts.

## Stack

- Go 1.26+, Wails v3.0.0 (GTK4 + WebKitGTK 6.0 on Linux), React + TypeScript + Vite, ShadCN + Tailwind, TanStack Query and Router, `pnpm`.
- SQLite via `github.com/mattn/go-sqlite3` (FTS5 through the `sqlite_fts5` build tag) with a libSQL-compatible schema, goose + sqlc.
- Playback: Google's Widevine CDM (`libwidevinecdm.so`, `.dylib` on macOS) downloaded by Sona from Google (`internal/playback/cdm`) and hosted through a C++ shim, GStreamer `appsrc` for AAC decode and PipeWire output (Core Audio on macOS).

## Architecture Rules

- Go owns all Apple Music HTTP traffic, tokens, sync, caching, MPRIS, and artwork. The frontend never holds tokens and never calls `*.apple.com`.
- The developer token is the web player token (`devtoken.WebPlayer`). Every Apple request must send `Origin: https://music.apple.com`; go through `applemusic.Client`.
- MUT acquisition: the native in-app sign-in (`internal/auth/appleid`, `auth.Manager.SignInWithPassword`/`SubmitSecurityCode`) is primary; the loopback redirect flow (`internal/auth/mut`) and token paste are fallbacks. Never MusicKit popups or webview header rewriting. Passwords and 2FA codes are never stored or logged.
- Protected playback runs in the CDM host child process (`internal/playback/cdmhost`): it loads the downloaded Widevine CDM, decrypts through the CDM's own `Decrypt` API, and hands clear AAC only to GStreamer inside that process. Decrypted audio never leaves the child, is never written to disk, and never crosses the pipe to the parent.
- The CDM host process makes no network requests. The parent fetches playlists, certificates, licences and segments and passes bytes to the child.
- The CDM is never bundled (in packages or the binary), redistributed or patched. Sona downloads it only from Google's servers: version and SHA-512 from Mozilla's pinned Firefox GMP manifest, the CRX3 signatures verified (`internal/playback/cdm`), stored in Sona's data dir (each library's SHA-256 recorded in `current.json`; a missing or damaged one is removed and downloaded again, and 3 failed host starts roll back to the previous version). A new version must pass a load check that loads only the CDM (no GStreamer); only a CDM that fails to load is rejected, while a check that can't finish keeps the verified files for the next try. Browser, system and user-supplied copies are never used.
- Never extract keys or use reimplemented CDMs (`openwv`, extracted device keys).
- Do not try to make Widevine work in WebKitGTK.
- Go's MPRIS (`org.mpris.MediaPlayer2.sona`) is the only Sona player on D-Bus.
- OS-specific code goes behind the platform seams (`platform_<os>.go`, `services.Platform`, build-tag conventions: CONTRIBUTING.md "Platforms"). Where a platform lacks a feature it reports it (`errors.ErrUnsupported`, `PlatformService.Capabilities`) and the UI hides or explains the control; never a stub that pretends to work.

## Linux Performance Rules

- No `backdrop-filter` or large CSS blurs on scrolling content. Pre-blur backdrops in Go.
- Cold start reads SQLite only. The network and the playback host start after first paint.

## Secrets

- Never store tokens or DRM credentials in database tables, logs, or committed files. Use the system keyring (go-keyring: Secret Service, Keychain, Credential Manager) with an explicit, user-visible 0600 file fallback.

## Worktrees and Branches

- Every change gets its own git worktree and branch, beside the main checkout, which stays on `main`: `git worktree add ../sona-go-<slug> -b <type>/<slug> origin/main` (`<type>` is the conventional-commit type, e.g. `feat/queue-autoplay`). Never commit or push to `main`: `.githooks/` refuses both. `devenv shell` enables the hooks (`core.hooksPath`, shared by every worktree); outside devenv run `git config core.hooksPath .githooks` once. `SONA_ALLOW_MAIN=1` overrides them only for an emergency the maintainer asked for.
- Run independent items in parallel worktrees where they touch different files; serialise items that edit the same files (group them into waves).
- Generated files are never merged by hand: after rebasing onto `main`, regenerate `frontend/bindings`, the Lingui catalogues (`pnpm i18n:extract`) and `internal/db/dbq` (`gen`), then commit them.
- A new worktree has no `frontend/node_modules`, `frontend/dist` or `.env`. Run `pnpm install` in `frontend/` inside devenv, build the frontend before `go build .`, and symlink `.env` from the main checkout only when a live token is needed.
- Worktrees running at once must not share runtime state: a different `WAILS_VITE_PORT` per worktree, private XDG dirs and `dbus-run-session` for every run except one on the real desktop session, and a separately named PipeWire null sink.
- Commits and PR titles follow Conventional Commits (`feat:`, `fix:`, `perf:`, `refactor:`, `docs:`, `test:`, `ci:`, `build:`, `chore:`, `revert:`; scope optional). CI (`.github/workflows/pr-title.yml`) fails a PR whose title doesn't match; squash merges use the PR title.

## Process

- One item per branch and PR. Fill in the PR template (`.github/pull_request_template.md`): What, Why, Evidence (commands with their output; light and dark screenshots for UI, with no personal account data), Tests, Docs, Left.
- Gate before every PR: `go vet ./...`, the cross-platform type-checks (`GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go vet ./...`, `go run ./cmd/typecheck -os darwin -arch arm64 ./...`), `go test -race ./...` (touched packages first, then repo-wide), `golangci-lint run --new-from-rev=origin/main ./...` clean, and in `frontend/`: `pnpm exec tsc --noEmit`, `pnpm exec eslint src`, `pnpm exec prettier --check src`, `pnpm i18n:check`, `pnpm exec vitest run`.
- Update the docs describing what you change in the same PR.
- Private mode for every playback run: tests, smokes and live checks run with `SONA_PRIVATE=1` (development builds start private, `playspike` is private by default) so nothing is reported to Apple or scrobbled. Never play against Apple without it.
- Live audio goes to your own PipeWire null sink (`pw-cli create-node adapter '{ factory.name=support.null-audio-sink node.name=<name> media.class=Audio/Sink object.linger=true audio.position=[FL FR] }'`), targeted with `pipewiresink target-object` (`-device` in playspike; `PIPEWIRE_NODE` pins a stream and breaks device-move tests) and recorded from its monitor with `pw-record --target <name> -P '{ stream.capture.sink=true }' out.wav` (without `stream.capture.sink`, `pw-record` falls back to the default source, i.e. the microphone; check the links with `pw-link -l`). Destroy only nodes you created; if WirePlumber stops linking streams, ask before restarting it.
- Only one Sona may own the real session bus (single instance and the MPRIS name). Keep real-session runs short and quit the app yourself; everything else uses `dbus-run-session` and private XDG dirs, never the real `~/.config/sona` or `~/.local/share/sona` (copy data into the private dirs when needed).

## Pull requests and Clarence

Pull requests here are merged by Clarence (GitHub App `bot-clarence`), not by hand.

- Never commit or push to `main`. Work on a branch in its own git worktree:
  `git worktree add ../<repo>-<topic> -b <type>/<topic> origin/main`. The git hooks refuse
  commits and pushes on `main`.
- Open the pull request with a conventional-commit title. Clarence posts one card on it (verdict,
  checks, what it changes, timeline) and keeps it current; read it before asking anyone to look.
- Never merge with `gh pr merge` or the GitHub button. Once CI is green and the work is done,
  comment `/clarence merge` (or tick 🎯 Merge on the card). Clarence merges one pull request at a
  time, then checks the merge (CI on `main`, or the deploy) and opens a revert if it fails.
- When Clarence needs a human it requests a review from @coredev-uk; don't approve or merge on
  the maintainer's behalf, and don't merge anything Clarence ruled `clarence/review` without his say.
- Commands (first line of a comment, `coredev-uk` only): `/clarence merge` (`--allow <check>`,
  `--batch`, `at HH:MM`, `after #N`), `cancel`, `hold`, `unhold`, `reclassify`, `status`, `explain`,
  `revert` (on a merged pull request), `help` (anyone).
- Renovate (`bot-cecil`) opens dependency updates and Clarence merges the eligible ones itself.
  Don't push to Renovate's branches; close the pull request or tick Renovate's rebase box instead.
- Labels: `clarence/pass|review|wait` (verdict), `clarence/queued|deploying|live|unhealthy`,
  `clarence/hold`, `clarence/security`, `clarence/proposal`, `clarence/revert`. Never remove
  `clarence/unhealthy` without fixing the cause: it halts merging.
