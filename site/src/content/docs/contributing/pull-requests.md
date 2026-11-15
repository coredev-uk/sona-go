---
title: Pull Requests
description: Branch, title and describe a pull request, how it gets merged, and the labels you will see.
---

One change per branch and pull request. Update the docs that describe your change in the same pull request.

## Open a pull request

1. Work in a [worktree on a branch](/contributing/setup/#get-the-code), never on `main`.
2. Run `check` and `tests` (see [Test Your Change](/contributing/test/)).
3. After rebasing onto `main`, regenerate what is generated and commit the result: `devenv shell -- gen` for `internal/db/dbq`, and `go run ./cmd/i18ntool extract` for the catalogues. Never edit generated files by hand.
4. Push your branch and open the pull request with a Conventional Commit title and the template.

## Write the title

Commits and pull request titles follow [Conventional Commits](https://www.conventionalcommits.org): `type(scope): summary`, with an optional scope.

| Type | Use for | In the changelog |
| --- | --- | --- |
| `feat` | A new feature | Features |
| `fix` | A bug fix | Bug Fixes |
| `perf` | A speed-up | Performance |
| `revert` | Undoing a change | Reverts |
| `refactor`, `docs`, `test`, `ci`, `build`, `chore` | Everything else | Not listed |

Mark a breaking change with `!` (`feat!: ...`) or a `BREAKING CHANGE:` footer. CI fails a title that doesn't conform. Pull requests are squash-merged, so the title becomes the commit and drives the release notes. Never edit `VERSION` or `CHANGELOG.md` by hand.

## Fill in the template

The template (`.github/pull_request_template.md`) has these sections:

- **What**: what changed, in a few lines.
- **Why**: the problem, with a link to its issue.
- **Evidence**: the commands you ran and the tail of their output. For UI changes, screenshots in light and dark, with no personal account data, tokens or library names.
- **Tests**: tests added or changed.
- **Docs**: docs you updated, or why none apply.
- **Left**: anything not done or not reachable. "Nothing" is fine.

## Get it merged

Clarence, the review bot (`bot-clarence`), merges pull requests. It posts one card on yours with a verdict, the checks and a timeline, and keeps it current.

- Don't merge with `gh pr merge` or the button. Once CI is green, the maintainer comments `@bot-clarence merge`.
- Required checks: "Go vet and test (-race)", "Conventional Commits title" and "gitleaks". Nobody pushes to `main`; a pull request is the only way in.
- A first pull request from a new contributor waits for the maintainer to approve its CI run.
- A pull request Clarence marks `verdict/review` needs the maintainer's call.

Bot commands and labels are in [docs/maintainers/clarence.md](https://github.com/coredev-uk/sona-go/blob/main/docs/maintainers/clarence.md).

## Report a problem

Use the issue form that matches. For anything larger than a fix, open an issue before writing code, so the design can be agreed first.

The bug form asks for your **Platform** (Linux, macOS or Windows), how you installed Sona, and the closest **Area** ("Not sure" is fine). Paste the diagnostics bundle (Settings → Diagnostics → Copy diagnostics: versions, devices and recent log lines, with secrets redacted; read it before posting). Tick that you searched existing issues, are on the latest release and removed personal data, and say how often it happens. The feature request form takes the same Area.

Report vulnerabilities privately, following [SECURITY.md](https://github.com/coredev-uk/sona-go/blob/main/SECURITY.md).

## Labels

The forms add `platform/*` and `area/*` labels from your answers. A change to the form's dropdowns goes in `cmd/issuelabels`' tables too; its test fails when they differ.

| Label | Means |
| --- | --- |
| `bug`, `enhancement`, `question`, `documentation` | The kind of issue |
| `needs-triage` | Nobody has looked yet |
| `decision` | A design choice to settle before any code |
| `needs-hardware` | Reproducing or fixing it needs a device or setup we don't have |
| `upstream` | The cause is in Apple's service, Google's CDM or a dependency |
| `platform/linux`, `platform/macos`, `platform/windows` | Where it happens |
| `area/playback` | Playback, the queue, audio output |
| `area/library` | Songs, albums, artists, playlists in your library |
| `area/search` | Search |
| `area/ui` | Pages, menus, windows, how Sona looks |
| `area/themes` | Themes |
| `area/add-ons` | Add-ons (plugins) |
| `area/updates` | Self-update and the release packages |
| `area/sign-in` | Signing in to Apple Music |
| `area/integrations` | Last.fm, Discord, media keys, the desktop |
| `area/accessibility` | Screen reader, keyboard, contrast |

The bots manage `state/*`, `verdict/*`, `kind/*`, `type/*` and `clarence:*` labels. Leave them alone.
