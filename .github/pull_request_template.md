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
UI changes: screenshots in light AND dark themes, with no personal account data, tokens or library names.
-->

## Tests

<!-- Tests added or changed: behaviour, boundaries, regressions. -->

## Docs

<!-- Docs updated in this PR, or why none apply. -->

## Left

<!-- Anything not done or not reachable, and why. "Nothing" is fine. -->

## Checklist

- [ ] `go vet ./...` and `go test -race ./...` pass
- [ ] `golangci-lint run --new-from-rev=origin/main ./...` is clean
- [ ] In `frontend/`: `pnpm exec tsc --noEmit`, `pnpm exec eslint src`, `pnpm exec prettier --check src`, `pnpm i18n:check`, `pnpm exec vitest run` pass
- [ ] Generated files (`frontend/bindings`, Lingui catalogues, `internal/db/dbq`) regenerated, not hand-edited
- [ ] Every playback run (tests, smokes, live checks) used private mode (`SONA_PRIVATE=1`), or there was none
- [ ] No secrets, tokens or personal data in code, logs or screenshots
