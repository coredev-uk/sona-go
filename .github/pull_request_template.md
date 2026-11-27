<!--
Title: Conventional Commits, e.g. `feat: queue autoplay` or `fix(playback): seek past end`.
Allowed types: feat, fix, perf, refactor, docs, test, ci, build, chore, revert. CI checks it.
-->

## What

<!-- What changed, in a few lines. -->

## Why

<!-- The problem this addresses, with a link to its issue if there is one. -->

## Evidence

<!--
Commands you ran and the tail of their output.
UI changes: screenshots in light AND dark themes, from the fixture library or a test account.
No personal data anywhere in this PR: names, handles, account IDs, real library or playlist contents, listening history, logs or diagnostics from a real account (AGENTS.md, Process).
-->

## Tests

<!-- Tests added or changed: behaviour, boundaries, regressions (`go test`; UI behaviour with `ui.Tester`). -->

## Docs

<!-- Docs updated in this PR, or why none apply. -->

## Left

<!-- Anything not done or not reachable, and why. "Nothing" is fine. -->

## Checklist

- [ ] `go vet ./...` and `go test -race ./...` pass
- [ ] `golangci-lint run --new-from-rev=origin/main ./...` is clean
- [ ] `go tool mygo vet internal/ui`, `go run ./cmd/i18ntool extract -check` and `go run ./cmd/pathcase` pass
- [ ] `sh build/gofumpt.sh` prints nothing (`fmt` formats the files)
- [ ] Generated files (the catalogue, `internal/db/dbq`) regenerated, not hand-edited
- [ ] Every playback run (tests, smokes, live checks) used private mode (`SONA_PRIVATE=1`), or there was none
- [ ] No secrets, tokens or personal data (a real account's names, IDs, library contents or logs) in code, tests, goldens, screenshots or this description
