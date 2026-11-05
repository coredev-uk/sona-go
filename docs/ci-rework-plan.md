# CI rework plan

Status: benched. Branch `ci/test-correctness` holds this plan and the test
fixes already made (see "Done on this branch"). Measurements are from the CI
runs of 2026-11-04/05 (`origin/main` at `63d0d40`…`c1276c7`).

Goal: every test that should run does run, a red check blocks a merge, and a
pull request's CI finishes in about 3–4 minutes within the free plan's limits
(20 concurrent jobs, 5 macOS) and a hard 10 GB Actions cache cap.

## Findings

| # | Problem | Evidence |
|---|---|---|
| 1 | Nothing blocks a red merge | `main` ruleset disabled, no branch protection; #138, #141, #144, #145 merged ~3 s after opening; CI on `main` red for 8 merges in a row |
| 2 | macOS test failures report green | `macos-test` runs `test.sh … \| tee` under the default `bash -e` (no `pipefail`); run 41950203787 had 8 failing tests in green jobs |
| 3 | Tests skip on every run unnoticed | Linux: kit motion goldens (no H.264 in the `lite` packages), 2 PipeWire tests in `playback/gst`, zsh completion. macOS: every motion test (`gst-launch-1.0` not on `PATH`: setup-macos never adds the framework's `bin`) |
| 4 | Go caches never restore | setup-linux/setup-macos restore `~/.cache/go-build` but save the expanded `$HOME/...`; `actions/cache` versions the entry by the path string (`947557cc…` asked for, `97a2653c…` saved). ~3.4 GB uploaded per `main` commit, never read; cache at 11.4 GB of 10 GB |
| 5 | Test results never cached | `test.sh` always passes `-artifacts`, which is not a cacheable `go test` flag (checked: three runs, none `(cached)`) |
| 6 | `-race` dominates UI time | Same commit, `-count=1`: `features/home` 42.5 s vs 8.4 s, `kit` 79.3 s vs 8.3 s. UI pool is 1,071 test-seconds under `-race`, hence 10 shards |
| 7 | Job count exceeds concurrency | 23 jobs per PR, 26 per `main` push; queue waits up to 130 s (PR) and 215 s (`main`) |
| 8 | Goldens depend on the runner image | Pango/fonts come from `apt` on `ubuntu-26.04`; three image versions in two weeks. Full-window goldens churn: `shell-albums-*` and `shell-account-*` re-captured 10× each in 7 days; 22 of the last 100 commits touched goldens |
| 9 | Flaky test | `TestLoaderCancelsACoverInFlightAndFetchesItAgainWhenAsked` (fixed here) |
| 10 | `ui/app` runs twice | the `@dev` run repeats all 69 tests for `restore_dev.go`/`restore_prod.go` (78 s, slowest shard) |

## Decisions

- Ship as a stacked series of four PRs, in the order below.
- `-race`: on PRs, for core, playback and the UI packages that start
  goroutines (`ui/store`, `ui/media`, `ui/kit`, `ui/lyricsync`); page and app
  packages run without it. Full `-race ./...` on every push to `main` and
  nightly. AGENTS.md's rule changes with PR 3.
- CI image on GHCR, amd64 and arm64.
- Prune the full-window shell goldens.
- Label-triggered golden update workflow.
- One macOS job on PRs; full macOS shards stay on `main`.
- Flaky tests (fail, then pass on rerun) pass the job and are reported.
- The `main` ruleset is enabled once the check names are final (the agent
  does it through `gh api`).
- No self-hosted runners: fork PRs could run code on them, and the cluster's
  CPUs are slower than hosted runners for `-race`/cgo work.

## Done on this branch

- macOS bash completion: the generated script used `mapfile` (bash 4+);
  macOS's `/bin/bash` is 3.2. `internal/cli/completion.go` now fills
  `COMPREPLY` with a `while read` loop; `TestBashCompletion` runs under the
  bash on `PATH` and `/bin/bash`.
- macOS socket paths: discord and probe/measure tests listened on sockets
  under `t.TempDir()`, longer than macOS's 104 bytes. They use a short
  `socketDir` helper, the pattern `internal/app/socket_test.go` already has.
  Reproduced before (8 failures with the runner's `TMPDIR`), passing after.
- Flaky artloader test: it waited for one cover's bitmap, then counted both
  requests, but a request is recorded only after it passes the gate. It now
  waits for both. 309 failures in 3,000 runs before (`-race -cpu 1,2,8`),
  0 after.

## PR 1: correctness (`ci/test-correctness`)

1. Workflow-level `defaults: run: shell: bash` in `ci.yml` (gives
   `-eo pipefail` everywhere, macOS included).
2. Cache paths: restore with `${{ steps.keys.outputs.path }}`, the same string
   the save uses, in setup-linux and setup-macos. Add the runner's
   `ImageVersion` to the Go cache key (no fallback across it), so a runner
   image update can't serve a cached pass with stale Pango.
3. Cache pruning: a final job on `push` to `main` (`actions: write`) deletes
   every entry but the newest per key prefix (`gh cache list`/`gh cache
   delete`).
4. Test cache: drop `-artifacts -outputdir` from `test.sh`. Add a leaf
   package `internal/ui/uitest/artifacts` with `Dir(t)`: returns
   `$SONA_TEST_ARTIFACTS/<test name>` when set, else `t.ArtifactDir()`. Call
   it only on the failure path (golden `save`, kit's controls and media
   goldens). A leaf package because `uitest` imports `kit`. `test.sh` sets
   the variable; the upload step is unchanged. Update the golden paragraphs
   in CONTRIBUTING.md and docs/ui-architecture.md.
5. gotestsum as a Go tool dependency (`go get -tool
   gotest.tools/gotestsum@v1.13.0`). `test.sh` runs `go tool gotestsum
   --jsonfile … --format pkgname-and-test-fails --rerun-fails=2
   --rerun-fails-report=… --packages … -- -race …` (gotestsum swaps its own
   `-run` in on reruns, so kit's split parts still work). Flaky tests go to
   `$GITHUB_STEP_SUMMARY` and a `::warning::`.
6. Skip allow-list replaces `mustpass.txt`: `.github/ci/skips-linux.txt` and
   `skips-darwin.txt`, lines `<package suffix|*> <test regex> [reason
   substring]`; `skips.sh` fails the job on any skip event (subtests
   included) that no line matches. Every test job runs it.
   - Linux allowed: live tests (`applemusic TestLive`, `motion
     TestLiveRendition`, `playback/cdm TestLiveDownload`), evidence tests
     (`*Evidence`), `ui/media TestOpenMotionWithoutDecoder` and
     `TestMotionFileOfTheCache`.
   - macOS allowed: the above plus D-Bus-only tests (reason `dbus-daemon not
     available`), goldens and pixel readings (reason "on Linux only"),
     `playback/native TestEqualizerAndDeviceFollowHosts`, `playback/gst
     TestFilePipelineMovesOutputInEveryState`, `probe/measure
     TestTreeUsageOfThisProcess` (`/proc`).
7. Prerequisites for the rest: UI shards install the `test` package list
   (H.264 for kit); core adds `zsh`; playback installs `pipewire` and
   `wireplumber` and starts them inside its `dbus-run-session` with a private
   `XDG_RUNTIME_DIR` (the gst tests look for `$XDG_RUNTIME_DIR/pipewire-0`
   and use `pw-cli`); setup-macos adds the GStreamer framework's `bin` to
   `GITHUB_PATH`.
8. Acceptance: `actionlint` clean; a dispatch run on the branch where macOS
   jobs fail on a planted failure and pass without it; second `main` run
   shows Go cache hits and `(cached)` packages; no un-allowed skips.

## PR 2: CI image

1. `build/ci/Containerfile` (Ubuntu 26.04): GTK headers, GStreamer with
   plugins base/good/bad/libav, sway, wtype, AT-SPI, D-Bus, Mesa, fonts,
   PipeWire, zsh, Go from `go.mod`, gotestsum. Sets `SONA_CI_IMAGE` to its
   tag.
2. `images.yml` builds and pushes `ghcr.io/coredev-uk/sona-ci:<Containerfile
   hash>` for amd64 (ubuntu runner) and arm64 (ubuntu-arm runner), merged
   into one manifest; on Containerfile change, weekly, and on dispatch. GHCR
   storage for a public repo does not count against the cache cap.
3. Vet, type-check, core, UI and E2E jobs run in `container:` with that
   image; their apt steps and `apt-*` caches go. Playback stays on the bare
   runner (the user-namespace `sysctl` is the host's).
4. The Go cache key uses the image tag instead of `ImageVersion`.
5. CONTRIBUTING.md: updating goldens on a Mac with `podman run … go test
   ./internal/ui/... -update`, noting that arm64 may differ slightly from
   CI's amd64 and CI is the reference.

## PR 3: `-race` scope and fewer jobs

1. Pools in `shard.sh`: `ui-race` (`ui/store`, `ui/media`, `ui/kit`,
   `ui/lyricsync`) and `ui` (the rest) without `-race` on PRs; on `main` and
   the nightly schedule every pool runs with `-race`.
2. Nightly `schedule` run: everything with `-race -count=1`, catching what
   the test cache can't see.
3. Shards: UI 2, UI-race 1, core 1, playback 1 on PRs (about 8 jobs total);
   re-measure with `timings.sh` and adjust.
4. One Go cache per OS and build kind (race, no-race, vet), saved by one job
   each, instead of one per shard.
5. `@dev` runs only the restore tests in `ui/app` (move them to
   `//go:build dev` test files or `-run`).
6. One macOS job on PRs: build, vet, core and playback tests.
7. AGENTS.md (the `-race` rule, the gate) and CONTRIBUTING.md ("CI and
   workflows", which still describes the old single-job layout).

## PR 4: goldens

1. Keep `shell-albums-light/dark`; delete `shell-account-*`,
   `shell-account-narrow-*` and the `shell-equalizer-*` sizes, after moving
   what each asserts that no page golden covers to page goldens or
   `Find`/`Texts`/accessibility-tree checks.
2. `golden-update.yml`: on the `update-goldens` label of a same-repo PR (fork
   PRs refused), run `-update` in the CI image, commit the changed PNGs and
   push as bot-cecil (App token, so CI runs on the push), then remove the
   label. Secrets in an environment limited to that workflow.

## Gate

After PR 3 (check names final), enable the `main` ruleset requiring: "Go vet
and test (-race)", "UI tests (goldens)", "E2E (headless sway)", "PR title",
"Secret scan". Clarence still merges.

## Cache budget (10 GB)

| Cache | Today per `main` commit | Planned |
|---|---|---|
| Go, Linux `-race` | 13 entries, ~1.8 GB | 1, ~0.5 GB |
| Go, Linux no-race | 4 entries, ~0.6 GB | 1, ~0.3 GB |
| Go, macOS | 5 entries, ~0.8 GB | 1, ~0.3 GB |
| Go, Windows | 0.46 GB | 0.46 GB |
| macOS GStreamer installer | 0.92 GB | 0.92 GB |
| apt `.deb`s | ~0.2 GB | 0 (image) |
| Widevine | 0.03 GB | 0.03 GB |
| Total | grows ~3.4 GB per commit, evicts | ~2.5 GB steady |

The planned sizes per entry are estimates; check them after PR 3.
