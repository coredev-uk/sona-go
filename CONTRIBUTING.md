# Contributing to Sona

Thanks for helping. Sona is a native Apple Music client for Linux, macOS and Windows. It wants fast, correct, keyboard-friendly changes that follow [the architecture](docs/architecture.md).

For anything larger than a fix, open an issue first so the design can be agreed before you write code. Report bugs and ideas with the matching issue form. Report security problems through [SECURITY.md](SECURITY.md), never a public issue.

## The path

1. **Fork, then work in a worktree.** Keep your checkout on `main` and give each change its own worktree and branch: `git worktree add ../sona-go-<slug> -b <type>/<slug> origin/main`.
2. **Enter the environment.** Everything runs through [devenv](https://devenv.sh) 2.4 or newer: `devenv shell -- <command>`, or `direnv allow` once per checkout.
3. **Make the change,** with the tests and docs that describe it in the same pull request.
4. **Check and test.** Run `devenv shell -- check` and `devenv shell -- tests`.
5. **Open a pull request** with a [Conventional Commit](https://www.conventionalcommits.org) title and the template. The maintainer merges it through the review bot once CI is green; don't merge it yourself.

## Where to read next

The [contributing pages](https://sona.coredev.uk/contributing/setup/) explain each step:

- [Set up your environment](https://sona.coredev.uk/contributing/setup/)
- [Build and run](https://sona.coredev.uk/contributing/build-and-run/)
- [Test your change](https://sona.coredev.uk/contributing/test/)
- [Architecture overview](https://sona.coredev.uk/contributing/architecture/)
- [UI conventions](https://sona.coredev.uk/contributing/ui-conventions/)
- [Add a feature](https://sona.coredev.uk/contributing/add-a-feature/)
- [Translations](https://sona.coredev.uk/contributing/translations/)
- [Pull requests](https://sona.coredev.uk/contributing/pull-requests/), with the issue labels
- [Writing docs](https://sona.coredev.uk/contributing/documentation/)

For depth, `docs/` holds the full references:

- [architecture.md](docs/architecture.md), [ui-architecture.md](docs/ui-architecture.md), [design.md](docs/design.md) and [accessibility.md](docs/accessibility.md)
- [development.md](docs/development.md): the environment, every devenv script, live playback and the Widevine CDM
- [testing.md](docs/testing.md), [performance.md](docs/performance.md), [platforms.md](docs/platforms.md) and [translations.md](docs/translations.md)
- [auth.md](docs/auth.md), [apple-music-reference.md](docs/apple-music-reference.md) and [security-review.md](docs/security-review.md)

[docs/maintainers/](docs/maintainers/) covers how the project is run: the merge bot, CI, releases, branch protection and dependencies. [AGENTS.md](AGENTS.md) holds the working rules for agents, which apply to people too.

## Code of conduct

Everyone taking part follows the [Code of Conduct](CODE_OF_CONDUCT.md).

## Licence

By contributing you agree your contribution is licensed under the AGPL-3.0 (`LICENSE`). Code copied from elsewhere must carry a compatible licence and its notice, and gets a row in `THIRD_PARTY_NOTICES.md`; the vendored Chromium CDM headers keep theirs (`internal/playback/widevine/LICENSE.chromium`, BSD-3-Clause).
