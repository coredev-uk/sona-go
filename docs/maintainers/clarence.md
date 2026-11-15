# Clarence

Pull requests are merged by Clarence (GitHub App `bot-clarence`), not by hand. The short rules for agents are in `AGENTS.md`; this is the full reference.

## Flow

- Open the pull request with a Conventional Commit title. Clarence posts one card on it (verdict, checks, what it changes, timeline) and keeps it current.
- Never merge with `gh pr merge` or the GitHub button. Once CI is green and the work is done, comment `@bot-clarence merge` (or tick 🎯 Merge on the card). Clarence merges one pull request at a time, then checks the merge (CI on `main`, or the deploy) and opens a revert if it fails.
- When Clarence needs a human it requests a review from @coredev-uk. Don't approve or merge on the maintainer's behalf, and don't merge anything Clarence ruled `verdict/review` without his say.

## Commands

A comment line starting with the command; only `coredev-uk` may issue them, except `help` (anyone).

- `@bot-clarence merge` (options `--allow <check>`, `--batch`, `at HH:MM`, `after #N`), `cancel`, `hold`, `unhold`, `reclassify`, `retry` (re-run what failed on the head), `status`, `explain`, `revert` (on a merged pull request), `help`.
- Clarence answers with a reaction: 👀 queued, 👍 done, 🚀 merged, 👎 refused or called off, 😕 not understood or failed; the card's timeline says why. Only `help`, `status` and `explain` get a reply.

## Labels

- `verdict/pass|review|wait`, `state/queued|deploying|soaking|live|unhealthy|hold`, `kind/security|proposal|revert`.
- Never remove `state/unhealthy` without fixing the cause: it halts merging.

## Renovate

Renovate (`bot-cecil`) opens dependency updates and Clarence merges the eligible ones itself. Don't push to Renovate's branches; close the pull request or tick Renovate's rebase box instead.
