# CI and secrets

How the GitHub Actions workflows protect Sona's credentials, where those credentials live and how to rotate them. For maintainers; the contributor rules for workflow changes are in `docs/maintainers/ci-workflows.md`, "CI workflows", and the release steps in [release.md](release.md).

The repository's GitHub Actions run on GitHub-hosted runners only (a self-hosted runner reachable from a pull request was ruled out). Once the repository is public, anyone can open a pull request, so the workflows assume the pull request's code is hostile.

## Where the secrets live

All CI secrets but the bot-cecil App's are Proton Pass items in the vault "CI":

| Item | Fields |
| --- | --- |
| "Update Signing Key" | `SONA_UPDATE_KEY`, the Ed25519 key `releasesign` signs the update manifests with. The master copy is also in Development → Sona Update Signing Key (the file `go tool mygo keygen` wrote); the public key is `internal/update/update.pub`. Never print or commit it. |
| "Last.fm API" | `SONA_LASTFM_KEY`, `SONA_LASTFM_SECRET` |
| "Discord App" | `SONA_DISCORD_CLIENT_ID` |
| "Apple Music User Token" | `MUSIC_USER_TOKEN`, for the live checks |

Item titles are matched exactly, case included.

The `release` and `live` environments each hold one secret, `PROTON_PASS_PERSONAL_ACCESS_TOKEN`: the same token, `sona-ci`, viewer on that vault alone, which `protonpass/load-secret-action` uses to read the items. A pull request's run can't read it.

The bot-cecil App's credentials are not in Proton Pass: their master copy is the 1Password item "bot-cecil" in the vault "Kubernetes" (the home-ops repository's service account reads it), fields `GITHUB_BOT_APP_CLIENT_ID` and `GITHUB_BOT_APP_PRIVATE_KEY`. They are copied into the four environments below as `RENOVATE_APP_CLIENT_ID` and `RENOVATE_APP_PRIVATE_KEY`, and into no repository-level secret. To rotate the key, generate a new one in the App's settings, update the item, set it in each environment (`op item get … | gh secret set RENOVATE_APP_PRIVATE_KEY --env <name>`, never printing it) and then delete the old key from the App.

- **Rotate a value:** edit the item in Proton Pass; nothing changes in GitHub.
- **Renew the token** (it expires 2027-11-14): run `pass-cli pat renew --pat-name sona-ci --expiration 1y`. It keeps its access to "CI", prints a new token string and stops the old one at once, so set the new one straight away with `gh secret set PROTON_PASS_PERSONAL_ACCESS_TOKEN --env release` and `--env live`.
- **Check it:** the monthly "Release secrets" run (`release-secrets.yml`) catches an expired or revoked token before a release. Dispatch it after any change.

`release-please.yml` calls `release.yml` with `secrets: inherit`, since a called workflow sees its environment's secret (the Proton Pass token) only that way. With the App secrets passed by name, v0.3.0 shipped without the credentials or manifests and was replaced by v0.3.1.

**Without the update key** (a fork, a run outside the environment) the manifest-signing step is skipped and says so in the run summary. The release then has no `update-<target>.json`, so macOS and Windows installs don't self-update. In this repository `publish` refuses instead: it checks that `SONA_UPDATE_KEY` and the Last.fm and Discord credentials are all there before it signs or publishes.

## What there is to protect

| Asset | Where it lives | Who can read it |
| --- | --- | --- |
| `MUSIC_USER_TOKEN` (a real account's Music User Token, about six months' access to its library and playback) | Proton Pass item "Apple Music User Token" in the vault "CI"; the `live` environment holds the vault token | Jobs that name `environment: live` and run from `main`: `live.yml` (weekly schedule, dispatch), `windows-playback.yml` (weekly schedule, dispatch) |
| Last.fm API key and secret, Discord application ID (compiled into every release binary, so readable by anyone who has one; the risk is someone else's builds or abuse posing as Sona before a release), and the Ed25519 update key | Proton Pass items in the vault "CI"; the `release` environment holds the vault token | Jobs that name `environment: release` and run from `main` or a `v*` tag: `release.yml`'s build job (not a dispatched dry run, which has no environment), `release-secrets.yml` (monthly, dispatch, prints only whether each is set) |
| bot-cecil GitHub App credentials (`RENOVATE_APP_CLIENT_ID`, `RENOVATE_APP_PRIVATE_KEY`; the App's token is how release-please and the release publish step write, as Renovate does) | Environment secrets: `bot-cecil` (branch policy: `main` and `v*` tags), `release` (branch policy: `main`), `golden-update` and `nix-vendor-hash` (required reviewers) | Jobs that name the environment: `renovate.yaml`, `release-please.yml`, `nix-branch.yml`'s push job (`bot-cecil`); the `publish` job of `release.yml` (`release`); `golden-update.yml`'s push job and `nix-vendor-hash.yml`'s fix job (a reviewer approves each run, which runs on a pull request's ref and so can't satisfy a branch policy). Each mints a job-scoped token (contents, pull requests, issues write for release-please; contents write to publish) |
| `GITHUB_TOKEN` write scopes | Per job | `live` (`issues: write`, to report a failed scheduled run), `issue-labels.yml` (`issues: write`, to add labels), `release.yml`'s build job (`id-token` and `attestations: write`, for the build provenance); every other job reads only; releases are written with the App token |
| Release assets and tags | GitHub releases | The `publish` job, after the CI checks and the build; the `release tags` ruleset keeps `v*` tags immutable |

## bot-cecil App permissions

release-please and the publish job act as the bot-cecil GitHub App (the same
App Renovate uses), through a token minted by
`actions/create-github-app-token` in each job from the secrets
`RENOVATE_APP_CLIENT_ID` and `RENOVATE_APP_PRIVATE_KEY` of the job's environment (none is a repository-level secret, which any pull request run from a branch of this repository could read). The App must be
installed on this repository with at least these repository permissions, all
of which Renovate's token (`renovate.yaml`) already requests:

| Permission     | Level | Used by                                                            |
| -------------- | ----- | ------------------------------------------------------------------ |
| Contents       | write | release-please: release branch, tag, draft release; publish job: upload and publish; golden-update.yml: push the regenerated golden images to a pull request's branch; nix-vendor-hash.yml: push the updated vendorHash to a pull request's branch; nix-branch.yml: push the flake's `nix` branch |
| Pull requests  | write | release-please: open and update the release PR; golden-update.yml: remove the `update-goldens` label (with Issues: a pull request's label needs both) |
| Issues         | write | release-please: the `autorelease:` labels on the release PR; golden-update.yml: remove the `update-goldens` label |
| Workflows      | write | nix-branch.yml: merging main onto the `nix` branch brings main's workflow changes with it, and GitHub refuses an App's push of those without this |

Each job requests only what it uses (`permission-*` inputs), so the token is
narrower than the installation. The same two secrets are in each of four
environments (Settings → Environments): `bot-cecil` (deploy branches: `main`
and `v*` tags), `release` (`main`), and `golden-update` and `nix-vendor-hash`
(required reviewers: the jobs run on a pull request's ref, which no branch
policy lets in, so a reviewer approves each run instead). The build provenance needs no App
permission: it uses the workflow's `id-token: write` and
`attestations: write`.

## Workflows that commit or label

Each runs the branch's code without secrets; the job that writes runs none of it.

- **`golden-update.yml`:** the `update-goldens` label on a pull request of this repository regenerates the golden images of the failing golden tests in the CI image, commits them to the branch as bot-cecil and removes the label. It needs the `golden-update` environment (required reviewers) and its App secrets.
- **`nix-vendor-hash.yml`:** builds the Go modules of `build/linux/nix/package.nix` on pushes to `main` and on pull requests that touch `go.mod`, `go.sum`, the flake or the package. On a mismatch in a pull request of this repository (a Renovate bump, say) it commits the hash Nix reported to the branch as bot-cecil, once, after a reviewer approves the `nix-vendor-hash` environment.
- **`nix-branch.yml`:** on every published release and prerelease, each night and on dispatch, it keeps the flake's `nix` branch (`main` plus `build/linux/nix/pins.json`) on the latest release and the newest published build, with no pull request. It checks both Arch packages against their `SHA256SUMS` and attestations, merges `main` into the branch, builds the flake's `sona` and `sona-nightly` with the new pins and pushes the result as bot-cecil, never force-pushing. It pushes only the tree the build checked.
- **`issue-labels.yml`:** on an issue opened or edited, adds the `platform/*` and `area/*` labels its form's answers name, with `cmd/issuelabels`. It only adds, never over a label of that kind already there, so a maintainer's triage stands (`issues: write` only).
- **Labels owned by bots:** `state/*`, `verdict/*`, `kind/*`, `type/*`, `clarence:*`, `autorelease:*`, `dependencies`, `go` and `update-goldens`. See [clarence.md](clarence.md).

## Threats and controls

- *A fork's pull request runs its own code in CI.* Every workflow a pull request triggers uses `pull_request`, never `pull_request_target` or `workflow_run`, so a fork's run gets a read-only `GITHUB_TOKEN` and no secrets; GitHub withholds both. None of those workflows (`ci.yml`, `pr-title.yml`) reads a secret other than `GITHUB_TOKEN`. Environment secrets stay out of reach even for a pull request from a branch of this repository: an environment deploys only to the refs in its policy (`bot-cecil`: `main` and `v*` tags; `release` and `live`: `main`), and a pull request's ref is `refs/pull/<n>/merge`. The two jobs that must run on a pull request's ref and need bot-cecil's key (`golden-update.yml`, `nix-vendor-hash.yml`'s fix job) name an environment with required reviewers, so a person approves each run. Nothing holds the App's key as a repository-level secret.
- *A token that outlives its job.* Every checkout sets `persist-credentials: false`, so the token isn't left in `.git/config` for later steps (tests, build scripts) to read. No workflow pushes with git.
- *Too much token scope.* Each workflow starts from `permissions: contents: read` (`pr-title.yml`: `pull-requests: read`; `release-secrets.yml`: none), and only a few jobs widen it (`release.yml`'s build job for the provenance, `live`, and `images.yml`'s CI image jobs, which write packages: a pull request's build goes to `pr-<tag>` and gets no provenance, and the runs that matter verify the main-built one, "Where the images come from"). The release PR, tag and release are written with a job-scoped bot-cecil App token instead. The repository's default workflow permission is read.
- *A third-party action moves its tag.* Every action is pinned to a full commit SHA, with the version in a comment; Renovate proposes updates to the pins, which are reviewed like any other pull request.
- *Script injection.* No `run:` step expands an attacker-controlled expression (pull request title, branch name, commit message, issue body); values reach scripts through `env:`. `issue-labels.yml` feeds an issue's body to `cmd/issuelabels`, which prints only labels from a fixed table, so what an issue says can't choose any other label or command.
- *A malicious dependency in CI.* Go modules are checked against `go.sum`. The release build runs a few third-party tools, each fixed to a version and checked against a hash kept in the repository before it runs: GStreamer (`build/windows/gstreamer.sha256`, `build/darwin/gstreamer.sha256`), linuxdeploy, its AppImage plugin and its GTK plugin (`build/linux/appimage-tools.sha256`, checked by `appimage.sh`), and NSIS and pkg-config-lite, Chocolatey packages at a pinned version whose installed files `build/windows/install-tool.sh` checks against `build/windows/tools.sha256`. A package manager's install script still runs for those two, so a version bump changes the hash in the same pull request. A compromised dependency still runs its code in a pull request's job, which holds nothing worth taking (above).
- *Where the images come from.* `images.yml` runs the branch's own workflow on a pull request, so its package write can't be limited by ref in the workflow. What is limited is what other runs trust: main, the nightly and the release run the CI image only when `build/ci/find-image.sh` verifies a build provenance attestation from `images.yml` on `refs/heads/main` (a pull request's run can't produce one), and the release job builds its Linux image from the checked-out Containerfile instead of pulling one.
- *Cache poisoning.* Caches written by a pull request's run are scoped to its ref; the default branch's caches, which every run can restore, are written only by runs on `main`. The cached Widevine CDM is reinstalled through Sona's verifying downloader (`cmd/cdmfetch`, "Widevine CDM downloader").
- *The live token in a test.* `live.yml` runs with `SONA_PRIVATE=1`, so nothing it does reaches the account's listening history, and plays nothing. `windows-playback.yml` plays one track through the CDM host on a Windows runner, also with `SONA_PRIVATE=1` and `playspike`, which reports nothing to Apple's play-activity service; it builds `main` only (never a branch the dispatcher names), so the secret only meets reviewed code, and prints the licence verdict, never the token or the licence.
- *Hosted-runner cost.* The macOS and Windows jobs in `ci.yml` run only when dispatched while the repository is private, and on every push and pull request once it is public.

## Residual risks

- Anyone with write access can push a `v*` tag, and with it a workflow that names `environment: release` or `bot-cecil`. A tag ruleset that limits who may create `v*` tags closes that; once releases run from `main` alone, the `v*` rule can leave the environments' policies.
- While the repository is private, forks' pull requests run no workflows at all ("Run workflows from fork pull requests" is off, and so are write tokens and secrets for them). Once it is public, a first-time contributor's pull request waits for a maintainer's approval before CI runs ("Require approval for outside contributors"); the controls above are what holds after that approval.
