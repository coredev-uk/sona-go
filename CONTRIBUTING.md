# Contributing to Sona

Thanks for taking the time to contribute. Everyone taking part follows the [Code of Conduct](CODE_OF_CONDUCT.md).

## Contents

- [Quick start](#quick-start)
- [Issues](#issues)
- [Pull requests](#pull-requests)
- [Style guides](#style-guides)
- [AI tools](#ai-tools)
- [Further reading](#further-reading)
- [Licence](#licence)

## Quick start

You need [Nix](https://nixos.org/download) and [devenv](https://devenv.sh) 2.4 or newer. [direnv](https://direnv.net) is optional.

```sh
git clone https://github.com/coredev-uk/sona-go.git
cd sona-go
devenv shell        # the first run downloads the toolchain
dev                 # build and run Sona, rebuilding on every change
check               # vet, type-checks, lint
tests               # go test -race
```

Outside the shell, run any of these as `devenv shell -- <command>`. Everything runs in devenv's pinned toolchain.

Development tools: a development build (`dev` makes one) adds **Inspect** to every item's right-click menu, which shows the item's IDs, artwork and what Apple sent, from Go's cache (see [Debugging the UI](docs/development.md#debugging-the-ui)); `SONA_INSPECTOR=1 dev` opens MyGo's element inspector. Release builds have neither the action nor its code.

## Issues

Use the matching [issue form](https://github.com/coredev-uk/sona-go/issues/new/choose):

- **Bug report.** Say your platform, how you installed Sona and the closest area, and paste the diagnostics bundle.
- **Feature request.** For anything larger than a fix, open an issue before writing code, so the design can be agreed first.
- **Questions and ideas.** Use [Discussions](https://github.com/coredev-uk/sona-go/discussions): [Q&A](https://github.com/coredev-uk/sona-go/discussions/categories/q-a) for how-to questions, [Ideas](https://github.com/coredev-uk/sona-go/discussions/categories/ideas) when it isn't yet a feature request.
- **Security problems.** Report them privately through [SECURITY.md](SECURITY.md), never in a public issue.

[Report a problem](https://sona.coredev.uk/contributing/pull-requests/#report-a-problem) explains the form, and [Labels](https://sona.coredev.uk/contributing/pull-requests/#labels) lists the labels you will see.

## Pull requests

One change per branch and pull request. Each step links to its page on the site.

1. [Set up](https://sona.coredev.uk/contributing/setup/)
   - [Get the code](https://sona.coredev.uk/contributing/setup/#get-the-code)
   - [Enter the environment](https://sona.coredev.uk/contributing/setup/#enter-the-environment)
   - [Add your token](https://sona.coredev.uk/contributing/setup/#add-your-token)
2. [Branch in a worktree](https://sona.coredev.uk/contributing/setup/#get-the-code): `git worktree add ../sona-go-<slug> -b <type>/<slug> origin/main`
3. [Build and run](https://sona.coredev.uk/contributing/build-and-run/)
   - [Run with live reload](https://sona.coredev.uk/contributing/build-and-run/#run-with-live-reload)
   - [Play without an account](https://sona.coredev.uk/contributing/build-and-run/#play-without-an-account)
   - [Use private mode](https://sona.coredev.uk/contributing/build-and-run/#use-private-mode)
4. [Make the change](https://sona.coredev.uk/contributing/architecture/), with its tests and docs in the same pull request
   - [Architecture overview](https://sona.coredev.uk/contributing/architecture/)
   - [Add a feature](https://sona.coredev.uk/contributing/add-a-feature/)
   - [Translations](https://sona.coredev.uk/contributing/translations/)
5. [Test](https://sona.coredev.uk/contributing/test/)
   - [Run the checks](https://sona.coredev.uk/contributing/test/#run-the-checks)
   - [Run the tests](https://sona.coredev.uk/contributing/test/#run-the-tests)
   - [Update golden images](https://sona.coredev.uk/contributing/test/#update-golden-images)
6. [Write the commit message](https://sona.coredev.uk/contributing/pull-requests/#write-the-title)
7. [Open the pull request](https://sona.coredev.uk/contributing/pull-requests/#open-a-pull-request)
   - [Fill in the template](https://sona.coredev.uk/contributing/pull-requests/#fill-in-the-template)
8. [Review and merge](https://sona.coredev.uk/contributing/pull-requests/#get-it-merged): the maintainer merges it through the review bot once CI is green. Don't merge it yourself.

## Style guides

- **Go.** Format with `fmt` (gofumpt). `check` runs golangci-lint on new code and must pass clean.
- **UI.** [UI conventions](https://sona.coredev.uk/contributing/ui-conventions/): modules, kit, tokens, performance and accessibility.
- **Docs.** [Writing docs](https://sona.coredev.uk/contributing/documentation/): lead with the task, keep it short.
- **Commits.** [Conventional Commits](https://www.conventionalcommits.org) for every commit and pull request title: `feat`, `fix`, `perf`, `refactor`, `docs`, `test`, `ci`, `build`, `chore` or `revert`.

## AI tools

You may use AI tools. A human reviews every change and can explain it, and you own what you submit. Agents follow the rules in [AGENTS.md](AGENTS.md), which apply to people too.

## Further reading

| Read | For |
| --- | --- |
| [docs/architecture.md](docs/architecture.md) | How Sona is built, and why |
| [docs/ui-architecture.md](docs/ui-architecture.md) | The UI's modules, store, kit and tokens |
| [docs/design.md](docs/design.md) | The visual direction |
| [docs/accessibility.md](docs/accessibility.md) | Keyboard and assistive-technology rules, and how they are checked |
| [docs/development.md](docs/development.md) | The environment, every devenv script, live playback, the Widevine CDM |
| [docs/testing.md](docs/testing.md) | Checks, preview playback, sandbox and packaging tests, goldens, end-to-end, VM tests |
| [docs/performance.md](docs/performance.md) | Measuring frame cost, start-up and memory |
| [docs/platforms.md](docs/platforms.md) | Linux, macOS and Windows: seams, build tags, the title bar, macOS setup |
| [docs/translations.md](docs/translations.md) | The string catalogues and Apple's translations |
| [docs/auth.md](docs/auth.md) | Sign-in |
| [docs/apple-music-reference.md](docs/apple-music-reference.md) | The Apple Music endpoints Sona uses |
| [docs/security-review.md](docs/security-review.md) | The threat model |
| [docs/maintainers/](docs/maintainers/) | How the project is run: the merge bot, CI, releases, branch protection, dependencies |

## Licence

By contributing you agree your contribution is licensed under the AGPL-3.0 (`LICENSE`). Code copied from elsewhere must carry a compatible licence and its notice, and gets a row in `THIRD_PARTY_NOTICES.md`; the vendored Chromium CDM headers keep theirs (`internal/playback/widevine/LICENSE.chromium`, BSD-3-Clause).
