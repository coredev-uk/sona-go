# Repository, releases and dependencies

Branch protection, how releases are cut by automation, how self-updates are built, and how dependencies are pinned and updated. The release checklist is `release.md`.

## Branch protection

- `main` is protected by a ruleset: changes land only through a pull request,
  squash-merged (linear history), with these checks green: "Go vet and test
  (-race)", "Conventional
  Commits title" and "gitleaks". Nobody bypasses it but the review bot, and
  only through a pull request. Force pushes and deleting `main` are
  blocked. Release tags (`v*`) can't be moved or deleted. A first pull
  request from someone outside the project waits for the maintainer to
  approve its CI run. Report vulnerabilities privately (SECURITY.md).

## Release automation

Releases are automated with release-please
(`.github/workflows/release-please.yml`, config in
`release-please-config.json`). Your PR title is what it reads: PRs are
squash-merged, so the title becomes the commit subject. `feat` titles land
under Features, `fix` under Bug Fixes, `perf` under Performance and `revert`
under Reverts in `CHANGELOG.md`; `refactor`, `docs`, `test`, `ci`, `build`
and `chore` stay out of the changelog. Mark a breaking change with `!`
(`feat!: …`) or a `BREAKING CHANGE:` footer. release-please keeps one
release PR open that bumps `VERSION` and `CHANGELOG.md`; a maintainer merges
it, which tags `v<VERSION>` and runs `release.yml`: the Linux packages
(.deb, .rpm, Arch, AppImage), the macOS app for Apple silicon (`.dmg`,
GStreamer bundled, ad-hoc signed; `build/darwin`) and the Windows x64
installer and portable zip (`build/windows`), each smoke-tested, then the
signed update manifests, one `SHA256SUMS` and build attestations, published
with the release. Never edit `VERSION` or `CHANGELOG.md` by hand. Changing the packaging or
`release.yml`? Dispatch `release.yml` on your branch (`gh workflow run
release.yml --ref <branch>`): it builds and smoke-tests every platform and
publishes nothing (`-f prerelease=rc.1` publishes a test prerelease instead).
The release steps are maintainer documentation: `docs/maintainers/release.md`.

## Self-updates (macOS and Windows)

`internal/update` and `UpdatesService` update a macOS `.app` or a Windows
folder in place (design: `docs/architecture.md`, "Updates"). What a release
needs:

- The public key is `internal/update/update.pub` (embedded; empty means
  updates are off). The private key is the maintainer's, from `go tool mygo
  keygen` (`docs/maintainers/ci-and-secrets.md`); losing it strands the
  installs that exist. Never print it or commit it.
- Release builds link MyGo's updater with `-ldflags "-X
  github.com/egoist/mygo.packageUpdateFeed=https://github.com/coredev-uk/sona-go/releases/latest/download/update-<target>.json
  -X github.com/egoist/mygo.packageUpdateKey=$(cat internal/update/update.pub)"`:
  `build/darwin/bundle.sh` and `build/windows/package.sh` get both from
  `build/ldflags.sh --update <target>` in CI when `update.pub` is not empty
  (or from `UPDATE_FEED` and `UPDATE_KEY`). Other builds leave both unset.
- The update archive is a gzip tar (`sona-<version>-darwin-arm64.tar.gz` with
  one top-level `Sona.app`; `sona-<version>-windows-amd64.tar.gz` with the
  folder's files at top level, made by `build/windows/update-archive.sh`:
  never `tar -czf OUT .`, whose `./` entry MyGo's extractor refuses, which
  made every Windows update fail). `update.CheckArchive` (`go run
  ./cmd/releasesign -check -target T -archive FILE`) applies MyGo's
  extraction rules and Sona's layout; `package.sh` runs it on what it made
  and `releasesign` runs it before it signs. `go run ./cmd/releasesign
  -target T -archive FILE -version V -url URL [-notes FILE] -out
  update-T.json` signs it (key from `$SONA_UPDATE_KEY`): the digest, as MyGo
  checks it, and a statement of target, version, channel and digest that
  Sona checks (`docs/architecture.md`, "Updates"); the publish job signs
  last, after the archives are uploaded.
- A prerelease's notes (and its manifests' notes) are `go run ./cmd/rcnotes
  -exclude <tag>`: the changes since the previous `v*` tag, by Conventional
  Commit type (`docs/maintainers/release.md`, "Releasing": the Prerelease notes item).
- Tests run against an `httptest` fake release server (`internal/update`,
  `internal/services/updates_self_test.go`): the check, a bad signature
  refused, a replayed older manifest, another platform's, a tampered
  version or digest and a manifest without a statement refused, no key
  (off), a development build never checking, the rollback
  decision, a failed install leaving the kept version as it was, and the
  archive rules (`archive_test.go`, pinned to what MyGo's extractor does with
  the same entries; the Windows archive script is run on a folder and its
  output checked). The real run (install, relaunch, rollback) is the phase 5
  update and rollback runs on a Mac and a Windows machine.

## Pinned inputs

`devenv.yaml` pins nixpkgs to one nixos-unstable revision and devenv's
modules to one revision, so every machine gets the same toolchain. To bump
nixpkgs, put the current nixos-unstable head
(`git ls-remote https://github.com/NixOS/nixpkgs refs/heads/nixos-unstable`)
in `devenv.yaml` and in `flake.nix` (the flake's packages build with the same
nixpkgs), run `devenv update` and `nix flake update`, then `check`, `tests`,
`play-spike`, `nix build .#sona-source` and the app, and commit
`devenv.yaml`, `devenv.lock`, `flake.nix` and `flake.lock` together.

Go is the one exception: `build/linux/nix/go.nix` builds Go 1.27.4 from its
source tarball on the pinned nixpkgs, because nixos-unstable still ships 1.27.3
(GO-2026-6859 needs 1.27.4; it is in staging-next only). `devenv.nix` and the
Nix package both use it. When the nixpkgs you bump to has `go_1_27` at 1.27.4
or newer, delete `go.nix` and go back to `pkgs.go_1_27`. A Go release bump
changes `go.mod`, `build/ci/go.sha256`, `build/linux/release/Containerfile`
(`GO_VERSION` and `GO_SHA256`), `go.nix`'s version and hash (all from
https://go.dev/dl/?mode=json) and `docs/architecture.md`.

golangci-lint is the other tool devenv and CI install separately: devenv from the
pinned nixpkgs, the CI image from the release archive named in
`build/ci/golangci-lint.sha256` (checked against its hash). They must be one
version, so a nixpkgs bump that changes `devenv shell -- golangci-lint --version`
changes that file (the version in its file names and both hashes, from the
release's `golangci-lint-<version>-checksums.txt`) in the same pull request.
gofumpt is not in nixpkgs' list: `go.mod` pins it (`go tool gofumpt`).

## Dependency updates

Renovate opens pull requests for Go modules, GitHub
Actions, the docs site's npm packages (`site/`, grouped into one weekly pull
request; its `overrides` in `site/package.json` pin transitive packages past
a vulnerable range, so drop one once the package that needs it has moved on) and the digests of the container base images (the `FROM` lines of
`build/ci/Containerfile` and `build/linux/release/Containerfile`, the Flatpak
workflow's `container:` image; a tag is never moved by Renovate). MyGo's are
always left for review, and so is the release image's digest. A review bot
checks and merges them; the pinned devenv inputs
are bumped by hand (see "Pinned inputs" above), and so are the downloaded
tools' hashes (`build/linux/appimage-tools.sha256`, `build/windows/*.sha256`,
`build/darwin/dmgbuild.sha256`, the Go archive hashes, `build/ci/golangci-lint.sha256`): Renovate cannot compute a hash. You
don't need to update dependencies yourself; a change that needs a newer
dependency bumps it in the same PR.
