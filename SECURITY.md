# Security policy

## Supported versions

Sona is before 1.0. Security fixes go into the latest release and the latest
release candidate. Development builds from `main` are fixed on `main`.

## Report a vulnerability

Report it privately through GitHub:
[Report a vulnerability](https://github.com/coredev-uk/sona-go/security/advisories/new)
(the repository's Security tab → Advisories → Report a vulnerability). Do not
open a public issue, pull request or discussion.

Include what is affected (the version from `sona --version` or Settings →
Diagnostics, the platform and how you installed Sona), the steps to reproduce,
and what an attacker gains. Leave out your Apple Music token, cookies and
account data.

## What to expect

You get an answer in the advisory. Once a fix is released, the advisory is
published with credit to you unless you ask otherwise.

## Scope

In scope: Sona's own code and release packages, including credential handling,
the sign-in server, the Widevine download and its verification, the sandboxed
playback host, `sona` and `music://` link handling, settings import and the
update check. [docs/security-review.md](docs/security-review.md) lists the
known findings and the threat model.

Out of scope: vulnerabilities in Apple's services, Google's Widevine CDM, MyGo
or other dependencies (report those upstream), and attacks that need an
already compromised account.

Never send or publish Widevine keys, device certificates or ways around Apple's
or Google's protection. Such reports are closed.

## Privacy in brief

- Sona stores your Apple Music token in the system keyring.
- Private listening keeps plays out of your history, scrobbles and Discord.
- Widevine runs in a sandboxed helper with no network access.

Details: [Privacy and Security](https://sona.coredev.uk/privacy-and-security/).
