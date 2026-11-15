# CI workflows

What each GitHub Actions workflow does, the CI image, and the rules for changing workflows. The threat model and the secrets are in `ci-and-secrets.md`.

## Workflows

`.github/workflows/`: `ci.yml` (every push to main, every pull request and a
nightly schedule: "Vet and type-check" (vet for the host, development and
probe builds, and the macOS and Windows type-checks); the Linux tests, in
pools that `.github/ci/shard.sh` packs by measured time (`timings.txt`):
core, playback, and the native UI's, split into `ui-race` (`ui/store`,
`ui/kit`, `ui/lyricsync`, which start goroutines) and `ui` (the page and app
packages). A pull request runs `-race` on core, playback and `ui-race` and
none on `ui`; main and the nightly run every pool under `-race` (the nightly
with `-count=1`, so the test cache can't hide what changed outside the code)
and `ui` without it too, which keeps the caches pull requests restore warm.
The required checks keep their names: "Go vet and test (-race)" gathers vet,
core and playback, "UI tests (goldens)" the UI shards; plus the end-to-end
smoke in headless sway, "E2E (headless sway)". macOS: a pull request that
reaches the code, and a push to main, each run one job, "macOS build, vet, core
and playback tests" (production build, go vet, core and playback tests with
the real CDM and Seatbelt). The macOS UI shards run only in the nightly and on
dispatch; the UI tests run on Linux on every push. GitHub runs at most five
macOS jobs at a time across all of the account's repositories, so one job a
push leaves room for the rest. Main also runs
"Windows build" (windows-2025: the official GStreamer packages and the MSVC-built CDM shim DLL from `.github/actions/setup-windows`, `go vet` with cgo, `sona.exe`, `playspike.exe`, two preview songs played to `fakesink` with no secret (the step runs again, up to four attempts, when `playspike -preview` exits with status 75, "the network failed"; any other failure fails it), the motion artwork tests (`internal/ui/media`: a generated H.264 clip played through Media Foundation by MyGo's `ui/video`, which must become Ready and loop, with the open and loop-point timings in the log; a skipped one fails the step), the CDM tests)),
`windows-playback.yml` (weekly and on dispatch, from main, `live` environment: playspike on windows-2025 plays a full track privately through the CDM host child to `fakesink`; its last line says whether Apple licensed the host; a failing weekly run opens an issue),
`build-deps.yml` (on a push to main that changes `build/windows/gstreamer.sha256` or `build/darwin/gstreamer.sha256`, and on dispatch: mirrors the pinned GStreamer packages as assets of the `build-deps` prerelease, which never becomes "latest" and has no v* tag. A Windows and a macOS job restore them from the setup actions' Actions cache entry, falling back to gstreamer.freedesktop.org on a miss, check each against its pin, and upload it. `build/gstreamer-fetch.sh` downloads from the mirror first and falls back to upstream. GitHub serves the 1 GB Windows installer in seconds and keeps it. freedesktop.org is slow and behind a bot check, and has timed a release out; the cache drops an entry 7 days after its last use, sooner near the 10 GB cap. After bumping GStreamer, let this run on main before a release, or dispatch it),
`images.yml` (also builds the CI image, below),
`golden-update.yml`, `nix-vendor-hash.yml` ("Nix vendorHash": builds the Go
modules of `build/linux/nix/package.nix` on pushes to main and on pull requests
that touch `go.mod`, `go.sum`, the flake or the package) and `nix-branch.yml`
(the flake's `nix` branch) commit as the bot-cecil App; how they work is
maintainer documentation (`docs/maintainers/ci-and-secrets.md`),
`cache-prune.yml` (after each CI run on main: keeps the newest Go cache of
each kind and deletes the rest, so the 10 GB cap never evicts what matters; a
`workflow_run` job that checks out and runs nothing from the repository),
`pr-title.yml` (Conventional Commits title),
`issue-labels.yml` (the issue forms' `platform/*` and `area/*` labels, with
`cmd/issuelabels`),
`live.yml` (weekly Apple Music contract tests, private mode),
`release-please.yml` (the release PR and tag), `release.yml`
(builds, smoke-tests, attests and publishes the Linux, macOS and Windows
releases; nightly, the rc of main as a prerelease; dispatch = dry run, a
prerelease with `-f prerelease=rc.1`, or the nightly now with `-f nightly=true`),
`release-secrets.yml` (maintainers: checks the release secrets resolve),
`renovate.yaml` (dependency updates; maintained with Renovate).

**The CI image.** The Linux vet, type-check, core, UI and end-to-end jobs (and
the dispatch-only probe and Orca walk) run
in `ghcr.io/coredev-uk/sona-ci` (`build/ci/Containerfile`: Ubuntu 26.04, the
GTK, GStreamer, sway, AT-SPI, PipeWire and font packages, Go and gotestsum at
`go.mod`'s versions; amd64 and arm64), so they start with the toolchain ready
and the system libraries, which the UI goldens depend on, change only when the
image is rebuilt. `images.yml` builds it when `build/ci/**` or `go.mod`
changes (on a pull request of this repository too: the CI job named "CI image"
waits for it), weekly and on dispatch; `build/ci/tag.sh` names the tag from the
Containerfile and the Go and gotestsum versions. A run pins the digest it finds
and keys Go's caches by it. Each kind of job (vet, e2e, one shard of a test pool with or without `-race`) has its own Go cache, saved only by a run on main, and `cache-prune.yml` keeps the newest of each: 7.5 GB of the repository's 10 GB in 34 entries after a main run (measured 2026-11-05: the macOS `-race` caches 1.7 GB, the UI shards' 1.8 GB, macOS's GStreamer installer 1.0 GB, the rest under 0.6 GB each); the pull-request jobs restore them and save nothing. The playback job stays on the bare runner (it
lifts the user-namespace restriction of the host). To rebuild it by hand:
`podman build $(build/ci/tag.sh args) -f build/ci/Containerfile build/ci`.

Rules for workflow changes: trigger pull requests with `pull_request`, never
`pull_request_target` or `workflow_run` (`cache-prune.yml` is the one
`workflow_run`: it reacts to pushes to main only and runs no repository code); start from `permissions: contents:
read` and widen per job only where needed; pin every action to a full commit
SHA with the version in a comment (`uses: actions/checkout@<sha> # v7.0.1`;
Renovate keeps pins current); `persist-credentials: false` on every checkout
that doesn't push; pass expressions to scripts through `env:`, never inline
in `run:`; the secrets are in environments that only `main` (and release
tags) deploy to, so no pull request run can read them
(`docs/maintainers/ci-and-secrets.md`). Run `actionlint` on changed
workflows. The test jobs run `.github/ci/test.sh` (gotestsum: a failing test
is rerun twice; one that passes on a rerun passes the job and is listed in the
job summary as flaky) and then `.github/ci/skips.sh` (the `finish-tests`
action, which also keeps the results), which fails the job on
any skipped test that `.github/ci/skips-linux.txt` (or `skips-darwin.txt`)
does not name: a skip means a missing prerequisite or a test nobody runs, so a
test that cannot run on a platform is added to the list with its reason, and
anything else is fixed in the job's setup. Go's caches are restored by the key
they were saved under (the CI image's digest, or the runner image's version on
the bare runner, is part of the Linux key, so an image update never serves a
cached pass), and `test.sh` leaves out
`-artifacts` so unchanged packages report `(cached)`.
